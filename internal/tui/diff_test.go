package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/mocks"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/stretchr/testify/mock"
)

func TestDiffMsg_Success(t *testing.T) {
	m := NewModel(testConfig("/tmp/test-state.json"), nil)
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
	m := NewModel(testConfig("/tmp/test-state.json"), nil)
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
	m := NewModel(testConfig("/tmp/test-state.json"), nil)
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
	m := NewModel(testConfig("/tmp/test-state.json"), nil)
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

	content, _ := m.diffFileTreeContent()
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
	m := NewModel(testConfig("/tmp/test-state.json"), nil)
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
	if !e.isDir || e.label != "cmd/dashboard/" || e.dirKey != "cmd/dashboard/" {
		t.Fatalf("entry 1: expected dir cmd/dashboard/ with dirKey, got %+v", e)
	}

	// Files under cmd/dashboard/ — should have matching dirKey
	e = m.diffTreeEntries[2]
	if e.isDir || e.fileIdx != 1 || e.label != "model.go" || e.dirKey != "cmd/dashboard/" {
		t.Fatalf("entry 2: expected file model.go (idx=1, dirKey=cmd/dashboard/), got %+v", e)
	}
	e = m.diffTreeEntries[3]
	if e.isDir || e.fileIdx != 2 || e.label != "view.go" {
		t.Fatalf("entry 3: expected file view.go (idx=2), got %+v", e)
	}

	// pkg/ dir header
	e = m.diffTreeEntries[4]
	if !e.isDir || e.label != "pkg/" || e.dirKey != "pkg/" {
		t.Fatalf("entry 4: expected dir pkg/ with dirKey, got %+v", e)
	}

	// File under pkg/
	e = m.diffTreeEntries[5]
	if e.isDir || e.fileIdx != 3 || e.label != "util.go" {
		t.Fatalf("entry 5: expected file util.go (idx=3), got %+v", e)
	}
}

func TestDiffCursorNavigation(t *testing.T) {
	m := NewModel(testConfig("/tmp/test-state.json"), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()

	m.diffFiles = []*gitdiff.File{
		{OldName: "README.md", NewName: "README.md"},
		{OldName: "cmd/dashboard/model.go", NewName: "cmd/dashboard/model.go"},
		{OldName: "cmd/dashboard/view.go", NewName: "cmd/dashboard/view.go"},
	}
	m.buildDiffTreeEntries()
	m.diffCursor = 0
	m.selectedDiffFile = 0

	// Visible entries: README.md(0), cmd/dashboard/(1), model.go(2), view.go(3)
	vis := m.visibleDiffEntries()
	if len(vis) != 4 {
		t.Fatalf("expected 4 visible entries, got %d", len(vis))
	}

	// Move down from README.md → lands on cmd/dashboard/ dir (cursor=1)
	m.moveDiffCursor(1)
	if m.diffCursor != 1 {
		t.Fatalf("expected cursor=1, got %d", m.diffCursor)
	}
	// selectedDiffFile should not change when landing on dir
	if m.selectedDiffFile != 0 {
		t.Fatalf("expected selectedDiffFile=0 on dir, got %d", m.selectedDiffFile)
	}

	// Move down → model.go (cursor=2, selectedDiffFile=1)
	m.moveDiffCursor(1)
	if m.diffCursor != 2 || m.selectedDiffFile != 1 {
		t.Fatalf("expected cursor=2 selected=1, got cursor=%d selected=%d", m.diffCursor, m.selectedDiffFile)
	}

	// Move down → view.go (cursor=3, selectedDiffFile=2)
	m.moveDiffCursor(1)
	if m.diffCursor != 3 || m.selectedDiffFile != 2 {
		t.Fatalf("expected cursor=3 selected=2, got cursor=%d selected=%d", m.diffCursor, m.selectedDiffFile)
	}

	// Move down past end — stays at 3
	m.moveDiffCursor(1)
	if m.diffCursor != 3 {
		t.Fatalf("expected cursor to clamp at 3, got %d", m.diffCursor)
	}

	// Move up back to dir
	m.moveDiffCursor(-1)
	m.moveDiffCursor(-1)
	if m.diffCursor != 1 {
		t.Fatalf("expected cursor=1, got %d", m.diffCursor)
	}

	// Move up past start — stays at 0
	m.moveDiffCursor(-1)
	m.moveDiffCursor(-1)
	if m.diffCursor != 0 {
		t.Fatalf("expected cursor to clamp at 0, got %d", m.diffCursor)
	}
}

func TestDiffTreeRootFilesNoHeader(t *testing.T) {
	m := NewModel(testConfig("/tmp/test-state.json"), nil)
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

func TestToggleDiffDir(t *testing.T) {
	m := NewModel(testConfig("/tmp/test-state.json"), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()

	m.diffFiles = []*gitdiff.File{
		{OldName: "README.md", NewName: "README.md"},
		{OldName: "cmd/main.go", NewName: "cmd/main.go"},
		{OldName: "cmd/util.go", NewName: "cmd/util.go"},
	}
	m.buildDiffTreeEntries()

	// Visible: README.md, cmd/, main.go, util.go = 4 entries
	vis := m.visibleDiffEntries()
	if len(vis) != 4 {
		t.Fatalf("expected 4 visible entries, got %d", len(vis))
	}

	// Move cursor to dir entry (index 1) and toggle collapse
	m.diffCursor = 1
	m.toggleDiffDir()

	// Now children of cmd/ should be hidden
	vis = m.visibleDiffEntries()
	if len(vis) != 2 {
		t.Fatalf("expected 2 visible entries after collapse, got %d", len(vis))
	}

	// Toggle again to expand
	m.diffCursor = 1
	m.toggleDiffDir()
	vis = m.visibleDiffEntries()
	if len(vis) != 4 {
		t.Fatalf("expected 4 visible entries after expand, got %d", len(vis))
	}
}

func TestDiffFilterBasic(t *testing.T) {
	m := NewModel(testConfig("/tmp/test-state.json"), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()

	m.diffFiles = []*gitdiff.File{
		{OldName: "README.md", NewName: "README.md"},
		{OldName: "cmd/main.go", NewName: "cmd/main.go"},
		{NewName: "pkg/util.go", IsNew: true},
	}
	m.buildDiffTreeEntries()

	// Filter for "main"
	m.diffFilterText = "main"
	m.applyTreeVisibility()

	vis := m.visibleDiffEntries()
	// Should show: cmd/ dir + main.go = 2 entries
	if len(vis) != 2 {
		names := make([]string, len(vis))
		for i, idx := range vis {
			names[i] = m.diffTreeEntries[idx].label
		}
		t.Fatalf("expected 2 visible entries with filter 'main', got %d: %v", len(vis), names)
	}

	// Clear filter
	m.diffFilterText = ""
	m.applyTreeVisibility()
	vis = m.visibleDiffEntries()
	if len(vis) != 5 { // README.md, cmd/, main.go, pkg/, util.go
		t.Fatalf("expected 5 visible entries without filter, got %d", len(vis))
	}
}

func TestDiffFileTreeContent_Chevrons(t *testing.T) {
	m := NewModel(testConfig("/tmp/test-state.json"), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()

	m.diffFiles = []*gitdiff.File{
		{OldName: "cmd/main.go", NewName: "cmd/main.go"},
	}
	m.diffVisible = true
	m.buildDiffTreeEntries()

	// Expanded by default — should show ▾
	content, _ := m.diffFileTreeContent()
	plain := stripANSI(content)
	if !strings.Contains(plain, "▾") {
		t.Fatalf("expected expanded chevron ▾, got:\n%s", plain)
	}

	// Collapse the dir
	m.diffCollapsedDirs["cmd/"] = true
	m.applyTreeVisibility()
	content, _ = m.diffFileTreeContent()
	plain = stripANSI(content)
	if !strings.Contains(plain, "▸") {
		t.Fatalf("expected collapsed chevron ▸, got:\n%s", plain)
	}
}

func TestDiffDirFileCount(t *testing.T) {
	m := NewModel(testConfig("/tmp/test-state.json"), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()

	m.diffFiles = []*gitdiff.File{
		{OldName: "cmd/a.go", NewName: "cmd/a.go"},
		{OldName: "cmd/b.go", NewName: "cmd/b.go"},
		{OldName: "cmd/c.go", NewName: "cmd/c.go"},
	}
	m.buildDiffTreeEntries()

	content, _ := m.diffFileTreeContent()
	plain := stripANSI(content)
	if !strings.Contains(plain, "(3)") {
		t.Fatalf("expected dir file count (3), got:\n%s", plain)
	}
}

func TestDiffSideBySideContent(t *testing.T) {
	m := NewModel(testConfig("/tmp/test-state.json"), nil)
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
	m := NewModel(testConfig("/tmp/test-state.json"), nil)
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
	m := mocks.NewMockGitRunner(t)
	orig := gitRunner
	gitRunner = m
	t.Cleanup(func() { gitRunner = orig })

	dir := "/fake/project"

	// findMergeBase tries origin/main first
	m.On("Output", mock.Anything, "git", "-C", dir, "merge-base", "HEAD", "origin/main").
		Return([]byte("abc123\n"), nil)

	// git diff against the merge-base — one tracked file changed
	trackedDiff := "diff --git a/tracked.txt b/tracked.txt\n" +
		"index 2e65efe..d5b24fa 100644\n" +
		"--- a/tracked.txt\n" +
		"+++ b/tracked.txt\n" +
		"@@ -1 +1 @@\n" +
		"-original\n" +
		"+modified\n"
	m.On("Output", mock.Anything, "git", "-C", dir, "diff", "abc123").
		Return([]byte(trackedDiff), nil)

	// git ls-files --others — two untracked files
	m.On("Output", mock.Anything, "git", "-C", dir,
		"ls-files", "--others", "--exclude-standard").
		Return([]byte("new_service.py\nnew_test.py\n"), nil)

	// git diff --no-index for each untracked file
	untrackedDiff1 := "diff --git a/dev/null b/new_service.py\n" +
		"new file mode 100644\n" +
		"--- /dev/null\n" +
		"+++ b/new_service.py\n" +
		"@@ -0,0 +1,2 @@\n" +
		"+def run():\n" +
		"+    pass\n"
	m.On("Output", mock.Anything, "git", "-C", dir,
		"diff", "--no-index", "--", "/dev/null", "new_service.py").
		Return([]byte(untrackedDiff1), nil)

	untrackedDiff2 := "diff --git a/dev/null b/new_test.py\n" +
		"new file mode 100644\n" +
		"--- /dev/null\n" +
		"+++ b/new_test.py\n" +
		"@@ -0,0 +1,2 @@\n" +
		"+def test_run():\n" +
		"+    pass\n"
	m.On("Output", mock.Anything, "git", "-C", dir,
		"diff", "--no-index", "--", "/dev/null", "new_test.py").
		Return([]byte(untrackedDiff2), nil)

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
