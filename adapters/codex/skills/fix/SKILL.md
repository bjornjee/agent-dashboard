---
name: fix
description: Diagnose and fix a bug in an isolated git worktree with reproduce-first, test-first methodology
disable-model-invocation: true
effort: max
---

Diagnose and fix a bug.

Bug description: $ARGUMENTS

## Instructions

Follow these phases in order. Each phase has a gate. Apply all project rules and conventions already in context.

---

### Phase 1: Setup

1. Derive a short kebab-case name from the bug description.
2. Derive the app name from the git repo: `basename $(git rev-parse --show-toplevel)`.
3. Switch to main: `git checkout main`.
4. Pull latest: `git pull origin main`.
5. Create branch `fix/<name>` and worktree `../worktrees/<app>/<name>` from main:
   `mkdir -p ../worktrees/<app> && git worktree add ../worktrees/<app>/<name> -b fix/<name> main`
   - If the branch already exists, ask the user whether to resume it or choose a new name.
   - Register the worktree with the dashboard:
     `node "$PLUGIN_ROOT/scripts/stamp-worktree.js" "$(cd ../worktrees/<app>/<name> && pwd -P)"`
6. Copy `.env*` files from the source repo into the worktree, preserving relative paths and excluding `.git` and dependency directories.
   - Commands that write outside the current sandbox must use Codex escalation: set `sandbox_permissions` to `require_escalated` with a concise justification.
7. cd into the worktree and confirm with `pwd` and `git branch --show-current`.
8. Verify the env-file list in the source and worktree. If source env files existed and any are missing, halt and report the mismatch.

**Gate:** Working directory is the new worktree on the correct branch, based on latest main. Any source `.env*` files are present in the worktree.

---

### Phase 2: Evidence

Before touching code, collect grounded evidence from observable sources.

1. Parse the bug description for the error, expected behavior, observed behavior, and environment.
2. Collect at least one concrete source: logs, stack trace, reproducible steps, issue details, metrics, or recent git history for affected files.
3. Search the repo for related code and existing tests.
4. Summarize what is known from evidence versus what is still a hypothesis.
5. If a gating decision is missing, ask it before planning. In Plan Mode use `request_user_input` when available; otherwise ask one concise direct question.

**Gate:** At least one evidence source is collected and the likely affected files/tests are identified.

---

### Phase 3: Reproduce RED

1. Write the smallest failing test that reproduces the bug from the evidence.
2. Run the focused test command and confirm it fails for the expected reason.
3. If the failure does not match the observed bug, fix the test before implementation.

**Gate:** A focused RED test fails for the right reason.

---

### Phase 4: Plan The Fix

Use `/plan` or Plan Mode for the fix plan when the change is more than a trivial local edit. `update_plan` is only Default-mode progress tracking; it is not a Plan Mode substitute.

Submit the plan in this exact shape and wait for approval:

```xml
<proposed_plan>
## Summary
<root cause and proposed fix>

## Evidence
<what reproduced and where>

## Affected Files
<files and ownership boundaries>

## Test Plan
<RED command, GREEN command, final verification>

## Risks
<specific risks and mitigations>
</proposed_plan>
```

For a trivial local bug fix, proceed after stating the evidence and focused RED output. Do not include unresolved decisions, TBDs, or placeholders in any submitted plan.

**Gate:** Fix approach is clear and approved when approval is needed.

---

### Phase 5: Implement GREEN

1. Implement the minimum change to pass the RED test.
2. Run the focused test and confirm it passes.
3. Refactor only after GREEN, then rerun the same focused test.
4. Add separate tests for edge cases and error paths when the bug surface warrants them.

**Gate:** Focused tests pass after each code change.

---

### Phase 6: Review And Verify

1. Review the diff for correctness, security, convention adherence, and scope control.
2. Run the full project test command, normally `make test`.
3. Commit with a conventional `fix:` message.
4. Open the PR through `$agent-dashboard:pr` when a PR is requested or expected.

**Gate:** Tests pass and no critical or high-severity review issues remain.

---

## Delegation

Use Codex `spawn_agent` only when explicitly requested. Use `explorer` for independent investigation and `worker` for bounded code changes. Workers must receive exact file paths, the failing test output, the approved fix scope, and a note that other agents may also be editing the codebase.
