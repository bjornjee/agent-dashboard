---
name: fix
description: Diagnose and fix a bug in an isolated git worktree with reproduce-first, test-first methodology
disable-model-invocation: true
effort: max
---

<codex_skill_must>
1. Run `mkdir -p` and `git worktree add -b fix/<name> ... main` as separate `exec_command` calls.
2. Write and run the reproducing failing test before changing implementation.
3. Change only what makes the reproducing test pass.
4. Allowed tools: `exec_command`, `request_user_input`, `spawn_agent`, `wait_agent`, `apply_patch`.
5. Every `spawn_agent` call must be followed by `wait_agent`.
</codex_skill_must>

Diagnose and fix a bug.

Bug description: $ARGUMENTS

## Instructions

Follow these phases in order. Each phase has a gate — do not proceed until the gate is satisfied. Apply all project rules and conventions that are in your context.

If the bug involves browser UI, Playwright, dev-server ports, screenshots, or interactive Browser/Chrome inspection, apply `../_shared/ui-automation.md` at evidence gathering, environment setup, reproduction, verification, delegation, and cleanup points.

---

### Phase 1: Setup

1. Derive a short kebab-case name from the bug description.
2. Derive the app name from the git repo: `basename $(git rev-parse --show-toplevel)`
3. Switch to main: `git checkout main`
4. Pull latest: `git pull origin main`
5. Create branch `fix/<name>` and worktree `../worktrees/<app>/<name>` from main. Run **two separate `exec_command` tool calls** — do not chain them with `&&`. The dashboard's PostToolUse hook only stamps `worktree_cwd` + `branch` when the command starts with `git worktree add`; a compound `mkdir … && git worktree add …` slips past the regex and leaves the dashboard unable to detect dir or branch.

   First, ensure the parent directory exists:
   ```
   mkdir -p ../worktrees/<app>
   ```
   Then run `git worktree add -b fix/<name> ../worktrees/<app>/<name> main` as its own `exec_command` tool call:
   ```
   git worktree add -b fix/<name> ../worktrees/<app>/<name> main
   ```
   - If the branch already exists, ask the user whether to resume it or choose a new name.
6. **From the source repo root** (before cd'ing), copy environment files into the worktree **preserving their exact relative path from the project root**:
   - Find all env files recursively: `find . -name '.env*' -not -name '.env-setup-*' -not -path './.git/*' -not -path './node_modules/*'`
   - For each file found, recreate its directory structure in the worktree and copy it. For example:
     - `./.env` → `../worktrees/<app>/<name>/.env`
     - `./services/api/.env.local` → `../worktrees/<app>/<name>/services/api/.env.local`
   - Use: `for f in $(find . -name '.env*' -not -name '.env-setup-*' -not -path './.git/*' -not -path './node_modules/*'); do mkdir -p "../worktrees/<app>/<name>/$(dirname "$f")" && cp "$f" "../worktrees/<app>/<name>/$f"; done`
   - If `.claude/settings.local.json` exists: `mkdir -p ../worktrees/<app>/<name>/.claude && cp .claude/settings.local.json ../worktrees/<app>/<name>/.claude/`
   - **Important:** Commands in this step write outside the project root. Use Codex escalation (`sandbox_permissions: "require_escalated"`) with a concise justification; do not try to route around approvals.
7. cd into the worktree, run `node "${PLUGIN_ROOT:-${CLAUDE_PLUGIN_ROOT:-$(ls -dt "${CODEX_HOME:-$HOME/.codex}/plugins/cache/agent-dashboard/agent-dashboard"/* 2>/dev/null | head -1)}}/hooks/claim-worktree.js"`, and confirm with `pwd` and `git branch --show-current`
8. Verify: compare env files between source and worktree. Run the same `find` command in both directories and diff the file lists. If any files are missing in the worktree, **halt and report failure**. If the source repo had no `.env*` files, note that explicitly.

**Gate:** Working directory is the new worktree on the correct branch, based on latest main. If `.env*` files existed in the source repo, they are all present in the worktree.

---

### Phase 2: Gather Evidence

Start two tracks in parallel:

**Background — Environment setup:** First check for a reusable environment: if `.env-setup-done` exists in the worktree root AND every dependency manifest/lockfile present (`package-lock.json`, `pnpm-lock.yaml`, `yarn.lock`, `requirements.txt`, `pyproject.toml`, `uv.lock`, `go.mod`, `go.sum`) is older than the sentinel (`[ "$f" -ot .env-setup-done ]`), skip the launch and note the reuse — the setup from a prior run in this worktree is current. Otherwise, launch a background `exec_command` to set up the dev environment. It must:

1. Auto-detect project type from project files (highest match wins):

   | Priority | Signal | Type |
   |----------|--------|------|
   | 1 | `react-native` in package.json dependencies | Mobile |
   | 2 | `next`, `vite`, or `webpack` in package.json | Web |
   | 3 | `requirements.txt`, `pyproject.toml`, or `setup.py` | Python |
   | 4 | `go.mod` | Go |
   | 5 | `Dockerfile` or `docker-compose.yml` | Containerized |

   Ask the user only if no signal matches.

2. Install dependencies appropriate for the project type (e.g. `pip install`, `npm install`, `go mod download`). Configure ports, create emulators/simulators as needed. For browser UI bugs, allocate worktree-local Playwright/server/profile/output resources per `../_shared/ui-automation.md`.
3. Symlink large source-content directories (`data/`, `datasets/`, `evals/`, `models/`, `artifacts/`) from the source repo rather than copying. NEVER symlink build outputs or per-project caches (`.next/`, `dist/`, `build/`, `out/`, `target/`, `.turbo/`, `.cache/`, `.parcel-cache/`, `.vite/`, `__pycache__/`, `.pytest_cache/`, `.gradle/`, `.venv/`, `node_modules/`) — they bake absolute paths and corrupt across worktrees, and must be regenerated per-worktree.
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
2. Run the smallest command that executes the reproducing test and confirm it **fails**.
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

**Effort note:** When launched via the agent-dashboard's New Agent flow, this skill starts at implementation effort. Use Codex Plan Mode for high-reasoning diagnosis or planning when the fix needs it, then return to proportional implementation effort after approval.

**Delegation gate:** Use Codex `spawn_agent` **only if** the user explicitly requested subagents OR the fix touches 10+ files / ~3,000+ lines of implementation. Below that threshold, the orchestration overhead costs more tokens than implementing directly. If delegating, pass the diagnosis (Phase 4), failing test output (Phase 3), exact file paths, and a bounded write scope to a `worker`; then call `wait_agent`, review the result, and verify locally. Otherwise, proceed below.

1. Implement the **minimal fix** — change only what is necessary to fix the bug.
2. Run the reproducing test command — the previously failing test must now **pass**.
3. Run full `make test`/`make test-fast` only when the fix crosses packages, touches shared state/test/build infrastructure, or the risk cannot be bounded. Otherwise rely on the targeted reproducing command here; `$agent-dashboard:pr` owns the final branch-wide gate.
4. Show the passing test output and name whether full-suite verification was required.

For UI fixes, prefer headless Playwright with worktree-local resources. Use interactive Browser/Chrome inspection only when the shared policy says it is warranted.

**Gate:** The reproducing test passes. Full-suite verification ran only when required by risk. No unrelated changes.

---

### Phase 6: Refactor

1. Review the fix — is there a cleaner way to express it? Unnecessary duplication?
2. If changes are needed, make them and rerun the reproducing proof command; escalate to full `make test`/`make test-fast` if the refactor widens scope.
3. If no refactoring is needed, skip this phase.

**Gate:** The relevant proof passes. Code is clean.

---

### Phase 7: Review, Commit, and Open PR

1. Review all changes for correctness, security, and convention adherence.
2. Commit with a `fix:` conventional commit message that describes what was fixed and why.
3. Open the PR by invoking **`$agent-dashboard:pr`**. That skill owns conditional cleanup/formatting, final test gating when available, push, and `gh pr create`. Do not call `gh pr create` directly — a `pr-skill-gate` hook will block it.

**Gate:** Clean commit with conventional message. No critical or high-severity review issues. PR opened via `$agent-dashboard:pr`.

---

### Phase 8: Cleanup (on merge)

Triggered when the user indicates the fix has been merged upstream.

1. Verify the branch is merged (warn if unmerged commits remain)
2. Tear down environment resources: remove symlinks, stop dev servers or emulators, release any browser lease, remove worktree-local UI scratch state, delete `.env-setup-done`/`.env-setup-failed` sentinel files
3. Remove worktree and delete branch
4. Confirm cleanup is complete
