#!/usr/bin/env node
'use strict';

const { describe, it } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { detectPR, buildPRUpdate, PR_URL_RE, getCachedGhAuth } = require('./pr-detect');

describe('detectPR', () => {
  it('detects gh pr create with URL in output', () => {
    const result = detectPR(
      'gh pr create --title "feat: add foo" --body "bar"',
      'https://github.com/user/repo/pull/42\n'
    );
    assert.deepEqual(result, { action: 'created', prUrl: 'https://github.com/user/repo/pull/42' });
  });

  it('detects gh pr create without URL in output', () => {
    const result = detectPR(
      'gh pr create --title "feat: add foo"',
      'Creating pull request...\n'
    );
    assert.deepEqual(result, { action: 'created', prUrl: null });
  });

  it('detects gh pr create with heredoc body', () => {
    const cmd = `gh pr create --title "feat: stuff" --body "$(cat <<'EOF'\n## Summary\nEOF\n)"`;
    const result = detectPR(cmd, 'https://github.com/org/repo/pull/123');
    assert.equal(result.action, 'created');
    assert.equal(result.prUrl, 'https://github.com/org/repo/pull/123');
  });

  it('detects gh pr merge', () => {
    const result = detectPR(
      'gh pr merge 42 --squash',
      'Merged pull request #42\n'
    );
    assert.deepEqual(result, { action: 'merged', prUrl: null });
  });

  it('detects gh pr merge with URL in command', () => {
    const result = detectPR(
      'gh pr merge https://github.com/user/repo/pull/42 --squash',
      'Merged\n'
    );
    assert.deepEqual(result, { action: 'merged', prUrl: 'https://github.com/user/repo/pull/42' });
  });

  it('returns null for non-PR commands', () => {
    assert.equal(detectPR('git push origin main', ''), null);
    assert.equal(detectPR('gh issue create --title "bug"', ''), null);
    assert.equal(detectPR('make test', 'ok'), null);
  });

  it('returns null for null/empty command', () => {
    assert.equal(detectPR(null, ''), null);
    assert.equal(detectPR('', ''), null);
    assert.equal(detectPR(undefined, ''), null);
  });

  it('handles gh pr create with extra flags', () => {
    const result = detectPR(
      'cd /tmp && gh pr create --title "x" --base main --draft',
      'https://github.com/a/b/pull/99'
    );
    assert.equal(result.action, 'created');
    assert.equal(result.prUrl, 'https://github.com/a/b/pull/99');
  });
});

describe('buildPRUpdate', () => {
  it('returns state "pr" and pinned_state "pr" for created action', () => {
    const update = buildPRUpdate({ action: 'created', prUrl: 'https://github.com/a/b/pull/1' });
    assert.equal(update.state, 'pr');
    assert.equal(update.pinned_state, 'pr');
    assert.equal(update.pr_url, 'https://github.com/a/b/pull/1');
  });

  it('returns state "merged" and pinned_state "merged" for merged action', () => {
    const update = buildPRUpdate({ action: 'merged', prUrl: 'https://github.com/a/b/pull/1' });
    assert.equal(update.state, 'merged');
    assert.equal(update.pinned_state, 'merged');
    assert.equal(update.pr_url, 'https://github.com/a/b/pull/1');
  });

  it('omits pr_url when null', () => {
    const update = buildPRUpdate({ action: 'created', prUrl: null });
    assert.equal(update.state, 'pr');
    assert.equal(update.pinned_state, 'pr');
    assert.equal(update.pr_url, undefined);
  });

  it('omits pinned_state for created when ghAuthed is true', () => {
    const update = buildPRUpdate(
      { action: 'created', prUrl: 'https://github.com/a/b/pull/1' },
      { ghAuthed: true },
    );
    assert.equal(update.state, 'pr');
    assert.equal(update.pinned_state, undefined);
    assert.equal(update.pr_url, 'https://github.com/a/b/pull/1');
  });

  it('still sets pinned_state for merged when ghAuthed is true', () => {
    const update = buildPRUpdate(
      { action: 'merged', prUrl: 'https://github.com/a/b/pull/1' },
      { ghAuthed: true },
    );
    assert.equal(update.state, 'merged');
    assert.equal(update.pinned_state, 'merged');
    assert.equal(update.pr_url, 'https://github.com/a/b/pull/1');
  });

  it('sets pinned_state for created when ghAuthed is false (default)', () => {
    const update = buildPRUpdate(
      { action: 'created', prUrl: 'https://github.com/a/b/pull/1' },
      { ghAuthed: false },
    );
    assert.equal(update.pinned_state, 'pr');
  });
});

describe('getCachedGhAuth', () => {
  function makeTmpCachePath() {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'gh-auth-cache-'));
    return path.join(dir, 'gh-auth.json');
  }

  it('returns cached value when within TTL without invoking isGhAuthed', () => {
    const cachePath = makeTmpCachePath();
    const now = 1_000_000_000;
    fs.writeFileSync(cachePath, JSON.stringify({ authed: true, checked_at: now - 1000 }));

    let called = 0;
    const authed = getCachedGhAuth({
      cachePath,
      now,
      ttlMs: 60_000,
      isGhAuthed: () => { called++; return false; },
    });

    assert.equal(authed, true);
    assert.equal(called, 0, 'must not call isGhAuthed when cache is fresh');
  });

  it('refreshes when cache is expired', () => {
    const cachePath = makeTmpCachePath();
    const now = 1_000_000_000;
    fs.writeFileSync(cachePath, JSON.stringify({ authed: true, checked_at: now - 100_000 }));

    const authed = getCachedGhAuth({
      cachePath,
      now,
      ttlMs: 60_000,
      isGhAuthed: () => false,
    });

    assert.equal(authed, false);
    const written = JSON.parse(fs.readFileSync(cachePath, 'utf8'));
    assert.equal(written.authed, false);
    assert.equal(written.checked_at, now);
  });

  it('refreshes and creates file when cache is missing', () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'gh-auth-cache-'));
    const cachePath = path.join(dir, 'nested', 'gh-auth.json');
    const now = 2_000_000_000;

    const authed = getCachedGhAuth({
      cachePath,
      now,
      ttlMs: 60_000,
      isGhAuthed: () => true,
    });

    assert.equal(authed, true);
    assert.ok(fs.existsSync(cachePath), 'should create cache file (and parent dir)');
    const written = JSON.parse(fs.readFileSync(cachePath, 'utf8'));
    assert.equal(written.authed, true);
    assert.equal(written.checked_at, now);
  });

  it('returns false and does not throw on parse error', () => {
    const cachePath = makeTmpCachePath();
    fs.writeFileSync(cachePath, 'not-json');

    const authed = getCachedGhAuth({
      cachePath,
      now: 1_000_000_000,
      ttlMs: 60_000,
      isGhAuthed: () => { throw new Error('boom'); },
    });

    assert.equal(authed, false);
  });
});

describe('PR_URL_RE', () => {
  it('matches standard GitHub PR URLs', () => {
    assert.ok(PR_URL_RE.test('https://github.com/user/repo/pull/42'));
    assert.ok(PR_URL_RE.test('https://github.com/org-name/my-repo/pull/1'));
  });

  it('does not match non-PR GitHub URLs', () => {
    assert.ok(!PR_URL_RE.test('https://github.com/user/repo/issues/42'));
    assert.ok(!PR_URL_RE.test('https://github.com/user/repo'));
  });
});
