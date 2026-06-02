# Desktop redesign — preservation contract

The grader subagent uses this file as the ground truth for "what the desktop redesign must not break". Any change to the codebase under the desktop-redesign work that violates one of the bullets below is a regression and must be flagged.

This contract is **verbatim** — implementer code must satisfy every bullet exactly as written.

## Preservation contract

- Mobile layout below `1024px`: bit-exact unchanged. Existing visual regression for mobile must keep passing.
- Public HTTP API surface (`/api/agents`, `/events`, etc.): unchanged.
- `ui.js` primitive public APIs (appBar, dock, sheet, row, sectionLabel, card, message, composer, input): unchanged signatures. Internal CSS classes may change.
- SSE event handling, history routing, composer state preservation across action-bar swaps: untouched.
- Keyboard shortcuts: unchanged.
