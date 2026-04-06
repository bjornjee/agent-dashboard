---
title: Home
layout: home
nav_order: 1
---

# agent-dashboard

Monitor and control your Claude Code agents from terminal and phone.
{: .fs-6 .fw-300 }

A tmux-integrated TUI and mobile web companion that gives you real-time visibility into every Claude Code agent running across your sessions — with full remote control from your phone.
{: .fs-5 .fw-300 }

[Get Started](getting-started){: .btn .btn-primary .fs-5 .mb-4 .mb-md-0 .mr-2 }
[View on GitHub](https://github.com/bjornjee/agent-dashboard){: .btn .fs-5 .mb-4 .mb-md-0 }

---

## Quick Install

```bash
curl -fsSL https://raw.githubusercontent.com/bjornjee/agent-dashboard/main/install.sh | sh
```

Then register the Claude Code plugin:

```
/marketplace add bjornjee/agent-dashboard
/plugin install agent-dashboard@agent-dashboard
```

---

## What you get

### Real-time agent monitoring

Agents grouped by state — needs attention, running, completed — with live tmux pane capture, conversation history, and a subagent tree you can expand and collapse.

### GitHub PR workflow

Review diffs with syntax-highlighted split panes, create PRs, and merge — all without leaving the terminal. Smart context collapsing and sticky function headers keep you oriented in large diffs.

### Mobile remote control

A companion PWA for managing agents from your phone over your local network. Approve permissions, reply to questions, send numbered options, stop agents, and manage PRs — from the couch.

### Session creation

Spin up new agent sessions with z-plugin frecency-ranked path suggestions and skill selection (feature, fix, chore, refactor, investigate, pr, rca).

---

## Demo

<video src="https://github.com/user-attachments/assets/01aa0f85-cfd4-4dc3-ac46-651bcfc03f99" controls muted style="max-width: 100%; border-radius: 8px;"></video>

---

## Works best with

This dashboard pairs well with the [everything-claude-code](https://github.com/affaan-m/everything-claude-code) plugin for a complete agent workflow experience.
