#!/usr/bin/env node
/**
 * Fast state sync hook for agent dashboard.
 *
 * Registered for PreToolUse, PostToolUse, and PermissionRequest.
 * Updates only: state, permission_mode, current_tool, last_hook_event, worktree_cwd, harness.
 * Skips: git branch, git diff, tmux capture, session_id lookup, model, preview.
 *
 * Uses per-agent files keyed by session_id — no locking needed.
 *
 * Stdin: JSON from Claude Code or codex hook system (1:1 payload schemas
 * per codex-rs/hooks/schema/generated/*.json).
 * Env: TMUX_PANE, CLAUDE_PLUGIN_ROOT (claude + codex), PLUGIN_ROOT (codex only).
 */

'use strict';

const path = require('path');
const { spawnSync } = require('child_process');

const pluginRoot = process.env.CLAUDE_PLUGIN_ROOT || process.env.PLUGIN_ROOT || __dirname;
const { readAgentState, writeState } = require(path.join(pluginRoot, 'packages', 'agent-state'));
const { getTarget, getPaneId } = require(path.join(pluginRoot, 'packages', 'tmux'));
const { readEffortConfig } = require('./effort-config');

// dispatchEffortKeys types `/effort <level>\r` into the agent's tmux pane.
// Claude Code processes /effort as a slash command (supportsNonInteractive
// flag set in the bundled command definition), so this is the documented
// way to mutate session-level effort mid-conversation. Fire-and-forget;
// any failure (tmux missing, pane gone, slash command unavailable) is
// silently ignored — the displayed effort may briefly drift from CC's
// actual effortValue, accepted limitation.
function dispatchEffortKeys(tmuxPane, level) {
  if (!tmuxPane || !level) return;
  try {
    spawnSync('tmux', ['send-keys', '-t', tmuxPane, `/effort ${level}`, 'Enter'], {
      timeout: 500,
      stdio: 'ignore',
    });
  } catch { /* fire-and-forget */ }
}

// States that PostToolUse must not overwrite. "plan" is included because once
// ExitPlanMode flips state to "plan" (plan ready for review), a late
// PostToolUse from a prior tool must not clobber it before the user approves.
const STOP_STATES = new Set(['idle_prompt', 'done', 'question', 'plan']);

/**
 * Detect which coding-agent harness invoked us. Codex CLI 0.130.0 sets
 * PLUGIN_ROOT in addition to CLAUDE_PLUGIN_ROOT (codex-rs/hooks/src/engine/
 * discovery.rs — the "OOTB compat" env block); Claude Code only sets the
 * latter. Hook stdin payload model (input.model) is a fallback discriminator
 * since codex emits gpt-* models and Claude emits claude-*.
 *
 * @param {object} input - parsed hook stdin
 * @returns {string} "codex" | "claude"
 */
function detectHarness(input) {
  if (process.env.PLUGIN_ROOT) return 'codex';
  const model = String((input && input.model) || '').toLowerCase();
  if (model.startsWith('gpt-')) return 'codex';
  return 'claude';
}

/**
 * Determine the agent state from the hook event.
 *
 * "plan" means *the plan is ready for the user's review* — set only on
 * PreToolUse for ExitPlanMode (or a delegated Plan subagent). While the
 * orchestrator is still researching/asking inside plan mode, state flows
 * from the active tool. permission_mode='plan' is captured as a field but
 * intentionally does NOT override state, so the dashboard badge fires only
 * at the moment the user needs to act.
 *
 * @param {string} hookEvent - hook_event_name from stdin
 * @param {string} toolName - tool_name from stdin
 * @param {string} _permissionMode - permission_mode from stdin (captured upstream; not used for state)
 * @param {object} [toolInput] - tool_input from stdin (used to detect delegated planning)
 * @returns {string} "plan", "permission", "question", or "running"
 */
function resolveState(hookEvent, toolName, _permissionMode, toolInput) {
  // Tool-specific signals are strictly more informative than the generic
  // PermissionRequest event, so they take priority. In permission_mode='plan'
  // (non-bypass), Claude Code fires PermissionRequest for ExitPlanMode — the
  // generic fallback would otherwise swallow it and misclassify as BLOCKED.
  // Covers both PermissionRequest (non-bypass) and PreToolUse (bypassPermissions).
  if ((hookEvent === 'PermissionRequest' || hookEvent === 'PreToolUse')
      && toolName === 'ExitPlanMode') {
    return 'plan';
  }
  if ((hookEvent === 'PermissionRequest' || hookEvent === 'PreToolUse')
      && toolName === 'AskUserQuestion') {
    return 'question';
  }
  if (hookEvent === 'PermissionRequest') {
    return 'permission';
  }
  // Orchestrator delegates planning to a Plan subagent. The subagent's
  // permission_mode is invisible to the parent's hooks, so detect via the
  // Agent tool call itself.
  if (hookEvent === 'PreToolUse' && toolName === 'Agent'
      && toolInput && toolInput.subagent_type === 'Plan') {
    return 'plan';
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

// effortTransition returns the new effort level if permission_mode is moving
// into or out of 'plan'. Returns null when no transition (or when dynamic
// switching is disabled via AGENT_DASHBOARD_DYNAMIC_EFFORT=0|off|false, or
// when CC omitted permission_mode from the payload — common on PostToolUse
// for some events; we don't act on absent data).
//
// Levels come from settings.toml's [effort] section (plan/default keys),
// defaulting to max/high when the file or keys are absent — same defaults
// the Go side applies in DefaultSettings(). Dispatch to CC happens in
// fastUpdate via tmux send-keys; this function only computes the value.
function effortTransition(existingMode, newMode) {
  const off = process.env.AGENT_DASHBOARD_DYNAMIC_EFFORT;
  if (off === '0' || off === 'off' || off === 'false') return null;
  if (!newMode) return null;
  if (existingMode !== 'plan' && newMode === 'plan') return readEffortConfig().plan;
  if (existingMode === 'plan' && newMode !== 'plan') return readEffortConfig().default;
  return null;
}

/**
 * Build the state update object from hook input and existing state.
 * Pure logic — no I/O. Returns { changed, update } where update is the
 * fields to merge, or null if nothing changed.
 *
 * @param {object} params
 * @param {object} params.input - parsed hook stdin (includes input.cwd from Claude Code)
 * @param {object} params.existing - current agent state from disk
 * @param {string} params.target - tmux target string
 * @param {string} params.tmuxPane - TMUX_PANE env value
 * @returns {{ changed: boolean, update: object|null }}
 */
function buildUpdate({ input, existing, target, tmuxPane }) {
  // Stamp worktree_cwd when the main agent observes its session running in a
  // user worktree path. Treated as static for the agent's lifetime — downstream
  // features (diff viewer, PR creation, cleanup) trust this dir and shouldn't
  // have it shifting as the agent cd's around.
  //
  // Only the MAIN agent can write worktree_cwd. A subagent's hook fires with
  // input.cwd under `.claude/worktrees/agent-<id>/` (Claude Code's per-subagent
  // isolation dir), which is not a user worktree — drop those observations so
  // they cannot poison the stamp. Among main-agent observations,
  // first-stamp-wins.
  const liveCwd = input.cwd || null;
  const isMainWorktree = liveCwd
    && /\/worktrees\//.test(liveCwd)
    && !/\/\.claude\/worktrees\//.test(liveCwd);
  const worktreeCwd = (!existing.worktree_cwd && isMainWorktree) ? liveCwd : null;
  const hookEvent = input.hook_event_name;
  const toolName = input.tool_name || '';
  const permissionMode = input.permission_mode || '';
  const toolInput = input.tool_input || {};

  let state = resolveState(hookEvent, toolName, permissionMode, toolInput);

  // Detect plan-mode transition for dynamic effort dispatch. Computed before
  // the PostToolUse stop-state guard because a leaving-plan transition
  // (state=plan + permission_mode flipping to non-plan) must surface even
  // when state itself is guarded.
  const newEffort = effortTransition(existing.permission_mode, permissionMode);

  // Gate the keystroke dispatch (not the state-file write). When the agent is
  // in a STOP_STATE the user is composing input in some CC UI (plan-review
  // textarea, AskUserQuestion answer, idle prompt), so `tmux send-keys
  // '/effort <level>\r'` would land inside their text. The state-file effort
  // still updates so the dashboard badge stays accurate; CC's session-level
  // effort just won't auto-sync — the user can run `/effort <level>` manually.
  const dispatchEffort = !!newEffort
    && newEffort !== (existing.effort || '')
    && !STOP_STATES.has(existing.state);

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
    const guardedUpdate = {};
    if (consumeBlocked) guardedUpdate.hook_blocked = '';
    if (newEffort) guardedUpdate.effort = newEffort;
    if (Object.keys(guardedUpdate).length > 0) {
      return { changed: true, update: guardedUpdate, dispatchEffort };
    }
    return { changed: false, update: null, dispatchEffort: false };
  }

  const currentTool = hookEvent === 'PostToolUse' ? '' : toolName;

  // Stamp delegated_plan_tool_use_id on PreToolUse Agent+Plan; clear it once
  // state transitions out of plan (next user turn / next non-Plan tool call).
  const stampDelegatedPlanId = state === 'plan'
    && hookEvent === 'PreToolUse'
    && toolName === 'Agent'
    && toolInput.subagent_type === 'Plan'
    && !!input.tool_use_id;
  const clearDelegatedPlanId = state !== 'plan' && !!existing.delegated_plan_tool_use_id;

  const harness = detectHarness(input);

  const changed = existing.state !== state
    || existing.current_tool !== currentTool
    || existing.permission_mode !== permissionMode
    || (worktreeCwd && existing.worktree_cwd !== worktreeCwd)
    || consumeBlocked
    || stampDelegatedPlanId
    || clearDelegatedPlanId
    || !!newEffort
    || (existing.harness || '') !== harness;

  if (!changed && existing.state) {
    return { changed: false, update: null, dispatchEffort: false };
  }

  const update = {
    target,
    tmux_pane_id: tmuxPane,
    session_id: input.session_id,
    state,
    current_tool: currentTool,
    permission_mode: permissionMode || existing.permission_mode || '',
    last_hook_event: hookEvent || '',
    harness,
  };

  if (worktreeCwd) {
    update.worktree_cwd = worktreeCwd;
  }

  if (stampDelegatedPlanId) {
    update.delegated_plan_tool_use_id = input.tool_use_id;
  } else if (clearDelegatedPlanId) {
    update.delegated_plan_tool_use_id = '';
  }

  // Clear hook_blocked after consuming it (one-shot signal from blocking hooks).
  if (consumeBlocked) {
    update.hook_blocked = '';
  }

  if (newEffort) {
    update.effort = newEffort;
  }

  return { changed: true, update, dispatchEffort };
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

  const { changed, update, dispatchEffort } = buildUpdate({ input, existing, target, tmuxPane });
  if (changed && update) {
    // Pass guardStates on PostToolUse to eliminate the TOCTOU race with the
    // async Stop hook: buildUpdate's guard reads stale state, but writeState
    // re-reads from disk — the guard must run against that fresh read. Skip
    // the guard for partial updates that don't touch `state` (e.g. effort or
    // hook_blocked deltas during a leaving-plan transition); those must land
    // even when state is in STOP_STATES.
    const opts = (input.hook_event_name === 'PostToolUse' && update.state)
      ? { guardStates: STOP_STATES }
      : {};
    writeState(sessionId, update, undefined, opts);

    // dispatchEffort is set by buildUpdate only when (a) effort actually
    // changes and (b) the agent is not in a state where the user is composing
    // input — see the gate in buildUpdate for why.
    if (dispatchEffort) {
      dispatchEffortKeys(tmuxPane, update.effort);
    }
  }
}

// Export for testing
module.exports = { resolveState, buildUpdate, detectHarness };
