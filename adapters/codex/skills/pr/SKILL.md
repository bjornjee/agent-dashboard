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
screenshots, Playwright MCP output, and tmp scratch — before the cleaner pass
inspects the diff. **Untracked only.** Never touch tracked or staged files.

Deletion is destructive and irreversible. This phase **requires explicit user
confirmation** before any `rm` runs.

1. Identify untracked artifacts. Use `git ls-files --others --exclude-standard`
   for unignored untracked files, and `git ls-files --others --ignored
   --exclude-standard` for ignored untracked files. Filter for these patterns:
   - `*.png` at the repo root only — these are typically Playwright/visual-audit screenshots. PNGs inside subdirectories (build outputs `out/`, `dist/`, `build/`, asset dirs `public/`, `static/`, source dirs `src/`, etc.) are legitimate and must NOT be deleted.
   - `.playwright-mcp/` (directory)
   - `*.tmp` (anywhere)
   - `tmp/` (directory, when at repo root or inside a subproject root)

2. If the list is empty, skip the rest of this phase.

3. **Confirmation gate** — show the user the exact list of paths about to be
   deleted (one line each) and ask explicit permission before deleting. Use
   `request_user_input` when available, otherwise ask one concise direct
   question:

   > About to permanently delete N untracked scratch files: <list>. Proceed?

   Options: `"Delete all (Recommended)"`, `"Skip cleanup — keep these files"`,
   `"Let me edit the list"` (if the user picks edit, accept a revised path
   list and re-confirm before any deletion). If `request_user_input` is
   unavailable and the user does not affirm, **skip deletion** and proceed to
   Phase 3.

   Any path outside the worktree root (absolute paths, `..` traversal) must be
   rejected — fail this gate rather than delete.

4. Delete the confirmed paths. For files: `rm -f <path>`. For directories:
   `rm -rf <path>`. Run from the repo root. **Never** pass `/`, `~`, `.`,
   `..`, or a glob like `*` to `rm` — only the explicit confirmed paths from
   step 3.

5. Verify with `git status --porcelain` — none of the deletions should appear,
   because every removed path was untracked. If any tracked file shows as
   deleted, **stop** and surface it to the user (something matched a tracked
   path; the patterns above are wrong for this repo).

**Gate:** Either the user confirmed and matching untracked artifacts were
removed, or the user declined and the phase was skipped. `git status` shows no
unexpected tracked-file deletions.

---

### Phase 3: Cleaner pass on the branch diff

Codex cleanup uses a `worker` subagent with the inline cleanup brief below.
Keep the worker scoped to the changed-file list and wait for its result before
continuing.

1. Spawn a Codex `worker` subagent scoped to the changed-file list from
   Phase 1. Pass file paths explicitly — do not let it roam the whole repo.
   Then call `wait_agent` to block on its completion before continuing.

   Inline brief for the worker (pass as the worker's prompt):

   > You are doing a narrow cleanup pass on a branch diff. Scope: the files
   > listed below — do not read or edit anything outside this set.
   >
   > Files: `<paste changed-file list from Phase 1>`
   >
   > Allowed edits:
   > - Remove unused imports, dead variables, and unreachable code introduced
   >   by this diff.
   > - Remove debug/log statements added during implementation
   >   (`console.log`, `fmt.Println`, `print(...)` for debugging, etc.).
   > - Collapse trivially duplicated code added in this diff into a single
   >   call site when the duplication is obvious and local.
   > - Apply project formatting if a formatter config exists.
   >
   > Forbidden:
   > - Behavioral changes, refactors that move code between files, API
   >   renames, dependency additions/removals, or any change to tests.
   > - Editing files outside the listed scope.
   >
   > After your changes, run `make test` (if a `test` target exists) and
   > report whether it passes. Do not commit — the parent skill commits.

2. If the worker edited files:
   - Run `make test`. If it fails, fix the regression before continuing.
   - Commit: `git add -u && git commit -m "chore: ai-fmt"`.
3. If the worker made no changes, skip the commit.

**Gate:** Either the worker made no changes, or its changes are committed and tests are green.

---

### Phase 4: `make fmt`

Check if the target exists first: `make -n fmt >/dev/null 2>&1`.

- **If the target exists:** run `make fmt`. Then check `git status --porcelain`. If anything changed, run `make test`, then commit with `git add -u && git commit -m "chore: fmt"`. If nothing changed, skip the commit.
- **If no Makefile exists, or no `fmt` target:** surface this hint to the user, then ask with `request_user_input` when available. If unavailable, ask one concise direct question with the same two choices:

  > No `make fmt` target found in this repo. Recommend adding one — even a thin wrapper around the language-native formatter (`gofmt -w .`, `ruff format`, `prettier --write`, `cargo fmt`, etc.) is enough to make this gate work and keeps formatting deterministic across contributors.

  If the user says yes, add the target *before* opening the PR (a separate `chore: add make fmt target` commit). If no, proceed without formatting.

**Gate:** Either `make fmt` ran clean, or the user has been notified the target is missing.

---

### Phase 5: `make test`

Check if the target exists first: `make -n test >/dev/null 2>&1` (also accept `test-fast` per the `test-gate` hook's preference order).

- **If the target exists:** run it — must pass. If it fails, **stop**. Do not push a broken branch. Fix the failure (likely a separate `fix:` commit) and re-run.
- **If no Makefile exists, or no `test`/`test-fast` target:** surface this hint to the user, then ask with `request_user_input` when available. If unavailable, ask one concise direct question with the same two choices:

  > No `make test` target found in this repo. Recommend adding one (e.g. `test: ; <runner>`) so the PR gate can verify changes pass before opening the PR.

  If yes, add it (separate `chore: add make test target` commit) and run it. If no, proceed but note in the PR body that tests were not gated.

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

- "I'll just call `gh pr create` directly" → the hook will block you. Use the prefix.
- "Skip the cleaner, the diff looks clean" → don't. The cleaner is the whole point of this skill.
- "Tests fail but my changes are unrelated" → fix or revert. Never push red.
- "I'll bundle the fmt diff into the feature commit" → keep `chore: fmt` and `chore: ai-fmt` as their own commits; squash happens at merge.
