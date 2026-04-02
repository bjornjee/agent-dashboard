#!/usr/bin/env node
'use strict';

const { describe, it } = require('node:test');
const assert = require('node:assert/strict');
const { hasRmRF, DESTRUCTIVE_PATTERNS } = require('./warn-destructive');

// Helper: test a command against DESTRUCTIVE_PATTERNS
function isBlocked(command) {
  for (const { pattern, test } of DESTRUCTIVE_PATTERNS) {
    if (test ? test(command) : pattern.test(command)) return true;
  }
  return false;
}

describe('hasRmRF', () => {
  describe('should detect rm -rf variants', () => {
    it('rm -rf /', () => assert.equal(hasRmRF('rm -rf /'), true));
    it('rm -Rf dir', () => assert.equal(hasRmRF('rm -Rf dir'), true));
    it('rm -fr dir', () => assert.equal(hasRmRF('rm -fr dir'), true));
    it('rm -r -f file', () => assert.equal(hasRmRF('rm -r -f file'), true));
    it('rm -f -r file', () => assert.equal(hasRmRF('rm -f -r file'), true));
    it('rm -rvf dir', () => assert.equal(hasRmRF('rm -rvf dir'), true));
    it('rm -r -v -f dir', () => assert.equal(hasRmRF('rm -r -v -f dir'), true));
    it('piped: echo foo | rm -rf bar', () => assert.equal(hasRmRF('echo foo | rm -rf bar'), true));
    it('chained: ls && rm -rf dir', () => assert.equal(hasRmRF('ls && rm -rf dir'), true));
    it('semicolon: ls; rm -rf dir', () => assert.equal(hasRmRF('ls; rm -rf dir'), true));
    it('newline separated: ls\nrm -rf dir', () => assert.equal(hasRmRF('ls\nrm -rf dir'), true));
  });

  describe('should NOT flag safe rm commands', () => {
    it('rm file.txt', () => assert.equal(hasRmRF('rm file.txt'), false));
    it('rm -r dir (no force)', () => assert.equal(hasRmRF('rm -r dir'), false));
    it('rm -f file (no recursive)', () => assert.equal(hasRmRF('rm -f file'), false));
    it('rm file-reference (filename with -ref)', () => assert.equal(hasRmRF('rm file-reference'), false));
    it('rm .env-setup-failed', () => assert.equal(hasRmRF('rm .env-setup-failed'), false));
    it('rm -- -rf (-- ends flags)', () => assert.equal(hasRmRF('rm -- -rf'), false));
    it('rm /path/to/some-ref-file', () => assert.equal(hasRmRF('rm /path/to/some-ref-file'), false));
    it('rm -v file', () => assert.equal(hasRmRF('rm -v file'), false));
    it('no rm at all', () => assert.equal(hasRmRF('echo hello'), false));
    it('rm as substring: storm -rf', () => assert.equal(hasRmRF('storm -rf'), false));
    it('rm --recursive --force dir (long flags)', () => assert.equal(hasRmRF('rm --recursive --force dir'), false));
    it('rm --preserve-root dir', () => assert.equal(hasRmRF('rm --preserve-root dir'), false));
  });
});

describe('DESTRUCTIVE_PATTERNS integration', () => {
  describe('should block', () => {
    it('git reset --hard', () => assert.equal(isBlocked('git reset --hard'), true));
    it('git push --force', () => assert.equal(isBlocked('git push origin --force'), true));
    it('git push -f', () => assert.equal(isBlocked('git push -f'), true));
    it('git clean -f', () => assert.equal(isBlocked('git clean -f'), true));
    it('git checkout .', () => assert.equal(isBlocked('git checkout .'), true));
    it('git restore .', () => assert.equal(isBlocked('git restore .'), true));
    it('DROP TABLE users', () => assert.equal(isBlocked('DROP TABLE users'), true));
    it('drop database mydb', () => assert.equal(isBlocked('drop database mydb'), true));
    it('TRUNCATE TABLE logs', () => assert.equal(isBlocked('TRUNCATE TABLE logs'), true));
  });

  describe('should not block', () => {
    it('git status', () => assert.equal(isBlocked('git status'), false));
    it('git push origin main', () => assert.equal(isBlocked('git push origin main'), false));
    it('git checkout main', () => assert.equal(isBlocked('git checkout main'), false));
    it('SELECT * FROM table', () => assert.equal(isBlocked('SELECT * FROM table'), false));
  });
});
