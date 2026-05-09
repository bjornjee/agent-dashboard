---
title: Reviewing Diffs
parent: Guides
nav_order: 3
---

# Reviewing Diffs

Press `d` on any agent to open the diff viewer — a GitHub-style split-pane view with syntax highlighting.

![Diff viewer screenshot](https://github.com/user-attachments/assets/65e972e1-bd8b-4fdf-bb27-d1a0df6b1c4f)

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

## Mermaid diagram viewer

The `mermaid-extractor` adapter hook scans agent messages for fenced ` ```mermaid ` blocks and stores them per session. When the focused agent has at least one diagram, press `D` to toggle the diagrams panel. Inside the panel:

- `j` / `k` cycle between diagrams (the source preview re-renders on each move)
- `Enter` writes a temp HTML file and opens it in your default browser, where mermaid.js renders the diagram — useful for sequence diagrams and flowcharts that are unreadable as ASCII
- `x` deletes the current diagram (with `y`/`n` confirmation)
- `Esc` closes the panel
