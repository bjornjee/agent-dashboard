#!/usr/bin/env node
/**
 * PostToolUse hook for Bash — detects PR creation and merge via `gh` CLI.
 *
 * On `gh pr create`: extracts the PR URL from tool output, sets state to "pr".
 * On `gh pr merge`:  sets state to "merged".
 *
 * Writes pr_url and state into the agent's state file.
 *
 * Stdin: JSON from Claude Code hook system (PostToolUse)
 * Env: TMUX_PANE, CLAUDE_PLUGIN_ROOT
 */

'use strict';

const path = require('path');

const pluginRoot = process.env.CLAUDE_PLUGIN_ROOT || path.resolve(__dirname, '..', '..');
const { readAgentState, writeState } = require(path.join(pluginRoot, 'packages', 'agent-state'));

// GitHub PR URL pattern: https://github.com/<owner>/<repo>/pull/<number>
const PR_URL_RE = /https:\/\/github\.com\/[^/]+\/[^/]+\/pull\/\d+/;

/**
 * Detect PR action from a Bash command and its output.
 * Returns { action, prUrl } or null if no PR activity detected.
 *
 * @param {string} command - the Bash command that was executed
 * @param {string} output - the tool result / stdout
 * @returns {{ action: 'created'|'merged', prUrl: string|null } | null}
 */
function detectPR(command, output) {
  if (!command || typeof command !== 'string') return null;

  // gh pr create → look for PR URL in output
  if (/\bgh\s+pr\s+create\b/.test(command)) {
    const match = (output || '').match(PR_URL_RE);
    return { action: 'created', prUrl: match ? match[0] : null };
  }

  // gh pr merge → extract PR URL from command args or output
  if (/\bgh\s+pr\s+merge\b/.test(command)) {
    const match = (command + ' ' + (output || '')).match(PR_URL_RE);
    return { action: 'merged', prUrl: match ? match[0] : null };
  }

  return null;
}

/**
 * Build the state update from a detected PR action.
 * @param {{ action: string, prUrl: string|null }} detection
 * @returns {object} fields to merge into agent state
 */
function buildPRUpdate(detection) {
  const update = {};
  if (detection.action === 'created') {
    update.state = 'pr';
  } else if (detection.action === 'merged') {
    update.state = 'merged';
  }
  if (detection.prUrl) {
    update.pr_url = detection.prUrl;
  }
  return update;
}

// Only run stdin reader when executed directly (not when require()'d by tests)
if (require.main === module) {
  const MAX_STDIN = 1024 * 64;
  let data = '';

  process.stdin.setEncoding('utf8');
  process.stdin.on('data', chunk => {
    if (data.length < MAX_STDIN) data += chunk.substring(0, MAX_STDIN - data.length);
  });

  process.stdin.on('end', () => {
    try {
      const input = data.trim() ? JSON.parse(data) : {};
      const command = (input.tool_input && input.tool_input.command) || '';
      const output = input.tool_result || '';

      const detection = detectPR(command, output);
      if (!detection) return;

      const sessionId = input.session_id;
      if (!sessionId) return;

      const existing = readAgentState(sessionId) || {};
      const update = buildPRUpdate(detection);

      // Preserve existing fields, only merge PR-related updates
      writeState(sessionId, {
        ...update,
        last_hook_event: input.hook_event_name || existing.last_hook_event || '',
      });
    } catch {
      // Silent — don't break Claude Code
    }
  });
}

// Export for testing
module.exports = { detectPR, buildPRUpdate, PR_URL_RE };
