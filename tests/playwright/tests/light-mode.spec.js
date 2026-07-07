// @ts-check
// Phase F: light-mode regression at three widths.
//
// This spec is the cross-cutting backstop for the per-phase light-mode
// specs (light-foundation, light-mobile, light-laptop, light-monitor).
// Those specs each lock one slice of the register; this spec asserts the
// register invariants that span ALL three widths plus a flow smoke test
// at each width so we know list → detail → create → usage actually
// completes in light mode without surfacing console errors.
//
// Width grid:
//   - phone   412×900
//   - laptop  1440×900
//   - monitor 1920×1080
//
// Invariants asserted at every width (unless noted otherwise):
//   1. data-theme="light" cascade lands and persists.
//   2. --bg-base resolves to #FFFFFF (the entire app surface in light).
//   3. --text-primary resolves to #1C1A17 (rgb(28, 26, 23)).
//   4. document.body backgroundColor is pure white (#FFFFFF) — confirms
//      the cascade actually paints with the token, not just declares it.
//   5. Sidebar bg is #F4F4F5 at laptop+monitor; at phone the sidebar is
//      hidden, so we assert visibility, not bg.
//   6. list → detail → create → usage navigates without throwing a
//      console error. The per-phase specs assert static invariants on
//      the entry page; only this spec walks the views.
//
// Out of scope (per phase brief):
//   - Visual snapshots (.toHaveScreenshot). Project has no snapshot
//     baseline today; adding one bundles a workflow into this PR.
//   - Anything that would consolidate or replace the per-phase specs.

const { test, expect } = require('@playwright/test');

// ---------- shared helpers ----------

function makeAgent(overrides) {
  return {
    session_id: 'lf-001',
    cwd: '/Users/test/Code/myapp',
    branch: 'feat/light-flow',
    model: 'opus',
    state: 'running',
    started_at: new Date().toISOString(),
    subagent_count: 0,
    ...overrides,
  };
}

async function mockApi(page, agents) {
  await page.route('**/events', (route) => route.abort('connectionrefused'));
  await page.route(/\/api\/agents/, async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    if (path === '/api/agents') {
      await route.fulfill({ json: agents });
    } else if (path.endsWith('/conversation')) {
      await route.fulfill({
        json: [
          { Role: 'human', Content: 'Hello', Timestamp: new Date().toISOString() },
          { Role: 'assistant', Content: 'Hi', Timestamp: new Date().toISOString() },
        ],
      });
    } else if (path.endsWith('/usage')) {
      await route.fulfill({ json: { InputTokens: 1000, OutputTokens: 500, CostUSD: 0.05 } });
    } else if (path.endsWith('/subagents')) {
      await route.fulfill({ json: [] });
    } else if (path.endsWith('/activity')) {
      await route.fulfill({ json: [] });
    } else {
      await route.fulfill({ json: {} });
    }
  });
  await page.route('**/api/usage/ratelimit', (route) => route.fulfill({
    json: {
      session: { used_percent: 10, resets_at: '2099-01-01T00:00:00Z' },
      weekly: { used_percent: 5, resets_at: '2099-01-01T00:00:00Z' },
      plan: 'max_5',
    },
  }));
  await page.route('**/api/usage/daily*', (route) => route.fulfill({
    json: { days: [], today_cost: 0, total_cost: 0 },
  }));
  await page.route('**/api/skills', (route) => route.fulfill({ json: [] }));
  await page.route('**/api/harness-options', (route) => route.fulfill({
    json: {
      models: ['opus'],
      efforts: ['high'],
      default_model: { model: 'opus', source: '~/.claude/settings.json' },
      default_effort: { effort: 'high', source: 'agent-dashboard settings' },
    },
  }));
  await page.addInitScript(() => {
    try { sessionStorage.clear(); } catch {}
  });
}

async function applyLight(page) {
  await page.evaluate(async () => {
    const { Theme } = await import('/js/theme.js');
    localStorage.setItem('theme-preference', 'light');
    Theme.apply('light');
  });
  const theme = await page.evaluate(() => document.documentElement.dataset.theme);
  expect(theme).toBe('light');
}

async function cssVar(page, name) {
  return await page.evaluate((n) => {
    return getComputedStyle(document.documentElement).getPropertyValue(n).trim();
  }, name);
}

function norm(s) {
  return (s || '').replace(/\s+/g, '').toLowerCase();
}

// Run every width through this single body. Each describe() pre-sets its
// viewport and passes a `sidebarHidden` hint so we can branch on the
// one true asymmetry (mobile hides the sidebar).
function lightModeSuite({ label, width, height, sidebarHidden }) {
  test.describe(`Light mode @ ${label} (${width}x${height})`, () => {
    test.use({ viewport: { width, height } });

    // Per-test console-error capture. Each test creates its own listener,
    // does its work, then asserts zero errors. We intentionally do NOT
    // share a top-level listener because Playwright re-uses the page
    // across tests in a single describe and stale errors would bleed.

    test('--bg-base resolves to #FFFFFF and document.body paints white', async ({ page }) => {
      await mockApi(page, [makeAgent()]);
      await page.goto('/');
      await page.waitForSelector('#app-shell', { timeout: 5000 });
      await applyLight(page);

      const bgBase = await cssVar(page, '--bg-base');
      expect(bgBase.toUpperCase()).toBe('#FFFFFF');

      // Body background paints with the token in light mode.
      const bodyBg = await page.evaluate(() => getComputedStyle(document.body).backgroundColor);
      expect(norm(bodyBg)).toBe(norm('rgb(255, 255, 255)'));
    });

    test('--text-primary resolves to #1C1A17 (rgb(28, 26, 23))', async ({ page }) => {
      await mockApi(page, [makeAgent()]);
      await page.goto('/');
      await page.waitForSelector('#app-shell', { timeout: 5000 });
      await applyLight(page);

      const textPrimary = await cssVar(page, '--text-primary');
      expect(textPrimary.toUpperCase()).toBe('#1C1A17');

      // And the rendered cascade really lands on rgb(28, 26, 23) when an
      // element consumes --text-primary. We synthesize a probe so we
      // don't depend on which body content has rendered.
      const probeColor = await page.evaluate(() => {
        const probe = document.createElement('span');
        probe.style.color = 'var(--text-primary)';
        probe.textContent = 'probe';
        document.body.appendChild(probe);
        const c = getComputedStyle(probe).color;
        probe.remove();
        return c;
      });
      expect(norm(probeColor)).toBe(norm('rgb(28, 26, 23)'));
    });

    if (sidebarHidden) {
      test('sidebar is hidden at phone width (no sidebar-bg surface visible)', async ({ page }) => {
        await mockApi(page, [makeAgent()]);
        await page.goto('/');
        await page.waitForSelector('.ui-row', { timeout: 5000 });
        await applyLight(page);

        const sidebar = page.locator('#app-sidebar');
        expect(await sidebar.isVisible()).toBe(false);
      });
    } else {
      test('sidebar background resolves to #F4F4F5 in light', async ({ page }) => {
        await mockApi(page, [makeAgent()]);
        await page.goto('/');
        await page.waitForSelector('#app-sidebar .app-sidebar__inner', { timeout: 5000 });
        await applyLight(page);

        const sidebarBg = await page.locator('#app-sidebar').evaluate((el) => getComputedStyle(el).backgroundColor);
        // #F4F4F5 → rgb(244, 244, 245)
        expect(norm(sidebarBg)).toBe(norm('rgb(244, 244, 245)'));
      });
    }

    test('list → detail → create → usage flow completes without console errors', async ({ page }) => {
      const errors = [];
      page.on('console', (msg) => {
        if (msg.type() === 'error') errors.push(msg.text());
      });
      page.on('pageerror', (err) => {
        errors.push('pageerror: ' + (err && err.message ? err.message : String(err)));
      });

      await mockApi(page, [makeAgent()]);
      await page.goto('/');

      if (sidebarHidden) {
        // Mobile: list view drives render through shared UI.row markup.
        await page.waitForSelector('.page-scroll .ui-row', { timeout: 5000 });
      } else {
        await page.waitForSelector('#app-shell', { timeout: 5000 });
        await page.waitForSelector('#app-sidebar .app-sidebar__row[data-agent-id]', { timeout: 5000 });
      }
      await applyLight(page);

      // --- list → detail ---
      // Drive selection through the same Dashboard.selectAgent() entry
      // point the sidebar and list rows use. This decouples the test
      // from per-width markup (sidebar uses .app-sidebar__row, mobile
      // list uses a flatter .ui-row tree) while still exercising the
      // real router.
      await page.evaluate(() => {
        if (typeof window.Dashboard === 'undefined' || typeof window.Dashboard.selectAgent !== 'function') {
          throw new Error('Dashboard.selectAgent is not exposed on window');
        }
        window.Dashboard.selectAgent('lf-001');
      });
      await page.waitForSelector('#app-main .detail-layout, .detail-layout', { timeout: 5000 });

      // --- detail → create ---
      // The dashboard exposes a Dashboard.showCreate() global wired into
      // the "New agent" CTA. We invoke it through the same surface the
      // sidebar uses, so this exercises the real router path.
      await page.evaluate(() => {
        if (typeof window.Dashboard === 'undefined' || typeof window.Dashboard.showCreate !== 'function') {
          throw new Error('Dashboard.showCreate is not exposed on window');
        }
        window.Dashboard.showCreate();
      });
      // Create view renders a header titled "New agent" — it's the most
      // stable selector that doesn't depend on form field naming.
      await page.waitForFunction(
        () => document.body.innerText.includes('New agent'),
        null,
        { timeout: 5000 },
      );
      await expect(page.locator('#create-model-hint')).toContainText('Default: opus');
      await expect(page.locator('#create-effort-hint')).toContainText('Default: high');
      const hintColors = await page.locator('#create-model-hint, #create-effort-hint').evaluateAll((nodes) => nodes.map((n) => getComputedStyle(n).color));
      expect(hintColors).toEqual(['rgb(113, 113, 122)', 'rgb(113, 113, 122)']);

      // --- create → usage ---
      await page.evaluate(() => {
        if (typeof window.Dashboard.showUsage !== 'function') {
          throw new Error('Dashboard.showUsage is not exposed on window');
        }
        window.Dashboard.showUsage();
      });
      await page.waitForSelector('.usage-rate__rows, .usage-card, .usage-bar', { timeout: 5000 });

      // No console errors accumulated across the flow. Filter out
      // benign network-aborted noise from the SSE block (we mock
      // /events to connectionrefused above) — those surface as
      // "Failed to load resource" / "net::ERR_*" lines and don't
      // indicate a real regression.
      const real = errors.filter((m) => {
        const t = m.toLowerCase();
        return !t.includes('failed to load resource')
          && !t.includes('net::err_')
          && !t.includes('events');
      });
      expect(real, `unexpected console errors during light-mode flow:\n${real.join('\n')}`).toEqual([]);
    });
  });
}

lightModeSuite({ label: 'phone',   width: 412,  height: 900,  sidebarHidden: true });
lightModeSuite({ label: 'laptop',  width: 1440, height: 900,  sidebarHidden: false });
lightModeSuite({ label: 'monitor', width: 1920, height: 1080, sidebarHidden: false });
