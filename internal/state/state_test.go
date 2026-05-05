package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	writeAgentFile(t, tmp, "sess-a.json", domain.Agent{SessionID: "sess-a", Target: "a:0.1", State: "running", Session: "a", TmuxPaneID: "%1"})
	writeAgentFile(t, tmp, "sess-b.json", domain.Agent{SessionID: "sess-b", Target: "b:1.0", State: "question", Session: "b", TmuxPaneID: "%2"})

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
	// domain.Agent without session_id — should use filename stem as key
	writeAgentFile(t, tmp, "fallback-key.json", domain.Agent{Target: "a:0.1", State: "running", Session: "a"})

	sf := ReadState(tmp)
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
	writeAgentFile(t, tmp, "sess-a.json", domain.Agent{SessionID: "sess-a", Target: "a:0.1", State: "running"})

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
	writeAgentFile(t, tmp, "sess-a.json", domain.Agent{SessionID: "sess-a", Target: "main:1.0", State: "running", TmuxPaneID: "%1"})
	writeAgentFile(t, tmp, "sess-b.json", domain.Agent{SessionID: "sess-b", Target: "main:1.1", State: "done", TmuxPaneID: "%2"})

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
	writeAgentFile(t, tmp, "sess-a.json", domain.Agent{SessionID: "sess-a", Target: "main:1.0", State: "running", TmuxPaneID: "%1"})
	writeAgentFile(t, tmp, "sess-b.json", domain.Agent{SessionID: "sess-b", Target: "main:2.0", State: "running", TmuxPaneID: "%2"})

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
	writeAgentFile(t, tmp, "sess-a.json", domain.Agent{SessionID: "sess-a", Target: "main:1.0", State: "running", TmuxPaneID: "%1"})
	writeAgentFile(t, tmp, "sess-b.json", domain.Agent{SessionID: "sess-b", Target: "main:2.0", State: "running", TmuxPaneID: "%2"})

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
	writeAgentFile(t, tmp, "sess-a.json", domain.Agent{SessionID: "sess-a", Target: "main:1.0", State: "running", TmuxPaneID: "%1"})
	writeAgentFile(t, tmp, "sess-b.json", domain.Agent{SessionID: "sess-b", Target: "main:1.1", State: "running", TmuxPaneID: "%2"})
	writeAgentFile(t, tmp, "sess-c.json", domain.Agent{SessionID: "sess-c", Target: "main:2.0", State: "done", TmuxPaneID: "%3"})

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
	// domain.Agent without TmuxPaneID — should be considered dead
	writeAgentFile(t, tmp, "sess-a.json", domain.Agent{SessionID: "sess-a", Target: "main:1.0", State: "running"})
	writeAgentFile(t, tmp, "sess-b.json", domain.Agent{SessionID: "sess-b", Target: "main:2.0", State: "running", TmuxPaneID: "%2"})

	livePaneIDs := map[string]bool{
		"%2": true,
	}

	removed := PruneDead(tmp, livePaneIDs)
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
	removed := PruneDead(tmp, livePaneIDs)
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
	removed := PruneDead(tmp, livePaneIDs)
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
	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			"s1": {Target: "tomoro:3.2", TmuxPaneID: "%90", Window: 3, Pane: 2, State: "running"},
		},
	}
	// Empty (non-nil) map — all agents should be left unchanged
	ResolveAgentTargets(&sf, map[string]domain.PaneTarget{})
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
	ResolveAgentTargets(&sf, nil)
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
			"with-cwd": {Cwd: "/valid/repo", Branch: "stale-branch", State: "running"},
			"no-cwd":   {Cwd: "", Branch: "stale", State: "running"},
			"bad-cwd":  {Cwd: "/nonexistent/path", Branch: "stale", State: "running"},
		},
	}

	ResolveAgentBranches(&sf, nil)

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

func TestResolveAgentBranches_WorktreeCwd(t *testing.T) {
	m := withMockBranchRunner(t)
	mockGitBranch(m, "/valid/repo", "feat/from-cwd")
	mockGitBranch(m, "/valid/worktree", "feat/from-worktree")
	mockGitBranch(m, "/nonexistent/worktree", "")
	mockGitBranch(m, "/bad/wt", "")
	mockGitBranch(m, "/bad/cwd", "")

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			"worktree":         {WorktreeCwd: "/valid/worktree", Cwd: "/nonexistent/path", Branch: "stale", State: "running"},
			"fallback":         {WorktreeCwd: "/nonexistent/worktree", Cwd: "/valid/repo", Branch: "stale", State: "running"},
			"no-worktree":      {Cwd: "/valid/repo", Branch: "stale", State: "running"},
			"both-bad":         {WorktreeCwd: "/bad/wt", Cwd: "/bad/cwd", Branch: "stale", State: "running"},
			"worktree-deleted": {WorktreeCwd: "/bad/wt", Branch: "feat/old-stale", State: "running"},
		},
	}

	ResolveAgentBranches(&sf, nil)

	if sf.Agents["worktree"].Branch != "feat/from-worktree" {
		t.Errorf("worktree: expected feat/from-worktree, got %q", sf.Agents["worktree"].Branch)
	}
	if sf.Agents["fallback"].Branch != "feat/from-cwd" {
		t.Errorf("fallback: expected feat/from-cwd, got %q", sf.Agents["fallback"].Branch)
	}
	if sf.Agents["no-worktree"].Branch != "feat/from-cwd" {
		t.Errorf("no-worktree: expected feat/from-cwd, got %q", sf.Agents["no-worktree"].Branch)
	}
	if sf.Agents["both-bad"].Branch != "" {
		t.Errorf("both-bad: expected branch cleared, got %q", sf.Agents["both-bad"].Branch)
	}
	if sf.Agents["worktree-deleted"].Branch != "" {
		t.Errorf("worktree-deleted: expected branch cleared after worktree gone, got %q", sf.Agents["worktree-deleted"].Branch)
	}
}

func TestResolveAgentBranches_PaneCwdFallback(t *testing.T) {
	m := withMockBranchRunner(t)
	mockGitBranch(m, "/pane/cwd", "feat/pane-branch")
	mockGitBranch(m, "/existing/cwd", "feat/existing-branch")
	mockGitBranch(m, "/worktree/path", "feat/x")

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			"no-cwd":               {TmuxPaneID: "%10", Branch: "stale", State: "running"},
			"has-cwd":              {TmuxPaneID: "%11", Cwd: "/existing/cwd", Branch: "stale", State: "running"},
			"no-pane":              {Branch: "stale", State: "running"},
			"cwd-stale-pane-fresh": {TmuxPaneID: "%20", Cwd: "/source/repo", Branch: "main", State: "running"},
		},
	}

	paneCwds := map[string]string{
		"%10": "/pane/cwd",
		"%11": "/existing/cwd",
		"%20": "/worktree/path",
	}

	ResolveAgentBranches(&sf, paneCwds)

	if sf.Agents["no-cwd"].Cwd != "/pane/cwd" {
		t.Errorf("no-cwd: expected Cwd backfilled to /pane/cwd, got %q", sf.Agents["no-cwd"].Cwd)
	}
	if sf.Agents["no-cwd"].Branch != "feat/pane-branch" {
		t.Errorf("no-cwd: expected feat/pane-branch, got %q", sf.Agents["no-cwd"].Branch)
	}
	if sf.Agents["has-cwd"].Cwd != "/existing/cwd" {
		t.Errorf("has-cwd: Cwd should remain /existing/cwd, got %q", sf.Agents["has-cwd"].Cwd)
	}
	if sf.Agents["no-pane"].Branch != "" {
		t.Errorf("no-pane: expected branch cleared (no resolvable cwd), got %q", sf.Agents["no-pane"].Branch)
	}
	// Live pane cwd wins over stored Cwd when both resolve — fixes the "agent
	// cd'd into a worktree mid-session, hook hasn't refired yet" case.
	if sf.Agents["cwd-stale-pane-fresh"].Branch != "feat/x" {
		t.Errorf("cwd-stale-pane-fresh: expected live pane branch feat/x to win over stored main, got %q",
			sf.Agents["cwd-stale-pane-fresh"].Branch)
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

func TestCleanStale_SkipsLivePanes(t *testing.T) {
	dir := t.TempDir()
	staleTime := "2020-01-01T00:00:00Z" // very old

	// domain.Agent with a live tmux pane — should NOT be cleaned even if stale
	writeAgentFile(t, dir, "live-agent.json", domain.Agent{
		SessionID:  "live-agent",
		TmuxPaneID: "%42",
		State:      "input",
		UpdatedAt:  staleTime,
	})

	// domain.Agent with a dead tmux pane — SHOULD be cleaned because stale + dead
	writeAgentFile(t, dir, "dead-agent.json", domain.Agent{
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

func TestCleanStale_DedupSamePane(t *testing.T) {
	dir := t.TempDir()
	staleTime := "2020-01-01T00:00:00Z"
	recentTime := time.Now().UTC().Format(time.RFC3339)

	// Two agents sharing pane %42 — only the newest should survive.
	writeAgentFile(t, dir, "old-session.json", domain.Agent{
		SessionID:  "old-session",
		TmuxPaneID: "%42",
		State:      "running",
		UpdatedAt:  staleTime,
	})
	writeAgentFile(t, dir, "new-session.json", domain.Agent{
		SessionID:  "new-session",
		TmuxPaneID: "%42",
		State:      "running",
		UpdatedAt:  recentTime,
	})

	livePaneIDs := map[string]bool{"%42": true}
	CleanStale(dir, 10*60, livePaneIDs)

	agentsPath := filepath.Join(dir, "agents")

	// Old session should be removed (duplicate pane, older)
	if _, err := os.Stat(filepath.Join(agentsPath, "old-session.json")); err == nil {
		t.Error("old duplicate-pane agent was not cleaned")
	}

	// New session should survive (newest for this pane + pane alive)
	if _, err := os.Stat(filepath.Join(agentsPath, "new-session.json")); err != nil {
		t.Error("newest agent for live pane was incorrectly cleaned")
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
			ApplyPinnedStates(&sf)
			if got := sf.Agents["agent"].State; got != tt.wantState {
				t.Errorf("got %q, want %q", got, tt.wantState)
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
	sf := ReadState(tmp)
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

// planTestSetup creates a temp directory structure that ApplyIdleOverrides can resolve.
// Returns (projectsDir, cwd) where cwd is the agent's working directory and
// projectsDir contains the JSONL at the path ApplyIdleOverrides expects.
func planTestSetup(t *testing.T, sessionID, jsonl string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	cwd := "/test/myproject"
	slug := conversation.ProjectSlug(cwd)
	projDir := filepath.Join(dir, slug)
	os.MkdirAll(projDir, 0755)
	os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(jsonl), 0644)
	return dir, cwd
}

func TestApplyIdleOverrides_OverridesIdlePrompt(t *testing.T) {
	sessionID := "sess-plan"
	// Realistic JSONL: ExitPlanMode is always followed by a tool_result user entry
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"ExitPlanMode","input":{}}]},"timestamp":"2026-03-28T10:00:00Z"}
{"type":"user","message":{"role":"user","content":[{"tool_use_id":"t1","type":"tool_result","content":"Plan submitted for review"}]},"timestamp":"2026-03-28T10:00:01Z"}
`
	projectsDir, cwd := planTestSetup(t, sessionID, jsonl)

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			sessionID: {SessionID: sessionID, State: "idle_prompt", Cwd: cwd},
		},
	}

	ApplyIdleOverrides(&sf, projectsDir)

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
	projectsDir, cwd := planTestSetup(t, sessionID, jsonl)

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			sessionID: {SessionID: sessionID, State: "running", Cwd: cwd},
		},
	}

	ApplyIdleOverrides(&sf, projectsDir)

	if sf.Agents[sessionID].State != "running" {
		t.Errorf("expected state 'running' unchanged (running agents should be skipped), got %q", sf.Agents[sessionID].State)
	}
}

func TestApplyIdleOverrides_NoOverrideWithoutPlan(t *testing.T) {
	sessionID := "sess-idle"
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"done"}]},"timestamp":"2026-03-28T10:00:00Z"}
`
	projectsDir, cwd := planTestSetup(t, sessionID, jsonl)

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			sessionID: {SessionID: sessionID, State: "idle_prompt", Cwd: cwd},
		},
	}

	ApplyIdleOverrides(&sf, projectsDir)

	if sf.Agents[sessionID].State != "idle_prompt" {
		t.Errorf("expected state 'idle_prompt' unchanged, got %q", sf.Agents[sessionID].State)
	}
}

func TestApplyIdleOverrides_QuestionOverride(t *testing.T) {
	sessionID := "sess-question"
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"AskUserQuestion","input":{"question":"Which approach?"}}]},"timestamp":"2026-03-28T10:00:00Z"}
{"type":"user","message":{"role":"user","content":[{"tool_use_id":"t1","type":"tool_result","content":"Option A"}]},"timestamp":"2026-03-28T10:00:01Z"}
`
	projectsDir, cwd := planTestSetup(t, sessionID, jsonl)

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			sessionID: {SessionID: sessionID, State: "idle_prompt", Cwd: cwd},
		},
	}

	ApplyIdleOverrides(&sf, projectsDir)

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
	projectsDir, cwd := planTestSetup(t, sessionID, jsonl)

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			sessionID: {SessionID: sessionID, State: "idle_prompt", Cwd: cwd},
		},
	}

	ApplyIdleOverrides(&sf, projectsDir)

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
	projectsDir, cwd := planTestSetup(t, sessionID, jsonl)

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			sessionID: {SessionID: sessionID, State: "permission", Cwd: cwd},
		},
	}

	ApplyIdleOverrides(&sf, projectsDir)

	if sf.Agents[sessionID].State != "plan" {
		t.Errorf("expected state 'plan' (permission agent with pending ExitPlanMode), got %q", sf.Agents[sessionID].State)
	}
}

func TestApplyIdleOverrides_LeavesPermissionAloneWhenNoPlan(t *testing.T) {
	sessionID := "sess-perm-real"
	// Real permission prompt — no ExitPlanMode in JSONL.
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Write","input":{"path":"foo.txt"}}]},"timestamp":"2026-03-28T10:00:00Z"}
`
	projectsDir, cwd := planTestSetup(t, sessionID, jsonl)

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			sessionID: {SessionID: sessionID, State: "permission", Cwd: cwd},
		},
	}

	ApplyIdleOverrides(&sf, projectsDir)

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
	projectsDir, cwd := planTestSetup(t, sessionID, jsonl)

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			sessionID: {SessionID: sessionID, State: "idle_prompt", Cwd: cwd},
		},
	}

	ApplyIdleOverrides(&sf, projectsDir)

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
	projectsDir, cwd := planTestSetup(t, sessionID, jsonl)

	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			sessionID: {SessionID: sessionID, State: "idle_prompt", Cwd: cwd},
		},
	}

	ApplyIdleOverrides(&sf, projectsDir)

	if sf.Agents[sessionID].State != "plan" {
		t.Errorf("expected 'plan' (ExitPlanMode after AskUserQuestion), got %q", sf.Agents[sessionID].State)
	}
}
