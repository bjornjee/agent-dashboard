package tui

import (
	"context"
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/mocks"
	"github.com/stretchr/testify/mock"
)

func TestParseNameStatus(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want []string
	}{
		{
			name: "add modify delete",
			out:  "A\tnew.go\nM\texisting.go\nD\told.go\n",
			want: []string{"+new.go", "~existing.go", "-old.go"},
		},
		{
			name: "rename",
			out:  "R100\told_name.go\tnew_name.go\n",
			want: []string{"~new_name.go"},
		},
		{
			name: "copy",
			out:  "C100\tsrc.go\tdst.go\n",
			want: []string{"~dst.go"},
		},
		{
			name: "empty output",
			out:  "",
			want: nil,
		},
		{
			name: "whitespace only",
			out:  "  \n",
			want: nil,
		},
		{
			name: "unknown status treated as modified",
			out:  "T\tfile.go\n",
			want: []string{"~file.go"},
		},
		{
			name: "empty status field skipped",
			out:  "\tfile.go\n",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseNameStatus(tt.out)
			if len(got) != len(tt.want) {
				t.Fatalf("parseNameStatus() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseNameStatus()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestLoadFilesChanged_Success(t *testing.T) {
	m := mocks.NewMockGitRunner(t)
	t.Cleanup(setTestGitRunner(m))

	dir := "/fake/worktree"

	// findMergeBase
	m.On("Output", mock.Anything, "git", "-C", dir, "merge-base", "HEAD", "origin/main").
		Return([]byte("abc123\n"), nil)

	// git diff --name-status
	m.On("Output", mock.Anything, "git", "-C", dir, "diff", "--name-status", "abc123").
		Return([]byte("A\tnew.go\nM\texisting.go\n"), nil)

	mdl := NewModel(testConfig(""), nil)
	mdl.agents = []domain.Agent{{Target: "s:1.0", State: "running", WorktreeCwd: dir}}
	mdl.buildTree()
	mdl.selected = 1 // skip group header

	cmd := mdl.loadFilesChanged()
	if cmd == nil {
		t.Fatal("loadFilesChanged returned nil")
	}

	msg := cmd()
	fc, ok := msg.(filesChangedMsg)
	if !ok {
		t.Fatalf("expected filesChangedMsg, got %T", msg)
	}

	if fc.target != "s:1.0" {
		t.Errorf("target = %q, want %q", fc.target, "s:1.0")
	}
	if len(fc.files) != 2 {
		t.Fatalf("files = %v, want 2 entries", fc.files)
	}
	if fc.files[0] != "+new.go" || fc.files[1] != "~existing.go" {
		t.Errorf("files = %v, want [+new.go ~existing.go]", fc.files)
	}
}

func TestLoadFilesChanged_NoAgent(t *testing.T) {
	mdl := NewModel(testConfig(""), nil)
	cmd := mdl.loadFilesChanged()
	if cmd != nil {
		t.Error("expected nil cmd when no agent selected")
	}
}

func TestLoadFilesChanged_GitError(t *testing.T) {
	m := mocks.NewMockGitRunner(t)
	t.Cleanup(setTestGitRunner(m))

	dir := "/fake/project"

	// findMergeBase fails for all refs
	m.On("Output", mock.Anything, "git", "-C", dir, "merge-base", mock.Anything, mock.Anything).
		Return(nil, context.DeadlineExceeded)

	// fallback diff against HEAD also fails
	m.On("Output", mock.Anything, "git", "-C", dir, "diff", "--name-status", "HEAD").
		Return(nil, context.DeadlineExceeded)

	mdl := NewModel(testConfig(""), nil)
	mdl.agents = []domain.Agent{{Target: "s:1.0", State: "running", Cwd: dir}}
	mdl.buildTree()
	mdl.selected = 1 // skip group header

	cmd := mdl.loadFilesChanged()
	if cmd == nil {
		t.Fatal("loadFilesChanged returned nil")
	}

	msg := cmd()
	fc, ok := msg.(filesChangedMsg)
	if !ok {
		t.Fatalf("expected filesChangedMsg, got %T", msg)
	}

	if fc.files != nil {
		t.Errorf("files = %v, want nil on error", fc.files)
	}
}

func TestFilesChangedMsg_NonExistentTarget(t *testing.T) {
	mdl := NewModel(testConfig(""), nil)
	mdl.agents = []domain.Agent{
		{Target: "s:1.0", State: "running", Cwd: "/a", FilesChanged: []string{"+old.go"}},
	}
	mdl.buildTree()

	// Dispatch a message for a target that doesn't exist
	updated, _ := mdl.Update(filesChangedMsg{target: "nonexistent:9.9", files: []string{"+x.go"}})
	m2 := updated.(model)

	// Original agent should be unchanged
	if len(m2.agents[0].FilesChanged) != 1 || m2.agents[0].FilesChanged[0] != "+old.go" {
		t.Errorf("agents[0].FilesChanged = %v, want [+old.go]", m2.agents[0].FilesChanged)
	}
}
