#!/usr/bin/env node
'use strict';

const { describe, it } = require('node:test');
const assert = require('node:assert/strict');
const { spawnSync } = require('node:child_process');
const { mkdtempSync, writeFileSync, mkdirSync, rmSync } = require('node:fs');
const os = require('node:os');
const path = require('node:path');

const { isWorktree, isCodexAvailable, shouldNudge, shouldSkip, NUDGE_MESSAGE } = require('./codex-delegation-gate');

const HOOK_PATH = path.join(__dirname, 'codex-delegation-gate.js');

function runHook(input, env = {}) {
  return spawnSync('node', [HOOK_PATH], {
    input: typeof input === 'string' ? input : JSON.stringify(input),
    encoding: 'utf8',
    timeout: 5000,
    env: { ...process.env, ...env },
  });
}

describe('isWorktree', () => {
  it('.git is a file (worktree)', () => {
    const tmp = mkdtempSync(path.join(os.tmpdir(), 'codex-gate-'));
    try {
      writeFileSync(path.join(tmp, '.git'), 'gitdir: /some/path/.git/worktrees/test');
      assert.equal(isWorktree(tmp), true);
    } finally {
      rmSync(tmp, { recursive: true, force: true });
    }
  });

  it('.git is a directory (main repo)', () => {
    const tmp = mkdtempSync(path.join(os.tmpdir(), 'codex-gate-'));
    try {
      mkdirSync(path.join(tmp, '.git'));
      assert.equal(isWorktree(tmp), false);
    } finally {
      rmSync(tmp, { recursive: true, force: true });
    }
  });

  it('.git does not exist', () => {
    assert.equal(isWorktree(os.tmpdir()), false);
  });

  it('null cwd falls back to process.cwd()', () => {
    assert.equal(isWorktree(null), false);
  });
});

describe('isCodexAvailable', () => {
  it('returns a boolean', () => {
    assert.equal(typeof isCodexAvailable(), 'boolean');
  });
});

describe('shouldSkip', () => {
  it('false when SKIP_CODEX_GATE unset', () => {
    const orig = process.env.SKIP_CODEX_GATE;
    delete process.env.SKIP_CODEX_GATE;
    try {
      assert.equal(shouldSkip(), false);
    } finally {
      if (orig !== undefined) process.env.SKIP_CODEX_GATE = orig;
    }
  });

  it('true when SKIP_CODEX_GATE=1', () => {
    const orig = process.env.SKIP_CODEX_GATE;
    process.env.SKIP_CODEX_GATE = '1';
    try {
      assert.equal(shouldSkip(), true);
    } finally {
      if (orig !== undefined) process.env.SKIP_CODEX_GATE = orig;
      else delete process.env.SKIP_CODEX_GATE;
    }
  });

  it('false when SKIP_CODEX_GATE=0', () => {
    const orig = process.env.SKIP_CODEX_GATE;
    process.env.SKIP_CODEX_GATE = '0';
    try {
      assert.equal(shouldSkip(), false);
    } finally {
      if (orig !== undefined) process.env.SKIP_CODEX_GATE = orig;
      else delete process.env.SKIP_CODEX_GATE;
    }
  });
});

describe('shouldNudge', () => {
  it('false for non-worktree cwd', () => {
    assert.equal(shouldNudge(os.tmpdir()), false);
  });

  it('false when .git is a directory', () => {
    const tmp = mkdtempSync(path.join(os.tmpdir(), 'codex-gate-'));
    try {
      mkdirSync(path.join(tmp, '.git'));
      assert.equal(shouldNudge(tmp), false);
    } finally {
      rmSync(tmp, { recursive: true, force: true });
    }
  });
});

describe('hook integration', () => {
  it('exits 0 with no stderr for empty input', () => {
    const result = runHook({});
    assert.equal(result.status, 0);
    assert.equal(result.stderr, '');
  });

  it('exits 0 with no stderr for non-worktree cwd', () => {
    const result = runHook({ cwd: os.tmpdir(), tool_name: 'ExitPlanMode' });
    assert.equal(result.status, 0);
    assert.equal(result.stderr, '');
  });

  it('exits 0 with no stderr when SKIP_CODEX_GATE=1', () => {
    const tmp = mkdtempSync(path.join(os.tmpdir(), 'codex-gate-'));
    try {
      writeFileSync(path.join(tmp, '.git'), 'gitdir: /some/path');
      const result = runHook(
        { cwd: tmp, tool_name: 'ExitPlanMode' },
        { SKIP_CODEX_GATE: '1' },
      );
      assert.equal(result.status, 0);
      assert.equal(result.stderr, '');
    } finally {
      rmSync(tmp, { recursive: true, force: true });
    }
  });

  it('exits 0 with nudge in stderr when in worktree and codex available', () => {
    if (!isCodexAvailable()) return;
    const tmp = mkdtempSync(path.join(os.tmpdir(), 'codex-gate-'));
    try {
      writeFileSync(path.join(tmp, '.git'), 'gitdir: /some/path');
      const result = runHook({ cwd: tmp, tool_name: 'ExitPlanMode' });
      assert.equal(result.status, 0);
      assert.ok(result.stderr.includes('/codex-delegate'));
      assert.ok(result.stderr.includes('10+ files'));
    } finally {
      rmSync(tmp, { recursive: true, force: true });
    }
  });

  it('exits 0 gracefully on bad JSON input', () => {
    const result = runHook('not valid json');
    assert.equal(result.status, 0);
  });
});
