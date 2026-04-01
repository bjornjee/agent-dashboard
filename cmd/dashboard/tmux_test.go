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

func TestParseTarget(t *testing.T) {
	tests := []struct {
		input       string
		wantSession string
		wantWindow  int
		wantPane    int
		wantOK      bool
	}{
		{"main:0.1", "main", 0, 1, true},
		{"tomoro:3.2", "tomoro", 3, 2, true},
		{"dev:12.3", "dev", 12, 3, true},
		{"a-b_c:1.0", "a-b_c", 1, 0, true},
		// edge cases
		{"main:0", "", 0, 0, false},   // no pane
		{"main", "", 0, 0, false},     // no window or pane
		{"", "", 0, 0, false},         // empty
		{":0.1", "", 0, 0, false},     // no session
		{"main:a.b", "", 0, 0, false}, // non-numeric
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			session, window, pane, ok := parseTarget(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("parseTarget(%q) ok=%v, want %v", tt.input, ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if session != tt.wantSession {
				t.Errorf("session=%q, want %q", session, tt.wantSession)
			}
			if window != tt.wantWindow {
				t.Errorf("window=%d, want %d", window, tt.wantWindow)
			}
			if pane != tt.wantPane {
				t.Errorf("pane=%d, want %d", pane, tt.wantPane)
			}
		})
	}
}

func TestParsePaneTargetsOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   map[string]PaneTarget
	}{
		{
			name:   "multiple panes",
			output: "%85\ttomoro\t1\t1\n%87\ttomoro\t2\t1\n%90\ttomoro\t3\t2\n",
			want: map[string]PaneTarget{
				"%85": {Session: "tomoro", Window: 1, Pane: 1, Target: "tomoro:1.1"},
				"%87": {Session: "tomoro", Window: 2, Pane: 1, Target: "tomoro:2.1"},
				"%90": {Session: "tomoro", Window: 3, Pane: 2, Target: "tomoro:3.2"},
			},
		},
		{
			name:   "empty output",
			output: "",
			want:   map[string]PaneTarget{},
		},
		{
			name:   "malformed line skipped",
			output: "%85\ttomoro\t1\t1\nbadline\n%87\ttomoro\t2\t0\n",
			want: map[string]PaneTarget{
				"%85": {Session: "tomoro", Window: 1, Pane: 1, Target: "tomoro:1.1"},
				"%87": {Session: "tomoro", Window: 2, Pane: 0, Target: "tomoro:2.0"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePaneTargetsOutput(tt.output)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d entries, want %d", len(got), len(tt.want))
			}
			for k, wantV := range tt.want {
				gotV, ok := got[k]
				if !ok {
					t.Errorf("missing key %q", k)
					continue
				}
				if gotV != wantV {
					t.Errorf("key %q: got %+v, want %+v", k, gotV, wantV)
				}
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
