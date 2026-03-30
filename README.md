# agent-dashboard

A tmux-integrated TUI for monitoring Claude Code agents across sessions, built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and styled with Catppuccin Frappe.

https://github.com/user-attachments/assets/PLACEHOLDER

Reads agent state from `~/.claude/agent-dashboard/state.json` (written by the [bjornjee-skills](https://github.com/bjornjee/skills) Claude Code plugin).

## Features

- **Real-time agent monitoring** — agents grouped by state (needs attention, running, completed) with live tmux pane capture
- **Subagent tree** — expand/collapse/dismiss subagent nodes under parent agents
- **Conversation history** — timestamped messages with role-based colouring
- **File change tracking** — colour-coded additions, removals, and modifications
- **Plan viewer** — glamour-rendered markdown plans with syntax highlighting
- **Usage dashboard** — per-agent token breakdown, 7-day cost chart, cumulative totals
- **Session creation** — create new agent sessions with z-plugin path autocomplete
- **Quick reply** — send text or y/n responses directly to agent panes
- **Desktop notifications** — alerts when agents need attention (input/error state)
- **Daily quote** — fetched from API Ninjas with fallback to embedded quotes
- **Pixel art banner** — axolotl rendered with half-block Unicode characters

## Install

```bash
# Clone into tmux plugins directory
git clone https://github.com/bjornjee/agent-dashboard ~/.tmux/plugins/agent-dashboard

# Build the binary (requires Go 1.26+)
~/.tmux/plugins/agent-dashboard/scripts/install.sh

# Add to ~/.tmux.conf
run-shell ~/.tmux/plugins/agent-dashboard/agent-dashboard.tmux

# Reload tmux config
tmux source-file ~/.tmux.conf
```

## Usage

Press `prefix + D` to switch to the dashboard session. On first press it creates a dedicated `dashboard` tmux session; subsequent presses switch back to it.

Or run directly:

```bash
~/.tmux/plugins/agent-dashboard/bin/agent-dashboard
```

### Keybindings

| Key | Action |
|-----|--------|
| `j/k` or `arrows` | Navigate agent list |
| `Shift+Up/Down` | Jump to next parent agent |
| `Enter` | Jump to agent's tmux pane |
| `Tab/Shift+Tab` | Cycle focus between panels |
| `Ctrl+U/D` | Scroll focused panel |
| `r` | Reply to agent |
| `y/n` | Quick answer (when agent awaits input) |
| `e` | Open VS Code in agent's directory |
| `a` | Create new agent session |
| `c` | Collapse/expand subagent tree |
| `x` | Dismiss subagent or close agent pane |
| `p` | Toggle plan view |
| `u` | Toggle usage dashboard |
| `q` | Quit |

## Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `API_NINJAS_KEY` | API key for quote-of-the-day | No (falls back to built-in quotes) |

## Development

```bash
make build    # Build binary to bin/
make test     # Run tests
make install  # Copy to ~/.local/bin
make seed     # Create fake agent state for testing
make clean    # Remove build artifacts and state
```

## Companion Plugin

This dashboard reads state written by the [bjornjee-skills](https://github.com/bjornjee/skills) Claude Code plugin. Install both for the full experience:

```
/plugin marketplace add bjornjee/skills
```
