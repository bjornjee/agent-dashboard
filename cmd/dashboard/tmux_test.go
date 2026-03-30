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

func TestParseListPanesWithIDOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   map[string]string
	}{
		{
			name:   "two panes",
			output: "%0\tmain:0.0\n%1\tmain:1.0\n",
			want:   map[string]string{"%0": "main:0.0", "%1": "main:1.0"},
		},
		{
			name:   "empty output",
			output: "",
			want:   map[string]string{},
		},
		{
			name:   "single pane",
			output: "%5\tskills:2.1\n",
			want:   map[string]string{"%5": "skills:2.1"},
		},
		{
			name:   "malformed line ignored",
			output: "%0\tmain:0.0\nbadline\n%2\tmain:1.0\n",
			want:   map[string]string{"%0": "main:0.0", "%2": "main:1.0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseListPanesWithIDOutput(tt.output)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d items, want %d", len(got), len(tt.want))
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("key %q: got %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestBuildTargetRenames(t *testing.T) {
	before := map[string]string{
		"%0": "main:0.0", // dashboard — stays
		"%1": "main:1.0", // agent A — will be killed
		"%2": "main:2.0", // agent B — will be renumbered to main:1.0
	}
	after := map[string]string{
		"%0": "main:0.0", // dashboard — stays
		"%2": "main:1.0", // agent B — renumbered
	}
	killedTarget := "main:1.0"

	renames := BuildTargetRenames(before, after, killedTarget)

	// Agent B's old target should map to new target
	if renames["main:2.0"] != "main:1.0" {
		t.Errorf("expected rename main:2.0 → main:1.0, got %q", renames["main:2.0"])
	}
	// Should not include killed target or unchanged targets
	if _, ok := renames["main:1.0"]; ok {
		t.Error("killed target should not appear in renames")
	}
	if _, ok := renames["main:0.0"]; ok {
		t.Error("unchanged target should not appear in renames")
	}
}

func TestBuildTargetRenames_NoRenumbering(t *testing.T) {
	before := map[string]string{
		"%0": "main:0.0",
		"%1": "main:1.0",
		"%2": "main:1.1", // same window as killed
	}
	after := map[string]string{
		"%0": "main:0.0",
		"%2": "main:1.1", // stays same — no window renumbering
	}

	renames := BuildTargetRenames(before, after, "main:1.0")
	if len(renames) != 0 {
		t.Errorf("expected no renames when no renumbering, got %v", renames)
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
