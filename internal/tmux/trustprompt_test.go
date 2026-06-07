package tmux

import "testing"

func TestContainsTrustPrompt_Positive(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
	}{
		{"claude exact", []string{"Yes, I trust this folder"}},
		{"claude surrounded", []string{"", "  Yes, I trust this folder  ", ""}},
		{"claude mixed", []string{"Claude Code", "Yes, I trust this folder", "Yes / No"}},
		{"codex exact", []string{"Do you trust the contents of this directory?"}},
		{"codex mixed", []string{"OpenAI Codex (v0.130)", "Do you trust the contents of this directory? Working with untrusted contents...", "  Yes, continue", "  No, quit"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !ContainsTrustPrompt(tt.lines) {
				t.Errorf("expected trust prompt to be detected in %v", tt.lines)
			}
		})
	}
}

func TestContainsTrustPrompt_Negative(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
	}{
		{"empty", nil},
		{"no match", []string{"Hello world", "Running..."}},
		{"partial", []string{"I trust this"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if ContainsTrustPrompt(tt.lines) {
				t.Errorf("did not expect trust prompt to be detected in %v", tt.lines)
			}
		})
	}
}
