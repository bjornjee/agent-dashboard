package harness_test

import (
	"path/filepath"
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/harness"
)

func TestResolve_Claude(t *testing.T) {
	h := harness.Resolve("claude", domain.AgentProfile{Command: "claude"})
	if h.Name() != "claude" {
		t.Errorf("Resolve(\"claude\").Name() = %q, want \"claude\"", h.Name())
	}
}

func TestResolve_Pi(t *testing.T) {
	h := harness.Resolve("pi", domain.AgentProfile{HomeDir: "/home/u"})
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

func TestResolve_UnknownFallsBackToClaude(t *testing.T) {
	h := harness.Resolve("not-a-real-harness", domain.AgentProfile{Command: "claude"})
	if h.Name() != "claude" {
		t.Errorf("Resolve(unknown).Name() = %q, want \"claude\" (fallback)", h.Name())
	}
}

func TestResolve_EmptyFallsBackToClaude(t *testing.T) {
	h := harness.Resolve("", domain.AgentProfile{Command: "claude"})
	if h.Name() != "claude" {
		t.Errorf("Resolve(\"\").Name() = %q, want \"claude\" (fallback)", h.Name())
	}
}
