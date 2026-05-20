package harness_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/harness"
)

func TestResolve_Claude(t *testing.T) {
	h, err := harness.Resolve("claude", domain.AgentProfile{Command: "claude"})
	if err != nil {
		t.Fatalf("Resolve(\"claude\") returned err: %v", err)
	}
	if h.Name() != "claude" {
		t.Errorf("Resolve(\"claude\").Name() = %q, want \"claude\"", h.Name())
	}
}

func TestResolve_Codex(t *testing.T) {
	h, err := harness.Resolve("codex", domain.AgentProfile{HomeDir: "/home/u"})
	if err != nil {
		t.Fatalf("Resolve(\"codex\") returned err: %v", err)
	}
	if h.Name() != "codex" {
		t.Errorf("Resolve(\"codex\").Name() = %q, want \"codex\"", h.Name())
	}
	wantSessions := filepath.Join("/home/u", ".codex", "sessions")
	if h.SessionsDir() != wantSessions {
		t.Errorf("SessionsDir() = %q, want %q", h.SessionsDir(), wantSessions)
	}
	wantConfig := filepath.Join("/home/u", ".codex")
	if h.ConfigDir() != wantConfig {
		t.Errorf("ConfigDir() = %q, want %q", h.ConfigDir(), wantConfig)
	}
}

func TestResolve_PiReturnsUnknownHarness(t *testing.T) {
	h, err := harness.Resolve("pi", domain.AgentProfile{HomeDir: "/home/u"})
	if h != nil {
		t.Errorf("Resolve(\"pi\") returned non-nil harness %v", h)
	}
	var unkErr harness.ErrUnknownHarness
	if !errors.As(err, &unkErr) {
		t.Fatalf("Resolve(\"pi\") err = %v, want ErrUnknownHarness", err)
	}
	if unkErr.Name != "pi" {
		t.Errorf("ErrUnknownHarness.Name = %q, want \"pi\"", unkErr.Name)
	}
}

func TestResolve_UnknownReturnsErrUnknownHarness(t *testing.T) {
	h, err := harness.Resolve("not-a-real-harness", domain.AgentProfile{Command: "claude"})
	if h != nil {
		t.Errorf("Resolve(unknown) returned non-nil harness %v", h)
	}
	var unkErr harness.ErrUnknownHarness
	if !errors.As(err, &unkErr) {
		t.Fatalf("Resolve(unknown) err = %v, want ErrUnknownHarness", err)
	}
	if unkErr.Name != "not-a-real-harness" {
		t.Errorf("ErrUnknownHarness.Name = %q, want \"not-a-real-harness\"", unkErr.Name)
	}
}

func TestResolve_EmptyResolvesToClaude(t *testing.T) {
	h, err := harness.Resolve("", domain.AgentProfile{Command: "claude"})
	if err != nil {
		t.Fatalf("Resolve(\"\") returned err: %v", err)
	}
	if h.Name() != "claude" {
		t.Errorf("Resolve(\"\").Name() = %q, want \"claude\" (empty is the documented default)", h.Name())
	}
}
