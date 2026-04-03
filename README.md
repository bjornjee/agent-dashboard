# agent-dashboard

A tmux-integrated TUI for monitoring Claude Code agents across sessions, built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and styled with Catppuccin Frappe.

https://github.com/user-attachments/assets/736b633f-93d7-4d83-8c9a-fd7ebc598a89

Reads agent state from per-agent JSON files in `~/.agent-dashboard/agents/` (written by the Claude Code adapter in `adapters/claude-code/`).

## Features

- **Real-time agent monitoring** — agents grouped by state (needs attention, running, completed) with live tmux pane capture
- **Subagent tree** — expand/collapse/dismiss subagent nodes under parent agents
- **Conversation history** — timestamped messages with role-based colouring, incremental JSONL parsing
- **Diff viewer** — GitHub-style split-pane diff with syntax highlighting, smart context collapsing, and sticky function headers
- **File change tracking** — colour-coded additions, removals, and modifications
- **Plan viewer** — glamour-rendered markdown plans with syntax highlighting
- **Usage dashboard** — per-agent token breakdown, 7-day cost chart, cumulative totals persisted to SQLite
- **Session creation** — create new agent sessions with z-plugin frecency-ranked path autocomplete
- **Quick reply** — send free-text responses directly to agent panes
- **GitHub PR shortcut** — open existing PR diff or create new PR in browser (uses `gh` when available)
- **Help overlay** — full-screen keybinding reference grouped by context
- **Daily quote** — fetched from API Ninjas with fallback to embedded quotes
- **Pixel art banner** — axolotl rendered with half-block Unicode characters
- **Semantic versioning** — version injected at build time via `-ldflags`

## Prerequisites

| Dependency | Required | Purpose |
|------------|----------|---------|
| [tmux](https://github.com/tmux/tmux) | Yes | Agent pane management and live capture |
| [Claude Code](https://claude.com/claude-code) | Yes | The agents this dashboard monitors |
| [Go 1.21+](https://go.dev/dl/) | Yes | Building the dashboard binary |
| [Node.js 18+](https://nodejs.org/) | Yes | Claude Code adapter hooks |
| [git](https://git-scm.com/) | Yes | Diff viewer, branch detection |
| [GitHub CLI (`gh`)](https://cli.github.com/) | No | Detects existing PRs so `g` opens the diff page instead of creating a new PR |
| [z (zsh plugin)](https://github.com/agkozak/zsh-z) | No | Frecency-ranked directory suggestions when creating sessions |

## Install

### Option A: Claude Code plugin (recommended)

In a Claude Code session, run:

```
/plugin add bjornjee/agent-dashboard
```

### Option B: From source

```bash
git clone https://github.com/bjornjee/agent-dashboard
cd agent-dashboard
make install                        # defaults to claude-code adapter
# make install ADAPTER=claude-code  # or specify explicitly
```

The installer builds the binary to `~/.local/bin/` and registers the adapter's hooks with the host tool (e.g. Claude Code settings).

### Optional: tmux keybinding

The included `agent-dashboard.tmux` script binds `prefix + D` to switch to a dedicated dashboard session:

```bash
# Add to ~/.tmux.conf
run-shell /path/to/agent-dashboard/agent-dashboard.tmux

# Reload tmux config
tmux source-file ~/.tmux.conf
```

## Usage

Run the dashboard directly:

```bash
agent-dashboard
```

Or if you set up the tmux keybinding, press `prefix + D` to switch to a dedicated dashboard session.

### Keybindings

| Key | Action |
|-----|--------|
| `j/k` or `arrows` | Navigate agent list |
| `Shift+Up/Down` | Jump to next parent agent (skip subagents) |
| `Enter` | Jump to agent's tmux pane |
| `Tab/Shift+Tab` | Cycle focus between panels |
| `Ctrl+U/D` | Scroll focused panel (half-page) |
| `r` | Reply to agent (free-text) |
| `e` | Open VS Code in agent's directory |
| `d` | Show git diff (merge-base vs HEAD, syntax highlighted) |
| `g` | Open existing PR diff or create new PR in browser |
| `a` | Create new agent session (z-plugin suggestions) |
| `c` | Collapse/expand subagent tree |
| `x` | Dismiss subagent or close agent pane |
| `m` | Mark agent as merged and send cleanup |
| `p` | Toggle plan view |
| `u` | Toggle usage dashboard |
| `h` | Show help overlay with all keybindings |
| `J/K` | Line scroll (plan/diff views) |
| `q` / `Ctrl+C` | Quit |

### Diff Viewer Keybindings

| Key | Action |
|-----|--------|
| `j/k` or `arrows` | Select file |
| `J/K` (shift) | Scroll diff content (single line) |
| `Ctrl+U/D` | Scroll diff content (half page) |
| `e` | Expand/collapse all context |
| `d` or `Esc` | Exit diff viewer |

## User Settings

The dashboard supports a TOML configuration file at `~/.agent-dashboard/settings.toml` (or `$AGENT_DASHBOARD_DIR/settings.toml` if overridden). Any missing keys fall back to sensible defaults — you only need to include the settings you want to change.

Example `settings.toml`:

```toml
[banner]
show_mascot = false   # hide the axolotl pixel art (default: true)
show_quote  = false   # hide the daily quote (default: true)

[notifications]
enabled       = true  # enable desktop notifications from adapter hooks (default: false)
sound         = true  # play a sound with notifications (default: false)
silent_events = true  # suppress event-level notifications (default: false)
```

| Section | Key | Default | Description |
|---------|-----|---------|-------------|
| `banner` | `show_mascot` | `true` | Show the axolotl pixel art in the banner |
| `banner` | `show_quote` | `true` | Show the daily quote in the banner |
| `notifications` | `enabled` | `false` | Enable desktop notifications from adapter hooks |
| `notifications` | `sound` | `false` | Play a sound with notifications |
| `notifications` | `silent_events` | `false` | Suppress event-level notifications |

## Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `AGENT_DASHBOARD_DIR` | Override default state directory (`~/.agent-dashboard`) | No |
| `API_NINJAS_KEY` | API key for quote-of-the-day | No (falls back to built-in quotes) |

## Development

```bash
make build                        # Build binary to bin/ (injects version from VERSION file)
make test                         # Run tests
make install                      # Build + install binary + adapter (default: claude-code)
make install ADAPTER=claude-code  # Specify adapter explicitly
make seed                         # Create fake agent state for testing
make clean                        # Remove build artifacts and state
```

### Versioning

The project uses semantic versioning. The version is stored in the `VERSION` file at the repo root and injected into the binary at build time via Go's `-ldflags -X` mechanism. It is displayed in the dashboard banner (top right).

To bump the version, edit `VERSION` and rebuild.

### Project Structure

```
agent-dashboard/
├── Makefile
├── VERSION
├── install.sh                     # installer (accepts adapter name, default: claude-code)
├── agent-dashboard.tmux           # optional tmux keybinding (prefix + D)
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
│   │   ├── catppuccin-frappe.json # chroma syntax theme
│   │   ├── version.go             # build-time version variable
│   │   └── *_test.go              # tests
│   └── populate-quotes/
│       └── main.go                # bulk quote fetcher for SQLite cache
├── adapters/claude-code/          # Claude Code plugin
│   ├── hooks/hooks.json           # lifecycle hook definitions
│   ├── scripts/hooks/             # hook implementations (JS)
│   ├── packages/                  # shared JS modules (agent-state, git, tmux)
│   ├── skills/                    # workflow skills (feature, fix, pr, etc.)
│   └── agents/                    # agent methodology guides
├── schema/
│   └── agent-state.schema.json    # JSON Schema for agent state files
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

## Works Best With

This dashboard pairs well with the [everything-claude-code](https://github.com/affaan-m/everything-claude-code) plugin for a complete agent workflow experience.
