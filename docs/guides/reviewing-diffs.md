---
title: Reviewing Diffs
parent: Guides
nav_order: 3
---

# Reviewing Diffs

Press `d` on any agent to open the diff viewer — a GitHub-style split-pane view with syntax highlighting.

---

## What it shows

The diff viewer compares the merge-base of the agent's branch against HEAD, showing exactly what the agent has changed. Files are colour-coded:

- **Green** — new files
- **Red** — deleted files
- **Yellow** — modified files

## Navigation

| Key | Action |
|:----|:-------|
| `j` / `k` or arrows | Select file in the file list |
| `J` / `K` (shift) | Scroll diff content (single line) |
| `Ctrl+U` / `Ctrl+D` | Scroll diff content (half page) |
| `g` / `G` | Jump to first / last file |
| `{` / `}` | Jump file list by half page |
| `Enter` or `Space` | Toggle directory expand/collapse |
| `/` | Filter files by name |
| `e` | Expand/collapse all context |
| `d`, `q`, or `Esc` | Exit diff viewer |

## Smart context collapsing

Large diffs automatically collapse unchanged regions. The viewer keeps **sticky function headers** visible at the top of the scroll area so you always know which function you're reading, even deep into a long diff.

## Opening a diff without an agent

The diff viewer is also accessible through the PR workflow. Press `g` on an agent that has an open PR to view the PR diff directly.
