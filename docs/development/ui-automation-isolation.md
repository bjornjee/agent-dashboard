---
title: UI Automation Isolation
parent: Development
nav_order: 4
---

# UI Automation Isolation

Multiple dashboard agents can work on the same repository at once. UI automation must isolate resources that can conflict while preserving parallelism for headless tests.

## Policy

- Headless Playwright test runs should be parallel and per-worktree.
- Interactive Browser/Chrome inspection should be treated as a shared resource.
- Persistent browser profiles must not be shared between agents.

The operational policy lives in the adapter skill support files:

- `adapters/codex/skills/_shared/ui-automation.md`
- `adapters/claude-code/skills/_shared/ui-automation.md`

Workflow skills reference that policy at planning, environment setup, verification, delegation, and cleanup points instead of duplicating the full text.

## Repo Support

Playwright config should accept agent-local environment variables for server URL, port, and output paths. This lets each worktree run E2E checks without attaching to another agent's dev server or profile.

Interactive browser leasing is separate from Playwright test parallelism. A lease should serialize only shared Browser/Chrome plugin inspection; it should not globally lock all headless Playwright tests.
