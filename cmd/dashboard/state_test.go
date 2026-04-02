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
			"s5": {Target: "a:5.0", State: "done", Window: 5, Pane: 0, TmuxPaneID: "%5"},
			"s1": {Target: "a:1.0", State: "question", Window: 1, Pane: 0, TmuxPaneID: "%1"},
			"s3": {Target: "a:3.0", State: "running", Window: 3, Pane: 0, TmuxPaneID: "%3"},
			"s0": {Target: "a:0.0", State: "permission", Window: 0, Pane: 0, TmuxPaneID: "%0"},
			"s4": {Target: "a:4.0", State: "idle_prompt", Window: 4, Pane: 0, TmuxPaneID: "%4"},
			"s2": {Target: "a:2.0", State: "error", Window: 2, Pane: 0, TmuxPaneID: "%2"},
		},
	}

	sorted := SortedAgents(sf, "")

	// Group 1: blocked (permission, question, error) sorted by window
	// Group 2: running
	// Group 3: finished (idle_prompt, done) sorted by window
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

func TestSortedAgents_LegacyStates(t *testing.T) {
	// Legacy states (input, idle) should still sort correctly
	sf := StateFile{
		Agents: map[string]Agent{
			"s2": {Target: "a:2.0", State: "done", Window: 2, Pane: 0, TmuxPaneID: "%2"},
			"s0": {Target: "a:0.0", State: "input", Window: 0, Pane: 0, TmuxPaneID: "%0"},
			"s1": {Target: "a:1.0", State: "running", Window: 1, Pane: 0, TmuxPaneID: "%1"},
			"s3": {Target: "a:3.0", State: "idle", Window: 3, Pane: 0, TmuxPaneID: "%3"},
		},
	}

	sorted := SortedAgents(sf, "")
	expected := []string{"input", "running", "done", "idle"}

	if len(sorted) != 4 {
		t.Fatalf("expected 4 agents, got %d", len(sorted))
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

func TestResolveAgentTargets(t *testing.T) {
	sf := StateFile{
		Agents: map[string]Agent{
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

	// Simulate tmux having renumbered windows (window 3 → 2, window 2 → 1)
	paneTargets := map[string]PaneTarget{
		"%90": {Session: "tomoro", Window: 2, Pane: 2, Target: "tomoro:2.2"},
		"%87": {Session: "tomoro", Window: 1, Pane: 1, Target: "tomoro:1.1"},
	}

	ResolveAgentTargets(&sf, paneTargets)

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
	sf := StateFile{
		Agents: map[string]Agent{
			"alive": {Target: "tomoro:3.2", TmuxPaneID: "%90", Session: "tomoro", Window: 3, Pane: 2, State: "running"},
			"dead":  {Target: "tomoro:2.1", TmuxPaneID: "%87", Session: "tomoro", Window: 2, Pane: 1, State: "input"},
		},
	}

	// Only %90 is live; %87 is dead (not in the map)
	paneTargets := map[string]PaneTarget{
		"%90": {Session: "tomoro", Window: 2, Pane: 2, Target: "tomoro:2.2"},
	}

	ResolveAgentTargets(&sf, paneTargets)

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
	sf := StateFile{
		Agents: map[string]Agent{
			"s1": {Target: "tomoro:3.2", TmuxPaneID: "%90", Window: 3, Pane: 2, State: "running"},
		},
	}
	// Empty (non-nil) map — all agents should be left unchanged
	ResolveAgentTargets(&sf, map[string]PaneTarget{})
	if sf.Agents["s1"].Window != 3 {
		t.Errorf("expected window unchanged with empty map")
	}
}

func TestResolveAgentTargets_NilMap(t *testing.T) {
	sf := StateFile{
		Agents: map[string]Agent{
			"s1": {Target: "tomoro:3.2", TmuxPaneID: "%90", Window: 3, Pane: 2, State: "running"},
		},
	}
	// nil paneTargets should not panic, agents left unchanged
	ResolveAgentTargets(&sf, nil)
	if sf.Agents["s1"].Window != 3 {
		t.Errorf("expected window unchanged with nil map")
	}
}

func TestResolveAgentBranches(t *testing.T) {
	// Use the current repo directory — we know it's a git repo.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	sf := StateFile{
		Agents: map[string]Agent{
			"with-cwd": {Cwd: cwd, Branch: "stale-branch", State: "running"},
			"no-cwd":   {Cwd: "", Branch: "should-stay", State: "running"},
			"bad-cwd":  {Cwd: "/nonexistent/path", Branch: "should-stay", State: "running"},
		},
	}

	ResolveAgentBranches(&sf, nil)

	// Agent with valid cwd should have branch updated to something non-empty and not the stale value
	if sf.Agents["with-cwd"].Branch == "stale-branch" || sf.Agents["with-cwd"].Branch == "" {
		t.Errorf("expected branch to be resolved from git, got %q", sf.Agents["with-cwd"].Branch)
	}

	// Agent without cwd should be unchanged
	if sf.Agents["no-cwd"].Branch != "should-stay" {
		t.Errorf("expected branch unchanged for no-cwd agent, got %q", sf.Agents["no-cwd"].Branch)
	}

	// Agent with bad cwd should be unchanged
	if sf.Agents["bad-cwd"].Branch != "should-stay" {
		t.Errorf("expected branch unchanged for bad-cwd agent, got %q", sf.Agents["bad-cwd"].Branch)
	}
}

func TestResolveAgentBranches_WorktreeCwd(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	sf := StateFile{
		Agents: map[string]Agent{
			// WorktreeCwd set to valid git dir — should use it
			"worktree": {WorktreeCwd: cwd, Cwd: "/nonexistent/path", Branch: "stale", State: "running"},
			// WorktreeCwd set to invalid path — should fall back to Cwd
			"fallback": {WorktreeCwd: "/nonexistent/worktree", Cwd: cwd, Branch: "stale", State: "running"},
			// WorktreeCwd empty — should use Cwd as before
			"no-worktree": {Cwd: cwd, Branch: "stale", State: "running"},
			// Both invalid — should be unchanged
			"both-bad": {WorktreeCwd: "/bad/wt", Cwd: "/bad/cwd", Branch: "should-stay", State: "running"},
		},
	}

	ResolveAgentBranches(&sf, nil)

	if sf.Agents["worktree"].Branch == "stale" || sf.Agents["worktree"].Branch == "" {
		t.Errorf("worktree: expected branch from WorktreeCwd, got %q", sf.Agents["worktree"].Branch)
	}
	if sf.Agents["fallback"].Branch == "stale" || sf.Agents["fallback"].Branch == "" {
		t.Errorf("fallback: expected branch from Cwd fallback, got %q", sf.Agents["fallback"].Branch)
	}
	if sf.Agents["no-worktree"].Branch == "stale" || sf.Agents["no-worktree"].Branch == "" {
		t.Errorf("no-worktree: expected branch from Cwd, got %q", sf.Agents["no-worktree"].Branch)
	}
	if sf.Agents["both-bad"].Branch != "should-stay" {
		t.Errorf("both-bad: expected unchanged, got %q", sf.Agents["both-bad"].Branch)
	}
}

func TestResolveAgentBranches_PaneCwdFallback(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	sf := StateFile{
		Agents: map[string]Agent{
			// No Cwd, no WorktreeCwd — should backfill from paneCwds and resolve branch
			"no-cwd": {TmuxPaneID: "%10", Branch: "stale", State: "running"},
			// Has Cwd already — should NOT be overwritten by paneCwds
			"has-cwd": {TmuxPaneID: "%11", Cwd: cwd, Branch: "stale", State: "running"},
			// No TmuxPaneID — cannot use paneCwds fallback
			"no-pane": {Branch: "should-stay", State: "running"},
		},
	}

	paneCwds := map[string]string{
		"%10": cwd,
		"%11": "/should/not/be/used",
	}

	ResolveAgentBranches(&sf, paneCwds)

	// no-cwd: Cwd should be backfilled and branch resolved
	if sf.Agents["no-cwd"].Cwd != cwd {
		t.Errorf("no-cwd: expected Cwd backfilled to %q, got %q", cwd, sf.Agents["no-cwd"].Cwd)
	}
	if sf.Agents["no-cwd"].Branch == "stale" || sf.Agents["no-cwd"].Branch == "" {
		t.Errorf("no-cwd: expected branch resolved from paneCwds, got %q", sf.Agents["no-cwd"].Branch)
	}

	// has-cwd: Cwd should remain unchanged (not overwritten)
	if sf.Agents["has-cwd"].Cwd != cwd {
		t.Errorf("has-cwd: Cwd should remain %q, got %q", cwd, sf.Agents["has-cwd"].Cwd)
	}

	// no-pane: should be unchanged
	if sf.Agents["no-pane"].Branch != "should-stay" {
		t.Errorf("no-pane: expected unchanged, got %q", sf.Agents["no-pane"].Branch)
	}
}

func TestGitBranch(t *testing.T) {
	// Valid git repo
	cwd, _ := os.Getwd()
	branch := gitBranch(cwd)
	if branch == "" {
		t.Error("expected non-empty branch for current repo")
	}

	// Invalid path
	if gitBranch("/nonexistent/path") != "" {
		t.Error("expected empty branch for invalid path")
	}

	// Empty path — git -C "" resolves to cwd, so gitBranch may return
	// a value. ResolveAgentBranches guards against empty Cwd upstream.
}

func TestEffectiveDir(t *testing.T) {
	tests := []struct {
		name  string
		agent Agent
		want  string
	}{
		{"worktree preferred", Agent{Cwd: "/launch", WorktreeCwd: "/worktree"}, "/worktree"},
		{"cwd fallback", Agent{Cwd: "/launch"}, "/launch"},
		{"both empty", Agent{}, ""},
		{"worktree only", Agent{WorktreeCwd: "/worktree"}, "/worktree"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.agent.EffectiveDir(); got != tt.want {
				t.Errorf("EffectiveDir() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCleanStale_SkipsLivePanes(t *testing.T) {
	dir := t.TempDir()
	staleTime := "2020-01-01T00:00:00Z" // very old

	// Agent with a live tmux pane — should NOT be cleaned even if stale
	writeAgentFile(t, dir, "live-agent.json", Agent{
		SessionID:  "live-agent",
		TmuxPaneID: "%42",
		State:      "input",
		UpdatedAt:  staleTime,
	})

	// Agent with a dead tmux pane — SHOULD be cleaned because stale + dead
	writeAgentFile(t, dir, "dead-agent.json", Agent{
		SessionID:  "dead-agent",
		TmuxPaneID: "%99",
		State:      "input",
		UpdatedAt:  staleTime,
	})

	livePaneIDs := map[string]bool{"%42": true}
	CleanStale(dir, 10*60, livePaneIDs)

	// Live agent should survive
	agentsPath := filepath.Join(dir, "agents")
	if _, err := os.Stat(filepath.Join(agentsPath, "live-agent.json")); err != nil {
		t.Error("live agent was incorrectly cleaned: pane is still alive")
	}

	// Dead agent should be removed
	if _, err := os.Stat(filepath.Join(agentsPath, "dead-agent.json")); err == nil {
		t.Error("dead stale agent was not cleaned")
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

func TestIsBlocked(t *testing.T) {
	blocked := []string{"permission", "question", "error", "input"}
	for _, s := range blocked {
		if !isBlocked(s) {
			t.Errorf("expected isBlocked(%q) = true", s)
		}
	}
	notBlocked := []string{"running", "done", "idle_prompt", "idle", "merged", "unknown"}
	for _, s := range notBlocked {
		if isBlocked(s) {
			t.Errorf("expected isBlocked(%q) = false", s)
		}
	}
}

func TestIsFinished(t *testing.T) {
	finished := []string{"done", "idle_prompt", "idle"}
	for _, s := range finished {
		if !isFinished(s) {
			t.Errorf("expected isFinished(%q) = true", s)
		}
	}
	notFinished := []string{"permission", "question", "error", "input", "running", "merged", "unknown"}
	for _, s := range notFinished {
		if isFinished(s) {
			t.Errorf("expected isFinished(%q) = false", s)
		}
	}
}

func TestIsMerged(t *testing.T) {
	if !isMerged("merged") {
		t.Error("expected isMerged(\"merged\") = true")
	}
	for _, s := range []string{"done", "idle_prompt", "running", "permission", "unknown"} {
		if isMerged(s) {
			t.Errorf("expected isMerged(%q) = false", s)
		}
	}
}

func TestPromoteMerged(t *testing.T) {
	// Use the current repo — we're on a branch that may or may not be merged.
	// Instead, test the logic with agents that have no valid git dir (won't promote).
	sf := StateFile{
		Agents: map[string]Agent{
			"done-no-dir":   {State: "done", Cwd: "/nonexistent"},
			"idle-no-dir":   {State: "idle_prompt", Cwd: "/nonexistent"},
			"running-agent": {State: "running", Cwd: "/nonexistent"},
		},
	}

	PromoteMerged(&sf)

	// No agent should be promoted (invalid dirs)
	if sf.Agents["done-no-dir"].State != "done" {
		t.Errorf("expected done unchanged, got %s", sf.Agents["done-no-dir"].State)
	}
	if sf.Agents["idle-no-dir"].State != "idle_prompt" {
		t.Errorf("expected idle_prompt unchanged, got %s", sf.Agents["idle-no-dir"].State)
	}
	if sf.Agents["running-agent"].State != "running" {
		t.Errorf("expected running unchanged, got %s", sf.Agents["running-agent"].State)
	}
}

func TestSortedAgents_MergedGroup(t *testing.T) {
	sf := StateFile{
		Agents: map[string]Agent{
			"s0": {Target: "a:0.0", State: "permission", Window: 0, Pane: 0, TmuxPaneID: "%0"},
			"s1": {Target: "a:1.0", State: "running", Window: 1, Pane: 0, TmuxPaneID: "%1"},
			"s2": {Target: "a:2.0", State: "done", Window: 2, Pane: 0, TmuxPaneID: "%2"},
			"s3": {Target: "a:3.0", State: "merged", Window: 3, Pane: 0, TmuxPaneID: "%3"},
		},
	}

	sorted := SortedAgents(sf, "")
	expected := []string{"permission", "running", "done", "merged"}

	if len(sorted) != 4 {
		t.Fatalf("expected 4 agents, got %d", len(sorted))
	}
	for i, want := range expected {
		if sorted[i].State != want {
			t.Errorf("position %d: expected %s, got %s", i, want, sorted[i].State)
		}
	}
}
