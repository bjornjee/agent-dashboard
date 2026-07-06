# Skills grading scorecard

Baseline audit of the 8 workflow skills in `adapters/skills-src/`, scored for effectiveness with current frontier models (Claude Fable 5 / Opus 4.8, GPT-5.5 Codex) and minimality. Produced July 2026 by eight parallel cold-context graders, one per skill, then synthesized. This document is the evidence base for a follow-up trim PR; nothing here is a binding change by itself.

Line refs cite `adapters/skills-src/<skill>/SKILL.md` at the state committed alongside this doc (after the marker-pair collapse described below).

> **Status update:** the recommended trims (recs 1–6) landed in PR #396, so the per-skill line refs below describe the pre-trim source. Actual post-trim size is ~1,611 `.md` lines under `adapters/skills-src/` — slightly above the ~1,590 projection because the rca baseline below was miscounted and defence-in-depth lines were deliberately restored after review. Rec 7 (effort notes) remains open.

## Rubric (each dimension scored 1–5, 5 = excellent)

- **Token cost** — is file size proportionate to unique decision content? The full file loads into the agent's context on every invocation.
- **Redundancy** — rules stated more than once in-file; boilerplate duplicated across sibling skills that belongs in `_shared/`.
- **Model-fit** — instructions current models no longer need (anti-rationalization self-talk, triple reinforcement) vs ones that earn their place (exact command sequences, non-obvious constraints).
- **Enforcement overlap** — prose rules whose violation a hook already blocks (`pr-skill-gate`, `block-main-commit`, `commit-lint`, `test-gate`, `pr-detect`, `warn-destructive` — now in **both** harness plugins) can shrink to a one-line pointer; prose-only gates must stay explicit.
- **Cross-harness parity** — size of the `<!-- claude-only -->`/`<!-- codex-only -->` conditional surface; larger = more drift-prone.

## Already applied in this PR (not deferred)

- **30 identical marker pairs collapsed** across chore (4), feature (8), fix (3), implement (7), investigate (3), rca (2), refactor (3) — pairs whose content was byte-identical after normalizing the `/agent-dashboard:` → `$agent-dashboard:` prefix the generator already rewrites globally. Generated adapters proven byte-identical before/after. Chore now needs **zero** conditionals.
- **Sentinel bug fix** — `find . -name '.env*'` now excludes `.env-setup-done`/`.env-setup-failed` in feature/fix/refactor (stale sentinels no longer copied into fresh worktrees).
- **Plugin-root fallback** — Claude-side `claim-worktree.js` invocation no longer depends on `$CLAUDE_PLUGIN_ROOT` being exported.
- **`./ env` typo** in feature's env-copy example fixed.

## Summary

Grades were assessed before the marker-pair collapse; Redundancy and Parity are better than shown for every skill except pr (which had no identical pairs — its conditionals encode real differences).

| Skill | Source lines | Token cost | Redundancy | Model-fit | Enforcement | Parity | Projected after trim |
|---|---|---|---|---|---|---|---|
| chore | 73 | 4 | 2 | 4 | 4 | 1 | ~72 |
| feature | 459 | 3 | 2 | 3 | 3 | 2 | ~384 |
| fix | 210 | 3 | 2 | 4 | 4 | 2 | ~188 |
| implement | 220 | 3 | 2 | 4 | 4 | 2 | ~206 |
| investigate | 74 | 4 | 2 | 3 | 4 | 3 | ~72 |
| pr | 264 | 3 | 2 | 3 | 4 | 3 | ~229 |
| rca | 288 | 4 | 2 | 4 | 3 | 3 | ~278 |
| refactor | 191 | 4 | 3 | 4 | 4 | 3 | ~162 |
| **Total** | **1,779** | | | | | | **~1,590** |

Each source line emits into two adapters, so every trimmed line saves roughly double in shipped plugin content and once per agent invocation in context.

## Cross-cutting recommendations (follow-up PR)

1. **Extract worktree/env-setup boilerplate to `_shared/`.** The Phase 1 worktree+env-copy sequence and the Phase 2 project-type detection table + install/symlink rules are verbatim-triplicated in feature (`feature/SKILL.md:39-105`), fix (`fix/SKILL.md:~30-99`), and refactor (`refactor/SKILL.md:~30-98`). Extract to `_shared/worktree-setup.md` + `_shared/env-setup.md`, referenced the same way skills already reference `_shared/ui-automation.md` (reference-style — the generator copies `_shared/` verbatim; no include mechanism needed). Saves ~60 lines per affected skill across the tree. Risk: harness-conditional fragments (sandbox-escalation wording, two-exec-command split) must stay marker-gated in the per-skill files.
2. **Prune hook-enforced items from "Red Flags" sections.** `gh pr create` bullets (feature `:426+`, pr `:259+`, implement `:197+`) and commit-on-main bullets duplicate `pr-skill-gate`/`block-main-commit`, which block regardless. Keep the one actionable pointer in the phase body (the `AGENT_DASHBOARD_PR_SKILL=1` prefix and the "use the pr skill" instruction); cut the rest.
3. **Remove "Symptoms you're about to violate" scaffolding** (feature `:155`, `:209`). Each list restates the adjacent `<HARD-GATE>`; current models comply with the gate alone.
4. **Neutralize pr's cleaner/worker terminology.** ~10 of pr's conditional regions exist only to swap "refactor-cleaner agent" vs "Codex worker" (`pr/SKILL.md:107-189`). A shared neutral term ("cleanup pass") plus one marker-gated dispatch block per harness removes most of pr's conditional surface.
5. **Unify implement's red-flags list — and restore three checks Codex silently lost.** The codex variant carries only 4 of claude's 7 red flags (missing: plan-body edit guard, `.feature-plan-path` fallback guard, no-commit guard). Merge into one shared list with inline `AskUserQuestion` (Claude) / `request_user_input` (Codex) parentheticals. This is a behavior improvement, not just a trim.
6. **One-line dedupes:** investigate `:41` (third restatement of read-only; `:7` and `:68` suffice), investigate `:13` + chore `:13`/`:54` ("apply all project rules" stated twice), rca `:13` (speculation ban restated at Phase 5's gate), rca `:167-171` (code fence whose body is two marker-gated comment stubs — one prose sentence suffices), refactor `:95` ("Ask the user only if no signal matches" — implied), fix `:104` vs Phase 4's near-synonymous evidence-grounding sentence.
7. **Evaluate the claude-only effort notes** (refactor `:134`, same block in feature/fix): they document dashboard-internal effort plumbing, not agent decisions. Verify the spawn flag is authoritative, then cut or move to README.

## Per-skill notes

### chore — 73 lines
Lean and well-fitted; the commit-type table earns its place, and hook pointers are one-liners. Its Parity grade of 1 reflected 4 identical marker pairs — all collapsed in this PR; it now has zero conditionals. Remaining: the duplicated "apply all project rules" sentence (`:13`/`:54`).

### feature — 459 lines
The heaviest skill and the biggest trim target (~75 more lines). Load-bearing content to keep verbatim: the two-exec-command worktree split rationale, `claim-worktree.js` resolution, the `## Phases` dispatch-format contract, the AskUserQuestion/request_user_input schemas. Cut targets: Red Flags items covered by hooks, "Symptoms" lists, "Why:" rationale prose, and the worktree/env boilerplate once extracted to `_shared/` (rec 1). The codex env-setup block (`:316+`) near-duplicates the claude background-agent block and should unify around the shared extraction with only the launch mechanism marker-gated.

### fix — 210 lines
Strong model-fit: delegation thresholds, two-exec split, and `codex_skill_must` all earn their lines. Remaining work is the `_shared/` extraction (rec 1) and one near-synonymous evidence sentence (`:104`).

### implement — 220 lines
Sequential-dispatch constraint, one-commit-per-phase invariant, and the subagent prompt template are all necessary and correctly sized. Main remaining item is rec 5 (red-flags unification restoring the three lost codex checks); minor: "Loop to step 3" and `<prev-sha>` pseudocode are inferable.

### investigate — 74 lines
Lean. Research-technique specifics (`git log -S`, blame, context7) earn their place. Remaining: the read-only triple-statement and the zero-content "apply project rules" line.

### pr — 264 lines
Best enforcement-overlap discipline of the large skills (the `AGENT_DASHBOARD_PR_SKILL=1` prefix is necessary knowledge, not duplication). The dominant cost is terminology-swap conditionals (rec 4). Also: the codex-only `rm` argument blocklist (`:85-93`) is over-specified — the path-traversal guard above it is the real constraint; the destructive-deletion warning is stated twice in Phase 2.

### rca — 288 lines
Highest-density skill: macOS `log show` predicates, timestamp decoding, the JSONL session-walk script, and Go instrumentation patterns must stay verbatim — models get the syntax subtly wrong without them. Remaining trims are small (recs 6).

### refactor — 191 lines
Best overall grades. Remaining: the `_shared/` extraction (rec 1), the obvious-fallback line (`:95`), and the effort-note evaluation (rec 7).
