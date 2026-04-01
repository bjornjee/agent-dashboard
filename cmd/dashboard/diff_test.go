package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

func TestDiffMsg_Success(t *testing.T) {
	m := newModel("/tmp/test-state.json", "", nil)
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
	m := newModel("/tmp/test-state.json", "", nil)
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
	m := newModel("/tmp/test-state.json", "", nil)
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
	m := newModel("/tmp/test-state.json", "", nil)
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

	content := m.diffFileTreeContent()

	if !strings.Contains(content, "cmd/main.go") {
		t.Fatal("expected file tree to contain cmd/main.go")
	}
	if !strings.Contains(content, "new_file.go") {
		t.Fatal("expected file tree to contain new_file.go")
	}
	if !strings.Contains(content, "old_file.go") {
		t.Fatal("expected file tree to contain old_file.go")
	}
}

func TestDiffSideBySideContent(t *testing.T) {
	m := newModel("/tmp/test-state.json", "", nil)
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

	content := m.diffSideBySideContent()

	if content == "" {
		t.Fatal("expected non-empty side-by-side content")
	}
	if !strings.Contains(content, "old line") {
		t.Fatal("expected content to contain 'old line'")
	}
	if !strings.Contains(content, "new line") {
		t.Fatal("expected content to contain 'new line'")
	}
}
