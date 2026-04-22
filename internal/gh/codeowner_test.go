package gh

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// fakeRunner returns canned responses keyed by the binary name ("gh" or "git").
// For finer-grained matching, argResponses maps a substring found anywhere in
// the joined args to a response (checked before the name-level map).
type fakeRunner struct {
	responses    map[string]string
	errors       map[string]error
	argResponses map[string]string // key = substring to match in joined args
	argErrors    map[string]error
}

func (f *fakeRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	joined := name + " " + strings.Join(args, " ")
	for substr, err := range f.argErrors {
		if strings.Contains(joined, substr) {
			return nil, err
		}
	}
	for substr, resp := range f.argResponses {
		if strings.Contains(joined, substr) {
			return []byte(resp), nil
		}
	}
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

func TestIsCodeOwner_RootCODEOWNERS(t *testing.T) {
	t.Cleanup(resetCache)

	// CODEOWNERS at repo root instead of .github/CODEOWNERS
	r := &fakeRunner{
		responses: map[string]string{
			"gh": "alice\n",
		},
		argResponses: map[string]string{
			"CODEOWNERS": "* @alice\n",
		},
		argErrors: map[string]error{
			".github/CODEOWNERS": fmt.Errorf("path not found"),
		},
	}

	if !IsCodeOwner(r, "/repo") {
		t.Fatal("expected alice to be a code owner via root CODEOWNERS")
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
