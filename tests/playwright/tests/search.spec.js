// @ts-check
const { test, expect } = require('@playwright/test');

function makeAgent(overrides) {
  return {
    session_id: 'agt-' + Math.random().toString(36).slice(2, 8),
    cwd: '/Users/test/Code/myapp',
    branch: 'main',
    model: 'opus',
    state: 'running',
    started_at: new Date().toISOString(),
    subagent_count: 0,
    ...overrides,
  };
}

async function setupSearch(page, agents, opts) {
  await page.route('**/events', (route) => route.abort('connectionrefused'));
  await page.route(/\/api\/agents/, async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === '/api/agents') return route.fulfill({ json: agents });
    if (url.pathname.endsWith('/usage')) return route.fulfill({ json: { CostUSD: 0 } });
    if (url.pathname.endsWith('/subagents')) return route.fulfill({ json: [] });
    return route.fulfill({ json: {} });
  });
  await page.addInitScript(() => sessionStorage.clear());
  if (opts && opts.viewport) await page.setViewportSize(opts.viewport);
  await page.goto('/');
  await page.evaluate(() => sessionStorage.clear());
  await page.waitForSelector('.ui-row, .ui-dock', { timeout: 5000 });
}

const mobileViewport = { width: 390, height: 844 };
const desktopViewport = { width: 1280, height: 800 };

test.describe('Search overlay (mobile dock)', () => {
  test('clicking the floating dock search pill opens the overlay', async ({ page }) => {
    await setupSearch(page, [makeAgent({ cwd: '/u/myapp', branch: 'feat/foo' })],
      { viewport: mobileViewport });
    await page.click('.ui-dock__search');
    await expect(page.locator('#search-overlay-root')).toBeVisible();
    await expect(page.locator('#search-overlay-input')).toBeFocused();
  });

  test('typing fuzzy-filters by repo and branch', async ({ page }) => {
    await setupSearch(page, [
      makeAgent({ session_id: 'a', cwd: '/u/myapp', branch: 'main' }),
      makeAgent({ session_id: 'b', cwd: '/u/api', branch: 'feat/dashboard' }),
      makeAgent({ session_id: 'c', cwd: '/u/worktrees', branch: 'main' }),
    ], { viewport: mobileViewport });

    await page.click('.ui-dock__search');
    await page.fill('#search-overlay-input', 'feat');
    const rows = page.locator('.search-overlay__row');
    await expect(rows).toHaveCount(1);
    await expect(rows.first()).toHaveAttribute('data-agent-id', 'b');
    await expect(rows.first().locator('.search-overlay__hit').first()).toBeVisible();
  });

  test('Escape closes the overlay', async ({ page }) => {
    await setupSearch(page, [makeAgent({})], { viewport: mobileViewport });
    await page.click('.ui-dock__search');
    await expect(page.locator('#search-overlay-root')).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(page.locator('#search-overlay-root')).toHaveCount(0);
  });

  test('Cmd-K opens the overlay from the list view', async ({ page }) => {
    await setupSearch(page, [makeAgent({})], { viewport: mobileViewport });
    await page.keyboard.press('Meta+k');
    await expect(page.locator('#search-overlay-root')).toBeVisible();
  });

  test('Enter navigates to the selected agent', async ({ page }) => {
    const agent = makeAgent({ session_id: 'pick-me', cwd: '/u/myapp', branch: 'main' });
    await setupSearch(page, [agent], { viewport: mobileViewport });
    await page.click('.ui-dock__search');
    await page.fill('#search-overlay-input', 'myapp');
    await page.keyboard.press('Enter');
    await expect(page.locator('#search-overlay-root')).toHaveCount(0);
    // Detail view loaded for that agent — message stream area renders.
    await expect(page.locator('body.view-detail')).toBeVisible();
  });
});

test.describe('Search overlay (desktop sidebar)', () => {
  test('sidebar Search agents row is interactive and opens the overlay', async ({ page }) => {
    await setupSearch(page, [makeAgent({})], { viewport: desktopViewport });
    await page.waitForSelector('#app-sidebar .app-sidebar__nav-row');
    // The Search agents row is the second nav-row under the top stack
    // (New agent is first).
    const searchRow = page.locator('#app-sidebar .app-sidebar__nav-row .ui-row', { hasText: 'Search agents' });
    await searchRow.click();
    await expect(page.locator('#search-overlay-root')).toBeVisible();
  });
});
