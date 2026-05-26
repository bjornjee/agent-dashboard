package harness

import "testing"

func TestInitialPrompt(t *testing.T) {
	tests := []struct {
		name        string
		harnessName string
		skill       string
		message     string
		want        string
	}{
		{"codex plain prompt", "codex", "", "hello", "hello"},
		{"codex skill prompt", "codex", "feature", "add login", "$agent-dashboard:feature add login"},
		{"claude skill prompt", "claude", "feature", "add login", "/feature add login"},
		{"claude plain prompt", "claude", "", "hello", "hello"},
		{"empty prompt", "codex", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InitialPrompt(tt.harnessName, tt.skill, tt.message)
			if got != tt.want {
				t.Errorf("InitialPrompt(%q, %q, %q) = %q, want %q",
					tt.harnessName, tt.skill, tt.message, got, tt.want)
			}
		})
	}
}
