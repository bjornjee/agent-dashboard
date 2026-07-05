---
name: fix
description: Diagnose and fix a bug in an isolated git worktree with reproduce-first, test-first methodology
disable-model-invocation: true
effort: max
---
<!-- codex-only -->

<codex_skill_must>
1. Run `mkdir -p` and `git worktree add -b fix/<name> ... main` as separate `exec_command` calls.
2. Write and run the reproducing failing test before changing implementation.
3. Change only what makes the reproducing test pass.
4. Allowed tools: `exec_command`, `request_user_input`, `spawn_agent`, `wait_agent`, `apply_patch`.
5. Every `spawn_agent` call must be followed by `wait_agent`.
</codex_skill_must>
<!-- /codex-only -->

Diagnose and fix a bug.

Bug description: $ARGUMENTS

## Instructions

Follow these phases in order. Each phase has a gate — do not proceed until the gate is satisfied. Apply all project rules and conventions that are in your context.

If the bug involves browser UI, Playwright, dev-server ports, screenshots, or interactive Browser/Chrome inspection, apply `../_shared/ui-automation.md` at evidence gathering, environment setup, reproduction, verification, delegation, and cleanup points.

---

### Phase 1: Setup

Follow `../_shared/worktree-setup.md` with branch prefix `fix`.

**Gate:** Working directory is the new worktree on the correct branch, based on latest main. If `.env*` files existed in the source repo, they are all present in the worktree.

---

### Phase 2: Gather Evidence

Start two tracks in parallel:

<!-- claude-only -->
**Background — Environment setup:** Launch a background agent (`run_in_background: true`) to set up the dev environment per `../_shared/env-setup.md`.
<!-- /claude-only -->
<!-- codex-only -->
**Background — Environment setup:** Launch a background `exec_command` to set up the dev environment per `../_shared/env-setup.md`.
<!-- /codex-only -->

**Foreground — Evidence gathering:**

Before touching code, collect **grounded evidence** from observable sources. Do not guess from reading code alone.

1. Take the bug description — this may be an error message, stack trace, issue URL, or user description.
2. Collect evidence from these sources (check all that are available):
   - **Logs:** application logs, server logs, error tracking (Sentry, Datadog, etc.). Ask the user where logs live if not obvious.
   - **Metrics:** dashboards, monitoring, performance counters. Ask for links or screenshots.
   - **Stack traces:** the full trace, not just the top frame. Include line numbers and timestamps.
   - **Steps to reproduce:** exact inputs, environment, and sequence that triggers the bug.
   - **Git history:** `git log --oneline --since="2 weeks ago" -- <affected files>` — what changed recently in the area?
   - **Issue tracker:** if an issue URL was provided, read it fully including comments for additional context.
3. Summarize the evidence. State what is **known** (from logs/metrics/traces) vs what is **hypothesized**.

**Gate:** At least one source of grounded evidence (log, trace, metric, or reproducible steps) is collected. Do not proceed on hypothesis alone.

---

### Phase 3: Reproduce (RED)

**Pre-gate:** Check for `.env-setup-done` in the worktree root.
- If present: verify dependencies are installed (e.g. `node_modules/` exists, `pip list` succeeds, `go env GOPATH` works) and data symlinks resolve correctly.
- If `.env-setup-failed` exists: surface the error and halt.
- If neither file exists: the background agent is still running — wait for it to finish before proceeding.

1. Using the evidence from Phase 2, write a **failing test** that reproduces the bug. The test should:
   - Replicate the exact conditions from the evidence (inputs, state, sequence)
   - Target the specific behavior that is broken
   - Fail for the right reason (matching the observed error, not a typo or import error)
   - Be minimal — test only the broken behavior
2. Run the smallest command that executes the reproducing test and confirm it **fails**.
3. Compare the test failure output against the evidence from Phase 2 — the failure should match the observed bug (same error type, same behavior). If it doesn't, the test is wrong, not the code.
4. Show the failing test output to the user.

**Gate:** A test exists that fails, reproducing the bug. The failure matches the observed evidence.

---

### Phase 4: Diagnose

1. Trace the code path **from the failing test** to identify where the behavior diverges from what is expected. Ground every claim in the Phase 2 evidence and the failing test.
2. Cross-reference with git history: `git log -S "<relevant term>"` and `git bisect` if the bug is a regression.
3. Identify the root cause — explain:
   - What the code **does** (observed via test failure and logs)
   - What it **should do** (expected behavior from evidence)
   - **Why** it diverges (the specific line or logic that causes the mismatch)
4. Present the diagnosis to the user for confirmation. Include evidence citations (log lines, metric values, test output) — not just "I read the code and think X."

**Gate:** User agrees with the root cause analysis. Diagnosis cites observable evidence.

---

### Phase 5: Fix (GREEN)

<!-- claude-only -->
**Effort note:** When launched via the agent-dashboard's New Agent flow, this skill spawns with `--effort high` on the CLI, which Claude Code pins at the session level. The dynamic dispatcher in agent-state-fast.js bumps effort to `max` automatically while `permission_mode='plan'` (EnterPlanMode active) and drops back to `high` on exit — so planning runs at max effort without paying that cost during implementation. When invoked as a slash command inside an existing claude session, you can run `/effort max` before entering plan mode and `/effort high` (or lower) before implementation.
<!-- /claude-only -->
<!-- codex-only -->
**Effort note:** When launched via the agent-dashboard's New Agent flow, this skill starts at implementation effort. Use Codex Plan Mode for high-reasoning diagnosis or planning when the fix needs it, then return to proportional implementation effort after approval.
<!-- /codex-only -->

<!-- claude-only -->
**Delegation gate:** Invoke `/codex:setup` to check Codex CLI availability. If the output contains `"ready": true`, delegate **only if** the user explicitly requested Codex delegation OR the fix touches 10+ files / ~3,000+ lines of implementation. Below that threshold, the orchestration overhead costs more tokens than Claude implementing directly. If delegating, invoke `/codex-delegate` with the diagnosis (Phase 4) and failing test (Phase 3) as implementation context, then skip to the phase gate. Otherwise, proceed below.
<!-- /claude-only -->
<!-- codex-only -->
**Delegation gate:** Use Codex `spawn_agent` **only if** the user explicitly requested subagents OR the fix touches 10+ files / ~3,000+ lines of implementation. Below that threshold, the orchestration overhead costs more tokens than implementing directly. If delegating, pass the diagnosis (Phase 4), failing test output (Phase 3), exact file paths, and a bounded write scope to a `worker`; then call `wait_agent`, review the result, and verify locally. Otherwise, proceed below.
<!-- /codex-only -->

1. Implement the **minimal fix** — change only what is necessary to fix the bug.
2. Run the reproducing test command — the previously failing test must now **pass**.
3. Run full `make test`/`make test-fast` only when the fix crosses packages, touches shared state/test/build infrastructure, or the risk cannot be bounded. Otherwise rely on the targeted reproducing command here; `/agent-dashboard:pr` owns the final branch-wide gate.
4. Show the passing test output and name whether full-suite verification was required.

For UI fixes, prefer headless Playwright with worktree-local resources. Use interactive Browser/Chrome inspection only when the shared policy says it is warranted.

**Gate:** The reproducing test passes. Full-suite verification ran only when required by risk. No unrelated changes.

---

### Phase 6: Refactor

1. Review the fix — is there a cleaner way to express it? Unnecessary duplication?
2. If changes are needed, make them and rerun the reproducing proof command; escalate to full `make test`/`make test-fast` if the refactor widens scope.
3. If no refactoring is needed, skip this phase.

**Gate:** The relevant proof passes. Code is clean.

---

### Phase 7: Review, Commit, and Open PR

1. Review all changes for correctness, security, and convention adherence.
2. Commit with a `fix:` conventional commit message that describes what was fixed and why.
3. Open the PR by invoking **`/agent-dashboard:pr`**. That skill owns conditional cleanup/formatting, final test gating when available, push, and `gh pr create`. Do not call `gh pr create` directly — a `pr-skill-gate` hook will block it.

**Gate:** Clean commit with conventional message. No critical or high-severity review issues. PR opened via `/agent-dashboard:pr`.

---

### Phase 8: Cleanup (on merge)

Triggered when the user indicates the fix has been merged upstream.

1. Verify the branch is merged (warn if unmerged commits remain)
2. Tear down environment resources: remove symlinks, stop dev servers or emulators, release any browser lease, remove worktree-local UI scratch state, delete `.env-setup-done`/`.env-setup-failed` sentinel files
3. Remove worktree and delete branch
4. Confirm cleanup is complete
