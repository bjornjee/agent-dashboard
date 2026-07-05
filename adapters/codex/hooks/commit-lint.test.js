#!/usr/bin/env node
'use strict';

const { describe, it } = require('node:test');
const assert = require('node:assert/strict');
const path = require('node:path');
const { spawnSync } = require('node:child_process');
const { extractCommitMessage, validateCommitMessage, VALID_TYPES } = require('./commit-lint');

const HOOK_PATH = path.join(__dirname, 'commit-lint.js');

describe('extractCommitMessage', () => {
  it('extracts from double-quoted -m', () => {
    assert.equal(
      extractCommitMessage('git commit -m "feat: add login"'),
      'feat: add login'
    );
  });

  it('extracts from single-quoted -m', () => {
    assert.equal(
      extractCommitMessage("git commit -m 'fix: resolve crash'"),
      'fix: resolve crash'
    );
  });

  it('extracts from HEREDOC pattern', () => {
    const cmd = 'git commit -m "$(cat <<\'EOF\'\n' +
      'feat: add auth flow\n' +
      '\n' +
      'Co-Authored-By: Claude\n' +
      'EOF\n' +
      ')"';
    assert.equal(extractCommitMessage(cmd), 'feat: add auth flow\n\nCo-Authored-By: Claude');
  });

  it('handles git commit with flags before -m', () => {
    assert.equal(
      extractCommitMessage('git commit --no-edit -m "chore: bump version"'),
      'chore: bump version'
    );
  });

  it('returns null for commands without -m', () => {
    assert.equal(extractCommitMessage('git commit --amend'), null);
  });

  it('returns null for non-commit commands', () => {
    assert.equal(extractCommitMessage('git status'), null);
  });
});

describe('validateCommitMessage', () => {
  it('accepts all valid types', () => {
    for (const type of VALID_TYPES) {
      const result = validateCommitMessage(`${type}: some description`);
      assert.equal(result.valid, true, `${type} should be valid`);
    }
  });

  it('rejects missing type prefix', () => {
    const result = validateCommitMessage('add new feature');
    assert.equal(result.valid, false);
    assert.ok(result.reason.includes('does not follow conventional format'));
  });

  it('rejects invalid type', () => {
    const result = validateCommitMessage('update: something');
    assert.equal(result.valid, false);
  });

  it('rejects missing space after colon', () => {
    const result = validateCommitMessage('feat:no space');
    assert.equal(result.valid, false);
  });

  it('rejects empty description after type', () => {
    const result = validateCommitMessage('feat: ');
    assert.equal(result.valid, false);
  });

  it('validates only first line of multi-line message', () => {
    const result = validateCommitMessage('feat: add auth\n\nCo-Authored-By: Claude');
    assert.equal(result.valid, true);
  });

  it('returns invalid for null message', () => {
    const result = validateCommitMessage(null);
    assert.equal(result.valid, false);
    assert.ok(result.reason.includes('Could not parse'));
  });
});

describe('hook integration', () => {
  function runHook(input) {
    return spawnSync('node', [HOOK_PATH], {
      input: JSON.stringify(input),
      encoding: 'utf8',
      timeout: 5000,
    });
  }

  it('signals exit 2 for a non-conventional commit message', () => {
    const result = runHook({
      session_id: 't1',
      hook_event_name: 'PostToolUse',
      tool_name: 'Bash',
      tool_input: { command: 'git commit -m "bad message"' },
    });
    assert.equal(result.status, 2, `expected block, got ${result.status}: ${result.stderr}`);
    assert.match(result.stderr, /conventional format/);
  });

  it('emits {} and exits 0 for a conventional commit message', () => {
    const result = runHook({
      session_id: 't1',
      hook_event_name: 'PostToolUse',
      tool_name: 'Bash',
      tool_input: { command: 'git commit -m "feat: add login"' },
    });
    assert.equal(result.status, 0, `expected pass, got ${result.status}: ${result.stderr}`);
    assert.deepEqual(JSON.parse(result.stdout), {});
  });

  it('emits {} for non-commit commands', () => {
    const result = runHook({
      session_id: 't1',
      hook_event_name: 'PostToolUse',
      tool_name: 'Bash',
      tool_input: { command: 'ls -la' },
    });
    assert.equal(result.status, 0);
    assert.deepEqual(JSON.parse(result.stdout), {});
  });
});
