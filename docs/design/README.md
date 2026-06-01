# Design — Codex iOS UX redesign

Reference material and graded design record for the Codex iOS web-UI redesign delivered in PR `feat: codex-ux-redesign`.

## Contents

| File | Purpose |
|---|---|
| `codex-screenshots/mobile/` | 4 Codex iOS screenshots — primary register reference |
| `codex-screenshots/desktop/` | 7 Codex macOS desktop screenshots — secondary reference |
| `current-*.png` | Pre-redesign baselines of the existing dashboard |
| `flow-map.md` | 3 user flows: pick-agent-reply / +New-spawn / kebab-nav |
| `register.md` | Declared `refined-minimal` dark register, sampled hex values, type scale, spacing, radii, motion, token mapping table |
| `rubric-weights.md` | Up-weighted `visual-register-match` and `content-density` (1.5); down-weighted `brand-voice-adherence` and `cross-locale-consistency` (0.5) |
| `verdict-baseline.md` | Floor — pre-redesign grader pass (REJECT, weakest 2.25) |
| `verdict-final.md` | Ceiling — post-redesign cross-pass (PASS, weakest 4.0) |
| `iter-c5-{list-mobile,list-desktop,sheet}.png` | Final list view + kebab sheet |
| `iter-d5-{detail-mobile,detail-desktop}.png`, `iter-d8-composer.png` | Final detail view + composer-with-text state |
| `iter-e3-{create-mobile,create-desktop}.png` | Final create view (Spawn disabled until folder set) |

## Methodology

Designed and graded with [`skills:uiux-design-loop`](https://github.com/bjornjee/skills/tree/main/skills/uiux-design-loop) — outer cold-context grader subagent + inner implementer loop. Pass threshold = weakest weighted score ≥ 4 across non-N/A dimensions.

For the next pass: start from `register.md`, dispatch the `uiux-grader` against the most recent `iter-*.png` you've shipped, and iterate against the critique brief.
