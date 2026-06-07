# Confirmation Modal UX Fix

## Summary

What already exists: web destructive actions all call a single `showModal()` primitive in `internal/web/static/js/modal.js`, styled by `internal/web/static/style.css`, with existing Playwright coverage proving Merge -> Confirm still POSTs.

Implement a focused UI/UX pass for confirmation flows only. The modal should become a compact, centered, accessible confirmation dialog that matches the existing Codex/iOS product register: elevated surface, terse copy, clear action hierarchy, and deliberate friction for risky writes. Toast placement and broader web flow redesign stay out of scope for this feature.

## Phases

- [ ] **Phase A: Modal Behavior Tests** — files: `tests/playwright/tests/desktop-redesign.spec.js`, deps: -
- [ ] **Phase B: Dialog Primitive** — files: `internal/web/static/js/modal.js`, `internal/web/static/style.css`, deps: A
- [ ] **Phase C: Destructive Flow Copy** — files: `internal/web/static/app.js`, `tests/playwright/tests/desktop-redesign.spec.js`, deps: B

## Test Plan

- RED: run the new/updated Playwright modal tests and capture the failing output before implementation.
- GREEN: run the same Playwright tests after implementation.
- Full verification: run `make test`.
