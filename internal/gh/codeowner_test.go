package gh

import (
	"context"
	"fmt"
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

	r := &fakeRunner{
		responses: map[string]string{
			"gh":  "alice\n",
			"git": "* @alice @bob\n",
		},
	}

	if !IsCodeOwner(r, "/repo") {
		t.Fatal("expected alice to be a code owner")
	}
}

func TestIsCodeOwner_NotOwner(t *testing.T) {
	t.Cleanup(resetCache)

	r := &fakeRunner{
		responses: map[string]string{
			"gh":  "charlie\n",
			"git": "* @bob\n",
		},
	}

	if IsCodeOwner(r, "/repo") {
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

	if IsCodeOwner(r, "/repo") {
		t.Fatal("expected false when gh is unavailable")
	}
}

func TestIsCodeOwner_NoCODEOWNERS(t *testing.T) {
	t.Cleanup(resetCache)

	r := &fakeRunner{
		responses: map[string]string{
			"gh": "alice\n",
		},
		errors: map[string]error{
			"git": fmt.Errorf("path not found"),
		},
	}

	if IsCodeOwner(r, "/repo") {
		t.Fatal("expected false when CODEOWNERS does not exist on default branch")
	}
}

func TestMergeArgs_CodeOwner(t *testing.T) {
	t.Cleanup(resetCache)

	r := &fakeRunner{
		responses: map[string]string{
			"gh":  "alice\n",
			"git": "* @alice\n",
		},
	}

	args := MergeArgs(r, "/repo", "feat/foo")
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

	r := &fakeRunner{
		responses: map[string]string{
			"gh":  "charlie\n",
			"git": "* @bob\n",
		},
	}

	args := MergeArgs(r, "/repo", "feat/foo")
	expected := []string{"pr", "merge", "feat/foo", "--squash"}
	if len(args) != len(expected) {
		t.Fatalf("got %v, want %v", args, expected)
	}
}
