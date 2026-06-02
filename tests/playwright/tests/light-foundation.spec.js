// @ts-check
// Phase B: token foundation + monitor breakpoint.
//
// Locks invariants from the Phase A register/desktop-register contracts:
//   - Monitor-tier (≥1600px) tokens declared on :root and consumed by the
//     `@media (min-width: 1600px)` block: sidebar widens to 288px, content
//     caps at 1080px, page padding bumps to 32px.
//   - Meta theme-color reflects the actual surface in each mode
//     (#FFFFFF in light, #0B0E14 in dark). Previously light shipped a
//     warm-gray (#2A2724) that didn't match the rendered surface.
//   - Light --shadow-medium matches the register addendum value 0.06,
//     not the legacy 0.08.

const { test, expect } = require('@playwright/test');

async function mockApi(page) {
  await page.route('**/events', (route) => route.abort('connectionrefused'));
  await page.route(/\/api\/agents/, async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    if (path === '/api/agents') {
      await route.fulfill({ json: [] });
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

async function cssVar(page, name) {
  return await page.evaluate((n) => {
    return getComputedStyle(document.documentElement).getPropertyValue(n).trim();
  }, name);
}

function norm(s) {
  return (s || '').replace(/\s+/g, '').toLowerCase();
}

// ---------- Monitor-tier tokens (declared on :root) ----------

test.describe('Monitor-tier tokens declared on :root', () => {
  test('--sidebar-width-monitor is 288px', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await mockApi(page);
    await page.goto('/');
    await page.waitForSelector('#app-shell', { timeout: 5000 });

    const val = await cssVar(page, '--sidebar-width-monitor');
    expect(val).toBe('288px');
  });

  test('--reading-max-monitor is 1080px', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await mockApi(page);
    await page.goto('/');
    await page.waitForSelector('#app-shell', { timeout: 5000 });

    const val = await cssVar(page, '--reading-max-monitor');
    expect(val).toBe('1080px');
  });

  test('--page-padding-monitor is 32px', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await mockApi(page);
    await page.goto('/');
    await page.waitForSelector('#app-shell', { timeout: 5000 });

    const val = await cssVar(page, '--page-padding-monitor');
    expect(val).toBe('32px');
  });
});

// ---------- Monitor breakpoint behaviour (≥1600px) ----------

test.describe('Monitor breakpoint widens sidebar at 1600px', () => {
  test('at 1920px the #app-shell sidebar column resolves to 288px (monitor tier)', async ({ page }) => {
    await page.setViewportSize({ width: 1920, height: 1080 });
    await mockApi(page);
    await page.goto('/');
    await page.waitForSelector('#app-shell', { timeout: 5000 });

    const cols = await page.locator('#app-shell').evaluate((el) => getComputedStyle(el).gridTemplateColumns);
    const parts = cols.split(/\s+/);
    expect(parts.length).toBe(2);
    const sidebarPx = parseFloat(parts[0]);
    expect(sidebarPx).toBeCloseTo(288, 0);
  });

  test('at 1440px (laptop tier) the sidebar stays at 264px', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await mockApi(page);
    await page.goto('/');
    await page.waitForSelector('#app-shell', { timeout: 5000 });

    const cols = await page.locator('#app-shell').evaluate((el) => getComputedStyle(el).gridTemplateColumns);
    const parts = cols.split(/\s+/);
    expect(parts.length).toBe(2);
    const sidebarPx = parseFloat(parts[0]);
    expect(sidebarPx).toBeCloseTo(264, 0);
  });
});

// ---------- Meta theme-color parity ----------

test.describe('Meta theme-color matches the actual surface', () => {
  test('dark theme sets meta theme-color to #0B0E14', async ({ page }) => {
    await page.setViewportSize({ width: 412, height: 900 });
    await mockApi(page);
    await page.goto('/');
    await page.waitForSelector('#app-shell', { timeout: 5000 });

    await page.evaluate(async () => {
      const { Theme } = await import('/js/theme.js');
      localStorage.setItem('theme-preference', 'dark');
      Theme.apply('dark');
    });

    const content = await page.locator('meta[name="theme-color"]').getAttribute('content');
    expect((content || '').toUpperCase()).toBe('#0B0E14');
  });

  test('light theme sets meta theme-color to #FFFFFF (not the legacy warm gray)', async ({ page }) => {
    await page.setViewportSize({ width: 412, height: 900 });
    await mockApi(page);
    await page.goto('/');
    await page.waitForSelector('#app-shell', { timeout: 5000 });

    await page.evaluate(async () => {
      const { Theme } = await import('/js/theme.js');
      localStorage.setItem('theme-preference', 'light');
      Theme.apply('light');
    });

    const content = await page.locator('meta[name="theme-color"]').getAttribute('content');
    expect((content || '').toUpperCase()).toBe('#FFFFFF');
    // Negative: the legacy warm-gray value must not survive.
    expect((content || '').toUpperCase()).not.toBe('#2A2724');
  });
});

// ---------- Light-mode shadow token matches register addendum ----------

test.describe('Light shadow token matches register addendum', () => {
  test('light --shadow-medium is rgba(0, 0, 0, 0.06)', async ({ page }) => {
    await page.setViewportSize({ width: 412, height: 900 });
    await mockApi(page);
    await page.goto('/');
    await page.waitForSelector('#app-shell', { timeout: 5000 });

    await page.evaluate(() => {
      document.documentElement.dataset.theme = 'light';
    });

    const val = await cssVar(page, '--shadow-medium');
    // Browsers normalise rgba spacing; compare ignoring whitespace.
    expect(norm(val)).toBe(norm('rgba(0, 0, 0, 0.06)'));
  });
});
