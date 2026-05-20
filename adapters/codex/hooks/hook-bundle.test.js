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
      ]
    );
  });

  it('ships wrappers and runtime files required by the hook commands', () => {
    for (const file of [
      'agent-state-fast.sh',
      'agent-state-reporter.sh',
      'agent-state-fast.js',
      'agent-state-reporter.js',
      'effort-config.js',
      'packages/agent-state/index.js',
      'packages/tmux/index.js',
      'packages/git-status/index.js',
      'packages/toml-lite/index.js',
    ]) {
      assert.equal(fs.existsSync(path.join(ROOT, file)), true, file);
    }
  });

  it('keeps the copied codex source folder flat', () => {
    assert.equal(fs.existsSync(path.join(ROOT, 'scripts', 'hooks')), false);
  });
});
