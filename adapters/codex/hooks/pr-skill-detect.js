#!/usr/bin/env node
'use strict';

const path = require('path');

const START_CMD_RE = /^\s*\$agent-dashboard:pr(?:\s|$)/;
const SKILL_NAME_RE = /<name>agent-dashboard:pr<\/name>/;

function detectPRSkill(prompt) {
  if (typeof prompt !== 'string' || prompt.length === 0) return false;
  return START_CMD_RE.test(prompt) || SKILL_NAME_RE.test(prompt);
}

function buildPRSkillUpdate() {
  return { state: 'pr', pinned_state: 'pr' };
}

if (require.main === module) {
  const hookRoot = __dirname;
  const { writeState } = require(path.join(hookRoot, 'packages', 'agent-state'));

  const MAX_STDIN = 1024 * 64;
  let data = '';

  process.stdin.setEncoding('utf8');
  process.stdin.on('data', chunk => {
    if (data.length < MAX_STDIN) data += chunk.substring(0, MAX_STDIN - data.length);
  });

  process.stdin.on('end', () => {
    try {
      const input = data.trim() ? JSON.parse(data) : {};
      const prompt = input.prompt || '';

      if (detectPRSkill(prompt) && input.session_id) {
        writeState(input.session_id, {
          ...buildPRSkillUpdate(),
          last_hook_event: input.hook_event_name || 'UserPromptSubmit',
          report_seq: Date.now() * 1000,
        });
      }
    } catch {
      // Silent — don't break the hook host.
    }
    process.stdout.write('{}\n');
  });
}

module.exports = { detectPRSkill, buildPRSkillUpdate };
