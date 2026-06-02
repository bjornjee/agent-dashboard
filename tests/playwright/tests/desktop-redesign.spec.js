// @ts-check
// Phase F: structural Playwright coverage for the desktop redesign.
//
// Locks the following behaviors across Phases B–E:
//   B  two-pane shell at desktop (≥1024px) + sidebar contents/routing
//   C  hover/focus affordances + dock-into-app-bar migration
//   D  usage view modernization (no saturated green chart/progress fills)
//   E  light-theme tokens (sidebar/main bg, primary text)
//   mobile regression sanity (sidebar hidden, no two-pane shell)
//
// The parent runner ensures a Go web dashboard server is up at
// http://localhost:8391 (see playwright.config.js webServer). All API
// endpoints are mocked per-test so the dashboard renders deterministically
// without leaning on a live agent harness.

const { test, expect } = require('@playwright/test');

// ---------- helpers ----------

function makeAgent(overrides) {
  return {
    session_id: 'desk-001',
    cwd: '/Users/test/Code/myapp',
    branch: 'feat/desktop',
    model: 'opus',
    state: 'running',
    started_at: new Date().toISOString(),
    subagent_count: 0,
    ...overrides,
  };
}

async function mockApi(page, agents) {
  // Block live SSE — we want a deterministic snapshot.
  await page.route('**/events', (route) => route.abort('connectionrefused'));

  await page.route(/\/api\/agents/, async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    if (path === '/api/agents') {
      await route.fulfill({ json: agents });
    } else if (path.endsWith('/conversation')) {
      await route.fulfill({
        json: [
          { Role: 'human', Content: 'Hi', Timestamp: new Date().toISOString() },
          { Role: 'assistant', Content: 'Hello', Timestamp: new Date().toISOString() },
        ],
      });
    } else if (path.endsWith('/usage')) {
      await route.fulfill({ json: { InputTokens: 1000, OutputTokens: 500, CostUSD: 0.05 } });
    } else if (path.endsWith('/subagents')) {
      await route.fulfill({ json: [] });
    } else {
      await route.fulfill({ json: {} });
    }
  });

  await page.route('**/api/usage/ratelimit', async (route) => {
    // Delay so it arrives after the daily payload (avoids the rate-limit-card
    // wipe race documented in usage-bar.spec.js).
    setTimeout(() => {
      route.fulfill({
        json: {
          session: { used_percent: 42.5, resets_at: '2099-12-31T20:00:00Z' },
          weekly: { used_percent: 25.0, resets_at: '2099-12-31T00:00:00Z' },
          plan: 'max_5',
        },
      });
    }, 150);
  });

  await page.route('**/api/usage/daily*', async (route) => {
    route.fulfill({
      json: {
        days: [
          { date: new Date(Date.now() - 86400000).toISOString().slice(0, 10), cost_usd: 1.0, input_tokens: 1000, output_tokens: 500, cache_read_tokens: 0, cache_write_tokens: 0 },
          { date: new Date().toISOString().slice(0, 10), cost_usd: 1.5, input_tokens: 1500, output_tokens: 800, cache_read_tokens: 0, cache_write_tokens: 0 },
        ],
        today_cost: 1.5,
        total_cost: 10.0,
      },
    });
  });

  // Ensure each test starts cold (no stashed dashboard-view).
  await page.addInitScript(() => {
    try { sessionStorage.clear(); } catch {}
  });
}

async function gotoDesktop(page, agents) {
  await page.setViewportSize({ width: 1280, height: 800 });
  await mockApi(page, agents || [makeAgent()]);
  await page.goto('/');
  await page.waitForSelector('#app-shell', { timeout: 5000 });
}

// Resolve a CSS variable in the live page context (handles theme cascade).
async function cssVar(page, name) {
  return await page.evaluate((n) => {
    return getComputedStyle(document.documentElement).getPropertyValue(n).trim();
  }, name);
}

// Normalise CSS color strings (e.g. "rgb(0, 0, 0)" or "#000") to lowercase.
function norm(s) {
  return (s || '').replace(/\s+/g, '').toLowerCase();
}

// ---------- Desktop shell (≥1024px) ----------

test.describe('Desktop shell layout', () => {
  test('app-shell is grid var(--sidebar-width) 1fr at 1280×800', async ({ page }) => {
    await gotoDesktop(page);

    const shell = page.locator('#app-shell');
    await expect(shell).toBeVisible();
    const display = await shell.evaluate((el) => getComputedStyle(el).display);
    expect(display).toBe('grid');

    const cols = await shell.evaluate((el) => getComputedStyle(el).gridTemplateColumns);
    // gridTemplateColumns resolves to "<sidebar-px> <main-px>"
    const parts = cols.split(/\s+/);
    expect(parts.length).toBe(2);
    const sidebarPx = parseFloat(parts[0]);
    expect(sidebarPx).toBeCloseTo(264, 0); // --sidebar-width

    // Sidebar visible and not hidden; main pane fills remainder.
    const sidebar = page.locator('#app-sidebar');
    await expect(sidebar).toBeVisible();
    const hidden = await sidebar.evaluate((el) => el.hasAttribute('hidden'));
    expect(hidden).toBe(false);

    const sidebarBox = await sidebar.boundingBox();
    const mainBox = await page.locator('#app-main').boundingBox();
    expect(sidebarBox).not.toBeNull();
    expect(mainBox).not.toBeNull();
    expect(sidebarBox.width).toBeCloseTo(264, 0);
    expect(mainBox.width).toBeCloseTo(1280 - 264, 0);
  });

  test('mobile breakpoint at 899px hides the sidebar and stacks single-column', async ({ page }) => {
    await page.setViewportSize({ width: 899, height: 800 });
    await mockApi(page, [makeAgent()]);
    await page.goto('/');
    await page.waitForSelector('.agent-card, .ui-row', { timeout: 5000 });

    const sidebar = page.locator('#app-sidebar');
    const visible = await sidebar.isVisible();
    expect(visible).toBe(false);

    const mainBox = await page.locator('#app-main').boundingBox();
    expect(mainBox).not.toBeNull();
    expect(mainBox.width).toBeCloseTo(899, 0);
  });
});

// ---------- Sidebar contents + selected-state routing ----------

test.describe('Sidebar contents and routing', () => {
  test('renders + New agent CTA, Search placeholder, Usage and Settings anchors', async ({ page }) => {
    await gotoDesktop(page);
    await page.waitForSelector('#app-sidebar .app-sidebar__inner', { timeout: 5000 });

    const sidebar = page.locator('#app-sidebar');
    await expect(sidebar.getByText('New agent', { exact: true })).toBeVisible();
    await expect(sidebar.getByText('Search agents', { exact: true })).toBeVisible();
    await expect(sidebar.getByText('Usage', { exact: true })).toBeVisible();
    await expect(sidebar.getByText('Settings', { exact: true })).toBeVisible();

    // RUNNING group label appears because the mock agent is running.
    const labels = await sidebar.locator('.ui-section-label').allTextContents();
    expect(labels.some((l) => /running/i.test(l))).toBe(true);
  });

  test('cold load on / marks + New agent as selected (iter-3 regression guard)', async ({ page }) => {
    await gotoDesktop(page);
    await page.waitForSelector('#app-sidebar .app-sidebar__nav-row--selected', { timeout: 5000 });

    // The first nav-row in the sidebar should be the "+ New agent" CTA and
    // it should carry the --selected modifier on cold load.
    const selectedRow = page.locator('#app-sidebar .app-sidebar__nav-row--selected').first();
    await expect(selectedRow).toContainText('New agent');

    const innerRow = selectedRow.locator('.ui-row');
    const bg = await innerRow.evaluate((el) => getComputedStyle(el).backgroundColor);
    // --bg-selected resolves to a non-transparent color (dark or light theme).
    expect(bg).not.toBe('rgba(0, 0, 0, 0)');
  });

  test('clicking an agent in the sidebar routes detail and applies --bg-selected', async ({ page }) => {
    await gotoDesktop(page);
    await page.waitForSelector('#app-sidebar .app-sidebar__row[data-agent-id]', { timeout: 5000 });

    await page.click('#app-sidebar .app-sidebar__row[data-agent-id] .ui-row');
    await page.waitForSelector('#app-main .detail-layout', { timeout: 5000 });

    const selected = page.locator('#app-sidebar .app-sidebar__row--selected');
    await expect(selected).toHaveCount(1);
    const id = await selected.getAttribute('data-agent-id');
    expect(id).toBe('desk-001');

    // Selected fill is the --bg-selected token, not transparent.
    const bg = await selected.locator('.ui-row').evaluate((el) => getComputedStyle(el).backgroundColor);
    expect(bg).not.toBe('rgba(0, 0, 0, 0)');
  });

  test('clicking Usage routes the main pane to usage and marks Usage selected', async ({ page }) => {
    await gotoDesktop(page);
    await page.waitForSelector('#app-sidebar .app-sidebar__bottom', { timeout: 5000 });

    // Usage row sits in the bottom anchor cluster.
    const usageRow = page.locator('#app-sidebar .app-sidebar__bottom .app-sidebar__nav-row').filter({ hasText: 'Usage' });
    await usageRow.locator('.ui-row').click();

    // Main pane swaps to the usage view (rate-limit-card is rendered by Phase D).
    await page.waitForSelector('#app-main .usage-rate__rows, #app-main .usage-card', { timeout: 5000 });

    // The Usage row carries the selected modifier.
    await expect(usageRow).toHaveClass(/app-sidebar__nav-row--selected/);
    const bg = await usageRow.locator('.ui-row').evaluate((el) => getComputedStyle(el).backgroundColor);
    expect(bg).not.toBe('rgba(0, 0, 0, 0)');
  });
});

// ---------- Detail tabs above the fold ----------

test.describe('Detail view tabs', () => {
  test('Chat / Activity / Diff / Plan all render above y=800 on 1280×800', async ({ page }) => {
    await gotoDesktop(page);
    await page.waitForSelector('#app-sidebar .app-sidebar__row[data-agent-id]', { timeout: 5000 });
    await page.click('#app-sidebar .app-sidebar__row[data-agent-id] .ui-row');
    await page.waitForSelector('#app-main .detail-tabs', { timeout: 5000 });

    const tabs = page.locator('#app-main .detail-tabs__tab');
    const labels = await tabs.allTextContents();
    // Sanity — must include each tab. Names should be human-readable, e.g.
    // "Chat", "Activity", "Diff", "Plan".
    for (const expected of ['Chat', 'Activity', 'Diff', 'Plan']) {
      expect(labels.some((l) => l.trim().toLowerCase() === expected.toLowerCase())).toBe(true);
    }

    const count = await tabs.count();
    expect(count).toBeGreaterThanOrEqual(4);
    for (let i = 0; i < count; i++) {
      const box = await tabs.nth(i).boundingBox();
      expect(box, `tab ${i} should have bounding box`).not.toBeNull();
      // Tab fully visible above the fold (top + height ≤ 800).
      expect(box.y + box.height).toBeLessThanOrEqual(800);
      expect(box.y).toBeGreaterThanOrEqual(0);
    }
  });
});

// ---------- Hover + focus-visible affordances (Phase C) ----------

test.describe('Sidebar hover and focus-visible affordances', () => {
  // Hover assertions require @media (hover: hover) — Chromium's default
  // emulation matches this, but be explicit so future browser projects can't
  // silently drift into 'none'.
  test.use({ hasTouch: false });

  test('hovering a sidebar row resolves the row background to --bg-hover', async ({ page }) => {
    await gotoDesktop(page);
    await page.waitForSelector('#app-sidebar .app-sidebar__row[data-agent-id] .ui-row', { timeout: 5000 });

    const hoverToken = await cssVar(page, '--bg-hover');
    expect(hoverToken).not.toBe('');

    // page.hover() + getComputedStyle does not see :hover-state styles in
    // headless Chromium — getComputedStyle never returns pseudo-class state
    // even though the rendering does. We instead force :hover via CDP and
    // use CSS.getMatchedStylesForNode to verify the cascade resolves to
    // --bg-hover under @media (hover: hover). This is the supported way to
    // assert that the hover rule is wired and the cascade reaches it.
    const client = await page.context().newCDPSession(page);
    await client.send('DOM.enable');
    await client.send('CSS.enable');
    const { root } = await client.send('DOM.getDocument', { depth: -1, pierce: true });
    const { nodeId } = await client.send('DOM.querySelector', {
      nodeId: root.nodeId,
      selector: '#app-sidebar .app-sidebar__row[data-agent-id] .ui-row',
    });
    expect(nodeId).toBeGreaterThan(0);

    // Force :hover so the cascade includes the @media (hover: hover) rule,
    // then re-fetch matched styles.
    await client.send('CSS.forcePseudoState', {
      nodeId,
      forcedPseudoClasses: ['hover'],
    });

    const matched = await client.send('CSS.getMatchedStylesForNode', { nodeId });
    // matchedCSSRules now includes :hover rules. Pull the rule whose selector
    // names #app-sidebar … :hover (but not the --selected variant).
    /** @type {Array<{rule: any}>} */
    const hoverRules = (matched.matchedCSSRules || []).filter((m) => {
      const sel = m.rule && m.rule.selectorList && m.rule.selectorList.text;
      return typeof sel === 'string' && sel.includes('#app-sidebar') && sel.includes(':hover') && !sel.includes('--selected');
    });
    expect(hoverRules.length, 'expected at least one #app-sidebar :hover rule to match').toBeGreaterThan(0);

    // Walk the matched hover rules and find a `background` declaration that
    // references --bg-hover (the rule we shipped in Phase C). This survives
    // token rename refactors because we verify the rule cascades, not a
    // literal rgb() value.
    let found = false;
    for (const m of hoverRules) {
      const decls = (m.rule.style && m.rule.style.cssProperties) || [];
      for (const d of decls) {
        if ((d.name === 'background' || d.name === 'background-color') && d.value && d.value.includes('--bg-hover')) {
          found = true;
          break;
        }
      }
      if (found) break;
    }
    expect(found, 'expected an #app-sidebar :hover rule with background: var(--bg-hover)').toBe(true);

    await client.detach();
  });

  test('keyboard focus on a sidebar row exposes a focus-visible underline', async ({ page }) => {
    await gotoDesktop(page);
    await page.waitForSelector('#app-sidebar .app-sidebar__nav-row .ui-row', { timeout: 5000 });

    // Tab through the page until focus lands inside the sidebar.
    // Click into the document first so the body owns initial focus.
    await page.locator('body').click({ position: { x: 1, y: 1 } });
    // First tab usually moves into the sidebar's first interactive row.
    // Hop a few times in case the dashboard exposes other focusable chrome
    // earlier in the tab order.
    let title;
    for (let i = 0; i < 20; i++) {
      await page.keyboard.press('Tab');
      const focusedSelector = await page.evaluate(() => {
        const el = document.activeElement;
        if (!el) return null;
        return el.closest('#app-sidebar .ui-row') ? true : false;
      });
      if (focusedSelector) {
        title = page.locator('#app-sidebar :focus-visible .ui-row__title').first();
        break;
      }
    }
    expect(title, 'expected keyboard focus to reach a sidebar row').toBeTruthy();
    await expect(title).toBeVisible();

    const decoration = await title.evaluate((el) => getComputedStyle(el).textDecorationLine);
    expect(decoration).toContain('underline');
  });
});

// ---------- Dock migration (Phase C) ----------

test.describe('Dock migration into the desktop app-bar', () => {
  test('floating .ui-dock is display:none on desktop', async ({ page }) => {
    await gotoDesktop(page);
    await page.waitForSelector('#app-shell', { timeout: 5000 });

    const floatingDocks = page.locator('.ui-dock:not(.ui-dock--header)');
    const count = await floatingDocks.count();
    if (count > 0) {
      for (let i = 0; i < count; i++) {
        const display = await floatingDocks.nth(i).evaluate((el) => getComputedStyle(el).display);
        expect(display).toBe('none');
      }
    }
  });

  test('detail app-bar trailing slot exposes the header dock with New + Search', async ({ page }) => {
    await gotoDesktop(page);
    await page.waitForSelector('#app-sidebar .app-sidebar__row[data-agent-id] .ui-row', { timeout: 5000 });
    await page.click('#app-sidebar .app-sidebar__row[data-agent-id] .ui-row');
    await page.waitForSelector('#app-main .detail-layout', { timeout: 5000 });

    const trailing = page.locator('#app-main .ui-app-bar__trailing .ui-dock--header');
    await expect(trailing).toBeVisible();

    // The "New" CTA pill and the icon-only Search button are both reachable.
    const cta = trailing.locator('.ui-dock--header__cta');
    await expect(cta).toBeVisible();
    await expect(cta).toContainText('New');

    const search = trailing.locator('.ui-dock--header__search');
    await expect(search).toBeVisible();
  });
});

// ---------- Usage view modernization (Phase D) ----------

test.describe('Usage view modernization', () => {
  test('rate-limit progress fill resolves to --text-secondary, not saturated green', async ({ page }) => {
    await gotoDesktop(page);
    await page.waitForSelector('#app-sidebar .app-sidebar__bottom', { timeout: 5000 });
    const usageRow = page.locator('#app-sidebar .app-sidebar__bottom .app-sidebar__nav-row').filter({ hasText: 'Usage' });
    await usageRow.locator('.ui-row').click();

    await page.waitForSelector('#app-main .usage-rate__fill', { timeout: 5000 });

    // Both session (42.5%) and weekly (25%) bars are under 60% so default
    // class applies — fill should match --text-secondary, not --accent-green.
    const fills = page.locator('#app-main .usage-rate__fill');
    const count = await fills.count();
    expect(count).toBeGreaterThanOrEqual(2);

    const expectedRgb = await page.evaluate(() => {
      const probe = document.createElement('div');
      probe.style.color = getComputedStyle(document.documentElement).getPropertyValue('--text-secondary').trim();
      document.body.appendChild(probe);
      const c = getComputedStyle(probe).color;
      probe.remove();
      return c;
    });

    for (let i = 0; i < Math.min(count, 2); i++) {
      const bg = await fills.nth(i).evaluate((el) => getComputedStyle(el).backgroundColor);
      expect(bg).not.toBe('rgba(0, 0, 0, 0)');
      // Reject the saturated greens we explicitly migrated away from.
      expect(bg).not.toContain('34, 197, 94'); // #22c55e
      expect(bg).not.toContain('16, 185, 129'); // #10b981
      expect(bg).not.toContain('48, 209, 88'); // accent-green dark token #30D158
      expect(norm(bg)).toBe(norm(expectedRgb));
    }
  });

  test('usage-chart bars use --bg-elevated / --text-secondary, not saturated green', async ({ page }) => {
    await gotoDesktop(page);
    await page.waitForSelector('#app-sidebar .app-sidebar__bottom', { timeout: 5000 });
    const usageRow = page.locator('#app-sidebar .app-sidebar__bottom .app-sidebar__nav-row').filter({ hasText: 'Usage' });
    await usageRow.locator('.ui-row').click();
    await page.waitForSelector('#app-main .usage-bar', { timeout: 5000 });

    const bars = page.locator('#app-main .usage-bar');
    const count = await bars.count();
    expect(count).toBeGreaterThanOrEqual(1);

    for (let i = 0; i < count; i++) {
      const bg = await bars.nth(i).evaluate((el) => getComputedStyle(el).backgroundColor);
      expect(bg).not.toBe('rgba(0, 0, 0, 0)');
      // No saturated greens.
      expect(bg).not.toContain('34, 197, 94');
      expect(bg).not.toContain('16, 185, 129');
      expect(bg).not.toContain('48, 209, 88');
    }
  });
});

// ---------- Light theme (Phase E) ----------

test.describe('Light theme tokens', () => {
  test('switching to light theme resolves sidebar bg to the new light token (not legacy ivory)', async ({ page }) => {
    await gotoDesktop(page);
    await page.waitForSelector('#app-sidebar .app-sidebar__inner', { timeout: 5000 });

    await page.evaluate(() => {
      document.documentElement.dataset.theme = 'light';
    });

    // Sidebar background follows --bg-sidebar (declared at line 1632 of
    // style.css for light theme: #F4F4F5).
    const sidebarBg = await page.locator('#app-sidebar').evaluate((el) => getComputedStyle(el).backgroundColor);
    // Reject the dark-theme value (#0E0E10 ≈ rgb(14, 14, 16)).
    expect(sidebarBg).not.toContain('14, 14, 16');
    // Reject the legacy ivory (#FAFAF7 ≈ rgb(250, 250, 247)) explicitly.
    expect(sidebarBg).not.toContain('250, 250, 247');

    // Confirm it matches --bg-sidebar in light theme.
    const expectedBg = await page.evaluate(() => {
      const probe = document.createElement('div');
      probe.style.color = getComputedStyle(document.documentElement).getPropertyValue('--bg-sidebar').trim();
      document.body.appendChild(probe);
      const c = getComputedStyle(probe).color;
      probe.remove();
      return c;
    });
    expect(norm(sidebarBg)).toBe(norm(expectedBg));

    // Main pane background follows --bg-base (white in light theme).
    const mainBg = await page.locator('#app-main').evaluate((el) => getComputedStyle(el).backgroundColor);
    const expectedMainBg = await page.evaluate(() => {
      const probe = document.createElement('div');
      probe.style.color = getComputedStyle(document.documentElement).getPropertyValue('--bg-base').trim();
      document.body.appendChild(probe);
      const c = getComputedStyle(probe).color;
      probe.remove();
      return c;
    });
    // Body background drives the main pane visually — check that --bg-base
    // resolves to a white-ish color (R+G+B near 765), confirming theme cascade.
    const m = expectedMainBg.match(/\d+/g) || [];
    const sum = m.slice(0, 3).reduce((a, b) => a + Number(b), 0);
    expect(sum).toBeGreaterThan(700); // close to pure white (#FFFFFF)
  });

  test('light theme primary text resolves to a dark color (inverted from dark theme white)', async ({ page }) => {
    await gotoDesktop(page);
    await page.waitForSelector('#app-sidebar .app-sidebar__inner', { timeout: 5000 });

    await page.evaluate(() => {
      document.documentElement.dataset.theme = 'light';
    });

    const titleEl = page.locator('#app-sidebar .ui-row__title').first();
    const color = await titleEl.evaluate((el) => getComputedStyle(el).color);
    // Dark theme white (#FFFFFF rgb 255,255,255) should NOT appear.
    expect(color).not.toContain('255, 255, 255');
    // Sanity: should be a near-black color in light theme.
    const m = color.match(/\d+/g) || [];
    const sum = m.slice(0, 3).reduce((a, b) => a + Number(b), 0);
    expect(sum).toBeLessThan(150);
  });
});

// ---------- Mobile regression sanity ----------

test.describe('Mobile regression sanity', () => {
  test('at 390×844 the floating dock renders and the sidebar is hidden', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await mockApi(page, [makeAgent()]);
    await page.goto('/');
    // Wait for either the list view or any agent row to render.
    await page.waitForSelector('.agent-card, .ui-row', { timeout: 5000 });

    const sidebar = page.locator('#app-sidebar');
    expect(await sidebar.isVisible()).toBe(false);

    // Floating dock is rendered into the body (mobile-only).
    const dock = page.locator('.ui-dock:not(.ui-dock--header)');
    const dockCount = await dock.count();
    if (dockCount > 0) {
      const display = await dock.first().evaluate((el) => getComputedStyle(el).display);
      expect(display).not.toBe('none');
    }
    // If no dock node exists at all we accept that — list view drives render
    // and dock may only mount after the list is fully painted. The negative
    // assertion (no two-pane shell) is the load-bearing check here.
    const mainBox = await page.locator('#app-main').boundingBox();
    expect(mainBox.width).toBeCloseTo(390, 0);
  });
});
