#!/usr/bin/env node
'use strict';

const { describe, it } = require('node:test');
const assert = require('node:assert/strict');
const { spawnSync } = require('node:child_process');
const path = require('node:path');

const ROOT = __dirname;

function runHook(scriptName, payload, env = {}) {
  return spawnSync('node', [path.join(ROOT, scriptName)], {
    input: JSON.stringify(payload),
    env: {
      ...process.env,
      AGENT_DASHBOARD_DIR: path.join(process.env.TMPDIR || '/tmp', 'agent-dashboard-hook-test'),
      TMUX_PANE: '',
      ...env,
    },
    timeout: 5000,
    encoding: 'utf8',
  });
}

function parseStdout(result) {
  assert.notEqual(result.stdout.trim(), '', 'stdout must not be empty');
  return JSON.parse(result.stdout);
}

const BASE_PRE_TOOL_USE = {
  session_id: '00000000-0000-0000-0000-000000000001',
  cwd: '/tmp',
  model: 'gpt-5.5',
  hook_event_name: 'PreToolUse',
  tool_name: 'Bash',
};

const BASE_POST_TOOL_USE = {
  ...BASE_PRE_TOOL_USE,
  hook_event_name: 'PostToolUse',
  tool_response: { output: 'ok' },
};

describe('codex hook stdout contract for claude plugin hooks', () => {
  for (const { scriptName, payload } of [
    { scriptName: 'agent-state-fast.js', payload: BASE_PRE_TOOL_USE },
    { scriptName: 'agent-state-reporter.js', payload: { ...BASE_PRE_TOOL_USE, hook_event_name: 'Stop' } },
    { scriptName: 'warn-destructive.js', payload: BASE_PRE_TOOL_USE },
    { scriptName: 'block-main-commit.js', payload: BASE_PRE_TOOL_USE },
    { scriptName: 'pr-skill-gate.js', payload: BASE_PRE_TOOL_USE },
    { scriptName: 'codex-write-gate.js', payload: BASE_PRE_TOOL_USE },
    { scriptName: 'commit-lint.js', payload: BASE_POST_TOOL_USE },
    { scriptName: 'test-gate.js', payload: BASE_POST_TOOL_USE },
  ]) {
    it(`${scriptName} emits {} on allow`, () => {
      const result = runHook(scriptName, {
        ...payload,
        tool_input: { command: 'git status' },
      });

      assert.equal(result.status, 0, `exit ${result.status}; stderr=${result.stderr}`);
      assert.deepEqual(parseStdout(result), {});
    });
  }

  it('warn-destructive.js emits structured deny JSON on block', () => {
    const result = runHook('warn-destructive.js', {
      ...BASE_PRE_TOOL_USE,
      tool_input: { command: 'git reset --hard' },
    });

    assert.equal(result.status, 2, `exit ${result.status}; stderr=${result.stderr}`);
    assert.deepEqual(parseStdout(result), {
      hookSpecificOutput: {
        permissionDecision: 'deny',
        permissionDecisionReason: 'Blocked: "git reset --hard" is a destructive command. If intentional, ask the user to run it manually.',
      },
    });
  });

  it('pr-skill-gate.js emits structured deny JSON on block', () => {
    const result = runHook('pr-skill-gate.js', {
      ...BASE_PRE_TOOL_USE,
      tool_input: { command: 'gh pr create --title x' },
    });

    assert.equal(result.status, 2, `exit ${result.status}; stderr=${result.stderr}`);
    assert.equal(
      parseStdout(result).hookSpecificOutput.permissionDecision,
      'deny'
    );
  });

  it('codex-write-gate.js emits structured deny JSON on block', () => {
    const result = runHook('codex-write-gate.js', {
      ...BASE_PRE_TOOL_USE,
      tool_input: { command: 'node codex-companion.mjs task foo' },
    });

    assert.equal(result.status, 2, `exit ${result.status}; stderr=${result.stderr}`);
    assert.equal(
      parseStdout(result).hookSpecificOutput.permissionDecision,
      'deny'
    );
  });
});
