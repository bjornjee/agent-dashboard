# Verdict — Phase D iteration 0 (baseline)

Viewport: 1440×900, `[data-theme="light"]`. Screenshots:
`iter-light-laptop-0-list.png`, `iter-light-laptop-0-detail.png`,
`iter-light-laptop-0-diff.png`, `iter-light-laptop-0-usage.png`.

Per the rubric in `docs/design/rubric-weights.md`, scoring 1 (broken) → 5 (excellent).

## Per-dimension raw scores

| Dimension | Weight | Raw | Weighted | Notes |
|---|---|---|---|---|
| user-flow-fidelity | 1.0 | 4 | 4.0 | List → detail → reply path renders correctly; invariants honored. |
| visual-register-match | 1.5 | **2** | **3.0** | Composer card carries a `rgba(0,0,0,0.35)` 12px shadow inherited from the dark register. On a white pane this is a heavy charcoal smudge that screams "dark-mode chrome dropped onto light". Reads as a register violation per `register.md` line 219 ("layered, not bordered, inverted in light") and `register.md` line 224 ("only shadow token in use in light is `--shadow-medium`"). Selected sidebar row `#E4E4E7` against panel `#F4F4F5` is below the perceptual contrast threshold — the register addendum's note about "combine fill + 1px left-border accent" (phase plan) is unimplemented. |
| content-density | 1.5 | 4 | 6.0 | Density is correct. |
| affordance-honesty | 1.0 | **3** | **3.0** | The selected sidebar row is too quiet to telegraph the active selection at a glance — affordance ambiguity. The composer dark-shadow reads as a modal/popup affordance when it's really an inline sticky card. |
| brand-voice-adherence | 0.5 | 4 | 2.0 | n/a — passes. |
| cross-locale-consistency | 0.5 | N/A | — | EN-only per rubric. |

**Weakest weighted score: 3.0** (visual-register-match, affordance-honesty). **FAILS the ≥ 4 gate.**

## Findings (in order of impact)

1. **Composer-card shadow is dark-register chrome.** `style.css` line ~2494 sets
   `box-shadow: 0 1px 0 rgba(255,255,255,0.02) inset, 0 12px 32px rgba(0,0,0,0.35)`
   unconditionally inside `@media (min-width: 900px)`. On light this is a
   heavy charcoal lift that violates `register.md` line 224 (light's only
   shadow token is `--shadow-medium = rgba(0,0,0,0.06)`). Replace the
   composer's `box-shadow` with a token-driven value so dark keeps its lift
   and light gets the soft `--shadow-medium`. Inset white-on-white is also
   noise in light — drop the inset highlight.

2. **Composer-card border too thin in light.** The current border is
   `--border-subtle` (`#E8E8EB`) against a `--bg-elevated` fill (`#F2F2F4`)
   on a `--bg-base` pane (`#FFFFFF`). The `#F2F2F4` vs `#FFFFFF` step is
   real but tiny; `#E8E8EB` border is only 6 units darker than `#F2F2F4`.
   Promote the composer to `--border-default` (`#E4E4E7`) in light so the
   card's outline is the dominant separation cue (per register: "1px
   border-default around cards" in light, line 219).

3. **Sidebar selected-row needs a left-border accent in light.** The plan
   explicitly calls this out: "combine fill + 1px left-border accent" per
   the register addendum. Today the selected row in light is fill-only
   (`#E4E4E7` on `#F4F4F5`) with **no** left-accent. Add a 2px
   `--text-primary` left-border inside the row (inset, so layout doesn't
   shift) for `[data-theme="light"]` at `≥ 900px`. Dark keeps its current
   pure-fill selection because the dark register relies on the
   darkness-step (selected `#222226` > sidebar `#0E0E10`), which already
   reads strongly. This is purely a light-mode reinforcement.

4. **Composer-chip border in light is faint.** Chips use the same
   `--border-subtle` fallback. Visible but quiet — defer; the dominant
   composer-card border (fix #2) reasserts the composer's edge enough that
   chip-edge contrast is no longer load-bearing.

5. **`Stop` button (red square) and `Send` button on composer.** These
   render via tokens; in light the red-square colour reads with appropriate
   alarm. No change.

6. **Diff tab: hljs swap to `github.min.css` works.** Confirmed via
   `getElementById('hljs-theme').href`. No syntax-highlight artefacts
   spotted in the current diff (empty file `.env-setup.log`). The d2h
   styles render with d2h's own light-friendly default + our `--bg-base`
   / `--bg-surface` overrides — verified visually.

## Targets for iteration 1

- Replace composer `box-shadow` with a `var(--composer-shadow)`-style
  token. Define dark = current heavy lift, light = `0 8px 24px
  var(--shadow-medium)` (matches register's "subtle shadow on lifted
  surfaces" rule).
- Drop the inset highlight in light (it's white-on-white).
- Promote composer border to `var(--border-default)` in light only.
- Add 2px `var(--text-primary)` inset left-border to sidebar selected
  rows in light only.

Target post-fix score: visual-register-match raw 4 → weighted 6.0;
affordance-honesty raw 4 → weighted 4.0. Weakest weighted ≥ 4 → PASS.
