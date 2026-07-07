package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
)

func TestSpawnPinRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := SpawnPin{
		PaneID:      "%42",
		Target:      "main:2.3",
		WorktreeCwd: "/tmp/wt/x",
		Branch:      "feat/x",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if err := WriteSpawnPin(dir, p); err != nil {
		t.Fatalf("WriteSpawnPin: %v", err)
	}

	got, ok := ReadSpawnPin(dir, "%42")
	if !ok {
		t.Fatal("ReadSpawnPin returned ok=false")
	}
	if got.PaneID != "%42" || got.Target != "main:2.3" || got.WorktreeCwd != "/tmp/wt/x" || got.Branch != "feat/x" {
		t.Errorf("round-trip mismatch: %+v", got)
	}

	DeleteSpawnPin(dir, "%42")
	if _, ok := ReadSpawnPin(dir, "%42"); ok {
		t.Error("ReadSpawnPin still returns ok after DeleteSpawnPin")
	}
}

func TestSpawnPinReadMissing(t *testing.T) {
	dir := t.TempDir()
	if _, ok := ReadSpawnPin(dir, "%nonexistent"); ok {
		t.Error("ReadSpawnPin on empty dir returned ok=true")
	}
}

func TestSpawnPinFilenameEscapesPercent(t *testing.T) {
	dir := t.TempDir()
	p := SpawnPin{PaneID: "%7", WorktreeCwd: "/x", Branch: "b", CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := WriteSpawnPin(dir, p); err != nil {
		t.Fatalf("WriteSpawnPin: %v", err)
	}
	// Verify the file lives in spawn-pins/ subdir and the `%` was escaped to `_`
	entries, err := os.ReadDir(filepath.Join(dir, "spawn-pins"))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 file, got %d", len(entries))
	}
	if name := entries[0].Name(); name != "_7.json" {
		t.Errorf("expected escaped filename _7.json, got %q", name)
	}
}

func TestStageSpawnPin_ResolvesWorktreeMatch(t *testing.T) {
	dir := t.TempDir()
	m := withMockBranchRunner(t)

	absFolder := "/tmp/worktrees/feature-x"
	// git worktree list returns the matching worktree
	m.On("Output", mock.Anything, "git", "-C", absFolder, "worktree", "list", "--porcelain").
		Return([]byte("worktree /tmp/main\nbranch refs/heads/main\n\nworktree "+absFolder+"\nbranch refs/heads/feat/x\n\n"), nil)
	// per-worktree rev-parse for absolute-git-dir (best-effort, can fail safely)
	m.On("Output", mock.Anything, "git", "-C", "/tmp/main", "rev-parse", "--absolute-git-dir").
		Return(nil, os.ErrNotExist).Maybe()
	m.On("Output", mock.Anything, "git", "-C", absFolder, "rev-parse", "--absolute-git-dir").
		Return(nil, os.ErrNotExist).Maybe()

	if err := StageSpawnPin(dir, absFolder, "%55", "main:1.3"); err != nil {
		t.Fatalf("StageSpawnPin: %v", err)
	}
	pin, ok := ReadSpawnPin(dir, "%55")
	if !ok {
		t.Fatal("ReadSpawnPin returned ok=false")
	}
	if pin.WorktreeCwd != absFolder {
		t.Errorf("WorktreeCwd = %q, want %q", pin.WorktreeCwd, absFolder)
	}
	if pin.Branch != "feat/x" {
		t.Errorf("Branch = %q, want feat/x", pin.Branch)
	}
	if pin.Target != "main:1.3" {
		t.Errorf("Target = %q, want main:1.3", pin.Target)
	}
}

func TestStageSpawnPin_NonWorktreeFolder(t *testing.T) {
	dir := t.TempDir()
	m := withMockBranchRunner(t)
	m.On("Output", mock.Anything, "git", "-C", "/tmp/not-a-worktree", "worktree", "list", "--porcelain").
		Return(nil, os.ErrNotExist)

	if err := StageSpawnPin(dir, "/tmp/not-a-worktree", "%66", "main:0.0"); err != nil {
		t.Fatalf("StageSpawnPin: %v", err)
	}
	pin, ok := ReadSpawnPin(dir, "%66")
	if !ok {
		t.Fatal("ReadSpawnPin returned ok=false")
	}
	if pin.WorktreeCwd != "" || pin.Branch != "" {
		t.Errorf("expected empty worktree/branch on non-worktree absFolder, got %+v", pin)
	}
}

func TestStageSpawnPin_EmptyPaneIDIsNoop(t *testing.T) {
	dir := t.TempDir()
	if err := StageSpawnPin(dir, "/tmp/anything", "", "main:0.0"); err != nil {
		t.Fatalf("StageSpawnPin: %v", err)
	}
	// No file should have been created
	if _, err := os.Stat(filepath.Join(dir, "spawn-pins")); err == nil {
		t.Error("spawn-pins dir should not be created when paneID is empty")
	}
}

func TestApplySpawnPins_PopulatesAndConsumes(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(AgentsDir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	// Agent on disk with empty pin
	agentFile := filepath.Join(AgentsDir(dir), "sess-a.json")
	if err := os.WriteFile(agentFile, []byte(`{"session_id":"sess-a","tmux_pane_id":"%99","state":"running"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Staged spawn-pin for that pane
	if err := WriteSpawnPin(dir, SpawnPin{
		PaneID:      "%99",
		Target:      "main:1.2",
		WorktreeCwd: "/wt/a",
		Branch:      "feat/a",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}

	sf := readState(dir)
	applySpawnPins(&sf, dir)

	got := sf.Agents["sess-a"]
	if got.WorktreeCwd != "/wt/a" {
		t.Errorf("WorktreeCwd = %q, want /wt/a", got.WorktreeCwd)
	}
	if got.Branch != "feat/a" {
		t.Errorf("Branch = %q, want feat/a", got.Branch)
	}
	// Staging file should have been consumed
	if _, ok := ReadSpawnPin(dir, "%99"); ok {
		t.Error("spawn-pin should have been deleted after consumption")
	}
	// On-disk agent file should have been stamped
	data, _ := os.ReadFile(agentFile)
	if !containsAll(string(data), `"worktree_cwd": "/wt/a"`, `"branch": "feat/a"`) {
		t.Errorf("agent file not stamped: %s", data)
	}
}

func TestApplySpawnPins_SkipsAlreadyPinned(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(AgentsDir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	// Agent already pinned to /wt/old
	agentFile := filepath.Join(AgentsDir(dir), "sess-b.json")
	if err := os.WriteFile(agentFile, []byte(`{"session_id":"sess-b","tmux_pane_id":"%88","worktree_cwd":"/wt/old","branch":"feat/old","state":"running"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Stale staging file with a different pin
	if err := WriteSpawnPin(dir, SpawnPin{
		PaneID: "%88", WorktreeCwd: "/wt/new", Branch: "feat/new",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}

	sf := readState(dir)
	applySpawnPins(&sf, dir)

	got := sf.Agents["sess-b"]
	if got.WorktreeCwd != "/wt/old" {
		t.Errorf("WorktreeCwd should not overwrite, got %q", got.WorktreeCwd)
	}
	// Stale staging file should still be deleted (the pane already has a pin)
	if _, ok := ReadSpawnPin(dir, "%88"); ok {
		t.Error("stale spawn-pin should have been GC'd")
	}
}

// containsAll reports whether s contains every substr in subs.
func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

func TestReapStaleMarker_RemovesWhenOwnerHasNoStateFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(AgentsDir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	// Build a fake "linked worktree" layout: wt/.git is a file pointing to a
	// per-worktree git-dir; the marker lives in that dir.
	wt := filepath.Join(t.TempDir(), "wt")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	perWT := filepath.Join(t.TempDir(), "main.git", "worktrees", "wt")
	if err := os.MkdirAll(perWT, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: "+perWT+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(perWT, "agent-dashboard-session")
	if err := os.WriteFile(marker, []byte("ghost-session-id"), 0o600); err != nil {
		t.Fatal(err)
	}

	// "ghost-session-id" has no agent state file → marker should be reaped.
	ReapStaleMarker(dir, wt)
	if _, err := os.Stat(marker); err == nil {
		t.Error("stale marker should have been removed")
	}
}

func TestReapStaleMarker_KeepsLiveOwner(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(AgentsDir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(t.TempDir(), "wt")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	perWT := filepath.Join(t.TempDir(), "main.git", "worktrees", "wt")
	if err := os.MkdirAll(perWT, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: "+perWT+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(perWT, "agent-dashboard-session")
	if err := os.WriteFile(marker, []byte("live-session-id"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Owner has a live state file → marker must be preserved.
	if err := os.WriteFile(filepath.Join(AgentsDir(dir), "live-session-id.json"), []byte(`{"session_id":"live-session-id"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	ReapStaleMarker(dir, wt)
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("live owner's marker should not be removed: %v", err)
	}
}

func TestReapStaleMarker_NoopOnMainWorktree(t *testing.T) {
	dir := t.TempDir()
	// Main worktree: .git is a directory, not a file.
	main := filepath.Join(t.TempDir(), "main")
	if err := os.MkdirAll(filepath.Join(main, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Should not panic / not touch anything.
	ReapStaleMarker(dir, main)
}

func TestGCSpawnPins(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	// Stale (3 hours old)
	stale := SpawnPin{PaneID: "%1", CreatedAt: now.Add(-3 * time.Hour).UTC().Format(time.RFC3339)}
	if err := WriteSpawnPin(dir, stale); err != nil {
		t.Fatal(err)
	}
	// Fresh (1 minute old)
	fresh := SpawnPin{PaneID: "%2", CreatedAt: now.Add(-1 * time.Minute).UTC().Format(time.RFC3339)}
	if err := WriteSpawnPin(dir, fresh); err != nil {
		t.Fatal(err)
	}

	removed := gcSpawnPins(dir, 10*time.Minute)
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}
	if _, ok := ReadSpawnPin(dir, "%1"); ok {
		t.Error("stale spawn-pin not removed")
	}
	if _, ok := ReadSpawnPin(dir, "%2"); !ok {
		t.Error("fresh spawn-pin should have been preserved")
	}
}
