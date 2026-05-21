---
name: implement
description: Dispatch each phase of an approved plan to a fresh subagent with TDD; re-invoke to resume
disable-model-invocation: true
effort: high
---

Implement an approved worktree-local plan phase by phase.

Plan context: $ARGUMENTS

## When To Use

Use this only after the user explicitly chooses `$agent-dashboard:implement` or asks for subagent dispatch. The source plan must already be approved and recorded in `.feature-plan-path` or supplied as an explicit path.

## Instructions

Follow these phases in order. Each phase has a gate.

---

### Phase 1: Locate The Plan

1. If `$ARGUMENTS` is an existing file, use it.
2. Otherwise read `.feature-plan-path` from the worktree root.
3. If neither exists, ask the user for the approved plan path. Use `request_user_input` when available; otherwise ask one concise direct question.
4. Read the plan and find the `## Phases` checklist.

**Gate:** The approved plan file is available and contains a phase checklist.

---

### Phase 2: Resume State

1. Treat `- [x]` phases as complete and `- [ ]` phases as pending.
2. Pick the first pending phase whose dependencies are complete.
3. If no phases are pending, report that implementation is complete.

**Gate:** Exactly one next phase is selected, or all phases are complete.

---

### Phase 3: Dispatch Worker

Dispatch the selected phase with Codex `spawn_agent` using the `worker` role. The worker prompt must include:

1. Exact worktree path and current branch.
2. Exact files or globs owned by the phase.
3. The approved phase text copied inline.
4. Relevant RED/GREEN/TDD rules from the plan.
5. A reminder that other agents may also be editing the codebase and the worker must not revert unrelated changes.
6. The requirement to edit files directly and list changed paths in its final answer.

Do not dispatch overlapping write scopes in parallel. Use one worker per phase unless the approved plan explicitly splits disjoint ownership.

**Gate:** Worker finishes and reports changed paths and verification.

---

### Phase 4: Integrate And Verify

1. Review the worker diff.
2. Run the focused test command for the phase.
3. If the command fails, fix locally or dispatch a bounded follow-up worker with the failing output and exact file scope.
4. Run any broader command required by the plan for that phase.

**Gate:** Phase verification passes.

---

### Phase 5: Mark Complete

1. Update the plan file by changing the selected phase from `- [ ]` to `- [x]`.
2. Run `git diff --check` if available.
3. Continue to the next pending phase by returning to Phase 2, or stop if all phases are complete.

Use `update_plan` for Default-mode progress tracking while dispatching phases.

**Gate:** Plan state reflects completed work and all requested phase verification has passed.

---

## Failure Handling

If a worker fails, summarize the failure and ask whether to retry, skip, or abort. Use `request_user_input` when available; otherwise ask one concise direct question. Do not mark a phase complete unless verification passed or the user explicitly accepts the skip.
