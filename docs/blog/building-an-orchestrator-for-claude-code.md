---
title: Building an orchestrator for Claude Code
parent: Blog
nav_order: 1
date: 2026-05-04
---

# Building an orchestrator for Claude Code

*Posted on 2026-05-04 — what I learned about Claude Code's internals by dispatching my own agents.*
{: .fs-5 .fw-300 }

---

I run a handful of [Claude Code](https://claude.com/claude-code) agents in parallel most days. After a few weeks of context-switching between tmux panes — losing track of which agent was waiting on me, which one had silently failed, which one was about to commit straight to `main` — I built a thing to make the chaos legible. It's called [`agent-dashboard`](https://github.com/bjornjee/agent-dashboard). The name undersells it: it's a TUI **orchestrator and dispatcher** that spawns sessions, routes my replies, gates commits, enforces TDD, and runs a companion PWA so I can approve permissions from my phone.

What I didn't expect was how much I'd learn about Claude Code itself along the way. Skills, rules, hooks, plan mode, state files, async event ordering — most of these are documented somewhere, but you only really understand them when you're trying to build *on top of* them and hitting the edges. This post is a tour of those edges.

## Why I built it

The trigger was a small, dumb moment: I'd kicked off three agents on three feature branches, gone to make coffee, come back, and discovered all three had been waiting at a permission prompt for six minutes. None of them notified me. None of them showed up in any single window. I had to `tmux next-window` through them, one by one, like checking on roast chickens.

I tried writing a tiny shell script first. It didn't survive contact with reality — the moment you want subagent trees, plan-state detection, diff viewers, or anything time-ordered, you're building a TUI. So I picked [Bubble Tea](https://github.com/charmbracelet/bubbletea) and started over.

## Skills are the API I didn't know I had

The single most useful thing in Claude Code, in my opinion, is **skills**. A skill is a markdown file with phase-by-phase instructions that the model loads when you type `/<skill-name>`. The dashboard ships seven of them — `feature`, `fix`, `chore`, `refactor`, `investigate`, `pr`, `rca` — and they do most of the heavy lifting in keeping agents on-process.

What surprised me is how much of "good agentic engineering" turns out to be **gates**, not instructions. My `feature` skill has six phases: setup, plan, implement, review, deliver, cleanup. Each phase has a hard gate — a condition that must be visibly satisfied before the model is allowed to move on. Phase 3 is the one I care about most:

> The failing test run must be **pasted**, not paraphrased. "I assume it would fail" is not RED. A compile error is not RED — fix it and re-run until you get a real assertion failure.

Telling the model "do TDD" doesn't work. Telling it "you cannot proceed past this gate without pasting failing test output" does. Skills are how you encode that.

## Rules: doctrine the model re-reads on every turn

Skills fire on demand. Rules are the doctrine that loads on every turn, no matter what skill is active. Mine live in `~/.claude/rules/core.md` and contain things like "no Edit on a fix until you have pasted the offending file:line range", "spawn the Plan agent for any non-trivial implementation", and a dispatch table mapping triggers to subagents.

The reason this works — when the same advice in a CLAUDE.md hint somewhere often doesn't — is that rules are positionally guaranteed to be in context. There is no "I forgot to load it." The model literally re-reads them every turn. If I want a behaviour to be load-bearing, I put it in rules. If I want it to be situational, I put it in a skill.

The lesson generalises: **memory + on-demand loading is structurally different from in-context doctrine.** They serve different purposes.

## Hooks: how you teach Claude conventions without trusting it to remember

The dashboard registers nine hooks across the Claude Code lifecycle. Some of them are state reporters (so the dashboard knows what each agent is doing); the rest are **safety gates**:

- `block-main-commit.js` — refuses commits to `main`/`master`. Handles worktrees by parsing `cd` commands to figure out the effective working directory.
- `commit-lint.js` — validates conventional commit format on every `git commit`. Catches both `-m "..."` and heredoc patterns.
- `test-gate.js` — runs `make test` before allowing any commit. Bypassable with `SKIP_TEST_GATE=1` for docs.
- `warn-destructive.js` — pattern-matches `git reset --hard`, `git push --force`, `git clean -f`, `DROP TABLE`, and a few others. Blocks them.

Two things I had to learn the hard way:

**Hooks fire much more often than you think.** Pre/PostToolUse fire on every tool call. With five concurrent agents each making ten tool calls a minute, that's hundreds of Node.js process spawns per minute. I split into two tiers — a fast hook (`agent-state-fast.js`, sub-100ms, no subprocesses) and a full reporter that only runs on lifecycle events. That single change cut hot-path overhead by about 80%.

**Hooks must never break Claude.** Every hook in the dashboard follows the same skeleton:

```javascript
process.stdin.on('end', () => {
  try {
    const input = data.trim() ? JSON.parse(data) : {};
    // ...actual logic
  } catch {
    process.exit(0); // pass through on any error
  }
});
```

Exit code 2 blocks the tool call. Exit 0 lets it through. Anything else and you'll silently break the user's session — which is *exactly* what happened to me once when a `Stop` hook crashed inside a `try` block: the hook never wrote state, agents got stuck showing "running" forever, and I spent two days chasing the wrong bug.

The most surprising hook story was [cross-pane injection](https://github.com/bjornjee/agent-dashboard/commit/ebbbc8b). If an agent runs `tmux send-keys -t <other-pane> "rm -rf /"`, it can effectively take control of another pane — including the user's keyboard input. That's a privilege escalation path between agents. The fix is one line in the destructive-command blocklist:

```javascript
{ pattern: /\btmux\s+send-keys\b/, label: 'tmux send-keys (cross-pane injection)' }
```

If you're running multiple agents on the same machine, you want this.

## State management is harder than the cockpit makes it look

Each agent writes its state to `~/.agent-dashboard/agents/<session-id>.json`. The dashboard reads those files and renders them. Sounds straightforward. Three things made it not.

**JSONL conversation logs grow forever.** Re-reading the entire file on every poll is wasteful, so I parse incrementally — seek to the last byte offset, parse forward, append to an in-memory list. Per-agent caches preserve the offset when I switch agents in the dashboard.

**Hook ordering is async and not guaranteed.** The cleanest example: an agent finishes its turn, the `Stop` hook fires and correctly sets state to `idle_prompt`. But the *previous* tool's `PostToolUse` hook is still in flight. It arrives a few hundred ms later and overwrites the state back to `running`. Now my dashboard shows "running" for an agent that's actually waiting at a prompt, and I never know to reply. I fixed this with a stop-state guard — once a state is in `{idle_prompt, done, question}`, only the next `PreToolUse` (a new turn) is allowed to transition out:

```javascript
const STOP_STATES = new Set(['idle_prompt', 'done', 'question']);
if (hookEvent === 'PostToolUse' && STOP_STATES.has(existing.state)) {
  return { changed: false, update: null };
}
```

**Some states are user-driven, not hook-driven.** When you create a PR or merge it, you want that state to persist even if subsequent hooks report something else. I added a `PinnedState` field that takes priority over the hook-reported state, cleared only by an explicit user action.

These three patterns — incremental parsing, ordering guards, pinned overrides — are what makes the orchestrator feel like a stable surface on top of an inherently noisy event stream.

## The phantom keystroke epidemic

This one took me weeks to figure out, and it is my favourite war story.

Tmux mouse events are encoded as multi-byte ANSI escape sequences (`\x1b[M...`). When the terminal fragments them across reads — which it does, often — Bubble Tea's input parser sees a partial sequence followed by trailing bytes that look like regular keypresses. The result: a mouse click could spontaneously produce phantom `x` (dismiss agent), `enter` (confirm), `y` (approve), or `m` (merge PR). Imagine watching a PR get merged while your hands are nowhere near the keyboard.

I went through three escalating fixes:

1. **Scattered cooldown guards** — `if time.Since(lastMouse) < 50ms { return }` in 10+ places. Worked but unmaintainable.
2. **Centralised PhantomFilter** — Bubble Tea v2's `tea.WithFilter` lets you intercept all messages before they reach the update loop. One filter, three cooldown tiers (50ms after escape sequence, 100ms after mode transition, 300ms after entering confirm mode), guarded only the destructive keys.
3. **Mouse mode toggling** — disable mouse tracking entirely during text-input modes. No mouse events = no phantom source. This last fix is the one I'd reach for first if I did it again.

The real lesson, though, came from the **debug key log**. When the dashboard was dying mysteriously, I assumed phantom keypresses were the cause. Built a guard for the `q` key. Dashboard still died. Then I added an opt-in key logger that recorded every event with timing. The log showed *zero keystrokes* before death — only mouse events. Phantom keys weren't the cause. The next section is.

## macOS killed my binary three different ways

Once the key log ruled out phantom keys, I went looking for OS-level signals. The dashboard was being SIGKILL'd. Three separate ways:

1. **AMFI on test binaries.** `go test -race` requires CGO, which produces an unsigned binary. macOS Apple Mobile File Integrity rejects unsigned binaries running in temp directories with SIGKILL — no warning, no log, can't be caught. The cascade: SIGKILL on the test binary kills the parent Claude Code session, tmux detects no clients, `exit-empty` shuts the whole tmux server down, all running agents die. Fix: `CGO_ENABLED=0 go test ./...` for normal runs, separate `test-race` target for CI.

2. **`taskgated` on sleep/wake.** Same root cause, different vector. Closing the laptop and re-opening it killed the dashboard binary because Go's default build output has no code signature. Fix: ad-hoc codesign:

   ```makefile
   build:
   	go build $(LDFLAGS) -o bin/agent-dashboard ./cmd/dashboard/
   	@if [ "$$(uname)" = "Darwin" ]; then codesign -s - bin/agent-dashboard; fi
   ```

   No Apple Developer cert required. `codesign -s -` is the magic.

3. **The alternate screen hides stderr.** None of the above was *visible* until I made one change: redirect `os.Stderr` to a crash log file *before* entering Bubble Tea's alt screen. Add signal handlers to log SIGHUP/SIGTERM/SIGQUIT. Suddenly every crash was debuggable.

> When a TUI owns the alternate screen, stderr is invisible. Always redirect it to a file before entering alt screen.

This is the single most useful piece of advice I have for anyone building a TUI.

## What surprised me most

Three things, ordered by how strongly I'd evangelise them:

1. **Hard gates beat guidelines.** Every time. Skills with phase gates that refuse to advance work. CLAUDE.md hints that say "please do X" don't.
2. **Async hook ordering is not guaranteed.** Late events overwrite earlier ones. Guard your terminal states.
3. **Cache rendered output, not just data.** Markdown rendering and word-wrapping at 60Hz will eat your CPU. Three-tier cache — raw data, rendered string, file offset — keeps the UI smooth even with five concurrent agents.

## What's next

The Codex adapter is already shipped — the orchestrator now tracks Claude and Codex usage side by side, and gates Codex delegations through a `codex-write-gate.js` hook so it always runs in `--write` mode inside worktrees. Next up: more skills (I want a `triage` skill that ingests an issue and produces a plan), better mobile companion polish, and probably a Cursor adapter if I can find a clean integration point.

If you're running multiple coding agents and feeling the same chaos I was, [`agent-dashboard`](https://github.com/bjornjee/agent-dashboard) is on GitHub. The install is one curl + two slash commands. Open an issue if anything blows up — there's an even chance it's a bug I haven't found yet.
