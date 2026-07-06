// @ts-check
// Failed-send recovery lifecycle.
//
// Historical failure mode: Dashboard.sendInput cleared the composer
// BEFORE the POST and had no catch. A failed send therefore destroyed
// the typed message, left the optimistic bubble + "Sending…" caption
// re-asserting forever (reconcileOptimisticMessage re-adds them every
// poll), and kept a fake forced-running working indicator. A thrown
// fetch (network drop — the primary phone-user case) showed nothing at
// all.
//
// Contract pinned here:
//   failure (ok:false or thrown) →
//     - composer gets the text back (editable retry),
//     - a STICKY error toast with a dismiss button appears,
//     - no optimistic bubble, no "Sending…" caption, no forced
//       working indicator survive,
//   success →
//     - composer clears, caption lifts after ack, bubble stays until
//       the API echoes it.
const { test, expect } = require('@playwright/test');

const AGENT_ID = 'agt-sendfail-test';
const CONVERSATION = [
  { role: 'human', content: 'ship it', timestamp: '2026-06-04T10:00:00Z' },
  { role: 'assistant', content: 'Done.', timestamp: '2026-06-04T10:01:00Z' },
];

function makeAgent(overrides) {
  return {
    session_id: AGENT_ID,
    cwd: '/Users/test/Code/myapp',
    branch: 'main',
    model: 'opus',
    state: 'done',
    started_at: new Date().toISOString(),
    subagent_count: 0,
    ...overrides,
  };
}

// inputMode: 'fail-json' | 'fail-network' | 'ok'
async function setupAgent(page, inputMode) {
  const agent = makeAgent();
  const inputPosts = [];
  await page.route('**/events', (route) => route.abort('connectionrefused'));
  await page.route(/\/api\//, async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    if (path === `/api/agents/${AGENT_ID}/input` && route.request().method() === 'POST') {
      inputPosts.push(JSON.parse(route.request().postData() || '{}'));
      if (inputMode === 'fail-json') {
        return route.fulfill({ status: 500, json: { ok: false, error: 'tmux pane is gone' } });
      }
      if (inputMode === 'fail-network') {
        return route.abort('connectionrefused');
      }
      return route.fulfill({ json: { ok: true } });
    }
    if (path === '/api/agents') return route.fulfill({ json: [agent] });
    if (path === `/api/agents/${AGENT_ID}/conversation`) return route.fulfill({ json: CONVERSATION });
    if (path === `/api/agents/${AGENT_ID}/pending-question`) return route.fulfill({ json: null });
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
  await page.waitForSelector('#reply-input', { timeout: 5000 });
  return { inputPosts };
}

const MESSAGE = 'please also update the changelog';

async function typeAndSend(page) {
  const input = page.locator('#reply-input');
  await input.click();
  await page.keyboard.type(MESSAGE, { delay: 5 });
  await page.locator('.ui-composer__send').click();
}

async function expectFailureRecovered(page) {
  // Composer got the message back for an editable retry.
  await expect(page.locator('#reply-input')).toHaveValue(MESSAGE, { timeout: 3000 });
  await expect(page.locator('#reply-input')).toBeEnabled();
  // Sticky error toast with an explicit dismiss affordance.
  const toast = page.locator('.ui-toast--error.ui-toast--sticky');
  await expect(toast).toBeVisible({ timeout: 3000 });
  await expect(toast.locator('.ui-toast__close')).toBeVisible();
  // No lying leftovers — and none re-appear on later poll ticks.
  await page.waitForTimeout(1800); // > 2 conversation poll ticks (750ms)
  await expect(page.locator('.ui-msg__caption--sending')).toHaveCount(0);
  await expect(page.locator('[data-optimistic="1"]')).toHaveCount(0);
  await expect(page.locator('.ui-msg-status--working')).toHaveCount(0);
}

test.describe('failed-send recovery', () => {
  test('server-error send restores text, shows sticky toast, unwinds optimistic UI', async ({ page }) => {
    const ctx = await setupAgent(page, 'fail-json');
    await typeAndSend(page);
    await expect.poll(() => ctx.inputPosts.length, { timeout: 3000 }).toBe(1);
    await expectFailureRecovered(page);
  });

  test('network-drop send recovers identically (thrown fetch path)', async ({ page }) => {
    const ctx = await setupAgent(page, 'fail-network');
    await typeAndSend(page);
    await expect.poll(() => ctx.inputPosts.length, { timeout: 3000 }).toBe(1);
    await expectFailureRecovered(page);
  });

  test('successful send clears composer and lifts the Sending caption', async ({ page }) => {
    const ctx = await setupAgent(page, 'ok');
    await typeAndSend(page);
    await expect.poll(() => ctx.inputPosts.length, { timeout: 3000 }).toBe(1);
    expect(ctx.inputPosts[0].text).toBe(MESSAGE);
    await expect(page.locator('#reply-input')).toHaveValue('', { timeout: 3000 });
    // Ack lifts the caption; the optimistic bubble stays (API echo pending).
    await expect(page.locator('.ui-msg__caption--sending')).toHaveCount(0, { timeout: 3000 });
    await expect(page.locator('[data-optimistic="1"]')).toHaveCount(1);
    await expect(page.locator('[data-optimistic="1"]')).not.toHaveClass(/ui-msg--optimistic/);
  });
});
