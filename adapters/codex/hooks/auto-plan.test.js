#!/usr/bin/env node
'use strict';

const { describe, it } = require('node:test');
const assert = require('node:assert/strict');

const { runAutoPlan } = require('./auto-plan');

describe('auto-plan hook helper', () => {
  it('types prompt text literally before sending Enter', async () => {
    const cp = require('child_process');
    const origSpawnSync = cp.spawnSync;
    const modulePath = require.resolve('./auto-plan');
    const calls = [];

    cp.spawnSync = (cmd, args) => {
      calls.push({ cmd, args });
      return { status: 0 };
    };
    delete require.cache[modulePath];

    try {
      const { runAutoPlan: runWithDefaultSendLine } = require('./auto-plan');
      const states = [
        {},
        { permission_mode: 'plan' },
      ];

      const result = await runWithDefaultSendLine({
        sessionId: 'session-1',
        tmuxPane: '%7',
        deferredPrompt: '$agent-dashboard:feature fix plan mode',
        readAgentState: () => states.shift() || { permission_mode: 'plan' },
        writeState: () => {},
        sleep: async () => {},
        maxAttempts: 1,
        initialDelayMs: 0,
        autoPlan: true,
      });

      assert.equal(result.status, 'done');
      assert.deepEqual(calls.map(call => call.args), [
        ['send-keys', '-l', '-t', '%7', '/plan plan'],
        ['send-keys', '-t', '%7', 'Enter'],
        ['send-keys', '-l', '-t', '%7', '$agent-dashboard:feature fix plan mode'],
        ['send-keys', '-t', '%7', 'Enter'],
      ]);
    } finally {
      cp.spawnSync = origSpawnSync;
      delete require.cache[modulePath];
    }
  });

  it('sends /plan first, waits for plan mode, then sends the deferred prompt', async () => {
    const sent = [];
    const states = [
      { permission_mode: 'default' },
      { permission_mode: 'plan' },
    ];
    const writes = [];

    const result = await runAutoPlan({
      sessionId: 'session-1',
      tmuxPane: '%7',
      deferredPrompt: '$agent-dashboard:feature fix plan mode',
      readAgentState: () => states.shift() || { permission_mode: 'plan' },
      writeState: (_sessionId, update) => writes.push(update),
      sendLine: (_pane, text) => {
        sent.push(text);
        return true;
      },
      sleep: async () => {},
      maxAttempts: 3,
      initialDelayMs: 0,
      autoPlan: true,
    });

    assert.equal(result.status, 'done');
    assert.deepEqual(sent, ['/plan plan', '$agent-dashboard:feature fix plan mode']);
    assert.equal(writes.at(-1).auto_plan_status, 'done');
  });

  it('does not inject twice when the session is already marked', async () => {
    const sent = [];

    const result = await runAutoPlan({
      sessionId: 'session-1',
      tmuxPane: '%7',
      deferredPrompt: '$agent-dashboard:feature fix plan mode',
      readAgentState: () => ({ auto_plan_injected_at: '2026-05-21T00:00:00.000Z' }),
      writeState: () => {},
      sendLine: (_pane, text) => {
        sent.push(text);
        return true;
      },
      sleep: async () => {},
      initialDelayMs: 0,
    });

    assert.equal(result.status, 'already-injected');
    assert.deepEqual(sent, []);
  });

  it('times out without sending the deferred prompt when plan mode is never observed', async () => {
    const sent = [];
    const writes = [];

    const result = await runAutoPlan({
      sessionId: 'session-1',
      tmuxPane: '%7',
      deferredPrompt: '$agent-dashboard:feature fix plan mode',
      readAgentState: () => ({ permission_mode: 'default' }),
      writeState: (_sessionId, update) => writes.push(update),
      sendLine: (_pane, text) => {
        sent.push(text);
        return true;
      },
      sleep: async () => {},
      maxAttempts: 2,
      initialDelayMs: 0,
      autoPlan: true,
    });

    assert.equal(result.status, 'timeout');
    assert.deepEqual(sent, ['/plan plan']);
    assert.equal(writes.at(-1).auto_plan_status, 'timeout');
  });
});
