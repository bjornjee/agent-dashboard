// Package gh provides GitHub CLI helpers shared across TUI and web packages.
package gh

import (
	"context"
	"strings"
	"sync"
	"time"
)

// Runner abstracts command execution so callers can inject their own runner.
// Both tui.GitRunner and web.CommandRunner satisfy this interface.
type Runner interface {
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
}

var (
	cachedUser string
	userOnce   sync.Once
)

// MergeArgs returns the arguments for `gh pr merge` including --admin
// if the authenticated user is a code owner of the repository at dir.
func MergeArgs(r Runner, dir, branch string) []string {
	args := []string{"pr", "merge", branch, "--squash"}
	if IsCodeOwner(r, dir) {
		args = append(args, "--admin")
	}
	return args
}

// IsCodeOwner returns true if the authenticated gh user appears in the
// CODEOWNERS file on the default branch (origin/main).
// Reading from origin/main prevents spoofing via a modified CODEOWNERS
// on a feature branch.
func IsCodeOwner(r Runner, dir string) bool {
	user := ghUser(r)
	if user == "" {
		return false
	}

	data := codeownersFromDefault(r, dir)
	if data == "" {
		return false
	}

	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, "@"+user) {
			return true
		}
	}
	return false
}

// codeownersFromDefault reads .github/CODEOWNERS from origin/main via
// `git show`, so it cannot be spoofed by local working-tree changes.
func codeownersFromDefault(r Runner, dir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := r.Output(ctx, "git", "-C", dir, "show", "origin/main:.github/CODEOWNERS")
	if err != nil {
		return ""
	}
	return string(out)
}

// ghUser returns the authenticated GitHub username, cached after first call.
func ghUser(r Runner) string {
	userOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		out, err := r.Output(ctx, "gh", "api", "user", "-q", ".login")
		if err != nil {
			return
		}
		cachedUser = strings.TrimSpace(string(out))
	})
	return cachedUser
}
