# Worktree setup

Create an isolated git worktree for this task. `<prefix>` is the branch prefix the calling skill specifies (`feat`, `fix`, or `refactor`).

1. Derive a short kebab-case name from the task description
2. Derive the app name from the git repo: `basename $(git rev-parse --show-toplevel)`
3. Switch to main: `git checkout main`
4. Pull latest: `git pull origin main`
<!-- claude-only -->
5. Create branch `<prefix>/<name>` and worktree `../worktrees/<app>/<name>` from main:
   `mkdir -p ../worktrees/<app> && git worktree add ../worktrees/<app>/<name> -b <prefix>/<name> main`
<!-- /claude-only -->
<!-- codex-only -->
5. Create branch `<prefix>/<name>` and worktree `../worktrees/<app>/<name>` from main. Run **two separate `exec_command` tool calls** — do not chain them with `&&`. The dashboard's PostToolUse hook only stamps `worktree_cwd` + `branch` when the command starts with `git worktree add`; a compound `mkdir … && git worktree add …` slips past the regex and leaves the dashboard unable to detect dir or branch.

   First, ensure the parent directory exists:
   ```
   mkdir -p ../worktrees/<app>
   ```
   Then run `git worktree add -b <prefix>/<name> ../worktrees/<app>/<name> main` as its own `exec_command` tool call:
   ```
   git worktree add -b <prefix>/<name> ../worktrees/<app>/<name> main
   ```
<!-- /codex-only -->
   - If the branch already exists, ask the user whether to resume it or choose a new name.
6. **From the source repo root** (before cd'ing), copy environment files into the worktree **preserving their exact relative path from the project root**:
   - Find all env files recursively: `find . -name '.env*' -not -name '.env-setup-done' -not -name '.env-setup-failed' -not -path './.git/*' -not -path './node_modules/*'`
   - For each file found, recreate its directory structure in the worktree and copy it. For example:
     - `./.env` → `../worktrees/<app>/<name>/.env`
     - `./services/api/.env.local` → `../worktrees/<app>/<name>/services/api/.env.local`
   - Use: `for f in $(find . -name '.env*' -not -name '.env-setup-done' -not -name '.env-setup-failed' -not -path './.git/*' -not -path './node_modules/*'); do mkdir -p "../worktrees/<app>/<name>/$(dirname "$f")" && cp "$f" "../worktrees/<app>/<name>/$f"; done`
   - If `.claude/settings.local.json` exists: `mkdir -p ../worktrees/<app>/<name>/.claude && cp .claude/settings.local.json ../worktrees/<app>/<name>/.claude/`
<!-- claude-only -->
   - **Important:** All Bash tool calls in this step must set `dangerouslyDisableSandbox: true` because they write outside the project root.
7. cd into the worktree, run `node "${CLAUDE_PLUGIN_ROOT:-$(ls -dt "$HOME/.claude/plugins/cache/agent-dashboard/agent-dashboard"/* 2>/dev/null | head -1)}/scripts/hooks/claim-worktree.js"`, and confirm with `pwd` and `git branch --show-current`
<!-- /claude-only -->
<!-- codex-only -->
   - **Important:** Commands in this step write outside the project root. Use Codex escalation (`sandbox_permissions: "require_escalated"`) with a concise justification; do not try to route around approvals.
7. cd into the worktree, run `node "${PLUGIN_ROOT:-${CLAUDE_PLUGIN_ROOT:-$(ls -dt "${CODEX_HOME:-$HOME/.codex}/plugins/cache/agent-dashboard/agent-dashboard"/* 2>/dev/null | head -1)}}/hooks/claim-worktree.js"`, and confirm with `pwd` and `git branch --show-current`
<!-- /codex-only -->
8. Verify: compare env files between source and worktree. Run the same `find` command in both directories and diff the file lists. If any files are missing in the worktree, **halt and report failure**. If the source repo had no `.env*` files, note that explicitly.
