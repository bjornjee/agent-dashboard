---
title: Adapter
parent: Reference
nav_order: 3
---

# Claude Code Adapter

The adapter is a Claude Code plugin that bridges between Claude Code sessions and the dashboard. It writes agent state files that the dashboard reads, and provides workflow skills that agents use.

---

## How it works

The adapter lives in `adapters/claude-code/` and consists of three components:

### Hooks

Lifecycle hooks run automatically during Claude Code sessions. They write agent state to `~/.agent-dashboard/agents/<session-id>.json` on every significant event.

| Hook | Trigger | Purpose |
|:-----|:--------|:--------|
| `agent-state-fast.js` | Every tool call | Fast state update (minimal parsing) |
| `agent-state-reporter.js` | State changes | Full state report with conversation data |
| `block-main-commit.js` | Before commit | Prevents direct commits to main |
| `commit-lint.js` | Before commit | Validates conventional commit format |
| `desktop-notify.js` | State changes | Sends desktop notifications |
| `pr-detect.js` | Before PR actions | Detects existing PRs for the branch |
| `test-gate.js` | Before merge | Blocks merge if tests fail |
| `warn-destructive.js` | Before destructive git ops | Warns about force pushes, resets, etc. |

### Skills

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

### Agents

Pre-configured agent definitions for specialized tasks:

| Agent | Purpose |
|:------|:--------|
| `build-error-resolver` | Fixes build and test errors with minimal diffs |
| `code-reviewer` | Reviews code for correctness, security, and conventions |
| `planner` | Creates phased implementation plans |
| `security-reviewer` | Detects security vulnerabilities and OWASP issues |
| `tdd-guide` | Enforces RED-GREEN-REFACTOR test-driven development |

## Agent state schema

Each agent's state is stored as a JSON file conforming to the schema at `schema/agent-state.schema.json`. The dashboard reads these files to populate its views. Key fields include:

- **`status`** — current agent state (running, waiting, blocked, completed, error)
- **`directory`** — working directory
- **`branch`** — git branch name
- **`conversation`** — path to the JSONL conversation log
- **`subagents`** — nested agent references

## Installing the adapter

```
/marketplace add bjornjee/agent-dashboard
/plugin install agent-dashboard@agent-dashboard
```

See [Getting Started](../getting-started) for the full installation walkthrough.
