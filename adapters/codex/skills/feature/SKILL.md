---
name: feature
description: Start a new feature in an isolated git worktree with TDD workflow
when_to_use: when the user says "start a feature", "new feature", invokes "$agent-dashboard:feature", or describes work that needs an isolated branch + worktree + TDD loop. NOT for hotfixes, single-file edits, pure exploration, or non-code changes (use $agent-dashboard:chore, $agent-dashboard:fix, $agent-dashboard:investigate instead).
version: 1.0.0
disable-model-invocation: true
effort: max
---

Start a new feature in an isolated git worktree.

Feature description: $ARGUMENTS

## Instructions

Follow these phases in order. Each phase has a gate. Apply all project rules and conventions already in context.

---

### Phase 1: Setup

1. Derive a short kebab-case name from the description.
2. Derive the app name from the git repo: `basename $(git rev-parse --show-toplevel)`.
3. Switch to main: `git checkout main`.
4. Pull latest: `git pull origin main`.
5. Create branch `feat/<name>` and worktree `../worktrees/<app>/<name>` from main:
   `mkdir -p ../worktrees/<app> && git worktree add ../worktrees/<app>/<name> -b feat/<name> main`
   - If the branch already exists, ask the user whether to resume it or choose a new name.
   - Register the worktree with the dashboard:
     `node "$PLUGIN_ROOT/scripts/stamp-worktree.js" "$(cd ../worktrees/<app>/<name> && pwd -P)"`
6. Copy `.env*` files from the source repo into the worktree, preserving relative paths and excluding `.git` and dependency directories.
   - Commands that write outside the current sandbox must use Codex escalation: set `sandbox_permissions` to `require_escalated` with a concise justification.
7. cd into the worktree and confirm with `pwd` and `git branch --show-current`.
8. Verify the env-file list in the source and worktree. If source env files existed and any are missing, halt and report the mismatch.

**Gate:** Working directory is the new worktree on the correct branch, based on latest main. Any source `.env*` files are present in the worktree.

---

### Phase 2: Research And Plan

Run research before writing code.

1. Search the repo for existing implementation, tests, conventions, and dependency choices. Use `rg`/`rg --files` first.
2. For non-trivial independent codebase questions, use Codex `spawn_agent` with the `explorer` role. Pass exact file paths and enough context for the explorer to start without this session history.
3. If a gating decision is missing, ask it before planning.
   - In Plan Mode, use `request_user_input` when the tool is available.
   - If `request_user_input` is unavailable, ask one concise direct question in assistant text.
4. Use `/plan` or Plan Mode as the official planning primitive. `update_plan` is only Default-mode progress tracking; it is not a Plan Mode substitute.
5. Submit the plan in this exact shape and wait for approval:

```xml
<proposed_plan>
## Summary
<what will change and why>

## What Already Exists
<one-line research answer>

## Affected Files
<files and ownership boundaries>

## Phases
- [ ] **Phase A: <short name>** - files: <globs>, deps: -
- [ ] **Phase B: <short name>** - files: <globs>, deps: A

## Test Plan
<RED command, GREEN command, final verification>

## Risks
<specific risks and mitigations>
</proposed_plan>
```

For fewer than three work units, omit the `## Phases` checklist and describe the implementation directly. Do not include unresolved decisions, TBDs, or placeholders.

6. After approval, write the approved plan into a worktree-local markdown file, then record its absolute path:
   `printf '%s\n' "<absolute-plan-path>" > .feature-plan-path`
7. If the approved plan has three or more phases, ask whether to continue inline or use `$agent-dashboard:implement` for phase dispatch. Use `request_user_input` when available; otherwise ask directly.

**Gate:** Research is summarized, required decisions are resolved, and the user has approved the plan.

---

### Phase 3: Implement With TDD

1. Write the smallest failing test for the first behavior. Run the focused command and confirm it fails for the expected reason.
2. Implement the minimum code to pass.
3. Run the focused test and confirm it passes.
4. Refactor only after GREEN, then rerun the same test.
5. Repeat for each behavior. Keep tests atomic: one assertion focus per test, with golden path, edge cases, and error paths split.

Use `update_plan` to track Default-mode implementation progress after approval.

**Gate:** Every change follows RED -> GREEN -> REFACTOR and the focused tests pass after each code change.

---

### Phase 4: Review

Review the diff for correctness, security, convention adherence, and scope control. Address critical and high issues. Fix medium issues when cheap.

**Gate:** No critical or high-severity review issues remain.

---

### Phase 5: Commit And PR

1. Run the full project test command, normally `make test`.
2. Commit with a conventional `feat:` message.
3. Open the PR through `$agent-dashboard:pr`. That skill owns cleanup, formatting, final tests, push, and PR creation.

**Gate:** Tests pass, commit is clean, and the PR workflow owns publishing.

---

## Delegation

Use `spawn_agent` only when the user explicitly asks for subagents or when `$agent-dashboard:implement` is selected after plan approval. Use `explorer` for read-only codebase questions and `worker` for bounded implementation. Workers must receive exact file paths, the approved phase text, and a note that other agents may also be editing the codebase.
