# agent-dashboard

A tmux-integrated TUI for monitoring Claude Code agents across sessions.

Reads agent state from `~/.claude/agent-dashboard/state.json` (written by the [bjornjee-skills](https://github.com/bjornjee/skills) Claude Code plugin).

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

## Features

- Real-time agent state monitoring (input, running, done, error)
- Click agents to jump to their tmux pane
- Quote-of-the-day banner
- Usage stats via SQLite

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
