# Documentation Site Review

**Site:** https://bjornjee.github.io/agent-dashboard/
**Reviewed:** 2026-04-06
**Pages reviewed:** 17 (all tabs and subpages)
**Method:** Playwright automated crawl with accessibility snapshots, full-page screenshots, and source file analysis

---

## Scoring Rubric

Each page is scored 1-5 across five dimensions:

| Dimension | What it measures |
|-----------|-----------------|
| **Structure** | Heading hierarchy, logical flow, scanability |
| **Completeness** | Coverage of the topic, no gaps or TODOs |
| **Clarity** | Writing quality, conciseness, jargon handling |
| **Navigation** | Cross-links, breadcrumbs, discoverability |
| **Polish** | Callouts, code examples, visual aids, formatting |

**Rating scale:** 1 = needs rewrite, 2 = significant gaps, 3 = adequate, 4 = good, 5 = excellent

---

## Page-by-Page Scores

### Home (`/`)

| Dimension | Score | Notes |
|-----------|-------|-------|
| Structure | 5 | Clear hero, install, features, demo, ecosystem |
| Completeness | 4 | Video demo embed has no poster/fallback image |
| Clarity | 5 | Punchy two-line tagline, strong value proposition |
| Navigation | 5 | Two CTAs (Get Started, GitHub), sidebar nav |
| Polish | 4 | Code blocks lack `bash` language hint; large empty space around video |

**Overall: 4.6/5**

---

### Getting Started (`/getting-started/`)

| Dimension | Score | Notes |
|-----------|-------|-------|
| Structure | 5 | Step-based progression (1/2/3), prerequisites table, uninstall section |
| Completeness | 5 | Covers install, build from source, plugin registration, first interactions, uninstall |
| Clarity | 5 | "Five minutes" promise, clear instructions |
| Navigation | 4 | Links to dependencies but no "Next: tmux Setup" link at bottom |
| Polish | 4 | Good use of `{: .note}` callout; keybinding list is well formatted |

**Overall: 4.6/5**

---

### tmux Setup (`/getting-started/tmux-setup/`)

| Dimension | Score | Notes |
|-----------|-------|-------|
| Structure | 4 | Three clear sections; could use a "before you begin" note |
| Completeness | 4 | Covers keybinding, mechanism, and recommended settings; no troubleshooting |
| Clarity | 5 | Concise, practical |
| Navigation | 3 | No cross-links to architecture or adapter pages |
| Polish | 4 | Good `{: .tip}` callout; code blocks are copy-pasteable |

**Overall: 4.0/5**

---

### Guides Index (`/guides/`)

| Dimension | Score | Notes |
|-----------|-------|-------|
| Structure | 4 | Simple TOC landing page |
| Completeness | 3 | One-liner description per guide would help scanning |
| Clarity | 4 | Clear purpose statement |
| Navigation | 4 | Bulleted links to all children |
| Polish | 3 | Bare list with no descriptions; could add a sentence per guide |

**Overall: 3.6/5**

---

### Mobile Companion (`/guides/mobile-companion/`)

| Dimension | Score | Notes |
|-----------|-------|-------|
| Structure | 5 | Logical flow: start server, capabilities, PWA install, OAuth |
| Completeness | 4 | Missing: HTTPS requirement for PWA on non-localhost, partial OAuth env var handling |
| Clarity | 5 | Clear, practical instructions |
| Navigation | 3 | No cross-link to settings reference for env vars |
| Polish | 4 | Good `{: .tip}` for finding local IP |

**Overall: 4.2/5**

---

### Creating Sessions (`/guides/creating-sessions/`)

| Dimension | Score | Notes |
|-----------|-------|-------|
| Structure | 4 | Three sections covering the full workflow |
| Completeness | 4 | Could mention how to customize or add skills |
| Clarity | 5 | Clean, well-paced |
| Navigation | 3 | Links to z plugin but no cross-link to adapter reference |
| Polish | 4 | Good use of `{: .note}` callout and skill table |

**Overall: 4.0/5**

---

### Reviewing Diffs (`/guides/reviewing-diffs/`)

| Dimension | Score | Notes |
|-----------|-------|-------|
| Structure | 4 | Four sections covering what/how/extras |
| Completeness | 4 | Keybinding table duplicates reference page (maintenance risk) |
| Clarity | 5 | Color-coded file status list is helpful |
| Navigation | 3 | No cross-link to keybindings reference |
| Polish | 3 | No callouts; could highlight sticky function headers as a tip |

**Overall: 3.8/5**

---

### PR Workflow (`/guides/pr-workflow/`)

| Dimension | Score | Notes |
|-----------|-------|-------|
| Structure | 4 | Four sections: create, review, merge, mobile |
| Completeness | 3 | Missing: PR close without merge, failure handling, "cleanup message" explanation |
| Clarity | 4 | Concise but some terms unexplained |
| Navigation | 2 | **BROKEN LINK** to Reviewing Diffs (relative URL resolves to 404); no link to mobile companion |
| Polish | 4 | Good `{: .note}` about gh requirement |

**Overall: 3.4/5**

---

### Notifications (`/guides/notifications/`)

| Dimension | Score | Notes |
|-----------|-------|-------|
| Structure | 4 | Three sections: enable, triggers, mechanism |
| Completeness | 3 | Missing: testing notifications, macOS permission prompts, Linux daemon requirements |
| Clarity | 4 | `silent_events` naming is slightly confusing in context |
| Navigation | 3 | No cross-links |
| Polish | 3 | No callouts; trigger table is well-structured |

**Overall: 3.4/5**

---

### Reference Index (`/reference/`)

| Dimension | Score | Notes |
|-----------|-------|-------|
| Structure | 4 | Simple TOC |
| Completeness | 3 | Same as Guides index — bare list, no descriptions |
| Clarity | 4 | Clear purpose statement |
| Navigation | 4 | Links to all children |
| Polish | 3 | Could add one-line descriptions per child |

**Overall: 3.6/5**

---

### Keybindings (`/reference/keybindings/`)

| Dimension | Score | Notes |
|-----------|-------|-------|
| Structure | 5 | Two clear context-based sections |
| Completeness | 4 | Missing: create session view, usage dashboard view, reply input mode keybindings |
| Clarity | 5 | Clean tables, `h` overlay mention |
| Navigation | 3 | No cross-links to guides that explain each workflow |
| Polish | 4 | Tables are well-formatted with copy-pasteable key names |

**Overall: 4.2/5**

---

### Settings (`/reference/settings/`)

| Dimension | Score | Notes |
|-----------|-------|-------|
| Structure | 5 | TOML reference, table, env vars — three complementary views |
| Completeness | 3 | Missing OAuth env vars (GOOGLE_CLIENT_ID etc.) documented only in mobile guide |
| Clarity | 5 | Clear fallback behavior explanation |
| Navigation | 3 | No cross-link to mobile companion for OAuth vars |
| Polish | 3 | No callouts; could highlight installer default path |

**Overall: 3.8/5**

---

### Adapter (`/reference/adapter/`)

| Dimension | Score | Notes |
|-----------|-------|-------|
| Structure | 5 | Hooks/Skills/Agents subsections well organized |
| Completeness | 4 | Agent state schema section is brief; could inline JSON structure |
| Clarity | 5 | Clear explanation of the bridge concept |
| Navigation | 2 | **BROKEN LINK** to Getting Started resolves under `/reference/getting-started` (404) |
| Polish | 3 | No callouts; three comprehensive tables |

**Overall: 3.8/5**

---

### Development Index (`/development/`)

| Dimension | Score | Notes |
|-----------|-------|-------|
| Structure | 4 | Simple TOC |
| Completeness | 3 | Bare list, no descriptions |
| Clarity | 5 | Great intro line: "hard-won debugging stories" |
| Navigation | 4 | Links to all children |
| Polish | 3 | Could add one-line descriptions |

**Overall: 3.8/5**

---

### Contributing (`/development/contributing/`)

| Dimension | Score | Notes |
|-----------|-------|-------|
| Structure | 5 | Seven sections covering full contributor workflow |
| Completeness | 4 | Missing: code review expectations, branch naming, CI pipeline details |
| Clarity | 5 | Welcoming tone, clear instructions |
| Navigation | 2 | **BROKEN LINK** to learnings page AMFI section (relative URL resolves under `/contributing/`) |
| Polish | 3 | No callouts for important CGO_ENABLED=0 note; make targets as code block rather than table |

**Overall: 3.8/5**

---

### Architecture (`/development/architecture/`)

| Dimension | Score | Notes |
|-----------|-------|-------|
| Structure | 5 | Directory trees, package descriptions, dependencies, data flow |
| Completeness | 4 | Missing: web server SSE/HTTP API endpoint documentation |
| Clarity | 5 | Concise package descriptions, clean ASCII diagrams |
| Navigation | 4 | 11 external links to dependencies |
| Polish | 4 | Good use of ASCII tree diagrams; dependency table links to repos |

**Overall: 4.4/5**

---

### Learnings (`/development/learnings/`)

| Dimension | Score | Notes |
|-----------|-------|-------|
| Structure | 5 | Problem/Impact/Fix pattern throughout; strong Key Takeaways summary |
| Completeness | 5 | 8 distinct issues, each with clear diagnosis and fix |
| Clarity | 5 | "War story" format is engaging and educational |
| Navigation | 3 | No internal or external links; could link to relevant source files |
| Polish | 4 | 6 code examples; could use callouts for Fix/Lesson sections; page is very long |

**Overall: 4.4/5**

---

## Summary Scorecard

| Page | Structure | Complete | Clarity | Nav | Polish | **Avg** |
|------|-----------|----------|---------|-----|--------|---------|
| Home | 5 | 4 | 5 | 5 | 4 | **4.6** |
| Getting Started | 5 | 5 | 5 | 4 | 4 | **4.6** |
| tmux Setup | 4 | 4 | 5 | 3 | 4 | **4.0** |
| Guides Index | 4 | 3 | 4 | 4 | 3 | **3.6** |
| Mobile Companion | 5 | 4 | 5 | 3 | 4 | **4.2** |
| Creating Sessions | 4 | 4 | 5 | 3 | 4 | **4.0** |
| Reviewing Diffs | 4 | 4 | 5 | 3 | 3 | **3.8** |
| PR Workflow | 4 | 3 | 4 | 2 | 4 | **3.4** |
| Notifications | 4 | 3 | 4 | 3 | 3 | **3.4** |
| Reference Index | 4 | 3 | 4 | 4 | 3 | **3.6** |
| Keybindings | 5 | 4 | 5 | 3 | 4 | **4.2** |
| Settings | 5 | 3 | 5 | 3 | 3 | **3.8** |
| Adapter | 5 | 4 | 5 | 2 | 3 | **3.8** |
| Development Index | 4 | 3 | 5 | 4 | 3 | **3.8** |
| Contributing | 5 | 4 | 5 | 2 | 3 | **3.8** |
| Architecture | 5 | 4 | 5 | 4 | 4 | **4.4** |
| Learnings | 5 | 5 | 5 | 3 | 4 | **4.4** |

**Site-wide average: 3.9/5**

---

## Bugs Found

### Broken Links (3 confirmed via Playwright)

1. **PR Workflow** (`/guides/pr-workflow/`): "Reviewing Diffs" link uses relative `reviewing-diffs` which resolves to `/guides/pr-workflow/reviewing-diffs` (404). Fix: use `../reviewing-diffs/`.

2. **Adapter** (`/reference/adapter/`): "Getting Started" link resolves to `/reference/getting-started` (404). Fix: use `/agent-dashboard/getting-started/` or `../../getting-started/`.

3. **Contributing** (`/development/contributing/`): "macOS AMFI kills" link resolves to `/development/contributing/learnings#...` (404). Fix: use `../learnings/#macos-amfi-kills-unsigned-test-binaries`.

### Console Error

- Home page loads with 1 console error (likely the video embed or a missing asset).

---

## Recommendations

### P0 — Fix Now (broken functionality)

1. **Fix the 3 broken links** listed above. All are relative URL issues where the link resolves under the current page's path instead of the sibling path.

### P1 — High Impact Improvements

2. **Add one-line descriptions to index pages** (Guides, Reference, Development). Currently they're bare bullet lists. A sentence per child page helps users decide where to go without clicking.

3. **Add cross-links between related pages.** The weakest dimension across the site is Navigation (avg 3.1/5). Specific additions:
   - Settings reference should link to Mobile Companion for OAuth env vars
   - Keybindings reference should link to the guide that explains each workflow
   - PR Workflow should link to Mobile Companion
   - tmux Setup should link to Architecture for how state files work
   - Guides should link to their corresponding Reference pages and vice versa

4. **Consolidate OAuth env vars into the Settings reference.** `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, and `GOOGLE_ALLOWED_EMAIL` are only documented in the Mobile Companion guide, not in `/reference/settings/`. Users looking at the settings reference won't find them.

5. **Add a video poster/fallback image on the homepage.** The `<video>` tag has no `poster` attribute. If the GitHub user-attachments URL fails, there's a large empty space.

### P2 — Polish

6. **Use callouts more consistently.** The `warning` callout type is defined in `_config.yml` but never used. Good candidates:
   - Contributing: `{: .warning}` for the CGO_ENABLED=0 requirement
   - Notifications: `{: .tip}` for testing notifications
   - Learnings: `{: .note}` for Fix/Lesson sections

7. **Add a custom 404 page.** just-the-docs supports `docs/404.md`. With 3 broken internal links found, users hitting stale bookmarks or mistyped URLs get the default GitHub Pages 404.

8. **Add `bash` language hints to fenced code blocks** on the homepage for syntax highlighting.

9. **Document additional keybinding contexts.** The keybindings reference covers Main Dashboard and Diff Viewer but not the create session view, usage dashboard, or reply input mode.

10. **Clean up `_config.yml`:** Remove the empty `ga_tracking` key (or add a tracking ID). Remove the unused `warning` callout type definition if you choose not to use it.

### P3 — Nice to Have

11. **Add terminal screenshots or GIFs to guides.** Every page is text-only (except the homepage video). A screenshot of the diff viewer, the session creator, or the mobile PWA would significantly improve comprehension.

12. **Consider a "Next page" / "Previous page" footer.** just-the-docs doesn't do this by default, but a manual "Next: tmux Setup" link at the bottom of Getting Started would improve flow.

13. **Add a favicon.** No `logo` or `favicon` is configured. The browser tab shows a generic icon.

14. **Reduce keybinding table duplication.** The diff viewer keybindings appear in both `/guides/reviewing-diffs/` and `/reference/keybindings/`. If one is updated and the other isn't, they'll drift. Consider linking from the guide to the reference instead of duplicating.
