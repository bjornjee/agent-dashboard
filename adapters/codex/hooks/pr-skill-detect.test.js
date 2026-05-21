#!/usr/bin/env node
'use strict';

const { describe, it } = require('node:test');
const assert = require('node:assert/strict');
const { detectPRSkill, buildPRSkillUpdate } = require('./pr-skill-detect');

describe('detectPRSkill', () => {
  it('matches codex skill invocation at prompt start', () => {
    assert.equal(detectPRSkill('$agent-dashboard:pr'), true);
    assert.equal(detectPRSkill('$agent-dashboard:pr open this branch'), true);
  });

  it('matches codex-injected skill metadata', () => {
    assert.equal(
      detectPRSkill('<skill>\n<name>agent-dashboard:pr</name>\n</skill>'),
      true
    );
  });

  it('does not match a different agent-dashboard skill', () => {
    assert.equal(detectPRSkill('$agent-dashboard:feature'), false);
    assert.equal(
      detectPRSkill('<skill>\n<name>agent-dashboard:feature</name>\n</skill>'),
      false
    );
  });

  it('does not match plain prose mentioning the skill name', () => {
    assert.equal(detectPRSkill('I will use $agent-dashboard:pr later'), false);
  });

  it('does not match a hypothetical pr-foo skill', () => {
    assert.equal(detectPRSkill('$agent-dashboard:pr-foo'), false);
    assert.equal(
      detectPRSkill('<skill>\n<name>agent-dashboard:pr-foo</name>\n</skill>'),
      false
    );
  });

  it('returns false for null / empty / non-string', () => {
    assert.equal(detectPRSkill(null), false);
    assert.equal(detectPRSkill(undefined), false);
    assert.equal(detectPRSkill(''), false);
    assert.equal(detectPRSkill(42), false);
  });
});

describe('buildPRSkillUpdate', () => {
  it('returns state and pinned_state of "pr"', () => {
    const update = buildPRSkillUpdate();
    assert.equal(update.state, 'pr');
    assert.equal(update.pinned_state, 'pr');
  });
});
