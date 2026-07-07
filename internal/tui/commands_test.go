package tui

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/db"
	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/mocks"
	"github.com/stretchr/testify/mock"
)

func TestValidateFolder_ValidDir(t *testing.T) {
	dir := t.TempDir()
	absPath, err := validateFolder(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if absPath != dir {
		t.Errorf("expected %q, got %q", dir, absPath)
	}
}

func TestValidateFolder_Missing(t *testing.T) {
	_, err := validateFolder("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error for nonexistent path")
	}
	if !strings.Contains(err.Error(), "folder not found") {
		t.Errorf("expected 'folder not found' in error, got: %v", err)
	}
	// Should contain the underlying OS error (wrapped via %w)
	if !strings.Contains(err.Error(), "no such file") {
		t.Errorf("expected underlying OS error in message, got: %v", err)
	}
	// Verify error wrapping works with errors.Is
	if !errors.Is(err, fs.ErrNotExist) {
		t.Error("expected error to unwrap to fs.ErrNotExist")
	}
}

func TestValidateFolder_NotDir(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "afile.txt")
	if err := os.WriteFile(file, []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := validateFolder(file)
	if err == nil {
		t.Fatal("expected error for file path")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("expected 'not a directory' in error, got: %v", err)
	}
}

func TestParseGitHubRepo(t *testing.T) {
	tests := []struct {
		name      string
		remoteURL string
		wantOwner string
		wantRepo  string
		wantOK    bool
	}{
		{
			name:      "SSH URL",
			remoteURL: "git@github.com:bjornjee/agent-dashboard.git",
			wantOwner: "bjornjee",
			wantRepo:  "agent-dashboard",
			wantOK:    true,
		},
		{
			name:      "HTTPS URL",
			remoteURL: "https://github.com/bjornjee/agent-dashboard.git",
			wantOwner: "bjornjee",
			wantRepo:  "agent-dashboard",
			wantOK:    true,
		},
		{
			name:      "HTTPS without .git suffix",
			remoteURL: "https://github.com/bjornjee/agent-dashboard",
			wantOwner: "bjornjee",
			wantRepo:  "agent-dashboard",
			wantOK:    true,
		},
		{
			name:      "SSH without .git suffix",
			remoteURL: "git@github.com:bjornjee/agent-dashboard",
			wantOwner: "bjornjee",
			wantRepo:  "agent-dashboard",
			wantOK:    true,
		},
		{
			name:      "non-GitHub SSH",
			remoteURL: "git@gitlab.com:bjornjee/agent-dashboard.git",
			wantOK:    false,
		},
		{
			name:      "non-GitHub HTTPS",
			remoteURL: "https://gitlab.com/bjornjee/agent-dashboard.git",
			wantOK:    false,
		},
		{
			name:      "empty string",
			remoteURL: "",
			wantOK:    false,
		},
		{
			name:      "malformed URL",
			remoteURL: "not-a-url",
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, ok := parseGitHubRepo(tt.remoteURL)
			if owner != tt.wantOwner || repo != tt.wantRepo || ok != tt.wantOK {
				t.Errorf("parseGitHubRepo(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.remoteURL, owner, repo, ok, tt.wantOwner, tt.wantRepo, tt.wantOK)
			}
		})
	}
}

func TestBuildPRURL(t *testing.T) {
	tests := []struct {
		name   string
		owner  string
		repo   string
		base   string
		branch string
		want   string
	}{
		{
			name:   "simple branch",
			owner:  "bjornjee",
			repo:   "agent-dashboard",
			base:   "main",
			branch: "feat/auto-open-pr",
			want:   "https://github.com/bjornjee/agent-dashboard/compare/main...feat%2Fauto-open-pr?expand=1",
		},
		{
			name:   "master base",
			owner:  "bjornjee",
			repo:   "agent-dashboard",
			base:   "master",
			branch: "fix-bug",
			want:   "https://github.com/bjornjee/agent-dashboard/compare/master...fix-bug?expand=1",
		},
		{
			name:   "branch with special chars",
			owner:  "bjornjee",
			repo:   "agent-dashboard",
			base:   "main",
			branch: "feat/hello world",
			want:   "https://github.com/bjornjee/agent-dashboard/compare/main...feat%2Fhello%20world?expand=1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPRURL(tt.owner, tt.repo, tt.base, tt.branch)
			if got != tt.want {
				t.Errorf("buildPRURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolvePRURL(t *testing.T) {
	tests := []struct {
		name    string
		owner   string
		repo    string
		base    string
		branch  string
		ghPRURL string // non-empty means a PR exists
		wantURL string
	}{
		{
			name:    "no existing PR opens compare page",
			owner:   "bjornjee",
			repo:    "agent-dashboard",
			base:    "main",
			branch:  "fix/my-bug",
			ghPRURL: "",
			wantURL: "https://github.com/bjornjee/agent-dashboard/compare/main...fix%2Fmy-bug?expand=1",
		},
		{
			name:    "existing PR opens files page",
			owner:   "bjornjee",
			repo:    "agent-dashboard",
			base:    "main",
			branch:  "fix/my-bug",
			ghPRURL: "https://github.com/bjornjee/agent-dashboard/pull/42",
			wantURL: "https://github.com/bjornjee/agent-dashboard/pull/42/files",
		},
		{
			name:    "existing PR URL with trailing slash",
			owner:   "bjornjee",
			repo:    "agent-dashboard",
			base:    "main",
			branch:  "fix/my-bug",
			ghPRURL: "https://github.com/bjornjee/agent-dashboard/pull/42/",
			wantURL: "https://github.com/bjornjee/agent-dashboard/pull/42/files",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolvePRURL(tt.owner, tt.repo, tt.base, tt.branch, tt.ghPRURL)
			if got != tt.wantURL {
				t.Errorf("resolvePRURL() = %q, want %q", got, tt.wantURL)
			}
		})
	}
}

func TestValidateFolder_TildeExpansion(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}
	absPath, err := validateFolder("~")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if absPath != home {
		t.Errorf("expected %q, got %q", home, absPath)
	}
}

func TestLoadAllSubagents_RoutesCodexAgentToRolloutDiscovery(t *testing.T) {
	root := t.TempDir()
	parentID := "parent-codex"
	childID := "child-codex"
	writeTUIRollout(t, root, childID, `{"timestamp":"2026-05-21T14:44:03.645Z","type":"session_meta","payload":{"id":"child-codex","timestamp":"2026-05-21T14:44:03.645Z","source":{"subagent":{"thread_spawn":{"parent_thread_id":"parent-codex","agent_nickname":"Nietzsche","agent_role":"explorer"}}},"thread_source":"subagent","agent_nickname":"Nietzsche","agent_role":"explorer"}}
`)

	m := model{
		agents: []domain.Agent{{
			Target:    "main:1.0",
			SessionID: parentID,
			Harness:   "codex",
		}},
		codexSessionsDir: root,
	}

	cmd := m.loadAllSubagents()
	if cmd == nil {
		t.Fatal("loadAllSubagents returned nil, want batch cmd")
	}
	msg, ok := cmd().(subagentsBatchMsg)
	if !ok {
		t.Fatalf("got %T, want subagentsBatchMsg", cmd())
	}
	subs, ok := msg.byTarget["main:1.0"]
	if !ok {
		t.Fatalf("byTarget missing main:1.0: %+v", msg.byTarget)
	}
	if len(subs) != 1 {
		t.Fatalf("got %d subagents, want 1: %+v", len(subs), subs)
	}
	if subs[0].AgentID != childID {
		t.Errorf("AgentID = %q, want %q", subs[0].AgentID, childID)
	}
}

func TestLoadSubagentActivity_RoutesCodexSubagentToRollout(t *testing.T) {
	root := t.TempDir()
	childID := "child-codex"
	writeTUIRollout(t, root, childID, `{"timestamp":"2026-05-21T14:44:03.645Z","type":"session_meta","payload":{"id":"child-codex","timestamp":"2026-05-21T14:44:03.645Z"}}
{"timestamp":"2026-05-21T14:45:00.000Z","type":"event_msg","payload":{"type":"agent_message","message":"child result"}}
`)

	m := model{
		agents: []domain.Agent{{
			Target:    "main:1.0",
			SessionID: "parent-codex",
			Harness:   "codex",
		}},
		agentSubagents: map[string][]domain.SubagentInfo{
			"main:1.0": {{AgentID: childID}},
		},
		codexSessionsDir: root,
	}
	m.buildTree()
	m.selected = 2

	cmd := m.loadSubagentActivity()
	if cmd == nil {
		t.Fatal("loadSubagentActivity returned nil, want activity command")
	}
	msg, ok := cmd().(activityMsg)
	if !ok {
		t.Fatalf("got %T, want activityMsg", cmd())
	}
	if len(msg.entries) != 1 {
		t.Fatalf("got %d activity entries, want 1: %+v", len(msg.entries), msg.entries)
	}
	if msg.entries[0].Kind != "assistant" || msg.entries[0].Content != "child result" {
		t.Errorf("entry = %+v, want assistant child result", msg.entries[0])
	}
}

func TestLoadPlan_RoutesCodexAgentToRolloutPlan(t *testing.T) {
	root := t.TempDir()
	sessionID := "codex-plan-session"
	planText := "# Codex plan"
	writeTUIRollout(t, root, sessionID, `{"timestamp":"2026-05-21T14:44:03.645Z","type":"session_meta","payload":{"id":"codex-plan-session","timestamp":"2026-05-21T14:44:03.645Z"}}
{"timestamp":"2026-05-21T14:45:00.000Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"Plan","text":"# Codex plan"}}}
`)

	m := model{
		agents: []domain.Agent{{
			Target:    "main:1.0",
			SessionID: sessionID,
			Harness:   "codex",
		}},
		codexSessionsDir: root,
		cfg:              testConfig(""),
	}
	m.buildTree()
	selectFirstAgent(&m)

	cmd := m.loadPlan()
	if cmd == nil {
		t.Fatal("loadPlan returned nil, want plan command for codex agent without ProjDir")
	}
	msg, ok := cmd().(planMsg)
	if !ok {
		t.Fatalf("got %T, want planMsg", cmd())
	}
	if msg.content != planText {
		t.Errorf("plan content = %q, want %q", msg.content, planText)
	}
}

func writeTUIRollout(t *testing.T, root, sessionID, contents string) string {
	t.Helper()

	dir := filepath.Join(root, "2026", "05", "21")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir rollout dir: %v", err)
	}
	path := filepath.Join(dir, "rollout-2026-05-21T00-00-00-"+sessionID+".jsonl")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write rollout: %v", err)
	}
	return path
}

// --- postMergeCleanup tests ---
//
// The cleanup pipeline must:
//   - Refuse cleanly when the agent worked inside a git submodule.
//   - Pre-flight that the agent's branch actually exists in the source repo
//     (guards against stale WorktreeCwd hints pointing into a different repo).
//   - Skip `git worktree remove` when the agent is in the source repo itself
//     (Linked == false), not in a linked worktree.
//   - Skip `checkout main` / `pull origin main` when the source repo has
//     uncommitted changes, with a non-fatal status.
//   - Always run git operations against the resolved Source root, never
//     against an arbitrary cwd that might be inside a linked worktree.

// expectTopologyCalls primes a mock GitRunner with the three rev-parse calls
// the resolver issues for a single seed.
func expectTopologyCalls(m *mocks.MockGitRunner, seed, worktree, gitCommonDir, superproject string) {
	m.On("Output", mock.Anything, "git", "-C", seed, "rev-parse", "--show-toplevel").
		Return([]byte(worktree+"\n"), nil).Once()
	m.On("Output", mock.Anything, "git", "-C", seed, "rev-parse",
		"--path-format=absolute", "--git-common-dir").
		Return([]byte(gitCommonDir+"\n"), nil).Once()
	m.On("Output", mock.Anything, "git", "-C", seed, "rev-parse",
		"--show-superproject-working-tree").
		Return([]byte(superproject+"\n"), nil).Once()
}

func TestPostMergeCleanup_LinkedWorktree_HappyPath(t *testing.T) {
	const (
		cwd      = "/wt/feat/apps/web"
		worktree = "/wt/feat"
		source   = "/repo"
		branch   = "feat/wire-chart-options"
	)

	mr := mocks.NewMockGitRunner(t)
	t.Cleanup(setTestGitRunner(mr))

	expectTopologyCalls(mr, cwd, worktree, source+"/.git", "")

	mr.On("Output", mock.Anything, "git", "-C", source, "symbolic-ref",
		"refs/remotes/origin/HEAD").Return([]byte("refs/remotes/origin/main\n"), nil)

	mr.On("Output", mock.Anything, "git", "-C", source, "rev-parse",
		"--verify", "refs/heads/"+branch).Return([]byte("abc123\n"), nil)

	mr.On("RunDir", mock.Anything, "", "git", "-C", source, "diff-index",
		"--quiet", "HEAD").Return(nil)

	mr.On("CombinedOutputDir", mock.Anything, "", "git", "-C", source,
		"worktree", "remove", "--force", worktree).Return([]byte(""), nil)
	mr.On("CombinedOutputDir", mock.Anything, "", "git", "-C", source,
		"worktree", "prune").Return([]byte(""), nil)

	mr.On("CombinedOutputDir", mock.Anything, "", "git", "-C", source,
		"checkout", "main").Return([]byte(""), nil)
	mr.On("CombinedOutputDir", mock.Anything, "", "git", "-C", source,
		"pull", "origin", "main").Return([]byte(""), nil)
	mr.On("RunDir", mock.Anything, "", "git", "-C", source, "branch",
		"-d", branch).Return(nil)

	stateDir := t.TempDir()
	agent := domain.Agent{
		Cwd:        cwd,
		Branch:     branch,
		TmuxPaneID: "%99",
		SessionID:  "sess-1",
	}
	cmd := postMergeCleanup(agent, stateDir)
	if cmd == nil {
		t.Fatal("postMergeCleanup returned nil")
	}
	msg := cmd()
	res, ok := msg.(postMergeCleanupMsg)
	if !ok {
		t.Fatalf("expected postMergeCleanupMsg, got %T: %+v", msg, msg)
	}
	if res.err != nil {
		t.Fatalf("expected success, got err at %q: %v", res.progress, res.err)
	}
}

func TestPostMergeCleanup_NonWorktreeAgent_SkipsWorktreeRemove(t *testing.T) {
	const (
		cwd    = "/repo"
		source = "/repo"
		branch = "feat/x"
	)

	mr := mocks.NewMockGitRunner(t)
	t.Cleanup(setTestGitRunner(mr))

	expectTopologyCalls(mr, cwd, source, source+"/.git", "")

	mr.On("Output", mock.Anything, "git", "-C", source, "symbolic-ref",
		"refs/remotes/origin/HEAD").Return([]byte("refs/remotes/origin/main\n"), nil)
	mr.On("Output", mock.Anything, "git", "-C", source, "rev-parse",
		"--verify", "refs/heads/"+branch).Return([]byte("abc123\n"), nil)
	mr.On("RunDir", mock.Anything, "", "git", "-C", source, "diff-index",
		"--quiet", "HEAD").Return(nil)

	// NO worktree remove or prune.

	mr.On("CombinedOutputDir", mock.Anything, "", "git", "-C", source,
		"checkout", "main").Return([]byte(""), nil)
	mr.On("CombinedOutputDir", mock.Anything, "", "git", "-C", source,
		"pull", "origin", "main").Return([]byte(""), nil)
	mr.On("RunDir", mock.Anything, "", "git", "-C", source, "branch",
		"-d", branch).Return(nil)

	cmd := postMergeCleanup(domain.Agent{Cwd: cwd, Branch: branch, SessionID: "s"}, t.TempDir())
	res, _ := cmd().(postMergeCleanupMsg)
	if res.err != nil {
		t.Fatalf("expected success for non-worktree agent, got %v at %q", res.err, res.progress)
	}
}

func TestPostMergeCleanup_Submodule_Refuses(t *testing.T) {
	mr := mocks.NewMockGitRunner(t)
	t.Cleanup(setTestGitRunner(mr))

	expectTopologyCalls(mr, "/repo/sub", "/repo/sub", "/repo/.git/modules/sub", "/repo")

	cmd := postMergeCleanup(domain.Agent{Cwd: "/repo/sub", Branch: "feat/x", SessionID: "s"}, t.TempDir())
	res, _ := cmd().(postMergeCleanupMsg)
	if res.err == nil {
		t.Fatal("expected refusal on submodule, got nil err")
	}
	if !strings.Contains(res.err.Error(), "submodule") {
		t.Errorf("err = %v, want containing 'submodule'", res.err)
	}
}

func TestPostMergeCleanup_BranchMissingInSource_Refuses(t *testing.T) {
	const (
		cwd    = "/wt/feat"
		source = "/repo"
		branch = "feat/ghost"
	)

	mr := mocks.NewMockGitRunner(t)
	t.Cleanup(setTestGitRunner(mr))

	expectTopologyCalls(mr, cwd, cwd, source+"/.git", "")
	// Branch verification fires before gitDefaultBranch — only the verify
	// call is expected, and it fails to short-circuit the rest.
	mr.On("Output", mock.Anything, "git", "-C", source, "rev-parse",
		"--verify", "refs/heads/"+branch).Return(nil, errors.New("fatal: not a valid ref"))

	cmd := postMergeCleanup(domain.Agent{Cwd: cwd, Branch: branch, SessionID: "s"}, t.TempDir())
	res, _ := cmd().(postMergeCleanupMsg)
	if res.err == nil {
		t.Fatal("expected refusal on missing branch, got nil err")
	}
	if !strings.Contains(res.err.Error(), "stale") && !strings.Contains(res.err.Error(), "not found") {
		t.Errorf("err = %v, want containing 'stale' or 'not found'", res.err)
	}
}

func TestPostMergeCleanup_DirtySource_SkipsCheckoutPull(t *testing.T) {
	const (
		cwd      = "/wt/feat"
		worktree = "/wt/feat"
		source   = "/repo"
		branch   = "feat/x"
	)

	mr := mocks.NewMockGitRunner(t)
	t.Cleanup(setTestGitRunner(mr))

	expectTopologyCalls(mr, cwd, worktree, source+"/.git", "")
	mr.On("Output", mock.Anything, "git", "-C", source, "symbolic-ref",
		"refs/remotes/origin/HEAD").Return([]byte("refs/remotes/origin/main\n"), nil)
	mr.On("Output", mock.Anything, "git", "-C", source, "rev-parse",
		"--verify", "refs/heads/"+branch).Return([]byte("abc\n"), nil)
	mr.On("RunDir", mock.Anything, "", "git", "-C", source, "diff-index",
		"--quiet", "HEAD").Return(errors.New("exit status 1"))

	mr.On("CombinedOutputDir", mock.Anything, "", "git", "-C", source,
		"worktree", "remove", "--force", worktree).Return([]byte(""), nil)
	mr.On("CombinedOutputDir", mock.Anything, "", "git", "-C", source,
		"worktree", "prune").Return([]byte(""), nil)

	// NO checkout / pull.

	mr.On("RunDir", mock.Anything, "", "git", "-C", source, "branch",
		"-d", branch).Return(nil)

	cmd := postMergeCleanup(domain.Agent{Cwd: cwd, Branch: branch, SessionID: "s"}, t.TempDir())
	res, _ := cmd().(postMergeCleanupMsg)
	if res.err != nil {
		t.Fatalf("dirty source should be non-fatal, got err %v at %q", res.err, res.progress)
	}
}

func TestPostMergeCleanup_AllSeedsDead_Refuses(t *testing.T) {
	mr := mocks.NewMockGitRunner(t)
	t.Cleanup(setTestGitRunner(mr))

	mr.On("Output", mock.Anything, "git", "-C", "/dead", "rev-parse",
		"--show-toplevel").Return(nil, errors.New("fatal: not a git repo"))

	cmd := postMergeCleanup(domain.Agent{Cwd: "/dead", Branch: "x", SessionID: "s"}, t.TempDir())
	res, _ := cmd().(postMergeCleanupMsg)
	if res.err == nil {
		t.Fatal("expected error when topology cannot be resolved")
	}
}

// hasClaudeRow returns true if any row exists in daily_usage for provider='claude'.
// Tests use a non-zero CostUSD when they expect a row, so TotalCostForProvider
// is a sufficient presence proxy.
func hasClaudeRow(database *db.DB) bool {
	return database.TotalCostForProvider("claude") > 0
}

func TestPersistUsage_WritesClaudeAgent(t *testing.T) {
	database, err := db.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	defer database.Close()

	agents := []domain.Agent{{
		Target:    "%1",
		SessionID: "sess-claude",
		Harness:   "claude",
		ProjDir:   "/some/projects/-Users-foo",
	}}
	perAgent := map[string]domain.Usage{
		"%1": {InputTokens: 100, OutputTokens: 50, CostUSD: 1.50, Model: "claude-sonnet-4-6"},
	}

	_ = persistUsage(database, agents, perAgent)()

	if !hasClaudeRow(database) {
		t.Errorf("expected claude row written, found none")
	}
}

func TestPersistUsage_SkipsCodexHarness(t *testing.T) {
	database, _ := db.OpenDB(":memory:")
	defer database.Close()

	agents := []domain.Agent{{
		Target:    "%1",
		SessionID: "019e4eb3-aaa",
		Harness:   "codex",
		ProjDir:   "/some/projects",
	}}
	perAgent := map[string]domain.Usage{
		"%1": {InputTokens: 100, OutputTokens: 50, CostUSD: 1.0, Model: "claude-opus-4-7"},
	}

	_ = persistUsage(database, agents, perAgent)()

	if hasClaudeRow(database) {
		t.Errorf("codex agent wrote a claude row")
	}
}

func TestPersistUsage_SkipsEmptyProjDir(t *testing.T) {
	database, _ := db.OpenDB(":memory:")
	defer database.Close()

	agents := []domain.Agent{{
		Target:    "%1",
		SessionID: "sess-no-projdir",
		Harness:   "claude",
		ProjDir:   "",
	}}
	perAgent := map[string]domain.Usage{
		"%1": {InputTokens: 100, OutputTokens: 50, CostUSD: 1.0, Model: "claude-sonnet-4-6"},
	}

	_ = persistUsage(database, agents, perAgent)()

	if hasClaudeRow(database) {
		t.Errorf("empty ProjDir wrote a row")
	}
}

func TestPersistUsage_SkipsEmptySessionID(t *testing.T) {
	database, _ := db.OpenDB(":memory:")
	defer database.Close()

	agents := []domain.Agent{{
		Target:    "%1",
		SessionID: "",
		Harness:   "claude",
		ProjDir:   "/some/dir",
	}}
	perAgent := map[string]domain.Usage{
		"%1": {InputTokens: 100, OutputTokens: 50, CostUSD: 1.0},
	}

	_ = persistUsage(database, agents, perAgent)()

	if hasClaudeRow(database) {
		t.Errorf("empty SessionID wrote a row")
	}
}

func TestPersistUsage_SkipsZeroOutputTokens(t *testing.T) {
	database, _ := db.OpenDB(":memory:")
	defer database.Close()

	agents := []domain.Agent{{
		Target:    "%1",
		SessionID: "sess-empty",
		Harness:   "claude",
		ProjDir:   "/some/dir",
	}}
	perAgent := map[string]domain.Usage{
		"%1": {InputTokens: 100, OutputTokens: 0, CostUSD: 1.0},
	}

	_ = persistUsage(database, agents, perAgent)()

	if hasClaudeRow(database) {
		t.Errorf("zero-output wrote a row")
	}
}

func TestPersistUsage_EmptyHarnessTreatedAsClaude(t *testing.T) {
	database, _ := db.OpenDB(":memory:")
	defer database.Close()

	agents := []domain.Agent{{
		Target:    "%1",
		SessionID: "sess-legacy",
		Harness:   "",
		ProjDir:   "/some/dir",
	}}
	perAgent := map[string]domain.Usage{
		"%1": {InputTokens: 100, OutputTokens: 50, CostUSD: 1.0, Model: "claude-sonnet-4-6"},
	}

	_ = persistUsage(database, agents, perAgent)()

	if !hasClaudeRow(database) {
		t.Errorf("empty-harness should default to claude and write")
	}
}
