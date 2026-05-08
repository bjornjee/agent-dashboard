---
name: fix
description: Diagnose and fix a bug in an isolated git worktree with reproduce-first, test-first methodology
disable-model-invocation: true
effort: max
---

Diagnose and fix a bug.

Bug description: $ARGUMENTS

## Instructions

Follow these phases in order. Each phase has a gate — do not proceed until the gate is satisfied. Apply all project rules and conventions that are in your context.

---

### Phase 1: Setup

1. Derive a short kebab-case name from the bug description.
2. Derive the app name from the git repo: `basename $(git rev-parse --show-toplevel)`
3. Switch to main: `git checkout main`
4. Pull latest: `git pull origin main`
5. Create branch `fix/<name>` and worktree `../worktrees/<app>/<name>` from main:
   `mkdir -p ../worktrees/<app> && git worktree add ../worktrees/<app>/<name> -b fix/<name> main`
   - If the branch already exists, ask the user whether to resume it or choose a new name.
6. **From the source repo root** (before cd'ing), copy environment files into the worktree **preserving their exact relative path from the project root**:
   - Find all env files recursively: `find . -name '.env*' -not -path './.git/*' -not -path './node_modules/*'`
   - For each file found, recreate its directory structure in the worktree and copy it. For example:
     - `./.env` → `../worktrees/<app>/<name>/.env`
     - `./services/api/.env.local` → `../worktrees/<app>/<name>/services/api/.env.local`
   - Use: `for f in $(find . -name '.env*' -not -path './.git/*' -not -path './node_modules/*'); do mkdir -p "../worktrees/<app>/<name>/$(dirname "$f")" && cp "$f" "../worktrees/<app>/<name>/$f"; done`
   - If `.claude/settings.local.json` exists: `mkdir -p ../worktrees/<app>/<name>/.claude && cp .claude/settings.local.json ../worktrees/<app>/<name>/.claude/`
   - **Important:** All Bash tool calls in this step must set `dangerouslyDisableSandbox: true` because they write outside the project root.
6. cd into the worktree and confirm with `pwd` and `git branch --show-current`
7. Verify: compare env files between source and worktree. Run the same `find` command in both directories and diff the file lists. If any files are missing in the worktree, **halt and report failure**. If the source repo had no `.env*` files, note that explicitly.

**Gate:** Working directory is the new worktree on the correct branch, based on latest main. If `.env*` files existed in the source repo, they are all present in the worktree.

---

### Phase 2: Gather Evidence

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

**Foreground — Evidence gathering:**

Before touching code, collect **grounded evidence** from observable sources. Do not guess from reading code alone.

1. Take the bug description — this may be an error message, stack trace, issue URL, or user description.
2. Collect evidence from these sources (check all that are available):
   - **Logs:** application logs, server logs, error tracking (Sentry, Datadog, etc.). Ask the user where logs live if not obvious.
   - **Metrics:** dashboards, monitoring, performance counters. Ask for links or screenshots.
   - **Stack traces:** the full trace, not just the top frame. Include line numbers and timestamps.
   - **Steps to reproduce:** exact inputs, environment, and sequence that triggers the bug.
   - **Git history:** `git log --oneline --since="2 weeks ago" -- <affected files>` — what changed recently in the area?
   - **Issue tracker:** if an issue URL was provided, read it fully including comments for additional context.
3. Summarize the evidence. State what is **known** (from logs/metrics/traces) vs what is **hypothesized**.

**Gate:** At least one source of grounded evidence (log, trace, metric, or reproducible steps) is collected. Do not proceed on hypothesis alone.

---

### Phase 3: Reproduce (RED)

**Pre-gate:** Check for `.env-setup-done` in the worktree root.
- If present: verify dependencies are installed (e.g. `node_modules/` exists, `pip list` succeeds, `go env GOPATH` works) and data symlinks resolve correctly.
- If `.env-setup-failed` exists: surface the error and halt.
- If neither file exists: the background agent is still running — wait for it to finish before proceeding.

1. Using the evidence from Phase 2, write a **failing test** that reproduces the bug. The test should:
   - Replicate the exact conditions from the evidence (inputs, state, sequence)
   - Target the specific behavior that is broken
   - Fail for the right reason (matching the observed error, not a typo or import error)
   - Be minimal — test only the broken behavior
2. Run `make test` and confirm the new test **fails**.
3. Compare the test failure output against the evidence from Phase 2 — the failure should match the observed bug (same error type, same behavior). If it doesn't, the test is wrong, not the code.
4. Show the failing test output to the user.

**Gate:** A test exists that fails, reproducing the bug. The failure matches the observed evidence.

---

### Phase 4: Diagnose

Root cause analysis must be grounded in the evidence and the failing test, not speculation from reading code.

1. Trace the code path **from the failing test** to identify where the behavior diverges from what is expected.
2. Cross-reference with git history: `git log -S "<relevant term>"` and `git bisect` if the bug is a regression.
3. Identify the root cause — explain:
   - What the code **does** (observed via test failure and logs)
   - What it **should do** (expected behavior from evidence)
   - **Why** it diverges (the specific line or logic that causes the mismatch)
4. Present the diagnosis to the user for confirmation. Include evidence citations (log lines, metric values, test output) — not just "I read the code and think X."

**Gate:** User agrees with the root cause analysis. Diagnosis cites observable evidence.

---

### Phase 5: Fix (GREEN)

**Effort note:** This skill runs at `max` reasoning effort throughout. To save tokens during implementation, run `/effort high` before continuing — `effort: max` resumes on the next invocation.

**Delegation gate:** Invoke `/codex:setup` to check Codex CLI availability. If the output contains `"ready": true`, delegate **only if** the user explicitly requested Codex delegation OR the fix touches 10+ files / ~3,000+ lines of implementation. Below that threshold, the orchestration overhead costs more tokens than Claude implementing directly. If delegating, invoke `/codex-delegate` with the diagnosis (Phase 4) and failing test (Phase 3) as implementation context, then skip to the phase gate. Otherwise, proceed below.

1. Implement the **minimal fix** — change only what is necessary to fix the bug.
2. Run `make test` — the previously failing test must now **pass**.
3. Run the full test suite via `make test` — no regressions.
4. Show the passing test output.

**Gate:** The reproducing test passes. The full test suite passes. No unrelated changes.

---

### Phase 6: Refactor

1. Review the fix — is there a cleaner way to express it? Unnecessary duplication?
2. If changes are needed, make them and run `make test` to confirm tests still pass.
3. If no refactoring is needed, skip this phase.

**Gate:** Tests pass via `make test`. Code is clean.

---

### Phase 7: Review, Commit, and Open PR

1. Review all changes for correctness, security, and convention adherence.
2. Commit with a `fix:` conventional commit message that describes what was fixed and why.
3. Open the PR by invoking **`/agent-dashboard:pr`**. That skill owns the cleanup pass (`refactor-cleaner`), `make fmt`, `make test`, push, and `gh pr create`. Do not call `gh pr create` directly — a `pr-skill-gate` hook will block it.

**Gate:** Clean commit with conventional message. No critical or high-severity review issues. PR opened via `/agent-dashboard:pr`.

---

### Phase 8: Cleanup (on merge)

Triggered when the user indicates the fix has been merged upstream.

1. Verify the branch is merged (warn if unmerged commits remain)
2. Tear down environment resources: remove symlinks, stop dev servers or emulators, delete `.env-setup-done`/`.env-setup-failed` sentinel files
3. Remove worktree and delete branch
4. Confirm cleanup is complete
