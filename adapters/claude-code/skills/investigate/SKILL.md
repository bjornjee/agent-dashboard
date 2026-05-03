---
name: investigate
description: Deep-dive into a codebase question, failure, or architectural concern without making changes
when_to_use: when the user asks "how does X work", "why does Y do Z", "what would happen if", or wants a structured exploration of a codebase area without changing anything. NOT for fixing bugs (use /fix), implementing features (use /feature), or system-crash forensics (use /rca).
version: 1.0.0
disable-model-invocation: true
---

Investigate a codebase question or concern. **This is a read-only skill — do not modify any files. No exceptions.**

Question or concern: $ARGUMENTS

## Instructions

Follow these phases in order. Apply all project rules and conventions that are in your context.

---

### Phase 0: Sync

1. Switch to main: `git checkout main`
2. Pull latest: `git pull origin main`

---

### Phase 1: Scope

1. Parse the question or concern — is it about behavior, architecture, performance, a failure, a dependency, or something else?
2. If the scope is ambiguous, ask the user to clarify before proceeding.
3. Identify the likely entry points in the codebase (files, modules, services).

**Gate:** The investigation scope is clear and bounded.

---

### Phase 2: Research

**Read-only tools only. No Edit, no Write, no Bash that mutates state.** Allowed: Read, Grep, Glob, `git log`/`git blame`/`git show`, context7 lookups.

1. **Trace code paths** — read the relevant source files, following the call chain from entry point to the area of interest.
2. **Read tests** — understand what is tested and what is not. Look for edge cases and assumptions.
3. **Check git history** — use `git log --all -S "<term>"` to find when relevant code was introduced or changed. Use `git blame` for context on specific lines.
4. **Check dependencies** — if the question involves a library or framework, look up its documentation (use context7 if available).
5. **Check configuration** — read config files, environment variables, and infrastructure definitions that affect behavior.

Explore broadly first, then narrow to the relevant areas. Take notes as you go.

**No exceptions:**
- Don't "quickly fix" something you notice — log it in the report instead.
- Don't add a print/log statement to confirm a hypothesis — read the code or rerun a test from outside this skill.
- Don't run commands that mutate working tree, config, branches, or remote state.

---

### Phase 3: Report

Present a structured report to the user:

1. **Findings** — what you discovered, with file paths and line references.
2. **How it works** — trace the relevant code path, explaining the flow.
3. **Risks** — anything concerning: missing tests, edge cases, security issues, performance bottlenecks, implicit assumptions.
4. **Recommended next steps** — concrete actions the user could take (e.g., "run `/fix` to address the null check at `src/auth.py:42`", "run `/refactor` to extract the retry logic into a shared utility").

**Gate:** The user has received a clear, actionable report. No files were modified.

---

### Transition to implementation

This skill is read-only. If the user asks to implement changes based on your findings, **do not start editing files**. Instead, hand off to the appropriate skill:

- New feature or behavioral change → suggest `/feature <description>`
- Bug fix → suggest `/fix <description>`
- Restructuring existing code → suggest `/refactor <description>`

These skills handle branch/worktree setup, TDD, review, and delivery. Starting implementation inline from `/investigate` skips those gates.

---

## Red Flags — STOP

If you catch yourself saying or thinking any of these, pause and re-read the relevant phase:

- "I'll just fix this typo while I'm here" → wrong skill. Hand off to `/fix` or `/chore`.
- "Let me add a quick log statement to verify" → no. That's a code change. Read more, or hand off to `/fix`.
- "The user clearly wants a fix, let me skip the report" → no. Deliver the report. Then hand off.
- "I read the code, the answer is X" with no file:line citation → re-read with citations. The report needs them.
- "I'll skip git history, the current code is enough" → check `git log -S` and `git blame`. Bugs hide in recent commits.
