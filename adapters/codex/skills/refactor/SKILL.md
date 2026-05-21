---
name: refactor
description: Safely restructure code in an isolated git worktree with test-preserved, incremental transformations
disable-model-invocation: true
effort: max
---

Safely refactor code while preserving all existing behavior.

Refactoring goal: $ARGUMENTS

## Instructions

Follow these phases in order. Each phase has a gate. Apply all project rules and conventions already in context.

---

### Phase 1: Setup

1. Derive a short kebab-case name from the refactoring goal.
2. Derive the app name from the git repo: `basename $(git rev-parse --show-toplevel)`.
3. Switch to main: `git checkout main`.
4. Pull latest: `git pull origin main`.
5. Create branch `refactor/<name>` and worktree `../worktrees/<app>/<name>` from main:
   `mkdir -p ../worktrees/<app> && git worktree add ../worktrees/<app>/<name> -b refactor/<name> main`
   - If the branch already exists, ask the user whether to resume it or choose a new name.
   - Register the worktree with the dashboard:
     `node "$PLUGIN_ROOT/scripts/stamp-worktree.js" "$(cd ../worktrees/<app>/<name> && pwd -P)"`
6. Copy `.env*` files from the source repo into the worktree, preserving relative paths and excluding `.git` and dependency directories.
   - Commands that write outside the current sandbox must use Codex escalation: set `sandbox_permissions` to `require_escalated` with a concise justification.
7. cd into the worktree and confirm with `pwd` and `git branch --show-current`.
8. Verify the env-file list in the source and worktree. If source env files existed and any are missing, halt and report the mismatch.

**Gate:** Working directory is the new worktree on the correct branch, based on latest main. Any source `.env*` files are present in the worktree.

---

### Phase 2: Scope

1. Search the repo for the code to restructure and its dependents.
2. Identify all affected files and existing tests.
3. If test coverage is insufficient to preserve behavior, tell the user and propose characterization tests first.
4. If a gating decision is missing, ask it before planning. In Plan Mode use `request_user_input` when available; otherwise ask one concise direct question.

**Gate:** Scope, affected files, and coverage gaps are known.

---

### Phase 3: Baseline

1. Run the focused tests for the affected area.
2. Run the broader baseline, normally `make test`.
3. If the baseline fails, stop and report. Do not refactor on a broken baseline unless the user changes the task to fixing the failure first.

**Gate:** Baseline tests pass.

---

### Phase 4: Plan

Use `/plan` or Plan Mode for the refactor plan. `update_plan` is only Default-mode progress tracking; it is not a Plan Mode substitute.

Submit the plan in this exact shape and wait for approval:

```xml
<proposed_plan>
## Summary
<what structure changes and why behavior stays the same>

## What Already Exists
<current pattern and tests>

## Affected Files
<files and ownership boundaries>

## Transformation Steps
<small behavior-preserving steps>

## Test Plan
<baseline command and after-each-step command>

## Risks
<specific risks and mitigations>
</proposed_plan>
```

Do not include unresolved decisions, TBDs, or placeholders.

**Gate:** User has approved the behavior-preserving plan.

---

### Phase 5: Transform

1. Make one focused refactor step.
2. Run the focused test command immediately.
3. If tests fail, stop, inspect the failure, and repair the step before continuing.
4. Repeat until all planned transformations are complete.
5. Run the broader baseline again.

Use `update_plan` to track Default-mode progress after approval.

**Gate:** Tests pass after each step and after the final transformation.

---

### Phase 6: Review, Commit, And PR

1. Remove dead code and update affected docs when needed.
2. Review the diff for behavior preservation, correctness, security, convention adherence, and scope control.
3. Commit with a conventional `refactor:` message.
4. Open the PR through `$agent-dashboard:pr` when a PR is requested or expected.

**Gate:** Behavior is preserved, tests pass, and no critical or high-severity review issues remain.

---

## Delegation

Use Codex `spawn_agent` only when explicitly requested. Use `explorer` for independent dependency mapping and `worker` for bounded transformation steps. Workers must receive exact file paths, the approved phase text, the relevant baseline output, and a note that other agents may also be editing the codebase.
