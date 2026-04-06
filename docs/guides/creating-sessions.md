---
title: Creating Sessions
parent: Guides
nav_order: 2
---

# Creating Sessions

Press `a` in the dashboard to create a new agent session. The session creator guides you through directory selection and skill assignment.

---

## Directory selection

The session creator uses the [z zsh plugin](https://github.com/agkozak/zsh-z) to suggest directories ranked by frecency (frequency + recency). Start typing a path and suggestions appear automatically.

{: .note }
The z plugin is optional. Without it, you can type full paths manually.

## Skill selection

After choosing a directory, select a skill that determines the agent's workflow:

| Skill | Purpose |
|:------|:--------|
| **feature** | New feature development with TDD in an isolated git worktree |
| **fix** | Bug fix with root cause analysis |
| **chore** | Non-code changes (docs, config, CI) |
| **refactor** | Code restructuring with test preservation |
| **investigate** | Research and analysis without code changes |
| **pr** | PR review and iteration workflow |
| **rca** | Root cause analysis for incidents |

Each skill loads a specialized prompt that guides the agent through the appropriate workflow — including branching strategy, testing requirements, and delivery steps.

## What happens next

The dashboard:

1. Creates a new tmux pane
2. Starts Claude Code in the selected directory with the chosen skill
3. The adapter hooks begin writing agent state files
4. The agent appears in the dashboard list within seconds
