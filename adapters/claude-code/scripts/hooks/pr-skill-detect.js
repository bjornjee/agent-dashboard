#!/usr/bin/env node
/**
 * UserPromptSubmit hook — detects /agent-dashboard:pr slash-command invocation
 * and pins agent state to "pr".
 *
 * Complements pr-detect.js (PostToolUse / Bash), which only fires for direct
 * `gh pr create` / `gh pr merge`. When the skill runs against a branch with an
 * already-open PR it never calls `gh pr create`, so the bash hook never sees
 * it. This hook closes that gap by transitioning state on slash-command
 * invocation, regardless of which gh subcommands the skill ends up running.
 *
 * Stdin: JSON from Claude Code hook system (UserPromptSubmit)
 * Env: CLAUDE_PLUGIN_ROOT, AGENT_DASHBOARD_DIR
 */

'use strict';

const path = require('path');

const CMD_RE = /<command-name>\/agent-dashboard:pr<\/command-name>/;

function detectPRSkill(prompt) {
  if (typeof prompt !== 'string' || prompt.length === 0) return false;
  return CMD_RE.test(prompt);
}

function buildPRSkillUpdate() {
  return { state: 'pr', pinned_state: 'pr' };
}

module.exports = { detectPRSkill, buildPRSkillUpdate };

if (require.main === module) {
  const pluginRoot = process.env.CLAUDE_PLUGIN_ROOT || path.resolve(__dirname, '..', '..');
  const { writeState } = require(path.join(pluginRoot, 'packages', 'agent-state'));

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

      if (!detectPRSkill(prompt)) return;

      const sessionId = input.session_id;
      if (!sessionId) return;

      writeState(sessionId, {
        ...buildPRSkillUpdate(),
        last_hook_event: input.hook_event_name || 'UserPromptSubmit',
      });
    } catch {
      // Silent — don't break Claude Code
    }
  });
}
