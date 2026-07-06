package tmux

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

	"github.com/bjornjee/agent-dashboard/internal/domain"
)

const Timeout = 2 * time.Second

// Runner abstracts tmux command execution so tests can swap in a mock.
type Runner interface {
	Output(ctx context.Context, args ...string) ([]byte, error)
	Run(ctx context.Context, args ...string) error
}

// ExecRunner is the production Runner that shells out to tmux.
type ExecRunner struct{}

func (r *ExecRunner) Output(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "tmux", args...).Output()
}

func (r *ExecRunner) Run(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "tmux", args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

// runner is the package-level Runner used by all tmux functions.
// Tests replace this with a mock.
var runner Runner = &ExecRunner{}

// SetTestRunner swaps the package-level runner and returns a restore function.
// This allows test packages outside of tmux to inject a mock runner.
func SetTestRunner(r Runner) func() {
	orig := runner
	runner = r
	return func() { runner = orig }
}

// SilentRun runs a command with stdout and stderr discarded.
// This prevents child processes from writing to the terminal,
// which could inject escape sequences that bubbletea misinterprets as keys.
func SilentRun(cmd *exec.Cmd) error {
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

// SilentStart starts a command with stdout and stderr discarded.
func SilentStart(cmd *exec.Cmd) error {
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
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	return runner.Run(ctx, "list-sessions") == nil
}

// TmuxResolvePaneID returns the current pane's ID (%N format).
// It first checks TMUX_PANE (set in regular panes) and falls back to
// querying tmux directly (needed in popups where TMUX_PANE is unset).
func TmuxResolvePaneID() string {
	if pane := os.Getenv("TMUX_PANE"); pane != "" {
		return pane
	}
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	out, err := runner.Output(ctx, "display-message", "-p", "#{pane_id}")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// TmuxCapture captures the last N lines from a tmux pane.
func TmuxCapture(target string, lines int) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	out, err := runner.Output(ctx,
		"capture-pane", "-p", "-t", target, "-S", fmt.Sprintf("-%d", lines),
	)
	if err != nil {
		return nil, fmt.Errorf("capture-pane failed for %s: %w", target, err)
	}

	cleaned := ansiEscape.ReplaceAllString(string(out), "")
	cleaned = strings.ReplaceAll(cleaned, "\r", "")
	return strings.Split(cleaned, "\n"), nil
}

// TmuxJump switches to the tmux window and pane of the given target.
func TmuxJump(target string) error {
	sw := ExtractSessionWindow(target)

	ctx1, cancel1 := context.WithTimeout(context.Background(), Timeout)
	defer cancel1()
	if err := runner.Run(ctx1, "select-window", "-t", sw); err != nil {
		return fmt.Errorf("select-window failed for %s: %w", sw, err)
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), Timeout)
	defer cancel2()
	if err := runner.Run(ctx2, "select-pane", "-t", target); err != nil {
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
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	return runner.Run(ctx, "resize-pane", "-Z", "-t", target)
}

// TmuxSelectPane switches focus to the given tmux pane without changing window.
func TmuxSelectPane(target string) error {
	if err := ValidateTarget(target); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	return runner.Run(ctx, "select-pane", "-t", target)
}

// sendKeysChunkSize is the maximum number of bytes sent per send-keys call.
// tmux silently truncates text beyond its internal paste/input buffer (~1 KB).
const sendKeysChunkSize = 512

const pasteBufferName = "agent-dashboard-reply"

// TmuxSendKeys sends text literally to a tmux pane, followed by Enter.
// Long text is split into chunks to avoid tmux's input buffer truncation.
func TmuxSendKeys(target, text string) error {
	return tmuxSendKeysWithSubmit(target, text, "Enter")
}

func tmuxSendKeysWithSubmit(target, text string, submitKeys ...string) error {
	for len(text) > 0 {
		chunk := text
		if len(chunk) > sendKeysChunkSize {
			chunk = text[:sendKeysChunkSize]
		}
		text = text[len(chunk):]
		ctx, cancel := context.WithTimeout(context.Background(), Timeout)
		err := runner.Run(ctx, "send-keys", "-l", "-t", target, chunk)
		cancel()
		if err != nil {
			return err
		}
	}
	return tmuxSendSubmitKeys(target, submitKeys...)
}

func tmuxSendSubmitKeys(target string, submitKeys ...string) error {
	for _, key := range submitKeys {
		ctx, cancel := context.WithTimeout(context.Background(), Timeout)
		err := runner.Run(ctx, "send-keys", "-t", target, key)
		cancel()
		if err != nil {
			return err
		}
	}
	return nil
}

func TmuxSendKeysClearingInput(target, text string, submitKeys ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	err := runner.Run(ctx, "send-keys", "-t", target, "C-u")
	cancel()
	if err != nil {
		return err
	}
	return tmuxSendKeysWithSubmit(target, text, submitKeys...)
}

func TmuxPasteKeysClearingInput(target, text string, submitKeys ...string) error {
	if err := ValidateTarget(target); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	err := runner.Run(ctx, "send-keys", "-t", target, "C-u")
	cancel()
	if err != nil {
		return err
	}

	ctx, cancel = context.WithTimeout(context.Background(), Timeout)
	err = runner.Run(ctx, "set-buffer", "-b", pasteBufferName, "--", text)
	cancel()
	if err != nil {
		return err
	}

	ctx, cancel = context.WithTimeout(context.Background(), Timeout)
	err = runner.Run(ctx, "paste-buffer", "-p", "-r", "-d", "-b", pasteBufferName, "-t", target)
	cancel()
	if err != nil {
		return err
	}

	return tmuxSendSubmitKeys(target, submitKeys...)
}

// TmuxSendRaw sends a single key to a tmux pane without Enter.
func TmuxSendRaw(target, key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	return runner.Run(ctx, "send-keys", "-t", target, key)
}

// TmuxSendRawKeys sends raw keys to a tmux pane without adding Enter.
func TmuxSendRawKeys(target string, keys ...string) error {
	if err := ValidateTarget(target); err != nil {
		return err
	}
	return tmuxSendSubmitKeys(target, keys...)
}

// TmuxKillPane kills a tmux pane by target and rebalances the window layout.
func TmuxKillPane(target string) error {
	sw := ExtractSessionWindow(target)
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	if err := runner.Run(ctx, "kill-pane", "-t", target); err != nil {
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
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	return runner.Run(ctx, "select-layout", "-t", sessionWindow, "tiled")
}

// ResolveTarget resolves a tmux pane ID (%N) to its current target string
// (session:window.pane). Returns "" if the pane no longer exists.
func ResolveTarget(paneID string) string {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	out, err := runner.Output(ctx, "display-message", "-p", "-t", paneID,
		"#{session_name}:#{window_index}.#{pane_index}")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// TmuxListSessions returns the names of all tmux sessions.
func TmuxListSessions() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	out, err := runner.Output(ctx, "list-sessions", "-F", "#{session_name}")
	if err != nil {
		return nil, fmt.Errorf("no tmux sessions: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return nil, fmt.Errorf("no tmux sessions found")
	}
	return lines, nil
}

// TmuxListPanes returns the pane → target map, the pane → cwd map, and the
// tmux server PID from a single `tmux list-panes -a` invocation. The
// dashboard's hot-path state refresh used to issue two separate calls
// back-to-back; collapsing them halves the subprocess fanout per refresh
// tick. The server PID rides along in the same format string so pane
// liveness and server identity always come from the same server —
// enumeration succeeds ⟺ server PID known, which is what lets
// state.IsResumableOrphan event-scope orphan resume to server restarts.
//
// Returns (nil, nil, "") on tmux error — callers must handle nil gracefully
// (ResolveAgentTargets / ResolveAgentBranches do).
func TmuxListPanes() (map[string]domain.PaneTarget, map[string]string, map[string]string, string) {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	out, err := runner.Output(ctx, "list-panes", "-a",
		"-F", "#{pane_id}\t#{session_name}\t#{window_index}\t#{pane_index}\t#{pid}\t#{pane_current_path}\t#{pane_current_command}")
	if err != nil {
		return nil, nil, nil, ""
	}
	return parsePanesOutput(string(out))
}

// parsePanesOutput parses the tab-separated rows produced by the combined
// list-panes format. Each row is:
//
//	pane_id<TAB>session<TAB>window_idx<TAB>pane_idx<TAB>server_pid<TAB>pane_current_path<TAB>pane_current_command
//
// Rows missing the cwd column are tolerated for the targets map; the cwd
// map only gets entries whose path is non-empty. Rows missing the command
// column are also tolerated. The server PID is the same on every row; the
// first valid row wins.
func parsePanesOutput(output string) (map[string]domain.PaneTarget, map[string]string, map[string]string, string) {
	targets := make(map[string]domain.PaneTarget)
	cwds := make(map[string]string)
	cmds := make(map[string]string)
	serverPID := ""
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 7)
		if len(parts) < 5 {
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
		targets[paneID] = domain.PaneTarget{
			Session: session,
			Window:  w,
			Pane:    p,
			Target:  fmt.Sprintf("%s:%d.%d", session, w, p),
		}
		if serverPID == "" {
			serverPID = numericPID(parts[4])
		}
		if len(parts) == 6 && parts[5] != "" {
			cwds[paneID] = parts[5]
		}
		if len(parts) == 7 {
			if parts[5] != "" {
				cwds[paneID] = parts[5]
			}
			if parts[6] != "" {
				cmds[paneID] = parts[6]
			}
		}
	}
	return targets, cwds, cmds, serverPID
}

// TmuxListLivePaneIDs returns the set of all live tmux pane IDs (%N format)
// and the tmux server PID, from one invocation (same rationale as
// TmuxListPanes: liveness and server identity must be observed atomically).
// Returns (nil, "") on tmux error.
func TmuxListLivePaneIDs() (map[string]bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	out, err := runner.Output(ctx, "list-panes", "-a",
		"-F", "#{pane_id}\t#{pid}")
	if err != nil {
		return nil, ""
	}
	panes := make(map[string]bool)
	serverPID := ""
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		panes[parts[0]] = true
		if serverPID == "" && len(parts) == 2 {
			serverPID = numericPID(parts[1])
		}
	}
	return panes, serverPID
}

// numericPID returns pid only when it is a plain decimal number. A tmux
// that doesn't know the #{pid} format variable emits it as literal text;
// treating that as a server identity would poison every consumer that
// compares stamped server PIDs (orphan event-scoping, hydrate, sweep).
// Dropping it yields the enumeration-failed ("") case, which every
// consumer already fails safe on.
func numericPID(pid string) string {
	if pid == "" {
		return ""
	}
	for _, r := range pid {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return pid
}

// parseListWindowsOutput parses the output of tmux list-windows -F "#{window_index}\t#{window_name}".
func parseListWindowsOutput(output string) []domain.TmuxWindowInfo {
	var windows []domain.TmuxWindowInfo
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
		windows = append(windows, domain.TmuxWindowInfo{Index: idx, Name: parts[1]})
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

// parsePaneIDAndTarget splits the two-column tmux -F output produced by
// TmuxNewWindow / TmuxSplitWindow. Format: "#{pane_id}\t#{session_name}:#{window_index}.#{pane_index}".
// Returns ("", target) when the input is a bare target without a tab —
// preserves backwards-compat for any caller still producing single-column output.
func parsePaneIDAndTarget(output string) (paneID, target string) {
	line := strings.TrimSpace(output)
	if i := strings.IndexByte(line, '\t'); i >= 0 {
		return line[:i], strings.TrimSpace(line[i+1:])
	}
	return "", line
}

// TmuxListWindows lists all windows in a tmux session with their indices and names.
func TmuxListWindows(session string) ([]domain.TmuxWindowInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	out, err := runner.Output(ctx,
		"list-windows", "-t", session, "-F", "#{window_index}\t#{window_name}",
	)
	if err != nil {
		return nil, fmt.Errorf("list-windows failed for %s: %w", session, err)
	}
	return parseListWindowsOutput(string(out)), nil
}

// TmuxNewWindow creates a new window in the given session, returning the new
// pane's target and stable pane_id (`%N`). pane_id is needed so the dashboard
// can stage a spawn-pin keyed by an identifier that survives positional
// renumbering (window/pane indices shift when sibling panes close).
// The -d flag keeps focus on the current window (dashboard).
func TmuxNewWindow(session, windowName, startDir string, shellCmd ...string) (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	args := []string{
		"new-window", "-t", session + ":", "-n", windowName, "-c", startDir,
		"-d", "-P", "-F", "#{pane_id}\t#{session_name}:#{window_index}.#{pane_index}",
	}
	if len(shellCmd) > 0 && shellCmd[0] != "" {
		args = append(args, shellCmd[0])
	}
	out, err := runner.Output(ctx, args...)
	if err != nil {
		return "", "", fmt.Errorf("new-window failed: %w", err)
	}
	paneID, target := parsePaneIDAndTarget(string(out))
	if err := ValidateTarget(target); err != nil {
		return "", "", fmt.Errorf("new-window returned invalid target %q: %w", target, err)
	}
	return target, paneID, nil
}

// TmuxSplitWindow splits an existing window to create a new pane, returning
// its target and stable pane_id (`%N`). See TmuxNewWindow for why pane_id is
// emitted alongside the positional target. The -d flag keeps focus on the
// current pane (dashboard).
func TmuxSplitWindow(sessionWindow, startDir string, shellCmd ...string) (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	args := []string{
		"split-window", "-t", sessionWindow, "-c", startDir,
		"-d", "-P", "-F", "#{pane_id}\t#{session_name}:#{window_index}.#{pane_index}",
	}
	if len(shellCmd) > 0 && shellCmd[0] != "" {
		args = append(args, shellCmd[0])
	}
	out, err := runner.Output(ctx, args...)
	if err != nil {
		return "", "", fmt.Errorf("split-window failed: %w", err)
	}
	paneID, target := parsePaneIDAndTarget(string(out))
	if err := ValidateTarget(target); err != nil {
		return "", "", fmt.Errorf("split-window returned invalid target %q: %w", target, err)
	}
	// Rebalance panes in the window after adding one
	_ = TmuxEvenLayout(sessionWindow)
	return target, paneID, nil
}

// TmuxCountPanes returns the number of panes in a tmux window.
func TmuxCountPanes(sessionWindow string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	out, err := runner.Output(ctx,
		"list-panes", "-t", sessionWindow, "-F", "#{pane_index}",
	)
	if err != nil {
		return 0, fmt.Errorf("list-panes failed for %s: %w", sessionWindow, err)
	}
	return parseCountPanesOutput(string(out)), nil
}

// ParseTarget splits a fully-qualified "session:window.pane" string into its
// components. Requires all three components — "session:window" (no pane) returns
// ok=false. Used as a fallback to extract Window/Pane from domain.Agent.Target when
// tmux is unavailable.
func ParseTarget(target string) (session string, window, pane int, ok bool) {
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

// ExtractSession returns the session name from a tmux target (session:window.pane → session).
func ExtractSession(target string) string {
	if idx := strings.Index(target, ":"); idx != -1 {
		return target[:idx]
	}
	return target
}

// ExtractSessionWindow returns session:window from session:window.pane.
func ExtractSessionWindow(target string) string {
	lastDot := strings.LastIndex(target, ".")
	if lastDot == -1 {
		return target
	}
	return target[:lastDot]
}
