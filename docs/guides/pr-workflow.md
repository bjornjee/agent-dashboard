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

Once a PR is open, press `g` to view the diff in the split-pane viewer. See [Reviewing Diffs](reviewing-diffs) for navigation details.

## Merging

Press `m` to merge the PR via `gh pr merge`. The dashboard sends a cleanup message to the agent afterward.

{: .note }
The GitHub CLI (`gh`) must be installed and authenticated for PR features to work. The dashboard falls back to opening the browser if `gh` is unavailable.

## From mobile

The mobile companion supports the full PR workflow — create, review, merge, and close — from the agent detail view.
