'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');

const { mapToolCall, mapToolResult, mapSessionStart, mapAgentEnd, mapToolName } = require('../lib/event-mapper');

test('mapToolName: bash → Bash', () => {
  assert.equal(mapToolName('bash'), 'Bash');
});

test('mapToolName: known tools title-cased', () => {
  assert.equal(mapToolName('read'), 'Read');
  assert.equal(mapToolName('write'), 'Write');
  assert.equal(mapToolName('edit'), 'Edit');
  assert.equal(mapToolName('grep'), 'Grep');
  assert.equal(mapToolName('find'), 'Find');
  assert.equal(mapToolName('ls'), 'Ls');
});

test('mapToolName: custom tool name passes through unchanged', () => {
  assert.equal(mapToolName('mcp_my_tool'), 'mcp_my_tool');
  assert.equal(mapToolName('SomeCustomCase'), 'SomeCustomCase');
});

test('mapToolCall: bash event → PreToolUse Claude payload', () => {
  const event = {
    type: 'tool_call',
    toolCallId: 'tc_abc',
    toolName: 'bash',
    input: { command: 'rm -rf /tmp/foo' },
  };

  const payload = mapToolCall(event, { sessionId: 'sid-1', cwd: '/work' });

  assert.deepEqual(payload, {
    hook_event_name: 'PreToolUse',
    session_id: 'sid-1',
    cwd: '/work',
    tool_name: 'Bash',
    tool_input: { command: 'rm -rf /tmp/foo' },
    tool_call_id: 'tc_abc',
  });
});

test('mapToolCall: edit event preserves arbitrary input shape', () => {
  const event = {
    type: 'tool_call',
    toolCallId: 'tc_2',
    toolName: 'edit',
    input: { path: '/x', oldString: 'a', newString: 'b' },
  };

  const payload = mapToolCall(event, { sessionId: 'sid', cwd: '/' });

  assert.equal(payload.tool_name, 'Edit');
  assert.deepEqual(payload.tool_input, { path: '/x', oldString: 'a', newString: 'b' });
});

test('mapToolResult: bash result → PostToolUse Claude payload', () => {
  const event = {
    type: 'tool_result',
    toolCallId: 'tc_abc',
    toolName: 'bash',
    input: { command: 'git commit -m wip' },
    content: [{ type: 'text', text: 'ok' }],
    isError: false,
    details: { exitCode: 0 },
  };

  const payload = mapToolResult(event, { sessionId: 'sid-1', cwd: '/work' });

  assert.equal(payload.hook_event_name, 'PostToolUse');
  assert.equal(payload.tool_name, 'Bash');
  assert.equal(payload.session_id, 'sid-1');
  assert.deepEqual(payload.tool_input, { command: 'git commit -m wip' });
  assert.equal(payload.tool_response_is_error, false);
});

test('mapSessionStart: → SessionStart Claude payload', () => {
  const event = { type: 'session_start', reason: 'startup' };

  const payload = mapSessionStart(event, {
    sessionId: 'sid-1',
    cwd: '/work',
    model: 'claude-sonnet-4',
  });

  assert.equal(payload.hook_event_name, 'SessionStart');
  assert.equal(payload.session_id, 'sid-1');
  assert.equal(payload.cwd, '/work');
  assert.equal(payload.model, 'claude-sonnet-4');
  assert.equal(payload.source, 'startup');
});

test('mapAgentEnd: → Stop Claude payload', () => {
  const event = {
    type: 'agent_end',
    messages: [
      { role: 'assistant', content: [{ type: 'text', text: 'all done' }] },
    ],
  };

  const payload = mapAgentEnd(event, { sessionId: 'sid-1', cwd: '/work' });

  assert.equal(payload.hook_event_name, 'Stop');
  assert.equal(payload.session_id, 'sid-1');
  assert.equal(payload.cwd, '/work');
  assert.equal(payload.last_assistant_message, 'all done');
});

test('mapAgentEnd: empty messages → null last_assistant_message', () => {
  const event = { type: 'agent_end', messages: [] };

  const payload = mapAgentEnd(event, { sessionId: 'sid', cwd: '/' });

  assert.equal(payload.last_assistant_message, null);
});

test('mapAgentEnd: assistant message with multiple text blocks joined', () => {
  const event = {
    type: 'agent_end',
    messages: [
      { role: 'user', content: 'hi' },
      {
        role: 'assistant',
        content: [
          { type: 'text', text: 'first' },
          { type: 'tool_use', name: 'bash' },
          { type: 'text', text: 'second' },
        ],
      },
    ],
  };

  const payload = mapAgentEnd(event, { sessionId: 'sid', cwd: '/' });
  assert.equal(payload.last_assistant_message, 'first\nsecond');
});

test('mapAgentEnd: assistant message with plain string content', () => {
  const event = {
    type: 'agent_end',
    messages: [{ role: 'assistant', content: 'plain string body' }],
  };

  const payload = mapAgentEnd(event, { sessionId: 'sid', cwd: '/' });
  assert.equal(payload.last_assistant_message, 'plain string body');
});
