# Verdict — light mobile, iteration 0 (baseline)

Cold-context grader run against `docs/design/light-flow-map.md` (mobile section),
`docs/design/register.md` (Light mode section), and the preservation contract
in `docs/design/light-preservation.md`. Viewport: 412×900, `[data-theme="light"]`.

Screenshots:
- `iter-light-phone-0-list.png` (A1-m-light)
- `iter-light-phone-0-detail.png` (A2-m-light)
- `iter-light-phone-0-create.png` (B1-m-light)
- `iter-light-phone-0-kebab.png` (C1-m-light-kebab)

## Rubric scores (weighted)

| Dimension | Raw | Weight | Weighted | Notes |
|---|---|---|---|---|
| user-flow-fidelity | 4 | 1.0 | 4.0 | Flows render; BLOCKED-first invariant holds (no BLOCKED group seeded but section-label hierarchy is intact). |
| visual-register-match | **2** | 1.5 | **3.0** | **BROKEN.** Dock pill renders as a dark glass blob (`rgba(26,26,28,0.88)` hard-coded at `.ui-dock` L226) on a white page — flow map C1 mandates `var(--bg-surface)` (`#FFFFFF`) + 1px `--border-default` with shadow-medium. The dock looks like a foreign dark-mode element parachuted onto the white surface. |
| content-density | 4 | 1.5 | 6.0 | Generous Codex-style spacing preserved. |
| affordance-honesty | **3** | 1.0 | **3.0** | **FAIL.** (a) Disabled `Spawn` button (B1) becomes invisible: `--bg-surface` (`#FFFFFF`) on `--bg-base` (`#FFFFFF`) page + `--text-faint` (`#A1A1AA`). (b) Hard-coded dark hover `.ui-dock__search:hover { rgba(255,255,255,0.04) }` is a no-op in light. (c) `.ui-msg--user .ui-msg__bubble` uses `--bg-elevated`, same fill as the composer chip, attach button, code blocks — the user pill reads as a button, violating flow map A2 ("dark pill on white page — `background: var(--cta-bg)`"). |
| brand-voice-adherence | N/A | 0.5 | N/A | No declared voice. |
| cross-locale-consistency | N/A | 0.5 | N/A | EN-only. |

## Pass threshold

Weakest non-N/A weighted = **3.0** (visual-register-match) and **3.0** (affordance-honesty). Threshold is **≥ 4**. **FAIL.**

## Critique brief for iteration 1

1. **Dock pill background must follow theme.**
   - `.ui-dock` currently sets `background: rgba(26, 26, 28, 0.88)` directly. Replace with a theme-aware default that resolves correctly in light. Two clean options: (a) use a new local CSS custom property defaulted to the dark rgba and overridden under `[data-theme="light"]`, OR (b) keep the dark literal as the `:root` default and override under `[data-theme="light"]` directly on `.ui-dock`.
   - Light value (per flow map C1): `var(--bg-surface)` (`#FFFFFF`), 1px `var(--border-default)`, shadow `0 8px 32px var(--shadow-medium)`.
   - Also retune `.ui-dock__search:hover` to use `var(--bg-hover)` under light (or a token that resolves).

2. **User chat bubble must invert in light.**
   - `.ui-msg--user .ui-msg__bubble` uses `--bg-elevated` (`#F2F2F4` in light) → confuses with buttons.
   - Per flow map A2: `background: var(--cta-bg)` (`#1C1A17`), `color: var(--cta-text)` (`#FFFFFF`). This must apply ONLY in light mode (dark mode keeps the existing `--bg-elevated` bubble — that's covered by `light-preservation.md` line "Dark mode tokens: bit-exact unchanged"). Implementation: override under `[data-theme="light"] .ui-msg--user .ui-msg__bubble`.

3. **Disabled Spawn button must keep an outline in light.**
   - `.create-spawn:disabled` uses `--bg-surface` on a page that also resolves to `--bg-base = #FFFFFF` → invisible. Fix: add a 1px `--border-default` outline OR swap the disabled fill to `--bg-elevated` so it sits visibly one step above the page in light. Dark mode is unaffected because dark `--bg-surface` (`#181B22`) already contrasts with `--bg-base` (`#0B0E14`).

4. **Tap-flash for buttons / row hover already uses tokens (`--bg-elevated`, `--bg-hover`)** — no change needed at this level. Confirmed.

## Out of scope this iteration

- Status badges in the list: not currently shown in mobile rows (status dot only, which uses saturated `--accent-*` colors that remain readable). No change.
- Composer pill: `.ui-composer__input` already consumes `--bg-elevated` → resolves to `#F2F2F4` in light, which is the correct "one step above page" treatment. Confirmed.
- Disabled-state Spawn outline change is a light-mode-only override; dark untouched.

## Iteration plan

Three CSS changes, all scoped via `[data-theme="light"]` overrides — dark mode bit-exact preserved.
