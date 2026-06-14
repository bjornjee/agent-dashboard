#!/usr/bin/env node
'use strict';

const { describe, it } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const ROOT = __dirname;
const HOOKS_JSON = path.join(ROOT, 'hooks.json');

function readJson(filePath) {
  return JSON.parse(fs.readFileSync(filePath, 'utf8'));
}

function hasAsyncHook(value) {
  if (!value || typeof value !== 'object') return false;
  if (value.async === true) return true;
  return Object.values(value).some(hasAsyncHook);
}

function commands(hooksConfig) {
  return Object.values(hooksConfig.hooks).flatMap(entries =>
    entries.flatMap(entry => entry.hooks.map(hook => hook.command))
  );
}

describe('codex global hook bundle', () => {
  it('defines only synchronous hooks', () => {
    assert.equal(hasAsyncHook(readJson(HOOKS_JSON)), false);
  });

  it('uses stable global wrapper commands', () => {
    assert.deepEqual(
      [...new Set(commands(readJson(HOOKS_JSON)))].sort(),
      [
        '$HOME/.codex/hooks/agent-dashboard/agent-state-fast.sh',
        '$HOME/.codex/hooks/agent-dashboard/agent-state-reporter.sh',
        '$HOME/.codex/hooks/agent-dashboard/block-main-commit.sh',
        '$HOME/.codex/hooks/agent-dashboard/pr-skill-detect.sh',
        '$HOME/.codex/hooks/agent-dashboard/warn-destructive.sh',
      ]
    );
  });

  it('registers codex subagent lifecycle events', () => {
    const hooks = readJson(HOOKS_JSON).hooks;
    assert.ok(hooks.SubagentStart, 'SubagentStart hook should be registered');
    assert.ok(hooks.SubagentStop, 'SubagentStop hook should be registered');
  });

  it('ships wrappers and runtime files required by the hook commands', () => {
    for (const file of [
      'agent-state-fast.sh',
      'agent-state-reporter.sh',
      'block-main-commit.sh',
      'pr-skill-detect.sh',
      'warn-destructive.sh',
      'agent-state-fast.js',
      'agent-state-reporter.js',
      'block-main-commit.js',
      'claim-worktree.js',
      'pr-skill-detect.js',
      'warn-destructive.js',
      'effort-config.js',
      'packages/agent-state/index.js',
      'packages/tmux/index.js',
      'packages/git-status/index.js',
      'packages/toml-lite/index.js',
    ]) {
      assert.equal(fs.existsSync(path.join(ROOT, file)), true, file);
    }
  });

  it('registers PR skill detection on user prompt submit', () => {
    const hooks = readJson(HOOKS_JSON);
    const entries = hooks.hooks.UserPromptSubmit || [];
    const hookCommands = entries.flatMap(entry => entry.hooks.map(hook => hook.command));

    assert.deepEqual(hookCommands, ['$HOME/.codex/hooks/agent-dashboard/pr-skill-detect.sh']);
  });

  it('keeps the copied codex source folder flat', () => {
    assert.equal(fs.existsSync(path.join(ROOT, 'scripts', 'hooks')), false);
  });
});
