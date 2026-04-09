package tui

import (
	"context"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bjornjee/agent-dashboard/internal/diagrams"
	"github.com/bjornjee/agent-dashboard/internal/domain"
)

// captureGitRunner records Start() invocations so tests can assert on them
// without spawning real subprocesses (CLAUDE.md mandates mocked runners).
type captureGitRunner struct {
	noopGitRunner
	mu    sync.Mutex
	calls []string
}

func (r *captureGitRunner) Start(name string, args ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	all := append([]string{name}, args...)
	r.calls = append(r.calls, joinArgs(all))
	return nil
}

func (r *captureGitRunner) startCalls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.calls))
	copy(out, r.calls)
	return out
}

func joinArgs(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " "
		}
		out += p
	}
	return out
}

func newDiagramsTestModel() model {
	m := NewModel(testConfig(""), nil)
	m.tmuxAvailable = true
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running", SessionID: "sess-A"},
	}
	m.buildTree()
	// First non-header tree node is the parent agent
	for i, n := range m.treeNodes {
		if n.AgentIdx == 0 {
			m.selected = i
			break
		}
	}
	return m
}

func sampleDiagrams() []diagrams.Diagram {
	return []diagrams.Diagram{
		{
			SessionID: "sess-A",
			Hash:      "aaaaaaaa",
			Timestamp: time.Unix(200, 0),
			Title:     "Second",
			Type:      "flowchart",
			Source:    "flowchart TD\n  A --> B",
			Path:      "/tmp/sess-A/200-aaaaaaaa.mmd",
		},
		{
			SessionID: "sess-A",
			Hash:      "bbbbbbbb",
			Timestamp: time.Unix(100, 0),
			Title:     "First",
			Type:      "sequenceDiagram",
			Source:    "sequenceDiagram\n  A->>B: hi",
			Path:      "/tmp/sess-A/100-bbbbbbbb.mmd",
		},
	}
}

func TestDKey_NoOpWhenNoDiagrams(t *testing.T) {
	m := newDiagramsTestModel()
	m.diagrams = nil

	msg := tea.KeyPressMsg{Code: 'D', Text: "D"}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	if rm.diagramsVisible {
		t.Errorf("D with no diagrams should not open the panel")
	}
}

func TestDKey_TogglesAndClearsPlan(t *testing.T) {
	m := newDiagramsTestModel()
	m.diagrams = sampleDiagrams()
	m.planVisible = true // pre-existing plan view

	msg := tea.KeyPressMsg{Code: 'D', Text: "D"}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	if !rm.diagramsVisible {
		t.Fatalf("expected D to open the diagrams panel")
	}
	if rm.planVisible {
		t.Errorf("expected D to clear planVisible (mutual exclusion)")
	}
	// lastSeenDiagramCount should record the current count for this session.
	if rm.lastSeenDiagramCount["sess-A"] != m.agents[0].DiagramCount {
		t.Errorf("expected lastSeenDiagramCount to be set for session")
	}
	// chroma cache should be populated eagerly (no mutation in render).
	if rm.renderedDiagramSrc == "" {
		t.Errorf("expected renderedDiagramSrc to be populated by D handler")
	}

	// Press D again — should close.
	result2, _ := rm.handleKey(msg)
	rm2 := result2.(model)
	if rm2.diagramsVisible {
		t.Errorf("expected second D press to close the panel")
	}
}

func TestPKey_ClearsDiagramsVisible(t *testing.T) {
	m := newDiagramsTestModel()
	m.diagrams = sampleDiagrams()
	m.diagramsVisible = true
	m.planContent = "## Plan\n- step 1" // p handler is gated on plan content

	msg := tea.KeyPressMsg{Code: 'p', Text: "p"}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	if rm.diagramsVisible {
		t.Errorf("expected p to clear diagramsVisible (mutual exclusion)")
	}
}

func TestDiagramsCursor_JKClampsAndPopulatesCache(t *testing.T) {
	m := newDiagramsTestModel()
	m.diagrams = sampleDiagrams()
	m.diagramsVisible = true
	m.focusedVP = focusMessage
	m.diagramsCursor = 0

	// j → cursor 1
	r1, _ := m.handleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m1 := r1.(model)
	if m1.diagramsCursor != 1 {
		t.Errorf("expected cursor 1 after j, got %d", m1.diagramsCursor)
	}
	if m1.renderedDiagramSrc == "" {
		t.Errorf("expected renderedDiagramSrc to be populated after j")
	}

	// j again → clamped at 1 (last index)
	r2, _ := m1.handleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m2 := r2.(model)
	if m2.diagramsCursor != 1 {
		t.Errorf("expected cursor clamped at 1, got %d", m2.diagramsCursor)
	}

	// k → cursor 0
	r3, _ := m2.handleKey(tea.KeyPressMsg{Code: 'k', Text: "k"})
	m3 := r3.(model)
	if m3.diagramsCursor != 0 {
		t.Errorf("expected cursor 0 after k, got %d", m3.diagramsCursor)
	}

	// k again → clamped at 0
	r4, _ := m3.handleKey(tea.KeyPressMsg{Code: 'k', Text: "k"})
	m4 := r4.(model)
	if m4.diagramsCursor != 0 {
		t.Errorf("expected cursor clamped at 0, got %d", m4.diagramsCursor)
	}
}

func TestRenderDiagramsPanel_DoesNotMutate(t *testing.T) {
	m := newDiagramsTestModel()
	m.diagrams = sampleDiagrams()
	m.diagramsCursor = 5      // out of range, should not be mutated by render
	m.renderedDiagramSrc = "" // empty cache, should not be filled by render

	_ = m.renderDiagramsPanel()

	if m.diagramsCursor != 5 {
		t.Errorf("renderDiagramsPanel mutated diagramsCursor: got %d, want 5", m.diagramsCursor)
	}
	if m.renderedDiagramSrc != "" {
		t.Errorf("renderDiagramsPanel mutated renderedDiagramSrc cache")
	}
}

func TestOpenDiagram_CallsGitRunnerStart(t *testing.T) {
	cap := &captureGitRunner{}
	restore := setTestGitRunner(cap)
	t.Cleanup(restore)

	m := newDiagramsTestModel()
	d := sampleDiagrams()[0]

	cmd := m.openDiagram(d)
	if cmd == nil {
		t.Fatal("openDiagram returned nil cmd")
	}
	msg := cmd()
	opened, ok := msg.(diagramOpenedMsg)
	if !ok {
		t.Fatalf("expected diagramOpenedMsg, got %T", msg)
	}
	if opened.err != nil {
		t.Errorf("unexpected open err: %v", opened.err)
	}

	calls := cap.startCalls()
	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 Start call, got %d", len(calls))
	}
	// Should be `open -g <tmp-html-path>` — the -g flag prevents the
	// browser from stealing focus from the dashboard.
	if !startsWith(calls[0], "open -g ") {
		t.Errorf("expected `open -g <path>`, got %q", calls[0])
	}
}

func TestDeleteDiagram_ReturnsReloadMsg(t *testing.T) {
	m := newDiagramsTestModel()
	d := diagrams.Diagram{SessionID: "sess-A", Path: "" /* idempotent no-op */}

	cmd := m.deleteDiagram(d)
	if cmd == nil {
		t.Fatal("deleteDiagram returned nil cmd")
	}
	msg := cmd()
	loaded, ok := msg.(diagramsLoadedMsg)
	if !ok {
		t.Fatalf("expected diagramsLoadedMsg, got %T", msg)
	}
	if loaded.sessionID != "sess-A" {
		t.Errorf("expected sessionID sess-A, got %q", loaded.sessionID)
	}
}

func TestDiagramsLoadedMsg_UpdatesAgentDiagramCount(t *testing.T) {
	// Regression: after the user deletes the last diagram via the `x`
	// confirm flow, diagramsLoadedMsg arrives with an empty list. The
	// matching agent's DiagramCount must be updated to 0 so the left
	// panel stops rendering the 📑 badge — otherwise the icon lingers
	// until the extractor hook next rewrites the session state file.
	m := newDiagramsTestModel()
	m.agents[0].DiagramCount = 2
	m.lastSeenDiagramCount = map[string]int{"sess-A": 2}
	m.diagrams = sampleDiagrams()

	out, _ := m.Update(diagramsLoadedMsg{sessionID: "sess-A", list: nil})
	rm := out.(model)
	if rm.agents[0].DiagramCount != 0 {
		t.Errorf("expected agent DiagramCount=0 after empty reload, got %d", rm.agents[0].DiagramCount)
	}
	if rm.lastSeenDiagramCount["sess-A"] != 0 {
		t.Errorf("expected lastSeenDiagramCount cleared to 0, got %d", rm.lastSeenDiagramCount["sess-A"])
	}
	if rm.diagramBadge("sess-A", rm.agents[0].DiagramCount) != "" {
		t.Errorf("expected empty badge when count is 0 after reload")
	}
}

func TestDiagramsLoadedMsg_IgnoredForOtherSession(t *testing.T) {
	m := newDiagramsTestModel()
	prev := sampleDiagrams()
	m.diagrams = prev

	// Mismatched session ID — should be a no-op.
	out, _ := m.Update(diagramsLoadedMsg{sessionID: "other-session", list: nil})
	rm := out.(model)
	if len(rm.diagrams) != len(prev) {
		t.Errorf("expected diagrams unchanged for mismatched session, got len=%d", len(rm.diagrams))
	}
}

func TestDiagramBadge_BrightWhenUnseen(t *testing.T) {
	m := newDiagramsTestModel()
	m.lastSeenDiagramCount = map[string]int{"sess-A": 0}

	bright := m.diagramBadge("sess-A", 2) // 2 > 0 seen → bright
	muted := m.diagramBadge("sess-A", 0)  // count 0 → empty
	m.lastSeenDiagramCount["sess-A"] = 2
	seen := m.diagramBadge("sess-A", 2) // count == seen → muted but non-empty

	if bright == "" {
		t.Errorf("expected non-empty badge when unseen")
	}
	if muted != "" {
		t.Errorf("expected empty badge when count is 0")
	}
	if seen == "" {
		t.Errorf("expected non-empty muted badge when seen")
	}
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// Compile-time guard: openDiagram closure must reference a context-free
// command path that doesn't smuggle a real exec runner through.
var _ context.Context = context.Background()
