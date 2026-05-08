package repo

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeRunner records the args of each call and returns canned responses
// keyed by a recognizable substring of the args.
type fakeRunner struct {
	calls []string
	// resp maps a key (substring of the join of args) to the output / error.
	resp map[string]fakeResp
}

type fakeResp struct {
	out []byte
	err error
}

func (f *fakeRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	full := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, full)
	for key, r := range f.resp {
		if strings.Contains(full, key) {
			return r.out, r.err
		}
	}
	return nil, errors.New("fakeRunner: no match for: " + full)
}

func TestResolve_SourceRepo(t *testing.T) {
	r := &fakeRunner{resp: map[string]fakeResp{
		"-C /repo rev-parse --show-toplevel":                         {out: []byte("/repo\n")},
		"-C /repo rev-parse --path-format=absolute --git-common-dir": {out: []byte("/repo/.git\n")},
		"-C /repo rev-parse --show-superproject-working-tree":        {out: []byte("\n")},
	}}
	top, err := Resolve(context.Background(), r, "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if top.Worktree != "/repo" {
		t.Errorf("Worktree = %q, want /repo", top.Worktree)
	}
	if top.Source != "/repo" {
		t.Errorf("Source = %q, want /repo", top.Source)
	}
	if top.Linked {
		t.Error("Linked = true, want false")
	}
}

func TestResolve_LinkedWorktree(t *testing.T) {
	r := &fakeRunner{resp: map[string]fakeResp{
		"-C /wt/feat rev-parse --show-toplevel":                         {out: []byte("/wt/feat\n")},
		"-C /wt/feat rev-parse --path-format=absolute --git-common-dir": {out: []byte("/repo/.git\n")},
		"-C /wt/feat rev-parse --show-superproject-working-tree":        {out: []byte("\n")},
	}}
	top, err := Resolve(context.Background(), r, "/wt/feat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if top.Worktree != "/wt/feat" {
		t.Errorf("Worktree = %q, want /wt/feat", top.Worktree)
	}
	if top.Source != "/repo" {
		t.Errorf("Source = %q, want /repo", top.Source)
	}
	if !top.Linked {
		t.Error("Linked = false, want true")
	}
}

func TestResolve_SubdirectoryOfWorktree(t *testing.T) {
	r := &fakeRunner{resp: map[string]fakeResp{
		"-C /wt/feat/apps/web rev-parse --show-toplevel":                         {out: []byte("/wt/feat\n")},
		"-C /wt/feat/apps/web rev-parse --path-format=absolute --git-common-dir": {out: []byte("/repo/.git\n")},
		"-C /wt/feat/apps/web rev-parse --show-superproject-working-tree":        {out: []byte("\n")},
	}}
	top, err := Resolve(context.Background(), r, "/wt/feat/apps/web")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if top.Worktree != "/wt/feat" {
		t.Errorf("Worktree = %q, want /wt/feat (parent worktree, not subdir)", top.Worktree)
	}
	if top.Source != "/repo" {
		t.Errorf("Source = %q, want /repo", top.Source)
	}
	if !top.Linked {
		t.Error("Linked = false, want true")
	}
}

func TestResolve_FirstSeedDeadSecondAlive(t *testing.T) {
	r := &fakeRunner{resp: map[string]fakeResp{
		"-C /dead rev-parse --show-toplevel":                         {err: errors.New("fatal: not a git repository")},
		"-C /repo rev-parse --show-toplevel":                         {out: []byte("/repo\n")},
		"-C /repo rev-parse --path-format=absolute --git-common-dir": {out: []byte("/repo/.git\n")},
		"-C /repo rev-parse --show-superproject-working-tree":        {out: []byte("\n")},
	}}
	top, err := Resolve(context.Background(), r, "/dead", "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if top.Worktree != "/repo" {
		t.Errorf("Worktree = %q, want /repo (second seed)", top.Worktree)
	}
}

func TestResolve_AllSeedsDead(t *testing.T) {
	r := &fakeRunner{resp: map[string]fakeResp{
		"rev-parse --show-toplevel": {err: errors.New("fatal: not a git repository")},
	}}
	_, err := Resolve(context.Background(), r, "/dead1", "/dead2")
	if !errors.Is(err, ErrAllSeedsDead) {
		t.Errorf("err = %v, want ErrAllSeedsDead", err)
	}
}

func TestResolve_EmptySeedsSkipped(t *testing.T) {
	r := &fakeRunner{resp: map[string]fakeResp{
		"-C /repo rev-parse --show-toplevel":                         {out: []byte("/repo\n")},
		"-C /repo rev-parse --path-format=absolute --git-common-dir": {out: []byte("/repo/.git\n")},
		"-C /repo rev-parse --show-superproject-working-tree":        {out: []byte("\n")},
	}}
	top, err := Resolve(context.Background(), r, "", "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if top.Worktree != "/repo" {
		t.Errorf("Worktree = %q, want /repo", top.Worktree)
	}
	for _, c := range r.calls {
		if strings.Contains(c, "-C  ") || strings.HasSuffix(c, "-C") {
			t.Errorf("empty seed should be skipped, but called: %q", c)
		}
	}
}

func TestResolve_NoSeeds(t *testing.T) {
	r := &fakeRunner{}
	_, err := Resolve(context.Background(), r)
	if !errors.Is(err, ErrAllSeedsDead) {
		t.Errorf("err = %v, want ErrAllSeedsDead", err)
	}
}

func TestResolve_InsideSubmodule(t *testing.T) {
	r := &fakeRunner{resp: map[string]fakeResp{
		"-C /repo/sub rev-parse --show-toplevel":                         {out: []byte("/repo/sub\n")},
		"-C /repo/sub rev-parse --path-format=absolute --git-common-dir": {out: []byte("/repo/.git/modules/sub\n")},
		"-C /repo/sub rev-parse --show-superproject-working-tree":        {out: []byte("/repo\n")},
	}}
	_, err := Resolve(context.Background(), r, "/repo/sub")
	if !errors.Is(err, ErrInsideSubmodule) {
		t.Errorf("err = %v, want ErrInsideSubmodule", err)
	}
}

func TestResolve_TrimsTrailingNewline(t *testing.T) {
	r := &fakeRunner{resp: map[string]fakeResp{
		"-C /repo rev-parse --show-toplevel":                         {out: []byte("/repo\n\n")},
		"-C /repo rev-parse --path-format=absolute --git-common-dir": {out: []byte("/repo/.git \n")},
		"-C /repo rev-parse --show-superproject-working-tree":        {out: []byte("  \n")},
	}}
	top, err := Resolve(context.Background(), r, "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if top.Worktree != "/repo" {
		t.Errorf("Worktree = %q, want /repo (whitespace trimmed)", top.Worktree)
	}
	if top.Source != "/repo" {
		t.Errorf("Source = %q, want /repo (whitespace trimmed)", top.Source)
	}
}
