---
name: chore
description: Lightweight workflow for non-code changes — rules, config, docs, CI, dependency bumps
disable-model-invocation: true
---

Make a non-code change.

Change description: $ARGUMENTS

## Instructions

Follow these phases in order. Each phase has a gate — do not proceed until the gate is satisfied. Apply all project rules and conventions that are in your context.

---

### Phase 1: Branch Setup

1. Derive a short kebab-case name from the change description.
2. Switch to main: `git checkout main`
3. Pull latest: `git pull origin main`
4. Create a new branch from main: `git checkout -b chore/<name>`
   - If the branch already exists, ask the user whether to resume it (`git checkout chore/<name>`) or choose a new name.
5. Confirm the branch: `git branch --show-current`

**Gate:** On the correct `chore/<name>` branch, based on latest main.

---

### Phase 2: Scope

1. Parse the change description — what needs to change and why?
2. Identify the affected files.
3. Confirm this is a non-code change (config, docs, rules, CI, dependencies). If it involves application logic or tests, suggest `/feature` or `/fix` instead. If a rule or config change introduces new agent behavior, consider `/feature` for planning and review.

**Gate:** The scope is clear and limited to non-code changes.

---

### Phase 3: Implement

1. Make the changes.
2. Run `make test` to verify nothing breaks. Skip only if no Makefile exists.

**Gate:** Changes are applied. `make test` passes.

---

### Phase 4: Review

Review all changes for correctness and convention adherence. Apply all project rules and conventions that are in your context.

**Gate:** No issues remain.

---

### Phase 5: Commit and Open PR

1. Commit with a conventional commit message. Use the appropriate type:

   | Type | When |
   |------|------|
   | `chore` | Dependency bumps, version bumps, tooling config |
   | `docs` | Documentation, READMEs, comments |
   | `ci` | CI/CD pipeline changes |
   | `build` | Build system, Makefile changes |

2. Open the PR by invoking **`/agent-dashboard:pr`**. That skill owns the cleanup pass (`refactor-cleaner`), `make fmt`, `make test`, push, and `gh pr create`. Do not call `gh pr create` directly — a `pr-skill-gate` hook will block it.

**Gate:** Clean commit with conventional message. PR opened via `/agent-dashboard:pr`.
