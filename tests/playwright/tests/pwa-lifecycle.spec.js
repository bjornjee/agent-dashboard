// @ts-check
const { test, expect } = require('@playwright/test');
const {
  makeAgent,
  setupDashboard,
} = require('./helpers/dashboard-fixture');

const mobile = { width: 390, height: 844 };
const desktop = { width: 1280, height: 800 };
const iPhoneUA = 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1';

test.describe('PWA lifecycle flows', () => {
  test.use({ serviceWorkers: 'allow' });

  test('manifest shortcut opens create view and removes only the action query param', async ({ page }) => {
    await setupDashboard(page, {
      agents: [makeAgent({ session_id: 'shortcut-a', cwd: '/Code/shortcut' })],
      path: '/?action=new-agent&keep=1#frag',
      sessionStorage: {
        'dashboard-view': JSON.stringify({ view: 'detail', agentId: 'shortcut-a' }),
      },
      viewport: desktop,
    });

    await expect(page.locator('.create-shell')).toBeVisible();
    expect(new URL(page.url()).search).toBe('?keep=1');
    expect(new URL(page.url()).hash).toBe('#frag');
  });

  test('install prompt lifecycle toggles install availability by rule', async ({ page }) => {
    await setupDashboard(page, {
      agents: [makeAgent({ session_id: 'install-a', cwd: '/Code/install' })],
      viewport: desktop,
    });

    const result = await page.evaluate(async () => {
      let promptCalls = 0;
      const event = new Event('beforeinstallprompt');
      event.prompt = () => { promptCalls += 1; };
      event.userChoice = Promise.resolve({ outcome: 'accepted' });
      window.dispatchEvent(event);

      const offered = document.body.classList.contains('can-install');
      const accepted = await window.Dashboard.installApp();
      const clearedAfterPrompt = !document.body.classList.contains('can-install');

      const second = new Event('beforeinstallprompt');
      second.prompt = () => {};
      second.userChoice = Promise.resolve({ outcome: 'dismissed' });
      window.dispatchEvent(second);
      window.dispatchEvent(new Event('appinstalled'));

      return {
        offered,
        accepted,
        promptCalls,
        clearedAfterPrompt,
        clearedAfterInstalled: !document.body.classList.contains('can-install'),
      };
    });

    expect(result).toEqual({
      offered: true,
      accepted: true,
      promptCalls: 1,
      clearedAfterPrompt: true,
      clearedAfterInstalled: true,
    });
  });

  test('iOS install hint appears once and persists dismissal', async ({ browser }) => {
    const context = await browser.newContext({
      serviceWorkers: 'allow',
      userAgent: iPhoneUA,
      viewport: mobile,
    });
    const page = await context.newPage();
    try {
      await setupDashboard(page, {
        agents: [makeAgent({ session_id: 'ios-a', cwd: '/Code/ios' })],
        clearStorage: false,
        viewport: mobile,
      });

      await expect(page.locator('.modal-title')).toHaveText('Install Agent Dashboard');
      await page.locator('#modal-confirm').click();
      await expect(page.locator('.modal-overlay')).toHaveCount(0);

      await page.reload();
      await page.waitForSelector('.page-layout, .create-shell, #app-sidebar .app-sidebar__row', { timeout: 5000 });
      await expect(page.locator('.modal-title')).toHaveCount(0);
      expect(await page.evaluate(() => localStorage.getItem('ios-install-dismissed'))).toBe('1');
    } finally {
      await context.close();
    }
  });

  test('service worker caches static shell without storing live API routes', async ({ page }) => {
    await setupDashboard(page, {
      agents: [makeAgent({ session_id: 'sw-a', cwd: '/Code/sw' })],
      viewport: desktop,
    });

    await page.evaluate(async () => {
      await navigator.serviceWorker.ready;
    });

    const cacheState = await page.evaluate(async () => {
      const keys = await caches.keys();
      const cacheName = keys.find(k => k.startsWith('agent-dashboard-v'));
      const cache = cacheName ? await caches.open(cacheName) : null;
      const cachedUrls = cache ? (await cache.keys()).map(req => new URL(req.url).pathname).sort() : [];
      const shell = cache ? await cache.match('/') : null;
      return {
        cacheName,
        hasShell: cachedUrls.includes('/') && cachedUrls.includes('/app.js') && cachedUrls.includes('/manifest.json'),
        shellReadable: !!shell && (await shell.text()).includes('Agent Dashboard'),
        hasApi: cachedUrls.some(path => path.startsWith('/api/') || path === '/events'),
      };
    });

    expect(cacheState.cacheName).toMatch(/^agent-dashboard-v/);
    expect(cacheState.hasShell).toBe(true);
    expect(cacheState.shellReadable).toBe(true);
    expect(cacheState.hasApi).toBe(false);
  });

  test('service worker navigate-agent messages open the matching agent detail', async ({ page }) => {
    const agent = makeAgent({ session_id: 'notify-a', cwd: '/Code/notify' });
    await setupDashboard(page, {
      agents: [agent],
      viewport: desktop,
    });

    await page.evaluate((agentId) => {
      navigator.serviceWorker.dispatchEvent(new MessageEvent('message', {
        data: { type: 'navigate-agent', agentId },
      }));
    }, agent.session_id);

    await expect(page.locator('body.view-detail')).toBeVisible();
    await expect(page.locator('.detail-layout')).toContainText('notify');
  });
});
