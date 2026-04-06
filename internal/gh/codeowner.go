// Package gh provides GitHub CLI helpers shared across TUI and web packages.
package gh

import (
	"context"
	"os"
	"path/filepath"
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
// CODEOWNERS file for the repository containing dir.
func IsCodeOwner(r Runner, dir string) bool {
	user := ghUser(r)
	if user == "" {
		return false
	}

	root := repoRoot(r, dir)
	if root == "" {
		return false
	}

	data, err := os.ReadFile(filepath.Join(root, ".github", "CODEOWNERS"))
	if err != nil {
		return false
	}

	for _, line := range strings.Split(string(data), "\n") {
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

// repoRoot returns the top-level directory of the git repository containing dir.
func repoRoot(r Runner, dir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := r.Output(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
