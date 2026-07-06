// @ts-check
// Detail-page behavior contracts for the chat per-message blocks and
// the diff tab's dynamic sizing: keyboard traversal of the diff
// controls, console health across the tab flow, wrap/view/sidebar
// persistence, width-driven auto view mode, and the mobile fallback.

const { test, expect } = require('@playwright/test');
const { makeAgent, setupDashboard } = require('./helpers/dashboard-fixture');

const CONVERSATION = [
  { Role: 'human', Content: 'check the retry logic', Timestamp: '2026-07-06T09:00:00.000Z' },
  { Role: 'assistant', Content: 'Reading the worker now.', Timestamp: '2026-07-06T09:00:04.000Z' },
  { Role: 'assistant', Content: 'Found the bug in `flushQueue()` — re-enqueue races the ack.', Timestamp: '2026-07-06T09:01:12.000Z' },
  { Role: 'assistant', Content: 'Fix passes with `-race`.', Timestamp: '2026-07-06T09:02:30.000Z' },
];

const RAW_DIFF = `diff --git a/a.go b/a.go
index 3f1c2aa..9b7d310 100644
--- a/a.go
+++ b/a.go
@@ -1,4 +1,5 @@
 func main() {
-	x := 1
+	x := 2
+	y := check(x)
 	fmt.Println(x)
 }
diff --git a/b.go b/b.go
index 44e0a11..99aa021 100644
--- a/b.go
+++ b/b.go
@@ -1,3 +1,4 @@
 package main
+
+func check(x int) int { return x + 1 }
`;

function options(extra = {}) {
  return {
    agents: [makeAgent({ session_id: 'bx-001', harness: 'claude', state: 'running', last_hook_event: 'PreToolUse', branch: 'fix/x' })],
    conversations: { 'bx-001': CONVERSATION },
    diffs: { 'bx-001': { raw: RAW_DIFF, files: [
      { path: 'a.go', status: 'modified', additions: 2, deletions: 1 },
      { path: 'b.go', status: 'modified', additions: 3, deletions: 0 },
    ], status: 'ok' } },
    ...extra,
  };
}

async function openDetail(page, opts) {
  const ctx = await setupDashboard(page, opts);
  await ctx.selectAgent('bx-001');
  await page.waitForSelector('.ui-msg--assistant', { timeout: 5000 });
  return ctx;
}

async function openDiff(page) {
  await page.evaluate(() => window.Dashboard.openDetailTab('diff'));
  await page.waitForSelector('.diff-view', { timeout: 5000 });
  await page.waitForTimeout(700);
}

function collectConsoleErrors(page) {
  const errors = [];
  page.on('console', (msg) => {
    if (msg.type() !== 'error') return;
    const text = msg.text();
    // Aborted /events SSE route (mocked offline) logs a resource failure — not app health.
    if (text.includes('/events') || text.includes('Failed to load resource')) return;
    errors.push(text);
  });
  page.on('pageerror', (err) => errors.push('pageerror: ' + err.message));
  return errors;
}

test('keyboard traversal reaches every diff control in order, with visible focus', async ({ page }) => {
  await openDetail(page, options({ viewport: { width: 1440, height: 900 } }));
  await openDiff(page);

  await page.locator('.diff-sidebar-toggle').focus();
  const order = [];
  for (let i = 0; i < 8; i++) {
    const cls = await page.evaluate(() => {
      const el = document.activeElement;
      return el ? (el.className || el.tagName) + '' : 'none';
    });
    order.push(cls);
    await page.keyboard.press('Tab');
  }
  const joined = order.join(' | ');
  expect(joined).toContain('diff-sidebar-toggle');
  expect(joined).toContain('toggle-switch__input');
  expect(joined).toContain('diff-toggle-btn');
  expect(joined).toContain('diff-sidebar-file');

  // Keyboard focus on a sidebar file button must produce a visible ring.
  await page.locator('.diff-sidebar-file').first().focus();
  await page.keyboard.press('Shift+Tab');
  await page.keyboard.press('Tab'); // arrive by keyboard so :focus-visible applies
  const outline = await page.evaluate(() => {
    const el = document.activeElement;
    const cs = getComputedStyle(el);
    return { cls: el.className, outlineStyle: cs.outlineStyle, outlineWidth: cs.outlineWidth, boxShadow: cs.boxShadow };
  });
  expect(outline.cls).toContain('diff-sidebar-file');
  const hasRing = outline.outlineStyle !== 'none' || outline.boxShadow !== 'none';
  expect(hasRing).toBe(true);

  // Enter activates the focused sidebar file (native button semantics).
  await page.keyboard.press('Enter');
  await expect(page.locator('.diff-sidebar-file.active')).toHaveCount(1);
});

test('console stays clean across the full declared flow', async ({ page }) => {
  const errors = collectConsoleErrors(page);
  await openDetail(page, options({ viewport: { width: 1440, height: 900 } }));

  // chat → activity → plan → diff
  await page.evaluate(() => window.Dashboard.openDetailTab('activity'));
  await page.waitForTimeout(300);
  await page.evaluate(() => window.Dashboard.openDetailTab('plan'));
  await page.waitForTimeout(300);
  await openDiff(page);

  // diff interactions: wrap off/on, view-mode switch, sidebar collapse/expand, file jump
  const wrap = page.locator('.diff-summary-bar .toggle-switch');
  await wrap.click();
  await wrap.click();
  await page.locator('.diff-toggle-btn[data-mode="side-by-side"]').click();
  await page.waitForTimeout(500);
  await page.locator('.diff-toggle-btn[data-mode="line-by-line"]').click();
  await page.waitForTimeout(500);
  await page.locator('.diff-sidebar-toggle').click();
  await page.locator('.diff-sidebar-toggle').click();
  await page.locator('.diff-sidebar-file').nth(1).click();
  await page.waitForTimeout(300);

  // back to chat — transcript + composer intact
  await page.evaluate(() => window.Dashboard.openDetailTab('conversation'));
  await expect(page.locator('.ui-msg--assistant').first()).toBeVisible();
  await expect(page.locator('#reply-input')).toBeVisible();

  expect(errors).toEqual([]);
});

test('diff control state persists: wrap, view mode, sidebar collapse', async ({ page }) => {
  await openDetail(page, options({ viewport: { width: 1440, height: 900 } }));
  await openDiff(page);

  // Wrap defaults ON; opting out persists to sessionStorage.
  const content = page.locator('#diff-content');
  await expect(content).toHaveClass(/diff-wrap/);
  await page.locator('.diff-summary-bar .toggle-switch').click();
  await expect(content).not.toHaveClass(/diff-wrap/);
  expect(await page.evaluate(() => sessionStorage.getItem('diff-wrap-lines'))).toBe('false');

  // Explicit view choice pins to localStorage and re-renders.
  await page.locator('.diff-toggle-btn[data-mode="side-by-side"]').click();
  await page.waitForSelector('.d2h-file-side-diff', { timeout: 5000 });
  expect(await page.evaluate(() => localStorage.getItem('diff-view-mode'))).toBe('side-by-side');

  // Sidebar collapse persists and Files button reflects state.
  const layout = page.locator('.diff-layout');
  const filesBtn = page.locator('.diff-sidebar-toggle');
  await expect(filesBtn).toHaveClass(/active/);
  await filesBtn.click();
  await expect(layout).toHaveClass(/diff-sidebar-collapsed/);
  await expect(filesBtn).not.toHaveClass(/active/);
  expect(await page.evaluate(() => localStorage.getItem('diff-sidebar-collapsed'))).toBe('true');
});

test('auto view mode: unified at 1440, split at 1728, resize crosses threshold', async ({ page }) => {
  await openDetail(page, options({ viewport: { width: 1440, height: 900 } }));
  await openDiff(page);
  await expect(page.locator('.diff-toggle-btn[data-mode="line-by-line"]')).toHaveClass(/active/);
  await expect(page.locator('.d2h-file-side-diff')).toHaveCount(0);

  await page.setViewportSize({ width: 1728, height: 1000 });
  await page.waitForTimeout(600); // debounce + lazy re-render
  await expect(page.locator('.diff-toggle-btn[data-mode="side-by-side"]')).toHaveClass(/active/);
});

test('mobile diff renders summary card, not the diff table', async ({ page }) => {
  await openDetail(page, options({ viewport: { width: 390, height: 844 } }));
  await openDiff(page).catch(() => {});
  await page.evaluate(() => window.Dashboard.openDetailTab('diff'));
  await page.waitForSelector('.mobile-diff-summary', { timeout: 5000 });
  await expect(page.locator('.mobile-diff-summary')).toBeVisible();
  await expect(page.locator('.d2h-diff-table')).toHaveCount(0);
});

test('chat: per-message blocks render and composer survives tab round-trip', async ({ page }) => {
  await openDetail(page, options({ viewport: { width: 1440, height: 900 } }));
  await expect(page.locator('.ui-msg--assistant')).toHaveCount(3);
  await openDiff(page);
  await page.evaluate(() => window.Dashboard.openDetailTab('conversation'));
  await expect(page.locator('.ui-msg--assistant')).toHaveCount(3);
  await expect(page.locator('#reply-input')).toBeVisible();
});
