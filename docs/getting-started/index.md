---
title: Getting Started
nav_order: 2
has_children: true
---

# Getting Started

Get agent-dashboard running in five minutes.
{: .fs-6 .fw-300 }

---

## Prerequisites

| Dependency | Required | Purpose |
|:-----------|:---------|:--------|
| [tmux](https://github.com/tmux/tmux) | Yes | Agent pane management and live capture |
| [Claude Code](https://claude.com/claude-code) | Yes | The agents this dashboard monitors |
| [Node.js 18+](https://nodejs.org/) | Yes | Claude Code and Codex adapter hooks |
| [git](https://git-scm.com/) | Yes | Diff viewer, branch detection |
| [GitHub CLI (`gh`)](https://cli.github.com/) | No | Detects existing PRs so `g` opens the diff page instead of creating a new PR |
| [Codex CLI](https://developers.openai.com/codex/) 0.130+ | No | Show Codex sessions in the dashboard |
| [z (zsh plugin)](https://github.com/agkozak/zsh-z) | No | Frecency-ranked directory suggestions when creating sessions |

## Step 1: Install the binary

Download the pre-built binary from the latest [GitHub Release](https://github.com/bjornjee/agent-dashboard/releases):

```bash
curl -fsSL https://raw.githubusercontent.com/bjornjee/agent-dashboard/main/install.sh | sh
```

The installer downloads the binary for your platform, verifies its SHA256 checksum, and installs it to `~/.local/bin/agent-dashboard`. It also copies Codex dashboard hooks to `~/.codex/hooks/agent-dashboard` and copies `~/.codex/hooks.json` when that file does not already exist. No Go toolchain required.

### Build from source

If you prefer to build from source (requires [Go 1.26+](https://go.dev/dl/)):

```bash
git clone https://github.com/bjornjee/agent-dashboard
cd agent-dashboard
./install.sh --build
```

## Step 2: Register the plugin

In any Claude Code session, run:

```
/marketplace add bjornjee/agent-dashboard
/plugin install agent-dashboard@agent-dashboard
```

Then restart Claude Code sessions for hooks and skills to take effect.

### Recommended companion plugins

`agent-dashboard` works best when paired with [`bjornjee/skills`](https://github.com/bjornjee/skills) — a plugin of workflow skills (TDD guide, language-specific strict reviewers, refactor cleaner, codex delegation, terminal ops) that the dashboard's session-creation flow expects:

```
/marketplace add bjornjee/skills
/plugin install skills@bjornjee-skills
```

Without it, skill-gated session types (feature, fix, chore, refactor, investigate, implement, pr, rca) will not function as intended.

## Step 3: Codex CLI support

Codex support is installed by `install.sh`, not by editing the managed plugin cache. The installer only performs copy-if-missing actions: it copies the Codex hook bundle to `~/.codex/hooks/agent-dashboard` and copies the global hook template to `~/.codex/hooks.json` only when that file is absent.

After installing, restart Codex sessions and approve the `agent-dashboard` hooks prompt. Once approved, the dashboard sees Codex sessions like Claude sessions — same state file, same conversation panel, same cost dashboard.

If `~/.codex/hooks.json` already exists, the installer leaves it untouched. Review the template at `~/.codex/hooks/agent-dashboard/hooks.json` and reconcile it with your existing Codex hooks before restarting Codex.

## Step 4: Launch

```bash
agent-dashboard
```

You should see the dashboard with any running Claude Code or Codex agents listed. Try these first interactions:

- **`j` / `k`** — navigate the agent list
- **`Enter`** — jump to an agent's tmux pane
- **`d`** — view the git diff for an agent's work
- **`h`** — open the help overlay with all keybindings
- **`q`** — quit

{: .note }
If no agents appear, start a Claude Code or Codex session in tmux first. The dashboard watches for agent state files written by the adapter hooks.

## Uninstall

### Remove the plugin

In any Claude Code session:

```
/plugin uninstall agent-dashboard@agent-dashboard
/marketplace remove agent-dashboard
```

### Remove the binary and state

```bash
curl -fsSL https://raw.githubusercontent.com/bjornjee/agent-dashboard/main/uninstall.sh | sh
```

Or from a repo checkout: `make uninstall`. Pass `--yes` to skip the prompt for deleting `~/.agent-dashboard/` (which contains settings and usage data).
