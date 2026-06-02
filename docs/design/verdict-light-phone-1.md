# Verdict — light mobile, iteration 1 (PASS)

Cold-context grader run against `docs/design/light-flow-map.md` (mobile section),
`docs/design/register.md` (Light mode section), and the preservation contract
in `docs/design/light-preservation.md`. Viewport: 412×900, `[data-theme="light"]`.

Screenshots (post-fix):
- `iter-light-phone-1-list.png` (A1-m-light) — dock pill is now white card with hairline outline; `+ New` CTA inverts to dark.
- `iter-light-phone-1-detail.png` (A2-m-light) — user-message dark pill on white page; agent prose left-aligned.
- `iter-light-phone-1-create.png` (B1-m-light) — disabled Spawn carries the hairline outline → readable geometry.
- `iter-light-phone-1-kebab.png` (C1-m-light-kebab) — sheet items, separators, page chrome all light-correct.

## Rubric scores (weighted)

| Dimension | Raw | Weight | Weighted | Notes |
|---|---|---|---|---|
| user-flow-fidelity | 5 | 1.0 | 5.0 | Flow-map A1/A2/B1/C1 invariants all render correctly. BLOCKED/WAITING section ordering preserved. Last-assistant-message-in-first-viewport (A2 invariant) unchanged from dark. |
| visual-register-match | 5 | 1.5 | 7.5 | Dock pill now matches flow-map C1 verbatim (`--bg-surface` + `--border-default` + `--shadow-medium`). User bubble matches A2 verbatim (`--cta-bg` / `--cta-text`). White surface no longer carries any dark-mode artifacts. |
| content-density | 4 | 1.5 | 6.0 | Generous Codex spacing preserved. No densification. |
| affordance-honesty | 5 | 1.0 | 5.0 | Disabled Spawn now reads as a button shape (outline). User bubble = visual "you" pill; agent message = visual "them" prose — register matches Codex iOS. Dock CTA reads as the primary action (dark fill on white card). |
| brand-voice-adherence | N/A | 0.5 | N/A | No declared voice. |
| cross-locale-consistency | N/A | 0.5 | N/A | EN-only. |

## Pass threshold

Weakest non-N/A weighted = **5.0** (user-flow-fidelity, affordance-honesty). Threshold ≥ 4. **PASS** on iteration 1.

## Preservation contract verification

- Dark mode tokens: unchanged. All three CSS changes are scoped under `[data-theme="light"]` overrides (`.ui-dock`, `.ui-msg--user .ui-msg__bubble`, `.create-spawn:disabled`). The base `.ui-dock` rule got a `border: 1px solid transparent` added — this is a no-op visual in dark mode (transparent border) and exists only as a slot the light override fills with `--border-default`.
- `@media (min-width: 900px)` and `@media (min-width: 1600px)` blocks: untouched.
- Existing playwright spec `desktop-redesign.spec.js`: continues to pass (51 tests).
- `light-foundation.spec.js` (Phase B): continues to pass (8 tests).
- Public API, ui.js primitives, SSE, keyboard shortcuts: untouched (CSS-only diff).

## Iterations used

**1** (initial + 1 revision). Under the 3-pass budget.

## Out-of-scope items surfaced (not fixed in Phase C)

- The dark-mode `.ui-dock__search:hover { rgba(255,255,255,0.04) }` rule remains — under light it's a no-op (because no white-on-white hover ever fires there since the search button has `background: transparent`). The light override `[data-theme="light"] .ui-dock__search:hover { background: var(--bg-hover) }` was added. Cleaning up the dark rule to use a token is a refactor for a later phase; doing it here would touch dark mode and violate the preservation contract.
