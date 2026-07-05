package harness_test

import (
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/harness"
)

func TestSpawnOptsFor(t *testing.T) {
	settings := domain.Settings{
		Effort: domain.EffortSettings{Default: "max"},
		Harness: domain.HarnessSettings{
			Codex: domain.CodexHarnessSettings{
				Model:                  "gpt-5.5",
				Approval:               "on-request",
				Sandbox:                "workspace-write",
				DefaultReasoningEffort: "high",
			},
		},
	}

	tests := []struct {
		name        string
		harnessName string
		model       string
		effort      string
		want        domain.SpawnOpts
	}{
		{
			name:        "claude uses default effort",
			harnessName: "claude",
			want:        domain.SpawnOpts{DefaultEffort: "max"},
		},
		{
			name:        "claude model and effort override",
			harnessName: "claude",
			model:       "sonnet",
			effort:      "low",
			want:        domain.SpawnOpts{DefaultEffort: "max", Model: "sonnet", Effort: "low"},
		},
		{
			name:        "codex uses codex settings",
			harnessName: "codex",
			want: domain.SpawnOpts{
				DefaultEffort: "high",
				Model:         "gpt-5.5",
				Approval:      "on-request",
				Sandbox:       "workspace-write",
			},
		},
		{
			name:        "codex model override preserves other settings",
			harnessName: "codex",
			model:       "gpt-5.4",
			want: domain.SpawnOpts{
				DefaultEffort: "high",
				Model:         "gpt-5.4",
				Approval:      "on-request",
				Sandbox:       "workspace-write",
			},
		},
		{
			name:        "codex effort override is explicit",
			harnessName: "codex",
			effort:      "minimal",
			want: domain.SpawnOpts{
				DefaultEffort: "high",
				Model:         "gpt-5.5",
				Effort:        "minimal",
				Approval:      "on-request",
				Sandbox:       "workspace-write",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := harness.SpawnOptsFor(tc.harnessName, settings, tc.model, tc.effort)
			if got != tc.want {
				t.Errorf("SpawnOptsFor() = %+v, want %+v", got, tc.want)
			}
		})
	}
}
