---
name: pr
description: Create a pull request with full diff analysis, summary, and test plan
when_to_use: when the user says "open a PR", "create a pull request", invokes "/pr", or asks for the current branch to be pushed and PR'd. NOT for branch creation (that's part of /feature, /fix, /refactor) or for editing an existing PR (use `gh pr edit` directly).
version: 1.0.0
disable-model-invocation: true
---

Create a pull request for the current branch.

Optional arguments: $ARGUMENTS

## Instructions

Follow these phases in order.

---

### Phase 0: Sync

1. Switch to main: `git checkout main`
2. Pull latest: `git pull origin main`

---

### Phase 1: Analyze

Run all of these in parallel:

1. `git status` — check for uncommitted changes (warn if any)
2. Detect the default branch: `BASE=$(git symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed 's|refs/remotes/origin/||' || echo main)`
3. `git log --oneline $(git merge-base HEAD "$BASE")..HEAD` — all commits on this branch
4. `git diff "$BASE"...HEAD` — full diff from base branch
5. `git branch --show-current` — current branch name
6. Check if the branch has a remote tracking branch: `git rev-parse --abbrev-ref @{upstream} 2>/dev/null`

Analyze **all commits**, not just the latest. Identify:
- Type of change (feature, fix, refactor, etc.)
- What files were touched and why
- Any breaking changes or migration steps needed

**Gate:** You understand the full scope of changes across all commits.

---

### Phase 2: Draft

Generate a PR using this structure:

- **Title:** Under 70 characters. Matches the primary change type. Use conventional format: `<type>: <description>`.
- **Summary:** 1-3 bullet points explaining what changed and why.
- **Test plan:** Bulleted checklist of how to verify the changes.

**Don't pause to ask "should I push?".** Invoking this skill is the approval. Push + create in one step (Phase 3). Only stop if the diff has something dangerous — see Red Flags below.

**No exceptions:**
- Don't amend or rewrite existing commits to "tidy" the history. Squash happens at merge time. Create new commits if more changes are needed.
- Don't leave secrets, `.env` contents, large binaries, or `node_modules/` in the diff. Inspect `git status` and `git diff` before pushing.
- Don't write a generic "updates the X" summary — explain *why* the change matters, not just what it does.

**Gate:** Draft is written and the diff is clean of dangerous content.

---

### Phase 3: Push and Create

1. If no remote tracking branch exists, push with: `git push -u origin $(git branch --show-current)`
2. If remote tracking branch exists but is behind, push: `git push`
3. Create the PR:

```
gh pr create --title "<title>" --body "$(cat <<'EOF'
## Summary
<bullet points>

## Test plan
<checklist>

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

4. Return the PR URL to the user.

**Gate:** PR is created and URL is displayed.

---

## Red Flags — STOP

If you catch yourself saying or thinking any of these, pause and re-read the relevant phase:

- "I'll amend the last commit to tidy it" → no. Create a new commit. Squash on merge.
- "Force-push to fix the history" → no, unless the user explicitly asked. Force-push is destructive.
- "Push to main directly, the change is small" → blocked by hook. Branch first.
- "Open the PR against main even though I branched off feat/X" → check the base. Stacked branches must target their parent, not main.
- "Skip Phase 1 analysis, only the latest commit matters" → analyze **all** commits between base and HEAD. The summary spans the whole branch.
- "The diff includes a `.env` / large binary / secret — I'll PR anyway" → STOP. Investigate before pushing. `git rm --cached` and amend if needed.
- "I'll write the summary as `updates X`" → say *why*, not *what*. The diff already shows what.
