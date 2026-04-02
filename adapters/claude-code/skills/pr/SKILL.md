---
name: pr
description: Create a pull request with full diff analysis, summary, and test plan
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

Present the draft to the user for review. Wait for approval or edits before proceeding.

**Gate:** User has approved or edited the PR content.

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
