---
name: feature
description: Start a new feature in an isolated git worktree with proportional verification
when_to_use: when the user says "start a feature", "new feature", invokes "$agent-dashboard:feature", or describes work that needs an isolated branch + worktree + planned implementation loop. NOT for hotfixes, single-file edits, pure exploration, or non-code changes (use $agent-dashboard:chore, $agent-dashboard:fix, $agent-dashboard:investigate instead).
version: 1.0.0
disable-model-invocation: true
effort: max
---

<codex_skill_must>
1. Phase 2 is gated on `permission_mode='plan'`; if not plan, stop and ask the user to run `/plan`.
2. Run `mkdir -p` and `git worktree add -b feat/<name> ... main` as separate `exec_command` calls.
3. Submit plans only inside `<proposed_plan>...</proposed_plan>`.
4. After `<proposed_plan>`, stop until plan-review approval; ordinary chat approval is not implementation approval.
5. After approval, write `.feature-plan-path` before implementation.
6. Allowed tools: `exec_command`, `request_user_input`, `spawn_agent`, `wait_agent`, `update_plan`, `apply_patch`; every `spawn_agent` needs `wait_agent`.
</codex_skill_must>

Start a new feature in an isolated git worktree.

Feature description: $ARGUMENTS

## Instructions

Follow these phases in order. Each phase has a gate — do not proceed until the gate is satisfied.

Before every action, identify the current phase and check its gate. If a gate is not satisfied, stop instead of falling back. If you violate phase order, halt and report the violated gate.

If the feature touches browser UI, Playwright, dev-server ports, screenshots, or interactive Browser/Chrome inspection, apply `../_shared/ui-automation.md` at planning, environment setup, verification, delegation, and cleanup points.

---

### Phase 1: Setup

1. Derive a short kebab-case name from the description
2. Derive the app name from the git repo: `basename $(git rev-parse --show-toplevel)`
3. Switch to main: `git checkout main`
4. Pull latest: `git pull origin main`
5. Create branch `feat/<name>` and worktree `../worktrees/<app>/<name>` from main. Run **two separate `exec_command` tool calls** — do not chain them with `&&`. The dashboard's PostToolUse hook only stamps `worktree_cwd` + `branch` when the command starts with `git worktree add`; a compound `mkdir … && git worktree add …` slips past the regex and leaves the dashboard unable to detect dir or branch.

   First, ensure the parent directory exists:
   ```
   mkdir -p ../worktrees/<app>
   ```
   Then run `git worktree add -b feat/<name> ../worktrees/<app>/<name> main` as its own `exec_command` tool call:
   ```
   git worktree add -b feat/<name> ../worktrees/<app>/<name> main
   ```
   - If the branch already exists, ask the user whether to resume it or choose a new name.
6. **From the source repo root** (before cd'ing), copy environment files into the worktree **preserving their exact relative path from the project root**:
   - Find all env files recursively: `find . -name '.env*' -not -path './.git/*' -not -path './node_modules/*'`
   - For each file found, recreate its directory structure in the worktree and copy it. For example:
     - `./. env` → `../worktrees/<app>/<name>/.env`
     - `./services/api/.env.local` → `../worktrees/<app>/<name>/services/api/.env.local`
     - `./infra/.env.production` → `../worktrees/<app>/<name>/infra/.env.production`
   - Use: `for f in $(find . -name '.env*' -not -path './.git/*' -not -path './node_modules/*'); do mkdir -p "../worktrees/<app>/<name>/$(dirname "$f")" && cp "$f" "../worktrees/<app>/<name>/$f"; done`
   - If `.claude/settings.local.json` exists: `mkdir -p ../worktrees/<app>/<name>/.claude && cp .claude/settings.local.json ../worktrees/<app>/<name>/.claude/`
   - **Important:** Commands in this step write outside the project root. Use Codex escalation (`sandbox_permissions: "require_escalated"`) with a concise justification; do not try to route around approvals.
7. cd into the worktree, run `node "$HOME/.codex/hooks/agent-dashboard/claim-worktree.js"`, and confirm with `pwd` and `git branch --show-current`
8. Verify: compare env files between source and worktree. Run the same `find` command in both directories and diff the file lists. If any files are missing in the worktree, **halt and report failure**. If the source repo had no `.env*` files, note that explicitly.

**Gate:** Working directory is the new worktree on the correct branch, based on latest main. If `.env*` files existed in the source repo, they are all present in the worktree.

---

### Phase 2: Plan

**Plan-first guarantee.** Phase 2 is planning only. No dependency installs, no
sentinel writes, no environment setup runs until the plan is approved. The
only writes allowed in this phase are read-only research (Codex `explorer`)
and the `<proposed_plan>` submission itself. Environment setup kicks off in
the post-approval actions below.

Phase order: Plan Mode first, then research, then interview, then submit. Plan Mode is the *entry* gate to Phase 2 — nothing else in this phase happens outside it. Each step has a HARD-GATE you cannot rationalize past.

1. **Enter Codex Plan Mode via `/plan` before continuing.** Dashboard-created Codex feature sessions should auto-enter Plan Mode before the skill prompt is submitted. If `permission_mode` is not `plan`, stop immediately and ask the user to run `/plan`; do not research, interview, draft, call `spawn_agent`, call `update_plan`, edit files, write tests, run setup, or create sentinels.

   <HARD-GATE>No `spawn_agent`, `request_user_input`, or plan drafting until `permission_mode='plan'`.</HARD-GATE>

2. **Research with `spawn_agent` explorer.** Use a Codex `explorer` subagent for non-trivial codebase or library lookups. Always pair `spawn_agent` with a `wait_agent` to retrieve the explorer's findings — never spawn-and-forget. Do not delegate planning — composing the plan is your job. Synthesize what you found inline as your own assistant text so it lands in the visible plan artifact (not in a tool result the user can't approve).

   <HARD-GATE>`explorer` for research is fine; never dispatch another role to compose the plan. Every `spawn_agent` must be followed by `wait_agent`.</HARD-GATE>

3. **Interview the user via `request_user_input`.** Identify every gating decision the implementation depends on — URLs, IDs, scope boundaries, copy text, what to delete vs keep, version pins, credentials. Ask them as a single `request_user_input` call with multi-choice `options`, **not** as freeform numbered text. If `request_user_input` is unavailable during Phase 2, stop and ask the user to run `/plan`.

   Schema: 1–3 questions per call, each with a `header` (≤12 chars), 2–3 mutually exclusive `options`. Recommended option goes first with `(Recommended)` suffix. The client adds an "Other" escape hatch automatically.

   Worked example:
   ```
   request_user_input({
     questions: [{
       id: "focus_path",
       question: "Where should focus.json live?",
       header: "Focus path",
       options: [
         {label: "~/.agent-dashboard/focus.json (Recommended)", description: "Co-located with agents/ state dir already watched."},
         {label: "$XDG_RUNTIME_DIR/agent-dashboard/focus.json", description: "Tmpfs-backed, cleared on reboot."},
       ],
     }]
   })
   ```

   The plan you submit in step 4 must be implementable as written. No "Decisions needed", "Phase 0", "TBD", "?", or "to be confirmed" sections. If a user answer changes scope, re-interview before submitting.

   <HARD-GATE>If you find yourself typing "1." "2." "3." questions in assistant text — STOP. Use `request_user_input`.</HARD-GATE>

4. **Draft inline, then submit via `<proposed_plan>`. Wait for user approval.** Plan Mode is already active (step 1); draft the plan as your own assistant text and then wrap the full plan markdown in `<proposed_plan>...</proposed_plan>`. This renders the plan through Codex's native plan-review flow for accept/reject. After submitting `<proposed_plan>`, stop until the plan-review approval arrives; ordinary chat approval is not implementation approval.

   Caveat: `update_plan` is a progress checklist tool, not a planning-mode substitute. Use it after approval to track implementation progress; do not use it to bypass Plan Mode.

   **Phase format for multi-phase plans.** If the plan has 3+ distinct work units, structure it with a `## Phases` checklist and matching `### Phase X:` headings. The post-approval probe reads this format to offer dispatch; `$agent-dashboard:implement` parses it to drive the dispatch loop. Plans without it can't be dispatched.

   ```markdown
   ## Phases

   - [ ] **Phase A: <short name>** — files: <globs>, deps: -
   - [ ] **Phase B: <short name>** — files: <globs>, deps: A
   - [ ] **Phase C: <short name>** — files: <globs>, deps: B

   ### Phase A: <short name>

   <10–50 lines: what files, Verification profile (Surgical/Targeted/Full) + proof command, what invariants, what to leave alone.>

   ### Phase B: <short name>

   <...>
   ```

   Rules:
   - The `## Phases` block is the dispatch index. Phase names MUST match between checklist and `### Phase X:` headings (case-sensitive).
   - Each phase body MUST name a Verification profile and proof command so `$agent-dashboard:implement` does not default to whole-repo tests for isolated work.
   - `deps:` defaults to "depends on previous phase". Use `-` for "no dependencies".
   - `- [ ]` = pending. `- [x]` = done. `$agent-dashboard:implement` flips these as it dispatches.
   - **Fewer than 3 work units?** Skip this format. Inline paragraphs are fine; the probe won't fire below the threshold.

   **`<proposed_plan>` is the only acceptable submission.** Pasting the plan as ordinary assistant text is a violation, even if you also update a checklist afterwards. The user reviews and approves through the plan-review UI — nowhere else.

   **No exceptions:**
   - Don't start a "small" preparatory edit while waiting.
   - Don't write the test file "to save time".
   - Don't ask "should I proceed?" in assistant text — the plan-review UI's accept action is the approval.

   **Post-approval actions** (immediately after the user accepts the plan, before Phase 3):

   1. **Write the plan-path sentinel** (always). Save the approved plan markdown to a worktree-local file and record that absolute path so `$agent-dashboard:implement` can find it:
      ```bash
      echo "<absolute-plan-path>" > .feature-plan-path
      ```

   2. **Kick off environment setup as a background `exec_command`** (always). The setup task runs in parallel with the dispatch probe and any pre-Phase-3 work so its install time is amortised. It must:

      1. Auto-detect project type from project files (highest match wins):

         | Priority | Signal | Type |
         |----------|--------|------|
         | 1 | `react-native` in package.json dependencies | Mobile |
         | 2 | `next`, `vite`, or `webpack` in package.json | Web |
         | 3 | `requirements.txt`, `pyproject.toml`, or `setup.py` | Python |
         | 4 | `go.mod` | Go |
         | 5 | `Dockerfile` or `docker-compose.yml` | Containerized |

         Ask the user only if no signal matches.

      2. Install dependencies appropriate for the project type (e.g. `pip install`, `npm install`, `go mod download`). Configure ports, create emulators/simulators as needed. For browser UI work, allocate worktree-local Playwright/server/profile/output resources per `../_shared/ui-automation.md`.
      3. Symlink large source-content directories (`data/`, `datasets/`, `evals/`, `models/`, `artifacts/`) from the source repo rather than copying. NEVER symlink build outputs or per-project caches (`.next/`, `dist/`, `build/`, `out/`, `target/`, `.turbo/`, `.cache/`, `.parcel-cache/`, `.vite/`, `__pycache__/`, `.pytest_cache/`, `.gradle/`, `.venv/`, `node_modules/`) — they bake absolute paths and corrupt across worktrees, and must be regenerated per-worktree.
      4. On success, write a sentinel file: `touch .env-setup-done`
         On failure, write the error: `echo "<error message>" > .env-setup-failed`

   3. **Count the phases.** Read the plan; count `- [ ]` / `- [x]` lines under `## Phases`. If there's no `## Phases` block or the count is `< 3`, skip step 4 below and start Phase 3 (inline TDD).

   4. **Probe for dispatch handoff** (only when phase count ≥ 3). Call `request_user_input` exactly once when available; otherwise ask one concise direct question and wait for the user's answer. Never choose the recommended option yourself.
      - Question: `"Plan has {N} phases. Continue inline here, or hand off to $agent-dashboard:implement for context isolation?"`
      - Header: `"Dispatch"`
      - Options (recommended first):
        - `"Continue inline (Recommended for ≤4 phases)"` — Stay in this session; run RED → GREEN → REFACTOR per phase in order.
        - `"Hand off to $agent-dashboard:implement"` — Exit $agent-dashboard:feature. The user invokes `$agent-dashboard:implement` in a fresh session; each phase dispatches to its own subagent.

      **If `Continue inline`:** start Phase 3. The `## Phases` structure becomes documentation — inline TDD ignores the index.

      **If `Hand off to $agent-dashboard:implement`:** print the message below and exit cleanly. Do not start Phase 3.

      ```
      Plan saved to <plan-path>.
      Worktree ready at <worktree-path>.
      To continue, run:

          $agent-dashboard:implement

      (Recommended: open a fresh terminal session for max context isolation.)
      ```

**Gate:** Plan approved with no open decisions. `.feature-plan-path` written. Either Phase 3 begins (inline) or the skill exited with the handoff message (dispatch).

---

### Phase 3: Implement

**Pre-gate:** Check for `.env-setup-done` in the worktree root.
- If present: verify dependencies are installed (e.g. `node_modules/` exists, `pip list` succeeds, `go env GOPATH` works) and data symlinks resolve correctly.
- If `.env-setup-failed` exists: surface the error and halt.
- If neither file exists: the background agent is still running — wait for it to finish before proceeding.

**Effort note:** When launched via the agent-dashboard's New Agent flow, this skill starts at implementation effort. Codex Plan Mode is the high-reasoning planning surface; once the approved plan is accepted, use `update_plan` for progress tracking and keep implementation effort proportional to the work.

**Delegation gate:** Use Codex `spawn_agent` **only if** the user explicitly requested subagents OR the plan touches 10+ files / ~3,000+ lines of implementation. Below that threshold, the orchestration overhead (skill loading, prompt construction, subagent context, result parsing, review) costs more tokens than implementing directly. If delegating, use `$agent-dashboard:implement` with the approved plan (Phase 2) as implementation context, then skip to the phase gate. Otherwise, proceed below.

Build the feature using the active AGENTS.md/core proportional verification doctrine. The profile taxonomy is owned by core rules, not this skill. This skill only selects the profile, records the proof command, and runs that command.

Loop:

1. **Profile first.** State `Verification profile: <Surgical|Targeted|Full>` and `Proof command: <command or none>` before editing. Escalate the profile if the diff grows.
2. **RED where it adds value.** When the selected profile calls for behavior or regression coverage, write the smallest failing test first and show the failing output. Do not add implementation-only tests; when no new test is warranted, name the existing proof or why no executable proof applies.
3. **GREEN.** Write the minimum implementation and rerun the proof command until it passes. No "while I'm here" additions or premature abstractions.
4. **REFACTOR.** Clean up only structure. Rerun the proof command after meaningful edits; escalate if the refactor crosses package boundaries or changes shared behavior.
5. **Final implementation proof.** Before committing from this skill, run the profile's proof command. The PR skill owns the final branch-wide cleanup, formatting, and full test gate.

For UI verification, prefer headless Playwright with worktree-local resources. Use interactive Browser/Chrome inspection only when the shared policy says it is warranted.

**Gate:** Environment ready. The selected verification profile passes. Implementation matches the approved plan, and unnecessary new tests were not added.

---

### Phase 4: Review

Review all changes for correctness, security, and convention adherence. Apply all project rules and conventions that are in your context.

**Gate:** No critical or high-severity issues remain.

---

### Phase 5: Deliver

1. Commit the feature changes with a `feat:` conventional commit message.
2. Open the PR by invoking **`$agent-dashboard:pr`**. That skill owns conditional cleanup/formatting, final test gating when available, push, and `gh pr create`. Do not call `gh pr create` directly — a `pr-skill-gate` hook will block it.

**Gate:** Clean commit history with conventional commit messages. PR opened via `$agent-dashboard:pr`.

---

### Phase 6: Cleanup (on merge)

Triggered when the user indicates the feature has been merged upstream.

1. Verify the branch is merged (warn if unmerged commits remain)
2. Tear down environment resources: remove symlinks, stop dev servers or emulators, release any browser lease, remove worktree-local UI scratch state, delete `.env-setup-done` / `.env-setup-failed` / `.feature-plan-path` sentinel files
3. Remove worktree and delete branch
4. Confirm cleanup is complete

---

## Red Flags — STOP

Failure modes the MUST block doesn't already cover. If you catch yourself saying any of these, pause:

- "I'll just sketch the implementation first" → Phase 3 verification violation. Pick a profile, then follow it.
- "The plan is obvious, let me start" → Phase 2 gate violation. Wait for `<proposed_plan>` approval.
- "Tests pass on my reading of the code" → no executable proof. Run the profile's command, or state why none applies.
- "I'll just call `gh pr create` directly" → Phase 5 violation. The `pr-skill-gate` hook will block it; use `$agent-dashboard:pr`.
- "I'll bundle this unrelated cleanup into the feature commit" → split it into a separate PR.
- "User picked hand-off, but I'm already here — I'll just do Phase 3 myself" → exit cleanly. They opted out of inline TDD for a reason; don't second-guess.
