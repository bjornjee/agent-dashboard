#!/usr/bin/env node
'use strict';

// Codex 0.130 parses hook stdout as JSON conforming to
// codex-rs/hooks/schema/generated/<event>.command.output.schema.json.
// Empty stdout fails with "hook returned invalid <event> JSON output".
// Every codex hook script must emit at least `{}` (all schema fields are
// optional with defaults). These tests spawn the actual scripts and assert
// stdout is parseable JSON.

const { describe, it, beforeEach, afterEach } = require('node:test');
const assert = require('node:assert/strict');
const { spawnSync } = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');

const ROOT = __dirname;

function runHook(scriptName, payload, env = {}) {
  const tmpHome = fs.mkdtempSync(path.join(os.tmpdir(), 'hook-stdout-test-'));
  try {
    const result = spawnSync('node', [path.join(ROOT, scriptName)], {
      input: JSON.stringify(payload),
      env: {
        ...process.env,
        HOME: tmpHome,
        AGENT_DASHBOARD_DIR: path.join(tmpHome, '.agent-dashboard'),
        PLUGIN_ROOT: ROOT,
        CLAUDE_PLUGIN_ROOT: ROOT,
        TMUX_PANE: '',
        ...env,
      },
      timeout: 5000,
      encoding: 'utf8',
    });
    return result;
  } finally {
    fs.rmSync(tmpHome, { recursive: true, force: true });
  }
}

const SESSION_ID = '00000000-0000-0000-0000-000000000001';

const TOOL_USE_EVENTS = [
  {
    name: 'PreToolUse',
    payload: {
      session_id: SESSION_ID,
      cwd: '/tmp',
      model: 'gpt-5.5',
      hook_event_name: 'PreToolUse',
      tool_name: 'Bash',
      tool_input: { command: 'ls' },
    },
  },
  {
    name: 'PostToolUse',
    payload: {
      session_id: SESSION_ID,
      cwd: '/tmp',
      model: 'gpt-5.5',
      hook_event_name: 'PostToolUse',
      tool_name: 'Bash',
      tool_input: { command: 'ls' },
      tool_response: { output: 'ok' },
    },
  },
  {
    name: 'PermissionRequest',
    payload: {
      session_id: SESSION_ID,
      cwd: '/tmp',
      model: 'gpt-5.5',
      hook_event_name: 'PermissionRequest',
      tool_name: 'Edit',
      tool_input: { file_path: '/tmp/x' },
    },
  },
];

const REPORTER_EVENTS = [
  {
    name: 'SessionStart',
    payload: {
      session_id: SESSION_ID,
      cwd: '/tmp',
      model: 'gpt-5.5',
      hook_event_name: 'SessionStart',
    },
  },
  {
    name: 'Stop',
    payload: {
      session_id: SESSION_ID,
      cwd: '/tmp',
      model: 'gpt-5.5',
      hook_event_name: 'Stop',
    },
  },
];

describe('codex hook stdout contract', () => {
  for (const { name, payload } of TOOL_USE_EVENTS) {
    it(`agent-state-fast.js emits valid JSON on stdout for ${name}`, () => {
      const result = runHook('agent-state-fast.js', payload);
      assert.equal(result.status, 0, `exit ${result.status}; stderr=${result.stderr}`);
      assert.notEqual(result.stdout.trim(), '', `${name}: stdout empty`);
      assert.doesNotThrow(
        () => JSON.parse(result.stdout),
        `${name}: stdout not parseable JSON: ${JSON.stringify(result.stdout)}`
      );
    });
  }

  for (const { name, payload } of REPORTER_EVENTS) {
    it(`agent-state-reporter.js emits valid JSON on stdout for ${name}`, () => {
      const result = runHook('agent-state-reporter.js', payload);
      assert.equal(result.status, 0, `exit ${result.status}; stderr=${result.stderr}`);
      assert.notEqual(result.stdout.trim(), '', `${name}: stdout empty`);
      assert.doesNotThrow(
        () => JSON.parse(result.stdout),
        `${name}: stdout not parseable JSON: ${JSON.stringify(result.stdout)}`
      );
    });
  }
});
