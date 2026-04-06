---
title: Home
layout: home
nav_order: 1
---

# agent-dashboard
{: .fs-9 }

Monitor and control your Claude Code agents from terminal and phone.
{: .fs-6 .fw-300 }

A tmux-integrated TUI and mobile web companion that gives you real-time visibility into every Claude Code agent running across your sessions.
{: .fs-5 .fw-300 }

[Get Started](getting-started){: .btn .btn-primary .fs-5 .mb-4 .mb-md-0 .mr-2 }
[View on GitHub](https://github.com/bjornjee/agent-dashboard){: .btn .fs-5 .mb-4 .mb-md-0 }

---

## Demo

<video src="https://github.com/user-attachments/assets/01aa0f85-cfd4-4dc3-ac46-651bcfc03f99" controls muted style="max-width: 100%; border-radius: 8px;"></video>

---

## What you get

<div class="feature-grid" markdown="0">
  <div class="feature-card">
    <div class="feature-icon">📡</div>
    <h3>Real-time agent monitoring</h3>
    <p>Agents grouped by state with live tmux pane capture, conversation history, and a collapsible subagent tree.</p>
  </div>
  <div class="feature-card">
    <div class="feature-icon">🔀</div>
    <h3>GitHub PR workflow</h3>
    <p>Review syntax-highlighted split-pane diffs, create PRs, and merge — all without leaving the terminal.</p>
  </div>
  <div class="feature-card">
    <div class="feature-icon">📱</div>
    <h3>Mobile remote control</h3>
    <p>A companion PWA for managing agents from your phone. Approve permissions, reply, stop agents, and manage PRs.</p>
  </div>
  <div class="feature-card">
    <div class="feature-icon">🚀</div>
    <h3>Session creation</h3>
    <p>Spin up new agent sessions with frecency-ranked path suggestions and skill selection.</p>
  </div>
</div>

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

{: .tip }
This dashboard pairs well with [everything-claude-code](https://github.com/affaan-m/everything-claude-code) for a complete agent workflow experience.
