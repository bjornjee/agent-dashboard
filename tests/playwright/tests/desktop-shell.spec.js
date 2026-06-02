// @ts-check
// Phase B: two-pane desktop shell at viewport ≥ 1024px.
// Mobile (< 1024px) remains a single-column stack — covered by other specs.
const { test, expect } = require('@playwright/test');

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

async function setup(page, agents) {
  await page.route('**/events', async (route) => {
    route.abort('connectionrefused');
  });

  await page.route(/\/api\/agents/, async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    if (path === '/api/agents') {
      await route.fulfill({ json: agents });
    } else if (path.endsWith('/conversation')) {
      await route.fulfill({
        json: [
          { Role: 'human', Content: 'Hello', Timestamp: new Date().toISOString() },
          { Role: 'assistant', Content: 'Hi there', Timestamp: new Date().toISOString() },
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

  await page.addInitScript(() => sessionStorage.clear());
}

test.describe('Desktop two-pane shell (≥ 1024px)', () => {
  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
  });

  test('renders #app-shell wrapping sidebar and main pane', async ({ page }) => {
    await setup(page, [makeAgent()]);
    await page.goto('/');
    await page.waitForSelector('#app-shell', { timeout: 5000 });

    const shell = page.locator('#app-shell');
    await expect(shell).toBeVisible();

    const sidebar = page.locator('#app-sidebar');
    await expect(sidebar).toBeVisible();

    const main = page.locator('#app-main');
    await expect(main).toBeVisible();

    // The existing #app container nests inside the main pane
    const appInsideMain = page.locator('#app-main #app');
    await expect(appInsideMain).toHaveCount(1);
  });

  test('sidebar uses grid layout at desktop width', async ({ page }) => {
    await setup(page, [makeAgent()]);
    await page.goto('/');
    await page.waitForSelector('#app-shell', { timeout: 5000 });

    const shellDisplay = await page.locator('#app-shell').evaluate(el => getComputedStyle(el).display);
    expect(shellDisplay).toBe('grid');

    // Sidebar width should be the declared --sidebar-width (264px) per design contract
    const sidebarBox = await page.locator('#app-sidebar').boundingBox();
    expect(sidebarBox).not.toBeNull();
    expect(sidebarBox.width).toBeCloseTo(264, 0);
  });

  test('agent list renders inside the sidebar on desktop', async ({ page }) => {
    await setup(page, [makeAgent()]);
    await page.goto('/');
    await page.waitForSelector('#app-sidebar .app-sidebar__row', { timeout: 5000 });

    // Agent rows live in the sidebar (carry the data-agent-id marker),
    // not in the main pane's #app.
    const sidebarAgentRows = page.locator('#app-sidebar .app-sidebar__row[data-agent-id]');
    await expect(sidebarAgentRows.first()).toBeVisible();
  });

  test('floating dock is hidden on desktop', async ({ page }) => {
    await setup(page, [makeAgent()]);
    await page.goto('/');
    await page.waitForSelector('#app-shell', { timeout: 5000 });

    // The mobile floating dock either doesn't render or is display:none
    const dockCount = await page.locator('.ui-dock').count();
    if (dockCount > 0) {
      const dockDisplay = await page.locator('.ui-dock').first().evaluate(el => getComputedStyle(el).display);
      expect(dockDisplay).toBe('none');
    }
  });

  test('detail view renders into main pane while sidebar persists', async ({ page }) => {
    await setup(page, [makeAgent()]);
    await page.goto('/');
    await page.waitForSelector('#app-sidebar .app-sidebar__row[data-agent-id]', { timeout: 5000 });

    await page.click('#app-sidebar .app-sidebar__row[data-agent-id] .ui-row');
    // Detail layout appears inside main pane's #app
    await page.waitForSelector('#app-main .detail-layout', { timeout: 5000 });

    // Sidebar is still present and visible after navigation
    await expect(page.locator('#app-sidebar')).toBeVisible();
    await expect(page.locator('#app-sidebar .app-sidebar__row[data-agent-id]').first()).toBeVisible();
  });

  test('mobile viewport keeps single-column layout (sidebar hidden)', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 800 });
    await setup(page, [makeAgent()]);
    await page.goto('/');
    await page.waitForSelector('.agent-card, .ui-row', { timeout: 5000 });

    // Sidebar exists in DOM but is hidden below the breakpoint
    const sidebar = page.locator('#app-sidebar');
    const visible = await sidebar.isVisible();
    expect(visible).toBe(false);
  });
});
