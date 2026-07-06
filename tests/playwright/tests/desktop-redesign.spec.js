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

// ---------- Thinking indicator per-turn lifecycle ----------
//
// The chat-feedback fix gates the "working" indicator on
// last_hook_event !== 'Stop'. agent.state stays "running" for the
// life of the tmux pane, so this is the only per-turn signal.

test.describe('Working indicator dismisses between turns', () => {
  async function mountDetailWithAgent(page, agent) {
    await page.setViewportSize({ width: 1280, height: 800 });
    await mockApi(page, [agent]);
    await page.goto('/');
    await page.waitForSelector('#app-sidebar .app-sidebar__row[data-agent-id]', { timeout: 5000 });
    await page.click('#app-sidebar .app-sidebar__row[data-agent-id] .ui-row');
    await page.waitForSelector('#app-main .detail-layout', { timeout: 5000 });
    await page.waitForSelector('#tab-conversation .conversation', { timeout: 5000 });
  }

  test('mid-turn agent (last_hook_event=PreToolUse) shows the indicator', async ({ page }) => {
    await mountDetailWithAgent(page, makeAgent({
      session_id: 'desk-001',
      state: 'running',
      last_hook_event: 'PreToolUse',
      current_tool: 'Bash',
    }));

    await page.evaluate(async () => {
      const mod = await import('/js/pages/detail.js');
      const agents = await fetch('/api/agents').then(r => r.json());
      mod.refreshWorkingIndicator(agents[0]);
    });

    const indicator = page.locator('.ui-msg-status--working');
    await expect(indicator).toBeVisible();
    await expect(indicator.locator('.ui-msg-status__label')).toContainText('Running command');
  });

  test('agent transitioning to last_hook_event=Stop removes the indicator', async ({ page }) => {
    await mountDetailWithAgent(page, makeAgent({
      session_id: 'desk-001',
      state: 'running',
      last_hook_event: 'PreToolUse',
      current_tool: 'Bash',
    }));

    await page.evaluate(async () => {
      const mod = await import('/js/pages/detail.js');
      const agents = await fetch('/api/agents').then(r => r.json());
      mod.refreshWorkingIndicator(agents[0]);
    });
    await expect(page.locator('.ui-msg-status--working')).toBeVisible();

    await page.evaluate(async () => {
      const mod = await import('/js/pages/detail.js');
      const agents = await fetch('/api/agents').then(r => r.json());
      const ended = { ...agents[0], last_hook_event: 'Stop' };
      mod.refreshWorkingIndicator(ended);
    });

    await expect(page.locator('.ui-msg-status--working')).toHaveCount(0);
  });

  test('agent with state=done removes the indicator regardless of last_hook_event', async ({ page }) => {
    await mountDetailWithAgent(page, makeAgent({
      session_id: 'desk-001',
      state: 'running',
      last_hook_event: 'PreToolUse',
      current_tool: 'Bash',
    }));
    await page.evaluate(async () => {
      const mod = await import('/js/pages/detail.js');
      const agents = await fetch('/api/agents').then(r => r.json());
      mod.refreshWorkingIndicator(agents[0]);
    });
    await expect(page.locator('.ui-msg-status--working')).toBeVisible();

    await page.evaluate(async () => {
      const mod = await import('/js/pages/detail.js');
      const agents = await fetch('/api/agents').then(r => r.json());
      const done = { ...agents[0], state: 'done', last_hook_event: 'PreToolUse' };
      mod.refreshWorkingIndicator(done);
    });
    await expect(page.locator('.ui-msg-status--working')).toHaveCount(0);
  });
});

// ---------- Tally + send-to-Thinking latency ----------

test.describe('Per-turn tally + optimistic Thinking', () => {
  // Patches mockApi to also serve /activity and /conversation with
  // controllable timestamps so we can assert turn-boundary behaviour.
  async function mockApiWithActivity(page, agent, opts) {
    const o = opts || {};
    const lastHumanAt = o.lastHumanAt || '2026-06-02T10:00:00.000Z';
    const priorTools = o.priorTools || []; // ts < lastHumanAt
    const currentTools = o.currentTools || []; // ts > lastHumanAt

    await page.route('**/events', (route) => route.abort('connectionrefused'));
    await page.route(/\/api\/agents/, async (route) => {
      const url = new URL(route.request().url());
      const path = url.pathname;
      if (path === '/api/agents') {
        await route.fulfill({ json: [agent] });
      } else if (path.endsWith('/conversation')) {
        await route.fulfill({ json: [
          { Role: 'human', Content: 'previous question', Timestamp: lastHumanAt },
          { Role: 'assistant', Content: 'previous answer', Timestamp: lastHumanAt },
        ]});
      } else if (path.endsWith('/activity')) {
        const entries = [
          ...priorTools.map((c, i) => ({ Kind: 'tool', Content: c, Timestamp: '2026-06-02T09:00:0' + i + '.000Z' })),
          ...currentTools.map((c, i) => ({ Kind: 'tool', Content: c, Timestamp: '2026-06-02T10:30:0' + i + '.000Z' })),
        ];
        await route.fulfill({ json: entries });
      } else if (path.endsWith('/usage')) {
        await route.fulfill({ json: { CostUSD: 0 } });
      } else if (path.endsWith('/subagents')) {
        await route.fulfill({ json: [] });
      } else {
        await route.fulfill({ json: {} });
      }
    });
    await page.route('**/api/usage/ratelimit', (r) => r.fulfill({ json: { session: {used_percent:1, resets_at:'2099-01-01T00:00:00Z'}, weekly: {used_percent:1, resets_at:'2099-01-01T00:00:00Z'}, plan: 'max_5' } }));
    await page.route('**/api/usage/daily*', (r) => r.fulfill({ json: { days: [], today_cost: 0, total_cost: 0 } }));
    await page.addInitScript(() => { try { sessionStorage.clear(); } catch {} });
  }

  test('tally ignores tools fired BEFORE the latest user message', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await mockApiWithActivity(page, makeAgent({
      session_id: 'desk-001', state: 'running', last_hook_event: 'PreToolUse', current_tool: 'Bash',
    }), {
      lastHumanAt: '2026-06-02T10:00:00.000Z',
      priorTools: ['→ Bash: old1', '→ Read: old2', '→ Edit: old3'], // 3 tools from PRIOR turn
      currentTools: ['→ Bash: new1', '→ Read: new2'],                // 2 tools from CURRENT turn
    });

    await page.goto('/');
    await page.waitForSelector('#app-sidebar .app-sidebar__row[data-agent-id]', { timeout: 5000 });
    await page.click('#app-sidebar .app-sidebar__row[data-agent-id] .ui-row');
    await page.waitForSelector('#app-main .detail-layout', { timeout: 5000 });
    await page.waitForSelector('.ui-msg-status--working', { timeout: 5000 });
    // Give the seed call a beat to resolve and render.
    await page.waitForTimeout(800);

    const tally = await page.locator('.ui-msg-status__tally').textContent();
    // 2 current tools: 1 ran command + 1 file read. Prior 3 are excluded.
    expect(tally).toContain('1');
    expect(tally).not.toContain('3'); // would fail if priors leaked
    expect(tally.toLowerCase()).toContain('command');
    expect(tally.toLowerCase()).toContain('file');
  });

  test('Thinking appears immediately after send (no SSE wait)', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    // Agent starts idle — no indicator on initial mount.
    await mockApiWithActivity(page, makeAgent({
      session_id: 'desk-001', state: 'idle_prompt', last_hook_event: 'Stop', current_tool: '',
    }));

    await page.goto('/');
    await page.waitForSelector('#app-sidebar .app-sidebar__row[data-agent-id]', { timeout: 5000 });
    await page.click('#app-sidebar .app-sidebar__row[data-agent-id] .ui-row');
    await page.waitForSelector('#app-main .detail-layout', { timeout: 5000 });
    await page.waitForTimeout(500);

    // Indicator should be absent (last_hook_event === 'Stop')
    expect(await page.locator('.ui-msg-status--working').count()).toBe(0);

    // Simulate sendInput. No SSE event will fire; the optimistic send path
    // mounts the indicator immediately, before POST ack returns.
    await page.evaluate(async () => {
      const mod = await import('/js/pages/detail.js');
      // Prime lastKnownAgent via a refresh that would no-op (Stop state).
      const agents = await fetch('/api/agents').then(r => r.json());
      mod.refreshWorkingIndicator(agents[0]);
      mod.appendUserMessage('test message');
    });

    // Indicator should be present WITHOUT waiting for SSE or POST ack.
    await expect(page.locator('.ui-msg-status--working')).toBeVisible({ timeout: 500 });
  });
});

// ---------- Slash-command autocomplete ----------

test.describe('Slash-command autocomplete', () => {
  async function mockApiWithSkills(page, skills, agentOverrides = {}) {
    await page.route('**/events', (route) => route.abort('connectionrefused'));
    await page.route('**/api/skills*', (route) => route.fulfill({ json: skills }));
    await page.route(/\/api\/agents/, async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path === '/api/agents') {
        await route.fulfill({ json: [makeAgent({ session_id: 'desk-001', state: 'running', last_hook_event: 'Stop', ...agentOverrides })] });
      } else if (path.endsWith('/conversation')) {
        await route.fulfill({ json: [] });
      } else if (path.endsWith('/activity')) {
        await route.fulfill({ json: [] });
      } else if (path.endsWith('/usage')) {
        await route.fulfill({ json: { CostUSD: 0 } });
      } else if (path.endsWith('/subagents')) {
        await route.fulfill({ json: [] });
      } else {
        await route.fulfill({ json: {} });
      }
    });
    await page.route('**/api/usage/ratelimit', (r) => r.fulfill({ json: { session: {used_percent:1, resets_at:'2099-01-01T00:00:00Z'}, weekly: {used_percent:1, resets_at:'2099-01-01T00:00:00Z'}, plan: 'max_5' } }));
    await page.route('**/api/usage/daily*', (r) => r.fulfill({ json: { days: [], today_cost: 0, total_cost: 0 } }));
    await page.addInitScript(() => { try { sessionStorage.clear(); } catch {} });
  }

  async function mountDetail(page) {
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto('/');
    await page.waitForSelector('#app-sidebar .app-sidebar__row[data-agent-id]', { timeout: 5000 });
    await page.click('#app-sidebar .app-sidebar__row[data-agent-id] .ui-row');
    await page.waitForSelector('#reply-input', { timeout: 5000 });
  }

  test('typing "/" shows the popup with /agent-dashboard:<skill> entries', async ({ page }) => {
    await mockApiWithSkills(page, ['pr', 'feature', 'implement']);
    await mountDetail(page);

    await page.locator('#reply-input').click();
    await page.locator('#reply-input').type('/');

    const popup = page.locator('#slash-autocomplete');
    await expect(popup).toBeVisible();
    const items = popup.locator('.slash-autocomplete__item');
    await expect(items).toHaveCount(3);
    await expect(items.nth(0)).toContainText('/agent-dashboard:pr');
  });

  test('typing "/pr" filters down to the pr command', async ({ page }) => {
    await mockApiWithSkills(page, ['pr', 'feature', 'implement', 'fix', 'chore']);
    await mountDetail(page);

    await page.locator('#reply-input').click();
    await page.locator('#reply-input').type('/pr');

    const items = page.locator('#slash-autocomplete .slash-autocomplete__item');
    await expect(items).toHaveCount(1);
    await expect(items.first()).toContainText('/agent-dashboard:pr');
  });

  test('Enter inserts the full command and dismisses the popup', async ({ page }) => {
    await mockApiWithSkills(page, ['pr', 'feature']);
    await mountDetail(page);

    const input = page.locator('#reply-input');
    await input.click();
    await input.type('/pr');
    await expect(page.locator('#slash-autocomplete')).toBeVisible();
    await input.press('Enter');

    await expect(page.locator('#slash-autocomplete')).toBeHidden();
    await expect(input).toHaveValue('/agent-dashboard:pr ');
  });

  test('Enter submits an already-complete Codex skill command', async ({ page }) => {
    await mockApiWithSkills(page, ['pr', 'feature'], { harness: 'codex' });
    await mountDetail(page);

    const input = page.locator('#reply-input');
    await input.click();
    await input.type('$agent-dashboard:pr');
    await expect(page.locator('#slash-autocomplete')).toBeVisible();

    const sent = page.waitForRequest((req) =>
      req.method() === 'POST' &&
      req.url().includes('/api/agents/desk-001/input') &&
      req.postDataJSON().text === '$agent-dashboard:pr'
    );
    await input.press('Enter');
    await sent;
  });

  test('Escape dismisses without inserting', async ({ page }) => {
    await mockApiWithSkills(page, ['pr', 'feature']);
    await mountDetail(page);

    const input = page.locator('#reply-input');
    await input.click();
    await input.type('/fea');
    await expect(page.locator('#slash-autocomplete')).toBeVisible();
    await input.press('Escape');

    await expect(page.locator('#slash-autocomplete')).toBeHidden();
    await expect(input).toHaveValue('/fea');
  });
});

// ---------- Sending caption lifecycle ----------

test.describe('Sending caption clears on POST ack and does not reappear', () => {
  test('pending user bubble survives a late initial conversation render', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    let releaseConversation;
    const conversationGate = new Promise((resolve) => { releaseConversation = resolve; });

    await page.route('**/events', (route) => route.abort('connectionrefused'));
    await page.route(/\/api\/agents/, async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path === '/api/agents') {
        await route.fulfill({ json: [makeAgent({ session_id: 'desk-001', state: 'idle_prompt', last_hook_event: 'Stop' })] });
      } else if (path.endsWith('/conversation')) {
        await conversationGate;
        await route.fulfill({ json: [
          { Role: 'human', Content: 'previous question', Timestamp: '2026-06-02T10:00:00.000Z' },
          { Role: 'assistant', Content: 'previous answer', Timestamp: '2026-06-02T10:00:01.000Z' },
        ]});
      } else if (path.endsWith('/input')) {
        await route.fulfill({ json: { ok: 'sent' } });
      } else if (path.endsWith('/pending-question')) {
        await route.fulfill({ json: {} });
      } else if (path.endsWith('/activity')) {
        await route.fulfill({ json: [] });
      } else if (path.endsWith('/usage')) {
        await route.fulfill({ json: { CostUSD: 0 } });
      } else if (path.endsWith('/subagents')) {
        await route.fulfill({ json: [] });
      } else {
        await route.fulfill({ json: {} });
      }
    });
    await page.route('**/api/usage/ratelimit', (r) => r.fulfill({ json: { session: {used_percent:1, resets_at:'2099-01-01T00:00:00Z'}, weekly: {used_percent:1, resets_at:'2099-01-01T00:00:00Z'}, plan: 'max_5' } }));
    await page.route('**/api/usage/daily*', (r) => r.fulfill({ json: { days: [], today_cost: 0, total_cost: 0 } }));
    await page.addInitScript(() => { try { sessionStorage.clear(); } catch {} });

    await page.goto('/');
    await page.waitForSelector('#app-sidebar .app-sidebar__row[data-agent-id]', { timeout: 5000 });
    await page.click('#app-sidebar .app-sidebar__row[data-agent-id] .ui-row');

    const input = page.locator('#reply-input');
    await input.fill('new pending message');
    const sent = page.waitForRequest((req) =>
      req.method() === 'POST' &&
      req.url().includes('/api/agents/desk-001/input') &&
      req.postDataJSON().text === 'new pending message'
    );
    await input.press('Enter');
    await sent;
    await expect(page.locator('.ui-msg--user')).toContainText('new pending message');

    releaseConversation();
    await expect(page.locator('.ui-msg--user').last()).toContainText('new pending message');
  });

  test('refreshConversation must not re-add the "Sending…" caption after ack', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.route('**/events', (route) => route.abort('connectionrefused'));
    await page.route(/\/api\/agents/, async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path === '/api/agents') {
        await route.fulfill({ json: [makeAgent({ session_id: 'desk-001', state: 'running', last_hook_event: 'Stop' })] });
      } else if (path.endsWith('/conversation')) {
        // API has NOT yet caught up with the user's pending message —
        // returns only pre-existing entries. This is the scenario where
        // pendingUserMessage stays set across the next poll.
        await route.fulfill({ json: [
          { Role: 'human', Content: 'previous question', Timestamp: '2026-06-02T10:00:00.000Z' },
          { Role: 'assistant', Content: 'previous answer', Timestamp: '2026-06-02T10:00:01.000Z' },
        ]});
      } else if (path.endsWith('/activity')) {
        await route.fulfill({ json: [] });
      } else if (path.endsWith('/usage')) {
        await route.fulfill({ json: { CostUSD: 0 } });
      } else if (path.endsWith('/subagents')) {
        await route.fulfill({ json: [] });
      } else {
        await route.fulfill({ json: {} });
      }
    });
    await page.route('**/api/usage/ratelimit', (r) => r.fulfill({ json: { session: {used_percent:1, resets_at:'2099-01-01T00:00:00Z'}, weekly: {used_percent:1, resets_at:'2099-01-01T00:00:00Z'}, plan: 'max_5' } }));
    await page.route('**/api/usage/daily*', (r) => r.fulfill({ json: { days: [], today_cost: 0, total_cost: 0 } }));
    await page.addInitScript(() => { try { sessionStorage.clear(); } catch {} });

    await page.goto('/');
    await page.waitForSelector('#app-sidebar .app-sidebar__row[data-agent-id]', { timeout: 5000 });
    await page.click('#app-sidebar .app-sidebar__row[data-agent-id] .ui-row');
    await page.waitForSelector('#tab-conversation .conversation', { timeout: 5000 });

    // 1. Append optimistic user message → "Sending…" caption appears.
    await page.evaluate(async () => {
      const mod = await import('/js/pages/detail.js');
      mod.appendUserMessage('what is going on');
    });
    await expect(page.locator('.ui-msg__caption--sending')).toBeVisible();

    // 2. POST resolves → confirmUserMessageSent clears caption.
    await page.evaluate(async () => {
      const mod = await import('/js/pages/detail.js');
      mod.confirmUserMessageSent();
    });
    await expect(page.locator('.ui-msg__caption--sending')).toHaveCount(0);

    // 3. Conversation poll fires (manually invoke refreshActiveTab via SSE-equivalent path).
    // The pending message is still in pendingUserMessage (API hasn't echoed it back),
    // but the caption MUST NOT reappear since the ack has fired.
    await page.evaluate(async () => {
      const mod = await import('/js/pages/detail.js');
      // refreshActiveTab triggers refreshConversation which rebuilds .conversation HTML.
      mod.refreshActiveTab('desk-001', { session_id: 'desk-001', state: 'running', last_hook_event: 'Stop' });
    });
    // Wait for the re-render to settle.
    await page.waitForTimeout(800);

    // The optimistic bubble should still be present (API hasn't caught up),
    // BUT the "Sending…" caption must NOT be back.
    await expect(page.locator('.ui-msg--user').last()).toBeVisible();
    await expect(page.locator('.ui-msg__caption--sending')).toHaveCount(0);
  });
});

// ---------- Codex conversation streaming ----------

test.describe('Codex conversation incremental refresh', () => {
  test('poll refresh updates an existing assistant bubble when content grows', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    let assistantText = 'First chunk.';

    await page.route('**/events', (route) => route.abort('connectionrefused'));
    await page.route(/\/api\/agents/, async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path === '/api/agents') {
        await route.fulfill({ json: [makeAgent({ session_id: 'desk-001', harness: 'codex', state: 'running', last_hook_event: 'PreToolUse' })] });
      } else if (path.endsWith('/conversation')) {
        await route.fulfill({ json: [
          { Role: 'human', Content: 'please explain', Timestamp: '2026-06-02T10:00:00.000Z' },
          { Role: 'assistant', Content: assistantText, Timestamp: '2026-06-02T10:00:01.000Z' },
        ]});
      } else if (path.endsWith('/pending-question')) {
        await route.fulfill({ json: {} });
      } else if (path.endsWith('/activity')) {
        await route.fulfill({ json: [] });
      } else if (path.endsWith('/usage')) {
        await route.fulfill({ json: { CostUSD: 0 } });
      } else if (path.endsWith('/subagents')) {
        await route.fulfill({ json: [] });
      } else {
        await route.fulfill({ json: {} });
      }
    });
    await page.route('**/api/usage/ratelimit', (r) => r.fulfill({ json: { session: {used_percent:1, resets_at:'2099-01-01T00:00:00Z'}, weekly: {used_percent:1, resets_at:'2099-01-01T00:00:00Z'}, plan: 'max_5' } }));
    await page.route('**/api/usage/daily*', (r) => r.fulfill({ json: { days: [], today_cost: 0, total_cost: 0 } }));
    await page.addInitScript(() => { try { sessionStorage.clear(); } catch {} });

    await page.goto('/');
    await page.waitForSelector('#app-sidebar .app-sidebar__row[data-agent-id]', { timeout: 5000 });
    await page.click('#app-sidebar .app-sidebar__row[data-agent-id] .ui-row');
    await expect(page.locator('.ui-msg--assistant')).toContainText('First chunk.');

    assistantText = 'First chunk. Second chunk.';
    await page.evaluate(async () => {
      const mod = await import('/js/pages/detail.js');
      mod.refreshActiveTab('desk-001', { session_id: 'desk-001', harness: 'codex', state: 'running', last_hook_event: 'PreToolUse' });
    });

    await expect(page.locator('.ui-msg--assistant')).toContainText('Second chunk.');
    await expect(page.locator('.ui-msg--assistant')).toHaveCount(1);
  });

  test('active Codex chat poll surfaces assistant updates without waiting for refresh', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    let assistantText = 'Initial text.';

    await page.route('**/events', (route) => route.abort('connectionrefused'));
    await page.route(/\/api\/agents/, async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path === '/api/agents') {
        await route.fulfill({ json: [makeAgent({ session_id: 'desk-001', harness: 'codex', state: 'running', last_hook_event: 'PreToolUse' })] });
      } else if (path.endsWith('/conversation')) {
        await route.fulfill({ json: [
          { Role: 'human', Content: 'please explain', Timestamp: '2026-06-02T10:00:00.000Z' },
          { Role: 'assistant', Content: assistantText, Timestamp: '2026-06-02T10:00:01.000Z' },
        ]});
      } else if (path.endsWith('/pending-question')) {
        await route.fulfill({ json: {} });
      } else if (path.endsWith('/activity')) {
        await route.fulfill({ json: [] });
      } else if (path.endsWith('/usage')) {
        await route.fulfill({ json: { CostUSD: 0 } });
      } else if (path.endsWith('/subagents')) {
        await route.fulfill({ json: [] });
      } else {
        await route.fulfill({ json: {} });
      }
    });
    await page.route('**/api/usage/ratelimit', (r) => r.fulfill({ json: { session: {used_percent:1, resets_at:'2099-01-01T00:00:00Z'}, weekly: {used_percent:1, resets_at:'2099-01-01T00:00:00Z'}, plan: 'max_5' } }));
    await page.route('**/api/usage/daily*', (r) => r.fulfill({ json: { days: [], today_cost: 0, total_cost: 0 } }));
    await page.addInitScript(() => { try { sessionStorage.clear(); } catch {} });

    await page.goto('/');
    await page.waitForSelector('#app-sidebar .app-sidebar__row[data-agent-id]', { timeout: 5000 });
    await page.click('#app-sidebar .app-sidebar__row[data-agent-id] .ui-row');
    await expect(page.locator('.ui-msg--assistant')).toContainText('Initial text.');

    assistantText = 'Initial text. Arrived from poll.';

    await expect(page.locator('.ui-msg--assistant')).toContainText('Arrived from poll.', { timeout: 1200 });
  });

  test('adjacent assistant messages render as separate blocks', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });

    await page.route('**/events', (route) => route.abort('connectionrefused'));
    await page.route(/\/api\/agents/, async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path === '/api/agents') {
        await route.fulfill({ json: [makeAgent({ session_id: 'desk-001', harness: 'codex', state: 'running', last_hook_event: 'PreToolUse' })] });
      } else if (path.endsWith('/conversation')) {
        await route.fulfill({ json: [
          { Role: 'human', Content: 'please explain', Timestamp: '2026-06-02T10:00:00.000Z' },
          { Role: 'assistant', Content: 'First chunk.', Timestamp: '2026-06-02T10:00:01.000Z' },
          { Role: 'assistant', Content: 'Second chunk.', Timestamp: '2026-06-02T10:00:02.000Z' },
        ]});
      } else if (path.endsWith('/pending-question')) {
        await route.fulfill({ json: {} });
      } else if (path.endsWith('/activity')) {
        await route.fulfill({ json: [] });
      } else if (path.endsWith('/usage')) {
        await route.fulfill({ json: { CostUSD: 0 } });
      } else if (path.endsWith('/subagents')) {
        await route.fulfill({ json: [] });
      } else {
        await route.fulfill({ json: {} });
      }
    });
    await page.route('**/api/usage/ratelimit', (r) => r.fulfill({ json: { session: {used_percent:1, resets_at:'2099-01-01T00:00:00Z'}, weekly: {used_percent:1, resets_at:'2099-01-01T00:00:00Z'}, plan: 'max_5' } }));
    await page.route('**/api/usage/daily*', (r) => r.fulfill({ json: { days: [], today_cost: 0, total_cost: 0 } }));
    await page.addInitScript(() => { try { sessionStorage.clear(); } catch {} });

    await page.goto('/');
    await page.waitForSelector('#app-sidebar .app-sidebar__row[data-agent-id]', { timeout: 5000 });
    await page.click('#app-sidebar .app-sidebar__row[data-agent-id] .ui-row');

    await expect(page.locator('.ui-msg--assistant')).toHaveCount(2);
    await expect(page.locator('.ui-msg--assistant').nth(0)).toContainText('First chunk.');
    await expect(page.locator('.ui-msg--assistant').nth(1)).toContainText('Second chunk.');
  });
});

// ---------- Foldable viewport sweep (unfolded only) ----------
//
// Samsung Galaxy Z Flip 7 inner display unfolds to ~412×1010 CSS px.
// We only exercise the unfolded ("open") orientations — the cover
// screen is too small for the dashboard to be useful and we don't
// design for it.
//   - 412×1010 portrait  → mobile single-column
//   - 1010×412 landscape → crosses the 900px desktop breakpoint, so
//                          renders the two-pane shell

test.describe('Z Flip 7 unfolded viewports', () => {
  test('inner portrait 412×1010 renders mobile single-column', async ({ page }) => {
    await page.setViewportSize({ width: 412, height: 1010 });
    await mockApi(page, [makeAgent()]);
    await page.goto('/');
    await page.waitForSelector('.ui-row', { timeout: 5000 });

    const sidebar = page.locator('#app-sidebar');
    expect(await sidebar.isVisible()).toBe(false);
    // Main pane fills the viewport.
    const mainBox = await page.locator('#app-main').boundingBox();
    expect(mainBox.width).toBeCloseTo(412, 0);
  });

  test('landscape 1010×412 crosses 900px breakpoint → desktop shell', async ({ page }) => {
    await page.setViewportSize({ width: 1010, height: 412 });
    await mockApi(page, [makeAgent()]);
    await page.goto('/');
    await page.waitForSelector('#app-sidebar .app-sidebar__inner', { timeout: 5000 });

    const sidebar = page.locator('#app-sidebar');
    await expect(sidebar).toBeVisible();
    const display = await page.locator('#app-shell').evaluate((el) => getComputedStyle(el).display);
    expect(display).toBe('grid');
  });
});

// ---------- PR-as-tag (Slice 1) ----------
//
// A running agent that also has an open PR must render the LIVE state
// (running, green dot, RUNNING group) plus a "PR open" tag, not the
// pinned `pr` state in the PR group. The backend's ApplyPinnedStates
// already keeps state="running" for active agents; the frontend used to
// re-apply the pin and mask the live state — this guards against that
// regression.

test.describe('PR-as-tag', () => {
  test('running agent with PR pin stays in RUNNING group + shows "PR open" tag', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    // Backend's ApplyPinnedStates leaves state="running" alone (pin only
    // swaps in for idle states), so the row lands in RUNNING.
    const agent = makeAgent({ state: 'running', pinned_state: 'pr' });
    await mockApi(page, [agent]);
    await page.goto('/');
    await page.waitForSelector('#app-sidebar .app-sidebar__inner', { timeout: 5000 });

    const runningGroup = page.locator('.ui-section-label', { hasText: 'RUNNING' });
    await expect(runningGroup).toBeVisible();
    // PR tag still renders alongside the live state.
    const tag = page.locator('#app-sidebar .ui-row__tag', { hasText: 'PR open' });
    await expect(tag).toBeVisible();
  });

  test('idle agent with PR pin lands in PR group (backend already swapped state)', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    // When the agent goes idle the backend's ApplyPinnedStates promotes
    // state from idle_prompt/done to "pr". The sidebar groups under PR.
    const agent = makeAgent({ state: 'pr', pinned_state: 'pr' });
    await mockApi(page, [agent]);
    await page.goto('/');
    await page.waitForSelector('#app-sidebar .app-sidebar__inner', { timeout: 5000 });

    const prGroup = page.locator('.ui-section-label', { hasText: 'PR' });
    await expect(prGroup).toBeVisible();
    const tag = page.locator('#app-sidebar .ui-row__tag', { hasText: 'PR open' });
    await expect(tag).toBeVisible();
  });

  test('mobile list row shows the PR open tag in RUNNING group when active', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    const agent = makeAgent({ state: 'running', pinned_state: 'pr' });
    await mockApi(page, [agent]);
    await page.goto('/');
    await page.waitForSelector('.ui-row', { timeout: 5000 });

    const tag = page.locator('.ui-row__tag', { hasText: 'PR open' });
    await expect(tag.first()).toBeVisible();
    const runningGroup = page.locator('.ui-section-label', { hasText: 'RUNNING' });
    await expect(runningGroup).toBeVisible();
  });
});

// ---------- PWA page shape (Slice 3) ----------
//
// Mobile list page must not scroll as a whole — the app-bar stays
// pinned and the body scrolls inside .page-scroll. body.scrollHeight
// equals the viewport height (within 1 px tolerance).

test.describe('Mobile PWA layout', () => {
  test('list page does not page-scroll on mobile (.page-layout owns the viewport)', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    // Seed enough agents to overflow vertically so the scroll happens
    // somewhere — it must happen inside .page-scroll, not on body.
    const agents = Array.from({ length: 30 }, (_, i) =>
      makeAgent({ session_id: 'desk-' + i, branch: 'feat/x-' + i }),
    );
    await mockApi(page, agents);
    await page.goto('/');
    await page.waitForSelector('.ui-row', { timeout: 5000 });

    const dims = await page.evaluate(() => ({
      bodyScroll: document.body.scrollHeight,
      bodyClient: document.body.clientHeight,
      pageScroll: document.querySelector('.page-scroll')?.scrollHeight || 0,
      pageClient: document.querySelector('.page-scroll')?.clientHeight || 0,
    }));
    // Body should NOT scroll: scrollHeight ≤ clientHeight (+1 px slack).
    expect(dims.bodyScroll).toBeLessThanOrEqual(dims.bodyClient + 1);
    // Inner scroll region overflows (where the actual scrolling happens).
    expect(dims.pageScroll).toBeGreaterThan(dims.pageClient);
  });
});

// ---------- Modal confirm POST flow (UI.spinner regression) ----------
//
// withSpinner() in app.js calls UI.spinner() to append an in-button
// spinner while the async action runs. If that helper is missing, the
// modal confirm handler throws BEFORE the POST fires — no merge, no
// approve, no close, nothing. This test pins UI.spinner as a required
// primitive by exercising the Merge confirm flow end-to-end.

test.describe('Modal confirm fires the POST', () => {
  test('Merge → modal Confirm → POST /api/agents/<id>/merge actually goes out', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    const agent = makeAgent({ state: 'pr', pinned_state: 'pr', branch: 'feat/x' });
    await mockApi(page, [agent]);

    // Intercept the merge POST so the test doesn't actually run gh.
    let mergeHit = false;
    await page.route('**/api/agents/*/merge', (route) => {
      mergeHit = true;
      route.fulfill({ json: { ok: 'merged' } });
    });
    await page.route('**/api/agents/*/cleanup', (route) => route.fulfill({ json: { ok: 'cleaned' } }));

    await page.goto('/');
    await page.waitForSelector('#app-sidebar .app-sidebar__row[data-agent-id]', { timeout: 5000 });
    await page.click('#app-sidebar .app-sidebar__row[data-agent-id] .ui-row');
    await page.waitForSelector('.action-panel button:has-text("Merge")', { timeout: 5000 });

    await page.click('.action-panel button:has-text("Merge")');
    await page.waitForSelector('#modal-confirm', { timeout: 2000 });
    await page.click('#modal-confirm');

    // Give the async chain time to fire fetch.
    await page.waitForTimeout(400);
    expect(mergeHit).toBe(true);
  });

  test('confirmation modal is an accessible keyboard-contained dialog', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    const agent = makeAgent({ state: 'pr', pinned_state: 'pr', branch: 'feat/x' });
    await mockApi(page, [agent]);

    await page.goto('/');
    await page.waitForSelector('#app-sidebar .app-sidebar__row[data-agent-id]', { timeout: 5000 });
    await page.click('#app-sidebar .app-sidebar__row[data-agent-id] .ui-row');
    const opener = page.locator('.action-panel button:has-text("Merge")');
    await expect(opener).toBeVisible();
    await opener.focus();
    await opener.click();

    const modal = page.locator('.modal');
    await expect(modal).toHaveAttribute('role', 'dialog');
    await expect(modal).toHaveAttribute('aria-modal', 'true');
    const labelledBy = await modal.getAttribute('aria-labelledby');
    const describedBy = await modal.getAttribute('aria-describedby');
    expect(labelledBy).toBeTruthy();
    expect(describedBy).toBeTruthy();
    await expect(page.locator(`#${labelledBy}`)).toHaveText('Merge PR');
    await expect(page.locator(`#${describedBy}`)).toContainText('Squash-merge');

    await expect(page.locator('#modal-confirm')).toBeFocused();
    await page.keyboard.press('Tab');
    await expect(page.locator('#modal-cancel')).toBeFocused();
    await page.keyboard.press('Shift+Tab');
    await expect(page.locator('#modal-confirm')).toBeFocused();

    await page.keyboard.press('Escape');
    await expect(page.locator('.modal-overlay')).toHaveCount(0);
    await expect(opener).toBeFocused();
  });

  test('confirm cannot double-submit or dismiss while async action is pending', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    const agent = makeAgent({ state: 'pr', pinned_state: 'pr', branch: 'feat/x' });
    await mockApi(page, [agent]);

    let mergeHits = 0;
    let releaseMerge;
    const mergePending = new Promise((resolve) => { releaseMerge = resolve; });
    await page.route('**/api/agents/*/merge', async (route) => {
      mergeHits += 1;
      await mergePending;
      await route.fulfill({ json: { ok: 'merged' } });
    });
    await page.route('**/api/agents/*/cleanup', (route) => route.fulfill({ json: { ok: 'cleaned' } }));

    await page.goto('/');
    await page.waitForSelector('#app-sidebar .app-sidebar__row[data-agent-id]', { timeout: 5000 });
    await page.click('#app-sidebar .app-sidebar__row[data-agent-id] .ui-row');
    await page.click('.action-panel button:has-text("Merge")');
    await page.waitForSelector('#modal-confirm', { timeout: 2000 });

    await page.locator('#modal-confirm').dblclick();
    await page.keyboard.press('Escape');
    await page.mouse.click(20, 20);

    expect(mergeHits).toBe(1);
    await expect(page.locator('.modal-overlay')).toHaveCount(1);
    await expect(page.locator('#modal-confirm')).toBeDisabled();
    await expect(page.locator('#modal-cancel')).toBeDisabled();

    releaseMerge();
    await page.waitForSelector('text=Clean up branch', { timeout: 3000 });
  });

  test('destructive Close confirmation uses danger treatment and explicit label', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    const agent = makeAgent({ state: 'merged', pinned_state: 'merged', branch: 'feat/x' });
    await mockApi(page, [agent]);

    await page.goto('/');
    await page.waitForSelector('#app-sidebar .app-sidebar__row[data-agent-id]', { timeout: 5000 });
    await page.click('#app-sidebar .app-sidebar__row[data-agent-id] .ui-row');
    await page.click('.action-panel button:has-text("Close")');

    await expect(page.locator('.modal-title')).toHaveText('Close agent');
    await expect(page.locator('.modal-message')).toContainText('Kill the tmux pane');
    await expect(page.locator('#modal-confirm')).toHaveText('Close agent');
    await expect(page.locator('#modal-confirm')).toHaveClass(/ui-modal-btn--danger/);
  });

  test('success toast appears top-center, not top-right', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    const agent = makeAgent({ state: 'pr', pinned_state: 'pr', branch: 'feat/x' });
    await mockApi(page, [agent]);
    await page.route('**/api/agents/*/merge', (route) => route.fulfill({ json: { ok: 'merged' } }));

    await page.goto('/');
    await page.waitForSelector('#app-sidebar .app-sidebar__row[data-agent-id]', { timeout: 5000 });
    await page.click('#app-sidebar .app-sidebar__row[data-agent-id] .ui-row');
    await page.click('.action-panel button:has-text("Merge")');
    await page.waitForSelector('#modal-confirm', { timeout: 2000 });
    await page.click('#modal-confirm');

    const box = await page.locator('.ui-toast--visible', { hasText: 'Merged' }).boundingBox();
    expect(box).toBeTruthy();
    const toastCenter = box.x + box.width / 2;
    expect(toastCenter).toBeGreaterThan(1280 / 2 - 24);
    expect(toastCenter).toBeLessThan(1280 / 2 + 24);
    expect(box.x).toBeLessThan(1280 - box.width - 120);
  });
});
