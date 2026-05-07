---
name: pr
description: Create a pull request with cleanup, fmt, and test gates
disable-model-invocation: true
---

Open a pull request for the current branch. This is the only sanctioned path
to PR creation — a `pr-skill-gate` hook blocks any direct `gh pr create` that
doesn't carry this skill's bypass marker.

Optional arguments: $ARGUMENTS

## Instructions

Each phase has a gate. Do not proceed until it passes.

---

### Phase 1: Sync state

Run in parallel:

1. `git status` — warn if uncommitted changes (but proceed; the cleanup phase may add commits anyway).
2. `BASE=$(git symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed 's|refs/remotes/origin/||' || echo main)` — detect default branch.
3. `git branch --show-current` — current branch.
4. `git rev-parse --abbrev-ref @{upstream} 2>/dev/null` — does it track a remote?
5. `git log --oneline $(git merge-base HEAD "$BASE")..HEAD` — commits on this branch.
6. `git diff --name-only "$BASE"...HEAD` — files changed vs base. Save this list — it's the input to Phase 2.

**Gate:** You have the changed-file list and the base branch.

---

### Phase 2: Refactor-cleaner pass on the branch diff

1. Spawn the `refactor-cleaner` agent (`run_in_background: false`) with the changed-file list from Phase 1 as scope. Pass file paths explicitly — don't let it roam the whole repo.
2. If the cleaner edited files:
   - Run `make test`. If it fails, fix the regression before continuing.
   - Commit: `git add -u && git commit -m "chore: ai-fmt"`.
3. If the cleaner made no changes, skip the commit.

**Gate:** Either the cleaner made no changes, or its changes are committed and tests are green.

---

### Phase 3: `make fmt`

Check if the target exists first: `make -n fmt >/dev/null 2>&1`.

- **If the target exists:** run `make fmt`. Then check `git status --porcelain`. If anything changed, run `make test`, then commit with `git add -u && git commit -m "chore: fmt"`. If nothing changed, skip the commit.
- **If no Makefile exists, or no `fmt` target:** surface this hint to the user, then proceed:

  > No `make fmt` target found in this repo. Recommend adding one — even a thin wrapper around the language-native formatter (`gofmt -w .`, `ruff format`, `prettier --write`, `cargo fmt`, etc.) is enough to make this gate work and keeps formatting deterministic across contributors. Want me to add it now? (Y/n)

  If the user says yes, add the target *before* opening the PR (a separate `chore: add make fmt target` commit). If no, proceed without formatting.

**Gate:** Either `make fmt` ran clean, or the user has been notified the target is missing.

---

### Phase 4: `make test`

Check if the target exists first: `make -n test >/dev/null 2>&1` (also accept `test-fast` per the `test-gate` hook's preference order).

- **If the target exists:** run it — must pass. If it fails, **stop**. Do not push a broken branch. Fix the failure (likely a separate `fix:` commit) and re-run.
- **If no Makefile exists, or no `test`/`test-fast` target:** surface this hint to the user, then proceed:

  > No `make test` target found in this repo. Recommend adding one (e.g. `test: ; <runner>`) so the PR gate can verify changes pass before opening the PR. Want me to add it now? (Y/n)

  If yes, add it (separate `chore: add make test target` commit) and run it. If no, proceed but note in the PR body that tests were not gated.

**Gate:** Either tests are green, or the user has been notified the target is missing and accepted that trade-off.

---

### Phase 5: Draft

Compose the PR using **all** commits on the branch (not just the latest):

- **Title:** ≤70 chars, conventional format `<type>: <description>`. Pick the type that matches the *primary* change.
- **Summary:** 1–3 bullets — what changed and why. The "why" matters more than the "what".
- **Test plan:** bulleted checklist of how to verify.

Show the draft to the user. Wait for approval or edits.

**Gate:** User approved (or edited) the draft.

---

### Phase 6: Push and create

1. If no upstream: `git push -u origin $(git branch --show-current)`. If upstream exists but is behind: `git push`.
2. Create the PR. **The `AGENT_DASHBOARD_PR_SKILL=1` prefix is mandatory** — without it the `pr-skill-gate` hook blocks the call:

   ```
   AGENT_DASHBOARD_PR_SKILL=1 gh pr create --title "<title>" --body "$(cat <<'EOF'
   ## Summary
   <bullets>

   ## Test plan
   <checklist>
   EOF
   )"
   ```

3. Return the PR URL.

**Gate:** PR is open. URL displayed.

---

## Red Flags — STOP

- "I'll just call `gh pr create` directly" → the hook will block you. Use the prefix.
- "Skip the cleaner, the diff looks clean" → don't. The cleaner is the whole point of this skill.
- "Tests fail but my changes are unrelated" → fix or revert. Never push red.
- "I'll bundle the fmt diff into the feature commit" → keep `chore: fmt` and `chore: ai-fmt` as their own commits; squash happens at merge.
