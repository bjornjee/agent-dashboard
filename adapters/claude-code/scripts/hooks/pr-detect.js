#!/usr/bin/env node
/**
 * PostToolUse hook for Bash — detects PR creation and merge via `gh` CLI.
 *
 * On `gh pr create`: extracts the PR URL from tool output, sets state to "pr".
 *   Pins the state ("sticky pr") only when `gh auth status` is NOT available.
 *   When gh is authenticated (cached per ~/.agent-dashboard/gh-auth.json, 24h
 *   TTL), the state is left unpinned so follow-up agent activity transitions
 *   naturally while the user iterates on rough edges.
 * On `gh pr merge`: sets state to "merged" and pins it (terminal state).
 *
 * Writes pr_url and state into the agent's state file.
 *
 * Stdin: JSON from Claude Code hook system (PostToolUse)
 * Env: TMUX_PANE, CLAUDE_PLUGIN_ROOT, AGENT_DASHBOARD_DIR
 */

'use strict';

const fs = require('fs');
const os = require('os');
const path = require('path');
const { execFileSync } = require('child_process');

const pluginRoot = process.env.CLAUDE_PLUGIN_ROOT || path.resolve(__dirname, '..', '..');
const { readAgentState, writeState } = require(path.join(pluginRoot, 'packages', 'agent-state'));

// Default cache path for gh auth status, keyed off the same base as agent state.
const DEFAULT_GH_AUTH_CACHE = path.join(
  process.env.AGENT_DASHBOARD_DIR || path.join(process.env.HOME || process.env.USERPROFILE || os.tmpdir(), '.agent-dashboard'),
  'gh-auth.json',
);
const DEFAULT_GH_AUTH_TTL_MS = 24 * 60 * 60 * 1000; // 24h

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
 *
 * When `ghAuthed` is true we skip `pinned_state` on PR creation so subsequent
 * agent activity (running, idle_prompt, ...) transitions naturally while the
 * user iterates on rough edges post-PR. Merge still pins — merged is terminal
 * and the sticky visual cue is useful for worktree cleanup.
 *
 * @param {{ action: string, prUrl: string|null }} detection
 * @param {{ ghAuthed?: boolean }} [opts]
 * @returns {object} fields to merge into agent state
 */
function buildPRUpdate(detection, { ghAuthed = false } = {}) {
  const update = {};
  if (detection.action === 'created') {
    update.state = 'pr';
    if (!ghAuthed) {
      update.pinned_state = 'pr';
    }
  } else if (detection.action === 'merged') {
    update.state = 'merged';
    update.pinned_state = 'merged';
  }
  if (detection.prUrl) {
    update.pr_url = detection.prUrl;
  }
  return update;
}

/**
 * Check whether `gh auth status` succeeds. Synchronous, short timeout, and
 * never throws. Returns false if gh is missing, not authed, or slow.
 * @returns {boolean}
 */
function isGhAuthed() {
  try {
    execFileSync('gh', ['auth', 'status'], { stdio: 'ignore', timeout: 1500 });
    return true;
  } catch {
    return false;
  }
}

/**
 * Read or refresh the cached gh-auth result. Reads a small JSON file at
 * `cachePath` containing `{ authed, checked_at }` (ms epoch). If missing or
 * older than `ttlMs`, invokes `isGhAuthed` and rewrites the cache.
 *
 * Any I/O or parse error falls back to calling `isGhAuthed` directly, and a
 * thrown `isGhAuthed` yields `false`. This function must never throw.
 *
 * @param {object} [opts]
 * @param {string} [opts.cachePath]
 * @param {number} [opts.now]
 * @param {number} [opts.ttlMs]
 * @param {() => boolean} [opts.isGhAuthed] - injected for tests
 * @returns {boolean}
 */
function getCachedGhAuth({
  cachePath = DEFAULT_GH_AUTH_CACHE,
  now = Date.now(),
  ttlMs = DEFAULT_GH_AUTH_TTL_MS,
  isGhAuthed: check = isGhAuthed,
} = {}) {
  try {
    const raw = fs.readFileSync(cachePath, 'utf8');
    const entry = JSON.parse(raw);
    if (
      entry
      && typeof entry.checked_at === 'number'
      && typeof entry.authed === 'boolean'
      && now - entry.checked_at < ttlMs
    ) {
      return entry.authed;
    }
  } catch {
    // Missing or corrupt cache — fall through to refresh.
  }

  let authed = false;
  try {
    authed = !!check();
  } catch {
    authed = false;
  }

  try {
    fs.mkdirSync(path.dirname(cachePath), { recursive: true });
    fs.writeFileSync(cachePath, JSON.stringify({ authed, checked_at: now }));
  } catch {
    // Cache write failure is non-fatal.
  }

  return authed;
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
      const ghAuthed = getCachedGhAuth();
      const update = buildPRUpdate(detection, { ghAuthed });

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
module.exports = { detectPR, buildPRUpdate, PR_URL_RE, isGhAuthed, getCachedGhAuth };
