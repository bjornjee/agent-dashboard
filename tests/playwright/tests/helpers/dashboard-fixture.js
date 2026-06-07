// @ts-check

const DEFAULT_NOW = '2026-06-06T10:00:00.000Z';

function makeAgent(overrides = {}) {
  return {
    session_id: 'agent-001',
    cwd: '/Users/test/Code/myapp',
    branch: 'main',
    model: 'gpt-5',
    state: 'running',
    started_at: DEFAULT_NOW,
    updated_at: DEFAULT_NOW,
    subagent_count: 0,
    harness: 'codex',
    permission_mode: 'default',
    last_hook_event: 'Stop',
    files_changed: [],
    ...overrides,
  };
}

function makeUsage(overrides = {}) {
  return {
    InputTokens: 1000,
    OutputTokens: 500,
    CacheReadTokens: 0,
    CacheWriteTokens: 0,
    CostUSD: 0.05,
    ...overrides,
  };
}

function makeConversation(agentId) {
  return [
    { Role: 'human', Content: `hello ${agentId}`, Timestamp: DEFAULT_NOW },
    { Role: 'assistant', Content: 'ready', Timestamp: DEFAULT_NOW },
  ];
}

async function setupDashboard(page, options = {}) {
  const agents = options.agents || [makeAgent()];
  const mutable = {
    agents: [...agents],
    conversations: new Map(Object.entries(options.conversations || {})),
    pendingQuestions: new Map(Object.entries(options.pendingQuestions || {})),
    activity: new Map(Object.entries(options.activity || {})),
    plans: new Map(Object.entries(options.plans || {})),
    diffs: new Map(Object.entries(options.diffs || {})),
    usage: new Map(Object.entries(options.usage || {})),
    subagents: new Map(Object.entries(options.subagents || {})),
    prUrls: new Map(Object.entries(options.prUrls || {})),
    actionResponses: new Map(Object.entries(options.actionResponses || {})),
    skillsByHarness: options.skillsByHarness || {
      '': ['feature', 'fix'],
      codex: ['agent-dashboard:feature', 'agent-dashboard:fix'],
      claude: ['feature', 'implement'],
    },
    suggestions: options.suggestions || [],
    dailyUsage: options.dailyUsage || {
      days: [],
      today_cost: 0,
      total_cost: 0,
    },
    rateLimit: options.rateLimit || {
      session: { used_percent: 12.5, resets_at: '2026-12-31T20:00:00Z' },
      weekly: { used_percent: 40, resets_at: '2026-12-31T00:00:00Z' },
      plan: 'max_5',
    },
  };
  const requests = [];
  const eventRequests = [];
  const unhandledRequests = [];

  if (options.viewport) await page.setViewportSize(options.viewport);
  await page.addInitScript(({ clearStorage, sessionStorageValues, localStorageValues }) => {
    try {
      if (clearStorage) {
        localStorage.clear();
        sessionStorage.clear();
      }
      for (const [key, value] of Object.entries(sessionStorageValues || {})) {
        sessionStorage.setItem(key, value);
      }
      for (const [key, value] of Object.entries(localStorageValues || {})) {
        localStorage.setItem(key, value);
      }
    } catch {}
  }, {
    clearStorage: options.clearStorage !== false,
    sessionStorageValues: options.sessionStorage || {},
    localStorageValues: options.localStorage || {},
  });
  if (options.sse) {
    await page.addInitScript(() => {
      window.__dashboardEventSources = [];
      window.__dashboardEmitAgents = (agents) => {
        for (const source of window.__dashboardEventSources || []) {
          if (typeof source.onmessage === 'function') {
            source.onmessage({ data: JSON.stringify(agents) });
          }
        }
      };
      window.EventSource = class FakeEventSource {
        constructor(url) {
          this.url = url;
          this.readyState = 1;
          this.onmessage = null;
          this.onerror = null;
          window.__dashboardEventSources.push(this);
        }
        close() {
          this.readyState = 2;
          window.__dashboardEventSources = window.__dashboardEventSources.filter(source => source !== this);
        }
      };
    });
  }

  if (!options.sse) {
    await page.route('**/events', route => {
      eventRequests.push({ method: route.request().method(), path: new URL(route.request().url()).pathname });
      return route.abort('connectionrefused');
    });
  }
  await page.route(/\/api\//, async route => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;
    const method = request.method();

    let postData = null;
    const raw = request.postData();
    if (raw) {
      try { postData = JSON.parse(raw); } catch { postData = raw; }
    }
    requests.push({ method, path, postData, search: url.search });

    if (path === '/api/agents') return route.fulfill({ json: mutable.agents });
    if (path === '/api/skills') {
      const harness = url.searchParams.get('harness') || '';
      return route.fulfill({ json: mutable.skillsByHarness[harness] || [] });
    }
    if (path === '/api/suggestions') return route.fulfill({ json: mutable.suggestions });
    if (path === '/api/usage/daily') return route.fulfill({ json: mutable.dailyUsage });
    if (path === '/api/usage/ratelimit') return route.fulfill({ json: mutable.rateLimit });
    if (path === '/api/file-picker' && method === 'POST') {
      return route.fulfill({ json: { path: options.filePickerPath || '/tmp/example.txt' } });
    }
    if (path === '/api/agents/create' && method === 'POST') {
      const response = mutable.actionResponses.get(path) || { ok: true, session_id: 'created-agent' };
      return route.fulfill({ status: response.status || 200, json: response.body || response });
    }

    const match = path.match(/^\/api\/agents\/([^/]+)(?:\/([^/]+))?$/);
    if (match) {
      const id = decodeURIComponent(match[1]);
      const action = match[2] || '';
      const agent = mutable.agents.find(a => a.session_id === id);
      if (!agent) return route.fulfill({ status: 404, json: { error: 'agent not found' } });

      if (!action) return route.fulfill({ json: agent });
      if (action === 'conversation') {
        return route.fulfill({ json: mutable.conversations.get(id) || makeConversation(id) });
      }
      if (action === 'activity') return route.fulfill({ json: mutable.activity.get(id) || [] });
      if (action === 'pending-question') {
        return route.fulfill({ json: mutable.pendingQuestions.has(id) ? mutable.pendingQuestions.get(id) : null });
      }
      if (action === 'plan') return route.fulfill({ json: { content: mutable.plans.get(id) || '' } });
      if (action === 'diff') return route.fulfill({ json: mutable.diffs.get(id) || { diff: '' } });
      if (action === 'usage') return route.fulfill({ json: mutable.usage.get(id) || makeUsage() });
      if (action === 'subagents') return route.fulfill({ json: mutable.subagents.get(id) || [] });
      if (action === 'pr-url') return route.fulfill({ json: { url: mutable.prUrls.get(id) || agent.pr_url || '' } });
      if (['approve', 'reject', 'input', 'answer-question', 'stop', 'close', 'merge', 'cleanup'].includes(action) && method === 'POST') {
        const response = mutable.actionResponses.get(path) || mutable.actionResponses.get(action) || { ok: true };
        return route.fulfill({ status: response.status || 200, json: response.body || response });
      }
    }

    const unhandled = { method, path, search: url.search, postData };
    unhandledRequests.push(unhandled);
    await route.fulfill({ status: 599, json: { error: 'unhandled mocked API route', ...unhandled } });
    throw new Error(`Unhandled mocked API route: ${method} ${path}${url.search}`);
  });

  await page.goto(options.path || '/');
  await page.waitForSelector('#app .page-layout, #app .detail-layout, #app .create-shell, #app .usage-shell, #app-sidebar .app-sidebar__row', { timeout: 5000 });

  return {
    requests,
    mutable,
    setAgents(nextAgents) {
      mutable.agents = [...nextAgents];
    },
    setPendingQuestion(agentId, pending) {
      mutable.pendingQuestions.set(agentId, pending);
    },
    async emitAgents(nextAgents) {
      mutable.agents = [...nextAgents];
      await page.evaluate((snapshot) => window.__dashboardEmitAgents(snapshot), mutable.agents);
    },
    getRequests(path) {
      return requests.filter(r => !path || r.path === path);
    },
    eventRequests() {
      return [...eventRequests];
    },
    postRequests(path) {
      return requests.filter(r => r.method === 'POST' && (!path || r.path === path));
    },
    unhandledRequests() {
      return [...unhandledRequests];
    },
    async openCreate() {
      if (await page.locator('#app-sidebar').isVisible()) {
        await page.locator('#app-sidebar .ui-row', { hasText: 'New agent' }).click();
      } else {
        await page.locator('.ui-dock__cta', { hasText: '+ New' }).click();
      }
      await page.waitForSelector('.create-shell', { timeout: 5000 });
    },
    async openUsage() {
      if (await page.locator('#app-sidebar').isVisible()) {
        await page.locator('#app-sidebar .ui-row', { hasText: 'Usage' }).click();
      } else {
        await page.evaluate(() => window.Dashboard.openKebab());
        await page.locator('.ui-sheet__item', { hasText: 'Usage' }).click();
      }
      await page.waitForSelector('.usage-view', { timeout: 5000 });
    },
    async selectAgent(agentId) {
      if (await page.locator('#app-sidebar').isVisible()) {
        await page.click(`#app-sidebar [data-agent-id="${agentId}"] .ui-row`);
      } else {
        await page.evaluate((id) => window.Dashboard.selectAgent(id), agentId);
      }
      await page.waitForSelector('.detail-layout', { timeout: 5000 });
    },
    async confirmModal(title) {
      if (title) await page.locator('.modal-title').waitFor({ state: 'visible', timeout: 5000 });
      if (title) {
        const actual = await page.locator('.modal-title').textContent();
        if (actual !== title) throw new Error(`Expected modal ${title}, got ${actual}`);
      }
      await page.locator('#modal-confirm').click();
    },
  };
}

module.exports = {
  makeAgent,
  makeUsage,
  setupDashboard,
};
