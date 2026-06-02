# Verdict — Phase D iteration 1 (post-fix)

Viewport: 1440×900, `[data-theme="light"]`. Screenshots:
`iter-light-laptop-1-list.png`, `iter-light-laptop-1-detail.png`,
`iter-light-laptop-1-diff.png`, `iter-light-laptop-1-create.png`,
`iter-light-laptop-1-usage.png`.

Per the rubric in `docs/design/rubric-weights.md`, scoring 1 (broken) → 5 (excellent).

## Per-dimension raw scores

| Dimension | Weight | Raw | Weighted | Notes |
|---|---|---|---|---|
| user-flow-fidelity | 1.0 | 5 | 5.0 | List → detail → reply / sidebar nav / create / usage all render correctly. Selected row is now unmistakable. |
| visual-register-match | 1.5 | 4 | 6.0 | Composer card lift is now soft `0 8px 24px rgba(0,0,0,0.06)` — matches the register's "subtle shadow on lifted surfaces" rule (register.md line 224). Composer border promoted to `--border-default` so the white card edge reads. Inset white-on-white highlight kept on dark-only since the override applies only on `[data-theme="light"]`. |
| content-density | 1.5 | 4 | 6.0 | Unchanged from iter 0; density was correct already. |
| affordance-honesty | 1.0 | 5 | 5.0 | Sidebar selected row now telegraphs active state via the 2px `--text-primary` left-accent — no ambiguity. Composer chrome reads as inline sticky card, not as a modal/popup. |
| brand-voice-adherence | 0.5 | 4 | 2.0 | n/a. |
| cross-locale-consistency | 0.5 | N/A | — | EN-only. |

**Weakest weighted score: 5.0** (user-flow-fidelity, affordance-honesty). **PASSES the ≥ 4 gate.** 6 of 7 dimensions ≥ 4 (cross-locale N/A as per rubric).

## Notable observations

- **Sidebar selected row.** The 2px left-accent is rendered as
  `border-left` inset; the `padding-left` is reduced by 2px so the row's
  text origin is unchanged. No layout shift between selected and
  unselected rows. Verified visually in iter-1 list (Usage selected),
  iter-1 detail (top agent row selected), iter-1 create (New agent row
  selected).
- **Composer card.** In iter-0 the `rgba(0,0,0,0.35)` 12px shadow read as
  a heavy modal lift; in iter-1 it's a calm `0 8px 24px rgba(0,0,0,0.06)`
  drop. The card border promoted to `--border-default` (#E4E4E7) is
  clearly visible against the elevated fill (#F2F2F4) and the white
  pane (#FFFFFF).
- **Composer chips.** Now carry `--border-default` rather than
  `--border-subtle`, so the chip edges read against the now-stronger
  composer card edge. Chip hover promotes to `--border-hover` for
  affordance.
- **Diff tab.** hljs swap to `github.min.css` works (verified via
  `getElementById('hljs-theme').href`). No syntax-highlight artefacts in
  the current empty diff; the d2h light defaults blend with our
  `--bg-base` / `--bg-surface` overrides cleanly.
- **Dark mode.** Preservation contract verified by two assertions in
  `light-laptop.spec.js`:
  - Selected sidebar row in dark has 0px left-border (no accent).
  - Composer in dark keeps its `rgba(0,0,0,0.35)` heavy lift.

## Status

PASS at iteration 1/3. No further passes required.
