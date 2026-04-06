---
title: Notifications
parent: Guides
nav_order: 5
---

# Notifications

Desktop notifications alert you when agents need attention — permission requests, questions, or unexpected stops.

---

## Enabling notifications

Notifications are disabled by default. Enable them in `~/.agent-dashboard/settings.toml`:

```toml
[notifications]
enabled       = true   # enable desktop notifications
sound         = true   # play alert sound on attention events
silent_events = false  # show notification for non-alerting stops
```

## What triggers a notification

| Event | Notification | Sound |
|:------|:-------------|:------|
| Agent requests permission | Yes | If `sound = true` |
| Agent asks a question | Yes | If `sound = true` |
| Agent stops unexpectedly | Only if `silent_events = true` | No |
| Agent completes normally | Only if `silent_events = true` | No |

## How it works

Notifications are sent by the Claude Code adapter hooks — specifically `desktop-notify.js`. The hook runs after each agent state change and uses the system's native notification API (macOS `osascript`, Linux `notify-send`).
