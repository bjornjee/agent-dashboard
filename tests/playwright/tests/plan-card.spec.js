// @ts-check
// Plan-card action integrity.
//
// Historical failure mode: renderPlanLinkCard suppressed its in-card
// Approve/Reject only while the action panel showed the same pair
// (state plan/permission). The moment state moved on, every plan card
// in the transcript RE-GAINED live buttons — and tapping one posts
// picker keystrokes into a live CLI session. A resolved plan in history
// was a loaded gun.
//
// Contract pinned here:
//   - a plan-saved entry renders actions ONLY while it is the final
//     visible entry (the transcript hasn't moved on);
//   - historical plan cards render as plain link cards, no buttons;
//   - while the action panel owns Approve/Reject (state plan), the
//     in-card pair is suppressed even on the final entry;
//   - tapping Approve swaps the card's actions for a static receipt.
const { test, expect } = require('@playwright/test');

const AGENT_ID = 'agt-plancard-test';

async function setupAgent(page, { conversation, agentOverrides }) {
  const agent = {
    session_id: AGENT_ID,
    cwd: '/Users/test/Code/myapp',
    branch: 'main',
    model: 'gpt-5.5',
    harness: 'codex',
    state: 'running',
    started_at: new Date().toISOString(),
    subagent_count: 0,
    ...agentOverrides,
  };
  const approvePosts = [];
  await page.route('**/events', (route) => route.abort('connectionrefused'));
  await page.route(/\/api\//, async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    if (path === `/api/agents/${AGENT_ID}/approve` && route.request().method() === 'POST') {
      approvePosts.push(1);
      return route.fulfill({ json: { ok: true } });
    }
    if (path === '/api/agents') return route.fulfill({ json: [agent] });
    if (path === `/api/agents/${AGENT_ID}/conversation`) return route.fulfill({ json: conversation });
    if (path === `/api/agents/${AGENT_ID}/pending-question`) return route.fulfill({ json: null });
    if (path === `/api/agents/${AGENT_ID}/activity`) return route.fulfill({ json: [] });
    if (path === `/api/agents/${AGENT_ID}/usage`) return route.fulfill({ json: { CostUSD: 0 } });
    if (path === `/api/agents/${AGENT_ID}/plan`) return route.fulfill({ json: { content: '# Plan' } });
    if (path === '/api/skills') return route.fulfill({ json: [] });
    if (path === '/api/suggestions') return route.fulfill({ json: [] });
    return route.fulfill({ json: {} });
  });
  await page.goto('/');
  await page.waitForSelector('.ui-row, .ui-dock', { timeout: 5000 });
  await page.evaluate((id) => window.Dashboard.selectAgent(id), AGENT_ID);
  await page.waitForSelector('.conversation', { timeout: 5000 });
  return { approvePosts };
}

const HUMAN = { role: 'human', content: 'plan it', timestamp: '2026-06-04T10:00:00Z' };
const PLAN_SAVED = { role: 'plan-saved', timestamp: '2026-06-04T10:02:00Z' };
const ASSISTANT_AFTER = { role: 'assistant', content: 'Implementing now.', timestamp: '2026-06-04T10:03:00Z' };

test.describe('plan-card action integrity', () => {
  test('historical plan card renders without live buttons', async ({ page }) => {
    await setupAgent(page, { conversation: [HUMAN, PLAN_SAVED, ASSISTANT_AFTER] });
    const card = page.locator('.ui-msg--plan-link');
    await expect(card).toBeVisible({ timeout: 5000 });
    await expect(card.locator('.chat-plan-link__actions')).toHaveCount(0);
    await expect(card.locator('.ui-modal-btn')).toHaveCount(0);
  });

  test('final plan card is armed and Approve swaps to a receipt', async ({ page }) => {
    const ctx = await setupAgent(page, { conversation: [HUMAN, PLAN_SAVED] });
    const card = page.locator('.ui-msg--plan-link');
    await expect(card).toBeVisible({ timeout: 5000 });
    const actions = card.locator('.chat-plan-link__actions');
    await expect(actions).toHaveCount(1);
    await actions.getByRole('button', { name: 'Approve' }).click();
    await expect.poll(() => ctx.approvePosts.length, { timeout: 3000 }).toBe(1);
    await expect(card.locator('.chat-plan-link__receipt')).toHaveText(/Approved/, { timeout: 3000 });
    await expect(card.locator('.chat-plan-link__actions')).toHaveCount(0);
    // The receipt survives subsequent poll ticks (sig unchanged).
    await page.waitForTimeout(1800);
    await expect(card.locator('.chat-plan-link__receipt')).toHaveCount(1);
  });

  test('action panel ownership suppresses in-card buttons on the final entry', async ({ page }) => {
    await setupAgent(page, {
      conversation: [HUMAN, PLAN_SAVED],
      agentOverrides: { state: 'plan', harness: 'claude', model: 'opus' },
    });
    // Panel renders the pair…
    const panel = page.locator('.action-panel');
    await expect(panel).toBeVisible({ timeout: 5000 });
    await expect(panel.getByRole('button', { name: 'Approve' })).toBeVisible();
    // …so the card must not duplicate it.
    const card = page.locator('.ui-msg--plan-link');
    await expect(card).toBeVisible();
    await expect(card.locator('.chat-plan-link__actions')).toHaveCount(0);
  });
});
