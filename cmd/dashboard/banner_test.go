package main

import (
	"strings"
	"testing"
	"time"
)

func TestGreeting_Morning(t *testing.T) {
	morning := time.Date(2026, 3, 29, 9, 0, 0, 0, time.Local)
	got := greeting(morning, "TestUser")
	want := "Good Morning, TestUser"
	if got != want {
		t.Fatalf("greeting(9am) = %q, want %q", got, want)
	}
}

func TestGreeting_Afternoon(t *testing.T) {
	afternoon := time.Date(2026, 3, 29, 14, 0, 0, 0, time.Local)
	got := greeting(afternoon, "TestUser")
	want := "Good Afternoon, TestUser"
	if got != want {
		t.Fatalf("greeting(2pm) = %q, want %q", got, want)
	}
}

func TestGreeting_Evening(t *testing.T) {
	evening := time.Date(2026, 3, 29, 20, 0, 0, 0, time.Local)
	got := greeting(evening, "TestUser")
	want := "Good Evening, TestUser"
	if got != want {
		t.Fatalf("greeting(8pm) = %q, want %q", got, want)
	}
}

func TestGreeting_Boundaries(t *testing.T) {
	tests := []struct {
		hour int
		want string
	}{
		{0, "Good Morning, TestUser"},
		{11, "Good Morning, TestUser"},
		{12, "Good Afternoon, TestUser"},
		{16, "Good Afternoon, TestUser"},
		{17, "Good Evening, TestUser"},
		{23, "Good Evening, TestUser"},
	}
	for _, tt := range tests {
		t.Run(time.Date(2026, 1, 1, tt.hour, 0, 0, 0, time.Local).Format("15:04"), func(t *testing.T) {
			now := time.Date(2026, 1, 1, tt.hour, 0, 0, 0, time.Local)
			got := greeting(now, "TestUser")
			if got != tt.want {
				t.Fatalf("greeting(hour=%d) = %q, want %q", tt.hour, got, tt.want)
			}
		})
	}
}

func TestRandomQuote_ReturnsFromList(t *testing.T) {
	q := fallbackQuoteText()
	found := false
	for _, candidate := range quotes {
		if len(candidate) <= maxQuoteLen && q == candidate {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("fallbackQuoteText() returned %q which is not in the quotes list", q)
	}
}

func TestRandomQuote_NotEmpty(t *testing.T) {
	q := fallbackQuoteText()
	if q == "" {
		t.Fatal("fallbackQuoteText() returned empty string")
	}
}

func TestRenderBanner_ContainsGreeting(t *testing.T) {
	cfg := testConfig("")
	cfg.Username = "TestUser"
	m := newModel(cfg, "", nil)
	m.width = 120
	m.nowFunc = func() time.Time {
		return time.Date(2026, 3, 29, 9, 0, 0, 0, time.Local)
	}
	m.quote = "Test quote"
	out := m.renderBanner()
	if !strings.Contains(out, "Good Morning, TestUser") {
		t.Fatalf("banner missing greeting, got:\n%s", out)
	}
}

func TestRenderBanner_ContainsQuote(t *testing.T) {
	m := newModel(testConfig(""), "", nil)
	m.width = 120
	m.nowFunc = func() time.Time {
		return time.Date(2026, 3, 29, 9, 0, 0, 0, time.Local)
	}
	m.quote = "Ship it!"
	out := m.renderBanner()
	if !strings.Contains(out, "Ship it!") {
		t.Fatalf("banner missing quote, got:\n%s", out)
	}
}

func TestRenderBanner_ContainsAxolotl(t *testing.T) {
	m := newModel(testConfig(""), "", nil)
	m.width = 120
	m.nowFunc = func() time.Time {
		return time.Date(2026, 3, 29, 9, 0, 0, 0, time.Local)
	}
	m.quote = "Test"
	out := m.renderBanner()
	// Half-block pixel art uses ▀, ▄, and █ characters
	hasBlocks := strings.Contains(out, "▀") || strings.Contains(out, "▄") || strings.Contains(out, "█")
	if !hasBlocks {
		t.Fatalf("banner missing axolotl pixel art (no block chars), got:\n%s", out)
	}
}

func TestFormatQuote_AuthoredFitsOneLine(t *testing.T) {
	got := formatQuote("Short quote", "Author", 40)
	if strings.Contains(got, "\n") {
		t.Fatalf("expected single line, got:\n%s", got)
	}
	if got != `" Short quote — Author "` {
		t.Fatalf("unexpected format: %q", got)
	}
}

func TestFormatQuote_AuthoredWrapsAuthorToLastLine(t *testing.T) {
	got := formatQuote("If you can't win with words then show them a good example!", "Stephen Richards", 40)
	lines := strings.Split(got, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected multiple lines, got %d:\n%s", len(lines), got)
	}
	last := lines[len(lines)-1]
	if !strings.Contains(last, "Stephen Richards") {
		t.Fatalf("expected author on last line, got:\n%s", got)
	}
	if !strings.HasPrefix(last, "    ") {
		t.Fatalf("expected last line to have padding, got: %q", last)
	}
}

func TestFormatQuote_FallbackWraps2Words(t *testing.T) {
	got := formatQuote("Be yourself; everyone else is already taken.", "", 30)
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d:\n%s", len(lines), got)
	}
	// Line 2 must have at least 2 words (not counting the closing quote mark)
	words := strings.Fields(strings.TrimSuffix(strings.TrimSpace(lines[1]), `"`))
	if len(words) < 2 {
		t.Fatalf("expected at least 2 words on line 2, got %d: %q", len(words), lines[1])
	}
}

func TestFormatQuote_FallbackFitsOneLine(t *testing.T) {
	got := formatQuote("Ship it!", "", 40)
	if strings.Contains(got, "\n") {
		t.Fatalf("expected single line, got:\n%s", got)
	}
}

func TestRenderAxolotl_CorrectHeight(t *testing.T) {
	art := renderAxolotl()
	lines := strings.Split(art, "\n")
	want := (len(axolotlPixels) + 1) / 2
	if len(lines) != want {
		t.Fatalf("renderAxolotl() has %d lines, want %d", len(lines), want)
	}
}
