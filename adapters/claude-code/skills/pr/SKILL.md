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
6. `git diff --name-only "$BASE"...HEAD` — files changed vs base. Save this list — it's the input to Phase 3.

**Gate:** You have the changed-file list and the base branch.

---

### Phase 2: Clean up scratch artifacts

Delete transient files left over from implementation, testing, and discovery —
screenshots, Playwright MCP output, and tmp scratch — before the cleanup pass
inspects the diff. **Untracked only.** Never touch tracked or staged files.

1. Identify untracked artifacts. Use `git ls-files --others --exclude-standard`
   for unignored untracked files, and `git ls-files --others --ignored
   --exclude-standard` for ignored untracked files. Filter for these patterns:
   - `*.png` at the repo root only — these are typically Playwright/visual-audit screenshots. PNGs inside subdirectories (build outputs `out/`, `dist/`, `build/`, asset dirs `public/`, `static/`, source dirs `src/`, etc.) are legitimate and must NOT be deleted.
   - `.playwright-mcp/` (directory)
   - `*.tmp` (anywhere)
   - `tmp/` (directory, when at repo root or inside a subproject root)

2. If the list is empty, skip the rest of this phase.

3. **Confirmation gate** — deletion is destructive and irreversible. Show the
   user the exact list of paths about to be deleted (one line each) and ask
   explicit permission before deleting. If the user declines, **skip deletion**
   and proceed to Phase 3.

   Any path outside the worktree root (absolute paths, `..` traversal) must be
   rejected — fail this gate rather than delete.

4. Delete the confirmed paths. For files: `rm -f <path>`. For directories:
   `rm -rf <path>`. Run from the repo root. Never pass `/`, `~`, `.`, `..`,
   or a glob like `*` to `rm` — only the explicit confirmed paths from step 3.

5. Verify with `git status --porcelain` — none of the deletions should appear,
   because every removed path was untracked. If any tracked file shows as
   deleted, **stop** and surface it to the user (something matched a tracked
   path; the patterns above are wrong for this repo).

**Gate:** Either the user confirmed and matching untracked artifacts were
removed, or the user declined and the phase was skipped. `git status` shows no
unexpected tracked-file deletions.

---

### Phase 3: Conditional cleanup pass on the branch diff

Do not launch the cleanup pass by default. First classify the diff from
Phase 1:

- **Skip cleanup:** docs/config-only changes, ≤3 simple files, or a diff that
  has no debug output, unused imports, local duplication, or mechanical churn.
  Do one inline scan of the changed-file list and continue.
- **Run cleanup:** broad diffs, mixed-language changes, generated/manual churn,
  obvious debug leftovers, or user-requested cleanup.

1. If cleanup is warranted, spawn the `refactor-cleaner` agent (`run_in_background: false`) with the changed-file list from Phase 1 as scope. Pass file paths explicitly — don't let it roam the whole repo. If cleanup is not warranted, skip to step 4.
2. If the cleaner edited files:
   - Run the smallest relevant proof command for the edited files. Use full
     `make test`/`make test-fast` only when the cleanup crossed package
     boundaries or touched shared test/build infrastructure.
   - Commit: `git add -u && git commit -m "chore: ai-fmt"`.
3. If the cleanup pass made no changes, skip the commit.

**Then prune implementation-only tests.** The cleanup pass above never touches tests — this step does.

4. From the Phase 1 changed-file list, take only the test files this branch ADDED or MODIFIED — identify tests by their role, not a fixed extension list. Never consider pre-existing tests.
5. Remove cases that exist only to scaffold the implementation and add no regression value: trivial assertions (constructor returns non-nil, plain getters/setters, framework behavior), placeholder / `assert true` stubs, and cases fully subsumed or duplicated by another retained test. **NEVER** remove a test that is the sole coverage of a behavior, branch, edge case, error path, or regression — if unsure the coverage is unique, keep it.
6. Report each removed test, one line with its rationale (trivial / subsumed-by-X / duplicate).
7. If tests were removed, run the smallest relevant proof command for those test files; use full `make test` only when coverage spans multiple packages or the proof cannot be bounded. Commit the removals on their own: `git add -A && git commit -m "test: remove implementation-only tests"`.
8. If nothing qualifies, skip silently.

**Gate:** Cleanup ran only when warranted; cleanup changes (if any) are committed and green; implementation-only tests are pruned in their own commit (or none qualified); no sole-coverage test was removed.

---

### Phase 4: Format only when relevant

Check if the target exists first: `make -n fmt >/dev/null 2>&1`.

- **If the target exists and the changed-file list includes formatter-owned source files:** run `make fmt`. Then check `git status --porcelain`. If anything changed, run the smallest relevant proof command, then commit with `git add -u && git commit -m "chore: fmt"`. If nothing changed, skip the commit.
- **If `fmt` exists but the branch is docs/config-only or otherwise outside
  formatter scope:** skip and note why.
- **If no Makefile exists, or no `fmt` target:** do not add one during PR
  cleanup unless the user explicitly asked. Use a language-native formatter
  only when it is obvious and scoped to changed files; otherwise proceed and
  note that no formatter gate exists.

**Gate:** Formatting ran when relevant, or was explicitly skipped as out of scope/missing.

---

### Phase 5: `make test`

Check if the target exists first: `make -n test >/dev/null 2>&1` (also accept `test-fast` per the `test-gate` hook's preference order).

- **If the target exists:** run it — must pass. If it fails, **stop**. Do not push a broken branch. Fix the failure (likely a separate `fix:` commit) and re-run.
- **If no Makefile exists, or no `test`/`test-fast` target:** do not add one
  during PR cleanup unless the user explicitly asked. Run an obvious native
  project test command only if it is already documented/configured; otherwise
  proceed and note in the PR body that tests were not gated.

**Gate:** Either tests are green, or the user has been notified the target is missing and accepted that trade-off.

---

### Phase 6: Draft, push, and create

Compose the PR using **all** commits on the branch (not just the latest):

- **Title:** ≤70 chars, conventional format `<type>: <description>`. Pick the type that matches the *primary* change.
- **Summary:** 1–3 bullets — what changed and why. The "why" matters more than the "what".
- **Test plan:** bulleted checklist of how to verify.

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

- "I'll run the cleanup pass for a tiny docs/config diff" → don't. It is for broad or messy diffs.
- "Tests fail but my changes are unrelated" → fix or revert. Never push red.
