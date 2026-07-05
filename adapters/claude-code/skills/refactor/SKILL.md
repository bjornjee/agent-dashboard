---
name: refactor
description: Safely restructure code in an isolated git worktree with test-preserved, incremental transformations
disable-model-invocation: true
effort: max
---

Safely refactor code while preserving all existing behavior.

Refactoring goal: $ARGUMENTS

## Instructions

Follow these phases in order. Each phase has a gate — do not proceed until the gate is satisfied. Apply all project rules and conventions that are in your context.

If the refactor touches browser UI, Playwright, dev-server ports, screenshots, or interactive Browser/Chrome inspection, apply `../_shared/ui-automation.md` at scoping, environment setup, baseline, verification, delegation, and cleanup points.

---

### Phase 1: Setup

Follow `../_shared/worktree-setup.md` with branch prefix `refactor`.

**Gate:** Working directory is the new worktree on the correct branch, based on latest main. If `.env*` files existed in the source repo, they are all present in the worktree.

---

### Phase 2: Scope

Start two tracks in parallel:

**Background — Environment setup:** Launch a background agent (`run_in_background: true`) to set up the dev environment per `../_shared/env-setup.md`.

**Foreground — Scoping:**

1. Parse the refactoring goal — what is being restructured and why?
2. Identify all affected files by searching the codebase for the code to be changed and its dependents.
3. Check test coverage for the affected code — what tests exist? What is untested?
4. If test coverage is insufficient for safe refactoring, **tell the user** and suggest writing tests first before refactoring.

**Gate:** The scope is clear. Affected files and their test coverage are identified.

---

### Phase 3: Baseline

**Pre-gate:** Check for `.env-setup-done` in the worktree root.
- If present: verify dependencies are installed (e.g. `node_modules/` exists, `pip list` succeeds, `go env GOPATH` works) and data symlinks resolve correctly.
- If `.env-setup-failed` exists: surface the error and halt.
- If neither file exists: the background agent is still running — wait for it to finish before proceeding.

1. Choose a baseline proof command from the affected scope. Prefer the smallest package/test command that covers the code being moved; use full `make test`/`make test-fast` when the refactor crosses packages, touches shared infrastructure, or cannot be bounded.
2. Run that baseline proof command.
3. If it fails, **stop and report**. Do not refactor on a broken codebase. Suggest using `/agent-dashboard:fix` first.
4. Record the command and output as the regression baseline.

For UI baselines, prefer headless Playwright with worktree-local resources. Use interactive Browser/Chrome inspection only when the shared policy says it is warranted.

**Gate:** The scoped baseline passes. The proof command is recorded.

---

### Phase 4: Transform

**Effort note:** When launched via the agent-dashboard's New Agent flow, this skill spawns with `--effort high` on the CLI, which Claude Code pins at the session level. The dynamic dispatcher in agent-state-fast.js bumps effort to `max` automatically while `permission_mode='plan'` (EnterPlanMode active) and drops back to `high` on exit — so planning runs at max effort without paying that cost during implementation. When invoked as a slash command inside an existing claude session, you can run `/effort max` before entering plan mode and `/effort high` (or lower) before implementation.

**Delegation gate:** Invoke `/codex:setup` to check Codex CLI availability. If the output contains `"ready": true`, delegate **only if** the user explicitly requested Codex delegation OR the refactor touches 10+ files / ~3,000+ lines of implementation. Below that threshold, the orchestration overhead costs more tokens than Claude implementing directly. If delegating, invoke `/codex-delegate` with the scope (Phase 2) and baseline (Phase 3) as implementation context, then skip to the phase gate. Otherwise, proceed below.

Apply the refactoring in small, atomic steps. For each step:

1. Make a single, focused change (e.g., extract a function, rename a variable, move a file).
2. Run the baseline proof command immediately after the change.
3. If tests fail:
   - Revert only the changed files (`git checkout -- <file1> <file2> ...`)
   - Analyze why it failed
   - Try a different approach
4. If tests pass, proceed to the next step.

Do not batch multiple changes between proof runs. One change, one proof run.

**Gate:** All transformations applied. The scoped proof passes after each step.

---

### Phase 5: Cleanup

1. Remove dead code — unused imports, functions, variables, files.
2. Update any affected documentation or comments.
3. Run the baseline proof one final time; run full `make test`/`make test-fast` only if the cleanup widened the scope.

**Gate:** No dead code remains. The relevant proof passes.

---

### Phase 6: Review, Commit, and Open PR

1. Review all changes for correctness, security, and convention adherence.
2. Verify that behavior is preserved — no new features, no bug fixes, only structural changes.
3. Commit with a `refactor:` conventional commit message that describes what was restructured and why.
4. Open the PR by invoking **`/agent-dashboard:pr`**. That skill owns conditional cleanup/formatting, final test gating when available, push, and `gh pr create`. Do not call `gh pr create` directly — a `pr-skill-gate` hook will block it.

**Gate:** Clean commit with conventional message. Behavior is unchanged. No critical or high-severity review issues. PR opened via `/agent-dashboard:pr`.

---

### Phase 7: Cleanup (on merge)

Triggered when the user indicates the refactor has been merged upstream.

1. Verify the branch is merged (warn if unmerged commits remain)
2. Tear down environment resources: remove symlinks, stop dev servers or emulators, release any browser lease, remove worktree-local UI scratch state, delete `.env-setup-done`/`.env-setup-failed` sentinel files
3. Remove worktree and delete branch
4. Confirm cleanup is complete
