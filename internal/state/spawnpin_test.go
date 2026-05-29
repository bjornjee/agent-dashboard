package state

import (
	"os"
	"path/filepath"
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

	removed := GCSpawnPins(dir, 10*time.Minute)
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
