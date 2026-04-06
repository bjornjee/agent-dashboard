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
| [Node.js 18+](https://nodejs.org/) | Yes | Claude Code adapter hooks |
| [git](https://git-scm.com/) | Yes | Diff viewer, branch detection |
| [GitHub CLI (`gh`)](https://cli.github.com/) | No | Detects existing PRs so `g` opens the diff page instead of creating a new PR |
| [z (zsh plugin)](https://github.com/agkozak/zsh-z) | No | Frecency-ranked directory suggestions when creating sessions |

## Step 1: Install the binary

Download the pre-built binary from the latest [GitHub Release](https://github.com/bjornjee/agent-dashboard/releases):

```bash
curl -fsSL https://raw.githubusercontent.com/bjornjee/agent-dashboard/main/install.sh | sh
```

The installer downloads the binary for your platform, verifies its SHA256 checksum, and installs it to `~/.local/bin/agent-dashboard`. No Go toolchain required.

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

## Step 3: Launch

```bash
agent-dashboard
```

You should see the dashboard with any running Claude Code agents listed. Try these first interactions:

- **`j` / `k`** — navigate the agent list
- **`Enter`** — jump to an agent's tmux pane
- **`d`** — view the git diff for an agent's work
- **`h`** — open the help overlay with all keybindings
- **`q`** — quit

{: .note }
If no agents appear, start a Claude Code session in tmux first. The dashboard watches for agent state files written by the adapter hooks.

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
