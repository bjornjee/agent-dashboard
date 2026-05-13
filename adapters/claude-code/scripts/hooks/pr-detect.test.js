#!/usr/bin/env node
'use strict';

const { describe, it } = require('node:test');
const assert = require('node:assert/strict');
const { detectPR, buildPRUpdate, PR_URL_RE } = require('./pr-detect');

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

  it('does not match gh pr merge inside a grep argument', () => {
    assert.equal(detectPR('grep "gh pr merge" .', 'src/file.go:42'), null);
  });

  it('does not match gh pr merge inside an rg argument', () => {
    assert.equal(detectPR('rg "gh pr merge"', 'src/file.go:42'), null);
  });

  it('does not match gh pr merge inside an echo argument', () => {
    assert.equal(detectPR('echo "todo: gh pr merge later"', 'todo: gh pr merge later'), null);
  });

  it('does not match gh pr merge inside a piped grep', () => {
    assert.equal(detectPR('cat file.md | grep "gh pr merge"', 'line containing gh pr merge'), null);
  });

  it('does not match gh pr create inside a grep argument', () => {
    assert.equal(detectPR('grep "gh pr create" .', 'src/file.go:42'), null);
  });

  it('detects gh pr merge after && segment', () => {
    const result = detectPR(
      'cd /tmp && gh pr merge 42 --squash',
      'Merged pull request #42\n'
    );
    assert.equal(result.action, 'merged');
  });

  it('skips gh pr merge --help', () => {
    assert.equal(
      detectPR('gh pr merge --help', 'Merge a pull request on GitHub\n\nUSAGE\n  gh pr merge ...'),
      null
    );
  });

  it('skips gh pr merge -h', () => {
    assert.equal(detectPR('gh pr merge -h', 'help text'), null);
  });

  it('treats gh pr merge --auto as created (PR queued, not merged)', () => {
    const result = detectPR(
      'gh pr merge 42 --auto --squash',
      '✓ Pull request #42 will be automatically merged via squash when all requirements are met\n'
    );
    assert.deepEqual(result, { action: 'created', prUrl: null });
  });

  it('treats gh pr merge --auto with URL in command as created with prUrl', () => {
    const result = detectPR(
      'gh pr merge https://github.com/u/r/pull/42 --auto',
      ''
    );
    assert.deepEqual(result, { action: 'created', prUrl: 'https://github.com/u/r/pull/42' });
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

  it('always pins created regardless of extra args', () => {
    const update = buildPRUpdate(
      { action: 'created', prUrl: 'https://github.com/a/b/pull/1' },
    );
    assert.equal(update.state, 'pr');
    assert.equal(update.pinned_state, 'pr');
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
