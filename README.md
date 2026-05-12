# agent-dashboard

A tmux-integrated orchestrator and dispatcher for AI coding agents — Claude Code, Codex, and more.

`agent-dashboard` runs Claude Code agents across tmux panes, dispatches your input to whichever one needs you, and gates each session through workflow skills (TDD, conventional commits, branch policy) enforced by hooks. The TUI is built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and styled with Catppuccin Frappe; a companion PWA exposes the same orchestration surface from your phone over your local network.

https://github.com/user-attachments/assets/01aa0f85-cfd4-4dc3-ac46-651bcfc03f99


Both interfaces read agent state from per-agent JSON files in `~/.agent-dashboard/agents/` (written by the Claude Code adapter in `adapters/claude-code/`).

## Why agent-dashboard?

- **One pane for many agents.** Stop alt-tabbing between tmux panes — see every agent grouped by state (blocked, running, review, PR, merged) in a single TUI, with live pane capture so you don't lose context when you switch.
- **Workflow gates, not vibes.** Skills (feature, fix, chore, refactor, investigate, pr, rca) plus hooks (commit-lint, test-gate, no-commits-to-main, destructive-op warnings) enforce TDD and conventional commits at the harness level — agents can't merge if tests fail.
- **Phone-first remote control.** A companion PWA on `cmd/web/` exposes the same orchestration surface over your local network: approve permissions, reply to questions, open/merge PRs, and get browser notifications when an agent needs you — all without opening your laptop.
- **tmux-native, not tmux-replacing.** Agents stay where they live; the dashboard adds a control plane on top of `tmux capture-pane`. No new pane manager, no daemon competing with tmux.
- **Multi-backend.** Claude Code is first-class; pi-mono is a second-class harness (including OpenAI / codex `gpt-5.x` models via pi-mono's unified LLM API); Codex CLI is supported via skill delegation. The harness picker in `settings.toml` (`[harness] default`) selects the active backend; per-spawn override is exposed in the New Agent form.

## FAQ

**Do I need tmux?** Yes. agent-dashboard reads live pane content via `tmux capture-pane` and spawns agent sessions in tmux panes. Without tmux there are no panes to monitor.

**Which agents are supported?** Claude Code is first-class via the adapter in `adapters/claude-code/`. A second adapter for [`@mariozechner/pi-coding-agent`](https://www.npmjs.com/package/@mariozechner/pi-coding-agent) lives in `adapters/pi/` (install with `make install-pi-adapter`); pi-mono's unified LLM API also lets you run OpenAI / codex `gpt-5.x` models inside the dashboard. Codex CLI itself is spawned through skill delegation (`/codex-delegate`). The architecture supports additional backends via the `domain.Harness` interface.

**How do I use codex / gpt-5.x models?** Set `[harness] default = "pi"` and `[harness.pi] provider = "openai" model = "openai-codex/gpt-5.5"` in `~/.agent-dashboard/settings.toml`, configure your OpenAI key in `~/.pi/auth.json`, and restart the dashboard. The New Agent form's Harness dropdown also lets you override per-spawn. See [adapters/pi/README.md](adapters/pi/README.md#codex--gpt-5x-models) for the full walkthrough.

**Does this require a paid Claude account?** No — it uses whatever Claude Code itself requires (Pro, Max, or API). agent-dashboard does not call the Anthropic API directly; it reads the JSONL transcripts Claude Code writes locally.

**Can I use the dashboard without the mobile companion?** Yes. The TUI is the primary interface. The PWA in `cmd/web/` is optional and runs separately via `make web`.

**How is this different from a generic tmux session manager?** A session manager creates and switches panes. agent-dashboard *understands* what's running in each pane — it parses Claude Code's JSONL transcripts to detect state (blocked, waiting, running, done, PR, merged), captures plans and Mermaid diagrams, tracks token usage, and integrates the GitHub PR workflow.

**Is this related to Claude Code's official UI?** No. agent-dashboard is an unofficial third-party plugin. It builds on top of Claude Code's hooks system and JSONL transcripts but is not affiliated with Anthropic.

**Does it work on Windows?** Native Windows is unsupported — the project targets macOS and Linux because tmux is required. WSL with tmux installed should work but is untested.

## Features

### TUI (terminal)

- **Real-time agent monitoring** — agents grouped by state (needs attention, running, completed) with live tmux pane capture
- **Subagent tree** — expand/collapse/dismiss subagent nodes under parent agents
- **Conversation history** — timestamped messages with role-based colouring, incremental JSONL parsing
- **Diff viewer** — GitHub-style split-pane diff with syntax highlighting, smart context collapsing, and sticky function headers
- **File change tracking** — colour-coded additions, removals, and modifications
- **Plan viewer** — glamour-rendered markdown plans with syntax highlighting
- **Mermaid diagram viewer** — captures `mermaid` blocks from agent messages, browse per-session, render in browser via `D`
- **Usage dashboard** — per-agent token breakdown (Claude and Codex), weekly cost in the bottom bar, rolling 7-day breakdown anchored to Monday, cumulative totals persisted to SQLite, live rate-limit bars (auto-discovered from Claude OAuth credentials)
- **Session creation** — spawn new agent sessions with z-plugin frecency-ranked path autocomplete, skill selection (feature, fix, chore, refactor, investigate, pr, rca), and automatic detection of Claude Code's "trust this folder?" prompt during spawn
- **ASCII pet** — experimental animated red panda companion in the left panel (opt-in via settings)
- **Dino runner game** — experimental Chrome-style endless runner in the left panel with jump, duck, speed ramp, and score counter (opt-in via settings)
- **Quick reply** — send free-text responses directly to agent panes
- **GitHub PR workflow** — open existing PR diff, create new PR, or merge via `gh` CLI (falls back to browser)
- **Status feedback** — colour-coded success/error messages in the help bar for user actions
- **Help overlay** — full-screen keybinding reference grouped by context
- **Daily quote** — fetched from API Ninjas with fallback to embedded quotes
- **Pixel art banner** — axolotl rendered with half-block Unicode characters
- **Singleton lock** — prevents multiple dashboard instances from running simultaneously
- **Semantic versioning** — version injected at build time via `-ldflags`

### Mobile remote control

A companion PWA (`cmd/web/`) for dispatching and managing agents from your phone over your local network:

- **Agent list and detail views** — same state grouping as the TUI, with a collapsible top bar, conversation timeline, and diff viewer; detail pages auto-refresh over Server-Sent Events as agents progress
- **Full remote control** — approve/reject permissions, reply to questions via a free-text input, send numbered options, stop agents, open PRs, merge, and close — all from your phone
- **Session creation** — spawn new agent sessions with z-plugin suggestions and skill selection; new sessions for the same repo reuse an existing tmux pane instead of stacking new ones
- **Browser notifications** — opt-in web notifications nudge you when an agent needs attention, even when the tab is backgrounded
- **Usage dashboard** — token breakdown and cost tracking
- **Google OAuth** — optional single-user authentication so only you can access the companion
- **Installable PWA** — add to home screen for a native app feel with offline caching via service worker

<details>
<summary>Agent state model</summary>

| Raw state | Display group | Priority | Description |
|-----------|--------------|----------|-------------|
| `permission` | BLOCKED | 1 | Waiting for tool permission |
| `plan` | BLOCKED | 1 | Plan review pending |
| `question` | WAITING | 2 | Agent asked a question |
| `error` | WAITING | 2 | Agent encountered an error |
| `running` | RUNNING | 3 | Actively executing |
| `idle_prompt` | REVIEW | 4 | Idle, waiting for user input |
| `done` | REVIEW | 4 | Session completed |
| `pr` | PR | 5 | Pull request created |
| `merged` | MERGED | 6 | Pull request merged |

**Pinned states:** `pr` and `merged` are sticky — once set, idle states (`idle_prompt`, `done`, `question`) restore back to the pinned value. Active states (`running`, `permission`) display through normally so the dashboard reflects live work.

**Group headers:** when a priority-1 or priority-2 group is homogeneous, the TUI renders a more specific label — `PLAN` (yellow) or `PERMISSION` instead of `BLOCKED`, and `QUESTION` (yellow) or `ERROR` (red) instead of `WAITING`. Mixed groups keep the generic header. Sort order is unchanged.

</details>

## Prerequisites

| Dependency | Required | Purpose |
|------------|----------|---------|
| [tmux](https://github.com/tmux/tmux) | Yes | Agent pane management and live capture |
| [Claude Code](https://claude.com/claude-code) | Yes | The agents this dashboard monitors |
| [Node.js 18+](https://nodejs.org/) | Yes | Claude Code adapter hooks |
| [git](https://git-scm.com/) | Yes | Diff viewer, branch detection |
| [GitHub CLI (`gh`)](https://cli.github.com/) | No | Detects existing PRs so `g` opens the diff page instead of creating a new PR |
| [z (zsh plugin)](https://github.com/agkozak/zsh-z) | No | Frecency-ranked directory suggestions when creating sessions |

## Install

### Step 1: Install the binary

Download the pre-built binary from the latest [GitHub Release](https://github.com/bjornjee/agent-dashboard/releases):

```bash
curl -fsSL https://raw.githubusercontent.com/bjornjee/agent-dashboard/main/install.sh | sh
```

The installer downloads the binary for your platform, verifies its SHA256 checksum, and installs it to `~/.local/bin/agent-dashboard`. No Go toolchain required.

Or build from source (requires [Go 1.26+](https://go.dev/dl/)):

```bash
git clone https://github.com/bjornjee/agent-dashboard
cd agent-dashboard
./install.sh --build
```

### Step 2: Register the marketplace and install the plugin

In any Claude Code session, run:

```
/marketplace add bjornjee/agent-dashboard
/plugin install agent-dashboard@agent-dashboard
```

Then restart Claude Code sessions for hooks and skills to take effect.

### Recommended companion plugins

`agent-dashboard` works best when paired with [`bjornjee/skills`](https://github.com/bjornjee/skills) — a plugin of workflow skills (TDD guide, language-specific strict reviewers, refactor cleaner, codex delegation, terminal ops) that the dashboard's session-creation flow expects:

```
/marketplace add bjornjee/skills
/plugin install skills@bjornjee-skills
```

Without it, skill-gated session types (feature, fix, refactor, pr, rca) will not function as intended.

## Uninstall

### Step 1: Remove the plugin

In any Claude Code session, run:

```
/plugin uninstall agent-dashboard@agent-dashboard
/marketplace remove agent-dashboard
```

### Step 2: Remove the binary and state

```bash
curl -fsSL https://raw.githubusercontent.com/bjornjee/agent-dashboard/main/uninstall.sh | sh
```

Or from a repo checkout: `make uninstall`. Pass `--yes` to skip the prompt for deleting `~/.agent-dashboard/` (which contains settings and usage data).

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
| `D` | Toggle diagrams panel (when diagrams present) |
| `u` | Toggle usage dashboard |
| `G` (Shift+G) | Toggle dino runner game (when enabled) |
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

The dashboard supports a TOML configuration file at `~/.agent-dashboard/settings.toml` (or `$AGENT_DASHBOARD_DIR/settings.toml` if overridden). The installer creates this from [`settings.example.toml`](settings.example.toml). Any missing keys fall back to sensible defaults — you only need to include the settings you want to change.

Example `settings.toml`:

```toml
[banner]
show_mascot = true   # show the axolotl pixel art (default: true)
show_quote  = true   # show the daily quote (default: true)

[notifications]
enabled       = false  # enable desktop notifications from adapter hooks (default: false)
sound         = false  # play alert sound on attention events (default: false)
silent_events = false  # show notification for non-alerting stops (default: false)

[debug]
key_log = false       # write key/mouse/focus events to debug-keys.log (default: false)

[experimental]
ascii_pet = false     # show animated ASCII pet in the left panel (default: false)
dino_game = false     # show Chrome-style dino runner game in the left panel (default: false)

[usage]
rate_limit_poll_seconds = 60  # how often to fetch rate limits from Anthropic API (default: 60, 0 = disable)

[effort]
plan    = "high"  # thinking-effort level pinned while permission_mode='plan' (default: high)
default = "high"  # thinking-effort level pinned at spawn and restored on plan exit (default: high)
```

The `[effort]` levels feed the `/effort` slash command Claude Code accepts (`low | medium | high | xhigh | max`). The `agent-state-fast` hook swaps in `plan` when the agent enters plan mode (`EnterPlanMode`) and restores `default` on exit. The `feature`, `fix`, and `refactor` skills additionally declare `effort: max` in their frontmatter, which Claude Code pins for the skill's lifetime when the skill is invoked as a slash command inside an existing session.

| Section | Key | Default | Description |
|---------|-----|---------|-------------|
| `banner` | `show_mascot` | `true` | Show the axolotl pixel art in the banner |
| `banner` | `show_quote` | `true` | Show the daily quote in the banner |
| `notifications` | `enabled` | `false` | Enable desktop notifications from adapter hooks |
| `notifications` | `sound` | `false` | Play alert sound on attention events |
| `notifications` | `silent_events` | `false` | Show notification for non-alerting stops |
| `debug` | `key_log` | `false` | Write key/mouse/focus events to `debug-keys.log` |
| `experimental` | `ascii_pet` | `false` | Show animated ASCII pet in the left panel |
| `experimental` | `dino_game` | `false` | Show Chrome-style dino runner game in the left panel (Shift+G to toggle) |
| `usage` | `rate_limit_poll_seconds` | `60` | How often (in seconds) to fetch rate-limit data from the Anthropic OAuth API. Set to `0` to disable. |
| `effort` | `plan` | `"high"` | Thinking-effort level pinned while the agent is in plan mode. One of `low`, `medium`, `high`, `xhigh`, `max`. |
| `effort` | `default` | `"high"` | Thinking-effort level pinned at spawn and restored when the agent exits plan mode. Same value set as `plan`. |

## Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `AGENT_DASHBOARD_DIR` | Override default state directory (`~/.agent-dashboard`) | No |
| `EDITOR` | Editor command for opening agent directories (default: `code`) | No |
| `API_NINJAS_KEY` | API key for quote-of-the-day | No (falls back to built-in quotes) |
| `GOOGLE_CLIENT_ID` | Google OAuth client ID for mobile companion authentication | No |
| `GOOGLE_CLIENT_SECRET` | Google OAuth client secret | No |
| `GOOGLE_ALLOWED_EMAIL` | Email address allowed to access the mobile companion | No |

## Development

Development requires [Go 1.26+](https://go.dev/dl/) (not needed for binary installation).

```bash
make build                        # Build TUI binary to bin/ (version from git tag or VERSION file)
make build-web                    # Build web server binary to bin/
make fmt                          # Auto-format Go source files
make vet                          # Check formatting + run go vet
make test                         # Run all tests (vets first)
make test-race                    # Run tests with race detector
make install                      # Build and install binary from source
make uninstall                    # Remove binary and state directory
make install-web                  # Install web server binary to ~/.local/bin/
make web                          # Run web server locally on port 8390
make seed                         # Create fake agent state for testing
make clean                        # Remove build artifacts and state
make help                         # Show all available targets
```

### Versioning

The project uses semantic versioning. The version is resolved from the latest git tag (stripping the `v` prefix), falling back to the `VERSION` file at the repo root. It is injected into the binary at build time via Go's `-ldflags -X` mechanism and displayed in the dashboard banner.

### Project Structure

<details>
<summary><code>agent-dashboard/</code> — top-level layout</summary>

```
agent-dashboard/
├── Makefile
├── VERSION
├── CHANGELOG.md
├── CONTRIBUTING.md
├── LICENSE
├── SECURITY.md
├── release-please-config.json
├── install.sh                         # installer (accepts adapter name, default: claude-code)
├── agent-dashboard.tmux               # optional tmux keybinding (prefix + D)
├── settings.example.toml              # default settings (copied by install.sh)
├── go.mod / go.sum
├── cmd/
│   ├── dashboard/
│   │   └── main.go                    # TUI entry point
│   ├── web/
│   │   └── main.go                    # web server entry point (Google OAuth, SSE)
│   └── populate-quotes/
│       └── main.go                    # bulk quote fetcher for SQLite cache
├── internal/                          # core packages (see below)
├── adapters/claude-code/              # Claude Code plugin (see below)
└── schema/
    └── agent-state.schema.json        # JSON Schema for agent state files
```

</details>

<details>
<summary><code>internal/</code> — Go packages (domain-oriented)</summary>

```
internal/
├── config/                            # agent profile + TOML settings loader
├── conversation/                      # JSONL parsing, subagent discovery
├── gh/                                # GitHub CLI helpers (CODEOWNERS, merge args)
├── db/                                # SQLite operations (usage + quotes)
├── domain/                            # shared type definitions (Agent, Message, etc.)
├── lock/                              # singleton instance lock
├── skills/                            # plugin skill discovery
├── state/                             # agent state structs + file I/O
├── tmux/                              # tmux integration helpers
├── tui/                               # Bubble Tea UI
│   ├── model.go                       # Bubble Tea model + update loop
│   ├── view.go                        # render logic (panels, layout)
│   ├── keys.go                        # keybindings + mouse handling
│   ├── commands.go                    # tea.Cmd functions (tmux, state)
│   ├── messages.go                    # tea message types + constants
│   ├── diff.go                        # git diff loading
│   ├── diff_view.go                   # diff rendering + syntax highlighting
│   ├── banner.go                      # axolotl pixel art + quote display
│   ├── helpers.go                     # text wrapping, markdown rendering
│   ├── wrapped_input.go              # soft-wrap text input helper
│   ├── dino.go                        # experimental dino runner game
│   ├── pet.go                         # animated ASCII pet (red panda)
│   ├── runner.go                      # exec.Command interface for testability
│   ├── styles.go                      # Catppuccin Frappe theme
│   ├── catppuccin-frappe.json         # chroma syntax theme
│   ├── version.go                     # build-time version variable
│   └── *_test.go                      # tests
├── usage/                             # token counting + pricing
├── web/                               # web server, handlers, OAuth, SSE, static assets
└── zsuggest/                          # z-plugin frecency suggestions
```

</details>

<details>
<summary><code>adapters/claude-code/</code> — Claude Code plugin</summary>

```
adapters/claude-code/
├── CLAUDE.md                          # agent instructions for the adapter
├── package.json                       # plugin metadata
├── hooks/hooks.json                   # lifecycle hook definitions
├── scripts/hooks/                     # hook implementations (JS)
│   ├── agent-state-fast.js            # fast agent state reporter
│   ├── agent-state-reporter.js        # full agent state reporter
│   ├── block-main-commit.js           # prevents commits to main branch
│   ├── codex-delegation-gate.js       # enforces Codex delegation rules for skills
│   ├── codex-write-gate.js            # ensures Codex uses --write in worktrees
│   ├── commit-lint.js                 # validates commit message format
│   ├── desktop-notify.js              # desktop notifications
│   ├── mermaid-extractor.js           # extracts mermaid diagrams for the viewer
│   ├── pr-detect.js                   # detects existing PRs
│   ├── test-gate.js                   # blocks merges if tests fail
│   └── warn-destructive.js            # warns about destructive git ops
├── packages/                          # shared JS modules
│   ├── agent-state/                   # state detection + schema validation
│   ├── git-status/                    # git status utility
│   └── tmux/                          # tmux integration utilities
├── skills/                            # workflow skills
│   ├── feature/                       # feature development
│   ├── fix/                           # bug fix
│   ├── chore/                         # non-code changes
│   ├── refactor/                      # refactoring
│   ├── investigate/                   # investigation
│   ├── pr/                            # PR creation gate (cleaner + fmt + test)
│   └── rca/                           # root cause analysis
└── agents/                            # pre-configured agent definitions
    ├── build-error-resolver.md
    ├── code-reviewer.md
    ├── planner.md
    ├── security-reviewer.md
    └── tdd-guide.md
```

</details>

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
| [oauth2](https://pkg.go.dev/golang.org/x/oauth2) | Google OAuth for mobile web companion |
