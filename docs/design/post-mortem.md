# Post-mortem — functional regressions in the Codex-iOS UX redesign

The grader-driven loop landed PASS on every phase, every cross-pass, every dimension ≥ 4 weighted. The actual app shipped to the user broke: Usage view crashed, Settings + Notifications buttons did nothing, Activity / Diff / Plan tabs rendered as raw skeleton placeholders, subagent rows ran fields together, the composer wasn't pinned, the chat opened scrolled to the top instead of the tail.

**Verdicts said PASS. The user said "so many broken windows."** That gap is the lesson. Below: where each regression hid, why the methodology missed it, and what to change in the `skills:uiux-design-loop` so the next pass catches it autonomously.

## What broke

| Regression | Root cause | When introduced |
|---|---|---|
| Usage view: blank/crash | `usage.js` calls `UI.header`, `UI.btn`, `UI.metricsStrip`, `UI.chartContainer`, `UI.chartBar`, `UI.dateRangeSelector`, `UI.tableCard`, `UI.loadingBlock` — all deleted from `ui.js` when the primitive set was reduced to 9. | Phase B. The new `ui.js` was sized to the in-scope views (list/detail/create) only. `usage.js` was preserved as "out of scope" but its dependencies weren't preserved. |
| Settings sheet item: silent dead-end | `Dashboard.openSettings` called `showModal(...)`, which called `UI.btn` for Cancel/Confirm — also deleted. | Phase C. Patched mid-session when discovered. |
| Notifications: works but visibly silent | `Dashboard.toggleNotifications` works, but the kebab sheet immediately dismissed itself; toast styling preserved but no visible confirmation when permission is `denied`. | Phase C. Not surfaced by any grader pass. |
| Activity / Diff / Plan tabs: skeleton stuck | Legacy CSS for `.timeline-entry`, `.timeline-header`, `.timeline-icon`, `.activity-filter-bar`, `.activity-filter-btn`, `.plan-content`, `.conversation`, `.msg-time`, `.tool-group`, `.tool-call` was dropped when `style.css` was rewritten. The JS still emits those classes; without CSS, the content renders as undifferentiated text. | Phase B. Bulk-rewrote `style.css`, only preserved usage + diff + modal + spinner blocks. |
| Subagent rows: fields run together | Legacy CSS for `.subagent-summary-list`, `.subagent-pill`, `.subagent-type`, `.subagent-desc`, `.subagent-mode`, `.subagent-time` dropped. | Phase B. Same root cause as above. |
| Composer not pinned at the bottom | `.detail-layout` used `min-height: 100vh` instead of `height: 100dvh`, which let content stretch the layout taller than the viewport; `position: sticky` on `.action-bar` then attached to a non-scrolling parent and effectively did nothing. | Phase D. Fixed mid-session. |
| Chat opens at top, not tail | `loadTabContent('conversation', ...)` set `scrollParent.scrollTop = scrollParent.scrollHeight` correctly, but `scrollParent` resolved to a non-scrolling element because of the layout bug above. The auto-scroll silently no-op'd. | Phase D. Same root cause as composer pinning. |
| Multi-line user pill renders as a giant capsule | `border-radius: var(--radius-lg)` (20px) on multi-line content produces an oval, not a rectangle — looks like a stretched egg. | Phase D. The grader saw it but interpreted my register-table claim ("multi-line wraps as rounded-rect") as a definitional shield. |
| Style.css comment unclosed → 700+ lines silently ignored | While preserving the legacy activity-timeline CSS block, I appended `/* === Activity Timeline` with no closing `*/`. The CSS parser treated everything from line 937 onward as one comment until the next `*/` deep in the file. All `.timeline-*` / `.diff-*` / `.plan-content` / `.usage-*` rules were silently dropped by the browser. | Phase D fixup. Caught only by running the actual page and inspecting computed styles. |

## Why planning missed these

**The plan said "Usage view: Out — keeps rendering via preserved token *names*."** That was wrong — what `usage.js` actually depended on was the *primitive functions* in `ui.js`, not just token names. The plan conflated two preservation contracts (CSS tokens vs JS API) into one sentence and never enumerated which `UI.*` methods the out-of-scope views needed.

Planning failure modes that produced this:
- **"Out of scope" was treated as "won't change," not as "must keep working."** A real out-of-scope contract has a *compatibility surface* (every method that out-of-scope code calls). I never listed it.
- **The plan optimized for the new-code shape (9 primitives), not the preservation contract.** Constraint asymmetry: the new code was scoped tightly; the preservation was scoped loosely ("token names match").
- **No tab-by-tab inventory of detail.js callers.** Detail has 4 tabs and each tab calls different `UI.*` helpers. I knew that intellectually but didn't enumerate them when sizing the new primitive set.

## Why implementation missed them

- **I bulk-rewrote `style.css` to ~700 lines without re-running each view against it.** I only ran the *new* views (style tile, list, detail-chat, create). Usage view was never opened in the browser between Phase B (when I dropped its CSS) and the user reporting it.
- **I sliced legacy CSS blocks by line ranges with `sed -n 'A,Bp'`.** The ranges crossed comment boundaries, leading to the unclosed-comment bug. CSS parses comments greedy; the silent failure mode is the entire downstream rule set being treated as a comment and ignored.
- **I trusted "the rule is in the file" instead of "the rule is being applied."** Multiple grader passes returned PASS because the LAYOUT screenshots looked right — but `getComputedStyle()` would have shown `display: block` where `display: flex` was declared. The grader has no `getComputedStyle` channel.
- **Optimistic state ("preserved verbatim from legacy") didn't survive contact with the slicing tool.** I should have tested the slice boundaries before declaring them clean.

## Why the audit (grader) missed them

- **The grader scores screenshots, not behavior.** No interaction states, no tab switches, no kebab-sheet → Settings → modal flow. The static pixel of the Settings sheet looks correct; the broken click is invisible.
- **The grader scored only in-scope views.** Phase B's style-tile, Phase C's list+sheet, Phase D's detail-chat, Phase E's create. Usage was explicitly excluded — but Usage *also exists in the app*. The user found it because they tapped the kebab. The grader never tapped anything.
- **The grader was told to ignore plan/implementation files.** Cold-context is the right discipline for register-match scoring, but it means the grader has no visibility into the preservation contract claimed by the plan. It can't notice when a "preserved" view is in fact broken.
- **Multi-iteration PASS is a local maximum.** The grader passed each view alone. The cross-view pass scored the three in-scope views together. But the *out-of-scope-but-still-reachable* surfaces (Usage, Settings sheet item, every tab switch in detail) were never in any screenshot the grader saw.

## How to improve `skills:uiux-design-loop` so this doesn't happen again

Three concrete additions, in priority order:

### 1. Preservation contract is a first-class artifact

Add `preservation-contract.md` to Gate 0, alongside `flow-map.md` and `register.md`. Format:

```markdown
# Preservation contract

Surfaces NOT in the redesign scope but still reachable in the running app.
The new code MUST keep these working without changes to their internals.

## Surface: Usage view
- Reached via: kebab sheet → Usage
- Calls these primitives from `ui.js`: UI.header, UI.btn, UI.metricsStrip,
  UI.chartContainer, UI.chartBar, UI.dateRangeSelector, UI.tableCard, UI.loadingBlock
- Calls these CSS classes from `style.css`: .usage-view, .usage-metrics, .rate-limit-*,
  .usage-chart-*, .chart-tooltip, .date-range-selector
- Verification: open Usage, screenshot, confirm no console errors and the chart bars render

## Surface: Settings modal (from kebab sheet)
- Reached via: kebab sheet → Settings
- Calls: showModal(title, message, onConfirm, opts) → .modal-overlay, .modal-actions,
  .ui-modal-btn (or its replacement)
- Verification: tap kebab, tap Settings, confirm modal opens with two buttons that both work
```

Gate 0 fails until this file lists every surface the redesign doesn't touch but the app still routes to. Implementer can't claim "out of scope" without writing the compatibility surface.

### 2. Behavior-test the reachable surface, not just screenshot it

Add a `behavior-check.md` to Gate 4 (exit). Format:

```markdown
# Behavior check (Gate 4)

For each entry in `preservation-contract.md`, run the surface in the live app and
confirm. Format: one row per surface, with pass/fail evidence.

| Surface | Steps | Pass? | Evidence |
|---|---|---|---|
| Usage view | Open kebab, tap Usage | ✅ | charts render, no console errors |
| Settings modal | Open kebab, tap Settings, tap Cycle theme, tap Cancel | ✅ | theme cycles, modal dismisses |
| Detail Activity tab | Open agent, click Activity | ✅ | timeline entries render with separated columns |
| Detail Diff tab | Open agent, click Diff | ✅ | diff renders or empty-state shown |
| Detail Plan tab | Open agent, click Plan | ✅ | markdown plan renders or empty-state shown |
```

This is the explicit `verify` skill chained after `uiux-design-loop` — and **the skill should refuse to exit Gate 4 if the contract isn't fully passed**.

### 3. CSS slicing needs a structural pass

The unclosed-comment failure mode is generic to any text-slice approach. Add a small validator to the skill's inner-loop (Gate 2) that runs after any `style.css` edit:

```bash
# Pseudo-code; run before grader sees a screenshot
awk 'BEGIN{o=0} /\/\*/{o++} /\*\//{o--} END{exit o!=0}' style.css \
  || fail "Unbalanced CSS comments — slicing left dangling /* or */"

awk 'BEGIN{n=0} /\{/{n++} /\}/{n--} END{exit n!=0}' style.css \
  || fail "Unbalanced CSS braces"
```

Generalized: every Gate 2 commit (the inner loop's smallest change) runs a one-shot structural-integrity check on the *artifacts the design depends on* (CSS for style work, HTML for markup work, JS modules for behavior). If structural integrity fails, the inner loop reverts the change and tries smaller. This prevents the grader from ever seeing a render that's broken because of a parser fault.

### 4. The grader needs a "live URL" mode, not just a "screenshot" mode

Right now the grader scores PNG files. For the next iteration of the skill: add an *optional* mode where the grader subagent receives a Playwright session handle and can `click`, `evaluate(computedStyle)`, `console_messages`, in addition to `screenshot`. Same rubric, additional channels. The grader stays cold-context (only sees flow-map + register + the live URL), but it can verify *behavior* not just *appearance*.

That's what would have caught the dead-click Settings button: the grader would have tried `click("Settings")`, observed no modal appearing, and scored `affordance-honesty: 1` instead of `4`.

### 5. Out-of-scope still demands "smoke pass" in the grader

Even without live-URL mode, the cross-pass grader should be passed `out-of-scope-screenshots/` of every surface in `preservation-contract.md`. If those surfaces appear broken (skeleton stuck, fields run together, blank page), the cross-pass fails. The rule:

> Cross-pass weakest weighted ≥ 4 across BOTH in-scope and preservation-contract surfaces.

Preservation surfaces don't need to score 5 — they need to score "not visibly regressed." A separate rubric anchor for that, with an anchor at "regressed" = 1 and "no change" = 4.

## What I'd change about my own workflow regardless of the skill

Independent of the skill design, my own loop:

- **Run the full app, click every reachable destination, before any grader call.** That single discipline catches Usage-crash and Settings-dead-click before either gets scored.
- **Trust `getComputedStyle`, not `grep style.css`.** Several hours of this session were spent debugging "the rule is in the file but not applied." The screenshot tells you nothing; the computed style tells you everything.
- **CSS slicing should never use line ranges across comments.** Use AST-aware tooling (PostCSS) or just write the file fresh.
- **A grader PASS at iteration N tells you the screenshot is OK, not that the app works.** The skill's PASS bar is a necessary condition, not sufficient.
