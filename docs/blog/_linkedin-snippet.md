---
title: LinkedIn snippet (not published)
exclude_from_search: true
sitemap: false
---

> This file lives next to the blog post for convenience. It is **not** published to the site (the leading underscore tells Jekyll to treat the file as a partial). Copy-paste from below into LinkedIn when sharing.

---

## Short version (~600 chars)

I built an orchestrator for Claude Code so I'd stop losing track of which of my five agents was waiting on me.

Along the way I had to learn — the hard way — how skills, rules, hooks, and state files actually behave under load. Three things I won't forget:

→ Hard gates beat guidelines. Every time.
→ Async hook ordering is not guaranteed. Late events overwrite earlier ones.
→ macOS will silently SIGKILL your unsigned binary on sleep/wake. `codesign -s -` saved me.

Wrote it all up here, including the phantom-keystroke war story:
https://bjornjee.github.io/agent-dashboard/blog/building-an-orchestrator-for-claude-code/

Repo: https://github.com/bjornjee/agent-dashboard

---

## Longer version (~1300 chars, with structure)

I run a handful of Claude Code agents in parallel most days. After one too many "wait, which one is blocked?" moments, I built `agent-dashboard` — a tmux-integrated orchestrator that spawns sessions, routes my replies, gates commits, and runs a companion PWA so I can approve permissions from my phone.

What I didn't expect: how much I'd learn about Claude Code's own internals in the process.

A few things that surprised me along the way:

• **Skills are an API.** `/feature`, `/fix`, `/refactor` aren't just prompts — they're phase-gated workflows. Hard gates ("paste failing test output before writing implementation") work where instructions ("please do TDD") don't.

• **Hooks fire much more often than you think.** PreToolUse + PostToolUse on every tool call across all concurrent agents = hundreds of Node spawns/min. Two-tier reporting (fast hot-path, full reporter on lifecycle) cut overhead by ~80%.

• **Async hook ordering bit me.** A late `PostToolUse` overwrote a `Stop` event, leaving agents stuck showing "running" while they were actually waiting at a prompt. Guard your terminal states.

• **macOS killed my binary three different ways** — AMFI on test binaries, taskgated on sleep/wake, and the alt-screen hiding stderr the whole time. Redirect stderr to a file *before* entering alt screen. This is the single most useful TUI debugging tip I have.

Full write-up (skills, rules, hooks, state, phantom keystrokes, the AMFI story):
https://bjornjee.github.io/agent-dashboard/blog/building-an-orchestrator-for-claude-code/

Repo: https://github.com/bjornjee/agent-dashboard
