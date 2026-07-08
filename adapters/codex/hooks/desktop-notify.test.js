#!/usr/bin/env node
'use strict';

const { describe, it } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');

const { buildBody, shouldAlert } = require('./desktop-notify');

describe('shouldAlert', () => {
  it('does not alert for Codex rate-limit stop failures', () => {
    assert.equal(shouldAlert({
      hook_event_name: 'StopFailure',
      error: 'rate_limit',
      cwd: '/tmp',
    }), false);
  });
});

describe('buildBody', () => {
  it('uses pending request_user_input text and branch for elicitation notifications', () => {
    const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'codex-notify-'));
    const agentsDir = path.join(tmp, 'agents');
    fs.mkdirSync(agentsDir, { recursive: true });
    fs.writeFileSync(path.join(agentsDir, 'sess-question.json'), JSON.stringify({
      target: 'main:0.1',
      session_id: 'sess-question',
      state: 'question',
      pending_question: {
        questions: [{ question: 'Which API shape should I use?' }],
      },
    }));

    try {
      assert.equal(
        buildBody({
          session_id: 'sess-question',
          hook_event_name: 'Notification',
          notification_type: 'elicitation_dialog',
          message: 'fallback',
        }, 'feat/api', agentsDir),
        'main:0.1/api: Which API shape should I use?',
      );
    } finally {
      fs.rmSync(tmp, { recursive: true, force: true });
    }
  });

  it('falls back to notification message when pending question is unavailable', () => {
    assert.equal(
      buildBody({
        session_id: 'missing',
        hook_event_name: 'Notification',
        notification_type: 'elicitation_dialog',
        message: 'fallback',
      }, 'feat/api', path.join(os.tmpdir(), 'missing-agent-dashboard-agents')),
      'fallback',
    );
  });

  it('compacts long branch names before the question', () => {
    const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'codex-notify-'));
    const agentsDir = path.join(tmp, 'agents');
    fs.mkdirSync(agentsDir, { recursive: true });
    fs.writeFileSync(path.join(agentsDir, 'sess-question.json'), JSON.stringify({
      target: 'main:0.1',
      session_id: 'sess-question',
      state: 'question',
      pending_question: {
        questions: [{ question: 'Which API shape should I use?' }],
      },
    }));

    try {
      assert.equal(
        buildBody({
          session_id: 'sess-question',
          hook_event_name: 'Notification',
          notification_type: 'elicitation_dialog',
          message: 'fallback',
        }, 'feat/notification-description-context', agentsDir),
        'main:0.1/notification...: Which API shape should I use?',
      );
    } finally {
      fs.rmSync(tmp, { recursive: true, force: true });
    }
  });
});
