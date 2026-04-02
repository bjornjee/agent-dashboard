package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestSoftWrapRunes(t *testing.T) {
	tests := []struct {
		name     string
		input    []rune
		width    int
		wantRows int
		wantLast string
	}{
		{"empty", nil, 10, 1, ""},
		{"short", []rune("hi"), 10, 1, "hi"},
		{"exact", []rune("abcde"), 5, 1, "abcde"},
		{"overflow", []rune("abcdefghij"), 4, 3, "ij"},
		{"unicode", []rune("日本語テスト"), 3, 2, "テスト"},
		{"zero width", []rune("abc"), 0, 1, "abc"},
		{"width one", []rune("abc"), 1, 3, "c"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := softWrapRunes(tt.input, tt.width)
			if len(rows) != tt.wantRows {
				t.Errorf("got %d rows, want %d", len(rows), tt.wantRows)
			}
			last := string(rows[len(rows)-1])
			if last != tt.wantLast {
				t.Errorf("last row = %q, want %q", last, tt.wantLast)
			}
		})
	}
}

func TestIsSlashCommand(t *testing.T) {
	skills := []string{"(none)", "feature", "chore", "fix", "feat-fix"}

	tests := []struct {
		name    string
		runes   []rune
		start   int
		want    bool
		wantEnd int
	}{
		{"start of string", []rune("/feature rest"), 0, true, 8},
		{"after space", []rune("do /chore thing"), 3, true, 9},
		{"not in skills", []rune("/unknown"), 0, false, 0},
		{"bare slash", []rune("/"), 0, false, 0},
		{"slash space", []rune("/ nope"), 0, false, 0},
		{"mid-word no match", []rune("foo/feature"), 3, false, 0},
		{"hyphenated", []rune("/feat-fix"), 0, true, 9},
		{"none skipped", []rune("/(none)"), 0, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			end, matched := isSlashCommand(tt.runes, tt.start, skills)
			if matched != tt.want {
				t.Errorf("matched = %v, want %v", matched, tt.want)
			}
			if matched && end != tt.wantEnd {
				t.Errorf("end = %d, want %d", end, tt.wantEnd)
			}
		})
	}
}

func TestRenderWrappedInput_SlashHighlighting(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	skills := []string{"(none)", "feature", "chore"}

	// Matched skill should contain blue ANSI (themeBlue #8caaee)
	out := renderWrappedInput("/feature build it", 0, 40, false, skills)
	if !strings.Contains(out, "8caaee") && !strings.Contains(out, "38;2;140;170;238") {
		t.Errorf("expected blue ANSI for /feature, got %q", out)
	}

	// Unmatched slash — no blue
	out2 := renderWrappedInput("/unknown build", 0, 40, false, skills)
	if strings.Contains(out2, "8caaee") || strings.Contains(out2, "38;2;140;170;238") {
		t.Error("unexpected blue ANSI for unmatched /unknown")
	}

	// No slash at all
	out3 := renderWrappedInput("hello world", 0, 40, false, skills)
	if strings.Contains(out3, "8caaee") || strings.Contains(out3, "38;2;140;170;238") {
		t.Error("unexpected blue ANSI in plain text")
	}

	// Multiple slash commands
	out4 := renderWrappedInput("/feature then /chore", 0, 40, false, skills)
	// Both /feature (8 chars) and /chore (6 chars) should be styled
	// Each styled char gets its own ANSI sequence, so we should see many occurrences
	blueCount := strings.Count(out4, "38;2;140;170;238")
	if blueCount < 2 {
		t.Errorf("expected blue ANSI for both commands, got %d occurrences", blueCount)
	}
}

func TestRenderWrappedInput_Cursor(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	// Focused: cursor shows as reverse video
	out := renderWrappedInput("abc", 1, 40, true, nil)
	if !strings.Contains(out, "\x1b[7m") {
		t.Errorf("expected reverse video ANSI when focused, got %q", out)
	}

	// Not focused: no reverse video
	out2 := renderWrappedInput("abc", 1, 40, false, nil)
	if strings.Contains(out2, "\x1b[7m") {
		t.Error("unexpected reverse video when not focused")
	}

	// Cursor at end: synthetic space with reverse video
	out3 := renderWrappedInput("ab", 2, 40, true, nil)
	if !strings.Contains(out3, "\x1b[7m") {
		t.Errorf("expected reverse video for cursor at end, got %q", out3)
	}
}

func TestRenderWrappedInput_Wrapping(t *testing.T) {
	lipgloss.SetColorProfile(termenv.Ascii)

	// 20 chars at width 8 should produce 3 lines
	out := renderWrappedInput("abcdefghijklmnopqrst", 0, 8, false, nil)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Errorf("got %d lines, want 3", len(lines))
	}
}

func TestRenderWrappedInput_Placeholder(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	// Empty + focused: should show cursor block, not placeholder text
	out := renderWrappedInput("", 0, 40, true, nil)
	if !strings.Contains(out, "\x1b[7m") {
		t.Errorf("expected reverse video cursor when empty+focused, got %q", out)
	}
}

func TestRenderWrappedInput_NilSkills(t *testing.T) {
	// Should not panic
	out := renderWrappedInput("/feature test", 0, 40, true, nil)
	if out == "" {
		t.Error("expected non-empty output")
	}
}
