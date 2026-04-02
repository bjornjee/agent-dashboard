package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

func TestDiffMsg_Success(t *testing.T) {
	m := newModel(testConfig("/tmp/test-state.json"), "", nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()

	files := []*gitdiff.File{
		{OldName: "a.go", NewName: "a.go"},
		{OldName: "b.go", NewName: "b.go"},
	}

	result, _ := m.Update(diffMsg{files: files})
	got := result.(model)

	if !got.diffVisible {
		t.Fatal("expected diffVisible to be true")
	}
	if len(got.diffFiles) != 2 {
		t.Fatalf("expected 2 diff files, got %d", len(got.diffFiles))
	}
	if got.selectedDiffFile != 0 {
		t.Fatalf("expected selectedDiffFile=0, got %d", got.selectedDiffFile)
	}
}

func TestDiffMsg_Error(t *testing.T) {
	m := newModel(testConfig("/tmp/test-state.json"), "", nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()

	result, _ := m.Update(diffMsg{err: fmt.Errorf("git not found")})
	got := result.(model)

	if got.diffVisible {
		t.Fatal("expected diffVisible to be false on error")
	}
	if !strings.Contains(got.statusMsg, "Diff failed") {
		t.Fatalf("expected 'Diff failed' status, got %q", got.statusMsg)
	}
}

func TestDiffMsg_NoChanges(t *testing.T) {
	m := newModel(testConfig("/tmp/test-state.json"), "", nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()

	result, _ := m.Update(diffMsg{files: nil})
	got := result.(model)

	if got.diffVisible {
		t.Fatal("expected diffVisible to be false when no changes")
	}
	if got.statusMsg != "No changes" {
		t.Fatalf("expected 'No changes' status, got %q", got.statusMsg)
	}
}

func TestDiffFileTreeContent(t *testing.T) {
	m := newModel(testConfig("/tmp/test-state.json"), "", nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()

	m.diffFiles = []*gitdiff.File{
		{OldName: "cmd/main.go", NewName: "cmd/main.go"},
		{NewName: "new_file.go", IsNew: true},
		{OldName: "old_file.go", IsDelete: true},
	}
	m.selectedDiffFile = 1
	m.diffVisible = true
	m.buildDiffTreeEntries()

	content := m.diffFileTreeContent()
	plain := stripANSI(content)

	// Tree view should show dir header and basenames
	if !strings.Contains(plain, "cmd/") {
		t.Fatal("expected file tree to contain dir header cmd/")
	}
	if !strings.Contains(plain, "main.go") {
		t.Fatal("expected file tree to contain main.go")
	}
	if !strings.Contains(plain, "new_file.go") {
		t.Fatal("expected file tree to contain new_file.go")
	}
	if !strings.Contains(plain, "old_file.go") {
		t.Fatal("expected file tree to contain old_file.go")
	}
}

func TestBuildDiffTreeEntries(t *testing.T) {
	m := newModel(testConfig("/tmp/test-state.json"), "", nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()

	m.diffFiles = []*gitdiff.File{
		{OldName: "README.md", NewName: "README.md"},
		{OldName: "cmd/dashboard/model.go", NewName: "cmd/dashboard/model.go"},
		{OldName: "cmd/dashboard/view.go", NewName: "cmd/dashboard/view.go"},
		{NewName: "pkg/util.go", IsNew: true},
	}
	m.buildDiffTreeEntries()

	// Expected entries:
	// 0: file README.md (root, no dir header)
	// 1: dir  cmd/dashboard/
	// 2: file model.go (fileIdx=1)
	// 3: file view.go  (fileIdx=2)
	// 4: dir  pkg/
	// 5: file util.go  (fileIdx=3)

	if len(m.diffTreeEntries) != 6 {
		t.Fatalf("expected 6 tree entries, got %d: %+v", len(m.diffTreeEntries), m.diffTreeEntries)
	}

	// Root file — no dir header
	e := m.diffTreeEntries[0]
	if e.isDir || e.fileIdx != 0 || e.label != "README.md" {
		t.Fatalf("entry 0: expected root file README.md, got %+v", e)
	}

	// cmd/dashboard/ dir header
	e = m.diffTreeEntries[1]
	if !e.isDir || e.label != "cmd/dashboard/" {
		t.Fatalf("entry 1: expected dir cmd/dashboard/, got %+v", e)
	}

	// Files under cmd/dashboard/
	e = m.diffTreeEntries[2]
	if e.isDir || e.fileIdx != 1 || e.label != "model.go" {
		t.Fatalf("entry 2: expected file model.go (idx=1), got %+v", e)
	}
	e = m.diffTreeEntries[3]
	if e.isDir || e.fileIdx != 2 || e.label != "view.go" {
		t.Fatalf("entry 3: expected file view.go (idx=2), got %+v", e)
	}

	// pkg/ dir header
	e = m.diffTreeEntries[4]
	if !e.isDir || e.label != "pkg/" {
		t.Fatalf("entry 4: expected dir pkg/, got %+v", e)
	}

	// File under pkg/
	e = m.diffTreeEntries[5]
	if e.isDir || e.fileIdx != 3 || e.label != "util.go" {
		t.Fatalf("entry 5: expected file util.go (idx=3), got %+v", e)
	}
}

func TestDiffFileNavigation(t *testing.T) {
	m := newModel(testConfig("/tmp/test-state.json"), "", nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()

	m.diffFiles = []*gitdiff.File{
		{OldName: "README.md", NewName: "README.md"},
		{OldName: "cmd/dashboard/model.go", NewName: "cmd/dashboard/model.go"},
		{OldName: "cmd/dashboard/view.go", NewName: "cmd/dashboard/view.go"},
	}
	m.buildDiffTreeEntries()
	m.selectedDiffFile = 0

	// Next from first file should skip dir header and go to model.go (idx=1)
	next, ok := m.nextDiffFile()
	if !ok || next != 1 {
		t.Fatalf("expected next=1, got next=%d ok=%v", next, ok)
	}

	// Set to model.go, next should be view.go (idx=2)
	m.selectedDiffFile = 1
	next, ok = m.nextDiffFile()
	if !ok || next != 2 {
		t.Fatalf("expected next=2, got next=%d ok=%v", next, ok)
	}

	// From view.go, no next
	m.selectedDiffFile = 2
	_, ok = m.nextDiffFile()
	if ok {
		t.Fatal("expected no next from last file")
	}

	// Prev from view.go should be model.go
	prev, ok := m.prevDiffFile()
	if !ok || prev != 1 {
		t.Fatalf("expected prev=1, got prev=%d ok=%v", prev, ok)
	}

	// Prev from model.go should skip dir header and go to README.md
	m.selectedDiffFile = 1
	prev, ok = m.prevDiffFile()
	if !ok || prev != 0 {
		t.Fatalf("expected prev=0, got prev=%d ok=%v", prev, ok)
	}

	// No prev from first file
	m.selectedDiffFile = 0
	_, ok = m.prevDiffFile()
	if ok {
		t.Fatal("expected no prev from first file")
	}
}

func TestDiffTreeRootFilesNoHeader(t *testing.T) {
	m := newModel(testConfig("/tmp/test-state.json"), "", nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()

	m.diffFiles = []*gitdiff.File{
		{OldName: "main.go", NewName: "main.go"},
		{NewName: "util.go", IsNew: true},
	}
	m.buildDiffTreeEntries()

	// Should be 2 entries, no dir headers
	if len(m.diffTreeEntries) != 2 {
		t.Fatalf("expected 2 entries for root files, got %d: %+v", len(m.diffTreeEntries), m.diffTreeEntries)
	}
	for _, e := range m.diffTreeEntries {
		if e.isDir {
			t.Fatalf("unexpected dir header for root files: %+v", e)
		}
	}
}

func TestDiffSideBySideContent(t *testing.T) {
	m := newModel(testConfig("/tmp/test-state.json"), "", nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()

	m.diffFiles = []*gitdiff.File{
		{
			OldName: "test.go",
			NewName: "test.go",
			TextFragments: []*gitdiff.TextFragment{
				{
					OldPosition: 1,
					NewPosition: 1,
					Lines: []gitdiff.Line{
						{Op: gitdiff.OpContext, Line: "package main\n"},
						{Op: gitdiff.OpDelete, Line: "old line\n"},
						{Op: gitdiff.OpAdd, Line: "new line\n"},
						{Op: gitdiff.OpContext, Line: "unchanged\n"},
					},
				},
			},
		},
	}
	m.selectedDiffFile = 0
	m.diffVisible = true

	content, _ := m.diffSideBySideContent()
	plain := stripANSI(content)

	if content == "" {
		t.Fatal("expected non-empty side-by-side content")
	}
	if !strings.Contains(plain, "old line") {
		t.Fatalf("expected content to contain 'old line', got:\n%s", plain)
	}
	if !strings.Contains(plain, "new line") {
		t.Fatalf("expected content to contain 'new line', got:\n%s", plain)
	}
}

func TestSyntaxHighlightLine(t *testing.T) {
	lexer := getLexerForFile("test.go")
	if lexer == nil {
		t.Fatal("expected lexer for .go file")
	}

	highlighted := syntaxHighlightLine(lexer, "func main() {")
	if highlighted == "func main() {" {
		t.Fatal("expected highlighted output to differ from plain text")
	}
	if !strings.Contains(highlighted, "\x1b[") {
		t.Fatal("expected ANSI escape codes in highlighted output")
	}
}

func TestApplyDiffBackground(t *testing.T) {
	result := applyDiffBackground("hello", 40, 56, 40, 20)
	if !strings.Contains(result, "\x1b[48;2;40;56;40m") {
		t.Fatal("expected background ANSI code in result")
	}
	if !strings.Contains(result, "hello") {
		t.Fatal("expected original content preserved")
	}
}

func TestCollapsibleContextBlocks(t *testing.T) {
	m := newModel(testConfig("/tmp/test-state.json"), "", nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()

	// Create 10 context lines to trigger collapsing
	var lines []gitdiff.Line
	for i := 0; i < 10; i++ {
		lines = append(lines, gitdiff.Line{Op: gitdiff.OpContext, Line: fmt.Sprintf("context line %d\n", i)})
	}

	m.diffFiles = []*gitdiff.File{
		{
			OldName: "test.go",
			NewName: "test.go",
			TextFragments: []*gitdiff.TextFragment{
				{
					OldPosition: 1,
					NewPosition: 1,
					Lines:       lines,
				},
			},
		},
	}
	m.selectedDiffFile = 0
	m.diffVisible = true
	m.diffExpandedAll = false

	collapsed, _ := m.diffSideBySideContent()
	collapsedPlain := stripANSI(collapsed)
	if !strings.Contains(collapsedPlain, "lines hidden") {
		t.Fatalf("expected collapsed placeholder, got:\n%s", collapsedPlain)
	}

	// Now expand
	m.diffExpandedAll = true
	expanded, _ := m.diffSideBySideContent()
	expandedPlain := stripANSI(expanded)
	if strings.Contains(expandedPlain, "lines hidden") {
		t.Fatal("expected no collapsed placeholder when expanded")
	}
}

func TestLoadDiff_IncludesUntrackedFiles(t *testing.T) {
	// Set up a temporary git repo with a tracked change and an untracked file.
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("command %v failed: %v\n%s", args, err, out)
		}
	}

	// Init repo with initial commit
	run("git", "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("original\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("git", "add", "tracked.txt")
	run("git", "commit", "-m", "initial")

	// Create a feature branch with a tracked modification + an untracked new file
	run("git", "checkout", "-b", "feature")
	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("modified\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("git", "add", "tracked.txt")
	run("git", "commit", "-m", "modify tracked")

	// Create untracked files (simulates new files created by an agent)
	if err := os.WriteFile(filepath.Join(dir, "new_service.py"), []byte("def run():\n    pass\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new_test.py"), []byte("def test_run():\n    pass\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	files, err := loadDiff(ctx, dir)
	if err != nil {
		t.Fatalf("loadDiff failed: %v", err)
	}

	// Should have 3 files: 1 tracked change + 2 untracked new files
	if len(files) != 3 {
		names := make([]string, len(files))
		for i, f := range files {
			names[i] = f.NewName
		}
		t.Fatalf("expected 3 diff files (1 tracked + 2 untracked), got %d: %v", len(files), names)
	}

	// Verify we can find the untracked files
	found := map[string]bool{}
	for _, f := range files {
		found[filepath.Base(f.NewName)] = true
	}
	for _, want := range []string{"tracked.txt", "new_service.py", "new_test.py"} {
		if !found[want] {
			t.Errorf("expected diff to include %s", want)
		}
	}
}
