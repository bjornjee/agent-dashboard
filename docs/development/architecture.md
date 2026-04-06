---
title: Architecture
parent: Development
nav_order: 2
---

# Architecture

An overview of the project structure, package responsibilities, and key dependencies.

---

## Top-level layout

```
agent-dashboard/
├── Makefile
├── VERSION
├── CHANGELOG.md
├── CONTRIBUTING.md
├── LICENSE
├── SECURITY.md
├── release-please-config.json
├── install.sh                         # installer (accepts adapter name)
├── agent-dashboard.tmux               # optional tmux keybinding (prefix + D)
├── settings.example.toml              # default settings (copied by installer)
├── go.mod / go.sum
├── cmd/
│   ├── dashboard/main.go              # TUI entry point
│   ├── web/main.go                    # web server entry point
│   └── populate-quotes/main.go        # bulk quote fetcher for SQLite cache
├── internal/                          # core packages (see below)
├── adapters/claude-code/              # Claude Code plugin (see Adapter reference)
└── schema/
    └── agent-state.schema.json        # JSON Schema for agent state files
```

## Internal packages

```
internal/
├── config/          # agent profile + TOML settings loader
├── conversation/    # JSONL parsing, subagent discovery
├── gh/              # GitHub CLI helpers (CODEOWNERS, merge args)
├── db/              # SQLite operations (usage + quotes)
├── domain/          # shared type definitions (Agent, Message, etc.)
├── lock/            # singleton instance lock
├── skills/          # plugin skill discovery
├── state/           # agent state structs + file I/O
├── tmux/            # tmux integration helpers
├── tui/             # Bubble Tea UI (model, view, keys, commands)
├── usage/           # token counting + pricing
├── web/             # web server, handlers, OAuth, SSE, static assets
└── zsuggest/        # z-plugin frecency suggestions
```

### Key packages

**`internal/tui/`** — The Bubble Tea TUI. The model, view, and update loop live here, along with the diff viewer, banner, keybindings, and styles. All external command execution goes through a `Runner` interface for testability.

**`internal/state/`** — Reads and writes per-agent JSON state files in `~/.agent-dashboard/agents/`. The TUI polls these files and uses `fsnotify` for instant updates.

**`internal/conversation/`** — Incrementally parses JSONL conversation logs. Tracks file offsets to avoid re-reading entire logs on each poll.

**`internal/web/`** — The mobile companion web server. Serves the PWA, handles Google OAuth, and streams state updates via Server-Sent Events.

## Key dependencies

| Package | Purpose |
|:--------|:--------|
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
| [oauth2](https://pkg.go.dev/golang.org/x/oauth2) | Google OAuth |

## Data flow

```
Claude Code session
  └─ adapter hooks fire on tool calls / state changes
     └─ write JSON to ~/.agent-dashboard/agents/<session>.json
        └─ dashboard reads via fsnotify + polling
           ├─ TUI renders in terminal
           └─ web server streams via SSE to mobile PWA
```
