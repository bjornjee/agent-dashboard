---
title: Settings
parent: Reference
nav_order: 2
---

# Settings

The dashboard is configured via a TOML file at `~/.agent-dashboard/settings.toml` (or `$AGENT_DASHBOARD_DIR/settings.toml` if overridden). From a repo checkout, the installer copies this from `settings.example.toml` when the destination file does not already exist. Any missing keys fall back to sensible defaults — you only need to include the settings you want to change.

---

## Full reference

```toml
[banner]
show_mascot   = true   # show the axolotl pixel art
show_quote    = true   # show the daily quote

[notifications]
enabled       = false  # enable desktop notifications from adapter hooks
sound         = false  # play alert sound on attention events
silent_events = false  # show notification for non-alerting stops

[debug]
key_log       = false  # write key/mouse/focus events to debug-keys.log

[experimental]
ascii_pet     = false  # show animated ASCII pet in the left panel
dino_game     = false  # enable Chrome-style dino runner game (Shift+G)

[usage]
rate_limit_poll_seconds = 60  # how often to fetch rate limits from Anthropic API (0 = disable)

[effort]
plan    = "high"  # thinking-effort level pinned while permission_mode='plan'
default = "high"  # thinking-effort level pinned at spawn and restored on plan exit

[harness]
default = "claude"  # active coding-agent harness: "claude" or "pi"

[harness.pi]
provider = ""       # e.g. "openai" — passed as pi --provider
model    = ""       # e.g. "openai-codex/gpt-5.5" — passed as pi --model
```

The `[effort]` levels feed the `/effort` slash command Claude Code accepts (`low | medium | high | xhigh | max`). The `agent-state-fast` adapter hook swaps in `plan` when the agent enters plan mode (`EnterPlanMode`) and restores `default` on exit. The `feature`, `fix`, and `refactor` skills additionally declare `effort: max` in their frontmatter, which Claude Code pins for the skill's lifetime when the skill is invoked as a slash command inside an existing session.

The `[harness]` section selects which coding-agent binary backs newly-spawned sessions. `"claude"` uses Claude Code (default; reads `~/.claude`). `"pi"` uses [`@mariozechner/pi-coding-agent`](https://www.npmjs.com/package/@mariozechner/pi-coding-agent) (reads `~/.pi`) and unlocks OpenAI / codex `gpt-5.x` models via pi-mono's unified LLM API — see [`adapters/pi/README.md`](https://github.com/bjornjee/agent-dashboard/blob/main/adapters/pi/README.md#codex--gpt-5x-models). The `[harness.pi]` `provider` and `model` flow into `pi --provider <p> --model <m>`; either empty inherits pi-mono's auth resolution chain. Per-spawn override is exposed in the New Agent form's Harness dropdown.

## Settings table

| Section | Key | Default | Description |
|:--------|:----|:--------|:------------|
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
| `harness` | `default` | `"claude"` | Active coding-agent harness. `"claude"` runs Claude Code; `"pi"` runs pi-mono (unlocks OpenAI / codex `gpt-5.x` models). |
| `harness.pi` | `provider` | `""` | Provider passed to `pi --provider`. Leave empty to inherit pi-mono's auth resolution chain. Example: `"openai"`. |
| `harness.pi` | `model` | `""` | Model passed to `pi --model`. Leave empty to inherit pi-mono's default. Example: `"openai-codex/gpt-5.5"`. |

## Environment variables

| Variable | Description | Required |
|:---------|:------------|:---------|
| `AGENT_DASHBOARD_DIR` | Override default state directory (`~/.agent-dashboard`) | No |
| `EDITOR` | Editor command for opening agent directories (default: `code`) | No |
| `API_NINJAS_KEY` | API key for quote-of-the-day | No (falls back to built-in quotes) |
| `GOOGLE_CLIENT_ID` | Google OAuth client ID for [mobile companion](../guides/mobile-companion/) authentication | No |
| `GOOGLE_CLIENT_SECRET` | Google OAuth client secret | No |
| `GOOGLE_ALLOWED_EMAIL` | Email address allowed to access the mobile companion | No |
