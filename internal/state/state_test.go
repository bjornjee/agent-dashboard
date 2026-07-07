package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/conversation"
	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/mocks"
	"github.com/stretchr/testify/mock"
)

// writeAgentFile is a test helper that writes an agent JSON file to dir/agents/.
func writeAgentFile(t *testing.T, dir, filename string, agent domain.Agent) {
	t.Helper()
	agentsDir := filepath.Join(dir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatalf("create agents dir: %v", err)
	}
	data, err := json.Marshal(agent)
	if err != nil {
		t.Fatalf("marshal agent: %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentsDir, filename), data, 0644); err != nil {
		t.Fatalf("write agent file: %v", err)
	}
}

func TestReadState_MissingDir(t *testing.T) {
	sf := readState("/nonexistent/path")
	if len(sf.Agents) != 0 {
		t.Errorf("expected empty agents, got %d", len(sf.Agents))
	}
}

func TestReadState_EmptyAgentsDir(t *testing.T) {
	tmp := t.TempDir()
	os.MkdirAll(filepath.Join(tmp, "agents"), 0755)

	sf := readState(tmp)
	if len(sf.Agents) != 0 {
		t.Errorf("expected empty agents, got %d", len(sf.Agents))
	}
}

func TestReadState_InvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	agentsDir := filepath.Join(tmp, "agents")
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(filepath.Join(agentsDir, "bad.json"), []byte("not json{{{"), 0644)

	sf := readState(tmp)
	if len(sf.Agents) != 0 {
		t.Errorf("expected empty agents for invalid JSON, got %d", len(sf.Agents))
	}
}

func TestReadState_ValidState(t *testing.T) {
	tmp := t.TempDir()
	writeAgentFile(t, tmp, "sess-a.json", domain.Agent{SessionID: "sess-a", Target: "a:0.1", State: "running", Session: "a", TmuxPaneID: "%1"})
	writeAgentFile(t, tmp, "sess-b.json", domain.Agent{SessionID: "sess-b", Target: "b:1.0", State: "question", Session: "b", TmuxPaneID: "%2"})

	sf := readState(tmp)
	if len(sf.Agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(sf.Agents))
	}
	if sf.Agents["sess-a"].State != "running" {
		t.Errorf("expected running, got %s", sf.Agents["sess-a"].State)
	}
}

func TestReadState_FallbackToFilename(t *testing.T) {
	tmp := t.TempDir()
	// domain.Agent without session_id — should use filename stem as key
	writeAgentFile(t, tmp, "fallback-key.json", domain.Agent{Target: "a:0.1", State: "running", Session: "a"})

	sf := readState(tmp)
	if len(sf.Agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(sf.Agents))
	}
	if _, ok := sf.Agents["fallback-key"]; !ok {
		t.Error("expected agent keyed by filename stem 'fallback-key'")
	}
}

func TestSortedAgents_Priority(t *testing.T) {
	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			"s5": {Target: "a:5.0", State: "done", Window: 5, Pane: 0, TmuxPaneID: "%5"},
			"s1": {Target: "a:1.0", State: "question", Window: 1, Pane: 0, TmuxPaneID: "%1"},
			"s3": {Target: "a:3.0", State: "running", Window: 3, Pane: 0, TmuxPaneID: "%3"},
			"s0": {Target: "a:0.0", State: "permission", Window: 0, Pane: 0, TmuxPaneID: "%0"},
			"s4": {Target: "a:4.0", State: "idle_prompt", Window: 4, Pane: 0, TmuxPaneID: "%4"},
			"s2": {Target: "a:2.0", State: "error", Window: 2, Pane: 0, TmuxPaneID: "%2"},
		},
	}

	sorted := SortedAgents(sf, "")

	// Group 1: blocked (permission) sorted by window
	// Group 2: waiting (question, error) sorted by window
	// Group 3: running
	// Group 4: review (idle_prompt, done) sorted by window
	expected := []string{"permission", "question", "error", "running", "idle_prompt", "done"}

	if len(sorted) != 6 {
		t.Fatalf("expected 6 agents, got %d", len(sorted))
	}
	for i, want := range expected {
		if sorted[i].State != want {
			t.Errorf("position %d: expected %s, got %s", i, want, sorted[i].State)
		}
	}
}

func TestSortedAgents_ExcludesSelfByPaneID(t *testing.T) {
	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			"s1": {Target: "a:0.0", State: "running", TmuxPaneID: "%5"},
			"s2": {Target: "a:1.0", State: "running", TmuxPaneID: "%6"},
		},
	}

	sorted := SortedAgents(sf, "%5")
	if len(sorted) != 1 {
		t.Fatalf("expected 1 agent (self excluded), got %d", len(sorted))
	}
	if sorted[0].TmuxPaneID != "%6" {
		t.Errorf("expected pane %%6 to survive, got %s", sorted[0].TmuxPaneID)
	}
}

func TestSortedAgents_SkipsEmptyState(t *testing.T) {
	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			"good":  {Target: "good", State: "running", TmuxPaneID: "%1"},
			"good2": {Target: "", State: "running"}, // empty target is ok
			"bad":   {Target: "bad", State: ""},     // empty state -> skipped
		},
	}

	sorted := SortedAgents(sf, "")
	if len(sorted) != 2 {
		t.Errorf("expected 2 valid agents (only empty-state skipped), got %d", len(sorted))
	}
}

func TestRemoveAgent(t *testing.T) {
	tmp := t.TempDir()
	writeAgentFile(t, tmp, "sess-a.json", domain.Agent{SessionID: "sess-a", Target: "a:0.1", State: "running", TmuxPaneID: "%1"})
	writeAgentFile(t, tmp, "sess-b.json", domain.Agent{SessionID: "sess-b", Target: "b:1.0", State: "question", TmuxPaneID: "%2"})

	err := removeAgent(tmp, "sess-a")
	if err != nil {
		t.Fatalf("removeAgent failed: %v", err)
	}

	sf := readState(tmp)
	if len(sf.Agents) != 1 {
		t.Fatalf("expected 1 agent after removal, got %d", len(sf.Agents))
	}
	if _, ok := sf.Agents["sess-a"]; ok {
		t.Error("agent sess-a should have been removed")
	}
	if _, ok := sf.Agents["sess-b"]; !ok {
		t.Error("agent sess-b should still exist")
	}
}

func TestRemoveAgent_NonExistent(t *testing.T) {
	tmp := t.TempDir()
	writeAgentFile(t, tmp, "sess-a.json", domain.Agent{SessionID: "sess-a", Target: "a:0.1", State: "running"})

	err := removeAgent(tmp, "nonexistent")
	if err != nil {
		t.Fatalf("removeAgent should not fail on nonexistent session_id: %v", err)
	}

	sf := readState(tmp)
	if len(sf.Agents) != 1 {
		t.Errorf("expected 1 agent unchanged, got %d", len(sf.Agents))
	}
}

func TestReadAgent(t *testing.T) {
	tmp := t.TempDir()
	writeAgentFile(t, tmp, "sess-a.json", domain.Agent{
		SessionID:   "sess-a",
		Harness:     "codex",
		State:       "running",
		Cwd:         "/repo",
		WorktreeCwd: "/repo/.worktrees/feat",
		Branch:      "feat/x",
		TmuxPaneID:  "%7",
	})

	got, ok := ReadAgent(tmp, "sess-a")
	if !ok {
		t.Fatal("ReadAgent should find sess-a")
	}
	if got.Harness != "codex" || got.WorktreeCwd != "/repo/.worktrees/feat" || got.Cwd != "/repo" {
		t.Errorf("ReadAgent returned wrong fields: %+v", got)
	}
	if got.EffectiveDir() != "/repo/.worktrees/feat" {
		t.Errorf("EffectiveDir() = %q, want worktree cwd", got.EffectiveDir())
	}

	if _, ok := ReadAgent(tmp, "nonexistent"); ok {
		t.Error("ReadAgent should return false for unknown session id")
	}
}

func TestPruneDead_ByPaneID(t *testing.T) {
	tmp := t.TempDir()
	writeAgentFile(t, tmp, "sess-a.json", domain.Agent{SessionID: "sess-a", Target: "main:1.0", State: "running", TmuxPaneID: "%1"})
	writeAgentFile(t, tmp, "sess-b.json", domain.Agent{SessionID: "sess-b", Target: "main:1.1", State: "done", TmuxPaneID: "%2"})

	livePaneIDs := map[string]bool{
		"%0": true, // dashboard
		"%1": true, // agent A alive
		// %2 is dead
	}

	removed := PruneDead(tmp, livePaneIDs, "100", nil)
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}

	sf := readState(tmp)
	if len(sf.Agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(sf.Agents))
	}
	if _, ok := sf.Agents["sess-a"]; !ok {
		t.Error("sess-a should survive")
	}
}

// TestPruneDead_SweepsOrphanRows covers the PruneDead → sweepDeadRows wiring
// end-to-end: knownSessions built from all parsed files, livePaneIDs and
// serverPID forwarded, file-backed and orphan rows treated differently.
func TestPruneDead_SweepsOrphanRows(t *testing.T) {
	store, d := testStore(t)
	tmp := t.TempDir()

	live := domain.Agent{SessionID: "live", Target: "main:1.0", State: "running", TmuxPaneID: "%1", UpdatedAt: "2026-07-06T10:00:00Z"}
	deadfile := domain.Agent{SessionID: "deadfile", Target: "main:1.1", State: "running", TmuxPaneID: "%2", UpdatedAt: "2026-07-06T10:00:00Z"}
	orphan := domain.Agent{SessionID: "orphan", Target: "main:1.2", State: "running", TmuxPaneID: "%3", UpdatedAt: "2026-07-06T10:00:00Z"}

	// live and deadfile have hook files; orphan exists only as a DB row
	// (its file vanished while no dashboard was running).
	writeAgentFile(t, tmp, "live.json", live)
	writeAgentFile(t, tmp, "deadfile.json", deadfile)
	store.sync(&domain.StateFile{Agents: map[string]domain.Agent{
		"live": live, "deadfile": deadfile, "orphan": orphan,
	}})

	removed := PruneDead(tmp, map[string]bool{"%0": true, "%1": true}, "100", nil, store)
	if removed != 1 {
		t.Fatalf("expected 1 file removed, got %d", removed)
	}

	type row struct {
		SessionID       string  `db:"session_id"`
		DismissedReason *string `db:"dismissed_reason"`
	}
	var rows []row
	if err := d.Conn().Select(&rows, "SELECT session_id, dismissed_reason FROM agents ORDER BY session_id"); err != nil {
		t.Fatalf("select: %v", err)
	}
	got := map[string]string{}
	for _, r := range rows {
		reason := ""
		if r.DismissedReason != nil {
			reason = *r.DismissedReason
		}
		got[r.SessionID] = reason
	}
	want := map[string]string{
		"live":     "",          // pane alive, file kept
		"deadfile": "dead_pane", // file pruned this cycle → dismissed by dismissPruned
		"orphan":   "dead_pane", // no file, dead pane → swept
	}
	for id, reason := range want {
		if got[id] != reason {
			t.Fatalf("session %q dismissed_reason = %q, want %q", id, got[id], reason)
		}
	}
}

func TestPruneDead_EmptyLivePanes(t *testing.T) {
	tmp := t.TempDir()
	writeAgentFile(t, tmp, "sess-a.json", domain.Agent{SessionID: "sess-a", Target: "main:1.0", State: "running", TmuxPaneID: "%1"})
	writeAgentFile(t, tmp, "sess-b.json", domain.Agent{SessionID: "sess-b", Target: "main:2.0", State: "running", TmuxPaneID: "%2"})

	// Empty non-nil map simulates tmux returning success with empty output.
	// PruneDead should refuse to delete all agents in this case.
	livePaneIDs := map[string]bool{}
	removed := PruneDead(tmp, livePaneIDs, "100", nil)
	if removed != 0 {
		t.Errorf("expected 0 removed with empty livePaneIDs, got %d", removed)
	}

	sf := readState(tmp)
	if len(sf.Agents) != 2 {
		t.Fatalf("expected 2 agents preserved, got %d", len(sf.Agents))
	}
}

func TestPruneDead_AllAgentsDead(t *testing.T) {
	tmp := t.TempDir()
	writeAgentFile(t, tmp, "sess-a.json", domain.Agent{SessionID: "sess-a", Target: "main:1.0", State: "running", TmuxPaneID: "%1"})
	writeAgentFile(t, tmp, "sess-b.json", domain.Agent{SessionID: "sess-b", Target: "main:2.0", State: "running", TmuxPaneID: "%2"})

	// livePaneIDs has panes but none match any agent — all would be deleted.
	// Safety net should refuse to wipe all agents at once.
	livePaneIDs := map[string]bool{
		"%0": true, // dashboard pane only
	}
	removed := PruneDead(tmp, livePaneIDs, "100", nil)
	if removed != 0 {
		t.Errorf("expected 0 removed when all agents would be wiped, got %d", removed)
	}

	sf := readState(tmp)
	if len(sf.Agents) != 2 {
		t.Fatalf("expected 2 agents preserved, got %d", len(sf.Agents))
	}
}

func TestPruneDead_PartialDead(t *testing.T) {
	tmp := t.TempDir()
	writeAgentFile(t, tmp, "sess-a.json", domain.Agent{SessionID: "sess-a", Target: "main:1.0", State: "running", TmuxPaneID: "%1"})
	writeAgentFile(t, tmp, "sess-b.json", domain.Agent{SessionID: "sess-b", Target: "main:1.1", State: "running", TmuxPaneID: "%2"})
	writeAgentFile(t, tmp, "sess-c.json", domain.Agent{SessionID: "sess-c", Target: "main:2.0", State: "done", TmuxPaneID: "%3"})

	livePaneIDs := map[string]bool{
		"%0": true,
		"%1": true,
		"%2": true,
		// %3 is dead
	}

	removed := PruneDead(tmp, livePaneIDs, "100", nil)
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}

	sf := readState(tmp)
	if len(sf.Agents) != 2 {
		t.Fatalf("expected 2 agents after partial prune, got %d", len(sf.Agents))
	}
	if _, ok := sf.Agents["sess-c"]; ok {
		t.Error("sess-c should have been pruned")
	}
}

func TestPruneDead_NoPaneID(t *testing.T) {
	tmp := t.TempDir()
	// domain.Agent without TmuxPaneID — should be considered dead
	writeAgentFile(t, tmp, "sess-a.json", domain.Agent{SessionID: "sess-a", Target: "main:1.0", State: "running"})
	writeAgentFile(t, tmp, "sess-b.json", domain.Agent{SessionID: "sess-b", Target: "main:2.0", State: "running", TmuxPaneID: "%2"})

	livePaneIDs := map[string]bool{
		"%2": true,
	}

	removed := PruneDead(tmp, livePaneIDs, "100", nil)
	if removed != 1 {
		t.Errorf("expected 1 removed (agent without pane ID), got %d", removed)
	}
}

func TestPruneDead_DedupSamePane(t *testing.T) {
	tmp := t.TempDir()
	recentTime := time.Now().UTC().Format(time.RFC3339)
	staleTime := "2020-01-01T00:00:00Z"

	// Two agents sharing pane %42 — both panes alive.
	// PruneDead should keep only the newest and remove the older duplicate.
	writeAgentFile(t, tmp, "old-session.json", domain.Agent{
		SessionID:  "old-session",
		TmuxPaneID: "%42",
		State:      "running",
		UpdatedAt:  staleTime,
	})
	writeAgentFile(t, tmp, "new-session.json", domain.Agent{
		SessionID:  "new-session",
		TmuxPaneID: "%42",
		State:      "running",
		UpdatedAt:  recentTime,
	})

	livePaneIDs := map[string]bool{"%42": true}
	removed := PruneDead(tmp, livePaneIDs, "100", nil)
	if removed != 1 {
		t.Errorf("expected 1 removed (older duplicate pane agent), got %d", removed)
	}

	agentsPath := filepath.Join(tmp, "agents")
	if _, err := os.Stat(filepath.Join(agentsPath, "old-session.json")); err == nil {
		t.Error("old duplicate-pane agent was not cleaned by PruneDead")
	}
	if _, err := os.Stat(filepath.Join(agentsPath, "new-session.json")); err != nil {
		t.Error("newest agent for live pane was incorrectly removed")
	}
}

func TestPruneDead_DedupOnDeadPane(t *testing.T) {
	tmp := t.TempDir()
	recentTime := time.Now().UTC().Format(time.RFC3339)
	staleTime := "2020-01-01T00:00:00Z"

	// Two agents sharing pane %42 which is dead (not in livePaneIDs).
	// The dedup removal of the older agent should still happen even though
	// the safety net prevents removing all dead agents at once.
	writeAgentFile(t, tmp, "old-session.json", domain.Agent{
		SessionID:  "old-session",
		TmuxPaneID: "%42",
		State:      "running",
		UpdatedAt:  staleTime,
	})
	writeAgentFile(t, tmp, "new-session.json", domain.Agent{
		SessionID:  "new-session",
		TmuxPaneID: "%42",
		State:      "running",
		UpdatedAt:  recentTime,
	})

	livePaneIDs := map[string]bool{} // pane %42 is dead
	removed := PruneDead(tmp, livePaneIDs, "100", nil)
	if removed != 1 {
		t.Errorf("expected 1 removed (dedup of older agent on dead pane), got %d", removed)
	}

	agentsPath := filepath.Join(tmp, "agents")
	// Old duplicate should always be cleaned (dedup is unconditional)
	if _, err := os.Stat(filepath.Join(agentsPath, "old-session.json")); err == nil {
		t.Error("old duplicate-pane agent was not cleaned by PruneDead dedup")
	}
	// Newest agent is kept by the safety net (sole remaining dead agent)
	if _, err := os.Stat(filepath.Join(agentsPath, "new-session.json")); err != nil {
		t.Error("newest agent was incorrectly removed")
	}
}

// survivorAgent returns an agent that passes every IsResumableOrphan check
// against current server PID "100": stamped by a previous tmux server ("99"),
// dead pane, active state, real branch, existing workdir, fresh timestamp.
// Tests override individual fields to isolate each check.
func survivorAgent(t *testing.T) domain.Agent {
	t.Helper()
	return domain.Agent{
		SessionID:     "s",
		State:         "running",
		TmuxPaneID:    "%2",
		TmuxServerPID: "99",
		Branch:        "feat/x",
		Cwd:           t.TempDir(),
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
	}
}

func TestIsResumableOrphan(t *testing.T) {
	live := map[string]bool{"%1": true}
	now := time.Now()
	tests := []struct {
		name   string
		mutate func(*domain.Agent)
		want   bool
	}{
		{"running survivor with dead pane", func(a *domain.Agent) {}, true},
		{"idle_prompt survivor with dead pane", func(a *domain.Agent) { a.State = "idle_prompt" }, true},
		{"question survivor with dead pane", func(a *domain.Agent) { a.State = "question" }, true},
		{"live pane is not an orphan", func(a *domain.Agent) { a.TmuxPaneID = "%1" }, false},
		{"no session id", func(a *domain.Agent) { a.SessionID = "" }, false},
		{"no state", func(a *domain.Agent) { a.State = "" }, false},
		{"no pane id (never had one)", func(a *domain.Agent) { a.TmuxPaneID = "" }, false},
		{"done is finished, not resumable", func(a *domain.Agent) { a.State = "done" }, false},
		{"pr is finished, not resumable", func(a *domain.Agent) { a.State = "pr" }, false},
		{"merged is finished, not resumable", func(a *domain.Agent) { a.State = "merged" }, false},
		{"empty updated_at skips the TTL check", func(a *domain.Agent) { a.UpdatedAt = "" }, true},
		{"worktree_cwd preferred when set", func(a *domain.Agent) { a.WorktreeCwd = a.Cwd; a.Cwd = "/nonexistent" }, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			agent := survivorAgent(t)
			tc.mutate(&agent)
			if got := IsResumableOrphan(agent, live, "100", now); got != tc.want {
				t.Errorf("IsResumableOrphan(%+v) = %v, want %v", agent, got, tc.want)
			}
		})
	}
}

// Event scope: only panes that died with their tmux server are survivors. A
// pane that died while the current server kept running was closed on purpose,
// and pre-upgrade files without a stamped server PID are never resumable.
func TestIsResumableOrphan_EventScoped(t *testing.T) {
	live := map[string]bool{"%1": true}
	now := time.Now()
	tests := []struct {
		name   string
		mutate func(*domain.Agent)
		want   bool
	}{
		{"pane died under the current server (deliberate close)", func(a *domain.Agent) { a.TmuxServerPID = "100" }, false},
		{"no stamped server pid (pre-upgrade file)", func(a *domain.Agent) { a.TmuxServerPID = "" }, false},
		{"pane died with a previous server (genuine survivor)", func(a *domain.Agent) {}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			agent := survivorAgent(t)
			tc.mutate(&agent)
			if got := IsResumableOrphan(agent, live, "100", now); got != tc.want {
				t.Errorf("IsResumableOrphan(%+v) = %v, want %v", agent, got, tc.want)
			}
		})
	}
}

// Noise classes observed in the field (see fix/resumable-ux): an ad-hoc
// session with no branch, an agent whose pinned_state already marks the work
// shipped, a survivor whose workdir was cleaned up, and a survivor older than
// the TTL must not be offered as resumable orphans.
func TestIsResumableOrphan_NoiseClasses(t *testing.T) {
	live := map[string]bool{"%1": true}
	now := time.Now()
	tests := []struct {
		name   string
		mutate func(*domain.Agent)
		want   bool
	}{
		{
			"ad-hoc session without a branch is not resumable",
			func(a *domain.Agent) { a.Branch = "" },
			false,
		},
		{
			"pinned pr with raw idle_prompt is finished, not resumable",
			func(a *domain.Agent) { a.State = "idle_prompt"; a.PinnedState = "pr" },
			false,
		},
		{
			"pinned merged with raw done is finished, not resumable",
			func(a *domain.Agent) { a.State = "done"; a.PinnedState = "merged" },
			false,
		},
		{
			"missing workdir is not resumable (resume would fail)",
			func(a *domain.Agent) { a.Cwd = "/nonexistent-resumable-test-dir" },
			false,
		},
		{
			"survivor older than the TTL is stale",
			func(a *domain.Agent) { a.UpdatedAt = now.Add(-resumableTTL - time.Hour).UTC().Format(time.RFC3339) },
			false,
		},
		{
			"survivor just inside the TTL is kept",
			func(a *domain.Agent) { a.UpdatedAt = now.Add(-resumableTTL + time.Hour).UTC().Format(time.RFC3339) },
			true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			agent := survivorAgent(t)
			tc.mutate(&agent)
			if got := IsResumableOrphan(agent, live, "100", now); got != tc.want {
				t.Errorf("IsResumableOrphan(%+v) = %v, want %v", agent, got, tc.want)
			}
		})
	}
}

// A nil live-pane set means tmux enumeration FAILED — we cannot tell a live
// pane from a dead one, so no agent may be classified an orphan (else a
// transient failure would mark live agents resumable and let a resume delete a
// live session's state file). A non-nil empty set means tmux succeeded with
// zero panes — genuinely all dead, so the agent IS an orphan. An empty server
// PID can only accompany a failed enumeration and is fenced the same way.
func TestIsResumableOrphan_NilVsEmptyLiveSet(t *testing.T) {
	agent := survivorAgent(t)
	now := time.Now()
	if IsResumableOrphan(agent, nil, "100", now) {
		t.Error("nil live set (tmux failed) must NOT classify an agent as an orphan")
	}
	if !IsResumableOrphan(agent, map[string]bool{}, "100", now) {
		t.Error("empty non-nil live set (genuinely zero panes) should classify a dead agent as an orphan")
	}
	if IsResumableOrphan(agent, map[string]bool{}, "", now) {
		t.Error("empty server PID (enumeration failed) must NOT classify an agent as an orphan")
	}
}

// After a restart, resuming one orphan gives it a live pane. PruneDead must not
// then cascade-delete the remaining restart-survivors. Finished agents (done/
// pr/merged) on dead panes are still GC'd.
func TestPruneDead_RetainsActiveOrphans(t *testing.T) {
	tmp := t.TempDir()
	survivor := func(sid, state, pane string) domain.Agent {
		a := survivorAgent(t)
		a.SessionID = sid
		a.State = state
		a.TmuxPaneID = pane
		return a
	}
	writeAgentFile(t, tmp, "live.json", survivor("live", "running", "%1"))
	writeAgentFile(t, tmp, "orphan.json", survivor("orphan", "running", "%2"))
	writeAgentFile(t, tmp, "idle.json", survivor("idle", "idle_prompt", "%4"))
	writeAgentFile(t, tmp, "finished.json", survivor("finished", "done", "%3"))

	livePaneIDs := map[string]bool{"%0": true, "%1": true} // %2, %3, %4 dead

	removed := PruneDead(tmp, livePaneIDs, "100", nil)
	if removed != 1 {
		t.Errorf("expected 1 removed (only the finished done agent), got %d", removed)
	}

	sf := readState(tmp)
	if _, ok := sf.Agents["orphan"]; !ok {
		t.Error("active (running) survivor should be retained for resume (no cascade prune)")
	}
	if _, ok := sf.Agents["idle"]; !ok {
		t.Error("active (idle_prompt) survivor should be retained for resume")
	}
	if _, ok := sf.Agents["live"]; !ok {
		t.Error("live agent should be retained")
	}
	if _, ok := sf.Agents["finished"]; ok {
		t.Error("finished done agent on a dead pane should be pruned")
	}
}

// A pane closed while the current tmux server kept running is a deliberate
// close — PruneDead removes it like before the resume feature existed.
func TestPruneDead_DeliberateCloseIsPruned(t *testing.T) {
	tmp := t.TempDir()
	closed := survivorAgent(t)
	closed.SessionID = "closed"
	closed.TmuxServerPID = "100" // same server as current — closed on purpose
	writeAgentFile(t, tmp, "closed.json", closed)
	keep := survivorAgent(t)
	keep.SessionID = "keep"
	keep.TmuxPaneID = "%1"
	writeAgentFile(t, tmp, "keep.json", keep)

	removed := PruneDead(tmp, map[string]bool{"%0": true, "%1": true}, "100", nil)
	if removed != 1 {
		t.Errorf("expected 1 removed (deliberately closed pane), got %d", removed)
	}
	sf := readState(tmp)
	if _, ok := sf.Agents["closed"]; ok {
		t.Error("deliberately closed agent should be pruned")
	}
	if _, ok := sf.Agents["keep"]; !ok {
		t.Error("live agent should be retained")
	}
}

// Survivors whose branch is already merged shipped their work — GC'd when the
// caller supplies a merged checker. The checker receives the survivor's
// EffectiveDir and branch.
func TestPruneDead_MergedSurvivorIsPruned(t *testing.T) {
	tmp := t.TempDir()
	merged := survivorAgent(t)
	merged.SessionID = "merged-branch"
	merged.Branch = "feat/shipped"
	writeAgentFile(t, tmp, "merged.json", merged)
	open := survivorAgent(t)
	open.SessionID = "open-branch"
	open.TmuxPaneID = "%3"
	open.Branch = "feat/open"
	writeAgentFile(t, tmp, "open.json", open)
	live := survivorAgent(t)
	live.SessionID = "live"
	live.TmuxPaneID = "%1"
	writeAgentFile(t, tmp, "live.json", live)

	var gotDir, gotBranch string
	isMerged := func(dir, branch string) bool {
		if branch == "feat/shipped" {
			gotDir, gotBranch = dir, branch
			return true
		}
		return false
	}

	removed := PruneDead(tmp, map[string]bool{"%0": true, "%1": true}, "100", isMerged)
	if removed != 1 {
		t.Errorf("expected 1 removed (merged survivor), got %d", removed)
	}
	if gotDir != merged.EffectiveDir() || gotBranch != "feat/shipped" {
		t.Errorf("merged checker got (%q, %q), want (%q, %q)", gotDir, gotBranch, merged.EffectiveDir(), "feat/shipped")
	}
	sf := readState(tmp)
	if _, ok := sf.Agents["merged-branch"]; ok {
		t.Error("merged survivor should be pruned")
	}
	if _, ok := sf.Agents["open-branch"]; !ok {
		t.Error("unmerged survivor should be retained")
	}
}

func TestResolveAgentTargets(t *testing.T) {
	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			"s1": {
				SessionID:  "s1",
				Target:     "tomoro:3.2",
				TmuxPaneID: "%90",
				Session:    "tomoro",
				Window:     3,
				Pane:       2,
				State:      "running",
			},
			"s2": {
				SessionID:  "s2",
				Target:     "tomoro:2.1",
				TmuxPaneID: "%87",
				Session:    "tomoro",
				Window:     2,
				Pane:       1,
				State:      "input",
			},
			"s3": {
				// No TmuxPaneID — should be left unchanged
				SessionID: "s3",
				Target:    "tomoro:1.0",
				Session:   "tomoro",
				Window:    1,
				Pane:      0,
				State:     "done",
			},
		},
	}

	// Simulate tmux having renumbered windows (window 3 -> 2, window 2 -> 1)
	paneTargets := map[string]domain.PaneTarget{
		"%90": {Session: "tomoro", Window: 2, Pane: 2, Target: "tomoro:2.2"},
		"%87": {Session: "tomoro", Window: 1, Pane: 1, Target: "tomoro:1.1"},
	}

	resolveAgentTargets(&sf, paneTargets)

	// s1 should be updated
	s1 := sf.Agents["s1"]
	if s1.Target != "tomoro:2.2" {
		t.Errorf("s1.Target = %q, want %q", s1.Target, "tomoro:2.2")
	}
	if s1.Window != 2 || s1.Pane != 2 {
		t.Errorf("s1 Window.Pane = %d.%d, want 2.2", s1.Window, s1.Pane)
	}
	if s1.Session != "tomoro" {
		t.Errorf("s1.Session = %q, want %q", s1.Session, "tomoro")
	}

	// s2 should be updated
	s2 := sf.Agents["s2"]
	if s2.Target != "tomoro:1.1" {
		t.Errorf("s2.Target = %q, want %q", s2.Target, "tomoro:1.1")
	}
	if s2.Window != 1 || s2.Pane != 1 {
		t.Errorf("s2 Window.Pane = %d.%d, want 1.1", s2.Window, s2.Pane)
	}

	// s3 should be unchanged (no TmuxPaneID)
	s3 := sf.Agents["s3"]
	if s3.Target != "tomoro:1.0" {
		t.Errorf("s3.Target = %q, want %q (unchanged)", s3.Target, "tomoro:1.0")
	}
	if s3.Window != 1 || s3.Pane != 0 {
		t.Errorf("s3 Window.Pane = %d.%d, want 1.0 (unchanged)", s3.Window, s3.Pane)
	}
}

func TestResolveAgentTargets_DeadPane(t *testing.T) {
	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			"alive": {Target: "tomoro:3.2", TmuxPaneID: "%90", Session: "tomoro", Window: 3, Pane: 2, State: "running"},
			"dead":  {Target: "tomoro:2.1", TmuxPaneID: "%87", Session: "tomoro", Window: 2, Pane: 1, State: "question"},
		},
	}

	// Only %90 is live; %87 is dead (not in the map)
	paneTargets := map[string]domain.PaneTarget{
		"%90": {Session: "tomoro", Window: 2, Pane: 2, Target: "tomoro:2.2"},
	}

	resolveAgentTargets(&sf, paneTargets)

	// alive should be updated
	alive := sf.Agents["alive"]
	if alive.Window != 2 || alive.Pane != 2 {
		t.Errorf("alive Window.Pane = %d.%d, want 2.2", alive.Window, alive.Pane)
	}

	// dead should be unchanged (stale values kept as fallback)
	dead := sf.Agents["dead"]
	if dead.Window != 2 || dead.Pane != 1 {
		t.Errorf("dead Window.Pane = %d.%d, want 2.1 (unchanged)", dead.Window, dead.Pane)
	}
}

func TestResolveAgentTargets_EmptyMap(t *testing.T) {
	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			"s1": {Target: "tomoro:3.2", TmuxPaneID: "%90", Window: 3, Pane: 2, State: "running"},
		},
	}
	// Empty (non-nil) map — all agents should be left unchanged
	resolveAgentTargets(&sf, map[string]domain.PaneTarget{})
	if sf.Agents["s1"].Window != 3 {
		t.Errorf("expected window unchanged with empty map")
	}
}

func TestResolveAgentTargets_NilMap(t *testing.T) {
	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			"s1": {Target: "tomoro:3.2", TmuxPaneID: "%90", Window: 3, Pane: 2, State: "running"},
		},
	}
	// nil paneTargets should not panic, agents left unchanged
	resolveAgentTargets(&sf, nil)
	if sf.Agents["s1"].Window != 3 {
		t.Errorf("expected window unchanged with nil map")
	}
}

// withMockBranchRunner swaps the package-level branchRunner with a mock
// and restores the original on test cleanup.
func withMockBranchRunner(t *testing.T) *mocks.MockBranchRunner {
	t.Helper()
	m := mocks.NewMockBranchRunner(t)
	orig := branchRunner
	branchRunner = m
	t.Cleanup(func() { branchRunner = orig })
	return m
}

// mockGitBranch sets up a mock expectation: when git rev-parse is called
// for the given directory, return the given branch name. If branch is "",
// return an error (simulating a non-git directory).
func mockGitBranch(m *mocks.MockBranchRunner, dir, branch string) {
	if branch == "" {
		m.On("Output", mock.Anything, "git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD").
			Return(nil, fmt.Errorf("not a git repo"))
	} else {
		m.On("Output", mock.Anything, "git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD").
			Return([]byte(branch+"\n"), nil)
	}
}

func TestResolveAgentBranches(t *testing.T) {
	m := withMockBranchRunner(t)
	mockGitBranch(m, "/valid/repo", "feat/mock-branch")
	mockGitBranch(m, "/nonexistent/path", "")

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			// No WorktreeCwd → live read every refresh, even when Branch is
			// pre-populated. Vanilla source-repo agents have no pin.
			"with-cwd": {Cwd: "/valid/repo", Branch: "stale-branch", State: "running"},
			"no-cwd":   {Cwd: "", Branch: "stale", State: "running"},
			"bad-cwd":  {Cwd: "/nonexistent/path", Branch: "stale", State: "running"},
		},
	}

	resolveAgentBranches(&sf, nil, "")

	if sf.Agents["with-cwd"].Branch != "feat/mock-branch" {
		t.Errorf("expected branch feat/mock-branch, got %q", sf.Agents["with-cwd"].Branch)
	}
	if sf.Agents["no-cwd"].Branch != "" {
		t.Errorf("expected branch cleared for no-cwd agent, got %q", sf.Agents["no-cwd"].Branch)
	}
	if sf.Agents["bad-cwd"].Branch != "" {
		t.Errorf("expected branch cleared for bad-cwd agent, got %q", sf.Agents["bad-cwd"].Branch)
	}
}

func TestResolveAgentBranches_PinnedBranchPreserved(t *testing.T) {
	// Pin semantic: when WorktreeCwd is set and Branch is already on file, the
	// stored Branch is authoritative. resolveAgentBranches must NOT run git —
	// the recorded branch reflects what the worktree was created with, and the
	// dashboard should not flap to whatever HEAD a `git checkout` moves to.
	m := withMockBranchRunner(t)
	// Intentionally no mockGitBranch expectations. If git is called, the mock
	// fails the test via AssertExpectations.

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			"pinned":           {WorktreeCwd: "/valid/worktree", Cwd: "/nonexistent/path", Branch: "feat/pinned", State: "running"},
			"pinned-wt-broken": {WorktreeCwd: "/deleted/wt", Cwd: "/valid/repo", Branch: "feat/already-pinned", State: "running"},
		},
	}

	resolveAgentBranches(&sf, nil, "")

	if got := sf.Agents["pinned"].Branch; got != "feat/pinned" {
		t.Errorf("pinned: expected branch preserved as feat/pinned, got %q", got)
	}
	// Even when the worktree dir is gone, the pinned branch is still the right
	// label — the agent's task didn't change.
	if got := sf.Agents["pinned-wt-broken"].Branch; got != "feat/already-pinned" {
		t.Errorf("pinned-wt-broken: expected branch preserved as feat/already-pinned, got %q", got)
	}
	m.AssertExpectations(t)
}

func TestResolveAgentBranches_BackfillFromWorktree_Persists(t *testing.T) {
	// One-shot backfill: agent has WorktreeCwd set but Branch empty (legacy
	// state file written before the pin landed, or an agent stamped by the
	// fast hook that hasn't yet seen `git worktree add -b`). The next refresh
	// performs one live read against WorktreeCwd, stamps the result in
	// memory, AND persists it to the agent's JSON file so subsequent refreshes
	// skip the git call.
	m := withMockBranchRunner(t)
	worktreeDir := "/valid/worktree"
	mockGitBranch(m, worktreeDir, "feat/backfilled")

	stateDir := t.TempDir()
	sessionID := "sess-backfill"
	agent := domain.Agent{
		SessionID:   sessionID,
		WorktreeCwd: worktreeDir,
		Cwd:         "/repo",
		State:       "running",
	}
	seedAgentJSON(t, stateDir, agent)

	sf := domain.StateFile{Agents: map[string]domain.Agent{sessionID: agent}}
	resolveAgentBranches(&sf, nil, stateDir)

	if got := sf.Agents[sessionID].Branch; got != "feat/backfilled" {
		t.Errorf("in-memory Branch = %q, want feat/backfilled", got)
	}

	data, err := os.ReadFile(filepath.Join(stateDir, "agents", sessionID+".json"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if raw["branch"] != "feat/backfilled" {
		t.Errorf("persisted branch = %v, want feat/backfilled", raw["branch"])
	}
}

func TestResolveAgentBranches_BackfillNoStateDir_InMemoryOnly(t *testing.T) {
	// When stateDir is empty (callers without a write path, e.g. unit tests),
	// backfill happens in memory but is not persisted. Acceptable: next
	// refresh just re-reads.
	m := withMockBranchRunner(t)
	mockGitBranch(m, "/valid/worktree", "feat/in-mem")

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			"a": {SessionID: "a", WorktreeCwd: "/valid/worktree", State: "running"},
		},
	}

	resolveAgentBranches(&sf, nil, "")

	if got := sf.Agents["a"].Branch; got != "feat/in-mem" {
		t.Errorf("Branch = %q, want feat/in-mem", got)
	}
}

func TestResolveAgentBranches_NoWorktree_LiveReadEveryRefresh(t *testing.T) {
	// Vanilla source-repo agents (no WorktreeCwd) keep live-read behavior:
	// Branch reflects the source repo's current HEAD on every refresh.
	m := withMockBranchRunner(t)
	mockGitBranch(m, "/valid/repo", "main")

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			"vanilla": {Cwd: "/valid/repo", Branch: "feat/stale", State: "running"},
		},
	}

	resolveAgentBranches(&sf, nil, "")

	// "feat/stale" was wrong (source repo is actually on main) and the live
	// read clobbers it. This is the pre-existing semantic for unpinned agents
	// and is preserved.
	if got := sf.Agents["vanilla"].Branch; got != "main" {
		t.Errorf("vanilla: expected live branch main, got %q", got)
	}
	m.AssertExpectations(t)
}

func TestResolveAgentBranches_BrokenWorktree_EmptyBranch_ClearsBranch(t *testing.T) {
	// WorktreeCwd set but git fails (dir deleted post-merge) AND Branch is
	// empty: there's no pin to preserve and no live answer. Branch ends up "".
	// Crucially, we do NOT fall back to Cwd — that would surface the source
	// repo's branch (almost always "main") and reintroduce the original bug.
	m := withMockBranchRunner(t)
	mockGitBranch(m, "/bad/wt", "")

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			"broken": {WorktreeCwd: "/bad/wt", Cwd: "/valid/repo", State: "running"},
		},
	}

	resolveAgentBranches(&sf, nil, "")

	if got := sf.Agents["broken"].Branch; got != "" {
		t.Errorf("broken: expected empty branch (no fallback to Cwd), got %q", got)
	}
}

func TestResolveAgentBranches_PaneCwdFallback(t *testing.T) {
	m := withMockBranchRunner(t)
	mockGitBranch(m, "/pane/cwd", "feat/pane-branch")
	mockGitBranch(m, "/existing/cwd", "feat/existing-branch")

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			"no-cwd":  {TmuxPaneID: "%10", Branch: "stale", State: "running"},
			"has-cwd": {TmuxPaneID: "%11", Cwd: "/existing/cwd", Branch: "stale", State: "running"},
			"no-pane": {Branch: "stale", State: "running"},
		},
	}

	// Pane cwd is used to backfill an empty Cwd (display only), but never
	// to resolve branch — the agent's project dir is intentionally static.
	paneCwds := map[string]string{
		"%10": "/pane/cwd",
		"%11": "/should/not/be/used",
	}

	resolveAgentBranches(&sf, paneCwds, "")

	if sf.Agents["no-cwd"].Cwd != "/pane/cwd" {
		t.Errorf("no-cwd: expected Cwd backfilled to /pane/cwd, got %q", sf.Agents["no-cwd"].Cwd)
	}
	if sf.Agents["no-cwd"].Branch != "feat/pane-branch" {
		t.Errorf("no-cwd: expected feat/pane-branch, got %q", sf.Agents["no-cwd"].Branch)
	}
	if sf.Agents["has-cwd"].Cwd != "/existing/cwd" {
		t.Errorf("has-cwd: stored Cwd should remain /existing/cwd, got %q", sf.Agents["has-cwd"].Cwd)
	}
	if sf.Agents["has-cwd"].Branch != "feat/existing-branch" {
		t.Errorf("has-cwd: expected branch from stored Cwd (pane cwd must not influence), got %q",
			sf.Agents["has-cwd"].Branch)
	}
	if sf.Agents["no-pane"].Branch != "" {
		t.Errorf("no-pane: expected branch cleared (no resolvable cwd), got %q", sf.Agents["no-pane"].Branch)
	}
}

func TestGitBranch(t *testing.T) {
	m := withMockBranchRunner(t)
	mockGitBranch(m, "/valid/repo", "main")
	mockGitBranch(m, "/nonexistent/path", "")

	branch := gitBranch("/valid/repo")
	if branch != "main" {
		t.Errorf("expected main, got %q", branch)
	}

	if gitBranch("/nonexistent/path") != "" {
		t.Error("expected empty branch for invalid path")
	}
}

func TestFormatDuration(t *testing.T) {
	if FormatDuration("") != "" {
		t.Error("expected empty for empty input")
	}
	if FormatDuration("not a date") != "" {
		t.Error("expected empty for invalid date")
	}
	// Can't easily test specific durations without mocking time,
	// but we can verify it doesn't panic on valid input
	result := FormatDuration("2020-01-01T00:00:00Z")
	if result == "" {
		t.Error("expected non-empty for valid old date")
	}
}

func TestApplyPinnedStates(t *testing.T) {
	tests := []struct {
		name      string
		state     string
		pinned    string
		wantState string
	}{
		{"idle_prompt restored to pr", "idle_prompt", "pr", "pr"},
		{"done restored to pr", "done", "pr", "pr"},
		{"question restored to pr", "question", "pr", "pr"},
		{"done restored to merged", "done", "merged", "merged"},
		{"running passes through pinned pr", "running", "pr", "running"},
		{"permission passes through pinned pr", "permission", "pr", "permission"},
		{"error passes through pinned pr", "error", "pr", "error"},
		{"plan passes through pinned pr", "plan", "pr", "plan"},
		{"no pin leaves state unchanged", "running", "", "running"},
		{"no pin idle_prompt unchanged", "idle_prompt", "", "idle_prompt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sf := domain.StateFile{
				Agents: map[string]domain.Agent{
					"agent": {State: tt.state, PinnedState: tt.pinned},
				},
			}
			ApplyStateArbitration(&sf, "")
			if got := sf.Agents["agent"].State; got != tt.wantState {
				t.Errorf("got %q, want %q", got, tt.wantState)
			}
		})
	}
}

func TestArbitrateState(t *testing.T) {
	tests := []struct {
		name     string
		state    string
		pinned   string
		override string
		want     string
	}{
		// Precedence 1: pinned pr/merged wins over an idle resting state.
		{"pinned pr over idle_prompt", "idle_prompt", "pr", "", "pr"},
		{"pinned merged over done", "done", "merged", "", "merged"},
		{"pinned pr over question", "question", "pr", "", "pr"},
		{"pin beats a pending override", "idle_prompt", "pr", "plan", "pr"},
		// Pin only applies to idle states; active states pass through.
		{"running ignores pin", "running", "pr", "", "running"},
		{"permission ignores pin", "permission", "pr", "", "permission"},
		// Precedence 2: transcript override on an idle candidate.
		{"override promotes idle_prompt to plan", "idle_prompt", "", "plan", "plan"},
		{"override promotes permission to question", "permission", "", "question", "question"},
		// Precedence 3: raw state when nothing else applies.
		{"no pin no override passes through", "idle_prompt", "", "", "idle_prompt"},
		{"running passes through", "running", "", "", "running"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := domain.Agent{State: tt.state, PinnedState: tt.pinned}
			if got := arbitrateState(agent, tt.override); got != tt.want {
				t.Errorf("arbitrateState(%q, pin=%q, override=%q) = %q, want %q",
					tt.state, tt.pinned, tt.override, got, tt.want)
			}
		})
	}
}

func TestPinAgentState(t *testing.T) {
	tmp := t.TempDir()
	writeAgentFile(t, tmp, "sess-a.json", domain.Agent{SessionID: "sess-a", Target: "a:0.1", State: "done", TmuxPaneID: "%1"})

	err := PinAgentState(tmp, "sess-a", "pr")
	if err != nil {
		t.Fatalf("PinAgentState failed: %v", err)
	}

	// Read back and verify
	sf := readState(tmp)
	agent := sf.Agents["sess-a"]
	if agent.PinnedState != "pr" {
		t.Errorf("expected pinned_state=pr, got %q", agent.PinnedState)
	}
	// Original state should be preserved
	if agent.State != "done" {
		t.Errorf("expected state=done preserved, got %q", agent.State)
	}
}

func TestPinAgentState_NonExistent(t *testing.T) {
	tmp := t.TempDir()
	writeAgentFile(t, tmp, "sess-a.json", domain.Agent{SessionID: "sess-a", Target: "a:0.1", State: "running"})

	err := PinAgentState(tmp, "nonexistent", "pr")
	if err == nil {
		t.Error("expected error for nonexistent session_id")
	}
}

// TestStateFile_ConcurrentWrites_PreserveFields exercises the read-modify-write
// race between PinAgentState and stampAgentFields on the same agent file.
// Each goroutine writes a different field; with proper locking + atomic
// rename, every field a writer set must be present in the final state.
//
// Without locking the writers stomp each other: A reads file, B reads file,
// A writes (sets pinned_state), B writes (its stale snapshot lacks pinned_state
// so the merge drops it). The same shape wipes branch and worktree_cwd in
// production when the dashboard's Pin button races a hook subprocess.
func TestStateFile_ConcurrentWrites_PreserveFields(t *testing.T) {
	tmp := t.TempDir()
	writeAgentFile(t, tmp, "sess-a.json", domain.Agent{
		SessionID: "sess-a",
		Target:    "a:0.1",
		State:     "running",
	})

	const iters = 200
	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		for i := 0; i < iters; i++ {
			if err := PinAgentState(tmp, "sess-a", "review"); err != nil {
				t.Errorf("PinAgentState: %v", err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iters; i++ {
			if err := stampAgentFields(tmp, "sess-a", map[string]any{
				"branch": fmt.Sprintf("br-%d", i),
			}); err != nil {
				t.Errorf("stampAgentFields branch: %v", err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iters; i++ {
			if err := stampAgentFields(tmp, "sess-a", map[string]any{
				"worktree_cwd": fmt.Sprintf("/wt/%d", i),
			}); err != nil {
				t.Errorf("stampAgentFields worktree_cwd: %v", err)
				return
			}
		}
	}()
	wg.Wait()

	sf := readState(tmp)
	agent, ok := sf.Agents["sess-a"]
	if !ok {
		t.Fatalf("agent missing after concurrent writes")
	}
	if agent.PinnedState != "review" {
		t.Errorf("pinned_state wiped: got %q want %q", agent.PinnedState, "review")
	}
	if agent.Branch == "" {
		t.Errorf("branch wiped: got empty")
	}
	if agent.WorktreeCwd == "" {
		t.Errorf("worktree_cwd wiped: got empty")
	}
	// Baseline fields must also survive.
	if agent.SessionID != "sess-a" {
		t.Errorf("session_id corrupted: got %q", agent.SessionID)
	}
	if agent.Target != "a:0.1" {
		t.Errorf("target corrupted: got %q", agent.Target)
	}
}

func TestSortedAgents_PRAndMergedGroups(t *testing.T) {
	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			"s0": {Target: "a:0.0", State: "permission", Window: 0, Pane: 0, TmuxPaneID: "%0"},
			"s1": {Target: "a:1.0", State: "running", Window: 1, Pane: 0, TmuxPaneID: "%1"},
			"s2": {Target: "a:2.0", State: "done", Window: 2, Pane: 0, TmuxPaneID: "%2"},
			"s3": {Target: "a:3.0", State: "pr", Window: 3, Pane: 0, TmuxPaneID: "%3"},
			"s4": {Target: "a:4.0", State: "merged", Window: 4, Pane: 0, TmuxPaneID: "%4"},
		},
	}

	sorted := SortedAgents(sf, "")
	expected := []string{"permission", "running", "done", "pr", "merged"}

	if len(sorted) != 5 {
		t.Fatalf("expected 5 agents, got %d", len(sorted))
	}
	for i, want := range expected {
		if sorted[i].State != want {
			t.Errorf("position %d: expected %s, got %s", i, want, sorted[i].State)
		}
	}
}

// planTestSetup creates a temp directory structure that ApplyIdleOverrides can
// resolve. Writes the JSONL at projectsDir/<slug-of-cwd>/<sessionID>.jsonl and
// returns the resolved projDir (what resolveAgentProjDir would stamp) along
// with the agent's cwd. Tests should set Agent.ProjDir = projDir on the
// fixture; ApplyIdleOverrides reads ProjDir directly.
func planTestSetup(t *testing.T, sessionID, jsonl string) (projDir, cwd string) {
	t.Helper()
	dir := t.TempDir()
	cwd = "/test/myproject"
	slug := conversation.ProjectSlug(cwd)
	projDir = filepath.Join(dir, slug)
	os.MkdirAll(projDir, 0755)
	os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(jsonl), 0644)
	return projDir, cwd
}

func TestApplyIdleOverrides_OverridesIdlePrompt(t *testing.T) {
	sessionID := "sess-plan"
	// Realistic JSONL: ExitPlanMode is always followed by a tool_result user entry
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"ExitPlanMode","input":{}}]},"timestamp":"2026-03-28T10:00:00Z"}
{"type":"user","message":{"role":"user","content":[{"tool_use_id":"t1","type":"tool_result","content":"Plan submitted for review"}]},"timestamp":"2026-03-28T10:00:01Z"}
`
	projDir, cwd := planTestSetup(t, sessionID, jsonl)

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			sessionID: {SessionID: sessionID, State: "idle_prompt", Cwd: cwd, ProjDir: projDir},
		},
	}

	ApplyStateArbitration(&sf, "")

	if sf.Agents[sessionID].State != "plan" {
		t.Errorf("expected state 'plan', got %q", sf.Agents[sessionID].State)
	}
}

func TestApplyIdleOverrides_SkipsRunningAgents(t *testing.T) {
	sessionID := "sess-running"
	// Running agents are genuinely running — the hook-side stop-state guard
	// prevents the PostToolUse race. ApplyIdleOverrides skips them entirely
	// (the state check at line 236 is the only gate; JSONL content is irrelevant).
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}]},"timestamp":"2026-03-28T10:00:00Z"}
{"type":"user","message":{"role":"user","content":[{"tool_use_id":"t1","type":"tool_result","content":"file.txt"}]},"timestamp":"2026-03-28T10:00:01Z"}
`
	projDir, cwd := planTestSetup(t, sessionID, jsonl)

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			sessionID: {SessionID: sessionID, State: "running", Cwd: cwd, ProjDir: projDir},
		},
	}

	ApplyStateArbitration(&sf, "")

	if sf.Agents[sessionID].State != "running" {
		t.Errorf("expected state 'running' unchanged (running agents should be skipped), got %q", sf.Agents[sessionID].State)
	}
}

func TestApplyIdleOverrides_NoOverrideWithoutPlan(t *testing.T) {
	sessionID := "sess-idle"
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"done"}]},"timestamp":"2026-03-28T10:00:00Z"}
`
	projDir, cwd := planTestSetup(t, sessionID, jsonl)

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			sessionID: {SessionID: sessionID, State: "idle_prompt", Cwd: cwd, ProjDir: projDir},
		},
	}

	ApplyStateArbitration(&sf, "")

	if sf.Agents[sessionID].State != "idle_prompt" {
		t.Errorf("expected state 'idle_prompt' unchanged, got %q", sf.Agents[sessionID].State)
	}
}

func TestApplyIdleOverrides_QuestionOverride(t *testing.T) {
	sessionID := "sess-question"
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"AskUserQuestion","input":{"question":"Which approach?"}}]},"timestamp":"2026-03-28T10:00:00Z"}
{"type":"user","message":{"role":"user","content":[{"tool_use_id":"t1","type":"tool_result","content":"Option A"}]},"timestamp":"2026-03-28T10:00:01Z"}
`
	projDir, cwd := planTestSetup(t, sessionID, jsonl)

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			sessionID: {SessionID: sessionID, State: "idle_prompt", Cwd: cwd, ProjDir: projDir},
		},
	}

	ApplyStateArbitration(&sf, "")

	if sf.Agents[sessionID].State != "question" {
		t.Errorf("expected state 'question', got %q", sf.Agents[sessionID].State)
	}
}

func TestApplyIdleOverrides_PlanTakesPriorityOverQuestion(t *testing.T) {
	sessionID := "sess-both"
	// Both ExitPlanMode and AskUserQuestion present, no human response -> plan wins
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"AskUserQuestion","input":{"question":"Which?"}}]},"timestamp":"2026-03-28T10:00:00Z"}
{"type":"user","message":{"role":"user","content":[{"tool_use_id":"t1","type":"tool_result","content":"A"}]},"timestamp":"2026-03-28T10:00:01Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t2","name":"ExitPlanMode","input":{}}]},"timestamp":"2026-03-28T10:00:02Z"}
{"type":"user","message":{"role":"user","content":[{"tool_use_id":"t2","type":"tool_result","content":"Plan submitted"}]},"timestamp":"2026-03-28T10:00:03Z"}
`
	projDir, cwd := planTestSetup(t, sessionID, jsonl)

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			sessionID: {SessionID: sessionID, State: "idle_prompt", Cwd: cwd, ProjDir: projDir},
		},
	}

	ApplyStateArbitration(&sf, "")

	if sf.Agents[sessionID].State != "plan" {
		t.Errorf("expected 'plan' to take priority over 'question', got %q", sf.Agents[sessionID].State)
	}
}

func TestApplyIdleOverrides_OverridesPermissionWithPendingPlan(t *testing.T) {
	sessionID := "sess-perm-plan"
	// domain.Agent called ExitPlanMode, hook reported "permission" because the plan
	// selection menu looks like a permission prompt.
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"ExitPlanMode","input":{}}]},"timestamp":"2026-03-28T10:00:00Z"}
{"type":"user","message":{"role":"user","content":[{"tool_use_id":"t1","type":"tool_result","content":"Plan submitted for review"}]},"timestamp":"2026-03-28T10:00:01Z"}
`
	projDir, cwd := planTestSetup(t, sessionID, jsonl)

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			sessionID: {SessionID: sessionID, State: "permission", Cwd: cwd, ProjDir: projDir},
		},
	}

	ApplyStateArbitration(&sf, "")

	if sf.Agents[sessionID].State != "plan" {
		t.Errorf("expected state 'plan' (permission agent with pending ExitPlanMode), got %q", sf.Agents[sessionID].State)
	}
}

func TestApplyIdleOverrides_LeavesPermissionAloneWhenNoPlan(t *testing.T) {
	sessionID := "sess-perm-real"
	// Real permission prompt — no ExitPlanMode in JSONL.
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Write","input":{"path":"foo.txt"}}]},"timestamp":"2026-03-28T10:00:00Z"}
`
	projDir, cwd := planTestSetup(t, sessionID, jsonl)

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			sessionID: {SessionID: sessionID, State: "permission", Cwd: cwd, ProjDir: projDir},
		},
	}

	ApplyStateArbitration(&sf, "")

	if sf.Agents[sessionID].State != "permission" {
		t.Errorf("expected state 'permission' unchanged (real permission, no plan), got %q", sf.Agents[sessionID].State)
	}
}

func TestApplyIdleOverrides_QuestionAfterPlanWins(t *testing.T) {
	sessionID := "sess-interview"
	// ExitPlanMode first, then AskUserQuestion — the most recent blocking tool
	// should win. Only tool_results between them (no human text), so both are
	// technically "pending", but AskUserQuestion is the later action.
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"ExitPlanMode","input":{}}]},"timestamp":"2026-03-28T10:00:00Z"}
{"type":"user","message":{"role":"user","content":[{"tool_use_id":"t1","type":"tool_result","content":"Plan submitted"}]},"timestamp":"2026-03-28T10:00:01Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t2","name":"AskUserQuestion","input":{"question":"Before I update the plan, what framework do you prefer?"}}]},"timestamp":"2026-03-28T10:00:02Z"}
{"type":"user","message":{"role":"user","content":[{"tool_use_id":"t2","type":"tool_result","content":"Waiting for user response"}]},"timestamp":"2026-03-28T10:00:03Z"}
`
	projDir, cwd := planTestSetup(t, sessionID, jsonl)

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			sessionID: {SessionID: sessionID, State: "idle_prompt", Cwd: cwd, ProjDir: projDir},
		},
	}

	ApplyStateArbitration(&sf, "")

	if sf.Agents[sessionID].State != "question" {
		t.Errorf("expected 'question' (AskUserQuestion after ExitPlanMode), got %q", sf.Agents[sessionID].State)
	}
}

func TestApplyIdleOverrides_PlanAfterUnansweredQuestionWins(t *testing.T) {
	sessionID := "sess-plan-after-q"
	// AskUserQuestion first (only tool_result after), then ExitPlanMode —
	// the most recent blocking tool wins, so plan takes precedence.
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"AskUserQuestion","input":{"question":"Which?"}}]},"timestamp":"2026-03-28T10:00:00Z"}
{"type":"user","message":{"role":"user","content":[{"tool_use_id":"t1","type":"tool_result","content":"A"}]},"timestamp":"2026-03-28T10:00:01Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t2","name":"ExitPlanMode","input":{}}]},"timestamp":"2026-03-28T10:00:02Z"}
{"type":"user","message":{"role":"user","content":[{"tool_use_id":"t2","type":"tool_result","content":"Plan submitted"}]},"timestamp":"2026-03-28T10:00:03Z"}
`
	projDir, cwd := planTestSetup(t, sessionID, jsonl)

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			sessionID: {SessionID: sessionID, State: "idle_prompt", Cwd: cwd, ProjDir: projDir},
		},
	}

	ApplyStateArbitration(&sf, "")

	if sf.Agents[sessionID].State != "plan" {
		t.Errorf("expected 'plan' (ExitPlanMode after AskUserQuestion), got %q", sf.Agents[sessionID].State)
	}
}

// codexRolloutSetup writes a codex rollout under codexSessionsRoot's
// per-day directory tree (matching LocateRollout's expected layout) so
// the codex branch of ApplyIdleOverrides can resolve it. Returns the
// codexSessionsRoot path to pass through.
func codexRolloutSetup(t *testing.T, sessionID, jsonl string) string {
	t.Helper()
	root := t.TempDir()
	dayDir := filepath.Join(root, "2026", "06", "06")
	if err := os.MkdirAll(dayDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dayDir, "rollout-2026-06-06T11-00-00-"+sessionID+".jsonl")
	if err := os.WriteFile(path, []byte(jsonl), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// Codex's Stop hook fires when the model emits a request_user_input
// function_call, leaving the agent file with state="done" while the
// codex CLI is genuinely blocked at the inline picker. ApplyIdleOverrides
// recovers by scanning the rollout and promoting state to "question".
func TestApplyIdleOverrides_CodexPromotesDoneToQuestion(t *testing.T) {
	sessionID := "019e9c6f-58cf-7fc1-8440-28ff45163db3"
	jsonl := `{"timestamp":"2026-06-06T11:00:00Z","type":"response_item","payload":{"type":"function_call","name":"request_user_input","arguments":"{\"questions\":[{\"id\":\"q1\",\"header\":\"Scope\",\"question\":\"How broad?\",\"options\":[{\"label\":\"A\"},{\"label\":\"B\"}]}]}","call_id":"call_codex_done"}}
`
	root := codexRolloutSetup(t, sessionID, jsonl)

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			sessionID: {SessionID: sessionID, State: "done", Harness: "codex"},
		},
	}

	ApplyStateArbitration(&sf, root)

	if got := sf.Agents[sessionID].State; got != "question" {
		t.Errorf("expected 'question' (codex Stop race recovered), got %q", got)
	}
}

// Codex agents with an answered request_user_input stay in their original
// idle state — no false promotion.
func TestApplyIdleOverrides_CodexLeavesAnsweredAlone(t *testing.T) {
	sessionID := "019e9c6f-aaaa-1111-2222-333344445555"
	jsonl := `{"timestamp":"2026-06-06T11:00:00Z","type":"response_item","payload":{"type":"function_call","name":"request_user_input","arguments":"{\"questions\":[{\"id\":\"q1\",\"question\":\"x\",\"options\":[{\"label\":\"a\"}]}]}","call_id":"call_ans"}}
{"timestamp":"2026-06-06T11:00:01Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call_ans","output":"{\"answers\":{\"q1\":{\"answers\":[\"a\"]}}}"}}
`
	root := codexRolloutSetup(t, sessionID, jsonl)

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			sessionID: {SessionID: sessionID, State: "done", Harness: "codex"},
		},
	}

	ApplyStateArbitration(&sf, root)

	if got := sf.Agents[sessionID].State; got != "done" {
		t.Errorf("expected 'done' (answered question is not pending), got %q", got)
	}
}

// Claude agents in state="done" are not idle candidates — the
// PostToolUse stop-state guard already prevented the race for claude.
// Verify the new "done" branch is codex-only.
func TestApplyIdleOverrides_ClaudeDoneIsNotCandidate(t *testing.T) {
	sessionID := "sess-claude-done"
	// Same JSONL as the codex promotion test but framed as a claude
	// AskUserQuestion that has NOT been answered (no user message after).
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"AskUserQuestion","input":{"questions":[{"question":"Which?"}]}}]},"timestamp":"2026-03-28T10:00:00Z"}
`
	projDir, cwd := planTestSetup(t, sessionID, jsonl)

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			sessionID: {SessionID: sessionID, State: "done", Harness: "claude", Cwd: cwd, ProjDir: projDir},
		},
	}

	ApplyStateArbitration(&sf, "")

	if got := sf.Agents[sessionID].State; got != "done" {
		t.Errorf("expected 'done' (claude in done is not an idle candidate), got %q", got)
	}
}

// projDirTestSetup creates projectsDir/<slug>/<sid>.jsonl for resolveAgentProjDir
// tests. Returns the projectsDir.
func projDirTestSetup(t *testing.T, slugCwd, sessionID string) string {
	t.Helper()
	dir := t.TempDir()
	slug := conversation.ProjectSlug(slugCwd)
	if err := os.MkdirAll(filepath.Join(dir, slug), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, slug, sessionID+".jsonl"), []byte(""), 0o644); err != nil {
		t.Fatalf("write jsonl: %v", err)
	}
	return dir
}

// mockTopologySource sets up a runner expectation: rev-parse calls for
// `seed` resolve to the given source/worktree pair.
func mockTopologySource(m *mocks.MockBranchRunner, seed, worktree, source string) {
	m.On("Output", mock.Anything, "git", "-C", seed, "rev-parse", "--show-toplevel").
		Return([]byte(worktree+"\n"), nil).Maybe()
	m.On("Output", mock.Anything, "git", "-C", seed, "rev-parse", "--path-format=absolute", "--git-common-dir").
		Return([]byte(filepath.Join(source, ".git")+"\n"), nil).Maybe()
	m.On("Output", mock.Anything, "git", "-C", seed, "rev-parse", "--show-superproject-working-tree").
		Return([]byte("\n"), nil).Maybe()
}

func TestResolveAgentProjDir_AgentCwdSlugMatches(t *testing.T) {
	// Pane 4.2 shape: agent.Cwd matches the JSONL location's slug.
	sessionID := "sess-aligned"
	projectsDir := projDirTestSetup(t, "/repo", sessionID)
	wantProjDir := filepath.Join(projectsDir, conversation.ProjectSlug("/repo"))

	m := withMockBranchRunner(t)
	mockTopologySource(m, "/repo", "/repo", "/repo")

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			sessionID: {SessionID: sessionID, Cwd: "/repo"},
		},
	}

	resolveAgentProjDir(&sf, projectsDir, t.TempDir())

	if got := sf.Agents[sessionID].ProjDir; got != wantProjDir {
		t.Errorf("ProjDir = %q, want %q", got, wantProjDir)
	}
}

func TestResolveAgentProjDir_FallsThroughToTopologySource(t *testing.T) {
	// Pane 4.1 shape: agent.Cwd is the worktree path; JSONL is at the
	// source-repo slug. Only top.Source produces a hit.
	sessionID := "sess-mismatch"
	projectsDir := projDirTestSetup(t, "/repo", sessionID)
	wantProjDir := filepath.Join(projectsDir, conversation.ProjectSlug("/repo"))

	m := withMockBranchRunner(t)
	mockTopologySource(m, "/wt/feat", "/wt/feat", "/repo")

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			sessionID: {SessionID: sessionID, Cwd: "/wt/feat", WorktreeCwd: "/wt/feat"},
		},
	}

	resolveAgentProjDir(&sf, projectsDir, t.TempDir())

	if got := sf.Agents[sessionID].ProjDir; got != wantProjDir {
		t.Errorf("ProjDir = %q, want %q (should fall through to top.Source)", got, wantProjDir)
	}
}

func TestResolveAgentProjDir_NoJSONL_LeavesProjDirEmpty(t *testing.T) {
	sessionID := "sess-no-jsonl"
	projectsDir := t.TempDir() // no JSONL written

	m := withMockBranchRunner(t)
	mockTopologySource(m, "/repo", "/repo", "/repo")

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			sessionID: {SessionID: sessionID, Cwd: "/repo"},
		},
	}

	resolveAgentProjDir(&sf, projectsDir, t.TempDir())

	if got := sf.Agents[sessionID].ProjDir; got != "" {
		t.Errorf("ProjDir = %q, want empty", got)
	}
}

func TestResolveAgentProjDir_EmptySessionID_BackfilledByLocate(t *testing.T) {
	// Empty SessionID + Cwd whose session metadata exists — Locate finds
	// the SessionID, and resolveAgentProjDir then resolves ProjDir.
	sessionID := "sess-discovered"
	projectsDir := projDirTestSetup(t, "/repo", sessionID)
	wantProjDir := filepath.Join(projectsDir, conversation.ProjectSlug("/repo"))

	sessionsDir := t.TempDir()
	sessFile := map[string]any{
		"sessionId": sessionID,
		"cwd":       "/repo",
		"startedAt": int64(1000),
	}
	data, _ := json.Marshal(sessFile)
	os.WriteFile(filepath.Join(sessionsDir, "1.json"), data, 0o644)

	m := withMockBranchRunner(t)
	mockTopologySource(m, "/repo", "/repo", "/repo")

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			"agent-1": {Cwd: "/repo"}, // no SessionID
		},
	}

	resolveAgentProjDir(&sf, projectsDir, sessionsDir)

	got := sf.Agents["agent-1"]
	if got.SessionID != sessionID {
		t.Errorf("SessionID = %q, want %q (Locate should backfill)", got.SessionID, sessionID)
	}
	if got.ProjDir != wantProjDir {
		t.Errorf("ProjDir = %q, want %q", got.ProjDir, wantProjDir)
	}
}

func TestResolveAgentProjDir_NoSessionIDAndLocateFails_LeavesEmpty(t *testing.T) {
	projectsDir := projDirTestSetup(t, "/repo", "sess-1")

	m := withMockBranchRunner(t)
	mockTopologySource(m, "/repo", "/repo", "/repo")

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			"agent-1": {Cwd: "/repo"}, // no SessionID, no sessions metadata either
		},
	}

	resolveAgentProjDir(&sf, projectsDir, t.TempDir())

	if got := sf.Agents["agent-1"]; got.SessionID != "" || got.ProjDir != "" {
		t.Errorf("expected empty SessionID + ProjDir, got %+v", got)
	}
}

func TestResolveAgentProjDir_GitResolveFails_StillTriesAgentCwd(t *testing.T) {
	// repo.Resolve fails (path not in any git repo), but the agent's stamped
	// Cwd still matches the JSONL slug, so PickProjDir succeeds without
	// topology candidates.
	sessionID := "sess-no-git"
	projectsDir := projDirTestSetup(t, "/notagit", sessionID)
	wantProjDir := filepath.Join(projectsDir, conversation.ProjectSlug("/notagit"))

	m := withMockBranchRunner(t)
	m.On("Output", mock.Anything, "git", "-C", "/notagit", "rev-parse", "--show-toplevel").
		Return(nil, fmt.Errorf("not a git repo")).Maybe()

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			sessionID: {SessionID: sessionID, Cwd: "/notagit"},
		},
	}

	resolveAgentProjDir(&sf, projectsDir, t.TempDir())

	if got := sf.Agents[sessionID].ProjDir; got != wantProjDir {
		t.Errorf("ProjDir = %q, want %q (Cwd candidate should match even if topology fails)", got, wantProjDir)
	}
}

func TestResolveAgentProjDir_DeletedWorktree_FallsBackToScan(t *testing.T) {
	// Reproducer from pane 4.1 (post-merge):
	//   - agent.Cwd = /wt/feat (worktree path) — directory has been deleted
	//   - JSONL still on disk at the source-repo slug
	//   - repo.Resolve fails because /wt/feat doesn't exist
	//   - Topology candidates are empty; agent.Cwd's slug doesn't match
	//
	// Without the FindProjDirByScan fallback, ProjDir would resolve to "".
	// The JSONL is reachable; we should find it.
	sessionID := "sess-deleted-wt"
	projectsDir := projDirTestSetup(t, "/repo", sessionID) // JSONL at source slug
	wantProjDir := filepath.Join(projectsDir, conversation.ProjectSlug("/repo"))

	m := withMockBranchRunner(t)
	m.On("Output", mock.Anything, "git", "-C", "/wt/feat", "rev-parse", "--show-toplevel").
		Return(nil, fmt.Errorf("not a git repo")).Maybe()

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			sessionID: {SessionID: sessionID, Cwd: "/wt/feat", WorktreeCwd: "/wt/feat"},
		},
	}

	resolveAgentProjDir(&sf, projectsDir, t.TempDir())

	if got := sf.Agents[sessionID].ProjDir; got != wantProjDir {
		t.Errorf("ProjDir = %q, want %q (scan fallback should locate the JSONL)", got, wantProjDir)
	}
}

// seedAgentJSON writes a minimal agent state file to stateDir/agents/<id>.json
// containing all required fields for round-trip read/write.
func seedAgentJSON(t *testing.T, stateDir string, agent domain.Agent) {
	t.Helper()
	agentsDir := filepath.Join(stateDir, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir agents: %v", err)
	}
	data, err := json.Marshal(agent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentsDir, agent.SessionID+".json"), data, 0o600); err != nil {
		t.Fatalf("write agent file: %v", err)
	}
}

// writeMarker writes the agent-dashboard-session marker file at gitDir/agent-dashboard-session.
func writeMarker(t *testing.T, gitDir, sessionID string) {
	t.Helper()
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir git dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "agent-dashboard-session"), []byte(sessionID), 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}
}

// mockWorktreeList sets up the git.ListWorktrees expectations: porcelain
// output + one rev-parse --absolute-git-dir per worktree.
func mockWorktreeList(m *mocks.MockBranchRunner, cwd string, wts []struct{ Path, Branch, GitDir string }) {
	var sb strings.Builder
	for i, wt := range wts {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("worktree " + wt.Path + "\n")
		sb.WriteString("HEAD abc123\n")
		if wt.Branch != "" {
			sb.WriteString("branch refs/heads/" + wt.Branch + "\n")
		} else {
			sb.WriteString("detached\n")
		}
	}
	m.On("Output", mock.Anything, "git", "-C", cwd, "worktree", "list", "--porcelain").
		Return([]byte(sb.String()), nil)
	for _, wt := range wts {
		m.On("Output", mock.Anything, "git", "-C", wt.Path, "rev-parse", "--absolute-git-dir").
			Return([]byte(wt.GitDir+"\n"), nil)
	}
}

func TestResolveAgentWorktree_MarkerMatches_PinsFromPorcelain(t *testing.T) {
	m := withMockBranchRunner(t)
	sessionID := "sess-marker-match"
	cwd := "/repo/src"
	stateDir := t.TempDir()

	// Two worktrees in the porcelain output. The linked one has our marker.
	mainGit := filepath.Join(t.TempDir(), "main.git")
	linkedGit := filepath.Join(t.TempDir(), "linked.git")
	writeMarker(t, linkedGit, sessionID)
	mockWorktreeList(m, cwd, []struct{ Path, Branch, GitDir string }{
		{Path: cwd, Branch: "main", GitDir: mainGit},
		{Path: "/repo/wt/feat", Branch: "feat/x", GitDir: linkedGit},
	})

	agent := domain.Agent{SessionID: sessionID, Cwd: cwd, State: "running"}
	seedAgentJSON(t, stateDir, agent)
	sf := domain.StateFile{Agents: map[string]domain.Agent{sessionID: agent}}

	resolveAgentWorktree(&sf, stateDir)

	if got := sf.Agents[sessionID].WorktreeCwd; got != "/repo/wt/feat" {
		t.Errorf("WorktreeCwd = %q, want /repo/wt/feat", got)
	}
	if got := sf.Agents[sessionID].Branch; got != "feat/x" {
		t.Errorf("Branch = %q, want feat/x", got)
	}

	// Persisted to disk.
	data, _ := os.ReadFile(filepath.Join(stateDir, "agents", sessionID+".json"))
	var raw map[string]any
	_ = json.Unmarshal(data, &raw)
	if raw["worktree_cwd"] != "/repo/wt/feat" {
		t.Errorf("persisted worktree_cwd = %v", raw["worktree_cwd"])
	}
	if raw["branch"] != "feat/x" {
		t.Errorf("persisted branch = %v", raw["branch"])
	}
}

func TestResolveAgentWorktree_MarkerMismatches_NoPin(t *testing.T) {
	m := withMockBranchRunner(t)
	sessionID := "sess-mine"
	cwd := "/repo/src"
	stateDir := t.TempDir()

	linkedGit := filepath.Join(t.TempDir(), "linked.git")
	writeMarker(t, linkedGit, "sess-someone-else")
	mockWorktreeList(m, cwd, []struct{ Path, Branch, GitDir string }{
		{Path: "/repo/wt/feat", Branch: "feat/x", GitDir: linkedGit},
	})

	agent := domain.Agent{SessionID: sessionID, Cwd: cwd, State: "running"}
	seedAgentJSON(t, stateDir, agent)
	sf := domain.StateFile{Agents: map[string]domain.Agent{sessionID: agent}}

	resolveAgentWorktree(&sf, stateDir)

	if got := sf.Agents[sessionID].WorktreeCwd; got != "" {
		t.Errorf("WorktreeCwd = %q, want empty (marker belongs to other session)", got)
	}
}

func TestResolveAgentWorktree_NoMarker_DoesNotClaim(t *testing.T) {
	// Go reconciler is read-only: never drops a marker. Claiming is the JS
	// hook's job at PostToolUse, where the agent is actively running.
	m := withMockBranchRunner(t)
	sessionID := "sess-no-marker"
	cwd := "/repo/src"
	stateDir := t.TempDir()

	linkedGit := filepath.Join(t.TempDir(), "linked.git")
	if err := os.MkdirAll(linkedGit, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mockWorktreeList(m, cwd, []struct{ Path, Branch, GitDir string }{
		{Path: "/repo/wt/feat", Branch: "feat/x", GitDir: linkedGit},
	})

	agent := domain.Agent{SessionID: sessionID, Cwd: cwd, State: "running"}
	seedAgentJSON(t, stateDir, agent)
	sf := domain.StateFile{Agents: map[string]domain.Agent{sessionID: agent}}

	resolveAgentWorktree(&sf, stateDir)

	if got := sf.Agents[sessionID].WorktreeCwd; got != "" {
		t.Errorf("WorktreeCwd = %q, want empty (no marker, Go must not claim)", got)
	}
	// Marker must NOT be created by the Go reconciler.
	if _, err := os.Stat(filepath.Join(linkedGit, "agent-dashboard-session")); !os.IsNotExist(err) {
		t.Errorf("Go reconciler should not have written a marker file (err=%v)", err)
	}
}

func TestResolveAgentWorktree_MultipleWorktrees_OnlyMatchingOnePinned(t *testing.T) {
	m := withMockBranchRunner(t)
	sessionID := "sess-multi"
	cwd := "/repo/src"
	stateDir := t.TempDir()

	mineGit := filepath.Join(t.TempDir(), "mine.git")
	otherGit := filepath.Join(t.TempDir(), "other.git")
	writeMarker(t, mineGit, sessionID)
	writeMarker(t, otherGit, "different-sess")
	mockWorktreeList(m, cwd, []struct{ Path, Branch, GitDir string }{
		{Path: "/repo/wt/other", Branch: "feat/other", GitDir: otherGit},
		{Path: "/repo/wt/mine", Branch: "feat/mine", GitDir: mineGit},
	})

	agent := domain.Agent{SessionID: sessionID, Cwd: cwd, State: "running"}
	seedAgentJSON(t, stateDir, agent)
	sf := domain.StateFile{Agents: map[string]domain.Agent{sessionID: agent}}

	resolveAgentWorktree(&sf, stateDir)

	if got := sf.Agents[sessionID].WorktreeCwd; got != "/repo/wt/mine" {
		t.Errorf("WorktreeCwd = %q, want /repo/wt/mine", got)
	}
	if got := sf.Agents[sessionID].Branch; got != "feat/mine" {
		t.Errorf("Branch = %q, want feat/mine", got)
	}
}

func TestResolveAgentWorktree_PreStamped_FastPath(t *testing.T) {
	m := withMockBranchRunner(t)
	sessionID := "sess-pre"

	stateDir := t.TempDir()
	agent := domain.Agent{
		SessionID:   sessionID,
		Cwd:         "/repo",
		WorktreeCwd: "/already/stamped",
		State:       "running",
	}
	seedAgentJSON(t, stateDir, agent)

	sf := domain.StateFile{Agents: map[string]domain.Agent{sessionID: agent}}
	resolveAgentWorktree(&sf, stateDir)

	if got := sf.Agents[sessionID].WorktreeCwd; got != "/already/stamped" {
		t.Errorf("WorktreeCwd should be unchanged, got %q", got)
	}
	m.AssertExpectations(t) // no git calls expected
}

func TestResolveAgentWorktree_EmptySessionID_Skip(t *testing.T) {
	m := withMockBranchRunner(t)

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			"no-sess": {Cwd: "/repo", State: "running"},
		},
	}
	resolveAgentWorktree(&sf, t.TempDir())

	if got := sf.Agents["no-sess"].WorktreeCwd; got != "" {
		t.Errorf("WorktreeCwd = %q, want empty (no session id)", got)
	}
	m.AssertExpectations(t)
}

func TestResolveAgentWorktree_EmptyCwd_Skip(t *testing.T) {
	m := withMockBranchRunner(t)

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			"sess-1": {SessionID: "sess-1", State: "running"},
		},
	}
	resolveAgentWorktree(&sf, t.TempDir())

	if got := sf.Agents["sess-1"].WorktreeCwd; got != "" {
		t.Errorf("WorktreeCwd = %q, want empty (no cwd)", got)
	}
	m.AssertExpectations(t)
}

func TestResolveAgentWorktree_PorcelainError_Skip(t *testing.T) {
	m := withMockBranchRunner(t)
	sessionID := "sess-porcelain-err"
	cwd := "/repo/src"
	stateDir := t.TempDir()

	m.On("Output", mock.Anything, "git", "-C", cwd, "worktree", "list", "--porcelain").
		Return(nil, fmt.Errorf("not a git repo"))

	agent := domain.Agent{SessionID: sessionID, Cwd: cwd, State: "running"}
	seedAgentJSON(t, stateDir, agent)
	sf := domain.StateFile{Agents: map[string]domain.Agent{sessionID: agent}}

	resolveAgentWorktree(&sf, stateDir)

	if got := sf.Agents[sessionID].WorktreeCwd; got != "" {
		t.Errorf("WorktreeCwd = %q, want empty (porcelain failed)", got)
	}
}

// TestResolveAgentWorktree_NoMarker_ClaimsWhenCwdIsLinkedWorktree covers the
// scan-on-init fallback: an unpinned agent whose Cwd IS a linked worktree
// should get its marker atomically written by the Go reconciler so the pin
// recovers without waiting for the next hook event. Models the upgrade-past-
// #330 scenario where legacy state files exist but no marker was ever stamped.
func TestResolveAgentWorktree_NoMarker_ClaimsWhenCwdIsLinkedWorktree(t *testing.T) {
	m := withMockBranchRunner(t)
	sessionID := "sess-claim"
	stateDir := t.TempDir()

	wtRoot := t.TempDir()
	gitDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(wtRoot, ".git"), []byte("gitdir: "+gitDir+"\n"), 0o600); err != nil {
		t.Fatalf("write .git pointer: %v", err)
	}
	wtPath, err := filepath.EvalSymlinks(wtRoot)
	if err != nil {
		t.Fatalf("evalsymlinks wt: %v", err)
	}
	cwd := wtPath

	mockWorktreeList(m, cwd, []struct{ Path, Branch, GitDir string }{
		{Path: wtPath, Branch: "feat/x", GitDir: gitDir},
	})

	agent := domain.Agent{SessionID: sessionID, Cwd: cwd, State: "running"}
	seedAgentJSON(t, stateDir, agent)
	sf := domain.StateFile{Agents: map[string]domain.Agent{sessionID: agent}}

	resolveAgentWorktree(&sf, stateDir)

	if got := sf.Agents[sessionID].WorktreeCwd; got != wtPath {
		t.Errorf("WorktreeCwd = %q, want %q", got, wtPath)
	}
	if got := sf.Agents[sessionID].Branch; got != "feat/x" {
		t.Errorf("Branch = %q, want feat/x", got)
	}
	data, err := os.ReadFile(filepath.Join(gitDir, "agent-dashboard-session"))
	if err != nil {
		t.Fatalf("marker should have been claimed: %v", err)
	}
	if strings.TrimSpace(string(data)) != sessionID {
		t.Errorf("marker = %q, want %q", string(data), sessionID)
	}
	// Persisted to state file.
	raw, _ := os.ReadFile(filepath.Join(stateDir, "agents", sessionID+".json"))
	var got map[string]any
	_ = json.Unmarshal(raw, &got)
	if got["worktree_cwd"] != wtPath {
		t.Errorf("persisted worktree_cwd = %v, want %q", got["worktree_cwd"], wtPath)
	}
	if got["branch"] != "feat/x" {
		t.Errorf("persisted branch = %v, want feat/x", got["branch"])
	}
}

// TestResolveAgentWorktree_NoMarker_NoClaimWhenCwdIsMainWorktree asserts the
// Go reconciler still refuses to claim main worktrees. Attribution is too
// ambiguous there — any agent could match the same source repo Cwd.
func TestResolveAgentWorktree_NoMarker_NoClaimWhenCwdIsMainWorktree(t *testing.T) {
	m := withMockBranchRunner(t)
	sessionID := "sess-main"
	stateDir := t.TempDir()

	wtRoot := t.TempDir()
	gitDir := filepath.Join(wtRoot, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	wtPath, err := filepath.EvalSymlinks(wtRoot)
	if err != nil {
		t.Fatalf("evalsymlinks wt: %v", err)
	}
	canonicalGitDir, err := filepath.EvalSymlinks(gitDir)
	if err != nil {
		t.Fatalf("evalsymlinks git: %v", err)
	}
	cwd := wtPath

	mockWorktreeList(m, cwd, []struct{ Path, Branch, GitDir string }{
		{Path: wtPath, Branch: "main", GitDir: canonicalGitDir},
	})

	agent := domain.Agent{SessionID: sessionID, Cwd: cwd, State: "running"}
	seedAgentJSON(t, stateDir, agent)
	sf := domain.StateFile{Agents: map[string]domain.Agent{sessionID: agent}}

	resolveAgentWorktree(&sf, stateDir)

	if got := sf.Agents[sessionID].WorktreeCwd; got != "" {
		t.Errorf("WorktreeCwd = %q, want empty (main worktree must not be auto-claimed)", got)
	}
	if _, err := os.Stat(filepath.Join(canonicalGitDir, "agent-dashboard-session")); !os.IsNotExist(err) {
		t.Errorf("Go must not write marker into main .git (stat err=%v)", err)
	}
}

// TestResolveAgentWorktree_NoMarker_NoClaimWhenMarkerOwnedByOtherSession
// guards against the Go reconciler overwriting a marker that the JS hook (or
// another dashboard process) already claimed for a different session. The
// existing marker-mismatch loop covers this for cwd=source-repo agents; this
// case is the cwd=linked-worktree variant where the new fallback fires.
func TestResolveAgentWorktree_NoMarker_NoClaimWhenMarkerOwnedByOtherSession(t *testing.T) {
	m := withMockBranchRunner(t)
	sessionID := "sess-mine"
	stateDir := t.TempDir()

	wtRoot := t.TempDir()
	gitDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(wtRoot, ".git"), []byte("gitdir: "+gitDir+"\n"), 0o600); err != nil {
		t.Fatalf("write .git pointer: %v", err)
	}
	writeMarker(t, gitDir, "sess-other")
	wtPath, err := filepath.EvalSymlinks(wtRoot)
	if err != nil {
		t.Fatalf("evalsymlinks wt: %v", err)
	}
	cwd := wtPath

	mockWorktreeList(m, cwd, []struct{ Path, Branch, GitDir string }{
		{Path: wtPath, Branch: "feat/x", GitDir: gitDir},
	})

	agent := domain.Agent{SessionID: sessionID, Cwd: cwd, State: "running"}
	seedAgentJSON(t, stateDir, agent)
	sf := domain.StateFile{Agents: map[string]domain.Agent{sessionID: agent}}

	resolveAgentWorktree(&sf, stateDir)

	if got := sf.Agents[sessionID].WorktreeCwd; got != "" {
		t.Errorf("WorktreeCwd = %q, want empty (marker belongs to another session)", got)
	}
	data, _ := os.ReadFile(filepath.Join(gitDir, "agent-dashboard-session"))
	if strings.TrimSpace(string(data)) != "sess-other" {
		t.Errorf("marker was overwritten: got %q, want sess-other", string(data))
	}
}

// Restart-survivors sort after every live state group so the tree renders
// them in a dedicated bottom RESUMABLE section.
func TestSortedAgentsResumableLast(t *testing.T) {
	sf := domain.StateFile{Agents: map[string]domain.Agent{
		"orph": {SessionID: "orph", State: "running", TmuxPaneID: "%9", Window: 1, Resumable: true},
		"live": {SessionID: "live", State: "idle_prompt", TmuxPaneID: "%1", Window: 2},
	}}
	agents := SortedAgents(sf, "")
	if len(agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(agents))
	}
	if agents[0].SessionID != "live" || agents[1].SessionID != "orph" {
		t.Errorf("resumable agent should sort last, got order %s, %s", agents[0].SessionID, agents[1].SessionID)
	}
}

// flagResumable stamps the transient flag from one enumeration snapshot so
// every consumer (sort, tree, palette, web JSON) sees the same verdict.
func TestFlagResumable(t *testing.T) {
	surv := survivorAgent(t)
	live := survivorAgent(t)
	live.SessionID = "live"
	live.TmuxPaneID = "%1"
	sf := domain.StateFile{Agents: map[string]domain.Agent{"s": surv, "live": live}}

	flagResumable(&sf, map[string]bool{"%1": true}, "100", time.Now())
	if !sf.Agents["s"].Resumable {
		t.Error("dead-pane survivor should be flagged resumable")
	}
	if sf.Agents["live"].Resumable {
		t.Error("live agent should not be flagged resumable")
	}
}
