---
name: investigate
description: Deep-dive into a codebase question, failure, or architectural concern without making changes
disable-model-invocation: true
---

Investigate a codebase question or concern. **This is a read-only skill — do not modify any files.**

Question or concern: $ARGUMENTS

## Instructions

Follow these phases in order. Apply all project rules and conventions that are in your context.

This skill is **read-only**. Do not checkout another branch, pull from remotes, fetch, stash, or otherwise mutate repo state — even "preparatory" steps. Investigate the current branch. If you need a fresher base ref than the local `origin/main`, ask before refreshing it.

---

### Phase 1: Scope

1. Parse the question or concern — is it about behavior, architecture, performance, a failure, a dependency, or something else?
2. If the scope is ambiguous, ask the user to clarify before proceeding.
3. Identify the likely entry points in the codebase (files, modules, services).

**Gate:** The investigation scope is clear and bounded.

---

### Phase 2: Research

Use read-only tools only. Do not edit, write, or create any files.

1. **Trace code paths** — read the relevant source files, following the call chain from entry point to the area of interest.
2. **Read tests** — understand what is tested and what is not. Look for edge cases and assumptions.
3. **Check git history** — use `git log --all -S "<term>"` to find when relevant code was introduced or changed. Use `git blame` for context on specific lines.
4. **Check dependencies** — if the question involves a library or framework, look up its documentation (use context7 if available).
5. **Check configuration** — read config files, environment variables, and infrastructure definitions that affect behavior.

Explore broadly first, then narrow to the relevant areas. Take notes as you go.

---

### Phase 3: Report

Present a structured report to the user:

1. **Findings** — what you discovered, with file paths and line references.
2. **How it works** — trace the relevant code path, explaining the flow.
3. **Risks** — anything concerning: missing tests, edge cases, security issues, performance bottlenecks, implicit assumptions.
4. **Recommended next steps** — concrete actions the user could take (e.g., "run `$agent-dashboard:fix` to address the null check at `src/auth.py:42`", "run `$agent-dashboard:refactor` to extract the retry logic into a shared utility").

**Gate:** The user has received a clear, actionable report. No files were modified.

---

### Transition to implementation

This skill is read-only. If the user asks to implement changes based on your findings, **do not start editing files**. Instead, hand off to the appropriate skill:

- New feature or behavioral change → suggest `$agent-dashboard:feature <description>`
- Bug fix → suggest `$agent-dashboard:fix <description>`
- Restructuring existing code → suggest `$agent-dashboard:refactor <description>`

These skills handle branch/worktree setup, TDD, review, and delivery. Starting implementation inline from `$agent-dashboard:investigate` skips those gates.
