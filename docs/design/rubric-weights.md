# Rubric weights — Codex UX redesign

Per `skills/uiux-design-loop/rubric.md`, default weight is 1.0 per dimension. The `weights.json` mechanism multiplies the raw 1–5 score; pass threshold is "weakest weighted score ≥ 4". Higher weight = a low raw score on that dimension drags the verdict harder.

This PR redesigns the dashboard to match the Codex iOS visual register. Weights are tuned for that goal:

| Dimension | Weight | Rationale |
|---|---|---|
| `user-flow-fidelity` | **1.0** | Standard. The flow map already establishes invariants (e.g., BLOCKED first, last assistant message in first viewport); grader checks those. |
| `visual-register-match` | **1.5** | **Up-weighted.** The whole point of this PR is "look like Codex." A render that's polished but in a different register fails the brief, so this dimension gets the biggest pull. |
| `content-density` | **1.5** | **Up-weighted.** The existing dashboard is dense (heavy cards, multiple meta lines per row); Codex iOS is generous. Risk = swinging too far to sparse-because-stripped and missing the "intentional density" Codex actually has. Up-weighting forces the grader to flag both directions. |
| `affordance-honesty` | **1.0** | Standard. Real risk (composer send button styled as ghost, kebab disclosure unclear), but the register itself defines clear affordance language. |
| `brand-voice-adherence` | **0.5** | **Down-weighted.** The dashboard has no brand-voice rules in a `project-rules.md`. There's no declared tone, no source-doc copy to honor. The dimension would otherwise score `N/A` per rubric.md anchor 3; weight = 0.5 keeps it in the verdict as a sanity check without dragging the overall. |
| `cross-locale-consistency` | **0.5** | **Down-weighted.** Dashboard is EN-only. Per rubric.md, this would score `N/A`. Weight = 0.5 keeps the dimension visible if a future internationalization adds locale screenshots; for this PR it should always be `N/A` and not contribute to the weakest-score gate. |

## Pass threshold for this PR

Weakest *weighted* score ≥ 4 across non-N/A dimensions.

Concrete examples (illustrative):
- Raw `visual-register-match: 3` → weighted `4.5` → still PASSes the gate alone.
- Raw `visual-register-match: 2` → weighted `3.0` → fails (below 4).
- Raw `affordance-honesty: 3` → weighted `3.0` → fails (below 4).
- Raw `content-density: 3` + raw `affordance-honesty: 4` → content-density weighted `4.5`, affordance-honesty weighted `4.0` → PASS.

The down-weighted dimensions (`brand-voice-adherence`, `cross-locale-consistency`) cannot fail the gate alone — they'd need a raw score of 0 or N/A. This is intentional; this PR is purely visual.

## Loop budget

Per `skills/uiux-design-loop/SKILL.md`, hard cap at 6 inner-loop iterations. After iteration 6 without PASS, surface to user — design needs broader rework than this loop can deliver.

For this PR's expected scope (token rewrite + 9 primitives + 3 views), 2–3 iterations per view is the realistic target. Anything beyond 4 iterations on a single view = the register might be wrong, not the implementation.
