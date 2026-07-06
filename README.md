# agent-dashboard

A tmux-integrated orchestrator and dispatcher for AI coding agents — Claude Code, Codex, and more.

`agent-dashboard` runs coding agents across tmux panes, dispatches your input to whichever one needs you, and gates each session through workflow skills (TDD, conventional commits, branch policy) enforced by hooks. The TUI is built with [Bubble Tea](https://github.com/charmbracelet/bubbletea); a companion PWA exposes the same orchestration surface from your phone over your local network.

https://github.com/user-attachments/assets/01aa0f85-cfd4-4dc3-ac46-651bcfc03f99

**Documentation: [bjornjee.github.io/agent-dashboard](https://bjornjee.github.io/agent-dashboard/)** — getting started, guides, keybindings, settings, and architecture.

## Why agent-dashboard?

- **One pane for many agents.** Stop alt-tabbing between tmux panes — see every agent grouped by state (blocked, waiting, running, review, PR, merged) in a single TUI, with live pane capture so you don't lose context when you switch.
- **Workflow gates, not vibes.** Skills (feature, fix, chore, refactor, investigate, implement, pr, rca) plus hooks (commit-lint, test-gate, no-commits-to-main, destructive-op warnings) enforce TDD and conventional commits at the harness level — agents can't merge if tests fail.
- **Phone-first remote control.** A companion PWA exposes the same orchestration surface over your local network: approve permissions, reply to questions, open/merge PRs, and get browser notifications when an agent needs you — all without opening your laptop.
- **tmux-native, not tmux-replacing.** Agents stay where they live; the dashboard adds a control plane on top of `tmux capture-pane`. No new pane manager, no daemon competing with tmux. If tmux dies, orphaned sessions survive as resumable and continue in a fresh pane.
- **Multi-backend.** Claude Code and Codex CLI are supported directly, with per-spawn harness, model, and thinking-effort selection in the New Agent flow.

## Features

**TUI** — real-time agent monitoring with live tmux pane capture, subagent tree, conversation history, GitHub-style split-pane diff viewer, plan and mermaid-diagram viewers, per-agent token/cost dashboard with live rate-limit bars, session creation with frecency-ranked paths and skill/model/effort selection, a search palette (`/`) that also resumes orphaned sessions, quick replies and numbered-option sends, and a full GitHub PR workflow (create, review diff, merge via `gh`).

**Mobile companion** — an installable PWA with the same state grouping, full remote control (approve/reject, reply, stop, PR create/merge/close), session creation, browser notifications, usage dashboard, live updates over Server-Sent Events, and optional single-user Google OAuth.

See the [guides](https://bjornjee.github.io/agent-dashboard/guides/) for walkthroughs and the [reference](https://bjornjee.github.io/agent-dashboard/reference/) for the full keybinding and settings tables.

## FAQ

**Do I need tmux?** Yes. agent-dashboard reads live pane content via `tmux capture-pane` and spawns agent sessions in tmux panes. Without tmux there are no panes to monitor.

**Which agents are supported?** Claude Code is first-class via the adapter in `adapters/claude-code/`. Codex CLI is supported directly via the adapter in `adapters/codex/`. The architecture supports additional backends via the `domain.Harness` interface.

**How do I use codex / gpt-5.x models?** Pick `codex` in the New Agent harness step (TUI wizard or web form), with per-spawn flags from `[harness.codex]` in `~/.agent-dashboard/settings.toml`. Codex sessions appear in the dashboard once the Codex plugin is installed and its hooks approved (see [Codex CLI support](#codex-cli-support)).

**Does this require a paid Claude account?** No — it uses whatever Claude Code itself requires (Pro, Max, or API). agent-dashboard does not call the Anthropic API directly; it reads the JSONL transcripts Claude Code writes locally.

**Can I use the dashboard without the mobile companion?** Yes. The TUI is the primary interface. The PWA is optional and runs separately via `make web`.

**How is this different from a generic tmux session manager?** A session manager creates and switches panes. agent-dashboard *understands* what's running in each pane — it parses the harness's transcripts to detect state (blocked, waiting, running, review, PR, merged), captures plans and Mermaid diagrams, tracks token usage, and integrates the GitHub PR workflow.

**Is this related to Claude Code's official UI?** No. agent-dashboard is an unofficial third-party plugin. It builds on top of Claude Code's hooks system and JSONL transcripts but is not affiliated with Anthropic.

**Does it work on Windows?** Native Windows is unsupported — the project targets macOS and Linux because tmux is required. WSL with tmux installed should work but is untested.

## Prerequisites

| Dependency | Required | Purpose |
|------------|----------|---------|
| [tmux](https://github.com/tmux/tmux) | Yes | Agent pane management and live capture |
| [Claude Code](https://claude.com/claude-code) | Yes | The agents this dashboard monitors |
| [Node.js 18+](https://nodejs.org/) | Yes | Claude Code and Codex adapter hooks |
| [git](https://git-scm.com/) | Yes | Diff viewer, branch detection |
| [GitHub CLI (`gh`)](https://cli.github.com/) | No | PR detection and merge |
| [Codex CLI](https://developers.openai.com/codex/) 0.130+ | No | Show Codex sessions in the dashboard |
| [z (zsh plugin)](https://github.com/agkozak/zsh-z) | No | Frecency-ranked directory suggestions |

## Install

### Step 1: Install the binary

```bash
curl -fsSL https://raw.githubusercontent.com/bjornjee/agent-dashboard/main/install.sh | sh
```

The installer downloads the binary for your platform from the latest [GitHub Release](https://github.com/bjornjee/agent-dashboard/releases), verifies its SHA256 checksum, and installs it to `~/.local/bin/agent-dashboard`. No Go toolchain required.

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
/plugin enable agent-dashboard@agent-dashboard
```

Then restart Claude Code sessions for hooks and skills to take effect.

### Recommended companion plugins

`agent-dashboard` works best when paired with [`bjornjee/skills`](https://github.com/bjornjee/skills) — a plugin of workflow skills (TDD guide, language-specific strict reviewers, refactor cleaner, codex delegation, terminal ops) that the dashboard's session-creation flow expects:

```
/marketplace add bjornjee/skills
/plugin install skills@bjornjee-skills
```

Without it, skill-gated session types (feature, fix, chore, refactor, investigate, implement, pr, rca) will not function as intended.

### Codex CLI support

Register the marketplace entry with Codex:

```bash
codex plugin marketplace add bjornjee/agent-dashboard
```

Then enable the plugin by appending the following to `~/.codex/config.toml`:

```toml
[plugins."agent-dashboard@agent-dashboard"]
enabled = true
```

Restart Codex sessions and approve the `agent-dashboard` hooks prompt. Once approved, the dashboard sees Codex sessions just like Claude sessions — same state files, same conversation panel, same cost dashboard. From a repo checkout, `make install-codex-adapter` performs the registration and prints the config snippet.

### Optional: tmux keybinding

The included `agent-dashboard.tmux` script binds `prefix + D` to switch to a dedicated dashboard session:

```bash
# Add to ~/.tmux.conf
run-shell /path/to/agent-dashboard/agent-dashboard.tmux

# Reload tmux config
tmux source-file ~/.tmux.conf
```

## Uninstall

In any Claude Code session, run `/plugin uninstall agent-dashboard@agent-dashboard` and `/marketplace remove agent-dashboard`. Then remove the binary and state:

```bash
curl -fsSL https://raw.githubusercontent.com/bjornjee/agent-dashboard/main/uninstall.sh | sh
```

Or from a repo checkout: `make uninstall`.

## Usage

```bash
agent-dashboard
```

Or press `prefix + D` if you set up the tmux keybinding. The keys you'll use most:

| Key | Action |
|-----|--------|
| `j/k` | Navigate agent list |
| `Enter` | Jump to agent's tmux pane |
| `r` | Reply to agent (free-text) |
| `y/n` | Quick approve/reject |
| `a` | Create new agent session |
| `/` | Search palette (also resumes orphaned sessions) |
| `d` | Show git diff |
| `g` / `m` | Open/create PR / merge PR |
| `h` | Help overlay with all keybindings |

The full tables — including the diff viewer and search palette keys — are in the [keybindings reference](https://bjornjee.github.io/agent-dashboard/reference/keybindings/).

## Configuration

The dashboard reads a TOML file at `~/.agent-dashboard/settings.toml` — banner, notifications, usage polling, thinking-effort defaults, and the harness picker (`claude` or `codex`) all live there. Missing keys fall back to sensible defaults; see [`settings.example.toml`](settings.example.toml) and the [settings reference](https://bjornjee.github.io/agent-dashboard/reference/settings/) for every key, default, and environment variable.

## Development

Requires [Go 1.26+](https://go.dev/dl/).

```bash
make build       # Build TUI binary to bin/
make fmt         # Auto-format Go source files
make test        # Run all tests (vets first)
make web         # Run the mobile companion locally on port 8390
make gen-skills  # Regenerate adapter skills from adapters/skills-src/
make help        # Show all targets
```

Workflow skills are generated: edit `adapters/skills-src/`, run `make gen-skills`, and never hand-edit `adapters/claude-code/skills/` or `adapters/codex/skills/`. Architecture, package layout, and contribution guidelines are in the [development docs](https://bjornjee.github.io/agent-dashboard/development/).

## License

[MIT](LICENSE)
