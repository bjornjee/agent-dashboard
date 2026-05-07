#!/usr/bin/env node
/**
 * Agent state reporter hook.
 *
 * Writes agent state on lifecycle events (SessionStart, SubagentStart/Stop, Stop).
 * Uses per-agent files keyed by session_id — no locking needed.
 *
 * Stdin: JSON from Claude Code hook system
 * Env: TMUX_PANE, CLAUDE_PLUGIN_ROOT
 */

'use strict';

const path = require('path');
const fs = require('fs');
const os = require('os');

const pluginRoot = process.env.CLAUDE_PLUGIN_ROOT || path.resolve(__dirname, '..', '..');
const { readAgentState, writeState, detectState } = require(path.join(pluginRoot, 'packages', 'agent-state'));
const { hasPendingParentToolUse } = require(path.join(pluginRoot, 'packages', 'agent-state', 'pending-tools'));
const { getTarget, getPaneId, capture, parseTarget } = require(path.join(pluginRoot, 'packages', 'tmux'));
const { getChangedFiles } = require(path.join(pluginRoot, 'packages', 'git-status'));

function findSessionId() {
  const sessDir = path.join(os.homedir(), '.claude', 'sessions');
  // Walk up PID tree: hook → (possible sh) → claude
  let pid = process.ppid;
  for (let i = 0; i < 3 && pid > 1; i++) {
    try {
      const file = path.join(sessDir, `${pid}.json`);
      const data = JSON.parse(fs.readFileSync(file, 'utf8'));
      if (data.sessionId) return data.sessionId;
    } catch { /* not found, try parent */ }
    try {
      const { spawnSync } = require('child_process');
      const r = spawnSync('ps', ['-o', 'ppid=', '-p', String(pid)], { timeout: 1000 });
      pid = parseInt(r.stdout.toString().trim(), 10);
      if (isNaN(pid)) break;
    } catch { break; }
  }
  return null;
}

/**
 * Build the report entry from hook input and existing state.
 * Pure logic — no I/O. Extracted for testability.
 *
 * @param {object} params
 * @param {object} params.input - parsed hook stdin
 * @param {object} params.existing - current agent state from disk
 * @param {string} params.target - tmux target string
 * @param {string} params.tmuxPane - TMUX_PANE env value
 * @param {string} params.state - resolved agent state
 * @param {string[]} params.filesChanged - changed files list
 * @param {{session: string, window: number, pane: number}} params.parsed - parsed target
 * @param {string} params.cwd - working directory
 * @returns {{ changed: boolean, entry: object }}
 */
function buildReportEntry({ input, existing, target, tmuxPane, state, filesChanged, parsed, cwd }) {
  const hookEvent = input.hook_event_name;
  const lastMessage = input.last_assistant_message || null;

  const preview = lastMessage
    ? lastMessage.split('\n').filter(l => l.trim()).slice(-3).join(' ').substring(0, 200)
    : null;

  const model = (hookEvent === 'SessionStart' && input.model)
    ? input.model
    : (existing.model || '');

  const permissionMode = input.permission_mode || existing.permission_mode || '';

  let subagentCount = existing.subagent_count || 0;
  if (hookEvent === 'SubagentStart') {
    subagentCount++;
  } else if (hookEvent === 'SubagentStop') {
    subagentCount = Math.max(0, subagentCount - 1);
  }

  const entry = {
    target,
    tmux_pane_id: tmuxPane,
    session: parsed.session,
    window: parsed.window,
    pane: parsed.pane,
    state,
    cwd: existing.cwd || cwd || '',
    files_changed: filesChanged,
    last_message_preview: preview,
    session_id: input.session_id,
    started_at: existing.started_at || new Date().toISOString(),
    model,
    permission_mode: permissionMode,
    subagent_count: subagentCount,
    last_hook_event: hookEvent || '',
  };

  const changed = existing.state !== state
    || existing.subagent_count !== subagentCount
    || existing.last_message_preview !== preview
    || existing.permission_mode !== permissionMode
    || (existing.files_changed || []).join() !== filesChanged.join();

  return { changed: changed || !existing.state, entry };
}

/**
 * Resolve the state to write for Stop and SubagentStop events. Pure function.
 *
 * The previous implementation called detectState() unconditionally on every
 * SubagentStop where subagent_count reached 0. detectState is heuristic
 * (regex on last_assistant_message + tmux pane buffer for the ❯ glyph), so
 * it misclassifies the parent as idle/question while the parent is still
 * actively running between turns — producing visible state flicker and
 * spurious "Finished" notifications.
 *
 * The fix: gate the heuristic on a deterministic JSONL truth check. If the
 * parent transcript shows a tool_use without a matching tool_result, the
 * parent is definitively still working — preserve the existing state.
 *
 * @param {object} params
 * @param {string} params.hookEvent - 'Stop' or 'SubagentStop'
 * @param {object} params.existing - current on-disk agent state
 * @param {boolean} params.hasPendingTool - parent JSONL has an in-flight tool_use
 * @param {string|null} params.lastMessage - input.last_assistant_message
 * @param {string[]} params.paneBuffer - tmux capture-pane lines
 * @returns {string} resolved state
 */
function resolveStopState({ hookEvent, existing, hasPendingTool, lastMessage, paneBuffer }) {
  if (hookEvent === 'SubagentStop') {
    const state = existing.state || 'running';
    const subagentCount = Math.max(0, (existing.subagent_count || 0) - 1);
    if (state === 'running' && subagentCount <= 0 && !hasPendingTool) {
      return detectState(lastMessage, paneBuffer);
    }
    return state;
  }
  // Stop event — only run heuristic when JSONL says no tool is in flight.
  if (hasPendingTool) {
    return existing.state || 'running';
  }
  return detectState(lastMessage, paneBuffer);
}

// Only run stdin reader when executed directly (not when require()'d by tests)
if (require.main === module) {
  const MAX_STDIN = 1024 * 1024;
  let data = '';

  process.stdin.setEncoding('utf8');
  process.stdin.on('data', chunk => {
    if (data.length < MAX_STDIN) data += chunk.substring(0, MAX_STDIN - data.length);
  });

  process.stdin.on('end', () => {
    try {
      const input = data.trim() ? JSON.parse(data) : {};
      report(input);
    } catch {
      // Silent — don't break Claude Code
    }
  });
}

function report(input) {
  const tmuxPane = getPaneId();
  if (!tmuxPane) return;

  const target = getTarget(tmuxPane);
  if (!target) return;

  // Resolve session_id: from input, existing state, or PID-based lookup
  const sessionId = input.session_id
    || (readAgentState(input.session_id) || {}).session_id
    || findSessionId();
  if (!sessionId) return; // Can't write without a key

  const existing = readAgentState(sessionId) || {};

  // On Stop events, preserve merged state — it is terminal and must not be
  // overwritten by detectState(). PR state is allowed through so idle states
  // write to disk; the Go-side ApplyPinnedStates restores pr from idle states.
  // SubagentStart/Stop events are allowed through so subagent_count stays accurate.
  const hookEvent = input.hook_event_name;
  if (hookEvent === 'Stop' && existing.pinned_state === 'merged') return;

  const cwd = input.cwd || process.cwd();

  // Determine agent state based on hook event.
  // PreToolUse/PostToolUse/PermissionRequest are handled by agent-state-fast.js.
  const STOP_STATES = new Set(['idle_prompt', 'done', 'question', 'plan']);
  // hasPendingTool is hoisted out of the else branch so the writeState call
  // below can decide whether to pass guardStates.
  let hasPendingTool = false;
  let state;
  if (hookEvent === 'SessionStart' || hookEvent === 'SubagentStart') {
    state = 'running';
  } else {
    // SubagentStop / Stop: gate the heuristic detectState() on a deterministic
    // JSONL check. A pending parent tool_use means the agent is still working.
    hasPendingTool = hasPendingParentToolUse(input.transcript_path);
    const lastMessage = input.last_assistant_message || null;
    // Only capture the pane when detectState() will actually consume it.
    const subagentCount = Math.max(0, (existing.subagent_count || 0) - 1);
    const willDetect = !hasPendingTool && (
      hookEvent === 'Stop' ||
      (hookEvent === 'SubagentStop' && (existing.state || 'running') === 'running' && subagentCount <= 0)
    );
    const paneBuffer = willDetect ? capture(target, 15) : [];
    state = resolveStopState({
      hookEvent, existing, hasPendingTool, lastMessage, paneBuffer,
    });
  }

  const parsed = parseTarget(target);
  // Use worktree_cwd if available (agent may be working in a worktree)
  const effectiveCwd = existing.worktree_cwd || cwd;
  const filesChanged = getChangedFiles(effectiveCwd);

  const { changed, entry } = buildReportEntry({
    input, existing, target, tmuxPane, state, filesChanged, parsed, cwd,
  });

  if (changed) {
    const writeOpts = shouldGuardWrite(hookEvent, hasPendingTool)
      ? { guardStates: STOP_STATES }
      : {};
    writeState(sessionId, entry, undefined, writeOpts);
  }
}

// shouldGuardWrite decides whether writeState should pass guardStates so that
// a stale, concurrent write from this script cannot clobber a STOP_STATES
// value (question/plan/idle_prompt/done) just written by agent-state-fast.js.
//
// SubagentStop: always guard. The async Stop hook reads stale state; the
// guard runs against the fresh on-disk read inside writeState.
//
// Stop with hasPendingTool=true: guard. The parent JSONL has a tool_use
// without a matching tool_result — typically AskUserQuestion or ExitPlanMode
// blocking on user input. PreToolUse may have just written 'question' or
// 'plan'; this Stop's resolveStopState is preserving a pre-PreToolUse stale
// existing.state, so the write must not be allowed to overwrite STOP_STATES.
//
// Stop with hasPendingTool=false: do not guard. The assistant turn ended
// cleanly; detectState's idle_prompt/done result is authoritative and must
// be allowed to clear a lingering 'question' on disk after the user answers.
function shouldGuardWrite(hookEvent, hasPendingTool) {
  return hookEvent === 'SubagentStop'
    || (hookEvent === 'Stop' && hasPendingTool);
}

// Export for testing
module.exports = { buildReportEntry, resolveStopState, shouldGuardWrite };
