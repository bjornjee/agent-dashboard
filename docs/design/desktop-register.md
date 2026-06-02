# Desktop register addendum

Addendum to `docs/design/register.md`. Declares the additional tokens and rules required for the desktop layout (viewport ≥ 1024px). Mobile is unchanged: everything in `register.md` continues to apply below 1024px bit-exactly.

This file does **not** restate the mobile register; it only declares what's new or different on desktop. Where this addendum is silent, the mobile register applies.

## Breakpoint

Single desktop breakpoint:

```
--bp-desktop: 1024px
```

`@media (min-width: 1024px)` switches the app shell from single-column-with-floating-dock (mobile) to two-pane sidebar + main (desktop). There is no intermediate "tablet" breakpoint in this redesign; the layout flips once.

## Sidebar dimensions

```
--sidebar-width: 264px
```

Fixed sidebar width on desktop. Chosen to match the visual proportion of Codex's sidebar in `codex-dark-03-reactivated.png` and `codex-dark-04-new-chat-create-agent.png` (Codex's sidebar reads as ~22–24% of a typical desktop viewport; 264px hits that ratio at 1280px wide and degrades gracefully at 1024px). Adjustable in a later iteration if the agent-name truncation becomes the limiting factor; the token name does not change.

The sidebar does not resize, does not collapse, and is not draggable in v1.

## Surface tokens — new

| Token | Dark value | Light value | Use |
|---|---|---|---|
| `--bg-sidebar` | `#0E0E10` | `#FFFFFF` | Sidebar background. Dark value matches the page `--bg-base` in mobile dark; the visual separation between sidebar and main pane comes from the sidebar containing a `--bg-selected` row, not from a different bg color. Light value matches the white sidebar visible in `codex-light-02-new-chat-create-agent.png`. |
| `--bg-selected` | `#222226` | `#EFEFEF` | Selected sidebar row background. Dark value reuses the same hue as `--bg-elevated` for layering consistency. Light value: a soft neutral gray that's clearly distinct from white but never tinted. |
| `--bg-hover` | `#2F2F33` | `#F4F4F5` | Hover background for interactive rows on desktop. Dark value was **already declared in `register.md` color table** but was unused in mobile (Codex iOS has no hover); desktop is the first surface that consumes it. Light value is one step darker than `--bg-base` light, never tinted. |

The intentional ordering on dark: `--bg-sidebar` (`#0E0E10`) → `--bg-hover` (`#2F2F33`) → `--bg-selected` (`#222226`). Selected is a *darker* layered fill than hover so a selected row reads as "settled into the surface", while hover reads as "lifted off the surface for the cursor". This matches Codex's behavior in `codex-dark-03-reactivated.png` where the selected `Capture Codex agent dashboard fl...` row reads as a calm filled state, not as a hot accent.

## Focus styling

Keyboard focus on desktop interactive elements (sidebar rows, composer, buttons):

- **Style:** 1px underline in `var(--text-primary)` on the element's text content. No outline rectangle on the row. No glow ring. No box-shadow.
- **Rationale:** matches the existing register decision at `register.md` line 200 ("hover/focus states … `--text-primary` underline + `#2F2F33` background, no glow rings"). The desktop addendum carries that rule forward verbatim — it is not relaxed.
- **Hover precedence:** when a row is both hovered and focused, the underline appears on top of the `--bg-hover` fill.

## Token mapping table — additions to the existing table

Append-only additions to the table in `register.md`. Existing tokens are not changed.

| Token | Old value | New value | Notes |
|---|---|---|---|
| `--bp-desktop` | (new) | `1024px` | Single desktop breakpoint. |
| `--sidebar-width` | (new) | `264px` | Desktop sidebar fixed width. |
| `--bg-sidebar` | (new) | dark `#0E0E10` / light `#FFFFFF` | Desktop sidebar background. |
| `--bg-hover` | declared in register.md color table but unconsumed | dark `#2F2F33` / light `#F4F4F5` | First consumer is the desktop sidebar row hover state. |
| `--bg-selected` | (new) | dark `#222226` / light `#EFEFEF` | Desktop sidebar selected-row background. |

## What this addendum intentionally does not declare

- **Collapsible sidebar / hamburger affordance.** Out of scope for v1. The sidebar is always visible at ≥ 1024px.
- **Sub-rails inside the main pane** (e.g., a Settings-style sub-navigation in `codex-dark-05-settings.png`). The dashboard's `usage` view is a single page; no sub-rail token is needed.
- **Multi-window / split-pane affordances.** Out of scope.
- **Resize handles or persistence of sidebar width.** Out of scope.
- **Animation for sidebar mount / unmount.** The sidebar exists at all desktop widths; nothing animates in or out at the breakpoint flip (the page itself remounts at the breakpoint).
