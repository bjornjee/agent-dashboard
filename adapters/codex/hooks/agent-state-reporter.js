#!/usr/bin/env node
/**
 * Agent state reporter hook.
 *
 * Writes agent state on lifecycle events (SessionStart, SubagentStart/Stop, Stop).
 * Uses per-agent files keyed by session_id — no locking needed.
 *
 * Stdin: JSON from Claude Code or codex hook system
 * Env: TMUX_PANE
 */

'use strict';

const path = require('path');
const fs = require('fs');
const os = require('os');

const hookRoot = __dirname;
const { readAgentState, writeState, detectState } = require(path.join(hookRoot, 'packages', 'agent-state'));
const { detectHarness } = require('./agent-state-fast');
const { hasPendingParentToolUse } = require(path.join(hookRoot, 'packages', 'agent-state', 'pending-tools'));
const { getTarget, getPaneId, capture, parseTarget } = require(path.join(hookRoot, 'packages', 'tmux'));
const { getChangedFiles } = require(path.join(hookRoot, 'packages', 'git-status'));

// readSettingsEffort returns the persisted effortLevel from
// ~/.claude/settings.json. Claude Code writes this when the user passes
// --effort <level> at startup or runs /effort <level> mid-session, so it is
// the most reliable cross-session source of the user's chosen effort. Used
// only on SessionStart as a fallback when CLAUDE_CODE_EFFORT_LEVEL is unset
// (i.e. when the agent was launched any way other than the dashboard's New
// Agent flow). Returns '' on any error so the caller can chain fallbacks.
function readSettingsEffort() {
  try {
    const settingsPath = path.join(os.homedir(), '.claude', 'settings.json');
    const data = JSON.parse(fs.readFileSync(settingsPath, 'utf8'));
    const level = data.effortLevel;
    return (typeof level === 'string' && level) ? level : '';
  } catch {
    return '';
  }
}

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
function buildReportEntry({ input, existing, target, tmuxPane, state, filesChanged, parsed, cwd, settingsEffort }) {
  const hookEvent = input.hook_event_name;
  const lastMessage = input.last_assistant_message || null;

  const preview = lastMessage
    ? lastMessage.split('\n').filter(l => l.trim()).slice(-3).join(' ').substring(0, 200)
    : null;

  const model = (hookEvent === 'SessionStart' && input.model)
    ? input.model
    : (existing.model || '');

  // Effort is not exposed by Claude Code in hook stdin or JSONL, so SessionStart
  // resolves it via three fallbacks: CLAUDE_CODE_EFFORT_LEVEL (seeded by the
  // dashboard's New Agent flow), then ~/.claude/settings.json's effortLevel
  // (passed in as settingsEffort — Claude Code persists --effort and /effort
  // there), then any prior effort already on disk. Subsequent events preserve
  // whatever the fast hook (or this reporter) wrote earlier.
  const effort = (hookEvent === 'SessionStart')
    ? (process.env.CLAUDE_CODE_EFFORT_LEVEL || settingsEffort || existing.effort || '')
    : (existing.effort || '');

  const permissionMode = input.permission_mode || existing.permission_mode || '';

  let subagentCount = existing.subagent_count || 0;
  if (hookEvent === 'SubagentStart') {
    subagentCount++;
  } else if (hookEvent === 'SubagentStop') {
    subagentCount = Math.max(0, subagentCount - 1);
  }

  // Stamp harness on every lifecycle event so codex agents are routed to
  // the codex conversation parser from SessionStart onward (rather than
  // waiting for the first PreToolUse from agent-state-fast.js — pure-chat
  // codex sessions would never stamp it that way and the conversation
  // panel would silently render nothing).
  const harness = detectHarness(input);

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
    harness,
  };
  if (effort) {
    entry.effort = effort;
  }

  const changed = existing.state !== state
    || existing.subagent_count !== subagentCount
    || existing.last_message_preview !== preview
    || existing.permission_mode !== permissionMode
    || (existing.files_changed || []).join() !== filesChanged.join()
    || (existing.harness || '') !== harness;

  return { changed: changed || !existing.state, entry };
}

/**
 * Resolve the state to write for Stop and SubagentStop events. Pure function.
 *
 * detectState is heuristic (regex on last_assistant_message + tmux pane buffer
 * for the ❯ glyph), so it can misclassify the parent as idle/question while
 * the parent is still actively running between turns. We gate it on two
 * deterministic checks:
 *   - hasPendingTool=true → parent JSONL has an in-flight tool_use; parent is
 *     definitively still working; preserve existing state.
 *   - subagentCount > 0  → other subagents are still running; the parent is
 *     orchestrating; preserve existing state.
 *
 * For SubagentStop with subagentCount==0 and !hasPendingTool, run detectState
 * the same way Stop does — codex CLI sometimes lands at last_hook_event=
 * SubagentStop without a matching Stop write, leaving the dashboard stuck on
 * state=running for an idle agent. Both deterministic checks must pass before
 * detectState is allowed to decide.
 *
 * @param {object} params
 * @param {string} params.hookEvent - 'Stop' or 'SubagentStop'
 * @param {object} params.existing - current on-disk agent state
 * @param {boolean} params.hasPendingTool - parent JSONL has an in-flight tool_use
 * @param {number} [params.subagentCount=0] - post-decrement subagent_count
 * @param {string|null} params.lastMessage - input.last_assistant_message
 * @param {string[]} params.paneBuffer - tmux capture-pane lines
 * @returns {string} resolved state
 */
function resolveStopState({ hookEvent, existing, hasPendingTool, subagentCount = 0, lastMessage, paneBuffer }) {
  if (hasPendingTool) {
    return existing.state || 'running';
  }
  if (hookEvent === 'SubagentStop' && subagentCount > 0) {
    return existing.state || 'running';
  }
  return detectState(lastMessage, paneBuffer);
}

function resolveChangedFiles({
  hookEvent, existing, effectiveCwd, refreshSubagent = false, getChangedFilesFn = getChangedFiles,
}) {
  if ((hookEvent === 'SubagentStart' || hookEvent === 'SubagentStop')
    && !refreshSubagent
    && Array.isArray(existing.files_changed)) {
    return existing.files_changed;
  }
  return getChangedFilesFn(effectiveCwd);
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
      // Silent — don't break the hook host
    }
    // See agent-state-fast.js — codex 0.130 requires JSON stdout.
    process.stdout.write('{}\n');
  });
}

function report(input) {
  // Stamp at hook ENTRY (≈ when Stop fired), before the transcript/git/tmux I/O
  // below, so the seq reflects event order not I/O duration. See agent-state-fast.js.
  const reportSeq = Date.now() * 1000;
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
  let finalSubagentStop = false;
  let state;
  if (hookEvent === 'SessionStart' || hookEvent === 'SubagentStart') {
    state = 'running';
  } else {
    // SubagentStop / Stop: gate the heuristic detectState() on deterministic
    // checks. A pending parent tool_use means the agent is still working;
    // remaining subagents (post-decrement count > 0) mean the parent is still
    // orchestrating.
    hasPendingTool = hasPendingParentToolUse(input.transcript_path);
    const lastMessage = input.last_assistant_message || null;
    const postDecrementCount = hookEvent === 'SubagentStop'
      ? Math.max(0, (existing.subagent_count || 0) - 1)
      : (existing.subagent_count || 0);
    finalSubagentStop = hookEvent === 'SubagentStop'
      && !hasPendingTool
      && postDecrementCount === 0;
    // Skip the tmux capture (most expensive call in this hook) unless
    // detectState will actually run.
    const willDetect = !hasPendingTool
      && (hookEvent === 'Stop' || postDecrementCount === 0);
    const paneBuffer = willDetect ? capture(target, 15) : [];
    state = resolveStopState({
      hookEvent, existing, hasPendingTool, subagentCount: postDecrementCount, lastMessage, paneBuffer,
    });
  }

  const parsed = parseTarget(target);
  // Use worktree_cwd if available (agent may be working in a worktree)
  const effectiveCwd = existing.worktree_cwd || cwd;
  const filesChanged = resolveChangedFiles({
    hookEvent, existing, effectiveCwd, refreshSubagent: finalSubagentStop,
  });

  // Read settings.json only when SessionStart needs the fallback — every other
  // event preserves existing.effort, so disk reads on every PostToolUse would
  // be wasted I/O on a hook with a 5s budget.
  const settingsEffort = (hookEvent === 'SessionStart' && !process.env.CLAUDE_CODE_EFFORT_LEVEL)
    ? readSettingsEffort()
    : '';

  const { changed, entry } = buildReportEntry({
    input, existing, target, tmuxPane, state, filesChanged, parsed, cwd, settingsEffort,
  });

  if (changed) {
    entry.report_seq = reportSeq;
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
module.exports = { buildReportEntry, resolveStopState, resolveChangedFiles, shouldGuardWrite };
