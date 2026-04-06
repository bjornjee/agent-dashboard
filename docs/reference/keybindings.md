---
title: Keybindings
parent: Reference
nav_order: 1
---

# Keybindings

All keyboard shortcuts grouped by context. Press `h` in the dashboard to see this reference as an overlay.

---

## Main Dashboard

| Key | Action |
|:----|:-------|
| `j` / `k` or arrows | Navigate agent list |
| `Shift+Up` / `Shift+Down` | Jump to next parent agent (skip subagents) |
| `Enter` | Jump to agent's tmux pane |
| `Tab` / `Shift+Tab` | Cycle focus between panels |
| `Ctrl+U` / `Ctrl+D` | Scroll focused panel (half page) |
| `r` | Reply to agent (free-text) |
| `y` / `n` | Quick approve/reject (blocked or waiting agents) |
| `1`-`9` | Send numbered option to agent |
| `e` | Open editor in agent's directory |
| `d` | Show git diff (merge-base vs HEAD) |
| `g` | Open existing PR diff or create new PR |
| `m` | Merge PR via `gh` CLI and send cleanup |
| `a` | Create new agent session |
| `o` | Open dir in tmux window |
| `c` | Collapse/expand subagent tree |
| `C` | Collapse/expand status group |
| `x` | Dismiss subagent or close agent pane |
| `p` | Toggle plan view |
| `u` | Toggle usage dashboard |
| `h` | Show help overlay |
| `J` / `K` | Line scroll (plan/diff views) |
| `q` / `Ctrl+C` | Quit |

## Diff Viewer

| Key | Action |
|:----|:-------|
| `j` / `k` or arrows | Select file |
| `J` / `K` (shift) | Scroll diff content (single line) |
| `Ctrl+U` / `Ctrl+D` | Scroll diff content (half page) |
| `g` / `G` | Jump to first / last file |
| `{` / `}` | Jump file list by half page |
| `Enter` or `Space` | Toggle directory expand/collapse |
| `/` | Filter files by name |
| `e` | Expand/collapse all context |
| `d`, `q`, or `Esc` | Exit diff viewer |
