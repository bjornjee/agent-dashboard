---
name: feature
description: Start a new feature in an isolated git worktree with TDD workflow
disable-model-invocation: true
---

Start a new feature in an isolated git worktree.

Feature description: $ARGUMENTS

## Instructions

Follow these phases in order. Each phase has a gate — do not proceed until the gate is satisfied.

---

### Phase 1: Setup

1. Derive a short kebab-case name from the description
2. Derive the app name from the git repo: `basename $(git rev-parse --show-toplevel)`
3. Switch to main: `git checkout main`
4. Pull latest: `git pull origin main`
5. Create branch `feat/<name>` and worktree `../worktrees/<app>/<name>` from main:
   `mkdir -p ../worktrees/<app> && git worktree add ../worktrees/<app>/<name> -b feat/<name> main`
   - If the branch already exists, ask the user whether to resume it or choose a new name.
6. **From the source repo root** (before cd'ing), copy environment files into the worktree **preserving their exact relative path from the project root**:
   - Find all env files recursively: `find . -name '.env*' -not -path './.git/*' -not -path './node_modules/*'`
   - For each file found, recreate its directory structure in the worktree and copy it. For example:
     - `./. env` → `../worktrees/<app>/<name>/.env`
     - `./services/api/.env.local` → `../worktrees/<app>/<name>/services/api/.env.local`
     - `./infra/.env.production` → `../worktrees/<app>/<name>/infra/.env.production`
   - Use: `for f in $(find . -name '.env*' -not -path './.git/*' -not -path './node_modules/*'); do mkdir -p "../worktrees/<app>/<name>/$(dirname "$f")" && cp "$f" "../worktrees/<app>/<name>/$f"; done`
   - If `.claude/settings.local.json` exists: `mkdir -p ../worktrees/<app>/<name>/.claude && cp .claude/settings.local.json ../worktrees/<app>/<name>/.claude/`
   - **Important:** All Bash tool calls in this step must set `dangerouslyDisableSandbox: true` because they write outside the project root.
6. cd into the worktree and confirm with `pwd` and `git branch --show-current`
7. Verify: compare env files between source and worktree. Run the same `find` command in both directories and diff the file lists. If any files are missing in the worktree, **halt and report failure**. If the source repo had no `.env*` files, note that explicitly.

**Gate:** Working directory is the new worktree on the correct branch, based on latest main. If `.env*` files existed in the source repo, they are all present in the worktree.

---

### Phase 2: Plan

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

**Foreground — Planning:**

1. Research the codebase and design the implementation approach. Do not wait for environment setup to finish.
2. Present the plan to the user as text and wait for explicit approval before proceeding. Do not call `EnterPlanMode`/`ExitPlanMode` from inside this skill — they are deferred tools in CC 2.1.116+ that may not be loaded, and `ExitPlanMode` resets the session permission mode (losing `bypassPermissions`), causing every subsequent step to re-prompt for approval.

**Gate:** User has approved the approach. No code has been written yet.

---

### Phase 3: Implement

**Pre-gate:** Check for `.env-setup-done` in the worktree root.
- If present: verify dependencies are installed (e.g. `node_modules/` exists, `pip list` succeeds, `go env GOPATH` works) and data symlinks resolve correctly.
- If `.env-setup-failed` exists: surface the error and halt.
- If neither file exists: the background agent is still running — wait for it to finish before proceeding.

**Delegation gate:** Invoke `/codex:setup` to check Codex CLI availability. If the output contains `"ready": true`, delegate **only if** the user explicitly requested Codex delegation OR the plan touches 10+ files / ~3,000+ lines of implementation. Below that threshold, the orchestration overhead (skill loading, prompt construction, subagent context, result parsing, review) costs more tokens than Claude implementing directly. If delegating, invoke `/codex-delegate` with the approved plan (Phase 2) as implementation context, then skip to the phase gate. Otherwise, proceed below.

Build the feature following strict RED → GREEN → REFACTOR:

1. **RED:** Write failing tests first. Run `make test` to confirm they fail. Show the failing output. Do not proceed until tests fail for the right reason.
2. **GREEN:** Write the minimum implementation to make tests pass. Run `make test` to confirm all tests pass.
3. **REFACTOR:** Clean up the implementation. Run `make test` to confirm tests still pass.

**Gate:** Environment ready. All tests pass via `make test`. Implementation matches the approved plan.

---

### Phase 4: Review

Review all changes for correctness, security, and convention adherence. Apply all project rules and conventions that are in your context.

**Gate:** No critical or high-severity issues remain.

---

### Phase 5: Deliver

Before committing, run the `refactor-cleaner` agent as an automated cleanup pass — but only if needed:

1. Check git log for a recent cleaner run: `git log --oneline -20 --grep="chore: ai-fmt"`.
2. If no recent run is found, spawn the `refactor-cleaner` agent (`run_in_background: false`) on all changed files.
3. Run `make test` to confirm the cleaner's changes don't break anything.
4. If the cleaner made changes, commit them separately with `chore: ai-fmt` as the commit message.

Commit the feature changes and prepare for merge.

**Gate:** Clean commit history with conventional commit messages.

---

### Phase 6: Cleanup (on merge)

Triggered when the user indicates the feature has been merged upstream.

1. Verify the branch is merged (warn if unmerged commits remain)
2. Tear down environment resources: remove symlinks, stop dev servers or emulators, delete `.env-setup-done`/`.env-setup-failed` sentinel files
3. Remove worktree and delete branch
4. Confirm cleanup is complete
