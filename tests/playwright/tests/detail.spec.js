// @ts-check
// Detail-view action bar + composer contracts, per agent state.
//
// Rewritten 2026-07: the previous generation of this file asserted the
// pre-Codex-iOS UI (.agent-card rows, text-labeled "Send" buttons,
// "Send a message..." placeholder, .vital-signs, the since-removed
// subagents disclosure) and failed 12/12 against the current renderer.
// Send-flow lifecycles live in send-failure.spec.js; question-card
// behavior in ask-user-question.spec.js / question-card-input.spec.js;
// working-indicator states in blocked-indicator.spec.js. This file pins
// what remains: composer presence/placeholder per state, the trailing
// Send/Stop pair, action-panel chips, and the Stats disclosure.
const { test, expect } = require('@playwright/test');

const AGENT_ID = 'agt-detail-test';

function makeAgent(overrides) {
  return {
    session_id: AGENT_ID,
    cwd: '/Users/test/Code/myapp',
    branch: 'feat/test',
    model: 'opus',
    state: 'running',
    started_at: new Date().toISOString(),
    subagent_count: 0,
    ...overrides,
  };
}

async function setupAndNavigate(page, agent) {
  await page.route('**/events', (route) => route.abort('connectionrefused'));
  await page.route(/\/api\//, async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    if (path === '/api/agents') return route.fulfill({ json: [agent] });
    if (path.endsWith('/conversation')) {
      return route.fulfill({
        json: [
          { Role: 'human', Content: 'Hello', Timestamp: '2026-06-04T10:00:00Z' },
          { Role: 'assistant', Content: 'Hi there', Timestamp: '2026-06-04T10:01:00Z' },
        ],
      });
    }
    if (path.endsWith('/pending-question')) return route.fulfill({ json: null });
    if (path.endsWith('/usage')) {
      return route.fulfill({ json: { InputTokens: 1000, OutputTokens: 500, CostUSD: 0.05 } });
    }
    if (path.endsWith('/activity')) return route.fulfill({ json: [] });
    if (path.endsWith('/plan')) return route.fulfill({ json: { content: '' } });
    if (path.endsWith('/input') && route.request().method() === 'POST') {
      return route.fulfill({ json: { ok: true } });
    }
    if (path === '/api/skills' || path === '/api/suggestions') return route.fulfill({ json: [] });
    return route.fulfill({ json: {} });
  });
  await page.goto('/');
  await page.waitForSelector('.ui-row, .ui-dock', { timeout: 5000 });
  await page.evaluate((id) => window.Dashboard.selectAgent(id), AGENT_ID);
  await page.waitForSelector('.detail-layout', { timeout: 5000 });
}

test.describe('Composer per state', () => {
  test('running agent offers Message placeholder with Stop AND Send', async ({ page }) => {
    await setupAndNavigate(page, makeAgent({ state: 'running' }));
    const input = page.locator('#reply-input');
    await expect(input).toBeVisible();
    await expect(input).toHaveAttribute('placeholder', 'Message');
    await expect(page.locator('.ui-composer__stop')).toBeVisible();
    // Send exists in the DOM (CSS reveals it once text is typed).
    await expect(page.locator('.ui-composer__send')).toHaveCount(1);
    await input.fill('steer the work');
    await expect(page.locator('.ui-composer__send')).toBeVisible();
  });

  test('question state keeps the reply placeholder when no card is pending', async ({ page }) => {
    await setupAndNavigate(page, makeAgent({ state: 'question' }));
    const input = page.locator('#reply-input');
    await expect(input).toBeVisible();
    await expect(input).toHaveAttribute('placeholder', 'Type a reply…');
    await expect(page.locator('.ui-composer__send')).toBeVisible();
    await expect(page.locator('.ui-composer__stop')).toHaveCount(0);
  });

  test('merged agent keeps the follow-up composer alongside Close', async ({ page }) => {
    await setupAndNavigate(page, makeAgent({ state: 'merged' }));
    const input = page.locator('#reply-input');
    await expect(input).toBeVisible();
    await expect(input).toHaveAttribute('placeholder', 'Ask for follow-up changes…');
    await expect(page.locator('.action-panel .ui-modal-btn', { hasText: 'Close' })).toBeVisible();
  });

  test('send on click posts the text and clears the composer', async ({ page }) => {
    await setupAndNavigate(page, makeAgent({ state: 'done' }));
    const requestPromise = page.waitForRequest((req) =>
      req.url().includes('/input') && req.method() === 'POST'
    );
    const input = page.locator('#reply-input');
    await input.fill('test message');
    await page.locator('.ui-composer__send').click();
    const request = await requestPromise;
    expect(request.postDataJSON()).toEqual({ text: 'test message' });
    await expect(input).toHaveValue('');
  });

  test('no dead controls: composer renders no mic and no chip buttons', async ({ page }) => {
    await setupAndNavigate(page, makeAgent({ state: 'running' }));
    await expect(page.locator('.ui-composer__mic')).toHaveCount(0);
    await expect(page.locator('button.ui-composer__chip')).toHaveCount(0);
  });
});

test.describe('Action panel per state', () => {
  test('permission state shows Approve and Reject with the composer', async ({ page }) => {
    await setupAndNavigate(page, makeAgent({ state: 'permission' }));
    await expect(page.locator('#reply-input')).toBeVisible();
    await expect(page.locator('.action-panel .ui-modal-btn', { hasText: 'Approve' })).toBeVisible();
    await expect(page.locator('.action-panel .ui-modal-btn', { hasText: 'Reject' })).toBeVisible();
  });

  test('open PR shows Open PR and Merge chips', async ({ page }) => {
    await setupAndNavigate(page, makeAgent({
      state: 'pr',
      pinned_state: 'pr',
      pr_url: 'https://github.com/test/repo/pull/1',
    }));
    await expect(page.locator('#reply-input')).toBeVisible();
    await expect(page.locator('.action-panel .ui-modal-btn', { hasText: 'Open PR' })).toBeVisible();
    await expect(page.locator('.action-panel .ui-modal-btn', { hasText: 'Merge' })).toBeVisible();
  });
});

test.describe('Stats disclosure', () => {
  test('desktop starts expanded and toggles closed on click', async ({ page }) => {
    await page.setViewportSize({ width: 1024, height: 768 });
    await setupAndNavigate(page, makeAgent({ state: 'running' }));
    await page.waitForSelector('.vital-strip', { timeout: 5000 });
    const section = page.locator('#vital-signs-container-section');
    await expect(section).toHaveAttribute('open', '');
    await page.click('summary[data-section="vital-signs-container"]');
    await expect(section).not.toHaveAttribute('open', '');
  });

  test('mobile starts collapsed', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await setupAndNavigate(page, makeAgent({ state: 'running' }));
    const section = page.locator('#vital-signs-container-section');
    await expect(section).toBeVisible();
    await expect(section).not.toHaveAttribute('open', '');
  });
});
