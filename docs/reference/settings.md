---
title: Settings
parent: Reference
nav_order: 2
---

# Settings

The dashboard is configured via a TOML file at `~/.agent-dashboard/settings.toml` (or `$AGENT_DASHBOARD_DIR/settings.toml` if overridden). The installer creates this from the example file. Any missing keys fall back to sensible defaults — you only need to include the settings you want to change.

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
```

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

## Environment variables

| Variable | Description | Required |
|:---------|:------------|:---------|
| `AGENT_DASHBOARD_DIR` | Override default state directory (`~/.agent-dashboard`) | No |
| `EDITOR` | Editor command for opening agent directories (default: `code`) | No |
| `API_NINJAS_KEY` | API key for quote-of-the-day | No (falls back to built-in quotes) |
