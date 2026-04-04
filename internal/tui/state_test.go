package tui

import (
	"github.com/bjornjee/agent-dashboard/internal/domain"
	"testing"
)

func TestEffectiveDir(t *testing.T) {
	tests := []struct {
		name  string
		agent domain.Agent
		want  string
	}{
		{"worktree preferred", domain.Agent{Cwd: "/launch", WorktreeCwd: "/worktree"}, "/worktree"},
		{"cwd fallback", domain.Agent{Cwd: "/launch"}, "/launch"},
		{"both empty", domain.Agent{}, ""},
		{"worktree only", domain.Agent{WorktreeCwd: "/worktree"}, "/worktree"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.agent.EffectiveDir(); got != tt.want {
				t.Errorf("EffectiveDir() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsBlocked(t *testing.T) {
	blocked := []string{"permission"}
	for _, s := range blocked {
		if !isBlocked(s) {
			t.Errorf("expected isBlocked(%q) = true", s)
		}
	}
	notBlocked := []string{"question", "error", "running", "done", "idle_prompt", "pr", "merged", "unknown"}
	for _, s := range notBlocked {
		if isBlocked(s) {
			t.Errorf("expected isBlocked(%q) = false", s)
		}
	}
}

func TestIsWaiting(t *testing.T) {
	waiting := []string{"question", "error"}
	for _, s := range waiting {
		if !isWaiting(s) {
			t.Errorf("expected isWaiting(%q) = true", s)
		}
	}
	notWaiting := []string{"permission", "running", "done", "idle_prompt", "pr", "merged", "unknown"}
	for _, s := range notWaiting {
		if isWaiting(s) {
			t.Errorf("expected isWaiting(%q) = false", s)
		}
	}
}

func TestIsReview(t *testing.T) {
	review := []string{"done", "idle_prompt"}
	for _, s := range review {
		if !isReview(s) {
			t.Errorf("expected isReview(%q) = true", s)
		}
	}
	notReview := []string{"permission", "question", "error", "running", "pr", "merged", "unknown"}
	for _, s := range notReview {
		if isReview(s) {
			t.Errorf("expected isReview(%q) = false", s)
		}
	}
}

func TestIsPR(t *testing.T) {
	if !isPR("pr") {
		t.Error("expected isPR(\"pr\") = true")
	}
	for _, s := range []string{"done", "idle_prompt", "running", "permission", "merged", "unknown"} {
		if isPR(s) {
			t.Errorf("expected isPR(%q) = false", s)
		}
	}
}

func TestIsMerged(t *testing.T) {
	if !isMerged("merged") {
		t.Error("expected isMerged(\"merged\") = true")
	}
	for _, s := range []string{"done", "idle_prompt", "running", "permission", "unknown"} {
		if isMerged(s) {
			t.Errorf("expected isMerged(%q) = false", s)
		}
	}
}

func TestEffectiveState(t *testing.T) {
	tests := []struct {
		name  string
		agent domain.Agent
		want  string
	}{
		{"pinned overrides", domain.Agent{State: "running", PinnedState: "merged"}, "merged"},
		{"no pin uses state", domain.Agent{State: "done"}, "done"},
		{"empty pin uses state", domain.Agent{State: "idle_prompt", PinnedState: ""}, "idle_prompt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.agent.EffectiveState(); got != tt.want {
				t.Errorf("EffectiveState() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsBlocked_IncludesPlan(t *testing.T) {
	if !isBlocked("permission") {
		t.Error("permission should be blocked")
	}
	if !isBlocked("plan") {
		t.Error("plan should be blocked")
	}
	if isBlocked("running") {
		t.Error("running should not be blocked")
	}
	if isBlocked("idle_prompt") {
		t.Error("idle_prompt should not be blocked")
	}
}
