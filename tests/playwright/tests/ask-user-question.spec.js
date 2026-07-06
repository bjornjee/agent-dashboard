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
  let answerPosts = [];
  let pendingState = pending;
  let conversationState = conversation || [];
  await page.route('**/events', (route) => route.abort('connectionrefused'));
  await page.route(/\/api\//, async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    if (path === '/api/agents') return route.fulfill({ json: [agent] });
    if (path === `/api/agents/${AGENT_ID}/conversation`) {
      return route.fulfill({ json: conversationState });
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
    if (path === `/api/agents/${AGENT_ID}/answer-question` && route.request().method() === 'POST') {
      answerPosts.push(JSON.parse(route.request().postData() || '{}'));
      return route.fulfill({ json: { ok: 'answered' } });
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
    answerPosts,
    setPending(next) { pendingState = next; },
    setConversation(next) { conversationState = next; },
  };
}

function longConversation(n) {
  const out = [{ role: 'human', content: 'plan it', timestamp: '2026-06-04T10:00:00Z' }];
  for (let i = 0; i < n; i++) {
    out.push({
      role: 'assistant',
      content: 'Progress update ' + i + ' — still working through the plan, more scrollback filler text here.',
      timestamp: '2026-06-04T10:01:00Z',
    });
  }
  return out;
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

  test('Send fires from pointerdown alone — iOS keyboard-blur regression lock', async ({ page }) => {
    // On iOS Safari PWA, tapping Send while a freeform <input> has focus
    // blurs the input → dismisses the soft keyboard → reflows the viewport,
    // which moves the button off the touch point before `click` fires.
    // The fix is to wire the Send action on `pointerdown`, which fires
    // before that blur cascade. This test dispatches a pointerdown event
    // WITHOUT any preceding click and asserts the POST still fires —
    // proving the pointerdown path is wired independently of click.
    const ctx = await setupAgent(page, {
      pending: PENDING_NO_TOOL_USE_ID,
      conversation: [{ role: 'human', content: 'plan it', timestamp: '2026-06-04T10:00:00Z' }],
    });
    const card = page.locator('.question-card').first();
    await expect(card).toBeVisible({ timeout: 5000 });
    // Pick an option so submit becomes enabled.
    await page.evaluate(() => {
      const r = document.querySelector('.question-card__radio-input[value="Red"]');
      r.checked = true;
      r.dispatchEvent(new Event('input', { bubbles: true }));
    });
    await expect(card.locator('.question-card__submit')).toBeEnabled({ timeout: 2000 });
    // Dispatch a synthetic pointerdown event ONLY. No click(), no tap().
    // If the implementation still relies on inline onclick, this won't fire.
    await page.evaluate(() => {
      const btn = document.querySelector('.question-card__submit');
      btn.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, cancelable: true }));
    });
    await expect.poll(() => ctx.answerPosts.length, { timeout: 3000 }).toBeGreaterThan(0);
    // The POST must carry the picked option_index — not just any prior request.
    const post = ctx.answerPosts[0];
    expect(post.answers).toHaveLength(1);
    expect(post.answers[0].option_indices).toEqual([0]); // Red is index 0
    expect(post.option_counts).toEqual([3]);
  });

  test('multi-question payload renders carousel track + pager dots', async ({ page }) => {
    // Multi-question payload becomes a horizontal scroll-snap carousel on
    // mobile with one pager dot per question. Render-side assertions only
    // (CSS scroll-snap behavior is browser-native and out of scope here).
    const multi = {
      tool_use_id: 'toolu_multi',
      questions: [
        { header: 'A', question: 'Q one?', multi_select: false, options: [{ label: 'a1' }] },
        { header: 'B', question: 'Q two?', multi_select: false, options: [{ label: 'b1' }] },
        { header: 'C', question: 'Q three?', multi_select: false, options: [{ label: 'c1' }] },
      ],
    };
    await page.setViewportSize({ width: 390, height: 844 });
    await setupAgent(page, {
      pending: multi,
      conversation: [{ role: 'human', content: 'plan it', timestamp: '2026-06-04T10:00:00Z' }],
    });
    const card = page.locator('.question-card').first();
    await expect(card).toBeVisible({ timeout: 5000 });
    // Track wraps the blocks
    await expect(card.locator('.question-card__track')).toHaveCount(1);
    await expect(card.locator('.question-card__track .question-card__block')).toHaveCount(3);
    // One pager dot per question; only the first is active on mount
    await expect(card.locator('.question-card__pager-dot')).toHaveCount(3);
    await expect(card.locator('.question-card__pager-dot--active')).toHaveCount(1);
    await expect(card.locator('.question-card__pager-dot').first()).toHaveClass(/question-card__pager-dot--active/);
  });

  test('single-question payload renders without pager dots', async ({ page }) => {
    // The carousel still wraps the single block in __track (consistent
    // DOM shape), but the pager is omitted because there's nothing to
    // advance through.
    await page.setViewportSize({ width: 390, height: 844 });
    await setupAgent(page, {
      pending: PENDING_NO_TOOL_USE_ID,
      conversation: [{ role: 'human', content: 'plan it', timestamp: '2026-06-04T10:00:00Z' }],
    });
    const card = page.locator('.question-card').first();
    await expect(card).toBeVisible({ timeout: 5000 });
    await expect(card.locator('.question-card__track .question-card__block')).toHaveCount(1);
    await expect(card.locator('.question-card__pager')).toHaveCount(0);
  });

  test('Send Answer posts structured payload and triggers card teardown', async ({ page }) => {
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
    // POST fires with structured payload — option_index of the picked
    // Blue radio (index 1 in the original PENDING_NO_TOOL_USE_ID order),
    // plus the option count for "Other" digit computation server-side.
    await expect.poll(() => ctx.answerPosts.length, { timeout: 3000 }).toBeGreaterThan(0);
    const post = ctx.answerPosts[0];
    expect(post.answers).toHaveLength(1);
    expect(post.answers[0].option_indices).toEqual([1]); // Blue is index 1
    expect(post.answers[0].freeform).toBe('');
    expect(post.answers[0].multi).toBe(false);
    expect(post.option_counts).toEqual([3]);
    // Simulate the agent resolving the question — subsequent polls return null
    ctx.setPending(null);
    // Card disappears on the next poll tick
    await expect(card).toBeHidden({ timeout: 5000 });
  });
});

// Arrival + gate UX around the card. The card is the turn's workflow
// gate: it must be seen when it arrives, and the composer must not
// advertise a competing reply path (typed text into a native picker is
// silently dropped by the harness — see submitQuestionCard's comment).
test.describe('question-card arrival & composer gate', () => {
  test('mid-session card mount scrolls the card into view when at bottom', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    const ctx = await setupAgent(page, { pending: null, conversation: longConversation(24) });
    await page.waitForSelector('.conversation', { timeout: 5000 });
    await expect(page.locator('.question-card')).toHaveCount(0);
    // Initial load leaves the reader at the bottom; the question then
    // arrives mid-session on a poll tick.
    ctx.setPending(PENDING_NO_TOOL_USE_ID);
    const card = page.locator('.question-card').first();
    await expect(card).toBeVisible({ timeout: 5000 });
    await page.waitForTimeout(400); // let the rAF scroll settle
    const box = await card.boundingBox();
    expect(box).not.toBeNull();
    expect(box.y).toBeGreaterThanOrEqual(0);
    expect(box.y, 'card top should be pulled into the upper viewport, not below the fold').toBeLessThan(844 * 0.6);
  });

  test('composer is gated while a question is pending and released after', async ({ page }) => {
    const ctx = await setupAgent(page, {
      pending: PENDING_NO_TOOL_USE_ID,
      conversation: [{ role: 'human', content: 'plan it', timestamp: '2026-06-04T10:00:00Z' }],
    });
    await expect(page.locator('.question-card')).toBeVisible({ timeout: 5000 });
    const input = page.locator('#reply-input');
    await expect(input).toBeDisabled({ timeout: 3000 });
    await expect(input).toHaveAttribute('placeholder', /Answer the question card above/);
    await expect(page.locator('.ui-composer__send')).toBeDisabled();
    // Question resolves — the composer comes back with its normal placeholder.
    ctx.setPending(null);
    await expect(input).toBeEnabled({ timeout: 5000 });
    await expect(input).not.toHaveAttribute('placeholder', /Answer the question card above/);
    await expect(page.locator('.ui-composer__send')).toBeEnabled();
  });

  test('jump chip appears for new activity while scrolled up and jumps down', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    const convo = longConversation(24);
    const ctx = await setupAgent(page, { pending: null, conversation: convo });
    await page.waitForSelector('.conversation', { timeout: 5000 });
    await page.evaluate(() => { document.querySelector('.detail-scroll').scrollTop = 0; });
    ctx.setConversation([
      ...convo,
      { role: 'assistant', content: 'A brand new update arrives at the bottom.', timestamp: '2026-06-04T10:09:00Z' },
    ]);
    const chip = page.locator('.jump-latest');
    await expect(chip).toBeVisible({ timeout: 5000 });
    await expect(chip).toHaveText(/New activity/);
    await chip.click();
    await expect(chip).toHaveCount(0);
    const atBottom = await page.evaluate(() => {
      const s = document.querySelector('.detail-scroll');
      return s.scrollHeight - s.scrollTop - s.clientHeight < 60;
    });
    expect(atBottom).toBe(true);
  });
});
