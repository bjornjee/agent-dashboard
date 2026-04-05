package tmux

import (
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/mocks"
	"github.com/stretchr/testify/mock"
)

// withMockRunner swaps the package-level runner with a testify mock
// and restores the real ExecRunner when the test finishes.
func withMockRunner(t *testing.T) *mocks.MockRunner {
	t.Helper()
	m := mocks.NewMockRunner(t)
	orig := runner
	runner = m
	t.Cleanup(func() { runner = orig })
	return m
}

// ---------------------------------------------------------------------------
// Pure unit tests (no tmux interaction)
// ---------------------------------------------------------------------------

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
		got := ExtractSession(tt.input)
		if got != tt.want {
			t.Errorf("ExtractSession(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func Test_parseListWindowsOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   []domain.TmuxWindowInfo
	}{
		{
			name:   "two windows",
			output: "0\tdashboard\n1\tskills\n",
			want: []domain.TmuxWindowInfo{
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
			want: []domain.TmuxWindowInfo{
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

func Test_parseCountPanesOutput(t *testing.T) {
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

func Test_parsePaneTarget(t *testing.T) {
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
			session, window, pane, ok := ParseTarget(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("ParseTarget(%q) ok=%v, want %v", tt.input, ok, tt.wantOK)
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

func Test_parsePaneTargetsOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   map[string]domain.PaneTarget
	}{
		{
			name:   "multiple panes",
			output: "%85\ttomoro\t1\t1\n%87\ttomoro\t2\t1\n%90\ttomoro\t3\t2\n",
			want: map[string]domain.PaneTarget{
				"%85": {Session: "tomoro", Window: 1, Pane: 1, Target: "tomoro:1.1"},
				"%87": {Session: "tomoro", Window: 2, Pane: 1, Target: "tomoro:2.1"},
				"%90": {Session: "tomoro", Window: 3, Pane: 2, Target: "tomoro:3.2"},
			},
		},
		{
			name:   "empty output",
			output: "",
			want:   map[string]domain.PaneTarget{},
		},
		{
			name:   "malformed line skipped",
			output: "%85\ttomoro\t1\t1\nbadline\n%87\ttomoro\t2\t0\n",
			want: map[string]domain.PaneTarget{
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
		got := ExtractSessionWindow(tt.input)
		if got != tt.want {
			t.Errorf("ExtractSessionWindow(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Mocked tests (using testify mock Runner — no exec at all)
// ---------------------------------------------------------------------------

func TestTmuxResolvePaneID_EnvSet(t *testing.T) {
	t.Setenv("TMUX_PANE", "%42")
	got := TmuxResolvePaneID()
	if got != "%42" {
		t.Errorf("TmuxResolvePaneID() = %q, want %%42", got)
	}
}

func TestTmuxResolvePaneID_EnvEmpty(t *testing.T) {
	m := withMockRunner(t)
	m.On("Output", mock.Anything, "display-message", "-p", "#{pane_id}").
		Return([]byte("%99\n"), nil)

	t.Setenv("TMUX_PANE", "")
	got := TmuxResolvePaneID()
	if got != "%99" {
		t.Errorf("TmuxResolvePaneID() = %q, want %%99", got)
	}
	m.AssertExpectations(t)
}

func TestTmuxNewWindow_MultipleWindows(t *testing.T) {
	m := withMockRunner(t)
	m.On("Output", mock.Anything,
		"new-window", "-t", "test-new-window:", "-n", "test-win", "-c", mock.AnythingOfType("string"),
		"-d", "-P", "-F", "#{session_name}:#{window_index}.#{pane_index}",
	).Return([]byte("test-new-window:3.0"), nil)

	target, err := TmuxNewWindow("test-new-window", "test-win", t.TempDir())
	if err != nil {
		t.Fatalf("TmuxNewWindow failed: %v", err)
	}
	if target == "" {
		t.Fatal("expected non-empty target")
	}
	if err := ValidateTarget(target); err != nil {
		t.Errorf("returned invalid target %q: %v", target, err)
	}
	m.AssertExpectations(t)
}

func TestTmuxZoomPane(t *testing.T) {
	m := withMockRunner(t)
	m.On("Run", mock.Anything, "resize-pane", "-Z", "-t", "test-zoom:1.0").
		Return(nil)

	if err := TmuxZoomPane("test-zoom:1.0"); err != nil {
		t.Fatalf("TmuxZoomPane failed: %v", err)
	}
	m.AssertExpectations(t)
}

func TestTmuxSplitWindow_AppliesEvenLayout(t *testing.T) {
	m := withMockRunner(t)

	// split-window returns new pane target
	m.On("Output", mock.Anything,
		"split-window", "-t", "test-split:1", "-c", mock.AnythingOfType("string"),
		"-d", "-P", "-F", "#{session_name}:#{window_index}.#{pane_index}",
	).Return([]byte("test-split:1.1"), nil)

	// TmuxEvenLayout called after split
	m.On("Run", mock.Anything, "select-layout", "-t", "test-split:1", "tiled").
		Return(nil)

	// TmuxCountPanes to verify
	m.On("Output", mock.Anything,
		"list-panes", "-t", "test-split:1", "-F", "#{pane_index}",
	).Return([]byte("0\n1\n"), nil)

	_, err := TmuxSplitWindow("test-split:1", t.TempDir())
	if err != nil {
		t.Fatalf("TmuxSplitWindow failed: %v", err)
	}

	count, err := TmuxCountPanes("test-split:1")
	if err != nil {
		t.Fatalf("TmuxCountPanes failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 panes, got %d", count)
	}
	m.AssertExpectations(t)
}

func TestTmuxKillPane_AppliesEvenLayout(t *testing.T) {
	m := withMockRunner(t)

	// TmuxCountPanes before kill (3 panes)
	m.On("Output", mock.Anything,
		"list-panes", "-t", "test-kill:1", "-F", "#{pane_index}",
	).Return([]byte("0\n1\n2\n"), nil).Once()

	// kill-pane
	m.On("Run", mock.Anything, "kill-pane", "-t", "test-kill:1.2").
		Return(nil)

	// TmuxEvenLayout after kill
	m.On("Run", mock.Anything, "select-layout", "-t", "test-kill:1", "tiled").
		Return(nil)

	// TmuxCountPanes after kill (2 panes)
	m.On("Output", mock.Anything,
		"list-panes", "-t", "test-kill:1", "-F", "#{pane_index}",
	).Return([]byte("0\n1\n"), nil).Once()

	countBefore, err := TmuxCountPanes("test-kill:1")
	if err != nil {
		t.Fatalf("TmuxCountPanes before: %v", err)
	}
	if countBefore != 3 {
		t.Fatalf("expected 3 panes before kill, got %d", countBefore)
	}

	if err := TmuxKillPane("test-kill:1.2"); err != nil {
		t.Fatalf("TmuxKillPane failed: %v", err)
	}

	countAfter, _ := TmuxCountPanes("test-kill:1")
	if countAfter != 2 {
		t.Errorf("expected 2 panes after kill, got %d", countAfter)
	}
	m.AssertExpectations(t)
}

func TestTmuxCountPanes(t *testing.T) {
	m := withMockRunner(t)
	m.On("Output", mock.Anything,
		"list-panes", "-t", "test:1", "-F", "#{pane_index}",
	).Return([]byte("0\n1\n"), nil)

	count, err := TmuxCountPanes("test:1")
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
	m.AssertExpectations(t)
}
