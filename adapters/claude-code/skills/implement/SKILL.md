---
name: implement
description: Dispatch the phases of an approved plan to fresh subagents, with TDD per phase. Re-invoke to resume.
when_to_use: when `/feature` (or `/fix`, `/refactor`, `/chore`) has produced a plan with a `## Phases` block and the user opted into dispatch at the probe. Also the resume primitive — re-invoke on a partially-done worktree to pick up at the first pending phase. NOT for fresh features (use `/feature`), inline TDD on small features (also `/feature`), or non-code work.
disable-model-invocation: true
effort: max
---

Run the dispatch loop on a worktree with an approved multi-phase plan.

This skill is opt-in: it's invoked after `/feature` (or sibling task skills) probes the user at the end of plan approval and the user picks "Hand off to /implement". Each phase of the plan dispatches to a fresh `Agent()` subagent, keeping the main orchestrator session slim. Re-invoking the skill on a partially-done worktree resumes naturally from the first pending phase.

## Instructions

Follow these phases in order. Each phase has a gate — do not proceed until the gate is satisfied.

---

### Phase 1: Locate worktree + plan

1. **Verify cwd is inside a worktree.** Run `git rev-parse --show-toplevel`. The path must match `.../worktrees/<app>/<name>` (created by `/feature` or a sibling task skill). If it doesn't, halt and tell the user: "Run this from inside a feature worktree, not the main checkout."

2. **Read `.feature-plan-path`** from the worktree root:
   ```bash
   cat .feature-plan-path
   ```
   The single line is the absolute path to the approved plan markdown (typically under `~/.claude/plans/`).

3. **Fallback if `.feature-plan-path` is missing.** This means `/feature` was run with an older version that didn't write the sentinel, or it was deleted. Recover:
   - List recent plan files: `ls -lt ~/.claude/plans/*.md | head -5`
   - Show top 3 candidates to the user via `AskUserQuestion` with the current branch name in the prompt. Let them pick or "Other" to type a path.

4. **Wait for env setup if pending.** Check `.env-setup-done` / `.env-setup-failed` in the worktree root:
   - `.env-setup-done` exists → proceed.
   - `.env-setup-failed` exists → surface the contents and halt.
   - Neither → the background agent from `/feature` Phase 2 is still running. Wait for it to finish before proceeding.

**Gate:** Worktree confirmed, plan file path resolved, env setup complete.

---

### Phase 2: Dispatch loop

The loop reads the plan's `## Phases` checklist, picks the next pending phase, dispatches it to a subagent, verifies tests, and flips the checkbox. Resume is implicit — checkboxes already marked `[x]` are skipped.

1. **Read the plan file** (from Phase 1). Parse the `## Phases` block: extract each `- [ ]` / `- [x]` line and its `**Phase X: name**` identifier, dependencies, and model preference.

2. **Halt if no `## Phases` block.** This means the user opted into `/implement` on a plan without phase structure. Tell them: "Plan has no `## Phases` block. Either restructure the plan to use the phase format (see `/feature` Phase 2 step 3), or return to `/feature` for inline TDD." Halt.

3. **Pick the next pending phase** (`- [ ]`) in dependency order. If every phase is `[x]`, skip to Phase 3.

4. **Read the phase body slice.** Find the matching `### Phase X: name` heading and capture everything up to the next `### ` heading or end-of-file. This is the dispatch unit.

5. **Capture pre-state.** Record `git rev-parse HEAD` as `<prev-sha>` so the post-dispatch check can verify exactly one new commit landed.

6. **Spawn the subagent.** Call:
   ```
   Agent({
     description: "Phase X dispatch",
     subagent_type: "general-purpose",
     model: <phase's model field, default "sonnet">,
     prompt: <subagent prompt template, see below>,
   })
   ```
   Run in foreground (`run_in_background: false`). Phases must be sequential — never dispatch the next phase until the current returns. Dependencies in the checklist are advisory; sequential order is the safe default.

7. **Verify post-state when the subagent returns:**
   - Run `make test` in the worktree. Must pass. If it fails, surface the failure verbatim.
   - `git log --oneline <prev-sha>..HEAD` — must show exactly one new commit. If zero commits: the subagent didn't commit; surface and halt. If multiple commits: surface and ask the user to inspect.
   - On any failure, call `AskUserQuestion` with options: `["Retry the phase", "Skip and mark done anyway", "Abort the loop"]`.

8. **Mark the phase done.** Edit the plan file: flip the matching `- [ ]` to `- [x]` and append the short commit SHA in parentheses:
   ```
   - [x] **Phase X: name** ... (a1b2c3d)
   ```
   Do not touch the `### Phase X:` body — only the checklist line.

9. **Loop** back to step 3.

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

For multi-phase dispatch the review should cover the full diff (`git diff $(git merge-base HEAD main)..HEAD`), not just the last phase — subagents see only their phase, so cross-phase coherence is the orchestrator's responsibility.

**Gate:** No critical or high-severity issues remain.

---

### Phase 4: Handoff to /pr

Tell the user:

```
Implementation complete. All phases marked done. Open the PR with:

    /agent-dashboard:pr
```

Do not call `gh pr create` directly — a `pr-skill-gate` hook will block it.

**Gate:** User has been pointed at `/agent-dashboard:pr`.

---

## Resume behavior

There is no separate "resume" mode. Re-invoking `/implement` on a partially-done worktree just works:

- Phase 1 still finds the plan via `.feature-plan-path`.
- Phase 2 finds the checklist with some `[x]` already; the loop starts at the first `[ ]`.
- Phase 3 + 4 fire only when all phases reach `[x]`.

If the orchestrator session itself was compacted mid-flight, you may have lost the dispatch loop's in-memory state — but the plan file's checkbox state is the source of truth. Just re-invoke and the loop picks up exactly where it left off (the in-flight phase may need to be retried, since its checkbox is still `- [ ]`).

---

## Red Flags — STOP

If you catch yourself saying or thinking any of these, pause and re-read the relevant phase:

- "I'll dispatch all phases in parallel to save time" → Phase 2 violation. Phases run sequentially; a later phase may depend on an earlier one's commit.
- "The subagent reported test failure but I'll continue anyway" → Phase 2 step 7 violation. Halt and surface to the user.
- "I'll mark the phase done even though the subagent didn't commit" → Phase 2 step 7 violation. No commit means no phase. Either retry or have the user abort.
- "I'll modify the plan body to fix a typo while I'm in there" → Phase 2 step 8 violation. Only flip the checkbox. Plan content changes need a separate decision.
- "Plan has no `## Phases` block, but I'll dispatch heuristically" → Phase 2 step 2 violation. Halt and point the user to inline TDD.
- "`.feature-plan-path` is missing — I'll guess at the plan" → Phase 1 step 3 violation. Show the recent plans and let the user confirm.
- "I'll call `gh pr create` directly to skip Phase 4" → blocked by the `pr-skill-gate` hook. Use `/agent-dashboard:pr`.
