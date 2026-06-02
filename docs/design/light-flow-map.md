# Light flow map — agent-dashboard web UI under `[data-theme="light"]`

Companion to `docs/design/flow-map.md` (mobile dark) and `docs/design/desktop-flow-map.md` (desktop dark). This file describes the three target user flows — List → Detail → Reply, Create, Sidebar nav — **under light mode**, one per viewport class.

The grader subagent uses this file as the ground truth for "is the light-mode rendering correct?". Each section below maps a flow to the surface tokens declared in `register.md` "Light mode" section and `desktop-register.md` token table.

Form factor:
- **Mobile light:** 390×844, `[data-theme="light"]`, viewport < 900px
- **Desktop light:** 1280×800, `[data-theme="light"]`, 900px ≤ viewport < 1600px
- **Monitor light:** 1920×1080, `[data-theme="light"]`, viewport ≥ 1600px (sidebar 288px, main-pane content capped at 1080px)

Below 900px the mobile light flow is the source of truth. Where this file is silent on a given surface, the corresponding dark flow map applies with tokens swapped per `register.md` "Light mode".

---

## Mobile light (< 900px)

The mobile-light flows are 1:1 with mobile-dark from `flow-map.md`. Only the rendered colors change, not the structure or the visitor invariants.

### Flow A — List → Detail → Reply (mobile light)

| Step | Render shows (light deltas) |
|---|---|
| **A1** | Agent list on `var(--bg-base)` (`#FFFFFF`). Section headers (`BLOCKED`, `WAITING`, …) in `var(--text-muted)` (`#71717A`). Each row: agent name in `var(--text-primary)` (`#1C1A17`), meta in `var(--text-muted)`. No row background by default. Status dot uses the saturated status colors (`--status-*-text`) unchanged. |
| **A2** | Detail view. Top app bar: circular back button on `var(--bg-base)`, title in `var(--text-primary)`. Assistant prose left-aligned in `var(--text-secondary)` (`#3F3F46`). User message: dark pill on white page — `background: var(--cta-bg)` (`#1C1A17`), text in `var(--cta-text)` (`#FFFFFF`). Agent message: `var(--bg-surface)` (`#FFFFFF`) with 1px `var(--border-default)` (`#E4E4E7`). Sticky composer: `var(--bg-elevated)` (`#F2F2F4`) fill, `--text-primary` text. |
| **A3** | Send button activates with `background: var(--cta-bg)` (`#1C1A17`) and `color: var(--cta-text)` (`#FFFFFF`) — the inverse of dark mode's white-on-dark CTA. The send pulse is a 150ms ease-out opacity dip; no color change. |

**A1 visitor invariant (light):** `BLOCKED` still ranks first. The white background does not flatten the priority signal — section labels carry the same hierarchy as dark.

**A2 visitor invariant (light):** last assistant message in first viewport on load. Unchanged.

### Flow B — Create (mobile light)

| Step | Render shows (light deltas) |
|---|---|
| **B1** | Display title `What should we work on?` in `var(--text-primary)`. Composer: `var(--bg-elevated)` (`#F2F2F4`) fill, `--text-primary` cursor + text, placeholder in `var(--text-muted)`. Folder picker chip + harness chip: `var(--bg-elevated)` background, `var(--text-primary)` label, 1px `var(--border-default)` separator. |
| **B2** | Inline confirmation pill: `var(--bg-surface)` with 1px `var(--border-default)`, `--text-primary` text. Auto-redirect to A2. |

**B1 visitor invariant (light):** composer remains the visual centre. The white page does not make the chips compete — they sit at `--text-muted` prominence.

### Flow C — Sidebar nav (mobile light)

Mobile has no sidebar; this flow is the floating-dock + kebab pattern.

| Step | Render shows (light deltas) |
|---|---|
| **C1** | Floating dock anchored bottom-center, 24px from screen edge. `Search agents` pill: `var(--bg-surface)` (`#FFFFFF`) with 1px `var(--border-default)`, lifted by `box-shadow: 0 8px 32px var(--shadow-medium)` (`rgba(0,0,0,0.06)`). `+ New` CTA pill: `var(--cta-bg)` (`#1C1A17`) fill, `var(--cta-text)` (`#FFFFFF`) label — the inverse of dark mode's white CTA. Top-bar kebab opens a sheet with `Usage`, `Settings`, `Notifications`; sheet background `var(--bg-surface)`, 1px `var(--border-default)` per row. |

---

## Desktop light (900px ≤ viewport < 1600px)

The desktop-light flows are 1:1 with desktop-dark from `desktop-flow-map.md`. Light deltas only.

### Global desktop shell (light)

| Slot | Light rendering |
|---|---|
| Left rail | Sidebar 264px wide, `background: var(--bg-sidebar)` (`#F4F4F5`). Boundary between sidebar and main pane is the surface-color step `#F4F4F5` → `#FFFFFF`; no border-right is required to read the separation, but the shipped CSS includes a 1px `--border-subtle` hairline. |
| Main pane | `background: var(--bg-base)` (`#FFFFFF`). Vertical scroll inside this pane. |

Sidebar row states (light, derived from `register.md` "Light mode" section):

- **Default row:** no background, label in `var(--text-primary)`, icon in `var(--text-muted)`.
- **Hover row:** `background: var(--bg-hover)` (`#ECECEE`). Hover precedence rule applies: under the cursor, hover fill overrides selected fill.
- **Selected row (not hovered):** `background: var(--bg-selected)` (`#E4E4E7`). No leading bar, no border, no glow.
- **Selected + hovered:** `--bg-hover` wins under the cursor.
- **Keyboard focus:** 1px underline on the label in `var(--text-primary)` (`#1C1A17`). Geometry identical to dark; only the underline color resolves differently.

### View: `list` (desktop light)

Same structural mapping as desktop dark: the agent list is the sidebar's section-labels + agent-rows region; the main pane shows the selected agent's `detail` view or — on a cold load — the `create` view.

Light deltas: section labels (`BLOCKED`, `WAITING`, …) render in `var(--text-muted)` (`#71717A`) against `var(--bg-sidebar)` (`#F4F4F5`); the contrast ratio holds because the muted gray is dark enough against the light-gray panel.

### View: `detail` (desktop light)

Main-pane chrome:

- Top app bar: agent name in `var(--text-primary)`, machine sub-line in `var(--text-muted)`, spinner in `var(--text-secondary)`, kebab icon in `var(--text-muted)`.
- Tabs (Chat / Activity / Diff / Plan): horizontal segmented control. Active tab: `var(--text-primary)` label + 1px underline in `var(--text-primary)`. Inactive: `var(--text-muted)` label. No background fill on the segmented control.
- Transcript scroll: same chat-bubble inversion as mobile light — user message = dark pill (`--cta-bg` / `--cta-text`), agent message = white card with 1px `--border-default`.
- Tool-call / file-edit blocks: `background: var(--bg-elevated)` (`#F2F2F4`), 1px `var(--border-default)`, no shadow. Leading icon in `var(--text-muted)`.
- Sticky composer: `background: var(--bg-elevated)` (`#F2F2F4`), `--text-primary` text, send-circle uses `--cta-bg` / `--cta-text` when non-empty.

Visitor invariant: last assistant message in first viewport on load. Unchanged from dark.

### View: `create` (desktop light)

| Slot | Light rendering |
|---|---|
| Left rail | `+ New agent` row at the top is the selected row — `var(--bg-selected)` (`#E4E4E7`) fill. |
| Main pane | Display title `What should we work on?` in `var(--text-primary)`. Composer below: `var(--bg-elevated)` fill. Folder + harness chips below the composer: `var(--bg-elevated)` background, `var(--text-primary)` label. |

Visitor invariant: composer is the visual centre on a wide main pane. The white page does not let the chips compete — they sit one surface step below the composer.

### View: `usage` (desktop light)

Main-pane content uses the existing usage view sections, restyled to the new tokens. In light:

- Page edge padding + section spacing unchanged from dark.
- Cost tables: row alt background `var(--row-alt-bg)` (`rgba(0,0,0,0.02)`) — quoted from `style.css:1918`. Header row in `var(--text-muted)`.
- Charts: existing chart colors are theme-agnostic; only the chart background switches from `--bg-base` dark to `--bg-base` light.

---

## Monitor light (≥ 1600px)

The monitor-light flows are 1:1 with desktop light, with two layout adjustments derived from `--bp-monitor`:

1. **Sidebar widens to 288px** (vs. 264px on desktop). Agent-row truncation relaxes by ~24px of label width. Section labels, row internal padding, and row height are unchanged.
2. **Main-pane content max-width caps at 1080px**, centered. The motivation is line-length: at 1920px+ a full-width main pane produces 130+-character prose lines in the detail transcript. The cap applies to transcript + composer + create-view content; the page chrome (app bar, tabs) still spans the full main-pane width.

Light deltas vs. desktop light: none. All color tokens resolve identically; only the geometric tokens (`--sidebar-width`, the main-pane cap) change at `--bp-monitor`.

Visitor invariants (monitor light): same as desktop light, plus — at 1920px+ a centered 1080px content column should not feel "lost" in the main pane; the surrounding negative space is intentional and matches Codex's behavior on monitor widths.

---

## Page-state inventory (light, the grader will look at one screenshot per row)

| ID | Viewport class | View | State |
|---|---|---|---|
| A1-m-light | mobile (390×844) | list | Mixed state groups, 3+ agents, `[data-theme="light"]` |
| A2-m-light | mobile (390×844) | detail | RUNNING, mid-transcript, Chat tab, light |
| A2-m-light-tools | mobile (390×844) | detail | RUNNING, transcript with tool-call blocks, light |
| A2-m-light-composer | mobile (390×844) | detail | Composer focused, light |
| B1-m-light | mobile (390×844) | create | Empty composer, folder unset, light |
| C1-m-light-kebab | mobile (390×844) | kebab sheet | Open over A1-m-light |
| A1-d-light | desktop (1280×800) | list / detail | Sidebar with 3+ agents, main pane showing selected `detail`, light |
| A2-d-light | desktop (1280×800) | detail | Selected RUNNING agent, mid-transcript, light |
| B1-d-light | desktop (1280×800) | create | `+ New agent` selected, empty composer, light |
| U1-d-light | desktop (1280×800) | usage | `Usage` selected, usage content, light |
| A1-mon-light | monitor (1920×1080) | list / detail | Sidebar 288px, main-pane content capped at 1080px, light |
| A2-mon-light | monitor (1920×1080) | detail | Same data as A2-d-light at monitor width |

Twelve light-mode screenshots across three viewport classes. Dark coverage continues to come from `flow-map.md` + `desktop-flow-map.md` and is bit-exact preserved (see `light-preservation.md`).
