# Desktop flow map — agent-dashboard web UI at ≥ 1024px

Companion to `docs/design/flow-map.md` (which is mobile-first). This file describes the **desktop layout** the redesign targets at viewport widths ≥ 1024px. Below 1024px the mobile flow map is the source of truth and the rendering is unchanged.

The grader subagent uses this file as the ground truth for "is the desktop layout correct?". Each view below maps a dashboard view to a Codex desktop reference screenshot and declares left-rail content + main-pane content separately.

Form factor: desktop at 1280×800 = persistent left sidebar + flexible main pane. The mobile "single-column with floating dock" pattern is replaced wholesale at this breakpoint.

## Global desktop shell (applies to all views)

Two-pane app shell at the top level. There is no top-bar nav and no floating dock on desktop — those mobile primitives are replaced by the sidebar.

| Slot | Content |
|---|---|
| Left rail | Persistent sidebar (264px wide, `--sidebar-width`). Background `var(--bg-sidebar)`. Anchored top-to-bottom of viewport. Never collapses on desktop (no hamburger). |
| Main pane | Flexes to fill remaining width. Background `var(--bg-base)`. Vertical scroll lives inside this pane, not on the page. |

Sidebar internal structure (top-to-bottom), mirroring Codex's sidebar in `codex-dark-03-reactivated.png`, `codex-dark-04-new-chat-create-agent.png`, and `codex-light-03-chat-navigation-live-status.png`:

1. **Top CTA region** — `+ New agent` row (pencil/sparkle leading icon, body label). Matches Codex's `New chat` row at the top of its sidebar. This is the *only* place a `+ New` affordance lives on desktop — the floating dock is gone.
2. **Search row** — `Search agents` (magnifying-glass leading icon, body label). Matches Codex's `Search` row. v1 may render as a non-functional placeholder; the slot must still be declared.
3. **Section labels + agent rows** — small-caps muted labels (`--font-label`) grouping the agent list by state: `BLOCKED`, `WAITING`, `RUNNING`, `REVIEW`, `PR`, `MERGED`. Under each label, the corresponding agent rows. Mirrors Codex's `Chats` section in `codex-dark-03-reactivated.png` (single label + list below).
4. **Bottom anchor region** — pinned to the bottom of the sidebar via `margin-top: auto`. Contains `Settings` row with a gear leading icon. Matches Codex's `Settings` row at the bottom of `codex-dark-04-new-chat-create-agent.png`. `Usage` lives in the same bottom region above `Settings`.

Selected-state in the sidebar — derived from Codex's selected `Capture Codex agent dashboard fl...` row in `codex-dark-03-reactivated.png`:

- Selected row background = `var(--bg-selected)` (a subtle lift from sidebar bg, not a hard accent fill).
- Text color stays `var(--text-primary)` — no color change.
- No leading bar, no border, no glow ring. The background fill alone communicates selection.
- Hover (non-selected) row background = `var(--bg-hover)`. Distinguishable from selected only by intensity (hover is briefer / lower-contrast than selected).
- Keyboard focus = 1px underline on the row label in `var(--text-primary)`, no outline rectangle. Matches the focus decision in `register.md` line 200.

---

## View: `list`

**Desktop mapping: `list` is not its own view on desktop.** The agent list renders into the sidebar's section-labels + agent-rows region (slot 3 above). The main pane shows whatever the most recently selected agent's `detail` view contains, or — on a cold load with no selection — the `create` view (the `+ New agent` landing).

Codex reference: `codex-dark-03-reactivated.png` — the sidebar's `Chats` section is the analog. Codex shows a single `Chats` label with chat rows below; the dashboard shows multiple state-labeled groups (`BLOCKED`, `WAITING`, …) with agent rows under each.

| Slot | Content |
|---|---|
| Left rail | The shared sidebar (above). Section labels + agent rows are the focal content. |
| Main pane | The currently selected agent's `detail` view, OR the `create` view when no agent is selected. There is no standalone "list page" on desktop. |

Visitor invariant: the agent that needs human input is still first — the `BLOCKED` group renders at the top of the sidebar's section list, ahead of `RUNNING`. Same priority order as mobile.

Empty state (no agents at all): sidebar shows the section-labels region as empty (no headers, no rows); main pane renders the `create` view as the default landing.

---

## View: `detail`

Codex reference: `codex-dark-03-reactivated.png` (dark) and `codex-light-03-chat-navigation-live-status.png` (light) — Codex chat/detail two-pane.

| Slot | Content |
|---|---|
| Left rail | Shared sidebar. The agent corresponding to this detail view is the selected row (slot 3) — `var(--bg-selected)` fill on that row. |
| Main pane | Top: detail app bar — agent name title, machine sub-line, optional spinner when running, kebab on the right. Mirrors Codex's top-of-pane title `Capture Codex agent dashboard flows ···`. Below: chat transcript scroll (assistant prose + user pills + tool-call blocks). Bottom: sticky composer — full-width rounded textarea (`--radius-xl` corners), `+` attach button left, send-circle right. Mirrors Codex's composer `Ask for follow-up changes` row in `codex-dark-03-reactivated.png`. |

Tabs (Chat / Activity / Diff / Plan) render as a horizontal segmented control directly under the app bar, above the transcript. This is unchanged from the mobile flow-map's A2 step.

Visitor invariant: the last assistant message must be in the first viewport on page load. Unchanged from mobile.

---

## View: `create`

Codex reference: `codex-dark-04-new-chat-create-agent.png` (dark) and `codex-light-02-new-chat-create-agent.png` (light) — Codex new-chat landing.

| Slot | Content |
|---|---|
| Left rail | Shared sidebar. The `+ New agent` row at the top (slot 1) is the selected row — `var(--bg-selected)` fill. No agent row is selected. |
| Main pane | Vertically centered composition block on a wide main pane. Top: large display title `What should we work on?` (Codex shows literally this string). Below: composer — same rounded textarea component as `detail`'s composer, with placeholder `Do anything`. Below the composer: a row of secondary controls — folder picker chip (`Work in a project`), harness chip (`claude` / `codex`), model selector. Below that: optional integration tiles (deferred to a later phase; the slot is declared, content is empty in v1). |

Visitor invariant: the composer is the visual center of the main pane. Folder + harness chips are secondary chrome, not primary UI. The display title sits 24–32px above the composer. Unchanged in intent from the mobile flow-map's B1 step.

---

## View: `usage`

Codex reference: `codex-dark-05-settings.png` (dark) and `codex-light-01-settings-general.png` (light) — Codex settings two-pane. The dashboard's `usage` view is the analog (a metrics + admin surface, modernized to match the Codex settings register).

| Slot | Content |
|---|---|
| Left rail | Shared sidebar. The `Usage` row in the sidebar's bottom anchor region (slot 4) is the selected row — `var(--bg-selected)` fill. No agent row is selected. |
| Main pane | The existing usage view content (cost tables, charts, time-range controls), restyled to use the new tokens. Codex shows a section header `General` with stacked subsections (`Work mode`, `Permissions`, `General` sub-section); the dashboard's usage view stacks its existing sections the same way — a top-level header + grouped subsections separated by `--space-7` vertical rhythm. No internal sub-rail (Codex has one for Settings sub-pages — `General`, `Appearance`, …; the dashboard's `Usage` is a single page and does not need a sub-rail). |

The functional content of `usage` is preserved; only the surrounding chrome (page edge padding, header type scale, section spacing) is modernized.

---

## Page-state inventory (desktop targets)

The grader will look at one screenshot per row, at viewport 1280×800.

| ID | View | State |
|---|---|---|
| A1-d | list / detail | Sidebar with 3+ agents grouped by state, main pane showing the selected agent's `detail` view |
| A2-d | detail | Selected RUNNING agent, mid-transcript, Chat tab active |
| A2-d-tools | detail | Selected RUNNING agent, transcript with tool-call blocks visible |
| A2-d-composer | detail | Composer focused, multi-line input |
| B1-d | create | `+ New agent` selected in sidebar, main pane showing the empty composer + display title |
| B1-d-filled | create | Composer with prompt typed, folder set, harness chosen |
| U1-d | usage | `Usage` selected in sidebar, main pane showing the usage view content |
| A1-d-empty | list / detail | No agents at all — sidebar empty in section region, main pane showing `create` as the default landing |

Eight desktop screenshots. Mobile coverage continues to come from `flow-map.md` and is bit-exact preserved (see `desktop-preservation.md`).
