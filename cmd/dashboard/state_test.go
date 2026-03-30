package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadState_MissingFile(t *testing.T) {
	sf := ReadState("/nonexistent/path.json")
	if len(sf.Agents) != 0 {
		t.Errorf("expected empty agents, got %d", len(sf.Agents))
	}
}

func TestReadState_InvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "state.json")
	os.WriteFile(path, []byte("not json{{{"), 0644)

	sf := ReadState(path)
	if len(sf.Agents) != 0 {
		t.Errorf("expected empty agents for invalid JSON, got %d", len(sf.Agents))
	}
}

func TestReadState_ValidState(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "state.json")
	os.WriteFile(path, []byte(`{
		"agents": {
			"a:0.1": {"target":"a:0.1","state":"running","session":"a"},
			"b:1.0": {"target":"b:1.0","state":"input","session":"b"}
		}
	}`), 0644)

	sf := ReadState(path)
	if len(sf.Agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(sf.Agents))
	}
	if sf.Agents["a:0.1"].State != "running" {
		t.Errorf("expected running, got %s", sf.Agents["a:0.1"].State)
	}
}

func TestSortedAgents_Priority(t *testing.T) {
	sf := StateFile{
		Agents: map[string]Agent{
			"a:3.0": {Target: "a:3.0", State: "done", Window: 3, Pane: 0},
			"a:1.0": {Target: "a:1.0", State: "input", Window: 1, Pane: 0},
			"a:2.0": {Target: "a:2.0", State: "running", Window: 2, Pane: 0},
			"a:0.0": {Target: "a:0.0", State: "error", Window: 0, Pane: 0},
			"a:4.0": {Target: "a:4.0", State: "idle", Window: 4, Pane: 0},
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

func TestSortedAgents_SkipsInvalid(t *testing.T) {
	sf := StateFile{
		Agents: map[string]Agent{
			"good": {Target: "good", State: "running"},
			"bad1": {Target: "", State: "running"},
			"bad2": {Target: "bad2", State: ""},
		},
	}

	sorted := SortedAgents(sf, "")
	if len(sorted) != 1 {
		t.Errorf("expected 1 valid agent, got %d", len(sorted))
	}
}

func TestRemoveAgent(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "state.json")
	os.WriteFile(path, []byte(`{
		"agents": {
			"a:0.1": {"target":"a:0.1","state":"running","session":"a"},
			"b:1.0": {"target":"b:1.0","state":"input","session":"b"}
		}
	}`), 0644)

	err := RemoveAgent(path, "a:0.1")
	if err != nil {
		t.Fatalf("RemoveAgent failed: %v", err)
	}

	sf := ReadState(path)
	if len(sf.Agents) != 1 {
		t.Fatalf("expected 1 agent after removal, got %d", len(sf.Agents))
	}
	if _, ok := sf.Agents["a:0.1"]; ok {
		t.Error("agent a:0.1 should have been removed")
	}
	if _, ok := sf.Agents["b:1.0"]; !ok {
		t.Error("agent b:1.0 should still exist")
	}
}

func TestRemoveAgent_NonExistent(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "state.json")
	os.WriteFile(path, []byte(`{"agents":{"a:0.1":{"target":"a:0.1","state":"running"}}}`), 0644)

	err := RemoveAgent(path, "nonexistent:0.0")
	if err != nil {
		t.Fatalf("RemoveAgent should not fail on nonexistent target: %v", err)
	}

	sf := ReadState(path)
	if len(sf.Agents) != 1 {
		t.Errorf("expected 1 agent unchanged, got %d", len(sf.Agents))
	}
}

func TestPruneDead_WithRenames(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "state.json")
	os.WriteFile(path, []byte(`{
		"agents": {
			"main:1.0": {"target":"main:1.0","state":"running","session":"main","cwd":"/code/a"},
			"main:2.0": {"target":"main:2.0","state":"running","session":"main","cwd":"/code/b"}
		}
	}`), 0644)

	// After killing main:1.0, window 2 renumbered to window 1
	livePanes := map[string]bool{
		"main:0.0": true, // dashboard
		"main:1.0": true, // agent B (was main:2.0, now renumbered)
	}
	renames := map[string]string{
		"main:2.0": "main:1.0", // B was renumbered
	}

	removed := PruneDead(path, livePanes, renames)

	sf := ReadState(path)

	// Only 1 agent should remain (B with new target)
	if len(sf.Agents) != 1 {
		t.Fatalf("expected 1 agent after prune+rename, got %d", len(sf.Agents))
	}

	// Agent B should now be at main:1.0 (renamed from main:2.0)
	agent, ok := sf.Agents["main:1.0"]
	if !ok {
		t.Fatal("main:1.0 should exist after rename")
	}
	if agent.Cwd != "/code/b" {
		t.Errorf("main:1.0 should be agent B (cwd /code/b), got %q", agent.Cwd)
	}

	// main:2.0 (old key for B) should be gone
	if _, ok := sf.Agents["main:2.0"]; ok {
		t.Error("main:2.0 should have been renamed to main:1.0")
	}

	_ = removed
}

func TestPruneDead_NoRenames(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "state.json")
	os.WriteFile(path, []byte(`{
		"agents": {
			"main:1.0": {"target":"main:1.0","state":"running","session":"main"},
			"main:1.1": {"target":"main:1.1","state":"done","session":"main"}
		}
	}`), 0644)

	livePanes := map[string]bool{
		"main:0.0": true,
		"main:1.0": true,
		// main:1.1 is dead
	}

	removed := PruneDead(path, livePanes, nil)
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}

	sf := ReadState(path)
	if len(sf.Agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(sf.Agents))
	}
	if _, ok := sf.Agents["main:1.0"]; !ok {
		t.Error("main:1.0 should survive")
	}
}

func TestPruneDead_EmptyLivePanes(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "state.json")
	os.WriteFile(path, []byte(`{
		"agents": {
			"main:1.0": {"target":"main:1.0","state":"running","session":"main"},
			"main:2.0": {"target":"main:2.0","state":"running","session":"main"}
		}
	}`), 0644)

	// Empty non-nil map simulates tmux returning success with empty output.
	// PruneDead should refuse to delete all agents in this case.
	livePanes := map[string]bool{}
	removed := PruneDead(path, livePanes, nil)
	if removed != 0 {
		t.Errorf("expected 0 removed with empty livePanes, got %d", removed)
	}

	sf := ReadState(path)
	if len(sf.Agents) != 2 {
		t.Fatalf("expected 2 agents preserved, got %d", len(sf.Agents))
	}
}

func TestPruneDead_AllAgentsDead(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "state.json")
	os.WriteFile(path, []byte(`{
		"agents": {
			"main:1.0": {"target":"main:1.0","state":"running","session":"main"},
			"main:2.0": {"target":"main:2.0","state":"running","session":"main"}
		}
	}`), 0644)

	// livePanes has panes but none match any agent — all would be deleted.
	// Safety net should refuse to wipe all agents at once.
	livePanes := map[string]bool{
		"main:0.0": true, // dashboard pane only
	}
	removed := PruneDead(path, livePanes, nil)
	if removed != 0 {
		t.Errorf("expected 0 removed when all agents would be wiped, got %d", removed)
	}

	sf := ReadState(path)
	if len(sf.Agents) != 2 {
		t.Fatalf("expected 2 agents preserved, got %d", len(sf.Agents))
	}
}

func TestPruneDead_AllDeadWithRenames(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "state.json")
	os.WriteFile(path, []byte(`{
		"agents": {
			"main:2.0": {"target":"main:2.0","state":"running","session":"main","cwd":"/code/a"},
			"main:3.0": {"target":"main:3.0","state":"running","session":"main","cwd":"/code/b"}
		}
	}`), 0644)

	// Window renumbering: main:2.0 → main:1.0, main:3.0 → main:2.0
	// But no agents match livePanes after rename, so safety net should fire.
	// Renames should still be persisted.
	livePanes := map[string]bool{
		"main:0.0": true,
		"main:1.0": true,
		"main:2.0": true,
	}
	renames := map[string]string{
		"main:2.0": "main:1.0",
		"main:3.0": "main:2.0",
	}

	removed := PruneDead(path, livePanes, renames)
	if removed != 0 {
		t.Errorf("expected 0 removed (safety net), got %d", removed)
	}

	sf := ReadState(path)
	if len(sf.Agents) != 2 {
		t.Fatalf("expected 2 agents preserved, got %d", len(sf.Agents))
	}

	// Renames should have been persisted even though safety net fired
	if a, ok := sf.Agents["main:1.0"]; !ok || a.Cwd != "/code/a" {
		t.Error("main:2.0 should have been renamed to main:1.0")
	}
	if a, ok := sf.Agents["main:2.0"]; !ok || a.Cwd != "/code/b" {
		t.Error("main:3.0 should have been renamed to main:2.0")
	}
}

func TestPruneDead_PartialDead(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "state.json")
	os.WriteFile(path, []byte(`{
		"agents": {
			"main:1.0": {"target":"main:1.0","state":"running","session":"main"},
			"main:1.1": {"target":"main:1.1","state":"running","session":"main"},
			"main:2.0": {"target":"main:2.0","state":"done","session":"main"}
		}
	}`), 0644)

	livePanes := map[string]bool{
		"main:0.0": true,
		"main:1.0": true,
		"main:1.1": true,
		// main:2.0 is dead
	}

	removed := PruneDead(path, livePanes, nil)
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}

	sf := ReadState(path)
	if len(sf.Agents) != 2 {
		t.Fatalf("expected 2 agents after partial prune, got %d", len(sf.Agents))
	}
	if _, ok := sf.Agents["main:2.0"]; ok {
		t.Error("main:2.0 should have been pruned")
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
