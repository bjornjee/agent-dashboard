'use strict';

const path = require('path');

const eventMapper = require('./event-mapper');
const gates = require('./gates');
const stateBridge = require('./state-bridge');
const tmux = require(path.resolve(__dirname, '..', '..', 'claude-code', 'packages', 'tmux'));

function defaultDeps() {
  return {
    writePiState: stateBridge.writePiState,
    runGates: gates.runGates,
    runGate: gates.runGate,
    getPaneId: tmux.getPaneId,
    getTarget: tmux.getTarget,
    preBashGates: gates.PRE_BASH_GATES,
    postBashGates: gates.POST_BASH_GATES,
    stopHooks: gates.STOP_HOOKS,
  };
}

function buildSession(ctx, deps) {
  const sessionId = ctx.sessionManager && typeof ctx.sessionManager.getSessionId === 'function'
    ? ctx.sessionManager.getSessionId()
    : '';
  const cwd = ctx.cwd || process.cwd();
  const model = (ctx.model && (ctx.model.id || ctx.model.name)) || '';
  const paneId = deps.getPaneId();
  const target = paneId ? deps.getTarget(paneId) : '';
  return { sessionId, cwd, model, paneId, target };
}

function onSessionStart(event, ctx, depsArg) {
  const deps = depsArg || defaultDeps();
  const s = buildSession(ctx, deps);
  if (!s.sessionId || !s.target) return;
  deps.writePiState(s.sessionId, {
    target: s.target,
    tmux_pane_id: s.paneId,
    cwd: s.cwd,
    state: 'running',
    model: s.model,
    started_at: new Date().toISOString(),
    last_hook_event: 'SessionStart',
  });
}

function onToolCall(event, ctx, depsArg) {
  const deps = depsArg || defaultDeps();
  const s = buildSession(ctx, deps);
  if (!s.sessionId || !s.target) return undefined;

  // Update state — show what's running
  deps.writePiState(s.sessionId, {
    target: s.target,
    state: 'running',
    current_tool: eventMapper.mapToolName(event.toolName),
    last_hook_event: 'PreToolUse',
  });

  if (event.toolName !== 'bash') return undefined;

  const payload = eventMapper.mapToolCall(event, { sessionId: s.sessionId, cwd: s.cwd });
  const result = deps.runGates(deps.preBashGates, payload);
  if (result && result.block) {
    deps.writePiState(s.sessionId, {
      target: s.target,
      state: 'error',
      hook_blocked: result.reason,
      last_hook_event: 'PreToolUse',
    });
    return result;
  }
  return undefined;
}

function onToolExecutionStart(event, ctx, depsArg) {
  const deps = depsArg || defaultDeps();
  const s = buildSession(ctx, deps);
  if (!s.sessionId || !s.target) return;
  deps.writePiState(s.sessionId, {
    target: s.target,
    state: 'running',
    current_tool: eventMapper.mapToolName(event.toolName),
  });
}

function onToolExecutionEnd(event, ctx, depsArg) {
  const deps = depsArg || defaultDeps();
  const s = buildSession(ctx, deps);
  if (!s.sessionId || !s.target) return;
  deps.writePiState(s.sessionId, {
    target: s.target,
    state: 'running',
    current_tool: '',
    last_hook_event: 'PostToolUse',
  });
}

function onToolResult(event, ctx, depsArg) {
  const deps = depsArg || defaultDeps();
  if (event.toolName !== 'bash') return;
  const s = buildSession(ctx, deps);
  if (!s.sessionId) return;
  const payload = eventMapper.mapToolResult(event, { sessionId: s.sessionId, cwd: s.cwd });
  // commit-lint and similar are observe-only; ignore returned result
  deps.runGates(deps.postBashGates, payload);
}

function onAgentEnd(event, ctx, depsArg) {
  const deps = depsArg || defaultDeps();
  const s = buildSession(ctx, deps);
  if (!s.sessionId || !s.target) return;

  const payload = eventMapper.mapAgentEnd(event, { sessionId: s.sessionId, cwd: s.cwd });

  deps.writePiState(s.sessionId, {
    target: s.target,
    state: 'idle_prompt',
    current_tool: '',
    last_message_preview: payload.last_assistant_message
      ? payload.last_assistant_message.split('\n').filter(Boolean).slice(-3).join(' ').substring(0, 200)
      : null,
    last_hook_event: 'Stop',
  });

  // Fire desktop notification (observe-only)
  for (const hook of deps.stopHooks) {
    deps.runGate(hook, payload);
  }
}

function onSessionShutdown(event, ctx, depsArg) {
  const deps = depsArg || defaultDeps();
  const s = buildSession(ctx, deps);
  if (!s.sessionId || !s.target) return;
  deps.writePiState(s.sessionId, {
    target: s.target,
    state: 'done',
    last_hook_event: 'SessionEnd',
  });
}

module.exports = {
  defaultDeps,
  buildSession,
  onSessionStart,
  onToolCall,
  onToolExecutionStart,
  onToolExecutionEnd,
  onToolResult,
  onAgentEnd,
  onSessionShutdown,
};
