// Unit tests for state.js badge helpers added for the PWA state-gap fix.

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { pathToFileURL } = require('node:url');
const path = require('node:path');

let planBadge;
let subagentBadge;
let questionBadge;

test('load state module', async () => {
  const url = pathToFileURL(path.join(__dirname, 'state.js')).href;
  const mod = await import(url);
  planBadge = mod.planBadge;
  subagentBadge = mod.subagentBadge;
  questionBadge = mod.questionBadge;
  assert.equal(typeof planBadge, 'function');
  assert.equal(typeof subagentBadge, 'function');
  assert.equal(typeof questionBadge, 'function');
});

// -- planBadge --

test('planBadge: permission_mode=plan returns PLAN', () => {
  assert.equal(planBadge({ permission_mode: 'plan' }), 'PLAN');
});

test('planBadge: state=plan returns PLAN', () => {
  // Covers Claude Code's "plan" state (set by hook on ExitPlanMode
  // PreToolUse) even when permission_mode is empty.
  assert.equal(planBadge({ state: 'plan' }), 'PLAN');
});

test('planBadge: state=plan + permission_mode=default still returns PLAN', () => {
  // After plan approval, permission_mode flips back but state may briefly
  // remain 'plan' until the next hook. The badge is a status signal, not
  // a mode echo — either signal should fire it.
  assert.equal(planBadge({ state: 'plan', permission_mode: 'default' }), 'PLAN');
});

test('planBadge: running with no plan signal returns empty', () => {
  assert.equal(planBadge({ state: 'running', permission_mode: 'bypassPermissions' }), '');
});

test('planBadge: empty agent returns empty', () => {
  assert.equal(planBadge({}), '');
});

// -- subagentBadge --

test('subagentBadge: zero count returns empty', () => {
  assert.equal(subagentBadge({ subagent_count: 0 }), '');
  assert.equal(subagentBadge({}), '');
});

test('subagentBadge: positive count returns glyph + count', () => {
  assert.equal(subagentBadge({ subagent_count: 1 }), '↳ 1');
  assert.equal(subagentBadge({ subagent_count: 3 }), '↳ 3');
});

test('subagentBadge: rejects non-numeric or negative', () => {
  // Defensive: a malformed state file shouldn't render garbage.
  assert.equal(subagentBadge({ subagent_count: -1 }), '');
  assert.equal(subagentBadge({ subagent_count: 'x' }), '');
});

// -- questionBadge --

test('questionBadge: empty when no pending_question', () => {
  assert.equal(questionBadge({}), '');
  assert.equal(questionBadge({ pending_question: null }), '');
});

test('questionBadge: returns "ASK" for any pending question', () => {
  // No counter — the chip is a presence signal, not a status panel; the
  // detail view owns the question count.
  assert.equal(questionBadge({ pending_question: { questions: [{ question: 'A?' }] } }), 'ASK');
  assert.equal(questionBadge({
    pending_question: { questions: [{ question: 'A?' }, { question: 'B?' }] },
  }), 'ASK');
});

test('questionBadge: empty when pending_question has no questions array', () => {
  assert.equal(questionBadge({ pending_question: {} }), '');
  assert.equal(questionBadge({ pending_question: { questions: [] } }), '');
});
