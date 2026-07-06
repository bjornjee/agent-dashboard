// @ts-check
// Honest working-indicator states.
//
// WORKING_STATES includes blocked-on-human states (permission / plan /
// question / error) so the indicator stays mounted through a whole
// turn — but historically the same shimmering "Running <tool>" /
// "Thinking" treatment rendered for ALL of them. Live audit evidence:
// a shimmering "Running AskUserQuestion" under a question card that was
// waiting for the user, and "Thinking" beside an "Errored" pill. The
// busiest signal on screen said "don't act" at exactly the moment the
// agent needed the human.
//
// Contract pinned here:
//   - blocked states render a STATIC notice (amber dot + copy), no
//     shimmer node, with an aria-live status container;
//   - running keeps the shimmer treatment with the classified verb;
//   - unknown states surface as humanized pill labels, not raw
//     "Waiting_input".
const { test, expect } = require('@playwright/test');

const AGENT_ID = 'agt-blocked-test';
const CONVERSATION = [
  { role: 'human', content: 'audit the flows', timestamp: '2026-06-04T10:00:00Z' },
  { role: 'assistant', content: 'Starting now.', timestamp: '2026-06-04T10:01:00Z' },
];

async function setupAgent(page, agentOverrides) {
  const agent = {
    session_id: AGENT_ID,
    cwd: '/Users/test/Code/myapp',
    branch: 'main',
    model: 'opus',
    started_at: new Date().toISOString(),
    subagent_count: 0,
    ...agentOverrides,
  };
  await page.route('**/events', (route) => route.abort('connectionrefused'));
  await page.route(/\/api\//, async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    if (path === '/api/agents') return route.fulfill({ json: [agent] });
    if (path === `/api/agents/${AGENT_ID}/conversation`) return route.fulfill({ json: CONVERSATION });
    if (path === `/api/agents/${AGENT_ID}/pending-question`) return route.fulfill({ json: null });
    if (path === `/api/agents/${AGENT_ID}/activity`) return route.fulfill({ json: [] });
    if (path === `/api/agents/${AGENT_ID}/usage`) return route.fulfill({ json: { CostUSD: 0 } });
    if (path === `/api/agents/${AGENT_ID}/subagents`) return route.fulfill({ json: [] });
    if (path === `/api/agents/${AGENT_ID}/plan`) return route.fulfill({ json: { content: '' } });
    if (path === '/api/skills') return route.fulfill({ json: [] });
    if (path === '/api/suggestions') return route.fulfill({ json: [] });
    return route.fulfill({ json: {} });
  });
  await page.goto('/');
  await page.waitForSelector('.ui-row, .ui-dock', { timeout: 5000 });
  await page.evaluate((id) => window.Dashboard.selectAgent(id), AGENT_ID);
  await page.waitForSelector('.detail-layout', { timeout: 5000 });
}

test.describe('working indicator honesty', () => {
  test('question state renders a static blocked notice, not a shimmer', async ({ page }) => {
    await setupAgent(page, { state: 'question', current_tool: 'AskUserQuestion' });
    const status = page.locator('.ui-msg-status');
    await expect(status).toBeVisible({ timeout: 5000 });
    await expect(status.locator('.ui-msg-status__blocked')).toHaveText(/Waiting for your reply/);
    // No shimmer node, no raw tool name anywhere in the indicator.
    await expect(status.locator('.ui-msg-status__label')).toHaveCount(0);
    await expect(status).not.toContainText('AskUserQuestion');
    // Live region so screen readers hear the state change.
    await expect(status).toHaveAttribute('aria-live', 'polite');
    await expect(status).toHaveAttribute('role', 'status');
  });

  test('error state says stopped, not Thinking', async ({ page }) => {
    await setupAgent(page, { state: 'error', current_tool: '' });
    const status = page.locator('.ui-msg-status');
    await expect(status).toBeVisible({ timeout: 5000 });
    await expect(status.locator('.ui-msg-status__blocked')).toHaveText(/Stopped on an error/);
    await expect(status.locator('.ui-msg-status__label')).toHaveCount(0);
    await expect(status).not.toContainText('Thinking');
  });

  test('running state keeps the shimmer verb treatment', async ({ page }) => {
    await setupAgent(page, { state: 'running', current_tool: 'Bash' });
    const status = page.locator('.ui-msg-status');
    await expect(status).toBeVisible({ timeout: 5000 });
    await expect(status.locator('.ui-msg-status__label')).toHaveText(/Running command/);
    await expect(status.locator('.ui-msg-status__blocked')).toHaveCount(0);
  });

  test('unknown states get humanized pill labels, not raw Waiting_input', async ({ page }) => {
    await setupAgent(page, { state: 'waiting_input' });
    const pill = page.locator('.detail-title .ui-status-pill');
    await expect(pill).toBeVisible({ timeout: 5000 });
    await expect(pill).toHaveText('Waiting');
  });
});
