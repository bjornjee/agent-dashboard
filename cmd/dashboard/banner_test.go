package main

import (
	"strings"
	"testing"
	"time"
)

func TestGreeting_Morning(t *testing.T) {
	morning := time.Date(2026, 3, 29, 9, 0, 0, 0, time.Local)
	got := greeting(morning)
	want := "Good Morning, Bjorn"
	if got != want {
		t.Fatalf("greeting(9am) = %q, want %q", got, want)
	}
}

func TestGreeting_Afternoon(t *testing.T) {
	afternoon := time.Date(2026, 3, 29, 14, 0, 0, 0, time.Local)
	got := greeting(afternoon)
	want := "Good Afternoon, Bjorn"
	if got != want {
		t.Fatalf("greeting(2pm) = %q, want %q", got, want)
	}
}

func TestGreeting_Evening(t *testing.T) {
	evening := time.Date(2026, 3, 29, 20, 0, 0, 0, time.Local)
	got := greeting(evening)
	want := "Good Evening, Bjorn"
	if got != want {
		t.Fatalf("greeting(8pm) = %q, want %q", got, want)
	}
}

func TestGreeting_Boundaries(t *testing.T) {
	tests := []struct {
		hour int
		want string
	}{
		{0, "Good Morning, Bjorn"},
		{11, "Good Morning, Bjorn"},
		{12, "Good Afternoon, Bjorn"},
		{16, "Good Afternoon, Bjorn"},
		{17, "Good Evening, Bjorn"},
		{23, "Good Evening, Bjorn"},
	}
	for _, tt := range tests {
		t.Run(time.Date(2026, 1, 1, tt.hour, 0, 0, 0, time.Local).Format("15:04"), func(t *testing.T) {
			now := time.Date(2026, 1, 1, tt.hour, 0, 0, 0, time.Local)
			got := greeting(now)
			if got != tt.want {
				t.Fatalf("greeting(hour=%d) = %q, want %q", tt.hour, got, tt.want)
			}
		})
	}
}

func TestRandomQuote_ReturnsFromList(t *testing.T) {
	q := fallbackQuote()
	found := false
	for _, candidate := range quotes {
		if len(candidate) <= maxQuoteLen && q == "\" "+candidate+" \" " {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("fallbackQuote() returned %q which is not in the quotes list", q)
	}
}

func TestRandomQuote_NotEmpty(t *testing.T) {
	q := fallbackQuote()
	if q == "" {
		t.Fatal("fallbackQuote() returned empty string")
	}
}

func TestRenderBanner_ContainsGreeting(t *testing.T) {
	m := newModel("", "", nil)
	m.width = 120
	m.nowFunc = func() time.Time {
		return time.Date(2026, 3, 29, 9, 0, 0, 0, time.Local)
	}
	m.quote = "Test quote"
	out := m.renderBanner()
	if !strings.Contains(out, "Good Morning, Bjorn") {
		t.Fatalf("banner missing greeting, got:\n%s", out)
	}
}

func TestRenderBanner_ContainsQuote(t *testing.T) {
	m := newModel("", "", nil)
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
	m := newModel("", "", nil)
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

func TestRenderAxolotl_CorrectHeight(t *testing.T) {
	art := renderAxolotl()
	lines := strings.Split(art, "\n")
	want := (len(axolotlPixels) + 1) / 2
	if len(lines) != want {
		t.Fatalf("renderAxolotl() has %d lines, want %d", len(lines), want)
	}
}
