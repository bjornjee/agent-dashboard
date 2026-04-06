package gh

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// fakeRunner returns canned responses keyed by the binary name ("gh" or "git").
type fakeRunner struct {
	responses map[string]string
	errors    map[string]error
}

func (f *fakeRunner) Output(_ context.Context, name string, _ ...string) ([]byte, error) {
	if err, ok := f.errors[name]; ok {
		return nil, err
	}
	if resp, ok := f.responses[name]; ok {
		return []byte(resp), nil
	}
	return nil, fmt.Errorf("unexpected command: %s", name)
}

func resetCache() {
	cachedUser = ""
	userOnce = sync.Once{}
}

func TestIsCodeOwner(t *testing.T) {
	t.Cleanup(resetCache)

	// Set up a temp repo structure with CODEOWNERS.
	dir := t.TempDir()
	ghDir := filepath.Join(dir, ".github")
	os.MkdirAll(ghDir, 0o755)
	os.WriteFile(filepath.Join(ghDir, "CODEOWNERS"), []byte("* @alice @bob\n"), 0o644)

	r := &fakeRunner{
		responses: map[string]string{
			"gh":        "alice\n",
			"git":  dir + "\n",
		},
	}

	if !IsCodeOwner(r, dir) {
		t.Fatal("expected alice to be a code owner")
	}
}

func TestIsCodeOwner_NotOwner(t *testing.T) {
	t.Cleanup(resetCache)

	dir := t.TempDir()
	ghDir := filepath.Join(dir, ".github")
	os.MkdirAll(ghDir, 0o755)
	os.WriteFile(filepath.Join(ghDir, "CODEOWNERS"), []byte("* @bob\n"), 0o644)

	r := &fakeRunner{
		responses: map[string]string{
			"gh":        "charlie\n",
			"git":  dir + "\n",
		},
	}

	if IsCodeOwner(r, dir) {
		t.Fatal("expected charlie NOT to be a code owner")
	}
}

func TestIsCodeOwner_NoGH(t *testing.T) {
	t.Cleanup(resetCache)

	r := &fakeRunner{
		errors: map[string]error{
			"gh": fmt.Errorf("gh not installed"),
		},
	}

	if IsCodeOwner(r, "/nonexistent") {
		t.Fatal("expected false when gh is unavailable")
	}
}

func TestMergeArgs_CodeOwner(t *testing.T) {
	t.Cleanup(resetCache)

	dir := t.TempDir()
	ghDir := filepath.Join(dir, ".github")
	os.MkdirAll(ghDir, 0o755)
	os.WriteFile(filepath.Join(ghDir, "CODEOWNERS"), []byte("* @alice\n"), 0o644)

	r := &fakeRunner{
		responses: map[string]string{
			"gh":        "alice\n",
			"git":  dir + "\n",
		},
	}

	args := MergeArgs(r, dir, "feat/foo")
	expected := []string{"pr", "merge", "feat/foo", "--squash", "--admin"}
	if len(args) != len(expected) {
		t.Fatalf("got %v, want %v", args, expected)
	}
	for i := range expected {
		if args[i] != expected[i] {
			t.Fatalf("args[%d] = %q, want %q", i, args[i], expected[i])
		}
	}
}

func TestMergeArgs_NotCodeOwner(t *testing.T) {
	t.Cleanup(resetCache)

	dir := t.TempDir()
	ghDir := filepath.Join(dir, ".github")
	os.MkdirAll(ghDir, 0o755)
	os.WriteFile(filepath.Join(ghDir, "CODEOWNERS"), []byte("* @bob\n"), 0o644)

	r := &fakeRunner{
		responses: map[string]string{
			"gh":        "charlie\n",
			"git":  dir + "\n",
		},
	}

	args := MergeArgs(r, dir, "feat/foo")
	expected := []string{"pr", "merge", "feat/foo", "--squash"}
	if len(args) != len(expected) {
		t.Fatalf("got %v, want %v", args, expected)
	}
}
