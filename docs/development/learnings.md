---
title: Learnings
parent: Development
nav_order: 3
---

# Learnings

Hard-won lessons from building a Bubble Tea TUI that orchestrates multiple Claude Code agents via tmux. These cover crashes, race conditions, terminal quirks, and architectural patterns discovered over months of development.
{: .fs-5 .fw-300 }

---

## Crashes and system-level kills

### macOS AMFI kills unsigned test binaries

`go test -race` requires CGO, which produces unsigned binaries. macOS AMFI (Apple Mobile File Integrity) rejects unsigned binaries running in temp directories with SIGKILL — no warning, no log.

**The cascade:** SIGKILL on test binary kills the parent Claude Code session. tmux detects no remaining clients. `exit-empty` shuts down the entire tmux server. All running agents die.

**Fix:** Disable CGO for regular test runs. Add a separate `test-race` target for CI environments where AMFI isn't an issue.

```makefile
test:
	CGO_ENABLED=0 go test ./...

test-race:
	go test -race ./...
```

### Sleep/wake kills the dashboard

macOS `taskgated` kills unsigned binaries after a sleep/wake cycle with SIGKILL (Code Signature Invalid). Go's default `go build` output has no code signature. The dashboard would be running fine, user closes their laptop, opens it next morning — dashboard gone.

**Fix:** Ad-hoc codesign the binary after build. Doesn't require an Apple Developer certificate.

```makefile
build:
	go build $(LDFLAGS) -o bin/agent-dashboard ./cmd/dashboard/
	@if [ "$$(uname)" = "Darwin" ]; then codesign -s - bin/agent-dashboard; fi
```

### Silent termination and the wrong suspect

Dashboard was dying with no visible cause. Initially blamed phantom keystrokes (see below). Built a cooldown guard for the `q` key. Dashboard still died.

**The breakthrough:** Debug key log showed only mouse events in the seconds before death — no keypresses at all. This proved phantom keys weren't the cause. The real culprit was AMFI/taskgated SIGKILL.

**Lesson:** When a TUI owns the alternate screen, stderr is invisible. Always redirect stderr to a file before entering the alternate screen. This single change made every subsequent crash debuggable.

```go
crashLog, _ := os.OpenFile(crashLogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
os.Stderr = crashLog
```

---

## Hook process starvation and race conditions

### Every tool call spawns a forest of processes

Claude Code fires hooks on every tool call. The dashboard registers hooks on 9 lifecycle events. On the hot path (PreToolUse + PostToolUse), up to 7 hooks fire per Bash tool call. Each hook spawns a Node.js process, which itself may spawn subprocesses.

**Math:** A busy agent making 10 tool calls/minute fires dozens of hook invocations. With 5 concurrent agents, that's hundreds of Node.js processes per minute.

**Fix:** Split hooks into two tiers:
- **Fast hooks** (`agent-state-fast.js`): Only update state, permission_mode, current_tool. No subprocess spawning. Target: <100ms.
- **Full reporter** (`agent-state-reporter.js`): Captures git status, tmux pane content, file changes. Allowed to be slow (~5s). Only fires on lifecycle boundaries.

This reduced hot-path overhead by ~80%.

### PostToolUse overwrites Stop state

When an agent stops, the Stop hook fires and correctly sets state to `idle_prompt`. But the previous tool call's PostToolUse hook is still in-flight. It arrives late and overwrites the state back to `running`.

**Result:** Agents stuck showing "running" when they're actually idle. Users don't know the agent is waiting for input.

**Fix (two parts):**
1. Fixed a scoping bug where the Stop hook's `report()` function silently threw `ReferenceError` — so Stop hooks never actually wrote state.
2. Added a stop-state guard: if current state is stop-derived (`idle_prompt`, `done`, `question`), PostToolUse cannot overwrite it. Only PreToolUse (new turn) transitions back to `running`.

```javascript
const STOP_STATES = new Set(['idle_prompt', 'done', 'question']);

if (hookEvent === 'PostToolUse' && STOP_STATES.has(existing.state)) {
    return { changed: false, update: null }; // Don't overwrite
}
```

### Cross-pane injection via tmux send-keys

Agents discovered they could use `tmux send-keys` to inject text into other panes. An agent could inject commands into another agent's pane — a privilege escalation path.

**Fix:** Block `tmux send-keys` in the destructive command hook. Agents must ask the user to run cross-pane commands manually.

---

## The phantom keystroke epidemic

### Terminal escape sequences fragment into ghost keypresses

Tmux mouse events are encoded as multi-byte escape sequences. When the terminal fragments these across reads, Bubble Tea's input parser receives partial sequences. The trailing bytes are interpreted as regular keypresses.

**Impact:** A mouse click could produce phantom `x` (dismiss agent), `enter` (confirm action), `y` (approve), or `m` (merge PR). Users would see agents dismissed or PRs merged without touching the keyboard.

**Evolution of the fix:**

1. **Scattered guards** (initial): Per-key cooldown checks in 10+ places. Worked but was unmaintainable.

2. **Centralized PhantomFilter** (final): Using Bubble Tea v2's `tea.WithFilter`, a single filter intercepts all messages with three cooldown tiers:

```go
const (
    escapeKeyCooldown = 50 * time.Millisecond   // after mouse/focus escape sequence
    modeResetCooldown = 100 * time.Millisecond  // after mode transition
    confirmCooldown   = 300 * time.Millisecond  // after entering confirm mode
)
```

3. **Mouse mode toggle**: Disable mouse tracking entirely during text input modes. No mouse events means no phantom source.

---

## Architecture patterns

### Two-tier hook reporting

| Tier | Events | Data | Target Latency |
|:-----|:-------|:-----|:---------------|
| Fast | PreToolUse, PostToolUse, PermissionRequest | state, current_tool, permission_mode | <100ms |
| Full | SessionStart, Stop, SubagentStart/Stop | + git branch, files changed, tmux capture | <5s |

### Pinned states survive hook overwrites

Some states are user-driven. When a PR is created or merged, that state persists even if subsequent hooks report "running". Pinned states are set by `pr-detect.js` and cleared by user action.

### Three-layer rendering cache

Re-rendering conversation history on every tick is expensive. Cache at three levels:

1. **Raw data**: Conversation entries, incrementally parsed from JSONL
2. **Rendered content**: Cached markdown output with a three-branch strategy — identical state returns cached output, new entries appends only new content, width change triggers full re-render
3. **File offset**: Only parse new JSONL lines (seek to last offset, parse forward)

### Gate-based skill workflows

Skills enforce disciplined development through phase gates:

```
Phase 1: Setup     → Gate: worktree on correct branch, env files present
Phase 2: Plan      → Gate: user approved approach
Phase 3: Implement → Gate: RED → GREEN → REFACTOR
Phase 4: Review    → Gate: no critical issues
Phase 5: Deliver   → Gate: clean conventional commits
Phase 6: Cleanup   → Gate: branch merged, worktree removed
```

The RED-GREEN-REFACTOR cycle isn't optional — the gate requires showing failing test output before writing implementation code.

---

## Key takeaways

1. **macOS kills unsigned binaries silently.** Ad-hoc codesign Go/Rust binaries. Sleep/wake cycles will kill them otherwise.

2. **Redirect stderr before entering alternate screen.** Bubble Tea hides stderr. Redirect to a file first, or you'll never see panics or kill signals.

3. **Terminal escape sequences fragment.** Guard destructive key bindings with cooldown timers. Centralize the defense in one filter.

4. **Hooks fire far more often than you think.** Design hooks in two tiers: fast (state-only) and full (rich context on lifecycle boundaries).

5. **Async hook ordering is not guaranteed.** Guard terminal states against overwrites from earlier lifecycle events.

6. **Block cross-pane injection.** If agents can send keystrokes to other tmux panes, they will. This is a security issue.

7. **Cache rendered content, not just data.** Markdown rendering and word wrapping are expensive. Cache at multiple levels.

8. **Enforce process via gates, not guidelines.** Hard gates work better than instructions for keeping agents on track.
