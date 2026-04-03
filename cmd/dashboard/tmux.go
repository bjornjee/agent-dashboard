package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const tmuxTimeout = 2 * time.Second

// silentRun runs a command with stdout and stderr discarded.
// This prevents child processes from writing to the terminal,
// which could inject escape sequences that bubbletea misinterprets as keys.
func silentRun(cmd *exec.Cmd) error {
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

// silentStart starts a command with stdout and stderr discarded.
func silentStart(cmd *exec.Cmd) error {
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Start()
}

// validTarget matches tmux targets: session:window.pane where components are alphanumeric, dash, underscore, or dot.
var validTarget = regexp.MustCompile(`^[a-zA-Z0-9_.\-]+(:[0-9]+(\.[0-9]+)?)?$`)

// ansiEscape matches ANSI escape sequences (CSI, OSC, etc.)
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\][^\x1b]*\x1b\\|\x1b\][^\x07]*\x07|\x1b[^[\]].?`)

// ValidateTarget checks that a target string is a safe tmux target identifier.
func ValidateTarget(target string) error {
	if !validTarget.MatchString(target) {
		return fmt.Errorf("invalid tmux target: %q", target)
	}
	return nil
}

// TmuxIsAvailable checks if tmux is running.
func TmuxIsAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), tmuxTimeout)
	defer cancel()
	return silentRun(exec.CommandContext(ctx, "tmux", "list-sessions")) == nil
}

// TmuxResolvePaneID returns the current pane's ID (%N format).
// It first checks TMUX_PANE (set in regular panes) and falls back to
// querying tmux directly (needed in popups where TMUX_PANE is unset).
func TmuxResolvePaneID() string {
	if pane := os.Getenv("TMUX_PANE"); pane != "" {
		return pane
	}
	ctx, cancel := context.WithTimeout(context.Background(), tmuxTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tmux", "display-message", "-p", "#{pane_id}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// TmuxCapture captures the last N lines from a tmux pane.
func TmuxCapture(target string, lines int) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), tmuxTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "tmux",
		"capture-pane", "-p", "-t", target, "-S", fmt.Sprintf("-%d", lines),
	).Output()
	if err != nil {
		return nil, fmt.Errorf("capture-pane failed for %s: %w", target, err)
	}

	cleaned := ansiEscape.ReplaceAllString(string(out), "")
	cleaned = strings.ReplaceAll(cleaned, "\r", "")
	return strings.Split(cleaned, "\n"), nil
}

// launchErrorPatterns are shell error patterns that indicate a command
// failed to start in a newly created tmux pane.
var launchErrorPatterns = []string{
	"command not found",
	"permission denied",
	"no such file or directory",
	"compinit: insecure directories",
	"Would you like to update",
	"Segmentation fault",
	"Killed",
	"Abort trap",
	"Bus error",
}

// detectLaunchError checks captured pane lines for shell errors that indicate
// the launched command failed to start. Returns the matching error line or "".
func detectLaunchError(lines []string) string {
	for _, line := range lines {
		lower := strings.ToLower(line)
		for _, pat := range launchErrorPatterns {
			if strings.Contains(lower, strings.ToLower(pat)) {
				return strings.TrimSpace(line)
			}
		}
	}
	return ""
}

// TmuxJump switches to the tmux window and pane of the given target.
func TmuxJump(target string) error {
	sw := extractSessionWindow(target)

	ctx1, cancel1 := context.WithTimeout(context.Background(), tmuxTimeout)
	defer cancel1()
	if err := silentRun(exec.CommandContext(ctx1, "tmux", "select-window", "-t", sw)); err != nil {
		return fmt.Errorf("select-window failed for %s: %w", sw, err)
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), tmuxTimeout)
	defer cancel2()
	if err := silentRun(exec.CommandContext(ctx2, "tmux", "select-pane", "-t", target)); err != nil {
		return fmt.Errorf("select-pane failed for %s: %w", target, err)
	}

	// Best-effort zoom: fails harmlessly when the window has only one pane.
	_ = TmuxZoomPane(target)

	return nil
}

// TmuxZoomPane toggles zoom on a tmux pane (equivalent to prefix + z).
func TmuxZoomPane(target string) error {
	if err := ValidateTarget(target); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), tmuxTimeout)
	defer cancel()
	return silentRun(exec.CommandContext(ctx, "tmux", "resize-pane", "-Z", "-t", target))
}

// TmuxSelectPane switches focus to the given tmux pane without changing window.
func TmuxSelectPane(target string) error {
	if err := ValidateTarget(target); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), tmuxTimeout)
	defer cancel()
	return silentRun(exec.CommandContext(ctx, "tmux", "select-pane", "-t", target))
}

// TmuxSendKeys sends text literally to a tmux pane, followed by Enter.
// The -l flag prevents tmux from interpreting key names (e.g. "Enter", "Escape").
func TmuxSendKeys(target, text string) error {
	ctx1, cancel1 := context.WithTimeout(context.Background(), tmuxTimeout)
	defer cancel1()
	if err := silentRun(exec.CommandContext(ctx1, "tmux", "send-keys", "-l", "-t", target, text)); err != nil {
		return err
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), tmuxTimeout)
	defer cancel2()
	return silentRun(exec.CommandContext(ctx2, "tmux", "send-keys", "-t", target, "Enter"))
}

// TmuxSendRaw sends a single key to a tmux pane without Enter.
func TmuxSendRaw(target, key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), tmuxTimeout)
	defer cancel()
	return silentRun(exec.CommandContext(ctx, "tmux", "send-keys", "-t", target, key))
}

// TmuxKillPane kills a tmux pane by target and rebalances the window layout.
func TmuxKillPane(target string) error {
	sw := extractSessionWindow(target)
	ctx, cancel := context.WithTimeout(context.Background(), tmuxTimeout)
	defer cancel()
	if err := silentRun(exec.CommandContext(ctx, "tmux", "kill-pane", "-t", target)); err != nil {
		return fmt.Errorf("kill-pane failed for %s: %w", target, err)
	}
	// Rebalance remaining panes; ignore error (window may now be empty)
	_ = TmuxEvenLayout(sw)
	return nil
}

// TmuxEvenLayout applies a tiled layout to evenly space panes in a window.
func TmuxEvenLayout(sessionWindow string) error {
	if err := ValidateTarget(sessionWindow); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), tmuxTimeout)
	defer cancel()
	return silentRun(exec.CommandContext(ctx, "tmux", "select-layout", "-t", sessionWindow, "tiled"))
}

// ResolveTarget resolves a tmux pane ID (%N) to its current target string
// (session:window.pane). Returns "" if the pane no longer exists.
func ResolveTarget(paneID string) string {
	ctx, cancel := context.WithTimeout(context.Background(), tmuxTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tmux", "display-message", "-p", "-t", paneID,
		"#{session_name}:#{window_index}.#{pane_index}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// TmuxPaneCwd returns the current working directory of a tmux pane by its
// pane ID (%N format). Returns "" if the pane doesn't exist or on error.
func TmuxPaneCwd(paneID string) string {
	ctx, cancel := context.WithTimeout(context.Background(), tmuxTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tmux", "display-message", "-p", "-t", paneID,
		"#{pane_current_path}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// TmuxListPaneCwds returns pane_id → current working directory for every live pane.
// Used as a batch fallback when agent state files lack a cwd field.
func TmuxListPaneCwds() map[string]string {
	ctx, cancel := context.WithTimeout(context.Background(), tmuxTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tmux", "list-panes", "-a",
		"-F", "#{pane_id}\t#{pane_current_path}").Output()
	if err != nil {
		return nil
	}
	result := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 2 && parts[1] != "" {
			result[parts[0]] = parts[1]
		}
	}
	return result
}

// TmuxListLivePaneIDs returns the set of all live tmux pane IDs (%N format).
func TmuxListLivePaneIDs() map[string]bool {
	ctx, cancel := context.WithTimeout(context.Background(), tmuxTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tmux", "list-panes", "-a",
		"-F", "#{pane_id}").Output()
	if err != nil {
		return nil
	}
	panes := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			panes[line] = true
		}
	}
	return panes
}

// TmuxWindowInfo holds a tmux window's index and name.
type TmuxWindowInfo struct {
	Index int
	Name  string
}

// parseListWindowsOutput parses the output of tmux list-windows -F "#{window_index}\t#{window_name}".
func parseListWindowsOutput(output string) []TmuxWindowInfo {
	var windows []TmuxWindowInfo
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		idx, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		windows = append(windows, TmuxWindowInfo{Index: idx, Name: parts[1]})
	}
	return windows
}

// parseCountPanesOutput counts non-empty lines in tmux list-panes output.
func parseCountPanesOutput(output string) int {
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line != "" {
			count++
		}
	}
	return count
}

// parsePaneTarget extracts a clean pane target from tmux -P -F output.
func parsePaneTarget(output string) string {
	return strings.TrimSpace(output)
}

// TmuxListWindows lists all windows in a tmux session with their indices and names.
func TmuxListWindows(session string) ([]TmuxWindowInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), tmuxTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "tmux",
		"list-windows", "-t", session, "-F", "#{window_index}\t#{window_name}",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("list-windows failed for %s: %w", session, err)
	}
	return parseListWindowsOutput(string(out)), nil
}

// TmuxNewWindow creates a new window in the given session, returning the new pane's target.
// The -d flag keeps focus on the current window (dashboard).
func TmuxNewWindow(session, windowName, startDir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), tmuxTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "tmux",
		"new-window", "-t", session+":", "-n", windowName, "-c", startDir,
		"-d", "-P", "-F", "#{session_name}:#{window_index}.#{pane_index}",
	).Output()
	if err != nil {
		return "", fmt.Errorf("new-window failed: %w", err)
	}
	target := parsePaneTarget(string(out))
	if err := ValidateTarget(target); err != nil {
		return "", fmt.Errorf("new-window returned invalid target %q: %w", target, err)
	}
	return target, nil
}

// TmuxSplitWindow splits an existing window to create a new pane, returning its target.
// The -d flag keeps focus on the current pane (dashboard).
func TmuxSplitWindow(sessionWindow, startDir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), tmuxTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "tmux",
		"split-window", "-t", sessionWindow, "-c", startDir,
		"-d", "-P", "-F", "#{session_name}:#{window_index}.#{pane_index}",
	).Output()
	if err != nil {
		return "", fmt.Errorf("split-window failed: %w", err)
	}
	target := parsePaneTarget(string(out))
	if err := ValidateTarget(target); err != nil {
		return "", fmt.Errorf("split-window returned invalid target %q: %w", target, err)
	}
	// Rebalance panes in the window after adding one
	_ = TmuxEvenLayout(sessionWindow)
	return target, nil
}

// TmuxCountPanes returns the number of panes in a tmux window.
func TmuxCountPanes(sessionWindow string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), tmuxTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "tmux",
		"list-panes", "-t", sessionWindow, "-F", "#{pane_index}",
	).Output()
	if err != nil {
		return 0, fmt.Errorf("list-panes failed for %s: %w", sessionWindow, err)
	}
	return parseCountPanesOutput(string(out)), nil
}

// PaneTarget holds the resolved tmux coordinates for a pane.
type PaneTarget struct {
	Session string
	Window  int
	Pane    int
	Target  string // "session:window.pane"
}

// parseTarget splits a fully-qualified "session:window.pane" string into its
// components. Requires all three components — "session:window" (no pane) returns
// ok=false. Used as a fallback to extract Window/Pane from Agent.Target when
// tmux is unavailable.
func parseTarget(target string) (session string, window, pane int, ok bool) {
	colonIdx := strings.Index(target, ":")
	if colonIdx <= 0 || colonIdx >= len(target)-1 {
		return "", 0, 0, false
	}
	session = target[:colonIdx]
	rest := target[colonIdx+1:]

	dotIdx := strings.Index(rest, ".")
	if dotIdx < 0 || dotIdx >= len(rest)-1 {
		return "", 0, 0, false
	}

	w, err := strconv.Atoi(rest[:dotIdx])
	if err != nil {
		return "", 0, 0, false
	}
	p, err := strconv.Atoi(rest[dotIdx+1:])
	if err != nil {
		return "", 0, 0, false
	}
	return session, w, p, true
}

// parsePaneTargetsOutput parses the output of:
//
//	tmux list-panes -a -F "#{pane_id}\t#{session_name}\t#{window_index}\t#{pane_index}"
func parsePaneTargetsOutput(output string) map[string]PaneTarget {
	result := make(map[string]PaneTarget)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) != 4 {
			continue
		}
		paneID := parts[0]
		session := parts[1]
		w, err := strconv.Atoi(parts[2])
		if err != nil {
			continue
		}
		p, err := strconv.Atoi(parts[3])
		if err != nil {
			continue
		}
		result[paneID] = PaneTarget{
			Session: session,
			Window:  w,
			Pane:    p,
			Target:  fmt.Sprintf("%s:%d.%d", session, w, p),
		}
	}
	return result
}

// TmuxListPaneTargets returns the current target for every live tmux pane.
// The map key is the pane ID (%N format). Returns nil on error (e.g. tmux
// timeout); callers must handle nil gracefully (ResolveAgentTargets does).
func TmuxListPaneTargets() map[string]PaneTarget {
	ctx, cancel := context.WithTimeout(context.Background(), tmuxTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tmux", "list-panes", "-a",
		"-F", "#{pane_id}\t#{session_name}\t#{window_index}\t#{pane_index}").Output()
	if err != nil {
		return nil
	}
	return parsePaneTargetsOutput(string(out))
}

// extractSession returns the session name from a tmux target (session:window.pane → session).
func extractSession(target string) string {
	if idx := strings.Index(target, ":"); idx != -1 {
		return target[:idx]
	}
	return target
}

// extractSessionWindow returns session:window from session:window.pane.
func extractSessionWindow(target string) string {
	lastDot := strings.LastIndex(target, ".")
	if lastDot == -1 {
		return target
	}
	return target[:lastDot]
}
