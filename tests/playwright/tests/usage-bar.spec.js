// @ts-check
const { test, expect } = require('@playwright/test');

// Set up API mocks and navigate to the usage view.
// The daily endpoint is delayed so that loadRateLimits completes second,
// ensuring the rate-limit card is not wiped by loadUsageData's innerHTML.
async function setupUsagePage(page) {
  await page.route('**/events', route => route.abort('connectionrefused'));

  await page.route(/\/api\/agents/, async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    if (path === '/api/agents') {
      await route.fulfill({ json: [
        { session_id: 'u-001', cwd: '/Code/myapp', branch: 'main', model: 'opus', state: 'running', started_at: new Date().toISOString(), subagent_count: 0 },
      ]});
    } else if (path.endsWith('/usage')) {
      await route.fulfill({ json: { InputTokens: 1000, OutputTokens: 500, CostUSD: 0.05 } });
    } else if (path.endsWith('/subagents')) {
      await route.fulfill({ json: [] });
    } else {
      await route.fulfill({ json: {} });
    }
  });

  await page.route('**/api/usage/ratelimit', route => {
    // Respond after daily so that insertAdjacentHTML runs after innerHTML is set
    setTimeout(() => {
      route.fulfill({ json: {
        session: { used_percent: 42.5, resets_at: '2026-12-31T20:00:00Z' },
        weekly: { used_percent: 81.0, resets_at: '2026-12-31T00:00:00Z' },
        plan: 'max_5',
      }});
    }, 200);
  });

  await page.route('**/api/usage/daily*', route => {
    route.fulfill({ json: {
      days: [{ date: '2026-04-14', cost_usd: 1.00, input_tokens: 1000, output_tokens: 500, cache_read_tokens: 0, cache_write_tokens: 0 }],
      today_cost: 1.00,
      total_cost: 10.00,
    }});
  });

  await page.addInitScript(() => sessionStorage.clear());
  await page.goto('/');
  await page.evaluate(() => sessionStorage.clear());
  await page.waitForSelector('.agent-card', { timeout: 5000 });

  // Navigate to usage view
  await page.click('button:has-text("Usage")');
  await page.waitForSelector('.rate-limit-card', { timeout: 5000 });
}

test.describe('Rate limit bar fill', () => {
  test('bar fill should have a visible (non-transparent) background color', async ({ page }) => {
    await setupUsagePage(page);

    const fills = page.locator('.rate-limit-bar-fill');
    const count = await fills.count();
    expect(count).toBeGreaterThanOrEqual(2);

    for (let i = 0; i < count; i++) {
      const fill = fills.nth(i);
      const bg = await fill.evaluate(el => getComputedStyle(el).backgroundColor);
      // The background must NOT be transparent — that means the CSS variable resolved
      expect(bg, `bar fill ${i} should not be transparent`).not.toBe('rgba(0, 0, 0, 0)');
    }
  });

  test('session bar fill width should reflect used_percent', async ({ page }) => {
    await setupUsagePage(page);

    const sessionFill = page.locator('.rate-limit-bar-fill').first();
    const width = await sessionFill.evaluate(el => el.style.width);
    expect(width).toBe('42.5%');
  });

  test('high-usage bar should use red/warning color, not green', async ({ page }) => {
    await setupUsagePage(page);

    // Weekly bar is 81% — should be red-ish
    const weeklyFill = page.locator('.rate-limit-bar-fill').nth(1);
    const bg = await weeklyFill.evaluate(el => getComputedStyle(el).backgroundColor);

    // Should not be transparent
    expect(bg).not.toBe('rgba(0, 0, 0, 0)');

    // Should not be the green color (rgb(16, 185, 129) = #10B981 or rgb(13, 148, 136) = #0D9488)
    expect(bg).not.toContain('16, 185, 129');
    expect(bg).not.toContain('13, 148, 136');
  });
});
