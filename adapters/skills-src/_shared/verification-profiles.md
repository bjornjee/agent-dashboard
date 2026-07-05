# Verification Profile Glossary

Use this glossary when an agent-dashboard skill asks for a Verification profile. It is the minimal standalone contract for the plugin; if active AGENTS.md/core rules provide more specific doctrine, follow those rules.

- **Surgical:** docs, rules, config, generated metadata, or mechanical edits where a new test would only mirror the implementation. Run no test unless a relevant validator exists.
- **Targeted:** isolated behavior with nearby coverage or a clear regression risk. Use the smallest relevant test, package command, or validator. Use RED -> GREEN -> REFACTOR when changing behavior.
- **Full:** public APIs, shared state, persistence, auth/security, migrations, concurrency, build/test infrastructure, broad refactors, or risk that cannot be bounded. Run the repo's full project gate or documented equivalent.

Always record the selected profile and proof command before editing. Escalate the profile if the diff grows beyond the original risk boundary.
