package skills_test

import (
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/skills"
)

func TestExpandCodexShortkeys(t *testing.T) {
	validSkills := []string{"feature", "fix", "pr"}
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "expands bare skill",
			text: "$pr",
			want: "$agent-dashboard:pr",
		},
		{
			name: "preserves arguments",
			text: "$feature add shortkeys",
			want: "$agent-dashboard:feature add shortkeys",
		},
		{
			name: "expands multiple command tokens",
			text: "$fix then $pr",
			want: "$agent-dashboard:fix then $agent-dashboard:pr",
		},
		{
			name: "leaves namespaced skill unchanged",
			text: "$agent-dashboard:pr",
			want: "$agent-dashboard:pr",
		},
		{
			name: "leaves unknown token unchanged",
			text: "$custom",
			want: "$custom",
		},
		{
			name: "leaves mid-word token unchanged",
			text: "run$pr now",
			want: "run$pr now",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := skills.ExpandCodexShortkeys(tt.text, validSkills); got != tt.want {
				t.Errorf("ExpandCodexShortkeys(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}
