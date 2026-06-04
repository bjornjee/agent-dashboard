// @ts-check
// E2E coverage for the AskUserQuestion card lifecycle.
//
// Two regression scenarios this file pins:
//
// 1. **Empty tool_use_id render**. The agent-state hook's
//    PermissionRequest payload can stamp `pending_question.tool_use_id`
//    as `''`. Until baf2354 the frontend's
//    `if (pending && pending.tool_use_id)` gate skipped the render
//    because `''` is falsy — the card never appeared even though the
//    questions were in the payload. Now the render path falls back to
//    `questionCardSignature(pending)` as the card id.
//
// 2. **Send Answer + clear lifecycle**. Picking an option, typing a
//    freeform answer, and clicking Send Answer must POST to
//    `/api/agents/{id}/input` with the composed text, then the card
//    must disappear on the next poll tick when pending-question
//    returns null.
const { test, expect } = require('@playwright/test');

const AGENT_ID = 'agt-asq-test';

function makeAgent(overrides) {
  return {
    session_id: AGENT_ID,
    cwd: '/Users/test/Code/myapp',
    branch: 'main',
    model: 'opus',
    state: 'question',
    started_at: new Date().toISOString(),
    subagent_count: 0,
    last_hook_event: 'PermissionRequest',
    current_tool: 'AskUserQuestion',
    ...overrides,
  };
}

const PENDING_NO_TOOL_USE_ID = {
  tool_use_id: '',
  questions: [
    {
      question: "What's your favorite dummy color?",
      header: 'Color',
      multi_select: false,
      options: [
        { label: 'Red', description: 'Bold, warm, attention-grabbing.' },
        { label: 'Blue', description: 'Cool, calm, classic.' },
        { label: 'Green', description: 'Fresh, natural, balanced.' },
      ],
    },
  ],
};

async function setupAgent(page, { pending, conversation }) {
  const agent = makeAgent();
  let inputPosts = [];
  let pendingState = pending;
  await page.route('**/events', (route) => route.abort('connectionrefused'));
  await page.route(/\/api\//, async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    if (path === '/api/agents') return route.fulfill({ json: [agent] });
    if (path === `/api/agents/${AGENT_ID}/conversation`) {
      return route.fulfill({ json: conversation || [] });
    }
    if (path === `/api/agents/${AGENT_ID}/pending-question`) {
      return route.fulfill({ json: pendingState });
    }
    if (path === `/api/agents/${AGENT_ID}/usage`) {
      return route.fulfill({ json: { CostUSD: 0 } });
    }
    if (path === `/api/agents/${AGENT_ID}/subagents`) {
      return route.fulfill({ json: [] });
    }
    if (path === `/api/agents/${AGENT_ID}/plan`) {
      return route.fulfill({ json: { content: '' } });
    }
    if (path === `/api/agents/${AGENT_ID}/input` && route.request().method() === 'POST') {
      inputPosts.push(JSON.parse(route.request().postData() || '{}'));
      return route.fulfill({ json: { ok: true } });
    }
    if (path === '/api/skills') return route.fulfill({ json: [] });
    if (path === '/api/suggestions') return route.fulfill({ json: [] });
    return route.fulfill({ json: {} });
  });
  await page.goto('/');
  await page.waitForSelector('.ui-row, .ui-dock', { timeout: 5000 });
  // Open the agent detail
  await page.evaluate((id) => window.Dashboard.selectAgent(id), AGENT_ID);
  return {
    inputPosts,
    setPending(next) { pendingState = next; },
  };
}

test.describe('AskUserQuestion card lifecycle', () => {
  test('card renders even when pending.tool_use_id is empty', async ({ page }) => {
    await setupAgent(page, {
      pending: PENDING_NO_TOOL_USE_ID,
      conversation: [{ role: 'human', content: 'plan it', timestamp: '2026-06-04T10:00:00Z' }],
    });
    // Card mounts within a few poll ticks
    const card = page.locator('.question-card').first();
    await expect(card).toBeVisible({ timeout: 5000 });
    // Question block + radio options visible
    await expect(card.locator('.question-card__block')).toHaveCount(1);
    await expect(card.getByText("What's your favorite dummy color?")).toBeVisible();
    await expect(card.getByText('Bold, warm, attention-grabbing.')).toBeVisible();
    // data-tool-use-id is the question-signature fallback, NOT empty string
    const tid = await card.getAttribute('data-tool-use-id');
    expect(tid).toBeTruthy();
    expect(tid.length).toBeGreaterThan(0);
  });

  test('card carries valid tool_use_id when payload supplies one', async ({ page }) => {
    const pending = { ...PENDING_NO_TOOL_USE_ID, tool_use_id: 'toolu_realID123' };
    // Seed at least one conversation entry — refreshConversation early-bails
    // when entries.length === 0, which would skip the card mount.
    await setupAgent(page, {
      pending,
      conversation: [{ role: 'human', content: 'plan it', timestamp: '2026-06-04T10:00:00Z' }],
    });
    const card = page.locator('.question-card').first();
    await expect(card).toBeVisible({ timeout: 5000 });
    const tid = await card.getAttribute('data-tool-use-id');
    expect(tid).toBe('toolu_realID123');
  });

  test('Send Answer posts composed text and triggers card teardown', async ({ page }) => {
    const ctx = await setupAgent(page, {
      pending: PENDING_NO_TOOL_USE_ID,
      conversation: [{ role: 'human', content: 'plan it', timestamp: '2026-06-04T10:00:00Z' }],
    });
    const card = page.locator('.question-card').first();
    await expect(card).toBeVisible({ timeout: 5000 });
    // Pick the 'Blue' radio. The native input is visually replaced by a
    // custom .question-card__radio sibling, so it's not directly
    // clickable; set checked + dispatch input via evaluate so the
    // questionCardUpdate handler enables the submit button.
    await page.evaluate(() => {
      const r = document.querySelector('.question-card__radio-input[value="Blue"]');
      r.checked = true;
      r.dispatchEvent(new Event('input', { bubbles: true }));
    });
    // Click Send Answer
    const submit = card.locator('.question-card__submit');
    await expect(submit).toBeEnabled({ timeout: 2000 });
    await submit.click();
    // POST fires with composed text containing the picked option
    await expect.poll(() => ctx.inputPosts.length, { timeout: 3000 }).toBeGreaterThan(0);
    const post = ctx.inputPosts[0];
    expect(post.text).toContain('Color');
    expect(post.text).toContain('Blue');
    // Simulate the agent resolving the question — subsequent polls return null
    ctx.setPending(null);
    // Card disappears on the next poll tick
    await expect(card).toBeHidden({ timeout: 5000 });
  });
});
