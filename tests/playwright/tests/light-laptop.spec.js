// @ts-check
// Phase D: light-mode laptop (~1440px) iteration.
//
// Locks the behavioural invariants the iteration-0 grader identified for
// the desktop light surface (see docs/design/verdict-light-laptop-0.md):
//
//   1. The composer card (.ui-composer.detail-composer) at >= 900px must
//      NOT carry the dark-register `rgba(0,0,0,0.35)` 12px shadow when the
//      page is in light mode. Light uses a single shadow token —
//      --shadow-medium (rgba(0,0,0,0.06)) — applied softly to lifted
//      surfaces only (register.md "Elevation rules (light)").
//
//   2. The composer card border in light at >= 900px must resolve to
//      --border-default (#E4E4E7), not --border-subtle (#E8E8EB). Light's
//      card separation is driven by the 1px border (register.md line 219)
//      since the surface-color step is too small to read on its own.
//
//   3. The sidebar selected row in light at >= 900px must carry a 2px
//      left-border accent in var(--text-primary). The plan explicitly
//      calls for "combine fill + 1px left-border accent" because the
//      light fill step (#E4E4E7 on #F4F4F5) is below the perceptual
//      contrast threshold on its own.
//
// Dark mode at >= 900px is bit-exact preserved — the dark sidebar uses
// the inverted darkness-step (selected `#222226` > sidebar `#0E0E10`)
// which reads strongly without a left-accent; the dark composer keeps
// its heavy lift. We assert that here too for the preservation contract.

const { test, expect } = require('@playwright/test');

test.use({ viewport: { width: 1440, height: 900 } });

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
            session_id: 'll-001',
            cwd: '/Users/test/Code/myapp',
            branch: 'feat/light-laptop',
            model: 'opus',
            state: 'running',
            started_at: new Date().toISOString(),
            subagent_count: 0,
          },
          {
            session_id: 'll-002',
            cwd: '/Users/test/Code/other',
            branch: 'feat/other',
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
          { Role: 'human', Content: 'Sample user prompt.', Timestamp: new Date().toISOString() },
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

async function gotoLightLaptop(page) {
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

async function gotoDarkLaptop(page) {
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

// ---------- 1. Composer card shadow is soft in light ----------

test.describe('Laptop light theme — composer card lift', () => {
  test('detail composer at >= 900px does NOT carry the dark 0.35 shadow in light', async ({ page }) => {
    await gotoLightLaptop(page);
    // Render a synthetic detail composer so we lock the CSS rule directly
    // — the router has timing variance and we only care about the cascade.
    await page.evaluate(() => {
      const wrap = document.createElement('div');
      wrap.className = 'ui-composer detail-composer';
      wrap.id = '__test_composer__';
      wrap.style.position = 'fixed';
      wrap.style.top = '0';
      wrap.style.left = '0';
      wrap.style.width = '600px';
      document.body.appendChild(wrap);
    });
    const composer = page.locator('#__test_composer__');
    const shadow = await composer.evaluate((el) => getComputedStyle(el).boxShadow);
    // Must not contain the dark register's heavy charcoal lift.
    expect(shadow.toLowerCase()).not.toContain('0.35');
    // Must reference shadow-medium (rgba(0,0,0,0.06)) — accept either
    // `0.06` literal or `rgba(0, 0, 0, 0.06)`.
    expect(shadow.toLowerCase()).toMatch(/rgba\(0,\s*0,\s*0,\s*0\.06\)/);
  });

  test('detail composer at >= 900px in light has border-default, not border-subtle', async ({ page }) => {
    await gotoLightLaptop(page);
    await page.evaluate(() => {
      const wrap = document.createElement('div');
      wrap.className = 'ui-composer detail-composer';
      wrap.id = '__test_composer_border__';
      wrap.style.position = 'fixed';
      wrap.style.top = '0';
      wrap.style.left = '0';
      wrap.style.width = '600px';
      document.body.appendChild(wrap);
    });
    const composer = page.locator('#__test_composer_border__');
    const borderColor = await composer.evaluate((el) => getComputedStyle(el).borderTopColor);
    // --border-default in light = #E4E4E7 → rgb(228, 228, 231).
    expect(norm(borderColor)).toBe(norm('rgb(228, 228, 231)'));
    // Negative: must NOT be --border-subtle (#E8E8EB → rgb(232, 232, 235)).
    expect(norm(borderColor)).not.toBe(norm('rgb(232, 232, 235)'));
  });
});

// ---------- 2. Sidebar selected row has the left-accent in light ----------

test.describe('Laptop light theme — sidebar selected accent', () => {
  test('selected sidebar row in light at >= 900px carries a left-border accent in --text-primary', async ({ page }) => {
    await gotoLightLaptop(page);
    // Inject a selected-row marker so we lock the rule, not the data path.
    await page.evaluate(() => {
      const host = document.getElementById('app-sidebar');
      if (!host) throw new Error('no #app-sidebar');
      // Force-unhide just in case the skeleton path left it hidden.
      host.hidden = false;
      host.innerHTML = '';
      const inner = document.createElement('div');
      inner.className = 'app-sidebar__inner';
      const row = document.createElement('div');
      row.className = 'app-sidebar__row app-sidebar__row--selected';
      const btn = document.createElement('button');
      btn.className = 'ui-row';
      btn.id = '__test_sel_row__';
      btn.textContent = 'Selected probe';
      row.appendChild(btn);
      inner.appendChild(row);
      host.appendChild(inner);
    });
    const row = page.locator('#__test_sel_row__');
    const borderLeftWidth = await row.evaluate((el) => getComputedStyle(el).borderLeftWidth);
    const borderLeftColor = await row.evaluate((el) => getComputedStyle(el).borderLeftColor);
    // 2px accent in --text-primary (#1C1A17 → rgb(28, 26, 23)).
    expect(borderLeftWidth).toBe('2px');
    expect(norm(borderLeftColor)).toBe(norm('rgb(28, 26, 23)'));
  });

  test('selected sidebar row in dark at >= 900px has NO left-border accent (preservation)', async ({ page }) => {
    await gotoDarkLaptop(page);
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
      btn.id = '__test_sel_row_dark__';
      btn.textContent = 'Selected probe (dark)';
      row.appendChild(btn);
      inner.appendChild(row);
      host.appendChild(inner);
    });
    const row = page.locator('#__test_sel_row_dark__');
    const borderLeftWidth = await row.evaluate((el) => getComputedStyle(el).borderLeftWidth);
    // Dark mode keeps fill-only selection; left-border-width is 0px.
    expect(borderLeftWidth).toBe('0px');
  });
});

// ---------- 3. Dark composer preserved (preservation contract) ----------

test.describe('Laptop dark theme — composer preservation', () => {
  test('detail composer at >= 900px in dark KEEPS its heavy 0.35 lift', async ({ page }) => {
    await gotoDarkLaptop(page);
    await page.evaluate(() => {
      const wrap = document.createElement('div');
      wrap.className = 'ui-composer detail-composer';
      wrap.id = '__test_composer_dark__';
      wrap.style.position = 'fixed';
      wrap.style.top = '0';
      wrap.style.left = '0';
      wrap.style.width = '600px';
      document.body.appendChild(wrap);
    });
    const composer = page.locator('#__test_composer_dark__');
    const shadow = await composer.evaluate((el) => getComputedStyle(el).boxShadow);
    // Dark register: 0 12px 32px rgba(0,0,0,0.35) is preserved verbatim.
    expect(shadow.toLowerCase()).toContain('0.35');
  });
});
