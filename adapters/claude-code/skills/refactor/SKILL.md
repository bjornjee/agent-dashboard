---
name: refactor
description: Safely restructure code in an isolated git worktree with test-preserved, incremental transformations
disable-model-invocation: true
---

Safely refactor code while preserving all existing behavior.

Refactoring goal: $ARGUMENTS

## Instructions

Follow these phases in order. Each phase has a gate — do not proceed until the gate is satisfied. Apply all project rules and conventions that are in your context.

---

### Phase 1: Setup

1. Derive a short kebab-case name from the refactoring goal.
2. Derive the app name from the git repo: `basename $(git rev-parse --show-toplevel)`
3. Switch to main: `git checkout main`
4. Pull latest: `git pull origin main`
5. Create branch `refactor/<name>` and worktree `../worktrees/<app>/<name>` from main:
   `mkdir -p ../worktrees/<app> && git worktree add ../worktrees/<app>/<name> -b refactor/<name> main`
   - If the branch already exists, ask the user whether to resume it or choose a new name.
6. **From the source repo root** (before cd'ing), copy environment files into the worktree **preserving their exact relative path from the project root**:
   - Find all env files recursively: `find . -name '.env*' -not -path './.git/*' -not -path './node_modules/*'`
   - For each file found, recreate its directory structure in the worktree and copy it. For example:
     - `./.env` → `../worktrees/<app>/<name>/.env`
     - `./services/api/.env.local` → `../worktrees/<app>/<name>/services/api/.env.local`
   - Use: `for f in $(find . -name '.env*' -not -path './.git/*' -not -path './node_modules/*'); do mkdir -p "../worktrees/<app>/<name>/$(dirname "$f")" && cp "$f" "../worktrees/<app>/<name>/$f"; done`
   - If `.claude/settings.local.json` exists: `mkdir -p ../worktrees/<app>/<name>/.claude && cp .claude/settings.local.json ../worktrees/<app>/<name>/.claude/`
6. cd into the worktree and confirm with `pwd` and `git branch --show-current`
7. Verify: compare env files between source and worktree. Run the same `find` command in both directories and diff the file lists. If any files are missing in the worktree, **halt and report failure**. If the source repo had no `.env*` files, note that explicitly.

**Gate:** Working directory is the new worktree on the correct branch, based on latest main. If `.env*` files existed in the source repo, they are all present in the worktree.

---

### Phase 2: Scope

Start two tracks in parallel:

**Background — Environment setup:** Launch a background agent (`run_in_background: true`) to set up the dev environment. The agent must:

1. Auto-detect project type from project files (highest match wins):

   | Priority | Signal | Type |
   |----------|--------|------|
   | 1 | `react-native` in package.json dependencies | Mobile |
   | 2 | `next`, `vite`, or `webpack` in package.json | Web |
   | 3 | `requirements.txt`, `pyproject.toml`, or `setup.py` | Python |
   | 4 | `go.mod` | Go |
   | 5 | `Dockerfile` or `docker-compose.yml` | Containerized |

   Ask the user only if no signal matches.

2. Install dependencies appropriate for the project type (e.g. `pip install`, `npm install`, `go mod download`). Configure ports, create emulators/simulators as needed.
3. Symlink large data directories (`data/`, `datasets/`, `evals/`, `models/`, `artifacts/`) from the source repo rather than copying.
4. On success, write a sentinel file: `touch .env-setup-done`
   On failure, write the error: `echo "<error message>" > .env-setup-failed`

**Foreground — Scoping:**

1. Parse the refactoring goal — what is being restructured and why?
2. Identify all affected files by searching the codebase for the code to be changed and its dependents.
3. Check test coverage for the affected code — what tests exist? What is untested?
4. If test coverage is insufficient for safe refactoring, **tell the user** and suggest writing tests first before refactoring.

**Gate:** The scope is clear. Affected files and their test coverage are identified.

---

### Phase 3: Baseline

**Pre-gate:** Check for `.env-setup-done` in the worktree root.
- If present: verify dependencies are installed (e.g. `node_modules/` exists, `pip list` succeeds, `go env GOPATH` works) and data symlinks resolve correctly.
- If `.env-setup-failed` exists: surface the error and halt.
- If neither file exists: the background agent is still running — wait for it to finish before proceeding.

1. Run `make test` to establish a passing baseline.
2. If tests fail, **stop and report**. Do not refactor on a broken codebase. Suggest using `/fix` first.
3. Record the test output as the regression baseline.

**Gate:** All tests pass. The baseline is established.

---

### Phase 4: Transform

**Delegation gate:** Invoke `/codex:setup` to check Codex CLI availability. If the output contains `"ready": true`, invoke `/codex-delegate` with the scope (Phase 2) and baseline (Phase 3) as implementation context, then skip to the phase gate. Otherwise, proceed below.

Apply the refactoring in small, atomic steps. For each step:

1. Make a single, focused change (e.g., extract a function, rename a variable, move a file).
2. Run `make test` immediately after the change.
3. If tests fail:
   - Revert only the changed files (`git checkout -- <file1> <file2> ...`)
   - Analyze why it failed
   - Try a different approach
4. If tests pass, proceed to the next step.

Do not batch multiple changes between test runs. One change, one test run.

**Gate:** All transformations applied. All tests pass after each step.

---

### Phase 5: Cleanup

1. Remove dead code — unused imports, functions, variables, files.
2. Update any affected documentation or comments.
3. Run `make test` one final time.

**Gate:** No dead code remains. All tests pass.

---

### Phase 6: Review and Commit

Before committing, run the `refactor-cleaner` agent as an automated cleanup pass — but only if needed:

1. Check git log for a recent cleaner run: `git log --oneline -20 --grep="chore: ai-fmt"`.
2. If no recent run is found, spawn the `refactor-cleaner` agent (`run_in_background: false`) on all changed files.
3. Run `make test` to confirm the cleaner's changes don't break anything.
4. If the cleaner made changes, commit them separately with `chore: ai-fmt` as the commit message.

1. Review all changes for correctness, security, and convention adherence.
2. Verify that behavior is preserved — no new features, no bug fixes, only structural changes.
3. Commit with a `refactor:` conventional commit message that describes what was restructured and why.

**Gate:** Clean commit with conventional message. Behavior is unchanged. No critical or high-severity review issues.

---

### Phase 7: Cleanup (on merge)

Triggered when the user indicates the refactor has been merged upstream.

1. Verify the branch is merged (warn if unmerged commits remain)
2. Tear down environment resources: remove symlinks, stop dev servers or emulators, delete `.env-setup-done`/`.env-setup-failed` sentinel files
3. Remove worktree and delete branch
4. Confirm cleanup is complete
