#!/usr/bin/env node
'use strict';

const { describe, it } = require('node:test');
const assert = require('node:assert/strict');

const { shouldAlert } = require('./desktop-notify');

describe('shouldAlert', () => {
  it('does not alert for Codex rate-limit stop failures', () => {
    assert.equal(shouldAlert({
      hook_event_name: 'StopFailure',
      error: 'rate_limit',
      cwd: '/tmp',
    }), false);
  });
});
