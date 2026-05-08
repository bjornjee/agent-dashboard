// Package repo derives git topology facts from a path inside a repo.
//
// It answers two questions, given any path in (or near) a git repo:
//
//   - Worktree root: where the working tree containing path lives.
//   - Source repo root: where the canonical .git directory lives — i.e. the
//     repo whose `main` is checked out at the source, with linked worktrees
//     pointing back to it.
//
// The package depends only on a Runner abstraction; it has no knowledge of
// agents, the TUI, or Claude Code.
package repo

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// Topology describes the git layout for a path inside a repo.
type Topology struct {
	Worktree string // git rev-parse --show-toplevel
	Source   string // dirname(rev-parse --git-common-dir)
	Linked   bool   // Worktree != Source — i.e. path is in a linked worktree
}

// Runner abstracts command execution so tests can mock git invocations.
type Runner interface {
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
}

// Sentinel errors so callers can branch on classification.
var (
	// ErrNotARepo means the seed path is not inside any git repo.
	ErrNotARepo = errors.New("path is not in a git repo")
	// ErrInsideSubmodule means the seed resolves into a git submodule.
	// Callers decide whether to walk up to the superproject or refuse.
	ErrInsideSubmodule = errors.New("path is inside a git submodule")
	// ErrAllSeedsDead means none of the supplied seed paths resolved.
	ErrAllSeedsDead = errors.New("no seed path resolves to a git repo")
)

// Resolve returns the topology of the first seed inside a git repo.
// Empty seeds are skipped. Returns ErrAllSeedsDead if none resolve.
// Returns ErrInsideSubmodule when the resolved path is in a submodule.
func Resolve(ctx context.Context, run Runner, seeds ...string) (Topology, error) {
	var lastErr error
	tried := false
	for _, seed := range seeds {
		if seed == "" {
			continue
		}
		tried = true

		top, err := resolveOne(ctx, run, seed)
		if err == nil {
			return top, nil
		}
		// Submodule is a definitive answer: don't keep trying other seeds.
		if errors.Is(err, ErrInsideSubmodule) {
			return Topology{}, err
		}
		lastErr = err
	}
	if !tried {
		return Topology{}, ErrAllSeedsDead
	}
	if errors.Is(lastErr, ErrNotARepo) {
		return Topology{}, ErrAllSeedsDead
	}
	return Topology{}, ErrAllSeedsDead
}

func resolveOne(ctx context.Context, run Runner, seed string) (Topology, error) {
	wtOut, err := run.Output(ctx, "git", "-C", seed, "rev-parse", "--show-toplevel")
	if err != nil {
		return Topology{}, fmt.Errorf("%w: %s", ErrNotARepo, seed)
	}
	worktree := strings.TrimSpace(string(wtOut))
	if worktree == "" {
		return Topology{}, fmt.Errorf("%w: empty worktree for %s", ErrNotARepo, seed)
	}

	commonOut, err := run.Output(ctx, "git", "-C", seed, "rev-parse",
		"--path-format=absolute", "--git-common-dir")
	if err != nil {
		return Topology{}, fmt.Errorf("%w: %s", ErrNotARepo, seed)
	}
	commonDir := strings.TrimSpace(string(commonOut))
	if commonDir == "" {
		return Topology{}, fmt.Errorf("%w: empty git-common-dir for %s", ErrNotARepo, seed)
	}
	source := filepath.Dir(commonDir)

	// Submodule check — runs even on success of the previous calls because a
	// submodule still has a working tree and git-common-dir; what marks it as
	// a submodule is having a non-empty superproject working tree.
	subOut, err := run.Output(ctx, "git", "-C", seed, "rev-parse", "--show-superproject-working-tree")
	if err == nil {
		if super := strings.TrimSpace(string(subOut)); super != "" {
			return Topology{}, ErrInsideSubmodule
		}
	}
	// If the superproject probe itself errors, we don't treat it as fatal —
	// older git versions may not support the flag. Default to non-submodule.

	return Topology{
		Worktree: worktree,
		Source:   source,
		Linked:   worktree != source,
	}, nil
}
