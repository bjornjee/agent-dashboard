---
title: PR Workflow
parent: Guides
nav_order: 4
---

# PR Workflow

The dashboard integrates with the GitHub CLI (`gh`) to manage pull requests without leaving the terminal.

---

## Creating a PR

Press `g` on an agent. If no PR exists for the agent's branch, the dashboard creates one using `gh pr create`. The PR title and body are generated from the agent's commit history.

If a PR already exists, `g` opens the PR diff viewer instead.

## Reviewing

Once a PR is open, press `g` to view the diff in the split-pane viewer. See [Reviewing Diffs](../reviewing-diffs/) for navigation details.

## Merging

Press `m` to merge the PR via `gh pr merge`. The dashboard re-checks `gh auth status` synchronously at confirm time, then runs `gh pr merge --squash` and sends a cleanup message to the agent afterward.

If `gh` is missing or not authenticated when you confirm, the merge is cancelled — the dashboard shows `Cannot merge: gh CLI not available — run 'gh auth login' or install gh` and the agent's state is left untouched. Press `s` to open the [dependency status page](../reference/keybindings/) for a full view of which tools are available.

{: .note }
The `g` key (open / create PR) uses the browser as a fallback when `gh` is missing. Only the `m` (merge) flow requires `gh` to be present and authed — it has no safe browser-equivalent.

## From mobile

The mobile companion supports the full PR workflow — create, review, merge, and close — from the agent detail view.

## Invoking via `/agent-dashboard:pr`

Typing `/agent-dashboard:pr` inside a Claude Code session is intercepted by the `UserPromptSubmit` hook and routed to the `pr` skill. This is the same path the dashboard takes when you press `g` to open a new PR — useful when you want to drive PR creation from inside the agent's pane rather than the dashboard.

## Cleanup gate

The `pr` skill runs a cleanup phase before pushing:

- A scratch-artifact sweep removes throwaway files the agent left in the worktree (notes, fixtures, ad-hoc scripts) before they can land in the diff.
- A `test-gate` enforcement hook blocks `gh pr create` / `gh pr merge` if `make test` does not pass — agents cannot ship a red branch.

Both gates fire automatically; no extra keypresses required.
