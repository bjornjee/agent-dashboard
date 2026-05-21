#!/usr/bin/env node
'use strict';

const { describe, it } = require('node:test');
const assert = require('node:assert/strict');
const { spawnSync } = require('node:child_process');
const path = require('node:path');
const { isCommitOnMain } = require('./block-main-commit');

const HOOK_PATH = path.join(__dirname, 'block-main-commit.js');

describe('isCommitOnMain', () => {
  describe('should block git commit on main/master', () => {
    it('git commit -m "msg" on main', () =>
      assert.equal(isCommitOnMain('git commit -m "msg"', 'main'), true));
    it('git commit -am "msg" on main', () =>
      assert.equal(isCommitOnMain('git commit -am "msg"', 'main'), true));
    it('git commit on master', () =>
      assert.equal(isCommitOnMain('git commit -m "msg"', 'master'), true));
    it('chained: git add . && git commit -m "msg" on main', () =>
      assert.equal(isCommitOnMain('git add . && git commit -m "msg"', 'main'), true));
    it('semicolon: git add .; git commit -m "msg" on main', () =>
      assert.equal(isCommitOnMain('git add .; git commit -m "msg"', 'main'), true));
  });

  describe('should NOT block', () => {
    it('git commit on feature branch', () =>
      assert.equal(isCommitOnMain('git commit -m "msg"', 'feat/my-feature'), false));
    it('git commit on fix branch', () =>
      assert.equal(isCommitOnMain('git commit -m "msg"', 'fix/bug'), false));
    it('git status on main', () =>
      assert.equal(isCommitOnMain('git status', 'main'), false));
    it('git log on main', () =>
      assert.equal(isCommitOnMain('git log', 'main'), false));
    it('git diff on main', () =>
      assert.equal(isCommitOnMain('git diff', 'main'), false));
    it('git push on main', () =>
      assert.equal(isCommitOnMain('git push origin main', 'main'), false));
    it('echo "git commit" on main (not actual git)', () =>
      assert.equal(isCommitOnMain('echo "git commit"', 'main'), false));
  });
});

describe('hook integration: cwd resolution', () => {
  // Run the hook as a subprocess with JSON on stdin.
  // The worktree is on a feature branch, so git commit should NOT be blocked
  // even though the hook process itself runs from the main repo (on main).
  function runHook(input) {
    const result = spawnSync('node', [HOOK_PATH], {
      input: JSON.stringify(input),
      encoding: 'utf8',
      timeout: 5000,
    });
    return result;
  }

  it('uses input.cwd to resolve branch (feature branch → allowed)', () => {
    // This worktree is on feat/hook-blocked-state, not main
    const worktreeCwd = path.resolve(__dirname, '..', '..', '..', '..');
    const result = runHook({
      session_id: 'test-123',
      hook_event_name: 'PreToolUse',
      tool_name: 'Bash',
      tool_input: { command: 'git commit -m "test"' },
      cwd: worktreeCwd,
    });
    // Should pass through (exit 0), not block (exit 2)
    assert.equal(result.status, 0, `expected pass-through but got exit ${result.status}: ${result.stderr}`);
  });

  it('blocks when input.cwd points to a main branch repo', () => {
    // Use the main repo root which is on main
    const mainRepoCwd = path.resolve(__dirname, '..', '..', '..', '..', '..', '..', '..', 'agent-dashboard');
    const result = runHook({
      session_id: 'test-123',
      hook_event_name: 'PreToolUse',
      tool_name: 'Bash',
      tool_input: { command: 'git commit -m "test"' },
      cwd: mainRepoCwd,
    });
    assert.equal(result.status, 2, `expected block but got exit ${result.status}`);
  });
});
