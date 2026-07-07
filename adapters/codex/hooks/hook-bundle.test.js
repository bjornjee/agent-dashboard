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

function assertOnlyKeys(value, allowed, context) {
  const unknown = Object.keys(value)
    .filter(key => !allowed.includes(key))
    .sort();
  assert.deepEqual(unknown, [], context);
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
        '$HOME/.codex/hooks/agent-dashboard/commit-lint.sh',
        '$HOME/.codex/hooks/agent-dashboard/desktop-notify.sh',
        '$HOME/.codex/hooks/agent-dashboard/pr-detect.sh',
        '$HOME/.codex/hooks/agent-dashboard/pr-skill-detect.sh',
        '$HOME/.codex/hooks/agent-dashboard/pr-skill-gate.sh',
        '$HOME/.codex/hooks/agent-dashboard/test-gate.sh',
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
      'commit-lint.sh',
      'desktop-notify.sh',
      'pr-detect.sh',
      'pr-skill-detect.sh',
      'pr-skill-gate.sh',
      'test-gate.sh',
      'warn-destructive.sh',
      'agent-state-fast.js',
      'agent-state-reporter.js',
      'block-main-commit.js',
      'commit-lint.js',
      'desktop-notify.js',
      'claim-worktree.js',
      'pr-detect.js',
      'pr-skill-detect.js',
      'pr-skill-gate.js',
      'test-gate.js',
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

  it('keeps the agent-state package in lock-step with the claude-code copy', () => {
    // The codex packages/agent-state/* files are byte-identical copies of the
    // claude-code originals, kept in sync by hand (no sync script). This guard
    // is what stops the two writeState implementations from drifting apart —
    // a real hazard now that report_seq ordering lives in both.
    for (const rel of ['index.js', 'test.js']) {
      const codexFile = path.join(ROOT, 'packages', 'agent-state', rel);
      const claudeFile = path.join(ROOT, '..', '..', 'claude-code', 'packages', 'agent-state', rel);
      assert.equal(
        fs.readFileSync(codexFile, 'utf8'),
        fs.readFileSync(claudeFile, 'utf8'),
        `codex packages/agent-state/${rel} must match the claude-code copy`
      );
    }
  });

  it('registers PR skill detection on user prompt submit', () => {
    const hooks = readJson(HOOKS_JSON);
    const entries = hooks.hooks.UserPromptSubmit || [];
    const hookCommands = entries.flatMap(entry => entry.hooks.map(hook => hook.command));

    assert.deepEqual(hookCommands, ['$HOME/.codex/hooks/agent-dashboard/pr-skill-detect.sh']);
  });

  it('registers desktop notifications for attention events', () => {
    const hooks = readJson(HOOKS_JSON).hooks;

    assert.deepEqual(
      hooks.Notification.flatMap(entry => entry.hooks.map(hook => hook.command)),
      ['$HOME/.codex/hooks/agent-dashboard/desktop-notify.sh']
    );
    assert.deepEqual(
      hooks.StopFailure.flatMap(entry => entry.hooks.map(hook => hook.command)),
      ['$HOME/.codex/hooks/agent-dashboard/desktop-notify.sh']
    );
  });

  it('keeps the copied codex source folder flat', () => {
    assert.equal(fs.existsSync(path.join(ROOT, 'scripts', 'hooks')), false);
  });
});

describe('codex hook manifests reject unknown top-level fields', () => {
  for (const manifest of ['hooks.json', 'plugin-hooks.json']) {
    it(`${manifest} declares only "hooks" at the top level`, () => {
      const top = readJson(path.join(ROOT, manifest));
      assert.deepEqual(Object.keys(top).sort(), ['hooks']);
    });
  }
});

describe('codex hook manifests reject unknown hook-entry fields', () => {
  for (const manifest of ['hooks.json', 'plugin-hooks.json']) {
    it(`${manifest} uses only codex hook config fields`, () => {
      const top = readJson(path.join(ROOT, manifest));
      const matcherGroupFields = ['hooks', 'matcher'];
      const hookHandlerFields = [
        'async',
        'command',
        'commandWindows',
        'statusMessage',
        'timeout',
        'type',
      ];

      for (const [eventName, entries] of Object.entries(top.hooks)) {
        for (const [entryIndex, entry] of entries.entries()) {
          assertOnlyKeys(
            entry,
            matcherGroupFields,
            `${manifest} ${eventName}[${entryIndex}]`
          );

          for (const [hookIndex, hook] of entry.hooks.entries()) {
            assertOnlyKeys(
              hook,
              hookHandlerFields,
              `${manifest} ${eventName}[${entryIndex}].hooks[${hookIndex}]`
            );
          }
        }
      }
    });
  }
});
