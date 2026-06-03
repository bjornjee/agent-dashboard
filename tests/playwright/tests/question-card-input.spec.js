// @ts-check
// Regression lock for the AskUserQuestion freeform-input "constantly refreshing"
// bug. refreshConversation used to wipe #tab-conversation every poll and
// detach/re-append the question card, which dropped focus to <body> and
// reset the caret. With incremental rendering the card's DOM node is
// never touched when the pending payload is stable, so focus, caret, and
// input.value all survive the poll.

const { test, expect } = require('@playwright/test');

function makeAgent(overrides) {
  return {
    session_id: 'qcard-001',
    cwd: '/Users/test/Code/myapp',
    branch: 'feat/qcard',
    model: 'opus',
    state: 'running',
    started_at: new Date().toISOString(),
    subagent_count: 0,
    last_hook_event: 'Stop',
    ...overrides,
  };
}

const STABLE_PENDING = {
  tool_use_id: 'toolu_qcard_stable',
  questions: [{
    question: 'What is your name?',
    header: 'Name',
    multi_select: false,
    options: [],
  }],
};

async function mockApi(page) {
  await page.route('**/events', (route) => route.abort('connectionrefused'));
  await page.route(/\/api\/agents/, async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === '/api/agents') {
      await route.fulfill({ json: [makeAgent()] });
    } else if (path.endsWith('/conversation')) {
      await route.fulfill({ json: [
        { Role: 'human', Content: 'hi', Timestamp: '2026-06-03T10:00:00.000Z' },
        { Role: 'assistant', Content: 'hello', Timestamp: '2026-06-03T10:00:01.000Z' },
      ]});
    } else if (path.endsWith('/pending-question')) {
      await route.fulfill({ json: STABLE_PENDING });
    } else if (path.endsWith('/activity')) {
      await route.fulfill({ json: [] });
    } else if (path.endsWith('/usage')) {
      await route.fulfill({ json: { CostUSD: 0 } });
    } else if (path.endsWith('/subagents')) {
      await route.fulfill({ json: [] });
    } else {
      await route.fulfill({ json: {} });
    }
  });
  await page.route('**/api/usage/ratelimit', (r) => r.fulfill({ json: { session: {used_percent:1, resets_at:'2099-01-01T00:00:00Z'}, weekly: {used_percent:1, resets_at:'2099-01-01T00:00:00Z'}, plan: 'max_5' } }));
  await page.route('**/api/usage/daily*', (r) => r.fulfill({ json: { days: [], today_cost: 0, total_cost: 0 } }));
  await page.addInitScript(() => { try { sessionStorage.clear(); } catch {} });
}

test.describe('AskUserQuestion freeform input survives poll-tick re-renders', () => {
  test('focus, caret, and value persist across a refreshConversation tick', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await mockApi(page);
    await page.goto('/');
    await page.waitForSelector('#app-sidebar .app-sidebar__row[data-agent-id]', { timeout: 5000 });
    await page.click('#app-sidebar .app-sidebar__row[data-agent-id] .ui-row');
    await page.waitForSelector('#tab-conversation .question-card', { timeout: 5000 });

    const freeform = page.locator('.question-card__answer-input').first();
    await expect(freeform).toBeVisible();

    // User clicks into the freeform input and types "hello".
    await freeform.click();
    await freeform.fill('hello');

    // Stamp identity on the DOM node so we can tell whether the node
    // itself was replaced by the poll (vs preserved).
    const beforeIdentity = await freeform.evaluate((el) => {
      el.dataset.testIdentity = 'orig';
      return el.dataset.testIdentity;
    });
    expect(beforeIdentity).toBe('orig');

    // Pre-checks: input is focused, value is "hello", caret at end (position 5).
    const preFocused = await freeform.evaluate((el) => document.activeElement === el);
    const preValue = await freeform.inputValue();
    const preCaret = await freeform.evaluate((el) => el.selectionStart);
    expect(preFocused).toBe(true);
    expect(preValue).toBe('hello');
    expect(preCaret).toBe(5);

    // Fire a poll tick exactly the way the 2-second timer does.
    await page.evaluate(async () => {
      const mod = await import('/js/pages/detail.js');
      mod.refreshActiveTab('qcard-001', { session_id: 'qcard-001', state: 'running', last_hook_event: 'Stop' });
    });
    await page.waitForTimeout(400); // let the await get() settle

    // After the poll: the same DOM node, the same focus, the same caret, the same value.
    const postIdentity = await freeform.evaluate((el) => el.dataset.testIdentity || '');
    const postFocused = await freeform.evaluate((el) => document.activeElement === el);
    const postValue = await freeform.inputValue();
    const postCaret = await freeform.evaluate((el) => el.selectionStart);

    expect(postIdentity).toBe('orig'); // same DOM node — not rebuilt
    expect(postValue).toBe('hello');   // typed text survived
    expect(postFocused).toBe(true);    // still focused — user can keep typing
    expect(postCaret).toBe(5);         // caret survived
  });
});
