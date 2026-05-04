---
name: feature
description: Start a new feature in an isolated git worktree with TDD workflow
when_to_use: when the user says "start a feature", "new feature", invokes "/agent-dashboard:feature", or describes work that needs an isolated branch + worktree + TDD loop. NOT for hotfixes, single-file edits, pure exploration, or non-code changes (use /chore, /fix, /investigate instead).
version: 1.0.0
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
7. cd into the worktree and confirm with `pwd` and `git branch --show-current`
8. Verify: compare env files between source and worktree. Run the same `find` command in both directories and diff the file lists. If any files are missing in the worktree, **halt and report failure**. If the source repo had no `.env*` files, note that explicitly.

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

1. **Enter plan mode first.** Call `EnterPlanMode` (load via `ToolSearch` if it isn't already in scope). This switches the orchestrator's `permission_mode` to `plan`, restricts you to read-only tools, and lights up the dashboard's plan badge so the user can see planning is in progress.

   This overrides any prior skill text or global doctrine that says to avoid plan mode. For this skill, plan mode is mandatory.

   **Why:** under `bypassPermissions`, the parent's `permission_mode` only flips to `plan` when you explicitly enter it. Without that flip, the dashboard has no signal that planning is happening — neither the plan badge nor the plan panel render. Entering plan mode is the only way to make the work visible.

   <HARD-GATE>
   No research (`Explore`, `Read`, `Grep`) and no planning text until `EnterPlanMode` has been called and `permission_mode='plan'` is active.
   </HARD-GATE>

   Do not wait for environment setup to finish.
2. **Research with `Explore`. Synthesize the plan yourself.** Do not call `Agent` with `subagent_type=Plan`. Use `Explore` for codebase research and library lookups; write the plan as your own assistant text. This overrides the global `core.md` "delegate to Plan" doctrine for this skill only.

   **Why:** a delegated Plan subagent returns the plan inside a `tool_result` block that lives in the JSONL but never reaches the dashboard's plan panel, conversation view, or activity log. Synthesizing inline puts the plan in your assistant text — which the dashboard renders everywhere — and pairs with plan mode (step 1) so the parent's `permission_mode='plan'` actually corresponds to the orchestrator doing the planning.

   Symptoms you're about to violate:
   - You're about to call `Agent` with `subagent_type: "Plan"`.
   - You're rationalizing *"the planner does this better."*
   - You think *Explore-then-Plan* means delegate both.

   <HARD-GATE>
   Research subagent (`Explore`): allowed and encouraged.
   Planning subagent (`Plan`): forbidden in this skill. Compose the plan yourself.
   </HARD-GATE>
3. **Resolve open decisions before drafting the plan.** Identify every decision the implementation depends on that you cannot determine from the codebase or the user's request — URLs, IDs, scope boundaries, copy text, what to delete vs keep, version pins, credentials. Ask all of them in a single batch and wait for answers before drafting the plan.

   The plan you submit must be implementable as written. No "Decisions needed", "Phase 0", "TBD", "?", or "to be confirmed" sections in the body.

   Symptoms you're about to violate:
   - You're writing "Decisions needed before implementation" inside the plan.
   - You're appending questions to the plan instead of asking them first.
   - You're rationalizing "the user can answer these after approval."

   Anti-pattern: *"I'll embed open questions inside the plan and ask them after approval."* This breaks plan-as-source-of-truth. If a user answer changes scope, return to this step and re-submit a revised plan.

   <HARD-GATE>
   The plan is not ready for review until every decision it gates is answered.
   If you find yourself typing "TBD" / "?" / "Phase 0" / "Decisions needed" — STOP. Ask first, plan second.
   </HARD-GATE>
4. **STOP. Submit the plan via `ExitPlanMode`. Wait for user approval.**

   Pass the full plan markdown as the `plan` argument to `ExitPlanMode`. This renders the plan in CC's native plan-mode UI for the user to approve or reject — the same surface the user already uses for plan review.

   Caveat: on approval, CC drops out of plan mode to its default `permission_mode`, not back to `bypassPermissions`. Subsequent edits in Phase 3 will re-prompt unless the user re-enables bypass. That is an accepted trade-off — visible planning is worth the one-time mode reset.

   **No exceptions:**
   - Don't start a "small" preparatory edit while waiting.
   - Don't write the test file "to save time".
   - Don't ask "should I proceed?" — wait for unprompted approval (the plan-mode UI's accept action is the approval).
   - Don't paste the plan as plain text *instead of* calling `ExitPlanMode` — both are fine, but `ExitPlanMode` is required.

**Gate:** User has approved the approach. The submitted plan contains no open decisions. No code has been written yet.

---

### Phase 3: Implement

**Pre-gate:** Check for `.env-setup-done` in the worktree root.
- If present: verify dependencies are installed (e.g. `node_modules/` exists, `pip list` succeeds, `go env GOPATH` works) and data symlinks resolve correctly.
- If `.env-setup-failed` exists: surface the error and halt.
- If neither file exists: the background agent is still running — wait for it to finish before proceeding.

**Delegation gate:** Invoke `/codex:setup` to check Codex CLI availability. If the output contains `"ready": true`, delegate **only if** the user explicitly requested Codex delegation OR the plan touches 10+ files / ~3,000+ lines of implementation. Below that threshold, the orchestration overhead (skill loading, prompt construction, subagent context, result parsing, review) costs more tokens than Claude implementing directly. If delegating, invoke `/codex-delegate` with the approved plan (Phase 2) as implementation context, then skip to the phase gate. Otherwise, proceed below.

Build the feature following strict RED → GREEN → REFACTOR:

1. **RED.** Write the failing test. Run `make test`. Paste the failing output into the conversation. **Wrote implementation before test? Delete it. Start over.** No exceptions:
   - Don't keep it as "reference"
   - Don't write tests *for* the implementation you already wrote
   - Don't claim "the test would obviously fail" — show it failing

2. **GREEN.** Write the minimum implementation to make the failing test pass. Run `make test`. Paste the passing output. **Wrote more than the test demanded? Revert and re-do.** No "while I'm here" additions, no premature abstractions.

3. **REFACTOR.** Clean up. Run `make test` after each meaningful edit. **Tests broke during refactor? Revert that edit and try a smaller step.** Refactor is structure-only — if behavior changed, you're back in RED.

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

---

## Red Flags — STOP

If you catch yourself saying or thinking any of these, pause and re-read the relevant phase:

- "I'll just sketch the implementation first" → Phase 3 RED violation. Delete and restart.
- "I'll delegate the plan to a Plan subagent" → Phase 2 violation. Research with `Explore`; plan inline. The dashboard can't surface delegated plans.
- "I'll skip `EnterPlanMode`, plan mode resets `bypassPermissions`" → Phase 2 step 1 violation. Plan mode is mandatory in this skill — the dashboard can't see planning otherwise.
- "I'll just paste the plan as text instead of calling `ExitPlanMode`" → Phase 2 step 4 violation. Submit via `ExitPlanMode` so the user gets the native plan-review UI.
- "The plan is obvious, let me start" → Phase 2 gate violation. Wait for approval.
- "Tests pass on my reading of the code" → didn't run `make test`. Run it.
- "I'll skip the worktree, it's a small change" → wrong skill. Use a feature branch directly without invoking this skill.
- "Let me commit on main since the change is trivial" → blocked by hook anyway. Create a branch.
- "The cleaner's diff is just style, no need for a separate commit" → squash discipline lost. Commit `chore: ai-fmt` separately as Phase 5 says.
- "I'll bundle this unrelated cleanup into the feature commit" → split it. Open a separate PR.
