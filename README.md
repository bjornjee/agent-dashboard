# agent-dashboard

A tmux-integrated TUI for monitoring Claude Code agents across sessions, built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and styled with Catppuccin Frappe.

https://github.com/user-attachments/assets/736b633f-93d7-4d83-8c9a-fd7ebc598a89

Reads agent state from per-agent JSON files in `~/.claude/agent-dashboard/agents/` (written by the [bjornjee-skills](https://github.com/bjornjee/skills) Claude Code plugin).

## Features

- **Real-time agent monitoring** — agents grouped by state (needs attention, running, completed) with live tmux pane capture
- **Subagent tree** — expand/collapse/dismiss subagent nodes under parent agents
- **Conversation history** — timestamped messages with role-based colouring, incremental JSONL parsing
- **Diff viewer** — GitHub-style split-pane diff with syntax highlighting, smart context collapsing, and sticky function headers
- **File change tracking** — colour-coded additions, removals, and modifications
- **Plan viewer** — glamour-rendered markdown plans with syntax highlighting
- **Usage dashboard** — per-agent token breakdown, 7-day cost chart, cumulative totals persisted to SQLite
- **Session creation** — create new agent sessions with z-plugin frecency-ranked path autocomplete
- **Quick reply** — send text, y/n, or numbered responses directly to agent panes
- **Desktop notifications** — alerts when agents need attention (input/error state)
- **Daily quote** — fetched from API Ninjas with fallback to embedded quotes
- **Pixel art banner** — axolotl rendered with half-block Unicode characters
- **Semantic versioning** — version injected at build time via `-ldflags`

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
| `Shift+Up/Down` | Jump to next parent agent (skip subagents) |
| `Enter` | Jump to agent's tmux pane |
| `Tab/Shift+Tab` | Cycle focus between panels |
| `Ctrl+U/D` | Scroll focused panel (half-page) |
| `r` | Reply to agent (free-text) |
| `y/n` | Quick answer (when agent awaits input) |
| `1`-`9` | Send number (for numbered selections) |
| `e` | Open VS Code in agent's directory |
| `d` | Show git diff (merge-base vs HEAD, syntax highlighted) |
| `a` | Create new agent session (z-plugin suggestions) |
| `c` | Collapse/expand subagent tree |
| `x` | Dismiss subagent or close agent pane |
| `p` | Toggle plan view |
| `u` | Toggle usage dashboard |
| `q` / `Ctrl+C` | Quit |

### Diff Viewer Keybindings

| Key | Action |
|-----|--------|
| `j/k` or `arrows` | Select file |
| `J/K` (shift) | Scroll diff content (single line) |
| `Ctrl+U/D` | Scroll diff content (half page) |
| `e` | Expand/collapse all context |
| `d` or `Esc` | Exit diff viewer |

## Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `API_NINJAS_KEY` | API key for quote-of-the-day | No (falls back to built-in quotes) |

## Development

```bash
make build    # Build binary to bin/ (injects version from VERSION file)
make test     # Run tests
make install  # Copy to ~/.local/bin
make seed     # Create fake agent state for testing
make clean    # Remove build artifacts and state
```

### Versioning

The project uses semantic versioning. The version is stored in the `VERSION` file at the repo root and injected into the binary at build time via Go's `-ldflags -X` mechanism. It is displayed in the dashboard banner (top right).

To bump the version, edit `VERSION` and rebuild.

### Project Structure

```
agent-dashboard/
├── Makefile
├── VERSION
├── agent-dashboard.tmux           # tmux plugin binding (prefix + D)
├── scripts/install.sh             # build + setup helper
├── cmd/
│   ├── dashboard/
│   │   ├── main.go                # entry point
│   │   ├── model.go               # Bubble Tea model + update loop
│   │   ├── view.go                # render logic (panels, layout)
│   │   ├── keys.go                # keybindings + mouse handling
│   │   ├── commands.go            # tea.Cmd functions (tmux, state)
│   │   ├── state.go               # agent state structs + file I/O
│   │   ├── conversation.go        # JSONL parsing, subagent discovery
│   │   ├── messages.go            # tea message types + constants
│   │   ├── diff.go                # git diff loading
│   │   ├── diff_view.go           # diff rendering + syntax highlighting
│   │   ├── usage.go               # token counting + pricing
│   │   ├── db.go                  # SQLite operations (usage + quotes)
│   │   ├── banner.go              # axolotl pixel art + quote display
│   │   ├── tmux.go                # tmux integration helpers
│   │   ├── zsuggest.go            # z-plugin frecency suggestions
│   │   ├── helpers.go             # text wrapping, markdown rendering
│   │   ├── styles.go              # Catppuccin Frappe theme
│   │   ├── version.go             # build-time version variable
│   │   └── *_test.go              # tests
│   └── populate-quotes/
│       └── main.go                # bulk quote fetcher for SQLite cache
└── go.mod / go.sum
```

### Key Dependencies

| Package | Purpose |
|---------|---------|
| [bubbletea](https://github.com/charmbracelet/bubbletea) | TUI framework |
| [bubbles](https://github.com/charmbracelet/bubbles) | Viewport, text input, spinner |
| [lipgloss](https://github.com/charmbracelet/lipgloss) | ANSI styling |
| [glamour](https://github.com/charmbracelet/glamour) | Markdown rendering |
| [chroma](https://github.com/alecthomas/chroma) | Syntax highlighting |
| [go-gitdiff](https://github.com/bluekeyes/go-gitdiff) | Git diff parsing |
| [sqlx](https://github.com/jmoiron/sqlx) | SQL query helper |
| [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) | Pure Go SQLite |
| [fsnotify](https://github.com/fsnotify/fsnotify) | File system watcher |

## Companion Plugin

This dashboard reads state written by the [bjornjee-skills](https://github.com/bjornjee/skills) Claude Code plugin. Install both for the full experience:

```
/plugin marketplace add bjornjee/skills
```
