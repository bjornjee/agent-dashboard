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
- **Session creation** — create new agent sessions with z-plugin frecency-ranked path autocomplete and skill selection
- **Quick reply** — send free-text responses directly to agent panes
- **GitHub PR workflow** — open existing PR diff, create new PR, or merge via `gh` CLI (falls back to browser)
- **Help overlay** — full-screen keybinding reference grouped by context
- **Daily quote** — fetched from API Ninjas with fallback to embedded quotes
- **Pixel art banner** — axolotl rendered with half-block Unicode characters
- **Semantic versioning** — version injected at build time via `-ldflags`

## Prerequisites

| Dependency | Required | Purpose |
|------------|----------|---------|
| [tmux](https://github.com/tmux/tmux) | Yes | Agent pane management and live capture |
| [Claude Code](https://claude.com/claude-code) | Yes | The agents this dashboard monitors |
| [Go 1.26+](https://go.dev/dl/) | Yes | Building the dashboard binary |
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

The installer:
1. Builds the Go binary to `~/.local/bin/agent-dashboard`
2. Creates default settings at `~/.agent-dashboard/settings.toml`
3. Clones the marketplace repo to `~/.claude/plugins/marketplaces/agent-dashboard/`
4. Registers the marketplace in `~/.claude/plugins/known_marketplaces.json`
5. Installs the adapter to `~/.claude/plugins/cache/agent-dashboard/agent-dashboard/<version>/`
6. Registers the plugin in `~/.claude/plugins/installed_plugins.json`
7. Enables the plugin in `~/.claude/settings.json`

Restart Claude Code sessions after installation for hooks and skills to take effect.

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
| `y/n` | Quick approve/reject (blocked or waiting agents) |
| `1-9` | Send numbered option to agent |
| `e` | Open editor in agent's directory |
| `d` | Show git diff (merge-base vs HEAD, syntax highlighted) |
| `g` | Open existing PR diff or create new PR |
| `m` | Merge PR via `gh` CLI and send cleanup |
| `a` | Create new agent session (z-plugin suggestions + skill selection) |
| `c` | Collapse/expand subagent tree |
| `x` | Dismiss subagent or close agent pane |
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
| `g/G` | Jump to first/last file |
| `{/}` | Jump file list by half page |
| `Enter` or `Space` | Toggle directory expand/collapse |
| `/` | Filter files by name |
| `e` | Expand/collapse all context |
| `d`, `q`, or `Esc` | Exit diff viewer |

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
make build                        # Build binary to bin/ (version from git tag or VERSION file)
make fmt                          # Auto-format Go source files
make vet                          # Check formatting + run go vet
make test                         # Run all tests (vets first)
make install                      # Build + install binary + adapter (default: claude-code)
make install ADAPTER=claude-code  # Specify adapter explicitly
make seed                         # Create fake agent state for testing
make clean                        # Remove build artifacts and state
make help                         # Show all available targets
```

### Versioning

The project uses semantic versioning. The version is resolved from the latest git tag (stripping the `v` prefix), falling back to the `VERSION` file at the repo root. It is injected into the binary at build time via Go's `-ldflags -X` mechanism and displayed in the dashboard banner.

### Project Structure

```
agent-dashboard/
├── Makefile
├── VERSION
├── CHANGELOG.md
├── CONTRIBUTING.md
├── LICENSE
├── SECURITY.md
├── release-please-config.json
├── install.sh                     # installer (accepts adapter name, default: claude-code)
├── agent-dashboard.tmux           # optional tmux keybinding (prefix + D)
├── cmd/
│   ├── dashboard/
│   │   ├── main.go                # entry point
│   │   ├── model.go               # Bubble Tea model + update loop
│   │   ├── view.go                # render logic (panels, layout)
│   │   ├── keys.go                # keybindings + mouse handling
│   │   ├── commands.go            # tea.Cmd functions (tmux, state)
│   │   ├── config.go              # agent profile + dashboard configuration
│   │   ├── settings.go            # TOML settings loader (banner, notifications)
│   │   ├── state.go               # agent state structs + file I/O
│   │   ├── conversation.go        # JSONL parsing, subagent discovery
│   │   ├── messages.go            # tea message types + constants
│   │   ├── diff.go                # git diff loading
│   │   ├── diff_view.go           # diff rendering + syntax highlighting
│   │   ├── skills.go              # plugin skill discovery
│   │   ├── usage.go               # token counting + pricing
│   │   ├── db.go                  # SQLite operations (usage + quotes)
│   │   ├── banner.go              # axolotl pixel art + quote display
│   │   ├── tmux.go                # tmux integration helpers
│   │   ├── zsuggest.go            # z-plugin frecency suggestions
│   │   ├── wrapped_input.go       # soft-wrap text input helper
│   │   ├── helpers.go             # text wrapping, markdown rendering
│   │   ├── styles.go              # Catppuccin Frappe theme
│   │   ├── catppuccin-frappe.json # chroma syntax theme
│   │   ├── version.go             # build-time version variable
│   │   └── *_test.go              # tests
│   └── populate-quotes/
│       └── main.go                # bulk quote fetcher for SQLite cache
├── adapters/claude-code/          # Claude Code plugin
│   ├── CLAUDE.md                  # agent instructions for the adapter
│   ├── package.json               # plugin metadata
│   ├── hooks/hooks.json           # lifecycle hook definitions
│   ├── scripts/hooks/             # hook implementations (JS)
│   ├── packages/                  # shared JS modules (agent-state, git-status, tmux)
│   ├── skills/                    # workflow skills (feature, fix, chore, refactor, pr, investigate)
│   └── agents/                    # agent definitions (code-reviewer, planner, tdd-guide, etc.)
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
| [toml](https://github.com/BurntSushi/toml) | Settings file parsing |
| [sqlx](https://github.com/jmoiron/sqlx) | SQL query helper |
| [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) | Pure Go SQLite |
| [fsnotify](https://github.com/fsnotify/fsnotify) | File system watcher |

## Works Best With

This dashboard pairs well with the [everything-claude-code](https://github.com/affaan-m/everything-claude-code) plugin for a complete agent workflow experience.
