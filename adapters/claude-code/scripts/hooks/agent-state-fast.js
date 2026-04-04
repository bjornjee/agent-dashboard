#!/usr/bin/env node
/**
 * Fast state sync hook for agent dashboard.
 *
 * Registered for PreToolUse, PostToolUse, and PermissionRequest.
 * Updates only: state, permission_mode, current_tool, last_hook_event, worktree_cwd.
 * Skips: git branch, git diff, tmux capture, session_id lookup, model, preview.
 *
 * Uses per-agent files keyed by session_id — no locking needed.
 *
 * Stdin: JSON from Claude Code hook system
 * Env: TMUX_PANE, CLAUDE_PLUGIN_ROOT
 */

'use strict';

const path = require('path');

const pluginRoot = process.env.CLAUDE_PLUGIN_ROOT || path.resolve(__dirname, '..', '..');
const { readAgentState, writeState } = require(path.join(pluginRoot, 'packages', 'agent-state'));
const { getTarget, getPaneId } = require(path.join(pluginRoot, 'packages', 'tmux'));
const { extractCwdFromCommand } = require(path.join(pluginRoot, 'packages', 'git-status'));

// States set by the Stop hook that PostToolUse must not overwrite.
const STOP_STATES = new Set(['idle_prompt', 'done', 'question']);
// States set by pr-detect or dashboard pinning.
const PR_STATES = new Set(['pr', 'merged']);

/**
 * Determine the agent state from the hook event.
 * @param {string} hookEvent - hook_event_name from stdin
 * @param {string} toolName - tool_name from stdin
 * @returns {string} "permission", "question", or "running"
 */
function resolveState(hookEvent, toolName) {
  if (hookEvent === 'PermissionRequest') {
    return 'permission';
  }
  // AskUserQuestion fires as PreToolUse, not PermissionRequest.
  // It always blocks for user input — the agent asked a question.
  if (hookEvent === 'PreToolUse' && toolName === 'AskUserQuestion') {
    return 'question';
  }
  return 'running';
}

// Only run stdin reader when executed directly (not when require()'d by tests)
if (require.main === module) {
  const MAX_STDIN = 1024 * 64; // 64KB
  let data = '';

  process.stdin.setEncoding('utf8');
  process.stdin.on('data', chunk => {
    if (data.length < MAX_STDIN) data += chunk.substring(0, MAX_STDIN - data.length);
  });

  process.stdin.on('end', () => {
    try {
      const input = data.trim() ? JSON.parse(data) : {};
      fastUpdate(input);
    } catch {
      // Silent — don't break Claude Code
    }
  });
}

/**
 * Build the state update object from hook input and existing state.
 * Pure logic — no I/O. Returns { changed, update } where update is the
 * fields to merge, or null if nothing changed.
 *
 * @param {object} params
 * @param {object} params.input - parsed hook stdin
 * @param {object} params.existing - current agent state from disk
 * @param {string} params.target - tmux target string
 * @param {string} params.tmuxPane - TMUX_PANE env value
 * @param {string|null} params.worktreeCwd - detected worktree path from Bash cd, or null
 * @returns {{ changed: boolean, update: object|null }}
 */
function buildUpdate({ input, existing, target, tmuxPane, worktreeCwd }) {
  const hookEvent = input.hook_event_name;
  const toolName = input.tool_name || '';
  const permissionMode = input.permission_mode || '';

  let state = resolveState(hookEvent, toolName);

  // Only consume hook_blocked on PreToolUse — the same event type that blocking
  // hooks fire on. Ignoring it on PostToolUse prevents a rapid PostToolUse from
  // a prior tool clearing the signal before the dashboard reads it.
  const consumeBlocked = existing.hook_blocked && hookEvent === 'PreToolUse';
  if (consumeBlocked && state === 'running') {
    state = 'permission';
  }

  // Stop-derived states must not be overwritten by a late PostToolUse.
  // PreToolUse (next turn) is allowed through to correctly resume "running".
  if (hookEvent === 'PostToolUse' && STOP_STATES.has(existing.state)) {
    if (consumeBlocked) {
      return { changed: true, update: { hook_blocked: '' } };
    }
    return { changed: false, update: null };
  }

  // Preserve PR states set by pr-detect or dashboard pinning.
  // Still clear hook_blocked even when returning early, to avoid a stuck signal.
  if ((PR_STATES.has(existing.state) || PR_STATES.has(existing.pinned_state)) && state === 'running') {
    if (consumeBlocked) {
      return { changed: true, update: { hook_blocked: '' } };
    }
    return { changed: false, update: null };
  }

  const currentTool = hookEvent === 'PostToolUse' ? '' : toolName;

  const changed = existing.state !== state
    || existing.current_tool !== currentTool
    || existing.permission_mode !== permissionMode
    || (worktreeCwd && existing.worktree_cwd !== worktreeCwd)
    || consumeBlocked;

  if (!changed && existing.state) {
    return { changed: false, update: null };
  }

  const update = {
    target,
    tmux_pane_id: tmuxPane,
    session_id: input.session_id,
    state,
    current_tool: currentTool,
    permission_mode: permissionMode || existing.permission_mode || '',
    last_hook_event: hookEvent || '',
  };

  if (worktreeCwd) {
    update.worktree_cwd = worktreeCwd;
  }

  // Clear hook_blocked after consuming it (one-shot signal from blocking hooks).
  if (consumeBlocked) {
    update.hook_blocked = '';
  }

  return { changed: true, update };
}

function fastUpdate(input) {
  const tmuxPane = getPaneId();
  if (!tmuxPane) return;

  const sessionId = input.session_id;
  if (!sessionId) return; // Can't write without a session_id key

  const target = getTarget(tmuxPane);
  if (!target) return;

  const existing = readAgentState(sessionId) || {};

  // Yield to pr-detect for gh pr create/merge commands — it owns those state transitions.
  if (input.hook_event_name === 'PostToolUse' && (input.tool_name || '') === 'Bash') {
    const cmd = (input.tool_input || {}).command || '';
    if (/\bgh\s+pr\s+(create|merge)\b/.test(cmd)) return;
  }

  // Detect worktree cd from Bash PostToolUse commands.
  // Pattern: cd /path/to/worktrees/<app>/<feature> && ...
  let worktreeCwd = null;
  if (input.hook_event_name === 'PostToolUse' && (input.tool_name || '') === 'Bash') {
    const detectedCwd = extractCwdFromCommand((input.tool_input || {}).command);
    if (detectedCwd && /\/worktrees\//.test(detectedCwd)) {
      worktreeCwd = detectedCwd;
    }
  }

  const { changed, update } = buildUpdate({ input, existing, target, tmuxPane, worktreeCwd });
  if (changed && update) {
    writeState(sessionId, update);
  }
}

// Export for testing
module.exports = { resolveState, buildUpdate };
