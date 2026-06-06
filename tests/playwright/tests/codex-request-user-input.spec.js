// @ts-check
// E2E coverage for codex's request_user_input question card.
//
// Pins three claims the codex parity work depends on:
//
// 1. The existing question-card frontend (originally built for claude's
//    AskUserQuestion) renders unchanged for codex payloads. The only
//    schema difference is each question carries an optional `id` field
//    (codex needs it for answer routing); the frontend ignores `id` but
//    must not crash on it.
// 2. The dashboard polls /api/agents/{id}/pending-question for codex
//    agents — the previous behavior hard-returned null for harness
//    codex, so the card never mounted. This test fails fast if that
//    early-return creeps back in.
// 3. Clicking an option and submitting POSTs the same structured payload
//    (`{answers:[{option_indices, freeform, multi}], option_counts}`) to
//    /api/agents/{id}/answer-question that the claude flow uses. The
//    server-side picker driver is what differs per harness; the wire
//    contract is shared.
const { test, expect } = require('@playwright/test');

const AGENT_ID = 'codex-rui-test';

function makeCodexAgent(overrides) {
  return {
    session_id: AGENT_ID,
    cwd: '/Users/test/Code/myapp',
    branch: 'main',
    model: 'gpt-5.5',
    harness: 'codex',
    state: 'question',
    started_at: new Date().toISOString(),
    subagent_count: 0,
    last_hook_event: 'PreToolUse',
    current_tool: 'request_user_input',
    ...overrides,
  };
}

// Mirrors what internal/codex/conversation/parser.go emits: each
// question keeps codex's per-question `id` alongside the claude-style
// {question, header, multi_select, options} fields.
const CODEX_PENDING = {
  tool_use_id: 'call_codex_xyz',
  questions: [
    {
      id: 'pr_scope',
      question: 'How do you want to scope the web package refactor?',
      header: 'PR Scope',
      multi_select: false,
      options: [
        { label: 'Several smaller PRs (Recommended)', description: 'Keeps each diff reviewable.' },
        { label: 'One big PR', description: 'Finishes the cleanup in one branch.' },
      ],
    },
  ],
};

async function setupCodexAgent(page, { pending, conversation }) {
  const agent = makeCodexAgent();
  let answerPosts = [];
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
    if (path === `/api/agents/${AGENT_ID}/answer-question` && route.request().method() === 'POST') {
      answerPosts.push(JSON.parse(route.request().postData() || '{}'));
      // Clear pending on POST so the teardown path runs.
      pendingState = null;
      return route.fulfill({ json: { ok: 'answered' } });
    }
    return route.fulfill({ json: {} });
  });
  await page.goto('/');
  await page.waitForSelector('.ui-row, .ui-dock', { timeout: 5000 });
  await page.evaluate((id) => window.Dashboard.selectAgent(id), AGENT_ID);
  return {
    answerPosts,
    setPending(next) { pendingState = next; },
  };
}

test.describe('codex request_user_input card lifecycle', () => {
  test('card renders codex payload (with question id field) without crashing', async ({ page }) => {
    await setupCodexAgent(page, {
      pending: CODEX_PENDING,
      conversation: [{ role: 'human', content: 'do it now', timestamp: '2026-06-06T08:00:00Z' }],
    });
    const card = page.locator('.question-card').first();
    await expect(card).toBeVisible({ timeout: 5000 });
    await expect(card.getByText('How do you want to scope the web package refactor?')).toBeVisible();
    await expect(card.getByText('Several smaller PRs (Recommended)')).toBeVisible();
    await expect(card.getByText('One big PR')).toBeVisible();
    // tool_use_id is codex's call_id — passes through unchanged.
    const tid = await card.getAttribute('data-tool-use-id');
    expect(tid).toBe('call_codex_xyz');
  });

  test('Send Answer POSTs structured payload to /answer-question', async ({ page }) => {
    const ctx = await setupCodexAgent(page, {
      pending: CODEX_PENDING,
      conversation: [{ role: 'human', content: 'do it now', timestamp: '2026-06-06T08:00:00Z' }],
    });
    const card = page.locator('.question-card').first();
    await expect(card).toBeVisible({ timeout: 5000 });
    // Pick option 2 ("One big PR" — index 1).
    await page.evaluate(() => {
      const r = document.querySelector('.question-card__radio-input[value="One big PR"]');
      r.checked = true;
      r.dispatchEvent(new Event('input', { bubbles: true }));
    });
    await expect(card.locator('.question-card__submit')).toBeEnabled({ timeout: 2000 });
    await page.locator('.question-card__submit').click();
    await expect.poll(() => ctx.answerPosts.length, { timeout: 3000 }).toBeGreaterThan(0);
    const post = ctx.answerPosts[0];
    expect(post.answers).toHaveLength(1);
    expect(post.answers[0].option_indices).toEqual([1]);
    expect(post.option_counts).toEqual([2]);
  });

  test('card disappears after POST when pending-question clears', async ({ page }) => {
    // The codex hook clears pending_question on PostToolUse, mirroring
    // the claude flow. The frontend reconciles on the next poll tick.
    const ctx = await setupCodexAgent(page, {
      pending: CODEX_PENDING,
      conversation: [{ role: 'human', content: 'do it now', timestamp: '2026-06-06T08:00:00Z' }],
    });
    const card = page.locator('.question-card').first();
    await expect(card).toBeVisible({ timeout: 5000 });
    await page.evaluate(() => {
      const r = document.querySelector('.question-card__radio-input');
      r.checked = true;
      r.dispatchEvent(new Event('input', { bubbles: true }));
    });
    await page.locator('.question-card__submit').click();
    await expect.poll(() => ctx.answerPosts.length, { timeout: 3000 }).toBeGreaterThan(0);
    // setupCodexAgent flips pendingState to null on POST, so the next
    // poll tick (every 2s) tears down the card.
    await expect(page.locator('.question-card')).toHaveCount(0, { timeout: 5000 });
  });
});
