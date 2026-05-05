# pi adapter for agent-dashboard

Bridges [`@mariozechner/pi-coding-agent`](https://www.npmjs.com/package/@mariozechner/pi-coding-agent) into the agent-dashboard tile state contract, so pi sessions appear in the dashboard alongside Claude Code sessions.

## What this adapter ships

- `extensions/agent-dashboard.js` — pi extension that subscribes to pi lifecycle events and writes the dashboard's per-agent JSON file (`~/.agent-dashboard/agents/<sid>.json`)
- Reuses the existing claude-code adapter packages (`agent-state`, `tmux`, `git-status`) and gate scripts (`warn-destructive`, `block-main-commit`, `test-gate`, `commit-lint`, `desktop-notify`) — no logic is reimplemented

## Install

```sh
make install-pi-adapter
```

This symlinks `extensions/agent-dashboard.js` into `~/.pi/agent/extensions/`. Pi auto-discovers it on next launch. Re-running the command refreshes the symlink.

## Use

Launch pi normally (any directory, any tmux pane). The extension fires automatically:

```sh
cd /your/worktree
pi
# In another terminal:
ls ~/.agent-dashboard/agents/
# An entry with agent_id="pi" appears, updated as the session progresses
```

Open the dashboard — the pi session renders as a tile.

## Supported events

| pi event | Behavior |
|---|---|
| `session_start` | Initialize agent JSON: target, model, cwd, agent_id="pi" |
| `tool_call` | Run safety gates (destructive, main-commit, test-gate). Block on exit code 2. |
| `tool_execution_start` | Set `current_tool`, `state: "running"` |
| `tool_execution_end` | Clear `current_tool`. On Bash: run `commit-lint`. |
| `agent_end` | Set `state: "done"` or `"idle_prompt"`; fire desktop notification |
| `session_shutdown` | Final state flush |

## What does NOT port from the Claude adapter

These have no pi equivalent today and are intentionally out of scope:

- **`codex-delegation-gate`** — pi has no `ExitPlanMode` event
- **`codex-write-gate`** — codex-specific
- **`mermaid-extractor`**, **`pr-detect`** — deferred (not required for tile-state parity)
- **Conversation panel rendering for pi** — the dashboard's JSONL parser is currently Claude-schema-only; pi tiles will show state/tools/notifications but the conversation viewer will be empty until a follow-up PR adds a pi parser

## Test

```sh
make test-pi-adapter
```

Runs `node --test` over `adapters/pi/test/*.test.js`. Tests mock `spawnSync` and `writeState` — no real pi process or filesystem mutation.
