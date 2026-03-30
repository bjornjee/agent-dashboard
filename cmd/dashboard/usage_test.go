package main

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestLookupPricing(t *testing.T) {
	tests := []struct {
		model string
		want  string // "opus", "sonnet", "haiku"
	}{
		{"claude-opus-4-6", "opus"},
		{"claude-sonnet-4-6", "sonnet"},
		{"claude-haiku-4-5-20251001", "haiku"},
		{"unknown-model", "sonnet"}, // default
		{"", "sonnet"},              // empty
	}

	for _, tt := range tests {
		p := lookupPricing(tt.model)
		expected := pricingTable[tt.want]
		if p.Input != expected.Input || p.Output != expected.Output {
			t.Errorf("lookupPricing(%q): got input=%.2f output=%.2f, want input=%.2f output=%.2f",
				tt.model, p.Input, p.Output, expected.Input, expected.Output)
		}
	}
}

func TestReadUsage_ParsesTokens(t *testing.T) {
	tmp := t.TempDir()
	data := `{"type":"assistant","message":{"model":"claude-opus-4-6","role":"assistant","content":[],"usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":1000,"cache_creation_input_tokens":200}},"timestamp":"2026-03-28T10:00:00Z"}
{"type":"assistant","message":{"model":"claude-opus-4-6","role":"assistant","content":[],"usage":{"input_tokens":200,"output_tokens":100,"cache_read_input_tokens":2000,"cache_creation_input_tokens":300}},"timestamp":"2026-03-28T10:01:00Z"}
{"type":"user","message":{"role":"user","content":"hello"},"timestamp":"2026-03-28T10:00:30Z"}
`
	if err := os.WriteFile(filepath.Join(tmp, "test-session.jsonl"), []byte(data), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	u := ReadUsage(tmp, "test-session")
	if u.InputTokens != 300 {
		t.Errorf("InputTokens: got %d, want 300", u.InputTokens)
	}
	if u.OutputTokens != 150 {
		t.Errorf("OutputTokens: got %d, want 150", u.OutputTokens)
	}
	if u.CacheReadTokens != 3000 {
		t.Errorf("CacheReadTokens: got %d, want 3000", u.CacheReadTokens)
	}
	if u.CacheWriteTokens != 500 {
		t.Errorf("CacheWriteTokens: got %d, want 500", u.CacheWriteTokens)
	}
	if u.Model != "claude-opus-4-6" {
		t.Errorf("Model: got %q, want %q", u.Model, "claude-opus-4-6")
	}
	// Verify cost is non-zero and reasonable
	if u.CostUSD <= 0 {
		t.Error("CostUSD should be positive")
	}

	// Manual cost check: opus pricing
	// (300/1M * 15) + (150/1M * 75) + (3000/1M * 1.5) + (500/1M * 18.75)
	expected := 300.0/1e6*15 + 150.0/1e6*75 + 3000.0/1e6*1.5 + 500.0/1e6*18.75
	if math.Abs(u.CostUSD-expected) > 0.000001 {
		t.Errorf("CostUSD: got %f, want %f", u.CostUSD, expected)
	}
}

func TestReadUsage_MissingFile(t *testing.T) {
	u := ReadUsage("/nonexistent", "nosession")
	if u.InputTokens != 0 || u.CostUSD != 0 {
		t.Error("expected zero usage for missing file")
	}
}

func TestReadUsage_SkipsMalformed(t *testing.T) {
	tmp := t.TempDir()
	data := `not json at all
{"type":"assistant","message":{"model":"claude-sonnet-4-6","role":"assistant","content":[],"usage":{"input_tokens":50,"output_tokens":25}},"timestamp":"2026-03-28T10:00:00Z"}
`
	if err := os.WriteFile(filepath.Join(tmp, "test.jsonl"), []byte(data), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	u := ReadUsage(tmp, "test")
	if u.InputTokens != 50 {
		t.Errorf("InputTokens: got %d, want 50", u.InputTokens)
	}
}

func TestFormatCost(t *testing.T) {
	tests := []struct {
		cost float64
		want string
	}{
		{0.0, "$0.0000"},
		{0.005, "$0.0050"},
		{0.01, "$0.01"},
		{1.234, "$1.23"},
		{10.5, "$10.50"},
	}
	for _, tt := range tests {
		got := FormatCost(tt.cost)
		if got != tt.want {
			t.Errorf("FormatCost(%f): got %q, want %q", tt.cost, got, tt.want)
		}
	}
}

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{500, "500"},
		{1500, "1.5k"},
		{12345, "12.3k"},
		{1234567, "1.2M"},
	}
	for _, tt := range tests {
		got := FormatTokens(tt.n)
		if got != tt.want {
			t.Errorf("FormatTokens(%d): got %q, want %q", tt.n, got, tt.want)
		}
	}
}
