package codex

import "testing"

func TestRequiresPlanMode(t *testing.T) {
	tests := []struct {
		skill string
		want  bool
	}{
		{"feature", true},
		{"fix", false},
		{"refactor", false},
		{"chore", false},
		{"", false},
		{"unknown", false},
	}
	for _, tc := range tests {
		t.Run(tc.skill, func(t *testing.T) {
			if got := RequiresPlanMode(tc.skill); got != tc.want {
				t.Errorf("RequiresPlanMode(%q) = %v, want %v", tc.skill, got, tc.want)
			}
		})
	}
}

func TestPlanModeCommand(t *testing.T) {
	if PlanModeCommand == "" {
		t.Fatal("PlanModeCommand must not be empty")
	}
	if PlanModeCommand[0] != '/' {
		t.Errorf("PlanModeCommand %q must start with '/'", PlanModeCommand)
	}
}
