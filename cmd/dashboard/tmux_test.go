package main

import (
	"testing"
)

func TestValidateTarget(t *testing.T) {
	valid := []string{
		"main:0.1",
		"myproject:0.1",
		"dev:12.3",
		"session:0",
		"a-b_c:1.0",
	}
	for _, target := range valid {
		if err := ValidateTarget(target); err != nil {
			t.Errorf("ValidateTarget(%q) = %v, want nil", target, err)
		}
	}

	invalid := []string{
		"",
		"session; rm -rf ~",
		"{last}",
		"foo bar:0.1",
		"$(whoami):0.1",
		"session:window.pane",
	}
	for _, target := range invalid {
		if err := ValidateTarget(target); err == nil {
			t.Errorf("ValidateTarget(%q) = nil, want error", target)
		}
	}
}

func TestExtractSession(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"main:0.1", "main"},
		{"my-project:2.3", "my-project"},
		{"skills:0", "skills"},
	}

	for _, tt := range tests {
		got := extractSession(tt.input)
		if got != tt.want {
			t.Errorf("extractSession(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseListWindowsOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   []TmuxWindowInfo
	}{
		{
			name:   "two windows",
			output: "0\tdashboard\n1\tskills\n",
			want: []TmuxWindowInfo{
				{Index: 0, Name: "dashboard"},
				{Index: 1, Name: "skills"},
			},
		},
		{
			name:   "empty output",
			output: "",
			want:   nil,
		},
		{
			name:   "single window",
			output: "3\tmy-repo\n",
			want: []TmuxWindowInfo{
				{Index: 3, Name: "my-repo"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseListWindowsOutput(tt.output)
			if len(got) != len(tt.want) {
				t.Fatalf("parseListWindowsOutput() got %d items, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("item %d: got %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseCountPanesOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   int
	}{
		{"one pane", "0\n", 1},
		{"three panes", "0\n1\n2\n", 3},
		{"empty", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCountPanesOutput(tt.output)
			if got != tt.want {
				t.Errorf("parseCountPanesOutput() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestParsePaneTarget(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{"normal", "skills:1.2\n", "skills:1.2"},
		{"with spaces", "  main:0.0  \n", "main:0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePaneTarget(tt.output)
			if got != tt.want {
				t.Errorf("parsePaneTarget() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractSessionWindow(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"main:0.1", "main:0"},
		{"my.project:0.1", "my.project:0"},
		{"127.0.0.1:0.1", "127.0.0.1:0"},
		{"main:0", "main:0"},
		{"dev:12.3", "dev:12"},
	}

	for _, tt := range tests {
		got := extractSessionWindow(tt.input)
		if got != tt.want {
			t.Errorf("extractSessionWindow(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
