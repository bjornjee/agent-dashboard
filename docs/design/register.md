# Visual register — Codex iOS mobile, dark theme

## Declared register

**`refined-minimal` — dark, layered, generous.** Not "spacious" (Codex carries plenty of content per screen), not "editorial" (no decorative type tricks), not "brutalist" (no aggressive contrast). The register is **system-grade restraint**: pure-grayscale surfaces with millimetric whitespace, heavy display type used sparingly, and one visual accent (a near-white CTA pill) reserved for the single most important action on each screen.

What this register **rejects**:
- Saturated brand accents (the existing dashboard's bright `--accent-green` for cost values, the blue user-message bubbles)
- Stroke borders for separation (Codex separates by elevation + spacing, not lines)
- Mixed corner radii (everything is either heavily rounded or fully pilled — nothing 6px-or-less)
- Visible card outlines / `box-shadow: 0 1px 2px rgba(0,0,0,0.2)` chrome
- Drop shadows on text
- Icon buttons rendered as rectangles (Codex uses circles for icon-only actions)

## Reference screenshots used to derive this register

Source: `docs/design/codex-screenshots/mobile/` (4 photos, Codex iOS, dark theme).

- `photo_2026-06-01_17-44-47.jpg` — AskUserQuestion card during a running task
- `photo_2026-06-01_17-44-50.jpg` — active chat with assistant prose, user pill bubble, collapsed-messages indicator, plan card, action chips
- `photo_2026-06-01_17-44-52.jpg` — plan card expanded as a modal-style page
- `photo_2026-06-01_17-44-54.jpg` — Codex home: machine row, Projects list, Recents list, bottom dock with Search + Chat CTA

Codex desktop screenshots in `docs/design/codex-screenshots/desktop/` are secondary reference only — used to confirm tool-call rendering patterns where the mobile evidence is ambiguous.

## Color — derived from screenshot pixels

Sampled from the four mobile reference photos. JPEG compression introduces ±2 on each channel; values rounded to the nearest stable hue.

| Role | Hex | Sample location |
|---|---|---|
| Page background | `#0E0E10` | photo_47 background, photo_54 background |
| Elevated surface (cards) | `#1A1A1C` | photo_47 question card, photo_50 plan card, photo_52 modal |
| Secondary surface (inline inputs, pill chrome) | `#222226` | photo_47 "Type a response" fill, photo_50 user-message pill |
| Hairline divider | `#2A2A2E` | photo_47 between radio options (very faint) |
| Text primary | `#FFFFFF` | display titles in photo_47 / photo_52 |
| Text secondary (body) | `#E6E6E8` | body prose in photo_50 |
| Text muted (labels, meta) | `#8E8E93` | "Deliverable" / "External" small caps labels in photo_47 |
| Text faint (timestamps, "17 previous messages") | `#5C5C61` | photo_50 collapsed indicator |
| Code background | `#1F1F23` | inline `code` in photo_50 (`/plan`, `<proposed_plan>`) |
| Code text | `#C5C7CC` | same |
| Status dot — running | `#3CCB6B` | photo_54 green dot beside "MacBook Jeeb" |
| Primary CTA fill (single per screen) | `#FFFFFF` | photo_54 "Chat" pill |
| Primary CTA text | `#0E0E10` | same |
| Focus / interactive hover (web-only addition) | `#2F2F33` | inferred — Codex iOS doesn't expose hover |

No accent green for cost values. No blue for user bubbles. The redesign uses **one** color (white) for emphasis, on the single primary action per screen.

## Typography

Codex iOS uses the system stack — Apple SF Pro on iOS. The web equivalent that holds the visual register on macOS/iOS browsers and degrades cleanly elsewhere:

```
--font-primary: -apple-system, BlinkMacSystemFont, "SF Pro Text", "Inter", system-ui, sans-serif;
--font-display: -apple-system, BlinkMacSystemFont, "SF Pro Display", "Inter", system-ui, sans-serif;
--font-mono: ui-monospace, "SF Mono", "JetBrains Mono", "Fira Code", monospace;
```

Note: the existing CSS imports Inter from Google Fonts. We **drop that import** — Codex's visual register depends on the system stack carrying SF Pro's optical sizing on Apple devices. Inter as the fallback is fine.

### Type scale (mobile, 390px viewport baseline)

| Token | Size / line-height / weight | Sample |
|---|---|---|
| `--font-display` | 36 / 40 / 700 | plan modal title "SEO And Agent Readiness Optimisation" |
| `--font-h1` | 22 / 28 / 700 | "Codex needs input" / "Codex" page title (chip-style) |
| `--font-h2` | 17 / 22 / 600 | "Summary", radio option titles like "Implement fixes (Recommended)" |
| `--font-body` | 16 / 24 / 400 | assistant prose, question text |
| `--font-body-meta` | 15 / 20 / 400 | "Yes, implement this plan" action labels |
| `--font-label` | 12 / 16 / 500, letter-spacing 0.04em, color muted | "Deliverable" / "External" / "Projects" / "Recents" |
| `--font-meta` | 13 / 16 / 400 | "17m" / "1d" timestamps on chat rows |
| `--font-mono-inline` | 14 / 20 / 500 | inline `code` like `/plan` |

Display weight (700) is used **sparingly** — once per page on home (the page-title chip is medium-weight, not display), once per plan modal. Multiple display titles per screen = register drift toward editorial.

## Spacing

Codex iOS density derived from screenshots (verified at the home and detail screens):

- **Page edges:** 20px horizontal padding on the page container
- **Section gap:** 32px between major sections (e.g., between Projects list and Recents list)
- **Card internal padding:** 20px
- **List row vertical padding:** 14px top + 14px bottom (60px row height with `--font-body`)
- **App-bar height:** 56px above the safe area
- **Floating dock bottom margin:** 24px from screen bottom edge

Tokens (re-using the existing names, new values):

```
--space-1: 4px
--space-2: 8px
--space-3: 12px
--space-4: 16px
--space-5: 20px
--space-6: 24px
--space-7: 32px
--space-8: 40px
```

Same scale as before — the values don't need to change, the *usage* does (existing CSS uses `--space-3` between cards; new usage is `--space-5` for the same separation).

## Corners

Heavy rounding throughout. Codex iOS rounding is binary: "fully pilled" or "rounded card".

| Token | Value | Use |
|---|---|---|
| `--radius-sm` | 8px | inline `code` blocks, small chips |
| `--radius-md` | 16px | text inputs, secondary buttons |
| `--radius-lg` | 20px | cards (question card, plan card) |
| `--radius-xl` | 24px | the plan-card-as-modal corners |
| `--radius-pill` | 999px | floating dock pills, user message bubble, primary CTAs, radio circles |

No 6px or 12px — anything that needs separation gets 8px+; anything that needs prominence gets 16px+.

## Motion

- Page transitions: 200ms ease-out fade for view swaps. No slide.
- Composer-focus expand: 150ms ease-out.
- Spinner: continuous rotation, 1s linear (matches Codex iOS app-bar spinner).
- Status-dot pulse: 2s ease-in-out (running agents only).

No staggered list-row animations. No hero parallax. Subtle, deliberate, never decorative.

## Surface treatment

**Layered, not bordered.** Card distinction comes from `background-color` step (page → card → input) and `padding`, never from `border` or `box-shadow`. The one exception: the floating bottom dock uses a subtle `box-shadow: 0 8px 32px rgba(0,0,0,0.4)` to lift it visibly above content.

**No glassmorphism.** Codex iOS does not use `backdrop-filter: blur`. The top app bar is opaque `#0E0E10`.

## Token mapping table — old → new

The existing CSS uses well-named tokens (per the redesign plan's KISS rule, keep the *names*). Only the *values* change. Re-using token names means `usage.js` and the carried-over `[data-theme="light"]` block keep compiling.

| Token | Old value (current dark) | New value | Notes |
|---|---|---|---|
| `--bg-base` | `#0B0E14` | `#0E0E10` | Slightly warmer, less blue |
| `--bg-surface` | `#141821` | `#1A1A1C` | Pure neutral, no blue cast |
| `--bg-elevated` | `#1C2130` | `#222226` | Used for inputs + user pills |
| `--bg-code` | (varies) | `#1F1F23` | New explicit token |
| `--text-primary` | `#F9FAFB` | `#FFFFFF` | Pure white for display |
| `--text-secondary` | `#D1D5DB` | `#E6E6E8` | Slightly cooler than old; matches Codex body prose |
| `--text-muted` | `#9CA3AF` | `#8E8E93` | iOS system gray |
| `--text-faint` | (new) | `#5C5C61` | For collapsed indicators, timestamps |
| `--accent-green` | `#10B981` | `#3CCB6B` | Only used for running status dot |
| `--accent-blue` | `#3B82F6` | **REMOVED** | Was user-bubble bg; new design has no bubble |
| `--accent-amber` | `#F59E0B` | **DEMOTE to `--state-waiting`** | Used only by state badges, not as a general accent |
| `--accent-red` | `#EF4444` | **DEMOTE to `--state-error`** | Same |
| `--accent-indigo` | `#818CF8` | **REMOVED** | Unused once the chat-bubble redesign lands |
| `--cta-bg` | (new) | `#FFFFFF` | The one prominent fill, used on `+ New` |
| `--cta-text` | (new) | `#0E0E10` | |
| `--space-1..8` | `4..40px` | `4..40px` | Unchanged |
| `--radius-sm` | `6px` | `8px` | Bump up |
| `--radius-md` | `8px` | `16px` | Bump up |
| `--radius-lg` | `12px` | `20px` | Bump up |
| `--radius-xl` | (new) | `24px` | Plan-card-as-modal |
| `--radius-pill` | (new) | `999px` | |
| `--transition-fast` | `100ms ease-out` | `150ms ease-out` | Codex feels marginally slower than 100ms |
| `--transition-default` | `200ms ease-out` | `200ms ease-out` | Unchanged |
| `--font-primary` | `'Inter', ...` | `-apple-system, BlinkMacSystemFont, 'SF Pro Text', 'Inter', system-ui, sans-serif` | Drop Google Fonts import |

## Component-pattern derivations (from screenshots)

### Top app bar (Codex iOS pattern)

A circular back button (40px) left, a centered title or title-chip (pill background + truncated text + sub-line for context), an optional spinner button (40px circle, ring-only when running), and a circular kebab (40px). All four elements live on the page background, not in a bar with its own fill. Vertical padding = 12px top, 8px bottom; content starts at 56px from the top of the safe area.

### List row (Codex Recents pattern, screenshot 4)

`leading-icon (24px square)` `gap-16` `body` `trailing-meta (right-aligned, muted)`. Row height = 56–60px. No hover background on mobile; on desktop, hover = `#2F2F33`. Tap state = 80ms opacity-50 flash.

### Card (Codex question card / plan card pattern, screenshots 1 + 2 + 3)

`background: var(--bg-surface)`, `border-radius: 20px`, `padding: 20px`. No border, no shadow. Inside: optional small-caps label, primary content, optional separator hairline between sections (1px high, `var(--bg-elevated)`), optional inline input rendered with `border-radius: 16px` and a darker fill.

### User message (Codex chat pattern, screenshot 2)

`background: var(--bg-elevated)`, `border-radius: 999px`, `padding: 8px 16px`, `align-self: flex-end`, max-width ~70% of column. Text-only — single line preferred; multi-line is allowed but breaks the pill into a rounded-rectangle.

### Assistant message

No background. No border. Prose flows left-aligned. Below the last line: a single 16px copy icon (muted), aligned to the start of the prose. Inline `code` uses `var(--bg-code)` with 8px radius and `var(--font-mono-inline)`.

### Tool-call / file-edit block

Subtle dark card with leading icon (per the user's interview answer to "keep a subtle card around tool calls / file edits"). Pattern from Codex desktop reference: small `Approved request, ran N commands` footer below assistant prose. Mobile equivalent: a thin row with leading checkmark/spark icon + "Ran `tool_name`" + chevron disclosure. Background `var(--bg-surface)`, padding `12px 16px`, `border-radius: 12px`, no border.

### Composer (Codex iOS detail-view pattern)

Sticky bottom bar above the bottom dock. Full-width rounded textarea (`border-radius: 24px`, `background: var(--bg-elevated)`, `padding: 14px 16px`). Right-aligned send button = 32px circle in `var(--cta-bg)` when input is non-empty, `var(--bg-surface)` when empty. Left-aligned "+" attach button = 32px circle, muted, decorative-only for v1 (not wired).

### Floating dock (Codex home pattern, screenshot 4)

Anchored bottom-center, 24px from screen edge. Two pills side by side: left = `Search agents` (full-pill, `var(--bg-surface)` fill, leading magnifying-glass icon, ~180px wide); right = `+ New` (full-pill, `var(--cta-bg)` fill, leading pencil/sparkle icon, ~96px wide). The pair lifts with a subtle drop shadow to register above content.

## What's intentionally not in this register

- ~~Light theme tokens (deferred to follow-up PR; carry over old `[data-theme="light"]` block verbatim)~~ Light theme is now a first-class section of this register — see "Light mode" below.
- Dense data-viz styles (Usage view is out of scope; its existing tokens still resolve via the re-used names)
- Hover/focus states for keyboard navigation (will be added in Phase B styling pass — register declares them as `--text-primary` underline + `#2F2F33` background, no glow rings)
- Animation easings beyond `ease-out` (the register rejects bounce / elastic / spring)

## Light mode

Light mode is a peer of dark mode in this register, not an afterthought. Dark values declared above continue to apply when `[data-theme="dark"]` (or no `data-theme`) is set; light values below apply when `[data-theme="light"]` is set. The token *names* are identical across themes — only the *values* swap. All values in this section quote the shipped `[data-theme="light"]` block in `internal/web/static/style.css:1858-1920` verbatim; this section documents the rules that block embodies, it does not introduce new values.

### Surfaces

| Token | Light value | Use |
|---|---|---|
| `--bg-base` | `#FFFFFF` | Page background (main pane). Pure white. |
| `--bg-surface` | `#FFFFFF` | Card surface. Same as `--bg-base`; light mode separates cards from page via 1px `--border-default`, not via a surface-color step. |
| `--bg-elevated` | `#F2F2F4` | Inline inputs, secondary pill chrome, agent-message card fill. Soft neutral gray. |
| `--bg-code` | `#F4F4F5` | Inline `code` background. One step warmer than `--bg-elevated`. |
| `--bg-sidebar` | `#F4F4F5` | Sidebar background. Distinct from the white main pane; the boundary reads as a panel separation. |
| `--bg-selected` | `#E4E4E7` | Selected sidebar row background. Soft neutral gray, never tinted. |
| `--bg-hover` | `#ECECEE` | Hover background for interactive rows on desktop. One step lighter than `--bg-selected`. |

The dark register's "layered, not bordered" rule (line 127) is **inverted in light**: white-on-white surfaces cannot separate by elevation step, so light mode falls back to a 1px `--border-default` (`#E4E4E7`) around cards. The dark register's prohibition of card outlines applies to dark only.

### Elevation rules (light)

- **No warm shadows.** The dark register has no shadows at all on cards; light follows the same rule for cards but allows a single subtle shadow on lifted surfaces (the floating dock, in mobile light).
- `--shadow-medium: rgba(0, 0, 0, 0.06)` — the only shadow token in use in light. Quoted directly from `style.css:1919` (`#F2F2F4` resolves to roughly 6% black opacity). Consumed by the floating dock and any other lifted pill surface.
- **No `--shadow-large` declared for light in this phase.** The dark register's `box-shadow: 0 8px 32px rgba(0,0,0,0.4)` on the dock has no light analog beyond the medium shadow above. If a heavier lift is needed later, it will be added as a new token, not by darkening the existing one.

### Hover precedence (light, applies to dark too)

When a row is both **hovered** and **selected**, the **`--bg-hover` fill takes precedence under the cursor**: the lifted "the cursor is here" state outranks the settled "this row is the current selection" state. Rationale: a selected row that the user is hovering is, by definition, a row the user is about to interact with — the hover signal is the more informative one. This matches the dark register's intent (sidebar hover = `#2F2F33`, selected = `#222226`; hover is the brighter fill) and is asserted here as an explicit, theme-agnostic rule.

### Chat-bubble inversion

The dark register declares the user message as a `--bg-elevated` pill against a dark page (line 178). Light mode **inverts the contrast pair**, not the geometry:

- **User message (light):** `background: var(--cta-bg)` (`#1C1A17`, near-black), `color: var(--cta-text)` (`#FFFFFF`), same `border-radius: 999px` and `padding: 8px 16px` as dark. The bubble is the *dark* element in a light page; the inversion preserves user-message prominence.
- **Agent message (light):** `background: var(--bg-surface)` (`#FFFFFF`), `color: var(--text-primary)` (`#1C1A17`), `border: 1px solid var(--border-default)` (`#E4E4E7`). The agent message is a white card on a white page — separated by the 1px hairline, not by a surface-color step.

Dark mode is unchanged: user pill = `--bg-elevated` (dark gray on dark page), agent message = no background (prose on dark page).

### Text colors (light)

| Token | Light value | Use |
|---|---|---|
| `--text-primary` | `#1C1A17` | Display + headings + body emphasis. Pure near-black; matches `--cta-bg` for token symmetry. |
| `--text-secondary` | `#3F3F46` | Body prose. |
| `--text-muted` | `#71717A` | Labels, meta, timestamps with low prominence. |
| `--text-faint` | `#A1A1AA` | Collapsed indicators, the lowest-emphasis text class. |

### Focus underline carries forward verbatim

The desktop register's focus rule (`desktop-register.md` "Focus styling" section) — 1px underline in `var(--text-primary)`, no outline, no glow ring — applies **bit-exactly** in light. The only thing that changes is the resolved color: `--text-primary` is `#FFFFFF` in dark, `#1C1A17` in light. The underline geometry, the hover-precedence rule above, and the "no outline rectangle" prohibition are all theme-agnostic.

### What the light section intentionally does not declare

- A separate set of shadow tokens beyond `--shadow-medium`. If a future component needs more elevation steps in light, add a token; do not reuse `--shadow-medium` at a higher opacity.
- Theme-toggle UX. The toggle mechanism is owned by the existing code path; this register declares the values, not the switch.
- Light variants for `--status-*-bg/border/text`. Those are already declared in the shipped CSS block (`style.css:1893-1910`) and follow the same "saturated-on-tinted" pattern as dark; they do not need to be re-promoted here.
