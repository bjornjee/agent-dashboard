---
name: implement
description: Dispatch each phase of an approved plan to a fresh subagent with TDD; re-invoke to resume
when_to_use: when `/feature` (or `/fix`, `/refactor`, `/chore`) has produced a plan with a `## Phases` block and the user opted into dispatch at the probe. Also the resume primitive — re-invoke on a partially-done worktree to pick up at the first pending phase. NOT for fresh features (use `/feature`), inline TDD on small features (also `/feature`), or non-code work.
disable-model-invocation: true
effort: max
---

Run the dispatch loop on a worktree with an approved multi-phase plan.

Opt-in. Invoked after `/feature` (or a sibling task skill) probes the user at plan approval and the user picks "Hand off to /implement". Each phase dispatches to a fresh `Agent()` subagent, keeping the orchestrator session slim. Re-invoking on a partially-done worktree resumes from the first pending phase — no separate resume mode.

## Instructions

Follow these phases in order. Each phase has a gate — do not proceed until the gate is satisfied.

---

### Phase 1: Locate worktree + plan

1. **Confirm cwd is a worktree.** Run `git rev-parse --show-toplevel`; the path must match `.../worktrees/<app>/<name>`. If not, halt: "Run this from inside a feature worktree, not the main checkout."

2. **Read `.feature-plan-path`** from the worktree root. Its single line is the absolute path to the approved plan markdown:
   ```bash
   cat .feature-plan-path
   ```

3. **Fallback if the sentinel is missing** (older `/feature` run, or deleted):
   - `ls -lt ~/.claude/plans/*.md | head -5`
   - Show the top 3 candidates via `AskUserQuestion`, including the current branch name in the prompt for context. User picks one or "Other" to type a path.

4. **Wait for env setup.** Check the worktree root for the env sentinels:
   - `.env-setup-done` → proceed.
   - `.env-setup-failed` → surface contents, halt.
   - Neither → `/feature` Phase 2's background agent is still running. Wait.

**Gate:** Worktree confirmed, plan file path resolved, env setup complete.

---

### Phase 2: Dispatch loop

Read the plan's `## Phases` checklist, dispatch each pending phase to a subagent, verify tests, flip the checkbox. Resume is implicit — phases marked `[x]` are skipped.

1. **Parse the checklist.** Read the plan file (from Phase 1). Extract each `- [ ]` / `- [x]` line under `## Phases`, with its `**Phase X: name**` identifier and deps.

2. **Halt if no `## Phases` block.** The user opted into `/implement` on a plan without phase structure. Surface: "Plan has no `## Phases` block. Either restructure it using the phase format described in `/feature`, or return to `/feature` for inline TDD." Halt.

3. **Pick the next pending phase** (`- [ ]`) in checklist order. If all phases are `[x]`, skip to Phase 3.

4. **Slice the phase body.** Find the matching `### Phase X: name` heading; capture through the next `### ` heading or end-of-file. This is the dispatch unit.

5. **Record pre-state.** `<prev-sha> = git rev-parse HEAD`. The post-dispatch check uses this to confirm one new commit landed.

6. **Dispatch the subagent** (foreground, sequential — never overlap phases). Do not set `model`; the dashboard controls model selection at session level:
   ```
   Agent({
     description: "Phase X dispatch",
     subagent_type: "general-purpose",
     prompt: <subagent prompt template, see below>,
   })
   ```

7. **Verify** when the subagent returns:
   - `make test` in the worktree → must pass. On failure, surface output verbatim.
   - `git log --oneline <prev-sha>..HEAD` → must show exactly one new commit. Zero: subagent didn't commit; halt. Multiple: surface and ask the user to inspect.
   - On any failure, call `AskUserQuestion` with options `["Retry the phase", "Skip and mark done anyway", "Abort the loop"]`.

8. **Mark done.** Edit the plan: flip the matching `- [ ]` → `- [x]`, append the short commit SHA in parens. **Only** the checklist line — don't touch the `### Phase X:` body.
   ```
   - [x] **Phase X: name** ... (a1b2c3d)
   ```

9. **Loop** to step 3.

**Gate:** All phases in the `## Phases` checklist are marked `- [x]` and `make test` is green.

#### Subagent prompt template (verbatim, with `{placeholders}` filled in)

```
You are implementing Phase {ID} of an approved plan.

Worktree: {worktree_path}
Plan file (read-only reference): {plan_path}
Phase ID: {ID}
Phase scope (the only section you should implement):

<<<phase-body>>>
{inline copy of the `### Phase {ID}:` heading and body from the plan}
<<<end-phase-body>>>

Rules:
1. Strict TDD: RED → GREEN → REFACTOR. Run `make test` between each step.
2. Implement ONLY this phase. Do not touch files outside this phase's declared scope.
3. After GREEN + REFACTOR pass, commit with:
     git commit -m "feat({phase-id-kebab}): <one-line description>"
4. Do NOT modify the plan file. Do NOT update checkboxes — the orchestrator owns that.
5. If the phase is ambiguous or blocked, STOP and report a question instead of guessing.

Report back with:
- Commit SHA
- Test status (pass/fail + key output line)
- Files changed (paths)
- Anything unexpected
```

---

### Phase 3: Review

Review all changes for correctness, security, and convention adherence. Apply all project rules and conventions that are in your context.

Review the **full branch diff** (`git diff $(git merge-base HEAD main)..HEAD`), not just the last phase — subagents see only their phase, so cross-phase coherence is the orchestrator's responsibility.

**Gate:** No critical or high-severity issues remain.

---

### Phase 4: Handoff to /pr

Tell the user:

```
Implementation complete. All phases marked done. Open the PR with:

    /agent-dashboard:pr
```

**Gate:** User has been pointed at `/agent-dashboard:pr`.

---

## Resume behavior

No separate resume mode. Re-invoking `/implement` on a partially-done worktree just works:

- Phase 1 finds the plan via `.feature-plan-path`.
- Phase 2 reads the checklist with some `[x]` already; the loop starts at the first `[ ]`.
- Phases 3 and 4 fire only when every phase reaches `[x]`.

The plan file's checkbox state is the source of truth — the orchestrator's in-memory state is disposable. If the session itself got compacted mid-flight, just re-invoke. The in-flight phase whose checkbox is still `- [ ]` will simply be retried.

---

## Red Flags — STOP

If you catch yourself saying or thinking any of these, pause and re-read the relevant phase:

- "I'll dispatch all phases in parallel to save time" → Phase 2 step 6 violation. Phases run sequentially — a later phase may depend on an earlier one's commit.
- "The subagent's tests failed but the diff looks fine, I'll move on" → Phase 2 step 7 violation. Halt and surface to the user.
- "Subagent didn't commit, but the changes are there — I'll mark it done" → Phase 2 step 7 violation. No commit means no phase. Retry or abort.
- "I'll fix a typo in the plan body while I'm flipping the checkbox" → Phase 2 step 8 violation. Checkbox only. Plan-content changes need a separate decision.
- "No `## Phases` block, but I can infer the phases from the prose" → Phase 2 step 2 violation. Halt and point the user back to `/feature` for inline TDD.
- "`.feature-plan-path` is missing, but the latest plan is probably right" → Phase 1 step 3 violation. Show recent plans and let the user confirm.
- "I'll just `gh pr create` to skip Phase 4" → blocked by the `pr-skill-gate` hook. Use `/agent-dashboard:pr`.
