// @ts-check
// Regression lock for the composer auto-grow path.
//
// The composer textarea wires `oninput="UI.composerAutoSize(this)"` as an
// inline handler, which resolves `UI` in GLOBAL scope. app.js imports the
// UI module but historically never assigned `window.UI`, so every
// keystroke threw `ReferenceError: UI is not defined` and auto-grow was
// dead in production (multi-line prompts rendered inside a one-row
// scroll slit). This spec types into the chat composer and asserts:
//
//   1. zero pageerrors while typing — the ReferenceError is gone, and
//   2. the inline style.height actually grows — auto-size really ran.
const { test, expect } = require('@playwright/test');

const AGENT_ID = 'agt-autosize-test';

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

async function setupAgent(page) {
  const agent = makeAgent();
  await page.route('**/events', (route) => route.abort('connectionrefused'));
  await page.route(/\/api\//, async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    if (path === '/api/agents') return route.fulfill({ json: [agent] });
    if (path === `/api/agents/${AGENT_ID}/conversation`) {
      return route.fulfill({
        json: [
          { role: 'human', content: 'ship it', timestamp: '2026-06-04T10:00:00Z' },
          { role: 'assistant', content: 'Done.', timestamp: '2026-06-04T10:01:00Z' },
        ],
      });
    }
    if (path === `/api/agents/${AGENT_ID}/pending-question`) {
      return route.fulfill({ json: null });
    }
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
}

test.describe('composer auto-grow', () => {
  test('typing multi-line text throws no pageerror and grows the textarea', async ({ page }) => {
    // Narrow viewport so ~200 chars wraps to several visual lines.
    await page.setViewportSize({ width: 390, height: 844 });
    const pageErrors = [];
    page.on('pageerror', (err) => pageErrors.push(String(err && err.message ? err.message : err)));

    await setupAgent(page);

    const input = page.locator('#reply-input');
    const initialHeight = await input.evaluate((el) => el.getBoundingClientRect().height);

    await input.click();
    await page.keyboard.type(
      'this is a deliberately long steering prompt that should wrap across ' +
      'several visual lines in the chat composer so the auto-grow handler ' +
      'has to expand the textarea height well beyond its single-row start',
      { delay: 5 }
    );

    expect(pageErrors, `pageerrors while typing: ${pageErrors.join(' | ')}`).toHaveLength(0);

    const inlineHeight = await input.evaluate((el) => el.style.height);
    expect(inlineHeight, 'auto-size should set an inline height').toMatch(/px$/);
    const grownHeight = await input.evaluate((el) => el.getBoundingClientRect().height);
    expect(grownHeight).toBeGreaterThan(initialHeight);
  });
});
