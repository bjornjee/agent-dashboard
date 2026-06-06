# Product

## Register

product

## Users

Senior engineers running multiple AI coding agents (Claude Code, Codex) in parallel inside tmux. They are at a laptop most of the day but switch to their phone when they leave the desk and an agent needs them. Their context when using the dashboard is "I have 3-8 agents in various states; which one needs me, what state is it in, and what's the fastest way to unblock it?"

## Product Purpose

A control plane for AI coding agents. Reads per-agent state files written by Claude Code and Codex adapters, groups agents by state (blocked, running, review, PR, merged), and exposes the orchestration surface in two parallel renderers — a Bubble Tea TUI on the laptop and a PWA on the phone. Success looks like: zero alt-tabbing between tmux panes; every agent's blocked-state is visible at a glance; permissions, replies, PRs and merges all resolvable without leaving the dashboard.

It is not a code editor and not a session manager. It does not own the agents; it understands them.

## Brand Personality

Terse, expert, no-chrome. Workflow-tool voice. The README says "workflow gates, not vibes." The dashboard agrees: short copy, expert assumptions, decoration only where it serves a glance. Personality affordances (ASCII pet, dino game, axolotl banner) exist but are opt-in and live in left-panel chrome, never in the agent-state surface.

## Anti-references

- **Generic SaaS dashboard.** Cards-with-arrows, gradient hero panel, purple/indigo accent stripes, vague "workflow" or "supercharge" copy. The Linear / Notion / Stripe-clone stack is the canonical wrong answer here.
- **VS Code / IDE chrome.** Tree pane + status bar + bottom panel + collapsible sidebar minimaps. This is not a code editor and must not pretend to be one.
- **Marketing landing pages.** Scroll-driven sections, big `clamp()` display typography, eyebrow-heading-CTA stacks, social proof rails, "trusted by" logo grids. Agent-dashboard is the surface a power user lives inside; design ≠ product.

## Design Principles

1. **State first, chrome second.** The agent's current state is the largest signal on every surface. Tabs, sidebars, headers, and toolbars all subordinate to "what does this agent need from me right now?"
2. **Two renderers, one mental model.** TUI and PWA share state-group order, action vocabulary (approve / reply / merge / stop), and visual register intent. A user moving between them should not relearn the layout.
3. **Friction at writes, not reads.** Reading agent state is free and ambient (SSE, polling, live capture). Writes (approve, merge, stop) require a deliberate tap and confirm where stakes are high; never auto-action because "it looked safe."
4. **The phone is not a degraded laptop.** The PWA is its own register — single-column, swipeable, keyboard-aware, optimised for one-handed reply. Not a responsive shrink of the TUI's two-pane layout.
5. **Workflow gates, not vibes.** Hooks (commit-lint, test-gate, no-commits-to-main, destructive-op warnings) enforce TDD and conventional commits at the harness level. The dashboard surfaces the gates honestly rather than hiding them as "magic."

## Accessibility & Inclusion

- Target WCAG AA on body text. The current iOS-system palette mostly passes; the `--text-muted` color (#98989A on #000) reaches ~4.7:1 against base bg.
- Reduced motion: `prefers-reduced-motion: reduce` should crossfade or skip every animation. The new carousel relies on browser-native `scroll-behavior: smooth` and a 160ms transition on pager dots; both must collapse under reduced motion.
- Touch targets: minimum 44×44 on the PWA. The Send button + radio rows already exceed this; the pager dots are 6px circles and are decorative status indicators, not tap targets (declared in `.uiux-loop/register.md`).
- Color-only signals: avoid them. State group is also conveyed by header label + position, not just by accent color.
