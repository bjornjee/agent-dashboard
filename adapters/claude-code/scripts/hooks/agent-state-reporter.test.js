#!/usr/bin/env node
'use strict';

const { describe, it } = require('node:test');
const assert = require('node:assert/strict');

const { buildReportEntry, resolveStopState } = require('./agent-state-reporter');
const { detectState } = require('../../packages/agent-state/detect');

const BASE_INPUT = {
  session_id: 'abc-123',
  hook_event_name: 'SessionStart',
  cwd: '/Users/bjornjee/Code/bjornjee/skills',
  permission_mode: 'default',
  model: 'claude-opus-4-6',
};

const BASE_PARSED = { session: 'main', window: 1, pane: 0 };

describe('buildReportEntry', () => {
  it('includes cwd from input but not branch', () => {
    const { entry } = buildReportEntry({
      input: BASE_INPUT,
      existing: {},
      target: 'main:1.0',
      tmuxPane: '%0',
      state: 'running',
      filesChanged: [],
      parsed: BASE_PARSED,
      cwd: '/Users/bjornjee/Code/bjornjee/skills',
    });

    assert.equal(entry.branch, undefined, 'reporter should not set branch');
    assert.equal(entry.cwd, '/Users/bjornjee/Code/bjornjee/skills');
    assert.equal(entry.state, 'running');
    assert.equal(entry.model, 'claude-opus-4-6');
  });

  it('falls back to existing.cwd when cwd param is empty', () => {
    const { entry } = buildReportEntry({
      input: BASE_INPUT,
      existing: { cwd: '/existing/path' },
      target: 'main:1.0',
      tmuxPane: '%0',
      state: 'running',
      filesChanged: [],
      parsed: BASE_PARSED,
      cwd: '',
    });

    assert.equal(entry.cwd, '/existing/path');
  });

  it('skips write when nothing changed', () => {
    const { changed } = buildReportEntry({
      input: { ...BASE_INPUT, hook_event_name: 'SubagentStop' },
      existing: {
        state: 'running',
        subagent_count: 0,
        last_message_preview: null,
        permission_mode: 'default',
        files_changed: [],
      },
      target: 'main:1.0',
      tmuxPane: '%0',
      state: 'running',
      filesChanged: [],
      parsed: BASE_PARSED,
    });

    assert.equal(changed, false);
  });

  it('increments subagent count on SubagentStart', () => {
    const { entry } = buildReportEntry({
      input: { ...BASE_INPUT, hook_event_name: 'SubagentStart' },
      existing: { subagent_count: 2 },
      target: 'main:1.0',
      tmuxPane: '%0',
      state: 'running',
      filesChanged: [],
      parsed: BASE_PARSED,
    });

    assert.equal(entry.subagent_count, 3);
  });

  it('decrements subagent count on SubagentStop (floor 0)', () => {
    const { entry } = buildReportEntry({
      input: { ...BASE_INPUT, hook_event_name: 'SubagentStop' },
      existing: { subagent_count: 0 },
      target: 'main:1.0',
      tmuxPane: '%0',
      state: 'running',
      filesChanged: [],
      parsed: BASE_PARSED,
    });

    assert.equal(entry.subagent_count, 0);
  });

  it('preserves model from existing on non-SessionStart events', () => {
    const { entry } = buildReportEntry({
      input: { ...BASE_INPUT, hook_event_name: 'Stop', model: 'claude-haiku-4-5' },
      existing: { model: 'claude-opus-4-6' },
      target: 'main:1.0',
      tmuxPane: '%0',
      state: 'done',
      filesChanged: [],
      parsed: BASE_PARSED,
    });

    assert.equal(entry.model, 'claude-opus-4-6');
  });

  it('preserves existing cwd on non-SessionStart events', () => {
    const { entry } = buildReportEntry({
      input: { ...BASE_INPUT, hook_event_name: 'SubagentStart', cwd: '/Users/bjornjee/Code/project/src/subdir' },
      existing: { cwd: '/Users/bjornjee/Code/project', subagent_count: 0 },
      target: 'main:1.0',
      tmuxPane: '%0',
      state: 'running',
      filesChanged: [],
      parsed: BASE_PARSED,
      cwd: '/Users/bjornjee/Code/project/src/subdir',
    });

    assert.equal(entry.cwd, '/Users/bjornjee/Code/project');
  });

  it('always reports changed when existing has no state (first write)', () => {
    const { changed } = buildReportEntry({
      input: BASE_INPUT,
      existing: {},
      target: 'main:1.0',
      tmuxPane: '%0',
      state: 'running',
      filesChanged: [],
      parsed: BASE_PARSED,
    });

    assert.equal(changed, true);
  });
});

describe('SubagentStop state handling', () => {
  it('preserves idle_prompt when Stop already wrote it', () => {
    const { entry } = buildReportEntry({
      input: { ...BASE_INPUT, hook_event_name: 'SubagentStop' },
      existing: {
        state: 'idle_prompt',
        subagent_count: 1,
        last_message_preview: null,
        permission_mode: 'default',
        files_changed: [],
      },
      target: 'main:1.0',
      tmuxPane: '%0',
      state: 'idle_prompt',  // report() preserves existing stop state
      filesChanged: [],
      parsed: BASE_PARSED,
    });

    assert.equal(entry.state, 'idle_prompt', 'SubagentStop should not overwrite idle_prompt');
    assert.equal(entry.subagent_count, 0, 'subagent_count should still decrement');
  });

  it('preserves done when Stop already wrote it', () => {
    const { entry } = buildReportEntry({
      input: { ...BASE_INPUT, hook_event_name: 'SubagentStop' },
      existing: {
        state: 'done',
        subagent_count: 2,
        last_message_preview: null,
        permission_mode: 'default',
        files_changed: [],
      },
      target: 'main:1.0',
      tmuxPane: '%0',
      state: 'done',  // report() preserves existing stop state
      filesChanged: [],
      parsed: BASE_PARSED,
    });

    assert.equal(entry.state, 'done', 'SubagentStop should not overwrite done');
    assert.equal(entry.subagent_count, 1);
  });

  it('carries self-healed idle_prompt when Stop handler failed', () => {
    // When Stop failed silently, existing.state is still "running".
    // report() detects idle from pane buffer and passes the detected state.
    const { entry } = buildReportEntry({
      input: { ...BASE_INPUT, hook_event_name: 'SubagentStop' },
      existing: {
        state: 'running',
        subagent_count: 1,
        last_message_preview: null,
        permission_mode: 'default',
        files_changed: [],
      },
      target: 'main:1.0',
      tmuxPane: '%0',
      state: 'idle_prompt',  // report() detected idle via capture-pane fallback
      filesChanged: [],
      parsed: BASE_PARSED,
    });

    assert.equal(entry.state, 'idle_prompt', 'self-healed state should propagate');
    assert.equal(entry.subagent_count, 0);
  });

  it('keeps running when subagents remain active', () => {
    const { entry } = buildReportEntry({
      input: { ...BASE_INPUT, hook_event_name: 'SubagentStop' },
      existing: {
        state: 'running',
        subagent_count: 3,
        last_message_preview: null,
        permission_mode: 'default',
        files_changed: [],
      },
      target: 'main:1.0',
      tmuxPane: '%0',
      state: 'running',  // report() preserves running when subagents > 0
      filesChanged: [],
      parsed: BASE_PARSED,
    });

    assert.equal(entry.state, 'running', 'should stay running with active subagents');
    assert.equal(entry.subagent_count, 2);
  });
});

describe('SubagentStop writeState guard prevents TOCTOU race', () => {
  const fs = require('fs');
  const path = require('path');
  const os = require('os');
  const { writeState, readAgentState } = require('../../packages/agent-state');

  it('guardStates aborts write when on-disk state is idle_prompt', () => {
    const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'agent-test-'));
    const sessionId = 'race-test-session';
    const STOP_STATES = new Set(['idle_prompt', 'done', 'question', 'plan']);

    // Simulate: Stop hook already wrote idle_prompt to disk
    writeState(sessionId, { state: 'idle_prompt', target: 'main:2.1' }, tmpDir);

    // Simulate: SubagentStop tries to overwrite with running + guardStates
    writeState(sessionId, { state: 'running', target: 'main:2.1' }, tmpDir, { guardStates: STOP_STATES });

    const onDisk = readAgentState(sessionId, tmpDir);
    assert.equal(onDisk.state, 'idle_prompt', 'guardStates should prevent SubagentStop from overwriting idle_prompt');

    // Cleanup
    fs.rmSync(tmpDir, { recursive: true });
  });

  it('allows write when on-disk state is running (no guard hit)', () => {
    const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'agent-test-'));
    const sessionId = 'race-test-session-2';
    const STOP_STATES = new Set(['idle_prompt', 'done', 'question', 'plan']);

    // On-disk state is running (Stop hook hasn't fired yet)
    writeState(sessionId, { state: 'running', target: 'main:2.1', subagent_count: 1 }, tmpDir);

    // SubagentStop writes running with decremented count — should succeed
    writeState(sessionId, { state: 'running', target: 'main:2.1', subagent_count: 0 }, tmpDir, { guardStates: STOP_STATES });

    const onDisk = readAgentState(sessionId, tmpDir);
    assert.equal(onDisk.state, 'running');
    assert.equal(onDisk.subagent_count, 0, 'subagent_count should update when guard does not fire');

    // Cleanup
    fs.rmSync(tmpDir, { recursive: true });
  });
});

describe('resolveStopState (JSONL-gated detect)', () => {
  const PANE_IDLE = ['some output', '❯'];
  const MSG_PLAIN = 'Here is the report.';

  it('SubagentStop preserves running when parent has a pending tool_use', () => {
    const state = resolveStopState({
      hookEvent: 'SubagentStop',
      existing: { state: 'running', subagent_count: 1 },
      hasPendingTool: true,
      lastMessage: MSG_PLAIN,
      paneBuffer: PANE_IDLE,
    });
    assert.equal(state, 'running');
  });

  it('SubagentStop falls through to detectState when no parent tool pending', () => {
    const state = resolveStopState({
      hookEvent: 'SubagentStop',
      existing: { state: 'running', subagent_count: 1 },
      hasPendingTool: false,
      lastMessage: MSG_PLAIN,
      paneBuffer: PANE_IDLE,
    });
    assert.equal(state, 'idle_prompt');
  });

  it('SubagentStop preserves existing stop state regardless of pending', () => {
    const state = resolveStopState({
      hookEvent: 'SubagentStop',
      existing: { state: 'idle_prompt', subagent_count: 1 },
      hasPendingTool: false,
      lastMessage: MSG_PLAIN,
      paneBuffer: PANE_IDLE,
    });
    assert.equal(state, 'idle_prompt');
  });

  it('SubagentStop preserves running when subagents remain active', () => {
    const state = resolveStopState({
      hookEvent: 'SubagentStop',
      existing: { state: 'running', subagent_count: 3 },
      hasPendingTool: false,
      lastMessage: MSG_PLAIN,
      paneBuffer: PANE_IDLE,
    });
    assert.equal(state, 'running');
  });

  it('Stop preserves running when parent has a pending tool_use', () => {
    const state = resolveStopState({
      hookEvent: 'Stop',
      existing: { state: 'running' },
      hasPendingTool: true,
      lastMessage: MSG_PLAIN,
      paneBuffer: PANE_IDLE,
    });
    assert.equal(state, 'running');
  });

  it('Stop calls detectState when no pending tool', () => {
    const state = resolveStopState({
      hookEvent: 'Stop',
      existing: { state: 'running' },
      hasPendingTool: false,
      lastMessage: 'Should I proceed?',
      paneBuffer: PANE_IDLE,
    });
    assert.equal(state, 'question');
  });

  it('Stop with pending tool but no existing state defaults to running', () => {
    const state = resolveStopState({
      hookEvent: 'Stop',
      existing: {},
      hasPendingTool: true,
      lastMessage: MSG_PLAIN,
      paneBuffer: PANE_IDLE,
    });
    assert.equal(state, 'running');
  });
});

describe('detectState (Stop event path)', () => {
  it('returns idle_prompt when pane buffer has prompt char and no question', () => {
    const state = detectState('Here is the investigation report.', ['some output', '\u276f']);
    assert.equal(state, 'idle_prompt');
  });

  it('returns question when last message ends with a question', () => {
    const state = detectState('Should I proceed with the fix?', ['some output', '\u276f']);
    assert.equal(state, 'question');
  });

  it('returns done when no prompt char and no question', () => {
    const state = detectState('Done.', ['some output']);
    assert.equal(state, 'done');
  });

  it('handles null last_assistant_message without throwing', () => {
    const state = detectState(null, ['some output', '\u276f']);
    assert.equal(state, 'idle_prompt');
  });
});
