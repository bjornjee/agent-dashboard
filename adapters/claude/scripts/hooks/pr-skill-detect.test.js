#!/usr/bin/env node
'use strict';

const { describe, it } = require('node:test');
const assert = require('node:assert/strict');
const { detectPRSkill, buildPRSkillUpdate } = require('./pr-skill-detect');

describe('detectPRSkill', () => {
  it('matches canonical CC slash-command tag', () => {
    const prompt = '<command-message>agent-dashboard:pr</command-message>\n' +
      '<command-name>/agent-dashboard:pr</command-name>\n' +
      '<command-args>create PR</command-args>';
    assert.equal(detectPRSkill(prompt), true);
  });

  it('matches with surrounding text', () => {
    assert.equal(
      detectPRSkill('please run <command-name>/agent-dashboard:pr</command-name> now'),
      true
    );
  });

  it('does not match a different agent-dashboard skill', () => {
    const prompt = '<command-name>/agent-dashboard:feature</command-name>';
    assert.equal(detectPRSkill(prompt), false);
  });

  it('does not match plain prose mentioning the skill name', () => {
    assert.equal(
      detectPRSkill('I will use /agent-dashboard:pr later'),
      false
    );
  });

  it('does not match a hypothetical pr-foo skill', () => {
    assert.equal(
      detectPRSkill('<command-name>/agent-dashboard:pr-foo</command-name>'),
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
