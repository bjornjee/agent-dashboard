// Unit tests for sidebar rendering.
// Run via `node --test internal/web/static/js/sidebar.test.js` (chained from `make test`).
//
// sidebar.js touches the DOM (document.getElementById on '#app-sidebar')
// and depends on theme.js (which touches localStorage + document). We stub
// the minimum surface required to drive renderSidebar() and then assert
// on the resulting innerHTML.

const { test, before } = require('node:test');
const assert = require('node:assert/strict');
const { pathToFileURL } = require('node:url');
const path = require('node:path');

// --- minimal DOM stubs ------------------------------------------------------
// We only need:
//   - document.getElementById('app-sidebar') -> a host object with `innerHTML`
//     and `hidden` setter, plus `querySelector` / `querySelectorAll` no-ops.
//   - document.documentElement.dataset (Theme.apply writes to it).
//   - document.querySelector / querySelectorAll (Theme.cycle uses them).
//   - localStorage (Theme.getPreference / setItem).
//   - window.matchMedia (sidebar.isDesktop()).
function makeHost() {
  return {
    innerHTML: '',
    hidden: true,
  };
}

function installDom() {
  const host = makeHost();
  const documentEl = { dataset: {} };
  global.document = {
    getElementById(id) {
      if (id === 'app-sidebar') return host;
      return null;
    },
    documentElement: documentEl,
    querySelector() { return null; },
    querySelectorAll() { return []; },
  };
  global.localStorage = {
    _s: {},
    getItem(k) { return this._s[k] || null; },
    setItem(k, v) { this._s[k] = String(v); },
  };
  global.window = {
    matchMedia() { return { matches: true }; },
  };
  return host;
}

let renderSidebar;
let host;

before(async () => {
  host = installDom();
  const url = pathToFileURL(path.join(__dirname, 'sidebar.js')).href;
  const mod = await import(url);
  renderSidebar = mod.renderSidebar;
  assert.equal(typeof renderSidebar, 'function');
});

test('Usage nav row has a leading icon (A5)', () => {
  // Render with no agents so the only nav rows present are the bottom
  // anchor block (Install / Usage / theme toggle) + the top CTA block.
  renderSidebar([], null, 'list');

  const html = host.innerHTML;

  // Locate the "Usage" row: the wrapper div sits in .app-sidebar__bottom
  // and contains a UI.row primitive. The row primitive renders the title
  // inside <span class="ui-row__title">Usage</span>. The leading slot is
  // <div class="ui-row__leading">...</div> placed immediately after the
  // opening tag.
  const usageIdx = html.indexOf('>Usage<');
  assert.ok(usageIdx > -1, 'Usage row should render in sidebar');

  // Scan backwards from the Usage title to the nearest opening <button
  // class="ui-row" — the slice between them should contain a
  // ui-row__leading slot with an inline <svg.
  const buttonOpenIdx = html.lastIndexOf('<button class="ui-row"', usageIdx);
  assert.ok(buttonOpenIdx > -1, 'Usage row should be a button.ui-row');
  const rowFragment = html.slice(buttonOpenIdx, usageIdx);

  assert.match(
    rowFragment,
    /class="ui-row__leading"[^>]*>[^<]*<svg/,
    'Usage row should have an inline <svg> in its leading slot',
  );
});

test('Install app row still has its download icon (regression)', () => {
  renderSidebar([], null, 'list');
  const html = host.innerHTML;
  const installIdx = html.indexOf('>Install app<');
  assert.ok(installIdx > -1, 'Install app row should render in sidebar');
  const buttonOpenIdx = html.lastIndexOf('<button class="ui-row"', installIdx);
  const rowFragment = html.slice(buttonOpenIdx, installIdx);
  assert.match(
    rowFragment,
    /class="ui-row__leading"[^>]*>[^<]*<svg/,
    'Install row keeps its leading svg',
  );
});
