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

`install.sh` also syncs the Codex global hook bundle from `adapters/codex/hooks/` to `~/.codex/hooks/agent-dashboard` for non-plugin installs and upgrades.

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
| `test-gate.js` | Before merge | Blocks merge if tests fail |
| `warn-destructive.js` | Before destructive git ops | Warns about force pushes, resets, etc. |

#### Skills

Workflow skills are prompted routines that guide agents through specific tasks:

| Skill | Purpose |
|:------|:--------|
| `feature` | Feature development with TDD in isolated git worktrees |
| `fix` | Bug fix with root cause analysis |
| `chore` | Non-code changes (docs, config, CI) |
| `refactor` | Code restructuring with test preservation |
| `investigate` | Research and analysis without code changes |
| `pr` | PR review and iteration |
| `rca` | Root cause analysis for incidents |

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

The `adapters/codex/hooks/` directory also contains the global hook bundle that `install.sh --sync-adapters` can sync for non-plugin installs.

## Agent state schema

Each agent's state is stored as a JSON file conforming to the schema at `schema/agent-state.schema.json`. The dashboard reads these files to populate its views. Key fields include:

- **`status`** — current agent state (running, waiting, blocked, completed, error)
- **`directory`** — working directory
- **`branch`** — git branch name
- **`conversation`** — path to the JSONL conversation log
- **`subagents`** — nested agent references

## Installing the Claude Code adapter

```
/marketplace add bjornjee/agent-dashboard
/plugin install agent-dashboard@agent-dashboard
```

## Installing the Codex adapter

```
make install-codex-adapter
```

The target registers `.agents/plugins/marketplace.json`, whose `agent-dashboard` entry points at `./adapters/codex`. Enable `agent-dashboard@agent-dashboard` in `~/.codex/config.toml`, then restart Codex and approve the hooks prompt.

See [Getting Started](../../getting-started/) for the full installation walkthrough.
