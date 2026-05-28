package git

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/mocks"
	"github.com/stretchr/testify/mock"
)

func TestListWorktrees(t *testing.T) {
	tests := []struct {
		name      string
		porcelain string
		gitDirs   map[string]string
		want      []Worktree
		wantErr   bool
	}{
		{
			name: "main only",
			porcelain: "worktree /repo\n" +
				"HEAD abc123\n" +
				"branch refs/heads/main\n",
			gitDirs: map[string]string{"/repo": "/repo/.git"},
			want: []Worktree{
				{Path: "/repo", Branch: "main", GitDir: "/repo/.git"},
			},
		},
		{
			name: "main + linked worktree",
			porcelain: "worktree /repo\n" +
				"HEAD abc123\n" +
				"branch refs/heads/main\n" +
				"\n" +
				"worktree /repo/../wt/foo\n" +
				"HEAD def456\n" +
				"branch refs/heads/feat/foo\n",
			gitDirs: map[string]string{
				"/repo":           "/repo/.git",
				"/repo/../wt/foo": "/repo/.git/worktrees/foo",
			},
			want: []Worktree{
				{Path: "/repo", Branch: "main", GitDir: "/repo/.git"},
				{Path: "/repo/../wt/foo", Branch: "feat/foo", GitDir: "/repo/.git/worktrees/foo"},
			},
		},
		{
			name: "detached HEAD",
			porcelain: "worktree /repo\n" +
				"HEAD abc123\n" +
				"branch refs/heads/main\n" +
				"\n" +
				"worktree /repo/../wt/bar\n" +
				"HEAD def456\n" +
				"detached\n",
			gitDirs: map[string]string{
				"/repo":           "/repo/.git",
				"/repo/../wt/bar": "/repo/.git/worktrees/bar",
			},
			want: []Worktree{
				{Path: "/repo", Branch: "main", GitDir: "/repo/.git"},
				{Path: "/repo/../wt/bar", Branch: "", GitDir: "/repo/.git/worktrees/bar"},
			},
		},
		{
			name: "locked + prunable flags ignored, branch/path captured",
			porcelain: "worktree /repo\n" +
				"HEAD abc123\n" +
				"branch refs/heads/main\n" +
				"\n" +
				"worktree /repo/../wt/baz\n" +
				"HEAD def456\n" +
				"branch refs/heads/feat/baz\n" +
				"locked\n" +
				"prunable gitdir file points to non-existent location\n",
			gitDirs: map[string]string{
				"/repo":           "/repo/.git",
				"/repo/../wt/baz": "/repo/.git/worktrees/baz",
			},
			want: []Worktree{
				{Path: "/repo", Branch: "main", GitDir: "/repo/.git"},
				{Path: "/repo/../wt/baz", Branch: "feat/baz", GitDir: "/repo/.git/worktrees/baz"},
			},
		},
		{
			name: "blank line at EOF tolerated",
			porcelain: "worktree /repo\n" +
				"HEAD abc123\n" +
				"branch refs/heads/main\n" +
				"\n",
			gitDirs: map[string]string{"/repo": "/repo/.git"},
			want: []Worktree{
				{Path: "/repo", Branch: "main", GitDir: "/repo/.git"},
			},
		},
		{
			name: "non-branch ref kept verbatim",
			porcelain: "worktree /repo\n" +
				"HEAD abc123\n" +
				"branch refs/tags/v1\n",
			gitDirs: map[string]string{"/repo": "/repo/.git"},
			want: []Worktree{
				{Path: "/repo", Branch: "refs/tags/v1", GitDir: "/repo/.git"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runner := mocks.NewMockBranchRunner(t)
			runner.On("Output", mock.Anything, "git", "-C", "/repo", "worktree", "list", "--porcelain").
				Return([]byte(tc.porcelain), nil).Once()
			for _, wt := range tc.want {
				gd := tc.gitDirs[wt.Path]
				runner.On("Output", mock.Anything, "git", "-C", wt.Path, "rev-parse", "--absolute-git-dir").
					Return([]byte(gd+"\n"), nil).Once()
			}

			got, err := ListWorktrees(context.Background(), runner, "/repo")
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %#v\nwant %#v", got, tc.want)
			}
		})
	}
}

func TestListWorktrees_PorcelainError(t *testing.T) {
	runner := mocks.NewMockBranchRunner(t)
	runner.On("Output", mock.Anything, "git", "-C", "/repo", "worktree", "list", "--porcelain").
		Return(nil, errors.New("not a git repo")).Once()

	_, err := ListWorktrees(context.Background(), runner, "/repo")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListWorktrees_PerWorktreeGitDirErrorSkipsThatWorktree(t *testing.T) {
	runner := mocks.NewMockBranchRunner(t)
	runner.On("Output", mock.Anything, "git", "-C", "/repo", "worktree", "list", "--porcelain").
		Return([]byte("worktree /repo\nHEAD abc\nbranch refs/heads/main\n\nworktree /wt\nHEAD def\nbranch refs/heads/feat\n"), nil).Once()
	runner.On("Output", mock.Anything, "git", "-C", "/repo", "rev-parse", "--absolute-git-dir").
		Return([]byte("/repo/.git\n"), nil).Once()
	runner.On("Output", mock.Anything, "git", "-C", "/wt", "rev-parse", "--absolute-git-dir").
		Return(nil, errors.New("missing")).Once()

	got, err := ListWorktrees(context.Background(), runner, "/repo")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := []Worktree{
		{Path: "/repo", Branch: "main", GitDir: "/repo/.git"},
		{Path: "/wt", Branch: "feat", GitDir: ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v\nwant %#v", got, want)
	}
}
