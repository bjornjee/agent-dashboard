// @ts-check
const { test, expect } = require('@playwright/test');

// Mock agent data
function makeAgent(overrides) {
  return {
    session_id: 'list-001',
    cwd: '/Users/test/Code/myapp',
    branch: 'feat/dashboard',
    model: 'opus',
    state: 'running',
    started_at: new Date().toISOString(),
    subagent_count: 0,
    ...overrides,
  };
}

// Set up API mocks and navigate to the list view
async function setupList(page, agents) {
  await page.route('**/events', async (route) => {
    route.abort('connectionrefused');
  });

  await page.route(/\/api\/agents/, async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;

    if (path === '/api/agents') {
      await route.fulfill({ json: agents });
    } else if (path.endsWith('/usage')) {
      await route.fulfill({ json: { InputTokens: 1000, OutputTokens: 500, CostUSD: 0.05 } });
    } else if (path.endsWith('/subagents')) {
      await route.fulfill({ json: [] });
    } else {
      await route.fulfill({ json: {} });
    }
  });

  await page.addInitScript(() => sessionStorage.clear());
  await page.goto('/');
  await page.evaluate(() => sessionStorage.clear());
  await page.waitForSelector('.agent-card', { timeout: 5000 });
}

// --- Card Display Hierarchy Tests ---

test.describe('Agent Card Display', () => {
  test('should show repo name as primary text', async ({ page }) => {
    const agent = makeAgent({ cwd: '/Users/test/Code/myapp' });
    await setupList(page, [agent]);

    const name = page.locator('.agent-name');
    await expect(name).toHaveText('myapp');
  });

  test('should show branch as secondary text', async ({ page }) => {
    const agent = makeAgent({ branch: 'feat/dashboard' });
    await setupList(page, [agent]);

    const branch = page.locator('.agent-branch');
    await expect(branch).toBeVisible();
    await expect(branch).toHaveText('feat/dashboard');
  });

  test('should not show branch element when no branch', async ({ page }) => {
    const agent = makeAgent({ branch: '' });
    await setupList(page, [agent]);

    const branch = page.locator('.agent-branch');
    await expect(branch).toHaveCount(0);
  });

  test('should show model and duration in muted meta row', async ({ page }) => {
    const agent = makeAgent({ model: 'opus' });
    await setupList(page, [agent]);

    const meta = page.locator('.agent-meta');
    await expect(meta).toBeVisible();
    await expect(meta).toContainText('opus');
  });

  test('branch should not appear in muted meta row', async ({ page }) => {
    const agent = makeAgent({ branch: 'feat/test', model: 'opus' });
    await setupList(page, [agent]);

    const meta = page.locator('.agent-meta');
    // Branch should be in its own .agent-branch div, not in .agent-meta
    await expect(meta).not.toContainText('feat/test');
  });

  test('should display worktree agent with correct repo name', async ({ page }) => {
    const agent = makeAgent({
      cwd: '/Users/test/Code/worktrees/skills/feat-branch',
      worktree_cwd: '/Users/test/Code/worktrees/skills/feat-branch',
      branch: 'feat-branch',
    });
    await setupList(page, [agent]);

    const name = page.locator('.agent-name');
    // Should show "skills" (repo name), not "feat-branch" (branch/dir name)
    await expect(name).toHaveText('skills');
  });

  test('branch styled differently from repo name', async ({ page }) => {
    const agent = makeAgent({ branch: 'feat/test' });
    await setupList(page, [agent]);

    const name = page.locator('.agent-name');
    const branch = page.locator('.agent-branch');

    // Name should be primary color, branch should be secondary
    const nameColor = await name.evaluate(el => getComputedStyle(el).color);
    const branchColor = await branch.evaluate(el => getComputedStyle(el).color);
    expect(nameColor).not.toEqual(branchColor);
  });

  test('meta row text is visually muted', async ({ page }) => {
    const agent = makeAgent({ model: 'opus', branch: 'main' });
    await setupList(page, [agent]);

    const branch = page.locator('.agent-branch');
    const meta = page.locator('.agent-meta');

    const branchColor = await branch.evaluate(el => getComputedStyle(el).color);
    const metaColor = await meta.evaluate(el => getComputedStyle(el).color);
    // Meta should be more muted (lighter/less saturated) than branch
    expect(metaColor).not.toEqual(branchColor);
  });
});

// --- Multiple Agents Tests ---

test.describe('Multiple Agent Cards', () => {
  test('should render multiple agents with correct repo names', async ({ page }) => {
    const agents = [
      makeAgent({ session_id: 'a1', cwd: '/Code/skills', branch: 'main', state: 'running' }),
      makeAgent({ session_id: 'a2', cwd: '/Code/dashboard', branch: 'feat/ui', state: 'running' }),
    ];
    await setupList(page, agents);

    const cards = page.locator('.agent-card');
    await expect(cards).toHaveCount(2);

    const names = page.locator('.agent-name');
    await expect(names.nth(0)).toHaveText('skills');
    await expect(names.nth(1)).toHaveText('dashboard');
  });
});
