# Light monitor — iteration 1 verdict

**Viewport:** 1920×1080, `[data-theme="light"]`
**Iteration:** 1 (single inner-loop pass; iteration 0 surfaced one critical layout bug that was fixed inside this same iteration before screenshotting)
**Screenshots:** `iter-light-monitor-1-{list,detail,create,usage}.png`
**Flow map:** `docs/design/light-flow-map.md` "Monitor light" section
**Register:** `docs/design/register.md` light addendum + `docs/design/desktop-register.md`
**Preservation contract:** `docs/design/light-preservation.md`
**Weights:** `docs/design/rubric-weights.md`

## Per-dimension scoring

### 1. `user-flow-fidelity` — raw 5, weighted 5.0
List → Detail → Reply flow visible at monitor width. Sidebar shows BLOCKED/RUNNING groupings with section labels (per A1 invariant). At detail, the last assistant message would be the first one shown in the viewport (the seeded transcript is short enough that all three messages fit, but the order is preserved). Chat-bubble inversion present (dark user pill, white agent text). The Create flow is reachable from the sidebar's "New agent" row and renders `What should we work on?` at the top of the centred composer (B1 visitor invariant: composer is the visual centre). Usage view: rate limits, token usage, cost, daily chart all render correctly in the capped column.

### 2. `visual-register-match` — raw 5, weighted 7.5
Sidebar bg `#F4F4F5`, page bg `#FFFFFF`, text-primary on dark user pill is white on `#1C1A17`, agent message sits in a white card with 1px `#E4E4E7` border. Composer card carries the Phase D laptop polish (shadow-medium 0.06, border-default) — that polish carries through unchanged at monitor. The "New agent" row is rendered as the selected pill with the 2px left-accent in `--text-primary`, matching the Phase D register exactly. No new register additions at this tier; only the geometric cap shifts (sidebar 264 → 288, content cap 1080).

### 3. `content-density` — raw 5, weighted 7.5
At 1920×1080 the reading column caps at 1080px centred, leaving ~270px of negative space on each side. That negative space is intentional per the light-flow-map ("at 1920px+ a centered 1080px content column should not feel 'lost' in the main pane; the surrounding negative space is intentional and matches Codex's behavior on monitor widths"). The sidebar at 288px is comfortably proportioned for the agent-name + branch lines visible in the screenshots. Long agent names (`feat-refactor-conversation-router-and-history-with-additional-context`) ellipse cleanly; no orphaning, no wrap. The composer card sits at ~85% of the capped column (914px inside the 1080 cap), which keeps it visibly narrower than the transcript text — same proportion the laptop register established.

### 4. `affordance-honesty` — raw 4, weighted 4.0
Selected "New agent" pill: visible left-accent + softer fill. Hover would resolve to `--bg-hover` (not tested in the screenshot but defined by the same cascade as laptop). Send pill in the composer: dark filled circle in the bottom-right of the rail when input has content — affordance reads correctly as the primary submit. Detail-tabs row: active tab carries the 1px underline; inactive tabs are muted. Spawn pill in Create: outlined disabled state until input is non-empty (Phase C contract). One small but defensible compromise: the chrome (app bar + tabs) is full-pane width while the transcript is capped at 1080 → the active tab's underline doesn't line up exactly under the first message's column. The light-flow-map explicitly accepts this (the cap applies to transcript + composer; chrome spans the pane). Not down-grading further since the contract calls for it.

### 5. `brand-voice-adherence` — N/A
No declared brand voice in the dashboard; per `rubric-weights.md` this dimension is `N/A` for this PR and does not contribute to the gate.

### 6. `cross-locale-consistency` — N/A
Dashboard is EN-only; not applicable per `rubric-weights.md`.

### 7. `polish` — raw 4, weighted 4.0
No visible alignment errors, no orphaned glyphs, no overflow. Section labels (WAITING / RUNNING / BLOCKED) sit at consistent vertical rhythm in the sidebar. Composer card chip-rail items (main / opus / medium) align cleanly with the spacer + send pill on the right. Charts in the usage view render with the dark monitor charts colour (the bar fills carry the dark-register `#2D2D31` tone in light) — this is theme-agnostic chart code per the light-flow-map and is allowed. Could be tighter: the per-agent breakdown table's right-edge `COST` column ends well before the 1080 cap because the table is auto-sized to its content; that's a downstream usage-view concern, not a monitor-light concern.

## Weighted minimum

```
visual-register-match (1.5 × 5) = 7.5
content-density       (1.5 × 5) = 7.5
user-flow-fidelity    (1.0 × 5) = 5.0
affordance-honesty    (1.0 × 4) = 4.0
polish                (1.0 × 4) = 4.0
brand-voice           N/A
cross-locale          N/A
```

Weakest weighted score: **4.0** (affordance-honesty, polish). Pass threshold per `rubric-weights.md`: weakest weighted ≥ 4.0.

## Verdict: **PASS**

No broken dimensions. No critical issues. All five scoring dimensions hit ≥ 4 weighted. Iteration 1 closes the only gap the in-loop measurement surfaced (the flex-auto-margin collapse on `.detail-scroll` / `.action-bar`, fixed by switching to `align-self: center; width: 100%; max-width: …`).

## Follow-ups (out of scope for Phase E, surface as separate work)

- **Usage view per-agent breakdown** at monitor width has a long-tail right margin inside the 1080 cap because the table is auto-sized. Could be addressed by giving the table a `width: 100%` rule, but the usage view itself is owned by `js/pages/usage.js` and that's a Phase F+ surface decision.
- **Sidebar long-name ellipsis** is single-line-only — at 288px a name like `feat-refactor-conversation-router-…` truncates at ~30 chars. If we ever want two-line wrapping for the title and ellipsis on the subtitle, that's a `.ui-row__title` primitive change, not a monitor-light concern.
- **Chrome misalignment** with the capped column (tabs underline doesn't line up with first message column) could be tightened by also centring `.detail-tabs` separately inside `.detail-pinned`, but the light-flow-map explicitly rules that out — "the page chrome (app bar, tabs) still spans the full main-pane width." Doing so would violate the contract.
