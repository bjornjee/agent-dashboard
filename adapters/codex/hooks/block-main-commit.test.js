#!/usr/bin/env node
'use strict';

const { describe, it, before, after } = require('node:test');
const assert = require('node:assert/strict');
const { spawnSync } = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { isCommitOnMain } = require('./block-main-commit');

const HOOK_PATH = path.join(__dirname, 'block-main-commit.js');

function initRepo(branch) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'block-main-commit-test-'));
  const env = {
    ...process.env,
    GIT_AUTHOR_NAME: 'test', GIT_AUTHOR_EMAIL: 't@t',
    GIT_COMMITTER_NAME: 'test', GIT_COMMITTER_EMAIL: 't@t',
  };
  spawnSync('git', ['init', '-q', '-b', branch], { cwd: dir, env });
  spawnSync('git', ['commit', '-q', '--allow-empty', '-m', 'init'], { cwd: dir, env });
  return dir;
}

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
  let featureRepo, mainRepo;

  before(() => {
    featureRepo = initRepo('feat/test-branch');
    mainRepo = initRepo('main');
  });

  after(() => {
    fs.rmSync(featureRepo, { recursive: true, force: true });
    fs.rmSync(mainRepo, { recursive: true, force: true });
  });

  function runHook(input) {
    return spawnSync('node', [HOOK_PATH], {
      input: JSON.stringify(input),
      encoding: 'utf8',
      timeout: 5000,
    });
  }

  it('uses input.cwd to resolve branch (feature branch → allowed)', () => {
    const result = runHook({
      session_id: 'test-123',
      hook_event_name: 'PreToolUse',
      tool_name: 'Bash',
      tool_input: { command: 'git commit -m "test"' },
      cwd: featureRepo,
    });
    assert.equal(result.status, 0, `expected pass-through but got exit ${result.status}: ${result.stderr}`);
  });

  it('blocks when input.cwd points to a main branch repo', () => {
    const result = runHook({
      session_id: 'test-123',
      hook_event_name: 'PreToolUse',
      tool_name: 'Bash',
      tool_input: { command: 'git commit -m "test"' },
      cwd: mainRepo,
    });
    assert.equal(result.status, 2, `expected block but got exit ${result.status}: ${result.stderr}`);
  });

  it('emits "{}\\n" on stdout for non-blocking input (Codex JSON contract)', () => {
    const result = runHook({
      session_id: 'test-123',
      hook_event_name: 'PreToolUse',
      tool_name: 'Bash',
      tool_input: { command: 'git status' },
      cwd: featureRepo,
    });
    assert.equal(result.status, 0, `expected pass-through but got exit ${result.status}: ${result.stderr}`);
    assert.equal(result.stdout, '{}\n');
  });
});
