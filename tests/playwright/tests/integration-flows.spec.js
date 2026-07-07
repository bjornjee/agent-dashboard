// @ts-check
const { test, expect } = require('@playwright/test');
const {
  makeAgent,
  setupDashboard,
} = require('./helpers/dashboard-fixture');

const mobile = { width: 390, height: 844 };
const desktop = { width: 1280, height: 800 };

const PENDING_QUESTION = {
  tool_use_id: 'toolu_integration',
  questions: [
    {
      question: 'Pick scope',
      header: 'Scope',
      multi_select: false,
      options: [
        { label: 'Small', description: 'One narrow change.' },
        { label: 'Large', description: 'Broader coverage.' },
      ],
    },
  ],
};

const PENDING_MULTI_QUESTION = {
  tool_use_id: 'toolu_multi_integration',
  questions: [
    {
      question: 'Pick targets',
      header: 'Targets',
      multi_select: true,
      options: [{ label: 'API' }, { label: 'UI' }, { label: 'PWA' }],
    },
    {
      question: 'Add notes',
      header: 'Notes',
      multi_select: false,
      options: [],
    },
  ],
};

test.describe('dashboard integration flows', () => {
  test('list rows map to fixture agents and navigate to matching detail views', async ({ page }) => {
    const agents = [
      makeAgent({ session_id: 'nav-run', cwd: '/Code/alpha', branch: 'feat/alpha', state: 'running' }),
      makeAgent({ session_id: 'nav-perm', cwd: '/Code/beta', branch: 'feat/beta', state: 'permission' }),
      makeAgent({ session_id: 'nav-pr', cwd: '/Code/gamma', branch: 'feat/gamma', state: 'pr', pr_url: 'https://github.com/x/y/pull/9' }),
      makeAgent({ session_id: 'nav-merged', cwd: '/Code/delta', branch: 'feat/delta', state: 'merged' }),
    ];

    await setupDashboard(page, { agents, viewport: mobile });

    await expect(page.locator('.page-scroll .ui-row')).toHaveCount(agents.length);
    for (const group of ['RUNNING', 'BLOCKED', 'PR', 'MERGED']) {
      await expect(page.locator('.ui-section-label', { hasText: group })).toBeVisible();
    }

    for (const agent of agents) {
      await page.locator('.page-scroll .ui-row', { hasText: agent.cwd.split('/').pop() }).click();
      await expect(page.locator('body.view-detail')).toBeVisible();
      await expect(page.locator('.detail-layout')).toContainText(agent.branch);
      await expect(page.locator('.detail-layout')).toContainText(agent.cwd.split('/').pop());
      await page.evaluate(() => window.Dashboard.showList());
      await expect(page.locator('.page-scroll .ui-row')).toHaveCount(agents.length);
      await expect(page.locator('.ui-dock')).toHaveCount(1);
    }
  });

  test('empty list still exposes primary creation path', async ({ page }) => {
    const ctx = await setupDashboard(page, { agents: [], viewport: mobile });

    await expect(page.locator('.empty-state-title')).toHaveText('No agents');
    await expect(page.locator('.ui-dock')).toHaveCount(1);
    await ctx.openCreate();
    await expect(page.locator('.create-shell')).toBeVisible();
  });

  test('search results obey query intent and Enter opens the selected agent', async ({ page }) => {
    const agents = [
      makeAgent({ session_id: 'search-api', cwd: '/Code/api', branch: 'main' }),
      makeAgent({ session_id: 'search-web', cwd: '/Code/web-dashboard', branch: 'feat/search' }),
      makeAgent({ session_id: 'search-worker', cwd: '/Code/worker', branch: 'fix/queue' }),
    ];
    await setupDashboard(page, { agents, viewport: mobile });

    await page.click('.ui-dock__search');
    await page.fill('#search-overlay-input', 'feat');
    const rows = page.locator('.search-overlay__row');
    await expect(rows).toHaveCount(1);
    await expect(rows.first()).toHaveAttribute('data-agent-id', 'search-web');
    await expect(rows.first()).toContainText(/web-dashboard|feat\/search/);

    await page.keyboard.press('Escape');
    await expect(page.locator('#search-overlay-root')).toHaveCount(0);
    await expect(page.locator('body.view-detail')).toHaveCount(0);

    await page.keyboard.press('Meta+k');
    await page.fill('#search-overlay-input', 'api');
    await page.keyboard.press('Enter');
    await expect(page.locator('#search-overlay-root')).toHaveCount(0);
    await expect(page.locator('body.view-detail')).toBeVisible();
  });

  test('create-agent validation and spawn payload follow form rules', async ({ page }) => {
    const ctx = await setupDashboard(page, {
      agents: [
        makeAgent({ session_id: 'create-a', cwd: '/Code/known', state: 'running' }),
        makeAgent({ session_id: 'create-b', cwd: '/Code/known', state: 'merged' }),
      ],
      suggestions: ['/Code/suggested'],
      harnessOptions: {
        models: ['gpt-5.5'],
        efforts: ['high'],
        default_model: { model: 'gpt-5.5', source: '~/.codex/config.toml' },
        default_effort: { effort: 'high', source: '~/.codex/config.toml' },
      },
      viewport: desktop,
    });

    await ctx.openCreate();

    const spawn = page.locator('#create-spawn');
    await expect(spawn).toBeDisabled();
    await expect(page.locator('#folder-hint')).toContainText('Pick a folder');

    await page.fill('#create-folder', 'relative/path');
    await expect(spawn).toBeDisabled();
    await expect(page.locator('#folder-hint')).toContainText('Path should be absolute');

    await page.fill('#create-folder', '/Code/new-project');
    await expect(spawn).toBeEnabled();

    await page.locator('.create-action', { hasText: 'known' }).click();
    await expect(page.locator('#create-folder')).toHaveValue('/Code/known');
    await expect(page.locator('#folder-hint')).toContainText('Known folder');

    await page.selectOption('#create-harness', 'codex');
    await expect(page.locator('#create-model-hint')).toContainText('Default: gpt-5.5');
    await expect(page.locator('#create-effort-hint')).toContainText('Default: high');
    await expect(page.locator('#create-skill option')).toContainText(['Skill', 'agent-dashboard:feature', 'agent-dashboard:fix']);
    await page.selectOption('#create-skill', 'agent-dashboard:feature');
    await page.fill('#create-message', 'Add integration tests');
    await spawn.click();

    await expect.poll(() => ctx.postRequests('/api/agents/create').length).toBe(1);
    expect(ctx.postRequests('/api/agents/create')[0].postData).toEqual({
      folder: '/Code/known',
      skill: 'agent-dashboard:feature',
      message: 'Add integration tests',
      harness: 'codex',
      model: '',
      effort: '',
    });
  });

  test('detail actions post to their intended endpoints once', async ({ page }) => {
    const agents = [
      makeAgent({ session_id: 'action-perm', cwd: '/Code/perm', state: 'permission' }),
      makeAgent({ session_id: 'action-pr', cwd: '/Code/pr', state: 'pr', pinned_state: 'pr', pr_url: 'https://github.com/test/repo/pull/1' }),
      makeAgent({ session_id: 'action-run', cwd: '/Code/run', state: 'running' }),
    ];
    const ctx = await setupDashboard(page, { agents, viewport: desktop });

    await page.click('#app-sidebar [data-agent-id="action-perm"] .ui-row');
    await page.locator('.action-panel button', { hasText: 'Approve' }).click();
    await expect.poll(() => ctx.postRequests('/api/agents/action-perm/approve').length).toBe(1);

    await page.locator('#reply-input').fill('continue with tests');
    await page.keyboard.press('Enter');
    await expect.poll(() => ctx.postRequests('/api/agents/action-perm/input').length).toBe(1);
    expect(ctx.postRequests('/api/agents/action-perm/input')[0].postData).toEqual({ text: 'continue with tests' });

    await page.locator('.action-panel button', { hasText: 'Reject' }).click();
    await expect.poll(() => ctx.postRequests('/api/agents/action-perm/reject').length).toBe(1);

    await page.click('#app-sidebar [data-agent-id="action-run"] .ui-row');
    await page.getByRole('button', { name: 'Stop' }).click();
    await expect(page.locator('.modal-title')).toHaveText('Stop Agent');
    await page.locator('#modal-confirm').click();
    await expect.poll(() => ctx.postRequests('/api/agents/action-run/stop').length).toBe(1);

    await page.click('#app-sidebar [data-agent-id="action-pr"] .ui-row');
    await page.locator('.action-panel button', { hasText: 'Merge' }).click();
    await expect(page.locator('.modal-title')).toHaveText('Merge PR');
    await page.locator('#modal-confirm').click();
    await expect.poll(() => ctx.postRequests('/api/agents/action-pr/merge').length).toBe(1);
    await expect(page.locator('.modal-title')).toHaveText('Post-Merge Cleanup');
    await page.locator('#modal-confirm').click();
    await expect.poll(() => ctx.postRequests('/api/agents/action-pr/cleanup').length).toBe(1);
  });

  test('close, file attach, Open PR, fallback PR URL, and action failure all surface through the UI', async ({ page }) => {
    await page.addInitScript(() => {
      window.__openedUrls = [];
      window.open = (url) => { window.__openedUrls.push(url); };
    });
    const agents = [
      makeAgent({ session_id: 'misc-merged', cwd: '/Code/merged', state: 'merged' }),
      makeAgent({ session_id: 'misc-pr-direct', cwd: '/Code/pr-direct', state: 'running', pinned_state: 'pr', pr_url: 'https://github.com/test/repo/pull/7' }),
      makeAgent({ session_id: 'misc-pr-fallback', cwd: '/Code/pr-fallback', state: 'running', pinned_state: 'pr' }),
      makeAgent({ session_id: 'misc-fail', cwd: '/Code/fail', state: 'permission' }),
    ];
    const ctx = await setupDashboard(page, {
      agents,
      prUrls: { 'misc-pr-fallback': 'https://github.com/test/repo/pull/8' },
      filePickerPath: '/tmp/attached-plan.md',
      actionResponses: {
        '/api/agents/misc-fail/approve': { ok: false, error: 'approval denied' },
      },
      viewport: desktop,
    });

    await ctx.selectAgent('misc-merged');
    await page.locator('.action-panel button', { hasText: 'Close' }).click();
    await ctx.confirmModal('Close Agent');
    await expect.poll(() => ctx.postRequests('/api/agents/misc-merged/close').length).toBe(1);

    await ctx.selectAgent('misc-pr-direct');
    await page.getByRole('button', { name: 'Attach file' }).click();
    await expect(page.locator('#reply-input')).toHaveValue('/tmp/attached-plan.md ');
    await page.locator('.action-panel button', { hasText: 'Open PR' }).click();
    expect(await page.evaluate(() => window.__openedUrls)).toContain('https://github.com/test/repo/pull/7');

    await ctx.selectAgent('misc-pr-fallback');
    await page.locator('.action-panel button', { hasText: 'Open PR' }).click();
    await expect.poll(() => ctx.getRequests('/api/agents/misc-pr-fallback/pr-url').length).toBeGreaterThan(0);
    await expect.poll(() => page.evaluate(() => window.__openedUrls)).toContain('https://github.com/test/repo/pull/8');

    await ctx.selectAgent('misc-fail');
    await page.locator('.action-panel button', { hasText: 'Approve' }).click();
    await expect(page.locator('.ui-toast--error')).toContainText('approval denied');
  });

  test('question-card answers post structured option metadata and clear on poll', async ({ page }) => {
    const agent = makeAgent({ session_id: 'question-flow', cwd: '/Code/question', state: 'question', current_tool: 'request_user_input' });
    const ctx = await setupDashboard(page, {
      agents: [agent],
      pendingQuestions: { 'question-flow': PENDING_QUESTION },
      viewport: desktop,
    });

    await page.click('#app-sidebar [data-agent-id="question-flow"] .ui-row');
    const card = page.locator('.question-card').first();
    await expect(card).toBeVisible();
    await page.evaluate(() => {
      const input = document.querySelector('.question-card__radio-input[value="Large"]');
      input.checked = true;
      input.dispatchEvent(new Event('input', { bubbles: true }));
    });
    await expect(card.locator('.question-card__submit')).toBeEnabled();
    await card.locator('.question-card__submit').click();

    await expect.poll(() => ctx.postRequests('/api/agents/question-flow/answer-question').length).toBe(1);
    expect(ctx.postRequests('/api/agents/question-flow/answer-question')[0].postData).toEqual({
      answers: [{ option_indices: [1], freeform: '', multi: false }],
      option_counts: [2],
    });

    ctx.setPendingQuestion('question-flow', null);
    await page.evaluate(async () => {
      const mod = await import('/js/pages/detail.js');
      await mod.refreshActiveTab('question-flow', { session_id: 'question-flow', state: 'question' });
    });
    await expect(page.locator('.question-card')).toHaveCount(0);
  });

  test('question-card supports multi-select and freeform-only answers', async ({ page }) => {
    const agent = makeAgent({ session_id: 'question-multi', cwd: '/Code/question-multi', state: 'question', current_tool: 'AskUserQuestion' });
    const ctx = await setupDashboard(page, {
      agents: [agent],
      pendingQuestions: { 'question-multi': PENDING_MULTI_QUESTION },
      viewport: desktop,
    });

    await ctx.selectAgent('question-multi');
    await expect(page.locator('.question-card')).toBeVisible();
    await page.evaluate(() => {
      for (const value of ['API', 'PWA']) {
        const input = document.querySelector(`.question-card__radio-input[value="${value}"]`);
        input.checked = true;
        input.dispatchEvent(new Event('input', { bubbles: true }));
      }
      const free = document.querySelector('.question-card__block[data-qi="1"] .question-card__answer-input');
      free.value = 'Ship the high-signal paths first';
      free.dispatchEvent(new Event('input', { bubbles: true }));
    });
    await page.locator('.question-card__submit').click();

    await expect.poll(() => ctx.postRequests('/api/agents/question-multi/answer-question').length).toBe(1);
    expect(ctx.postRequests('/api/agents/question-multi/answer-question')[0].postData).toEqual({
      answers: [
        { option_indices: [0, 2], freeform: '', multi: true },
        { option_indices: [], freeform: 'Ship the high-signal paths first', multi: false },
      ],
      option_counts: [3, 0],
    });
  });

  test('usage view reflects response percentages and totals by rule', async ({ page }) => {
    const ctx = await setupDashboard(page, {
      agents: [makeAgent({ session_id: 'usage-a', cwd: '/Code/usage' })],
      dailyUsage: {
        today_cost: 2.5,
        total_cost: 9,
        days: [
          { date: '2026-06-05', cost_usd: 1.5, input_tokens: 1000, output_tokens: 500 },
          { date: '2026-06-06', cost_usd: 2.5, input_tokens: 2000, output_tokens: 750 },
        ],
      },
      rateLimit: {
        session: { used_percent: 42.5, resets_at: '2026-12-31T20:00:00Z' },
        weekly: { used_percent: 105, resets_at: '2026-12-31T00:00:00Z' },
        plan: 'max_5',
      },
      viewport: mobile,
    });

    await ctx.openUsage();
    const rateFills = page.locator('.usage-rate__fill');
    await expect(rateFills).toHaveCount(2);
    const widths = await rateFills.evaluateAll(nodes => nodes.map(n => n.style.width));
    expect(widths).toEqual(['42.5%', '100%']);
    const metrics = page.locator('.usage-metric');
    await expect(metrics.nth(0)).toContainText('Today');
    await expect(metrics.nth(0)).toContainText('$2.50');
    await expect(metrics.nth(1)).toContainText('This period');
    await expect(metrics.nth(1)).toContainText('$4.00');
    await expect(metrics.nth(2)).toContainText('All time');
    await expect(metrics.nth(2)).toContainText('$9.00');
  });

  test('live agent updates preserve focused composer text while refreshing sidebar state', async ({ page }) => {
    const agent = makeAgent({ session_id: 'live-a', cwd: '/Code/live', state: 'running' });
    const ctx = await setupDashboard(page, { agents: [agent], viewport: desktop, sse: true });

    await page.click('#app-sidebar [data-agent-id="live-a"] .ui-row');
    await page.locator('#reply-input').fill('do not lose this');

    await ctx.emitAgents([
      makeAgent({ ...agent, state: 'running', pinned_state: 'pr', pr_url: 'https://github.com/test/repo/pull/5' }),
    ]);

    await expect(page.locator('#reply-input')).toHaveValue('do not lose this');
    await expect(page.locator('#app-sidebar [data-agent-id="live-a"]')).toContainText('PR open');
  });
});
