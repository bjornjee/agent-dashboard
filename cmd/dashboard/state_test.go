package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeAgentFile is a test helper that writes an agent JSON file to dir/agents/.
func writeAgentFile(t *testing.T, dir, filename string, agent Agent) {
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
	sf := ReadState("/nonexistent/path")
	if len(sf.Agents) != 0 {
		t.Errorf("expected empty agents, got %d", len(sf.Agents))
	}
}

func TestReadState_EmptyAgentsDir(t *testing.T) {
	tmp := t.TempDir()
	os.MkdirAll(filepath.Join(tmp, "agents"), 0755)

	sf := ReadState(tmp)
	if len(sf.Agents) != 0 {
		t.Errorf("expected empty agents, got %d", len(sf.Agents))
	}
}

func TestReadState_InvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	agentsDir := filepath.Join(tmp, "agents")
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(filepath.Join(agentsDir, "bad.json"), []byte("not json{{{"), 0644)

	sf := ReadState(tmp)
	if len(sf.Agents) != 0 {
		t.Errorf("expected empty agents for invalid JSON, got %d", len(sf.Agents))
	}
}

func TestReadState_ValidState(t *testing.T) {
	tmp := t.TempDir()
	writeAgentFile(t, tmp, "sess-a.json", Agent{SessionID: "sess-a", Target: "a:0.1", State: "running", Session: "a", TmuxPaneID: "%1"})
	writeAgentFile(t, tmp, "sess-b.json", Agent{SessionID: "sess-b", Target: "b:1.0", State: "input", Session: "b", TmuxPaneID: "%2"})

	sf := ReadState(tmp)
	if len(sf.Agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(sf.Agents))
	}
	if sf.Agents["sess-a"].State != "running" {
		t.Errorf("expected running, got %s", sf.Agents["sess-a"].State)
	}
}

func TestReadState_FallbackToFilename(t *testing.T) {
	tmp := t.TempDir()
	// Agent without session_id — should use filename stem as key
	writeAgentFile(t, tmp, "fallback-key.json", Agent{Target: "a:0.1", State: "running", Session: "a"})

	sf := ReadState(tmp)
	if len(sf.Agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(sf.Agents))
	}
	if _, ok := sf.Agents["fallback-key"]; !ok {
		t.Error("expected agent keyed by filename stem 'fallback-key'")
	}
}

func TestSortedAgents_Priority(t *testing.T) {
	sf := StateFile{
		Agents: map[string]Agent{
			"s3": {Target: "a:3.0", State: "done", Window: 3, Pane: 0, TmuxPaneID: "%3"},
			"s1": {Target: "a:1.0", State: "input", Window: 1, Pane: 0, TmuxPaneID: "%1"},
			"s2": {Target: "a:2.0", State: "running", Window: 2, Pane: 0, TmuxPaneID: "%2"},
			"s0": {Target: "a:0.0", State: "error", Window: 0, Pane: 0, TmuxPaneID: "%0"},
			"s4": {Target: "a:4.0", State: "idle", Window: 4, Pane: 0, TmuxPaneID: "%4"},
		},
	}

	sorted := SortedAgents(sf, "")

	// Group 1: needs attention (input, error) sorted by window
	// Group 2: running
	// Group 3: completed (idle, done) sorted by window
	expected := []string{"error", "input", "running", "done", "idle"}

	if len(sorted) != 5 {
		t.Fatalf("expected 5 agents, got %d", len(sorted))
	}
	for i, want := range expected {
		if sorted[i].State != want {
			t.Errorf("position %d: expected %s, got %s", i, want, sorted[i].State)
		}
	}
}

func TestSortedAgents_ExcludesSelfByPaneID(t *testing.T) {
	sf := StateFile{
		Agents: map[string]Agent{
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
	sf := StateFile{
		Agents: map[string]Agent{
			"good":  {Target: "good", State: "running", TmuxPaneID: "%1"},
			"good2": {Target: "", State: "running"}, // empty target is ok
			"bad":   {Target: "bad", State: ""},     // empty state → skipped
		},
	}

	sorted := SortedAgents(sf, "")
	if len(sorted) != 2 {
		t.Errorf("expected 2 valid agents (only empty-state skipped), got %d", len(sorted))
	}
}

func TestRemoveAgent(t *testing.T) {
	tmp := t.TempDir()
	writeAgentFile(t, tmp, "sess-a.json", Agent{SessionID: "sess-a", Target: "a:0.1", State: "running", TmuxPaneID: "%1"})
	writeAgentFile(t, tmp, "sess-b.json", Agent{SessionID: "sess-b", Target: "b:1.0", State: "input", TmuxPaneID: "%2"})

	err := RemoveAgent(tmp, "sess-a")
	if err != nil {
		t.Fatalf("RemoveAgent failed: %v", err)
	}

	sf := ReadState(tmp)
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
	writeAgentFile(t, tmp, "sess-a.json", Agent{SessionID: "sess-a", Target: "a:0.1", State: "running"})

	err := RemoveAgent(tmp, "nonexistent")
	if err != nil {
		t.Fatalf("RemoveAgent should not fail on nonexistent session_id: %v", err)
	}

	sf := ReadState(tmp)
	if len(sf.Agents) != 1 {
		t.Errorf("expected 1 agent unchanged, got %d", len(sf.Agents))
	}
}

func TestPruneDead_ByPaneID(t *testing.T) {
	tmp := t.TempDir()
	writeAgentFile(t, tmp, "sess-a.json", Agent{SessionID: "sess-a", Target: "main:1.0", State: "running", TmuxPaneID: "%1"})
	writeAgentFile(t, tmp, "sess-b.json", Agent{SessionID: "sess-b", Target: "main:1.1", State: "done", TmuxPaneID: "%2"})

	livePaneIDs := map[string]bool{
		"%0": true, // dashboard
		"%1": true, // agent A alive
		// %2 is dead
	}

	removed := PruneDead(tmp, livePaneIDs)
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}

	sf := ReadState(tmp)
	if len(sf.Agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(sf.Agents))
	}
	if _, ok := sf.Agents["sess-a"]; !ok {
		t.Error("sess-a should survive")
	}
}

func TestPruneDead_EmptyLivePanes(t *testing.T) {
	tmp := t.TempDir()
	writeAgentFile(t, tmp, "sess-a.json", Agent{SessionID: "sess-a", Target: "main:1.0", State: "running", TmuxPaneID: "%1"})
	writeAgentFile(t, tmp, "sess-b.json", Agent{SessionID: "sess-b", Target: "main:2.0", State: "running", TmuxPaneID: "%2"})

	// Empty non-nil map simulates tmux returning success with empty output.
	// PruneDead should refuse to delete all agents in this case.
	livePaneIDs := map[string]bool{}
	removed := PruneDead(tmp, livePaneIDs)
	if removed != 0 {
		t.Errorf("expected 0 removed with empty livePaneIDs, got %d", removed)
	}

	sf := ReadState(tmp)
	if len(sf.Agents) != 2 {
		t.Fatalf("expected 2 agents preserved, got %d", len(sf.Agents))
	}
}

func TestPruneDead_AllAgentsDead(t *testing.T) {
	tmp := t.TempDir()
	writeAgentFile(t, tmp, "sess-a.json", Agent{SessionID: "sess-a", Target: "main:1.0", State: "running", TmuxPaneID: "%1"})
	writeAgentFile(t, tmp, "sess-b.json", Agent{SessionID: "sess-b", Target: "main:2.0", State: "running", TmuxPaneID: "%2"})

	// livePaneIDs has panes but none match any agent — all would be deleted.
	// Safety net should refuse to wipe all agents at once.
	livePaneIDs := map[string]bool{
		"%0": true, // dashboard pane only
	}
	removed := PruneDead(tmp, livePaneIDs)
	if removed != 0 {
		t.Errorf("expected 0 removed when all agents would be wiped, got %d", removed)
	}

	sf := ReadState(tmp)
	if len(sf.Agents) != 2 {
		t.Fatalf("expected 2 agents preserved, got %d", len(sf.Agents))
	}
}

func TestPruneDead_PartialDead(t *testing.T) {
	tmp := t.TempDir()
	writeAgentFile(t, tmp, "sess-a.json", Agent{SessionID: "sess-a", Target: "main:1.0", State: "running", TmuxPaneID: "%1"})
	writeAgentFile(t, tmp, "sess-b.json", Agent{SessionID: "sess-b", Target: "main:1.1", State: "running", TmuxPaneID: "%2"})
	writeAgentFile(t, tmp, "sess-c.json", Agent{SessionID: "sess-c", Target: "main:2.0", State: "done", TmuxPaneID: "%3"})

	livePaneIDs := map[string]bool{
		"%0": true,
		"%1": true,
		"%2": true,
		// %3 is dead
	}

	removed := PruneDead(tmp, livePaneIDs)
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}

	sf := ReadState(tmp)
	if len(sf.Agents) != 2 {
		t.Fatalf("expected 2 agents after partial prune, got %d", len(sf.Agents))
	}
	if _, ok := sf.Agents["sess-c"]; ok {
		t.Error("sess-c should have been pruned")
	}
}

func TestPruneDead_NoPaneID(t *testing.T) {
	tmp := t.TempDir()
	// Agent without TmuxPaneID — should be considered dead
	writeAgentFile(t, tmp, "sess-a.json", Agent{SessionID: "sess-a", Target: "main:1.0", State: "running"})
	writeAgentFile(t, tmp, "sess-b.json", Agent{SessionID: "sess-b", Target: "main:2.0", State: "running", TmuxPaneID: "%2"})

	livePaneIDs := map[string]bool{
		"%2": true,
	}

	removed := PruneDead(tmp, livePaneIDs)
	if removed != 1 {
		t.Errorf("expected 1 removed (agent without pane ID), got %d", removed)
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
