# UI Automation Resource Policy

Use this policy when a task plans, sets up, verifies, delegates, or cleans up UI automation.

## Resource Model

- Headless Playwright test runs are parallel work and should stay per-worktree.
- Interactive Browser/Chrome inspection is a shared resource and must be leased or queued when a lease mechanism is available.
- Persistent browser profiles are never shared between agents, worktrees, or test workers.

## Agent Rules

1. Prefer headless Playwright tests for UI verification.
2. Use the interactive Browser/Chrome plugin only for visual inspection or manual interaction that headless tests cannot cover.
3. Allocate UI resources from the current worktree, not from global defaults:
   - unique dev-server port
   - unique `PLAYWRIGHT_BASE_URL`
   - unique Playwright output/artifact directory
   - unique browser `userDataDir` when persistent context is required
4. Do not reuse another agent's running dev server unless the user explicitly asks.
5. Do not use the default Chrome profile or a shared persistent Playwright profile.
6. If an interactive browser lease is unavailable, report the wait state instead of retrying in a loop.
7. Release browser leases and remove worktree-local UI scratch state during cleanup.

## Environment Hints

When the project has browser-based UI tests, environment setup should export worktree-local values before running tests:

```bash
export AGENT_UI_ID="$(basename "$PWD")"
export PLAYWRIGHT_PORT="<worktree-unique-port>"
export PLAYWRIGHT_BASE_URL="http://127.0.0.1:$PLAYWRIGHT_PORT"
export PLAYWRIGHT_USER_DATA_DIR="$PWD/.agent-ui/browser-profile"
export PLAYWRIGHT_OUTPUT_DIR="$PWD/.agent-ui/playwright"
```

Use deterministic free-port selection when the repo already provides one. Otherwise pick a currently free localhost port and keep it in the worktree's environment for the session.

## Delegation Hint

When dispatching a subagent for a UI phase, include this policy by path and pass the resolved port/base URL/profile/output paths in the subagent prompt. Subagents do not inherit session context reliably.
