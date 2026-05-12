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

func TestResolve_Pi(t *testing.T) {
	h, err := harness.Resolve("pi", domain.AgentProfile{HomeDir: "/home/u"})
	if err != nil {
		t.Fatalf("Resolve(\"pi\") returned err: %v", err)
	}
	if h.Name() != "pi" {
		t.Errorf("Resolve(\"pi\").Name() = %q, want \"pi\"", h.Name())
	}
	wantSessions := filepath.Join("/home/u", ".pi", "agent", "sessions")
	if h.SessionsDir() != wantSessions {
		t.Errorf("SessionsDir() = %q, want %q", h.SessionsDir(), wantSessions)
	}
	wantConfig := filepath.Join("/home/u", ".pi")
	if h.ConfigDir() != wantConfig {
		t.Errorf("ConfigDir() = %q, want %q", h.ConfigDir(), wantConfig)
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
