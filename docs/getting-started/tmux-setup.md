---
title: tmux Setup
parent: Getting Started
nav_order: 2
---

# tmux Setup

The dashboard runs inside tmux and captures live output from agent panes. This page covers the optional keybinding that makes switching to the dashboard instant.

---

## Keybinding: `prefix + D`

The included `agent-dashboard.tmux` script binds `prefix + D` (capital D) to switch to a dedicated dashboard session. If the session doesn't exist, it creates one.

Add this to your `~/.tmux.conf`:

```bash
run-shell /path/to/agent-dashboard/agent-dashboard.tmux
```

Then reload your tmux config:

```bash
tmux source-file ~/.tmux.conf
```

Now press `prefix + D` from any tmux session to jump to your dashboard.

{: .tip }
If you installed via the installer script, the tmux plugin file is at `~/.local/share/agent-dashboard/agent-dashboard.tmux`.

## How it works

The dashboard reads agent state from per-agent JSON files in `~/.agent-dashboard/agents/`. These files are written by the Claude Code adapter hooks whenever agent state changes — new messages, tool calls, permission requests, and completion events.

The TUI polls these files and also uses `fsnotify` for instant updates. It captures live tmux pane content to show what each agent is currently outputting.

## Recommended tmux settings

The dashboard works with default tmux settings, but these options improve the experience:

```bash
# Increase scrollback for better pane capture
set-option -g history-limit 10000

# Enable mouse support for scrolling panels
set-option -g mouse on
```
