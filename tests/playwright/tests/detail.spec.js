// @ts-check
const { test, expect } = require('@playwright/test');

// Mock agent data for different states
function makeAgent(overrides) {
  return {
    session_id: 'test-001',
    repo: 'https://github.com/test/repo',
    branch: 'feat/test',
    model: 'opus',
    state: 'running',
    started_at: new Date().toISOString(),
    ...overrides,
  };
}

// Set up API mocks for a given agent and navigate to its detail view
async function setupAndNavigate(page, agent) {
  const agents = [agent];

  // Block SSE so real server data doesn't override mocks
  await page.route('**/events', async (route) => {
    route.abort('connectionrefused');
  });

  // Mock ALL /api/agents* routes with a single handler to avoid conflicts
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
      await route.fulfill({
        json: [
          { AgentType: 'explore', Description: 'Search code', StartedAt: new Date().toISOString(), Completed: true },
          { AgentType: 'plan', Description: 'Design approach', StartedAt: new Date().toISOString(), Completed: false },
        ],
      });
    } else if (path.endsWith('/input')) {
      await route.fulfill({ json: { ok: true } });
    } else {
      await route.fulfill({ json: {} });
    }
  });

  // Navigate to the app — routes and initScript are already set
  await page.addInitScript(() => sessionStorage.clear());
  await page.goto('/');
  await page.evaluate(() => sessionStorage.clear());
  // Wait for the app to render with SSE data, then click the agent card
  await page.waitForSelector('.agent-card', { timeout: 5000 });
  await page.click('.agent-card');
  // Wait for detail view to render
  await page.waitForSelector('.detail-layout', { timeout: 5000 });
}

// --- Reply Input Tests ---

test.describe('Reply Input Box', () => {
  test('should show reply input for running agent', async ({ page }) => {
    const agent = makeAgent({ state: 'running' });
    await setupAndNavigate(page, agent);

    const input = page.locator('#reply-input');
    await expect(input).toBeVisible();
    await expect(input).toHaveAttribute('placeholder', 'Send a message...');

    const sendBtn = page.locator('.action-bar button', { hasText: 'Send' });
    await expect(sendBtn).toBeVisible();
  });

  test('should show reply input with contextual placeholder for question state', async ({ page }) => {
    const agent = makeAgent({ state: 'question' });
    await setupAndNavigate(page, agent);

    const input = page.locator('#reply-input');
    await expect(input).toBeVisible();
    await expect(input).toHaveAttribute('placeholder', 'Type a reply...');
  });

  test('should NOT show reply input for merged agent', async ({ page }) => {
    const agent = makeAgent({ state: 'merged' });
    await setupAndNavigate(page, agent);

    const input = page.locator('#reply-input');
    await expect(input).not.toBeVisible();
  });

  test('should not show Open Claude button', async ({ page }) => {
    const agent = makeAgent({ state: 'running' });
    await setupAndNavigate(page, agent);

    const openClaudeBtn = page.locator('button', { hasText: 'Open Claude' });
    await expect(openClaudeBtn).toHaveCount(0);
  });

  test('should send input on button click', async ({ page }) => {
    const agent = makeAgent({ state: 'running' });
    await setupAndNavigate(page, agent);

    const requestPromise = page.waitForRequest(req =>
      req.url().includes('/input') && req.method() === 'POST'
    );

    const input = page.locator('#reply-input');
    await input.fill('test message');
    await page.click('.action-bar button:has-text("Send")');

    const request = await requestPromise;
    expect(request.postDataJSON()).toEqual({ text: 'test message' });

    // Input should be cleared after send
    await expect(input).toHaveValue('');
  });
});

// --- Collapsible Sections Tests ---

test.describe('Collapsible Sections', () => {
  test('should render collapsible toggles for vital signs and subagent pills', async ({ page }) => {
    const agent = makeAgent({ state: 'running' });
    await setupAndNavigate(page, agent);

    // Wait for vital signs to load
    await page.waitForSelector('.vital-signs', { timeout: 5000 });

    const toggles = page.locator('.collapsible-summary');
    await expect(toggles).toHaveCount(2);
  });

  test('should collapse vital signs on toggle click', async ({ page }) => {
    // Use desktop viewport so sections start expanded (open)
    await page.setViewportSize({ width: 1024, height: 768 });
    const agent = makeAgent({ state: 'running' });
    await setupAndNavigate(page, agent);

    await page.waitForSelector('.vital-signs', { timeout: 5000 });

    const vitalSection = page.locator('#vital-signs-container-section');
    await expect(vitalSection).toHaveAttribute('open', '');

    // Click the summary to collapse
    await page.click('.collapsible-summary[data-section="vital-signs-container"]');
    await expect(vitalSection).not.toHaveAttribute('open', '');
  });

  test('should expand vital signs after second click', async ({ page }) => {
    await page.setViewportSize({ width: 1024, height: 768 });
    const agent = makeAgent({ state: 'running' });
    await setupAndNavigate(page, agent);

    await page.waitForSelector('.vital-signs', { timeout: 5000 });

    const vitalSection = page.locator('#vital-signs-container-section');

    // Collapse
    await page.click('.collapsible-summary[data-section="vital-signs-container"]');
    await expect(vitalSection).not.toHaveAttribute('open', '');

    // Expand
    await page.click('.collapsible-summary[data-section="vital-signs-container"]');
    await expect(vitalSection).toHaveAttribute('open', '');
  });

  test('should default to collapsed on mobile viewport', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    const agent = makeAgent({ state: 'running' });
    await setupAndNavigate(page, agent);

    await page.waitForSelector('.collapsible-summary', { timeout: 5000 });

    const vitalSection = page.locator('#vital-signs-container-section');
    await expect(vitalSection).not.toHaveAttribute('open', '');

    const subagentSection = page.locator('#subagent-summary-section');
    await expect(subagentSection).not.toHaveAttribute('open', '');
  });
});

// --- Action Bar State Tests ---

test.describe('Action Bar States', () => {
  test('permission state shows Approve and Reject with input', async ({ page }) => {
    const agent = makeAgent({ state: 'permission' });
    await setupAndNavigate(page, agent);

    await expect(page.locator('#reply-input')).toBeVisible();
    await expect(page.locator('.action-bar button', { hasText: 'Approve' })).toBeVisible();
    await expect(page.locator('.action-bar button', { hasText: 'Reject' })).toBeVisible();
  });

  test('pr state shows Open PR and Merge with input', async ({ page }) => {
    const agent = makeAgent({ state: 'pr', pr_url: 'https://github.com/test/repo/pull/1' });
    await setupAndNavigate(page, agent);

    await expect(page.locator('#reply-input')).toBeVisible();
    await expect(page.locator('.action-bar button', { hasText: 'Open PR' })).toBeVisible();
    await expect(page.locator('.action-bar button', { hasText: 'Merge' })).toBeVisible();
  });

  test('merged state shows Close button without input', async ({ page }) => {
    const agent = makeAgent({ state: 'merged' });
    await setupAndNavigate(page, agent);

    await expect(page.locator('#reply-input')).not.toBeVisible();
    await expect(page.locator('.action-bar button', { hasText: 'Close' })).toBeVisible();
  });
});
