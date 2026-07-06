---
title: Creating Sessions
parent: Guides
nav_order: 2
---

# Creating Sessions

Press `a` in the dashboard to create a new agent session. The wizard walks through directory, harness, skill, model, thinking effort, and an optional first message.

---

## Directory selection

The session creator uses the [z zsh plugin](https://github.com/agkozak/zsh-z) to suggest directories ranked by frecency (frequency + recency). Start typing a path and suggestions appear automatically.

{: .note }
The z plugin is optional. Without it, you can type full paths manually.

## Harness and model selection

After choosing a directory, pick the harness that backs the session — Claude Code or Codex CLI (the default comes from `[harness]` in settings). The wizard then offers the harness's models (`fable`, `opus`, `sonnet`, `haiku` for Claude; `gpt-5.5` and friends for Codex) and a thinking-effort level, both of which override the settings defaults for this spawn only.

## Skill selection

Next, select a skill that determines the agent's workflow:

| Skill | Purpose |
|:------|:--------|
| **feature** | New feature development with TDD in an isolated git worktree |
| **fix** | Bug fix with root cause analysis |
| **chore** | Non-code changes (docs, config, CI) |
| **refactor** | Code restructuring with test preservation |
| **investigate** | Research and analysis without code changes |
| **implement** | Execute an approved plan phase-by-phase with context isolation |
| **pr** | PR review and iteration workflow |
| **rca** | Root cause analysis for incidents |

Each skill loads a specialized prompt that guides the agent through the appropriate workflow — including branching strategy, testing requirements, and delivery steps.

## Effort levels

The dashboard pins a Claude Code thinking-effort level on every spawned session and switches it dynamically as the agent works:

- Every dashboard-spawned session starts with `--effort {effort.default}` (default: `high`) — Claude Code pins this at the session level.
- When the agent enters plan mode (`EnterPlanMode`), the `agent-state-fast` adapter hook dispatches `/effort {effort.plan}` (default: `high`) into the pane and restores `default` on plan exit. With both keys set to `high` (the shipped default) the swap is a no-op; raise `effort.plan` to `max` if you want planning to think deeper than implementation.
- The `feature`, `fix`, and `refactor` skills additionally declare `effort: max` in their frontmatter. This applies only when the skill is invoked as a slash command (`/feature`, `/fix`, `/refactor`) inside an existing session — not when the dashboard launches a new agent with one of those skills selected.

Override the defaults in [`settings.toml`](../../reference/settings/) under the `[effort]` section, or per spawn in the wizard's effort step. Valid levels: `minimal`, `low`, `medium`, `high`, `max` (Codex clamps `max` to `high`).

## What happens next

The dashboard:

1. Creates a new tmux pane
2. Starts the chosen harness in the selected directory with the chosen skill, model, and effort
3. The adapter hooks begin writing agent state files
4. The agent appears in the dashboard list within seconds

## Resuming orphaned sessions

When tmux dies — a reboot or `tmux kill-server` — agent panes are gone but their session state survives. Orphaned sessions linger in the dashboard as resumable instead of being garbage-collected:

- **TUI** — press `/` to open the search palette, `Ctrl+O` to filter to orphans only, and `Enter` to resume the selected orphan in a fresh pane (`Enter` on a live agent jumps to its pane instead).
- **Web** — the Cmd+K palette marks resumable sessions with a pill and has an "orphaned only" toggle; selecting an orphan resumes it.

Resuming runs the harness's native resume (`claude --resume <session-id>` or `codex resume <session-id>`), so the conversation continues where it left off.
