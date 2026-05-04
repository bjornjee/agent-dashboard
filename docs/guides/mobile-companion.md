---
title: Mobile Companion
parent: Guides
nav_order: 6
---

# Mobile Companion

The mobile companion is a PWA (Progressive Web App) that lets you dispatch and manage your agents from your phone over your local network. Approve permissions from the couch, check on long-running tasks from another room, or review diffs on your tablet.

---

## Starting the web server

Build and run the web server:

```bash
make web
```

This starts the server on port 8390. Open `http://<your-machine-ip>:8390` on your phone's browser.

{: .tip }
Find your machine's local IP with `ipconfig getifaddr en0` (macOS) or `hostname -I` (Linux).

### Installing permanently

To install the web server binary to `~/.local/bin/`:

```bash
make install-web
```

Then run it with `agent-dashboard-web`.

## What you can do

The mobile interface mirrors the TUI's capabilities:

- **Agent list and detail views** — same state grouping (needs attention, running, completed) with a collapsible top bar, conversation timeline, and diff viewer
- **Full remote control** — approve/reject permissions, reply with free-text via the PWA reply input, send numbered options, stop agents
- **PR workflow** — open PRs, merge, and close from your phone
- **Session creation** — spawn new agent sessions with z-plugin suggestions and skill selection; new sessions for the same repo reuse an existing tmux pane instead of stacking new ones
- **Usage dashboard** — token breakdown and cost tracking

## Live updates

Detail pages stream updates over Server-Sent Events (SSE), so the conversation, diff, and PR state refresh in place as agents progress — no manual reload required. Once a PR is merged from the dashboard, the underlying tmux pane is cleaned up automatically.

## Browser notifications

Opt-in web notifications nudge you when an agent needs attention even if the tab is backgrounded:

1. Open the dashboard in a desktop or mobile browser
2. Accept the notification permission when prompted
3. Background the tab and switch to other apps — alerts will surface natively

The same `[notifications]` section in `settings.toml` (see [Settings](../../reference/settings/)) controls whether the underlying state changes generate notification events at all.

## Install as PWA

For a native app experience, add the dashboard to your home screen:

1. Open the dashboard URL in your phone's browser
2. Tap **Share** (iOS) or the **three-dot menu** (Android)
3. Select **Add to Home Screen**

The PWA includes offline caching via a service worker, so the shell loads instantly even if your machine is briefly unreachable.

## Google OAuth (optional)

By default, the web server is open to anyone on your local network. To restrict access to just you:

1. Create OAuth credentials in the [Google Cloud Console](https://console.cloud.google.com/apis/credentials)
2. Set the authorized redirect URI to `http://<your-ip>:8390/auth/callback`
3. Configure the environment variables:

```bash
export GOOGLE_CLIENT_ID="your-client-id"
export GOOGLE_CLIENT_SECRET="your-client-secret"
export GOOGLE_ALLOWED_EMAIL="your@gmail.com"
```

When configured, the web server requires Google sign-in and only allows the specified email address.
