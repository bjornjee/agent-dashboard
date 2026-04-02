#!/usr/bin/env node
'use strict';

const { describe, it } = require('node:test');
const assert = require('node:assert/strict');

const { buildReportEntry } = require('./agent-state-reporter');

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
