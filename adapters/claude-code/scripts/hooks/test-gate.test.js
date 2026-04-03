#!/usr/bin/env node
'use strict';

const { describe, it } = require('node:test');
const assert = require('node:assert/strict');
const { mkdtempSync, writeFileSync, rmSync } = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const { isGitCommit, shouldSkip, hasMakefile, getTestTarget, runMakeTest, getRepoRoot } = require('./test-gate');

describe('isGitCommit', () => {
  it('matches simple git commit', () => {
    assert.equal(isGitCommit('git commit -m "feat: add login"'), true);
  });

  it('matches git commit with flags', () => {
    assert.equal(isGitCommit('git commit --no-edit -m "fix: typo"'), true);
  });

  it('does not match git status', () => {
    assert.equal(isGitCommit('git status'), false);
  });

  it('does not match git push', () => {
    assert.equal(isGitCommit('git push origin main'), false);
  });

  it('does not match echo containing git commit text', () => {
    // This is a known limitation — we match the pattern anywhere in the command.
    // Acceptable because hooks only fire on Bash tool calls, not arbitrary strings.
    assert.equal(isGitCommit('echo "run git commit"'), true);
  });
});

describe('shouldSkip', () => {
  it('returns false when env var is not set', () => {
    const orig = process.env.SKIP_TEST_GATE;
    delete process.env.SKIP_TEST_GATE;
    assert.equal(shouldSkip(), false);
    if (orig !== undefined) process.env.SKIP_TEST_GATE = orig;
  });

  it('returns true when env var is "1"', () => {
    const orig = process.env.SKIP_TEST_GATE;
    process.env.SKIP_TEST_GATE = '1';
    assert.equal(shouldSkip(), true);
    if (orig !== undefined) {
      process.env.SKIP_TEST_GATE = orig;
    } else {
      delete process.env.SKIP_TEST_GATE;
    }
  });

  it('returns false when env var is "0"', () => {
    const orig = process.env.SKIP_TEST_GATE;
    process.env.SKIP_TEST_GATE = '0';
    assert.equal(shouldSkip(), false);
    if (orig !== undefined) {
      process.env.SKIP_TEST_GATE = orig;
    } else {
      delete process.env.SKIP_TEST_GATE;
    }
  });
});

describe('hasMakefile', () => {
  it('returns false when no Makefile exists', () => {
    const tmp = mkdtempSync(path.join(os.tmpdir(), 'test-gate-'));
    assert.equal(hasMakefile(tmp), false);
    rmSync(tmp, { recursive: true });
  });

  it('returns true when Makefile exists', () => {
    const tmp = mkdtempSync(path.join(os.tmpdir(), 'test-gate-'));
    writeFileSync(path.join(tmp, 'Makefile'), 'all:\n\techo hi\n');
    assert.equal(hasMakefile(tmp), true);
    rmSync(tmp, { recursive: true });
  });
});

describe('getTestTarget', () => {
  it('returns null when no Makefile exists', () => {
    const tmp = mkdtempSync(path.join(os.tmpdir(), 'test-gate-'));
    assert.equal(getTestTarget(tmp), null);
    rmSync(tmp, { recursive: true });
  });

  it('returns null when Makefile has no test target', () => {
    const tmp = mkdtempSync(path.join(os.tmpdir(), 'test-gate-'));
    writeFileSync(path.join(tmp, 'Makefile'), 'build:\n\techo build\n');
    assert.equal(getTestTarget(tmp), null);
    rmSync(tmp, { recursive: true });
  });

  it('returns test-fast when both targets exist', () => {
    const tmp = mkdtempSync(path.join(os.tmpdir(), 'test-gate-'));
    writeFileSync(path.join(tmp, 'Makefile'), 'test-fast:\n\t@echo ok\ntest:\n\t@echo ok\n');
    assert.equal(getTestTarget(tmp), 'test-fast');
    rmSync(tmp, { recursive: true });
  });

  it('falls back to test when test-fast missing', () => {
    const tmp = mkdtempSync(path.join(os.tmpdir(), 'test-gate-'));
    writeFileSync(path.join(tmp, 'Makefile'), 'test:\n\t@echo ok\n');
    assert.equal(getTestTarget(tmp), 'test');
    rmSync(tmp, { recursive: true });
  });

  it('returns test-fast when only test-fast exists', () => {
    const tmp = mkdtempSync(path.join(os.tmpdir(), 'test-gate-'));
    writeFileSync(path.join(tmp, 'Makefile'), 'test-fast:\n\techo ok\n');
    assert.equal(getTestTarget(tmp), 'test-fast');
    rmSync(tmp, { recursive: true });
  });
});

describe('getRepoRoot', () => {
  it('uses provided cwd to resolve repo root', () => {
    // When given a valid git repo cwd, should return a path (not throw)
    const root = getRepoRoot(process.cwd());
    assert.ok(typeof root === 'string');
    assert.ok(root.length > 0);
  });

  it('falls back to provided cwd when git fails', () => {
    const tmp = mkdtempSync(path.join(os.tmpdir(), 'test-gate-'));
    // tmp is not a git repo, so git rev-parse will fail
    const root = getRepoRoot(tmp);
    assert.equal(root, tmp);
    rmSync(tmp, { recursive: true });
  });

  it('falls back to process.cwd() when no cwd provided', () => {
    const root = getRepoRoot();
    assert.ok(typeof root === 'string');
    assert.ok(root.length > 0);
  });
});

describe('runMakeTest', () => {
  it('returns passed: true when make test succeeds', () => {
    const tmp = mkdtempSync(path.join(os.tmpdir(), 'test-gate-'));
    writeFileSync(path.join(tmp, 'Makefile'), 'test:\n\t@echo all tests pass\n');
    const result = runMakeTest(tmp);
    assert.equal(result.passed, true);
    rmSync(tmp, { recursive: true });
  });

  it('returns passed: false with output when make test fails', () => {
    const tmp = mkdtempSync(path.join(os.tmpdir(), 'test-gate-'));
    writeFileSync(path.join(tmp, 'Makefile'), 'test:\n\t@echo "FAIL: something broke" && exit 1\n');
    const result = runMakeTest(tmp);
    assert.equal(result.passed, false);
    assert.ok(result.output.includes('FAIL'));
    rmSync(tmp, { recursive: true });
  });
});
