package main

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// runTmux is a test helper that runs a tmux command with a timeout.
func runTmux(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "tmux", args...).Run()
}

// tmuxFirstWindow returns the first window index in a session (respects base-index).
func tmuxFirstWindow(session string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tmux", "list-windows", "-t", session,
		"-F", "#{window_index}").Output()
	if err != nil {
		return "0"
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) > 0 {
		return lines[0]
	}
	return "0"
}

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

func TestTmuxResolvePaneID_EnvSet(t *testing.T) {
	t.Setenv("TMUX_PANE", "%42")
	got := TmuxResolvePaneID()
	if got != "%42" {
		t.Errorf("TmuxResolvePaneID() = %q, want %%42", got)
	}
}

func TestTmuxResolvePaneID_EnvEmpty(t *testing.T) {
	if !TmuxIsAvailable() {
		t.Skip("tmux not available")
	}
	t.Setenv("TMUX_PANE", "")
	got := TmuxResolvePaneID()
	// In a real tmux session, should resolve to a pane ID like %N
	if got == "" {
		t.Error("TmuxResolvePaneID() returned empty when tmux is available")
	}
	if got != "" && got[0] != '%' {
		t.Errorf("TmuxResolvePaneID() = %q, expected %%N format", got)
	}
}

func TestTmuxNewWindow_MultipleWindows(t *testing.T) {
	if !TmuxIsAvailable() {
		t.Skip("tmux not available")
	}

	// Regression test: TmuxNewWindow must work when the session already has
	// multiple windows. Previously, passing "-t session" (without trailing
	// colon) caused tmux to try inserting at the current window index,
	// failing with "index N in use" when a client was attached.
	session := "test-new-window"
	dir := t.TempDir()

	if err := runTmux("new-session", "-d", "-s", session, "-x", "80", "-y", "24"); err != nil {
		t.Fatalf("new-session: %v", err)
	}
	defer runTmux("kill-session", "-t", session)

	// Fill up several windows
	_ = runTmux("new-window", "-t", session+":")
	_ = runTmux("new-window", "-t", session+":")

	target, err := TmuxNewWindow(session, "test-win", dir)
	if err != nil {
		t.Fatalf("TmuxNewWindow failed: %v", err)
	}
	if target == "" {
		t.Fatal("expected non-empty target")
	}
	if err := ValidateTarget(target); err != nil {
		t.Errorf("returned invalid target %q: %v", target, err)
	}
}

func TestTmuxZoomPane(t *testing.T) {
	if !TmuxIsAvailable() {
		t.Skip("tmux not available")
	}

	session := "test-zoom-pane"
	dir := t.TempDir()

	if err := runTmux("new-session", "-d", "-s", session, "-x", "80", "-y", "24", "-c", dir); err != nil {
		t.Fatalf("new-session: %v", err)
	}
	defer runTmux("kill-session", "-t", session)

	// Create a second pane so zoom is meaningful
	winIdx := tmuxFirstWindow(session)
	sw := session + ":" + winIdx
	if err := runTmux("split-window", "-t", sw, "-c", dir, "-d"); err != nil {
		t.Fatalf("split-window: %v", err)
	}

	// Get the first pane's actual index (respects pane-base-index)
	ctx0, cancel0 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel0()
	paneOut, err := exec.CommandContext(ctx0, "tmux", "list-panes", "-t", sw,
		"-F", "#{pane_index}").Output()
	if err != nil {
		t.Fatalf("list-panes: %v", err)
	}
	firstPane := strings.Split(strings.TrimSpace(string(paneOut)), "\n")[0]
	target := sw + "." + firstPane

	if err := TmuxZoomPane(target); err != nil {
		t.Fatalf("TmuxZoomPane failed: %v", err)
	}

	// Verify the pane is zoomed
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tmux",
		"display-message", "-p", "-t", target, "#{window_zoomed_flag}").Output()
	if err != nil {
		t.Fatalf("display-message failed: %v", err)
	}
	if strings.TrimSpace(string(out)) != "1" {
		t.Errorf("expected pane to be zoomed (flag=1), got %q", strings.TrimSpace(string(out)))
	}
}

func TestTmuxSplitWindow_AppliesEvenLayout(t *testing.T) {
	if !TmuxIsAvailable() {
		t.Skip("tmux not available")
	}

	session := "test-split-layout"
	dir := t.TempDir()

	if err := runTmux("new-session", "-d", "-s", session, "-x", "80", "-y", "24", "-c", dir); err != nil {
		t.Fatalf("new-session: %v", err)
	}
	defer runTmux("kill-session", "-t", session)

	winIdx := tmuxFirstWindow(session)
	sw := session + ":" + winIdx
	_, err := TmuxSplitWindow(sw, dir)
	if err != nil {
		t.Fatalf("TmuxSplitWindow failed: %v", err)
	}

	// After split + even layout, both panes should exist
	count, err := TmuxCountPanes(sw)
	if err != nil {
		t.Fatalf("TmuxCountPanes failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 panes, got %d", count)
	}
}

func TestTmuxKillPane_AppliesEvenLayout(t *testing.T) {
	if !TmuxIsAvailable() {
		t.Skip("tmux not available")
	}

	session := "test-kill-layout"
	dir := t.TempDir()

	if err := runTmux("new-session", "-d", "-s", session, "-x", "80", "-y", "24", "-c", dir); err != nil {
		t.Fatalf("new-session: %v", err)
	}
	defer runTmux("kill-session", "-t", session)

	// Create 3 panes total
	winIdx := tmuxFirstWindow(session)
	sw := session + ":" + winIdx
	if err := runTmux("split-window", "-t", sw, "-c", dir, "-d"); err != nil {
		t.Fatalf("split-window 1: %v", err)
	}
	if err := runTmux("split-window", "-t", sw, "-c", dir, "-d"); err != nil {
		t.Fatalf("split-window 2: %v", err)
	}

	countBefore, err := TmuxCountPanes(sw)
	if err != nil {
		t.Fatalf("TmuxCountPanes before: %v", err)
	}
	if countBefore != 3 {
		t.Fatalf("expected 3 panes before kill, got %d", countBefore)
	}

	// Find the last pane index dynamically (respects pane-base-index)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	paneOut, pErr := exec.CommandContext(ctx, "tmux", "list-panes", "-t", sw,
		"-F", "#{pane_index}").Output()
	if pErr != nil {
		t.Fatalf("list-panes: %v", pErr)
	}
	paneLines := strings.Split(strings.TrimSpace(string(paneOut)), "\n")
	lastPane := paneLines[len(paneLines)-1]

	// Kill the last pane
	if err := TmuxKillPane(sw + "." + lastPane); err != nil {
		t.Fatalf("TmuxKillPane failed: %v", err)
	}

	countAfter, _ := TmuxCountPanes(sw)
	if countAfter != 2 {
		t.Errorf("expected 2 panes after kill, got %d", countAfter)
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

func TestDetectShellError(t *testing.T) {
	tests := []struct {
		name    string
		lines   []string
		wantErr bool
		wantMsg string
	}{
		{
			name:    "clean startup",
			lines:   []string{"$ claude --prompt 'hello'", ""},
			wantErr: false,
		},
		{
			name:    "empty pane",
			lines:   []string{"", ""},
			wantErr: false,
		},
		{
			name:    "zsh compinit error",
			lines:   []string{"zsh compinit: insecure directories, run compaudit for list.", "$ "},
			wantErr: true,
			wantMsg: "zsh compinit",
		},
		{
			name:    "zsh command not found",
			lines:   []string{"zsh: command not found: claude", "$ "},
			wantErr: true,
			wantMsg: "zsh:",
		},
		{
			name:    "compdef error",
			lines:   []string{"compdef: unknown command or service: git", "$ "},
			wantErr: true,
			wantMsg: "compdef:",
		},
		{
			name:    "oh-my-zsh upgrade prompt",
			lines:   []string{"[Oh My Zsh] Would you like to update? [Y/n]", ""},
			wantErr: true,
			wantMsg: "Oh My Zsh",
		},
		{
			name:    "p10k instant prompt error",
			lines:   []string{"[WARNING]: Console output during zsh initialization detected.", ""},
			wantErr: true,
			wantMsg: "Console output during zsh initialization",
		},
		{
			name:    "bash syntax error",
			lines:   []string{"/Users/user/.bashrc: line 42: syntax error near unexpected token", "$ "},
			wantErr: true,
			wantMsg: "syntax error",
		},
		{
			name:    "permission denied on rc file",
			lines:   []string{"/Users/user/.zshrc: permission denied", "$ "},
			wantErr: true,
			wantMsg: "permission denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := detectShellError(tt.lines)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
			if tt.wantErr && err != nil && !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error %q should contain %q", err.Error(), tt.wantMsg)
			}
		})
	}
}
