---
name: chore
description: Lightweight workflow for non-code changes — rules, config, docs, CI, dependency bumps
when_to_use: when the user asks for a non-code change — config edit, dep bump, docs/README/comment update, CI pipeline tweak, version bump, tooling change. NOT for application logic, behavior changes, or anything requiring tests (use /feature or /fix instead).
version: 1.0.0
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
3. **Confirm this is a non-code change** (config, docs, rules, CI, dependencies). If it touches application logic, behavior, or tests, **STOP**. Hand off to `/feature` or `/fix` and abandon this skill. **No exceptions:**
   - Don't "just edit one .go/.ts/.py file while you're here".
   - Don't ship a config change that introduces new agent behavior without going through `/feature` for review.
   - Don't classify a behavior-changing rule edit (e.g. agent dispatch, hook logic) as a "doc change".

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

### Phase 5: Commit

Commit with a conventional commit message. Use the appropriate type:

| Type | When |
|------|------|
| `chore` | Dependency bumps, version bumps, tooling config |
| `docs` | Documentation, READMEs, comments |
| `ci` | CI/CD pipeline changes |
| `build` | Build system, Makefile changes |

**Gate:** Clean commit with conventional message.

---

## Red Flags — STOP

If you catch yourself saying or thinking any of these, pause and re-read the relevant phase:

- "I'll just tweak this one .go/.ts/.py file too" → wrong skill. Hand off to `/fix` or `/feature`.
- "This rule change is just docs, no review needed" → if it changes agent behavior, treat it as a feature.
- "Skip `make test`, the change is trivial" → run it. Trivial config edits break builds all the time.
- "I'll bundle a stray refactor into this commit" → split it. Open a separate PR.
- "Bump the plugin version while I'm here" → don't. release-please owns version bumps.
