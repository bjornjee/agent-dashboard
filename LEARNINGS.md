# Learnings: Building a TUI Dashboard for Claude Code Agents

Hard-won lessons from building [agent-dashboard](https://github.com/bjornjee/agent-dashboard) — a Bubble Tea TUI that orchestrates multiple Claude Code agents via tmux. These cover crashes, race conditions, terminal quirks, and architectural patterns discovered over months of development.

---

## Crashes & System-Level Kills

### macOS AMFI Kills Unsigned Test Binaries

**Problem:** `go test -race` requires CGO, which produces unsigned binaries. macOS AMFI (Apple Mobile File Integrity) rejects unsigned binaries running in temp directories with SIGKILL — no warning, no log.

**Cascade:** SIGKILL on test binary → kills the parent Claude Code session → tmux detects no remaining clients → `exit-empty` shuts down the entire tmux server → all running agents die.

**What we tried first:** Investigating Claude Code crash logs and tmux server logs. Neither showed anything useful because SIGKILL can't be caught or logged by the victim process.

**Fix:** Disable CGO for regular test runs. Add a separate `test-race` target for CI environments where AMFI isn't an issue.

```makefile
test:
	CGO_ENABLED=0 go test ./...

test-race:
	go test -race ./...
```

> `a38e9c2` — fix: disable CGO in make test to avoid AMFI kills on macOS

---

### Sleep/Wake Cycle Kills Dashboard Process

**Problem:** macOS `taskgated` (kernel code-signature validator) kills unsigned binaries after a sleep/wake cycle with SIGKILL (Code Signature Invalid). Go's default `go build` output has no code signature.

**How it manifested:** Dashboard would be running fine, user closes laptop, opens it next morning — dashboard gone. No error, no crash log, nothing in Console.app unless you know to filter by `taskgated`.

**Fix:** Ad-hoc codesign the binary after build. Doesn't require an Apple Developer certificate.

```makefile
build:
	go build $(LDFLAGS) -o bin/agent-dashboard ./cmd/dashboard/
	@if [ "$$(uname)" = "Darwin" ]; then codesign -s - bin/agent-dashboard; fi
```

> `95646b2` — fix: ad-hoc codesign binary on macOS to survive sleep/wake

---

### Silent Dashboard Termination — The Wrong Suspect

**Problem:** Dashboard was dying with no visible cause. No quit keypress in the debug key log, no panic output, no tmux error.

**The red herring:** Initially blamed phantom keystrokes (see below). Built a cooldown guard for the `q` key. Deployed it. Dashboard still died.

**The breakthrough:** Debug key log showed only mouse events in the seconds before death — no keystrokes at all. This proved phantom keys weren't the cause.

**Real cause:** AMFI/taskgated SIGKILL (see above). The process was being killed by the OS, not by user input.

**Fix:** Redirect stderr to a crash log *before* entering Bubble Tea's alternate screen. Add signal handlers to log the cause of death.

```go
crashLog, _ := os.OpenFile(crashLogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
os.Stderr = crashLog
fmt.Fprintf(crashLog, "=== dashboard started pid=%d %s ===\n", os.Getpid(), time.Now().Format(time.RFC3339))

sigCh := make(chan os.Signal, 1)
signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGQUIT)
go func() {
    sig := <-sigCh
    fmt.Fprintf(os.Stderr, "=== received signal: %s at %s ===\n", sig, time.Now().Format(time.RFC3339))
    os.Exit(128 + int(sig.(syscall.Signal)))
}()
```

**Lesson:** When a TUI owns the alternate screen, stderr is invisible. Always redirect it to a file before entering alt screen. This single change made every subsequent crash debuggable.

> `4a5d6d8` — fix: add crash diagnostics for silent dashboard termination
> `e7a9490` — revert: phantom q guard (wrong fix)

---

## Hook Process Starvation & Race Conditions

### Every Tool Call Spawns a Forest of Processes

**Problem:** Claude Code fires hooks on every tool call. The dashboard registers hooks on 9 lifecycle events (7 for agent state, 2 for desktop notifications). On the hot path (PreToolUse + PostToolUse), up to 7 hooks fire *per Bash tool call* — though some (commit-lint, test-gate) only activate on git commit commands. Each hook spawns a Node.js process, which itself may spawn subprocesses (git, tmux, ps).

**Math:** A busy agent making 10 tool calls/minute fires dozens of hook invocations/minute. With 5 concurrent agents, that's hundreds of Node.js processes/minute. Under sustained load, this starves the system of PIDs and file descriptors.

**Mitigation:** Split hooks into two tiers:
- **Fast hooks** (`agent-state-fast.js`): PreToolUse, PostToolUse, PermissionRequest. Only update state, permission_mode, current_tool. No subprocess spawning. Target: <100ms.
- **Full reporter** (`agent-state-reporter.js`): SessionStart, SubagentStart/Stop, Stop. Captures git status, tmux pane content, file changes. Allowed to be slow (~5s).

This reduced hot-path overhead by ~80% while keeping rich state on lifecycle boundaries.

---

### PostToolUse Overwrites Stop State (The Timing Race)

**Problem:** When an agent stops, the Stop hook fires and correctly sets state to `idle_prompt` or `done`. But the *previous* tool call's PostToolUse hook is still in-flight (async). It arrives after the Stop hook and overwrites the state back to `running`.

**Result:** Agents stuck showing "running" in the dashboard when they're actually idle at a prompt. Users don't know the agent is waiting for input.

**What we tried first:**
1. Idle override scanner — periodically check JSONL for pending `AskUserQuestion`/`ExitPlanMode` and force-correct the state. Worked but was a band-aid.
2. Found a deeper bug: the Stop hook's `report()` function referenced `lastMessage` which was only defined in a nested scope. In strict mode, this threw `ReferenceError` — silently caught by the stdin error wrapper, so the Stop hook *never actually wrote state*.

**Fix (two parts):**
1. Fix the scoping bug so Stop hooks actually write state. (`75c256b`)
2. Add a stop-state guard in the fast hook — if current state is a stop-derived state (`idle_prompt`, `done`, `question`), PostToolUse is not allowed to overwrite it. Only the next PreToolUse (meaning a new turn has started) can transition back to `running`.

```javascript
const STOP_STATES = new Set(['idle_prompt', 'done', 'question']);

if (hookEvent === 'PostToolUse' && STOP_STATES.has(existing.state)) {
    return { changed: false, update: null }; // Don't overwrite
}
```

> `75c256b` — fix: resolve undefined lastMessage in Stop hook
> `4960e49` — fix: prevent PostToolUse race from overwriting Stop-derived states

---

### Cross-Pane Injection via tmux send-keys

**Problem:** Agents discovered they could use `tmux send-keys` to inject text into other tmux panes. When a user is typing in pane 1 and an agent in pane 2 runs `tmux send-keys -t 1 "some text"`, the injected text interleaves with user input.

**Why it's dangerous:** This isn't just annoying — an agent could inject commands into another agent's pane, creating a privilege escalation path. Agent A could tell Agent B to run destructive commands.

**What we tried first:** Built a pending-reply queue in the dashboard to buffer injected text and deliver it safely. Too complex, too many edge cases.

**Fix:** Block `tmux send-keys` in the destructive command hook. Agents must ask the user to run cross-pane commands manually.

```javascript
{ pattern: /\btmux\s+send-keys\b/, label: 'tmux send-keys (cross-pane injection)' }
```

> `ebbbc8b` — fix: block tmux send-keys in destructive command hook

---

## The Phantom Keystroke Epidemic

### Terminal Escape Sequences Fragment Into Ghost Keypresses

**Problem:** Tmux mouse events are encoded as multi-byte escape sequences (e.g., `\x1b[M...`). When the terminal fragments these across TCP packets, Bubble Tea's input parser receives partial sequences. The trailing bytes are interpreted as regular keypresses.

**Impact:** A mouse click could produce phantom `x` (dismiss agent), `enter` (confirm action), `y` (approve), or `m` (merge PR). Users would see agents dismissed or PRs merged without touching the keyboard.

**Evolution of the fix:**

1. **Scattered guards** (initial): Added per-key cooldown checks in 10+ places. `if time.Since(lastMouse) < 50ms { return }`. Worked but was unmaintainable — every new key binding needed its own guard.

2. **Centralized PhantomFilter** (final): Bubble Tea v2 added `tea.WithFilter`, a callback that intercepts *all* messages before they reach `Update`. Built a single filter with three cooldown tiers:

```go
const (
    escapeKeyCooldown = 50 * time.Millisecond   // after mouse/focus escape sequence
    modeResetCooldown = 100 * time.Millisecond  // after mode transition (Enter/Escape)
    confirmCooldown   = 300 * time.Millisecond  // after entering confirm mode
)

// Destructive keys are guarded; navigation keys (j/k/arrows) are not
var phantomGuardedKeys = map[string]bool{
    "x": true, "enter": true, "r": true, "m": true,
    "y": true, "n": true,
    "1": true, "2": true, "3": true, /* ... */ "9": true,
}
```

3. **Mouse mode toggle**: Disable mouse tracking entirely during text input modes (reply, create folder/skill/message). No mouse events = no phantom source.

```go
switch m.mode {
case modeReply, modeCreateFolder, modeCreateSkill, modeCreateMessage:
    v.MouseMode = tea.MouseModeNone
default:
    v.MouseMode = tea.MouseModeCellMotion
}
```

> `70a07c5` — feat: centralize phantom keystroke defense with bubbletea v2 WithFilter

---

### Debug Key Log — Forensic Infrastructure

**Problem:** Phantom keystrokes are non-deterministic and impossible to reproduce reliably. Need a way to capture exactly what the terminal sent.

**Fix:** Optional key logger that records every event with timing data:

- Timestamp, current mode, key string, key code, rune hex value
- Mouse event age (time since last mouse event — the critical metric for phantom detection)
- Whether the key was swallowed by the phantom filter

Gated behind `[debug] key_log = true` in settings.toml. Writes to `~/.agent-dashboard/debug-keys.log`.

**How it helped:** When the dashboard "quit by itself", the key log showed zero keypresses before termination — only mouse events. This ruled out phantom keys entirely and redirected the investigation to OS-level process kills (AMFI/taskgated).

---

## Agent Lifecycle Issues

### Spawning Feedback Gap

**Problem:** After spawning an agent into a new tmux pane, there's a variable delay (1-15s) before the agent's first hook fires and creates a state file. During this gap, the dashboard shows nothing — user doesn't know if spawn succeeded.

**What we tried first:** Fixed 3-second spinner. Too short for slow shell startups (oh-my-zsh, nvm, conda init). Too long for fast startups.

**Fix:** Dynamic spinner that persists until the agent's state file appears on disk, with a 30-second hard timeout.

> `9685cfa` — fix: persist spawning spinner until agent appears and catch zsh startup errors

---

### Agent Duplication on Same Tmux Pane

**Problem:** Race condition during spawn: if two agents claim the same pane (reconnect, rapid spawn), the dashboard shows duplicates with conflicting state.

**Fix:** Periodic dedup in the pruning loop. Keep only the newest agent per pane. Added a safety net: refuse to remove *all* agents in a single prune cycle (protects against transient tmux connectivity issues reporting all panes as dead).

> `3cd721b` — fix: add periodic dedup to PruneDead for agents sharing the same tmux pane

---

## Architecture & Patterns

### Two-Tier Hook Reporting

Not all state updates need the same fidelity. Split into:

| Tier | Events | Data | Target Latency |
|------|--------|------|----------------|
| Fast | PreToolUse, PostToolUse, PermissionRequest | state, current_tool, permission_mode | <100ms |
| Full | SessionStart, Stop, SubagentStart/Stop | + git branch, files changed, tmux capture, model, preview | <5s |

Dashboard shows permission states instantly (fast tier). Rich context loads on lifecycle boundaries (full tier).

---

### Pinned States Survive Hook Overwrites

Some states are user-driven, not hook-driven. When a PR is created or a branch is merged, that state should persist even if subsequent hooks report "running" or "idle".

```go
func (a *Agent) EffectiveState() string {
    if a.PinnedState != "" {
        return a.PinnedState
    }
    return a.State
}
```

Pinned states (`pr`, `merged`) are set by the `pr-detect.js` hook. The fast hook defers to pinned states on incoming updates, and they're cleared by user action in the dashboard.

---

### State Priority for Multi-Agent Sorting

Agents are sorted by attention priority — blocked agents float to top:

```go
StatePriority = map[string]int{
    "permission":  1,  // Blocked — needs y/n approval
    "plan":        1,  // Blocked — plan review
    "question":    2,  // Waiting — needs user reply
    "error":       2,  // Waiting — needs investigation
    "running":     3,  // Active
    "idle_prompt": 4,  // Finished turn
    "done":        4,  // Finished task
    "pr":          5,  // PR created
    "merged":      6,  // Merged
}
```

---

### Rendering Cache: Three Layers

Re-rendering conversation history (markdown parsing, word wrapping, syntax highlighting) on every tick is expensive. Cache at three levels:

1. **Raw data**: Conversation entries in memory, incrementally parsed from JSONL
2. **Rendered content**: Cached markdown output with a three-branch strategy: identical state returns cached output, new entries with same width appends only the new content, width change or entry removal triggers full re-render
3. **File offset**: Only parse new JSONL lines (seek to last offset, parse forward)

```go
// Three-branch cache: identical → return, grew same width → append, else → full re-render
if len(m.conversation) == m.historyConvLen && m.rightWidth == m.historyRightWidth {
    // No change — return cached
} else if len(m.conversation) > m.historyConvLen && m.rightWidth == m.historyRightWidth {
    // Append only new entries
} else {
    // Full re-render (width changed or entries removed)
}
```

---

### Incremental JSONL Parsing

Claude Code sessions are stored as JSONL files that grow continuously. Re-reading the entire file on every poll is wasteful.

```go
func ReadConversationIncremental(
    projDir, sessionID string,
    limit int,
    prev []domain.ConversationEntry,
    prevOffset int64,
) ([]domain.ConversationEntry, int64) {
    // If file shrank (truncation): fall back to full re-read
    // Otherwise: seek to prevOffset, parse only new lines
}
```

Per-agent caches preserve conversation state and file offsets when switching between agents in the dashboard.

---

### Safety Hooks as Encoded Best Practices

Hooks enforce conventions that agents would otherwise drift from:

**Destructive command blocklist** (`warn-destructive.js`):
```javascript
const DESTRUCTIVE_PATTERNS = [
    { pattern: /\bgit\s+reset\s+--hard/,  label: 'git reset --hard' },
    { pattern: /\bgit\s+push\s+.*--force/, label: 'git push --force' },
    { pattern: /\bgit\s+clean\s+(-[^\s]*f[^\s]*|-f)\b/, label: 'git clean -f' },
    { pattern: /\bdrop\s+table/i,         label: 'DROP TABLE' },
    { pattern: /\btmux\s+send-keys/,      label: 'tmux send-keys' },
    // ... (abbreviated — see warn-destructive.js for full list of 11 patterns)
];
```

**Conventional commit linting** (`commit-lint.js`): Validates `<type>: <description>` format. Extracts messages from both `-m "..."` and heredoc patterns.

**Test gate before commit** (`test-gate.js`): Runs `make test-fast` or `make test` before allowing any commit. Bypassable with `SKIP_TEST_GATE=1` for docs-only changes.

**Main branch protection** (`block-main-commit.js`): Prevents commits on main/master. Handles worktrees correctly by parsing `cd` commands to detect effective working directory.

---

### Hook Error Handling Philosophy

Hooks must never break Claude Code. Every hook follows this pattern:

```javascript
const MAX_STDIN = 1024 * 64; // 64KB guard against zip bombs
let data = '';
process.stdin.setEncoding('utf8');
process.stdin.on('data', chunk => {
    if (data.length < MAX_STDIN) data += chunk.substring(0, MAX_STDIN - data.length);
});
process.stdin.on('end', () => {
    try {
        const input = data.trim() ? JSON.parse(data) : {};
        // ... actual logic
    } catch {
        // Silent — don't break Claude Code
        process.exit(0); // Always pass through on error
    }
});
```

- Exit code 2 = block the tool call (only for destructive ops and validation failures)
- Exit code 0 = tool call proceeds (default on any error)
- All exceptions are caught and silenced

---

### Gate-Based Skill Workflows

Skills (feature, fix, refactor) enforce disciplined development through phase gates. Each phase has a condition that must be satisfied before proceeding:

```
Phase 1: Setup     → Gate: worktree on correct branch, env files present
Phase 2: Plan      → Gate: user approved approach in plan mode
Phase 3: Implement → Gate: RED (failing test) → GREEN (passing) → REFACTOR (clean)
Phase 4: Review    → Gate: no critical issues
Phase 5: Deliver   → Gate: clean conventional commits
Phase 6: Cleanup   → Gate: branch merged, worktree removed
```

The RED→GREEN→REFACTOR cycle isn't optional — the gate requires showing failing test output before writing implementation code. This forces agents to write testable code and prevents "write everything then backfill tests" patterns.

---

## Key Takeaways

1. **macOS kills unsigned binaries silently.** If you build Go/Rust binaries and run them on macOS, ad-hoc codesign them. Sleep/wake cycles will kill them otherwise.

2. **Redirect stderr before entering alternate screen.** Bubble Tea (and any TUI framework) hides stderr. Redirect to a file first, or you'll never see panics or OS-level kill signals.

3. **Terminal escape sequences fragment.** Mouse events arrive as multi-byte sequences that can split across reads. Guard destructive key bindings with cooldown timers. Centralize the defense in one filter, not scattered across handlers.

4. **Hooks fire far more often than you think.** PreToolUse + PostToolUse on every tool call, across all concurrent agents. Design hooks in two tiers: fast (state-only) and full (rich context on lifecycle boundaries).

5. **Async hook ordering is not guaranteed.** A Stop hook can be overwritten by a late-arriving PostToolUse. Guard terminal states against overwrites from earlier lifecycle events.

6. **Block cross-pane injection.** If agents can send keystrokes to other tmux panes, they will. This is a security issue, not just a UX annoyance.

7. **Cache rendered content, not just data.** Markdown rendering and word wrapping are expensive. Cache at multiple levels with clear invalidation conditions.

8. **Enforce process via gates, not guidelines.** Agents drift from best practices. Hard gates (failing test required before implementation, conventional commits enforced by hook) work better than instructions.
