'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');

const handlers = require('../lib/handlers');

function makeCtx({ sessionId = 'sid-1', cwd = '/work', model = 'm' } = {}) {
  return {
    sessionManager: { getSessionId: () => sessionId },
    cwd,
    model: { id: model },
  };
}

function makeDeps(overrides = {}) {
  const writes = [];
  const gateCalls = [];
  const singleGateCalls = [];
  return {
    writes,
    gateCalls,
    singleGateCalls,
    deps: {
      writePiState: (sid, update) => writes.push({ sid, update }),
      runGates: (scripts, payload) => {
        gateCalls.push({ scripts, payload });
        return overrides.gatesResult || null;
      },
      runGate: (script, payload) => {
        singleGateCalls.push({ script, payload });
        return null;
      },
      getPaneId: () => '%5',
      getTarget: () => 'main:0.1',
      preBashGates: ['/g/pre1.js', '/g/pre2.js'],
      postBashGates: ['/g/post1.js'],
      stopHooks: ['/g/desktop-notify.js'],
    },
  };
}

test('onSessionStart: writes state with target and started_at', () => {
  const ctx = makeCtx();
  const { deps, writes } = makeDeps();

  handlers.onSessionStart({ type: 'session_start', reason: 'startup' }, ctx, deps);

  assert.equal(writes.length, 1);
  assert.equal(writes[0].sid, 'sid-1');
  assert.equal(writes[0].update.state, 'running');
  assert.equal(writes[0].update.target, 'main:0.1');
  assert.equal(writes[0].update.tmux_pane_id, '%5');
  assert.equal(writes[0].update.cwd, '/work');
  assert.equal(writes[0].update.model, 'm');
  assert.ok(writes[0].update.started_at);
});

test('onSessionStart: no-op when no tmux pane', () => {
  const ctx = makeCtx();
  const { deps, writes } = makeDeps();
  deps.getPaneId = () => null;

  handlers.onSessionStart({ type: 'session_start' }, ctx, deps);

  assert.equal(writes.length, 0);
});

test('onToolCall: bash → runs pre-bash gates with PreToolUse payload', () => {
  const ctx = makeCtx();
  const { deps, gateCalls } = makeDeps();

  const event = {
    type: 'tool_call',
    toolName: 'bash',
    toolCallId: 'tc',
    input: { command: 'ls' },
  };
  const result = handlers.onToolCall(event, ctx, deps);

  assert.equal(result, undefined);
  assert.equal(gateCalls.length, 1);
  assert.deepEqual(gateCalls[0].scripts, ['/g/pre1.js', '/g/pre2.js']);
  assert.equal(gateCalls[0].payload.hook_event_name, 'PreToolUse');
  assert.equal(gateCalls[0].payload.tool_name, 'Bash');
});

test('onToolCall: bash blocked → returns block, writes error state', () => {
  const ctx = makeCtx();
  const { deps, writes } = makeDeps({ gatesResult: { block: true, reason: 'NO!' } });

  const result = handlers.onToolCall({
    type: 'tool_call', toolName: 'bash', toolCallId: 'tc', input: { command: 'rm -rf /' },
  }, ctx, deps);

  assert.deepEqual(result, { block: true, reason: 'NO!' });
  // Two writes: one running update, one error update with hook_blocked
  const lastWrite = writes[writes.length - 1].update;
  assert.equal(lastWrite.state, 'error');
  assert.equal(lastWrite.hook_blocked, 'NO!');
});

test('onToolCall: non-bash → no gates, just state update', () => {
  const ctx = makeCtx();
  const { deps, gateCalls, writes } = makeDeps();

  handlers.onToolCall({
    type: 'tool_call', toolName: 'read', toolCallId: 'tc', input: { path: '/x' },
  }, ctx, deps);

  assert.equal(gateCalls.length, 0);
  assert.equal(writes.length, 1);
  assert.equal(writes[0].update.current_tool, 'Read');
});

test('onToolResult: bash → runs post-bash gates (no block)', () => {
  const ctx = makeCtx();
  const { deps, gateCalls } = makeDeps();

  handlers.onToolResult({
    type: 'tool_result', toolName: 'bash', toolCallId: 'tc',
    input: { command: 'git commit -m wip' }, content: [], isError: false,
  }, ctx, deps);

  assert.equal(gateCalls.length, 1);
  assert.deepEqual(gateCalls[0].scripts, ['/g/post1.js']);
  assert.equal(gateCalls[0].payload.hook_event_name, 'PostToolUse');
});

test('onToolResult: non-bash → no-op', () => {
  const ctx = makeCtx();
  const { deps, gateCalls } = makeDeps();

  handlers.onToolResult({
    type: 'tool_result', toolName: 'read', toolCallId: 'tc',
    input: {}, content: [], isError: false,
  }, ctx, deps);

  assert.equal(gateCalls.length, 0);
});

test('onAgentEnd: writes idle_prompt and fires stop hooks', () => {
  const ctx = makeCtx();
  const { deps, writes, singleGateCalls } = makeDeps();

  handlers.onAgentEnd({
    type: 'agent_end',
    messages: [{ role: 'assistant', content: 'all done' }],
  }, ctx, deps);

  assert.equal(writes.length, 1);
  assert.equal(writes[0].update.state, 'idle_prompt');
  assert.equal(writes[0].update.last_message_preview, 'all done');

  assert.equal(singleGateCalls.length, 1);
  assert.equal(singleGateCalls[0].script, '/g/desktop-notify.js');
  assert.equal(singleGateCalls[0].payload.hook_event_name, 'Stop');
});

test('onSessionShutdown: writes done state', () => {
  const ctx = makeCtx();
  const { deps, writes } = makeDeps();

  handlers.onSessionShutdown({ type: 'session_shutdown', reason: 'quit' }, ctx, deps);

  assert.equal(writes.length, 1);
  assert.equal(writes[0].update.state, 'done');
});

test('handlers: gracefully no-op when sessionId missing', () => {
  const ctx = { sessionManager: { getSessionId: () => '' }, cwd: '/' };
  const { deps, writes } = makeDeps();

  handlers.onSessionStart({ type: 'session_start' }, ctx, deps);
  handlers.onAgentEnd({ type: 'agent_end', messages: [] }, ctx, deps);
  handlers.onSessionShutdown({ type: 'session_shutdown' }, ctx, deps);

  assert.equal(writes.length, 0);
});
