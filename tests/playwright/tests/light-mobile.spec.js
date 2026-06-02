// @ts-check
// Phase C: light-mode mobile (~412px) iteration.
//
// Locks the three behavioural invariants identified by the grader for the
// mobile light-mode surface (see docs/design/verdict-light-phone-0.md):
//
//   1. Floating dock (.ui-dock) pill background must follow theme — in light
//      it renders as a white card with --border-default + --shadow-medium,
//      NOT the hard-coded dark glass blob from the dark register.
//   2. User chat bubble (.ui-msg--user .ui-msg__bubble) must invert in
//      light to a dark pill (--cta-bg / --cta-text) per flow-map A2,
//      otherwise it reads as a button competing with --bg-elevated chips.
//   3. Disabled Spawn button (.create-spawn:disabled) must remain visible
//      against the white page in light — outline or one-step-up surface,
//      not --bg-surface (which collapses into --bg-base = #FFFFFF).
//
// Dark mode is bit-exact preserved (see docs/design/light-preservation.md).
// We don't re-test dark here; the dark assertions live in
// desktop-redesign.spec.js + light-foundation.spec.js.

const { test, expect } = require('@playwright/test');

// ---------- helpers ----------

async function mockApi(page) {
  await page.route('**/events', (route) => route.abort('connectionrefused'));
  await page.route(/\/api\/agents/, async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    if (path === '/api/agents') {
      await route.fulfill({
        json: [
          {
            session_id: 'lm-001',
            cwd: '/Users/test/Code/myapp',
            branch: 'feat/light-mobile',
            model: 'opus',
            state: 'running',
            started_at: new Date().toISOString(),
            subagent_count: 0,
          },
        ],
      });
    } else if (path.endsWith('/conversation')) {
      await route.fulfill({
        json: [
          { Role: 'human', Content: 'Sample user prompt to render as a bubble.', Timestamp: new Date().toISOString() },
          { Role: 'assistant', Content: 'Sample reply.', Timestamp: new Date().toISOString() },
        ],
      });
    } else {
      await route.fulfill({ json: {} });
    }
  });
  await page.route('**/api/usage/ratelimit', (route) => route.fulfill({ json: {} }));
  await page.route('**/api/usage/daily*', (route) => route.fulfill({ json: { days: [] } }));
  await page.addInitScript(() => {
    try { sessionStorage.clear(); } catch {}
  });
}

async function gotoLightMobile(page) {
  await page.setViewportSize({ width: 412, height: 900 });
  await mockApi(page);
  await page.goto('/');
  await page.waitForSelector('#app-shell', { timeout: 5000 });
  await page.evaluate(async () => {
    const { Theme } = await import('/js/theme.js');
    localStorage.setItem('theme-preference', 'light');
    Theme.apply('light');
  });
  // Confirm light cascade actually settled.
  const theme = await page.evaluate(() => document.documentElement.dataset.theme);
  expect(theme).toBe('light');
}

function norm(s) {
  return (s || '').replace(/\s+/g, '').toLowerCase();
}

// ---------- 1. Floating dock background follows the light theme ----------

test.describe('Mobile light theme — floating dock', () => {
  test('floating .ui-dock background is the light surface, not the dark glass blob', async ({ page }) => {
    await gotoLightMobile(page);

    const dock = page.locator('.ui-dock').first();
    await expect(dock).toBeVisible();

    const bg = await dock.evaluate((el) => getComputedStyle(el).backgroundColor);
    // Must be opaque white (the surface) — explicitly NOT the dark glass
    // rgba(26, 26, 28, 0.88) that was hard-coded for dark mode.
    expect(norm(bg)).toBe(norm('rgb(255, 255, 255)'));
    expect(norm(bg)).not.toBe(norm('rgba(26, 26, 28, 0.88)'));
  });

  test('floating .ui-dock carries a 1px --border-default outline in light', async ({ page }) => {
    await gotoLightMobile(page);

    const dock = page.locator('.ui-dock').first();
    const borderColor = await dock.evaluate((el) => getComputedStyle(el).borderTopColor);
    const borderWidth = await dock.evaluate((el) => getComputedStyle(el).borderTopWidth);

    // --border-default in light = #E4E4E7 → rgb(228, 228, 231)
    expect(norm(borderColor)).toBe(norm('rgb(228, 228, 231)'));
    expect(borderWidth).toBe('1px');
  });
});

// ---------- 2. User chat bubble inverts in light ----------

test.describe('Mobile light theme — chat bubble inversion', () => {
  test('user-message bubble is a dark pill (--cta-bg) in light, not --bg-elevated', async ({ page }) => {
    await gotoLightMobile(page);

    // Inject a user message DOM node so the assertion does not depend on
    // mock conversation rendering through the full router. We're locking
    // the CSS rule, not the render path.
    await page.evaluate(() => {
      const host = document.body;
      const wrap = document.createElement('div');
      wrap.className = 'ui-msg ui-msg--user';
      wrap.style.position = 'fixed';
      wrap.style.top = '0';
      wrap.style.left = '0';
      wrap.style.zIndex = '9999';
      const bubble = document.createElement('div');
      bubble.className = 'ui-msg__bubble';
      bubble.id = '__test_user_bubble__';
      bubble.textContent = 'probe';
      wrap.appendChild(bubble);
      host.appendChild(wrap);
    });

    const bubble = page.locator('#__test_user_bubble__');
    const bg = await bubble.evaluate((el) => getComputedStyle(el).backgroundColor);
    const fg = await bubble.evaluate((el) => getComputedStyle(el).color);

    // --cta-bg in light = #1C1A17 → rgb(28, 26, 23)
    expect(norm(bg)).toBe(norm('rgb(28, 26, 23)'));
    // --cta-text in light = #FFFFFF → rgb(255, 255, 255)
    expect(norm(fg)).toBe(norm('rgb(255, 255, 255)'));
    // Negative: must NOT remain --bg-elevated (#F2F2F4 → rgb(242, 242, 244)).
    expect(norm(bg)).not.toBe(norm('rgb(242, 242, 244)'));
  });
});

// ---------- 3. Disabled Spawn button stays visible against the white page ----------

test.describe('Mobile light theme — create view disabled Spawn', () => {
  test('disabled .create-spawn has a visible 1px --border-default outline in light', async ({ page }) => {
    await gotoLightMobile(page);

    // Synthesize a disabled spawn button to lock the CSS rule — we don't
    // care about the create-view router here, only the rule.
    await page.evaluate(() => {
      const btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'create-spawn';
      btn.id = '__test_spawn__';
      btn.disabled = true;
      btn.textContent = 'Spawn';
      document.body.appendChild(btn);
    });

    const btn = page.locator('#__test_spawn__');
    const borderColor = await btn.evaluate((el) => getComputedStyle(el).borderTopColor);
    const borderWidth = await btn.evaluate((el) => getComputedStyle(el).borderTopWidth);

    // --border-default in light = #E4E4E7 → rgb(228, 228, 231)
    expect(norm(borderColor)).toBe(norm('rgb(228, 228, 231)'));
    expect(borderWidth).toBe('1px');
  });
});
