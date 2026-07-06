---
title: Adapter
parent: Reference
nav_order: 3
---

# Agent Adapters

Agent adapters bridge coding-agent sessions and the dashboard. They write agent state files that the dashboard reads, and provide workflow skills that agents use.

The project ships two adapters:

- `adapters/claude-code/` — Claude Code plugin adapter with hooks, skills, shared hook packages, and Claude subagent definitions.
- `adapters/codex/` — Codex plugin adapter with a `.codex-plugin` manifest, plugin-local hooks, and Codex-flavored workflow skills.

Hooks ship inside the plugin bundles themselves — `install.sh` installs the binary only and does not write into `~/.codex`. Register each adapter via its host's plugin marketplace (see below).

---

## How it works

### Claude adapter

The Claude adapter lives in `adapters/claude-code/` and consists of three components:

#### Hooks

Lifecycle hooks run automatically during Claude Code sessions. They write agent state to `~/.agent-dashboard/agents/<session-id>.json` on every significant event.

| Hook | Trigger | Purpose |
|:-----|:--------|:--------|
| `agent-state-fast.js` | Every tool call | Fast state update (minimal parsing) |
| `agent-state-reporter.js` | State changes | Full state report with conversation data |
| `block-main-commit.js` | Before commit | Prevents direct commits to main |
| `commit-lint.js` | Before commit | Validates conventional commit format |
| `desktop-notify.js` | State changes | Sends desktop notifications |
| `mermaid-extractor.js` | After messages | Extracts mermaid diagram blocks for the diagram viewer |
| `codex-delegation-gate.js` | Before Codex delegation | Enforces delegation rules for skills |
| `codex-write-gate.js` | Before Codex writes | Ensures Codex uses `--write` flag in worktrees |
| `pr-detect.js` | Before PR actions | Detects existing PRs for the branch |
| `pr-skill-detect.js` | Every user prompt | Detects `/agent-dashboard:pr` invocations and pins agent state to `pr` |
| `pr-skill-gate.js` | Before `gh pr create` | Blocks direct PR creation outside the `pr` skill |
| `test-gate.js` | Before merge | Blocks merge if tests fail |
| `warn-destructive.js` | Before destructive git ops | Warns about force pushes, resets, etc. |

Two more scripts ship alongside the hooks: `claim-worktree.js`, a utility skills run to claim a freshly created worktree for the current pane, and `effort-config.js`, a shared module that reads the `[effort]` settings for `agent-state-fast.js`.

#### Skills

Workflow skills are prompted routines that guide agents through specific tasks:

| Skill | Purpose |
|:------|:--------|
| `feature` | Feature development with TDD in isolated git worktrees |
| `fix` | Bug fix with root cause analysis |
| `chore` | Non-code changes (docs, config, CI) |
| `refactor` | Code restructuring with test preservation |
| `investigate` | Research and analysis without code changes |
| `implement` | Execute an approved plan phase-by-phase with context isolation |
| `pr` | PR review and iteration |
| `rca` | Root cause analysis for incidents |

Skills for both adapters are generated from a single source tree, `adapters/skills-src/`, via `make gen-skills` — the per-adapter copies under `adapters/claude-code/skills/` and `adapters/codex/skills/` are build outputs and never hand-edited.

#### Agents

Pre-configured agent definitions for specialized tasks:

| Agent | Purpose |
|:------|:--------|
| `build-error-resolver` | Fixes build and test errors with minimal diffs |
| `code-reviewer` | Reviews code for correctness, security, and conventions |
| `planner` | Creates phased implementation plans |
| `security-reviewer` | Detects security vulnerabilities and OWASP issues |
| `tdd-guide` | Enforces RED-GREEN-REFACTOR test-driven development |

### Codex adapter

The Codex adapter lives in `adapters/codex/` and consists of three plugin-facing components:

- `.codex-plugin/plugin.json` — Codex plugin manifest for skills, hooks, and marketplace metadata.
- `hooks/plugin-hooks.json` — plugin-local hook definitions that run the bundled hook scripts through `${PLUGIN_ROOT}`.
- `skills/` — Codex-flavored workflow skills that invoke agent-dashboard skills with `$agent-dashboard:<skill>` syntax.

The `adapters/codex/hooks/` directory holds the same hook bundle the plugin ships to Codex when installed via `codex plugin marketplace add bjornjee/agent-dashboard`.

Codex-specific behavior:

- **Workflow skills are available for Codex sessions.** The dashboard skills use Codex-native commands where the harness differs, including `$agent-dashboard:implement` and `$agent-dashboard:rca`.
- **Plan mode is signaled, not gated.** Codex's `/plan` slash command flips the hook payload's `permission_mode` to `"plan"`. The dashboard captures this as a field but doesn't flip state to `plan` — Codex has no `ExitPlanMode` equivalent, so there's no discrete "plan ready" review moment.
- **Plan-required skills auto-enter plan mode at spawn.** Dashboard-created Codex sessions for plan-required skills (`feature`) launch without the CLI prompt positional — Codex would auto-submit it before plan mode is active. Instead, the dashboard waits for the composer to render and injects `/plan <skill prompt>` as a bracketed paste (Codex dispatches `/plan` inline args atomically: enters Plan mode and submits the prompt in one action). If injection fails, the skill's own "stop and ask the user to run `/plan`" gate is the fallback.
- **Subagent state comes from Codex lifecycle hooks.** Codex sessions use `spawn_agent`/`wait_agent`; the dashboard tracks those sessions through the Codex hook events emitted by the plugin.

## Agent state schema

Each agent's state is stored as a JSON file conforming to the schema at `schema/agent-state.schema.json`. The dashboard reads these files to populate its views. Key fields include:

- **`status`** — current agent state (running, waiting, blocked, completed, error)
- **`directory`** — working directory
- **`branch`** — git branch name
- **`conversation`** — path to the JSONL conversation log
- **`subagents`** — nested agent references

### Agent state model

The dashboard maps raw states to display groups, sorted by priority:

| Raw state | Display group | Priority | Description |
|:----------|:-------------|:---------|:------------|
| `permission` | BLOCKED | 1 | Waiting for tool permission |
| `plan` | BLOCKED | 1 | Plan review pending |
| `question` | WAITING | 2 | Agent asked a question |
| `error` | WAITING | 2 | Agent encountered an error |
| `running` | RUNNING | 3 | Actively executing |
| `idle_prompt` | REVIEW | 4 | Idle, waiting for user input |
| `done` | REVIEW | 4 | Session completed |
| `pr` | PR | 5 | Pull request created |
| `merged` | MERGED | 6 | Pull request merged |

**Pinned states:** `pr` and `merged` are sticky — once set, idle states (`idle_prompt`, `done`, `question`) restore back to the pinned value. Active states (`running`, `permission`) display through normally so the dashboard reflects live work.

**Group headers:** when a priority-1 or priority-2 group is homogeneous, the TUI renders a more specific label — `PLAN` or `PERMISSION` instead of `BLOCKED`, and `QUESTION` or `ERROR` instead of `WAITING`. Mixed groups keep the generic header.

## Installing the Claude Code adapter

```
/marketplace add bjornjee/agent-dashboard
/plugin install agent-dashboard@agent-dashboard
/plugin enable agent-dashboard@agent-dashboard
```

## Installing the Codex adapter

```bash
codex plugin marketplace add bjornjee/agent-dashboard
```

Then enable the plugin by appending the following to `~/.codex/config.toml` and restarting Codex:

```toml
[plugins."agent-dashboard@agent-dashboard"]
enabled = true
```

From a repo checkout, `make install-codex-adapter` runs the marketplace add step and prints the same config snippet.

See [Getting Started](../../getting-started/) for the full installation walkthrough.
