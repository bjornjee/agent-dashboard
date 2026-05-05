'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');

const extension = require('../extensions/agent-dashboard');

test('extension entry: registers all expected event handlers', () => {
  const subscriptions = [];
  const fakePi = {
    on: (eventName, handler) => {
      subscriptions.push({ eventName, handler });
    },
  };

  extension(fakePi);

  const events = subscriptions.map(s => s.eventName).sort();
  assert.deepEqual(events, [
    'agent_end',
    'auto_retry_start',
    'session_shutdown',
    'session_start',
    'tool_call',
    'tool_execution_end',
    'tool_execution_start',
    'tool_result',
  ]);
});

test('extension entry: each handler is a function', () => {
  const subscriptions = [];
  const fakePi = { on: (n, h) => subscriptions.push({ n, h }) };
  extension(fakePi);
  for (const s of subscriptions) {
    assert.equal(typeof s.h, 'function', `${s.n} handler must be a function`);
  }
});

test('extension entry: re-exports handlers module for testing', () => {
  assert.ok(extension.handlers);
  assert.equal(typeof extension.handlers.onSessionStart, 'function');
});
