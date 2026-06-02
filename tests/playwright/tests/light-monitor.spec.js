// @ts-check
// Phase E: light-mode monitor (>= 1600px) iteration.
//
// Locks the monitor-light invariants the iteration-0 grader will hold
// against (docs/design/light-flow-map.md "Monitor light"):
//
//   1. The detail-view chat + composer at >= 1600px must cap at
//      --reading-max-monitor (1080px), centred. The light-flow-map
//      explicitly says "The cap applies to transcript + composer +
//      create-view content; the page chrome (app bar, tabs) still spans
//      the full main-pane width." Phase B only capped .page-scroll /
//      .page-pinned (list + usage views); detail uses .action-bar +
//      .conversation which were not capped. Phase E closes that gap.
//
//   2. The sidebar at >= 1600px widens to 288px in BOTH themes; the
//      sidebar bg in light continues to resolve to var(--bg-sidebar)
//      (#F4F4F5) at this viewport (no regression of the laptop light
//      sidebar bg rule when the monitor breakpoint fires).
//
//   3. Sidebar agent rows at the wider 288px must still ellipsize long
//      labels — no overflow, no wrap, no orphaning. The wider column
//      buys ~24px more glyph room but the truncation rule itself is
//      unchanged.
//
//   4. The Phase D laptop light-mode polish (composer shadow-medium
//      0.06, composer border-default, sidebar selected left-accent)
//      must continue to resolve at >= 1600px light. Phase E does not
//      relax the laptop register at the wider tier.
//
// Dark mode at 1920×1080 is preservation-only: we assert the sidebar
// column also widens to 288px (geometry token is theme-agnostic) so
// the monitor breakpoint stays uniform across themes.

const { test, expect } = require('@playwright/test');

test.use({ viewport: { width: 1920, height: 1080 } });

// ---------- helpers ----------

async function mockApi(page) {
  await page.route('**/events', (route) => route.abort('connectionrefused'));
  await page.route(/\/api\/agents/, async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    if (path === '/api/agents') {
      await route.fulfill({
        json: [
          {
            session_id: 'lm-001',
            cwd: '/Users/test/Code/myapp',
            branch: 'feat/light-monitor',
            model: 'opus',
            state: 'running',
            started_at: new Date().toISOString(),
            subagent_count: 0,
          },
          {
            session_id: 'lm-002-very-long-name-that-should-truncate-cleanly',
            cwd: '/Users/test/Code/something-with-an-unusually-long-branch-name',
            branch: 'feat/refactor-the-entire-conversation-router-and-history',
            model: 'opus',
            state: 'question',
            started_at: new Date().toISOString(),
            subagent_count: 0,
          },
        ],
      });
    } else if (path.endsWith('/conversation')) {
      await route.fulfill({
        json: [
          { Role: 'human', Content: 'Sample user prompt at monitor width.', Timestamp: new Date().toISOString() },
          { Role: 'assistant', Content: 'Sample reply.', Timestamp: new Date().toISOString() },
        ],
      });
    } else {
      await route.fulfill({ json: {} });
    }
  });
  await page.route('**/api/usage/ratelimit', (route) => route.fulfill({ json: {} }));
  await page.route('**/api/usage/daily*', (route) => route.fulfill({ json: { days: [] } }));
  await page.addInitScript(() => {
    try { sessionStorage.clear(); } catch {}
  });
}

async function gotoLightMonitor(page) {
  await mockApi(page);
  await page.goto('/');
  await page.waitForSelector('#app-shell', { timeout: 5000 });
  await page.evaluate(async () => {
    const { Theme } = await import('/js/theme.js');
    localStorage.setItem('theme-preference', 'light');
    Theme.apply('light');
  });
  const theme = await page.evaluate(() => document.documentElement.dataset.theme);
  expect(theme).toBe('light');
}

async function gotoDarkMonitor(page) {
  await mockApi(page);
  await page.goto('/');
  await page.waitForSelector('#app-shell', { timeout: 5000 });
  await page.evaluate(async () => {
    const { Theme } = await import('/js/theme.js');
    localStorage.setItem('theme-preference', 'dark');
    Theme.apply('dark');
  });
  const theme = await page.evaluate(() => document.documentElement.dataset.theme);
  expect(theme).toBe('dark');
}

function norm(s) {
  return (s || '').replace(/\s+/g, '').toLowerCase();
}

// ---------- 1. Detail-view composer + transcript cap at reading-max-monitor ----------

test.describe('Monitor light — detail view caps at reading-max', () => {
  test('synthetic .action-bar + composer at 1920 light is centred and <= 1080px wide', async ({ page }) => {
    await gotoLightMonitor(page);

    // Synthesize a detail layout inside #app-main so the monitor cap
    // applies even when the live router hasn't yet loaded a session.
    await page.evaluate(() => {
      const main = document.getElementById('app-main');
      if (!main) throw new Error('no #app-main');
      main.innerHTML = '';
      const layout = document.createElement('div');
      layout.className = 'detail-layout';
      const action = document.createElement('div');
      action.className = 'action-bar';
      action.id = '__test_action_bar__';
      const composer = document.createElement('div');
      composer.className = 'ui-composer detail-composer';
      composer.id = '__test_detail_composer__';
      action.appendChild(composer);
      layout.appendChild(action);
      main.appendChild(layout);
    });

    const actionBar = page.locator('#__test_action_bar__');
    const actionRect = await actionBar.evaluate((el) => el.getBoundingClientRect().toJSON());
    // Action-bar should be capped at reading-max-monitor (1080px) at >= 1600px.
    expect(actionRect.width).toBeLessThanOrEqual(1080);
    // And it should be centred inside the 1632px main pane (1920 - 288 sidebar).
    // Tolerate sub-pixel rounding within 4px.
    const mainRect = await page.locator('#app-main').evaluate((el) => el.getBoundingClientRect().toJSON());
    const expectedLeft = mainRect.left + (mainRect.width - actionRect.width) / 2;
    expect(Math.abs(actionRect.left - expectedLeft)).toBeLessThan(4);
  });

  test('synthetic .conversation transcript at 1920 light caps at <= 1080px wide and is centred', async ({ page }) => {
    await gotoLightMonitor(page);

    await page.evaluate(() => {
      const main = document.getElementById('app-main');
      if (!main) throw new Error('no #app-main');
      main.innerHTML = '';
      const layout = document.createElement('div');
      layout.className = 'detail-layout';
      const pinned = document.createElement('div');
      pinned.className = 'detail-pinned';
      const tabContent = document.createElement('div');
      tabContent.className = 'tab-content active';
      const conv = document.createElement('div');
      conv.className = 'conversation';
      conv.id = '__test_conv__';
      conv.textContent = 'Sample transcript content for measurement.';
      tabContent.appendChild(conv);
      pinned.appendChild(tabContent);
      layout.appendChild(pinned);
      main.appendChild(layout);
    });

    const conv = page.locator('#__test_conv__');
    const rect = await conv.evaluate((el) => el.getBoundingClientRect().toJSON());
    expect(rect.width).toBeLessThanOrEqual(1080);
    const mainRect = await page.locator('#app-main').evaluate((el) => el.getBoundingClientRect().toJSON());
    const expectedLeft = mainRect.left + (mainRect.width - rect.width) / 2;
    expect(Math.abs(rect.left - expectedLeft)).toBeLessThan(4);
  });

  test('the detail-tabs row at 1920 light spans the full main pane (chrome is not capped)', async ({ page }) => {
    await gotoLightMonitor(page);

    await page.evaluate(() => {
      const main = document.getElementById('app-main');
      if (!main) throw new Error('no #app-main');
      main.innerHTML = '';
      const layout = document.createElement('div');
      layout.className = 'detail-layout';
      const pinned = document.createElement('div');
      pinned.className = 'detail-pinned';
      const tabs = document.createElement('div');
      tabs.className = 'detail-tabs';
      tabs.id = '__test_tabs__';
      tabs.style.minHeight = '32px';
      pinned.appendChild(tabs);
      layout.appendChild(pinned);
      main.appendChild(layout);
    });

    const tabsRect = await page.locator('#__test_tabs__').evaluate((el) => el.getBoundingClientRect().toJSON());
    const mainRect = await page.locator('#app-main').evaluate((el) => el.getBoundingClientRect().toJSON());
    // Tabs (chrome) should be wider than the reading-max cap — they
    // span the full main pane, NOT the 1080 reading column.
    expect(tabsRect.width).toBeGreaterThan(1080);
    expect(Math.abs(tabsRect.width - mainRect.width)).toBeLessThan(4);
  });
});

// ---------- 2. Sidebar geometry & light-mode bg at monitor ----------

test.describe('Monitor light — sidebar geometry preserved', () => {
  test('sidebar column resolves to 288px in light at 1920', async ({ page }) => {
    await gotoLightMonitor(page);
    const cols = await page.locator('#app-shell').evaluate((el) => getComputedStyle(el).gridTemplateColumns);
    const parts = cols.split(/\s+/);
    expect(parts.length).toBe(2);
    expect(parseFloat(parts[0])).toBeCloseTo(288, 0);
  });

  test('sidebar bg in light at 1920 resolves to #F4F4F5', async ({ page }) => {
    await gotoLightMonitor(page);
    const bg = await page.locator('#app-sidebar').evaluate((el) => getComputedStyle(el).backgroundColor);
    expect(norm(bg)).toBe(norm('rgb(244, 244, 245)'));
  });

  test('sidebar column also widens to 288px in dark at 1920 (geometry is theme-agnostic)', async ({ page }) => {
    await gotoDarkMonitor(page);
    const cols = await page.locator('#app-shell').evaluate((el) => getComputedStyle(el).gridTemplateColumns);
    const parts = cols.split(/\s+/);
    expect(parseFloat(parts[0])).toBeCloseTo(288, 0);
  });
});

// ---------- 3. Sidebar row truncation at the wider 288px ----------

test.describe('Monitor light — sidebar rows do not orphan or wrap', () => {
  test('a long agent-name row at 1920 light stays single-line and is truncated by ellipsis', async ({ page }) => {
    await gotoLightMonitor(page);

    // Force-seed the sidebar with a long-name row using the real
    // .ui-row primitive structure (ui.js wraps title in .ui-row__title
    // which owns the nowrap+ellipsis cascade).
    await page.evaluate(() => {
      const host = document.getElementById('app-sidebar');
      if (!host) throw new Error('no #app-sidebar');
      host.hidden = false;
      host.innerHTML = '';
      const inner = document.createElement('div');
      inner.className = 'app-sidebar__inner';
      const row = document.createElement('div');
      row.className = 'app-sidebar__row';
      row.id = '__test_long_row__';
      const btn = document.createElement('button');
      btn.className = 'ui-row';
      const body = document.createElement('span');
      body.className = 'ui-row__body';
      const titleLine = document.createElement('span');
      titleLine.className = 'ui-row__title-line';
      const title = document.createElement('span');
      title.className = 'ui-row__title';
      title.id = '__test_long_title__';
      // 80-char label — well beyond 288px sidebar width.
      title.textContent = 'feat-refactor-conversation-router-and-history-with-additional-trailing-context';
      titleLine.appendChild(title);
      body.appendChild(titleLine);
      btn.appendChild(body);
      row.appendChild(btn);
      inner.appendChild(row);
      host.appendChild(inner);
    });

    const titleInfo = await page.locator('#__test_long_title__').evaluate((el) => {
      const cs = getComputedStyle(el);
      const rect = el.getBoundingClientRect();
      return {
        whiteSpace: cs.whiteSpace,
        overflow: cs.overflow,
        textOverflow: cs.textOverflow,
        clientWidth: el.clientWidth,
        scrollWidth: el.scrollWidth,
        height: rect.height,
        width: rect.width,
      };
    });
    // Title owns the ellipsis contract.
    expect(titleInfo.whiteSpace).toBe('nowrap');
    expect(titleInfo.textOverflow).toBe('ellipsis');
    // Title text overflowed the available space → ellipsis is engaged.
    expect(titleInfo.scrollWidth).toBeGreaterThan(titleInfo.clientWidth);
    // Sidebar row stays single-line at 288px — the row's bounding box
    // height should be the .ui-row min-height (~60px), never wrapping
    // into a multi-line layout.
    const rowRect = await page.locator('#__test_long_row__').evaluate((el) => el.getBoundingClientRect().toJSON());
    expect(rowRect.height).toBeLessThanOrEqual(64);
    // Title width is bounded by the sidebar column minus the row's
    // padding (16 + 20 = 36px); 252px is a generous upper bound.
    expect(titleInfo.width).toBeLessThan(260);
  });
});

// ---------- 4. Phase D polish still resolves at >= 1600px light ----------

test.describe('Monitor light — Phase D laptop polish carries through', () => {
  test('detail composer at 1920 light still has shadow-medium (0.06) and border-default', async ({ page }) => {
    await gotoLightMonitor(page);
    await page.evaluate(() => {
      const wrap = document.createElement('div');
      wrap.className = 'ui-composer detail-composer';
      wrap.id = '__test_composer_monitor__';
      wrap.style.position = 'fixed';
      wrap.style.top = '0';
      wrap.style.left = '0';
      wrap.style.width = '600px';
      document.body.appendChild(wrap);
    });
    const composer = page.locator('#__test_composer_monitor__');
    const shadow = await composer.evaluate((el) => getComputedStyle(el).boxShadow);
    expect(shadow.toLowerCase()).not.toContain('0.35');
    expect(shadow.toLowerCase()).toMatch(/rgba\(0,\s*0,\s*0,\s*0\.06\)/);
    const borderColor = await composer.evaluate((el) => getComputedStyle(el).borderTopColor);
    expect(norm(borderColor)).toBe(norm('rgb(228, 228, 231)'));
  });

  test('selected sidebar row at 1920 light still carries the 2px text-primary left-accent', async ({ page }) => {
    await gotoLightMonitor(page);
    await page.evaluate(() => {
      const host = document.getElementById('app-sidebar');
      host.hidden = false;
      host.innerHTML = '';
      const inner = document.createElement('div');
      inner.className = 'app-sidebar__inner';
      const row = document.createElement('div');
      row.className = 'app-sidebar__row app-sidebar__row--selected';
      const btn = document.createElement('button');
      btn.className = 'ui-row';
      btn.id = '__test_sel_monitor__';
      btn.textContent = 'Selected probe';
      row.appendChild(btn);
      inner.appendChild(row);
      host.appendChild(inner);
    });
    const row = page.locator('#__test_sel_monitor__');
    const borderLeftWidth = await row.evaluate((el) => getComputedStyle(el).borderLeftWidth);
    const borderLeftColor = await row.evaluate((el) => getComputedStyle(el).borderLeftColor);
    expect(borderLeftWidth).toBe('2px');
    expect(norm(borderLeftColor)).toBe(norm('rgb(28, 26, 23)'));
  });
});
