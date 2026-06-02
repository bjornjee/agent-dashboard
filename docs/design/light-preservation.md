# Light mode redesign — preservation contract

The grader subagent uses this file as the ground truth for "what the light-mode redesign must not break". Any change to the codebase under the light-mode-redesign work that violates one of the bullets below is a regression and must be flagged.

This contract is **verbatim** — implementer code must satisfy every bullet exactly as written.

## Preservation contract

- Dark mode tokens: bit-exact unchanged. Every value declared in `register.md` "Color" / "Token mapping table" continues to resolve to the same hex under `[data-theme="dark"]` (or no `data-theme`). The light-mode addendum only adds values; it does not retune dark.
- Existing `[data-theme="light"]` token values in `internal/web/static/style.css:1858-1920`: bit-exact unchanged. The light addendum to `register.md` **documents** these values; it does not introduce new ones. If a value needs to change, it goes through a separate proposal.
- Theme toggle mechanism: untouched. Whatever switches `[data-theme="dark"]` ↔ `[data-theme="light"]` today (the existing code path that owns the toggle) is preserved. The light redesign does not add a new toggle surface, does not change the toggle's location, and does not re-key the storage value.
- Mobile dock semantics: unchanged. The floating dock continues to render below 900px with `Search agents` + `+ New` pills; the kebab sheet continues to host `Usage`, `Settings`, `Notifications`. Only the rendered colors swap per `[data-theme]`.
- Existing playwright assertions in `tests/playwright/tests/desktop-redesign.spec.js`: continue to pass. The light redesign does not break any selector, any computed-style check, any viewport assertion, or any of the desktop-shell behaviors the spec already covers. New light-mode assertions land in a separate spec; the existing dark-mode spec is preserved verbatim.
- Public HTTP API surface (`/api/agents`, `/events`, etc.): unchanged.
- `ui.js` primitive public APIs (appBar, dock, sheet, row, sectionLabel, card, message, composer, input): unchanged signatures. Internal CSS classes may change.
- SSE event handling, history routing, composer state preservation across action-bar swaps: untouched.
- Keyboard shortcuts: unchanged.
- Mobile layout below `900px`: bit-exact unchanged in structure. Only the rendered colors swap under `[data-theme="light"]`. The mobile dark rendering continues to match the existing visual regression baseline.
- Desktop layout at `900px ≤ viewport < 1600px`: structure unchanged from the desktop redesign (sidebar 264px + main pane). Only the rendered colors swap under `[data-theme="light"]`.
- Status colors (`--status-*-bg/border/text`): the light-mode values shipped in `style.css:1893-1910` are preserved as-is. The light register addendum does not re-promote these tokens; they remain owned by the shipped CSS block.

## What this contract intentionally does not preserve

The following items are allowed to change in this redesign and must NOT be flagged as regressions:

- The light-mode rendering of any surface that was previously declared dark-only in `register.md`. The chat-bubble inversion (user message = dark pill in light) is the canonical example — this is a new rule, not a regression against an older light rendering.
- The way `register.md` documents light values. Promoting light from a side-column to a first-class section is a documentation change, not a behavioral one.
- The `--bp-desktop` value in `desktop-register.md`: 1024px → 900px. This is a doc reconciliation with the shipped CSS (`@media (min-width: 900px)` blocks). The CSS already uses 900px; the doc was the drift.
- The new `--bp-monitor: 1600px` token's appearance in the doc register. The token is declared here for the record; no `@media (min-width: 1600px)` block ships in this phase. A later phase consumes it.
