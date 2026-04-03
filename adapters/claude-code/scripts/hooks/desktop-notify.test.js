#!/usr/bin/env node
'use strict';

const { describe, it } = require('node:test');
const assert = require('node:assert/strict');
const path = require('path');
const fs = require('fs');
const os = require('os');
const { stripMarkdown, extractSummary, escapeAppleScript, sanitizeShellArg, shouldAlert, lastTurnHasAlertingTool, readSettingsBool, loadNotificationSettings, ALERTING_NOTIFICATION_TYPES, ALERTING_ERRORS } = require('./desktop-notify');
const { extractSessionWindow } = require(path.resolve(__dirname, '..', '..', 'packages', 'tmux'));

describe('stripMarkdown', () => {
  it('removes headings', () => {
    assert.equal(stripMarkdown('## Summary'), 'Summary');
    assert.equal(stripMarkdown('# Title'), 'Title');
    assert.equal(stripMarkdown('###### Deep'), 'Deep');
  });

  it('removes bold and italic', () => {
    assert.equal(stripMarkdown('**bold** and *italic*'), 'bold and italic');
  });

  it('removes inline code', () => {
    assert.equal(stripMarkdown('run `npm test` now'), 'run npm test now');
  });

  it('removes code blocks', () => {
    const input = 'before\n' + '```js\ncode\n```' + '\nafter';
    const result = stripMarkdown(input);
    assert.ok(!result.includes('```'), 'should not contain code fences');
    assert.ok(result.includes('before'), 'should keep text before');
    assert.ok(result.includes('after'), 'should keep text after');
  });

  it('removes links but keeps text', () => {
    assert.equal(stripMarkdown('see [docs](https://example.com)'), 'see docs');
  });

  it('removes list markers', () => {
    assert.equal(stripMarkdown('- item one\n* item two'), 'item one\nitem two');
  });

  it('handles empty/whitespace input', () => {
    assert.equal(stripMarkdown('   '), '');
    assert.equal(stripMarkdown(''), '');
  });
});

describe('extractSummary', () => {
  it('returns "Done" for null/undefined/empty', () => {
    assert.equal(extractSummary(null), 'Done');
    assert.equal(extractSummary(undefined), 'Done');
    assert.equal(extractSummary(''), 'Done');
    assert.equal(extractSummary(123), 'Done');
  });

  it('returns first non-empty line', () => {
    assert.equal(extractSummary('\n\nHello world\nSecond line'), 'Hello world');
  });

  it('strips markdown before extracting', () => {
    assert.equal(extractSummary('## Fixed the **bug**'), 'Fixed the bug');
  });

  it('truncates at MAX_BODY (100 chars)', () => {
    const long = 'a'.repeat(150);
    const result = extractSummary(long);
    assert.equal(result.length, 103); // 100 + '...'
    assert.ok(result.endsWith('...'));
  });

  it('does not truncate at exactly 100 chars', () => {
    const exact = 'b'.repeat(100);
    assert.equal(extractSummary(exact), exact);
  });

  it('returns "Done" for whitespace-only lines', () => {
    assert.equal(extractSummary('   \n   \n   '), 'Done');
  });
});

describe('escapeAppleScript', () => {
  it('escapes backslashes', () => {
    assert.equal(escapeAppleScript('path\\to\\file'), 'path\\\\to\\\\file');
  });

  it('escapes double quotes', () => {
    assert.equal(escapeAppleScript('say "hello"'), 'say \\"hello\\"');
  });

  it('handles both together', () => {
    assert.equal(escapeAppleScript('a\\b"c'), 'a\\\\b\\"c');
  });
});

describe('sanitizeShellArg', () => {
  it('keeps safe characters', () => {
    assert.equal(sanitizeShellArg('main:0.1'), 'main:0.1');
    assert.equal(sanitizeShellArg('session_name'), 'session_name');
    assert.equal(sanitizeShellArg('user@host/path'), 'user@host/path');
  });

  it('strips unsafe characters', () => {
    assert.equal(sanitizeShellArg('foo;rm -rf /'), 'foorm-rf/');
    assert.equal(sanitizeShellArg("test'quote"), 'testquote');
    assert.equal(sanitizeShellArg('$(cmd)'), 'cmd');
  });
});

describe('shouldAlert', () => {
  it('returns true for alerting Notification types', () => {
    assert.equal(shouldAlert({ hook_event_name: 'Notification', notification_type: 'permission_prompt' }), true);
    assert.equal(shouldAlert({ hook_event_name: 'Notification', notification_type: 'idle_prompt' }), true);
    assert.equal(shouldAlert({ hook_event_name: 'Notification', notification_type: 'elicitation_dialog' }), true);
  });

  it('returns false for non-alerting Notification types', () => {
    assert.equal(shouldAlert({ hook_event_name: 'Notification', notification_type: 'progress' }), false);
    assert.equal(shouldAlert({ hook_event_name: 'Notification', notification_type: 'unknown' }), false);
  });

  it('returns true for rate_limit StopFailure', () => {
    assert.equal(shouldAlert({ hook_event_name: 'StopFailure', error: 'rate_limit' }), true);
  });

  it('returns false for non-alerting StopFailure errors', () => {
    assert.equal(shouldAlert({ hook_event_name: 'StopFailure', error: 'unknown_error' }), false);
  });

  it('returns false for unknown hook events', () => {
    assert.equal(shouldAlert({ hook_event_name: 'PreToolUse' }), false);
    assert.equal(shouldAlert({ hook_event_name: 'PostToolUse' }), false);
  });

  it('returns false for Stop without transcript_path', () => {
    assert.equal(shouldAlert({ hook_event_name: 'Stop' }), false);
    assert.equal(shouldAlert({ hook_event_name: 'Stop', transcript_path: null }), false);
  });
});

describe('readSettingsBool', () => {
  it('reads true value from TOML', () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'notify-test-'));
    fs.writeFileSync(path.join(dir, 'settings.toml'), '[notifications]\nenabled = true\n');
    const orig = process.env.AGENT_DASHBOARD_DIR;
    process.env.AGENT_DASHBOARD_DIR = dir;
    try {
      assert.equal(readSettingsBool('enabled', false), true);
    } finally {
      if (orig === undefined) delete process.env.AGENT_DASHBOARD_DIR;
      else process.env.AGENT_DASHBOARD_DIR = orig;
      fs.rmSync(dir, { recursive: true });
    }
  });

  it('reads false value from TOML', () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'notify-test-'));
    fs.writeFileSync(path.join(dir, 'settings.toml'), '[notifications]\nenabled = false\n');
    const orig = process.env.AGENT_DASHBOARD_DIR;
    process.env.AGENT_DASHBOARD_DIR = dir;
    try {
      assert.equal(readSettingsBool('enabled', true), false);
    } finally {
      if (orig === undefined) delete process.env.AGENT_DASHBOARD_DIR;
      else process.env.AGENT_DASHBOARD_DIR = orig;
      fs.rmSync(dir, { recursive: true });
    }
  });

  it('returns default when file is missing', () => {
    const orig = process.env.AGENT_DASHBOARD_DIR;
    process.env.AGENT_DASHBOARD_DIR = '/nonexistent/path';
    try {
      assert.equal(readSettingsBool('enabled', false), false);
      assert.equal(readSettingsBool('enabled', true), true);
    } finally {
      if (orig === undefined) delete process.env.AGENT_DASHBOARD_DIR;
      else process.env.AGENT_DASHBOARD_DIR = orig;
    }
  });

  it('returns default when key is absent', () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'notify-test-'));
    fs.writeFileSync(path.join(dir, 'settings.toml'), '[banner]\nshow_mascot = true\n');
    const orig = process.env.AGENT_DASHBOARD_DIR;
    process.env.AGENT_DASHBOARD_DIR = dir;
    try {
      assert.equal(readSettingsBool('enabled', false), false);
    } finally {
      if (orig === undefined) delete process.env.AGENT_DASHBOARD_DIR;
      else process.env.AGENT_DASHBOARD_DIR = orig;
      fs.rmSync(dir, { recursive: true });
    }
  });
});

describe('loadNotificationSettings', () => {
  it('returns all false when file is missing', () => {
    const orig = process.env.AGENT_DASHBOARD_DIR;
    process.env.AGENT_DASHBOARD_DIR = '/nonexistent/path';
    try {
      const s = loadNotificationSettings();
      assert.equal(s.enabled, false);
      assert.equal(s.sound, false);
      assert.equal(s.silentEvents, false);
    } finally {
      if (orig === undefined) delete process.env.AGENT_DASHBOARD_DIR;
      else process.env.AGENT_DASHBOARD_DIR = orig;
    }
  });

  it('reads all three settings', () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'notify-test-'));
    fs.writeFileSync(path.join(dir, 'settings.toml'),
      '[notifications]\nenabled = true\nsound = true\nsilent_events = true\n');
    const orig = process.env.AGENT_DASHBOARD_DIR;
    process.env.AGENT_DASHBOARD_DIR = dir;
    try {
      const s = loadNotificationSettings();
      assert.equal(s.enabled, true);
      assert.equal(s.sound, true);
      assert.equal(s.silentEvents, true);
    } finally {
      if (orig === undefined) delete process.env.AGENT_DASHBOARD_DIR;
      else process.env.AGENT_DASHBOARD_DIR = orig;
      fs.rmSync(dir, { recursive: true });
    }
  });
});

describe('extractSessionWindow', () => {
  it('extracts session:window from simple target', () => {
    assert.equal(extractSessionWindow('main:0.1'), 'main:0');
  });

  it('handles session names with dots', () => {
    assert.equal(extractSessionWindow('my.project:0.1'), 'my.project:0');
  });

  it('handles IP-like session names', () => {
    assert.equal(extractSessionWindow('127.0.0.1:0.1'), '127.0.0.1:0');
  });

  it('handles multi-digit window and pane indices', () => {
    assert.equal(extractSessionWindow('dev:12.3'), 'dev:12');
  });

  it('returns input unchanged when no dot present', () => {
    assert.equal(extractSessionWindow('main:0'), 'main:0');
  });
});
