'use strict';

const path = require('path');
const handlers = require(path.resolve(__dirname, '..', 'lib', 'handlers'));

// Default export: pi extension entry. Receives the ExtensionAPI and registers
// lifecycle handlers that mirror the Claude Code adapter's per-agent state
// JSON contract at ~/.agent-dashboard/agents/<sid>.json.
module.exports = function (pi) {
  pi.on('session_start', (event, ctx) => handlers.onSessionStart(event, ctx));
  pi.on('tool_call', (event, ctx) => handlers.onToolCall(event, ctx));
  pi.on('tool_execution_start', (event, ctx) => handlers.onToolExecutionStart(event, ctx));
  pi.on('tool_execution_end', (event, ctx) => handlers.onToolExecutionEnd(event, ctx));
  pi.on('tool_result', (event, ctx) => handlers.onToolResult(event, ctx));
  pi.on('agent_end', (event, ctx) => handlers.onAgentEnd(event, ctx));
  pi.on('auto_retry_start', (event, ctx) => handlers.onAutoRetryStart(event, ctx));
  pi.on('session_shutdown', (event, ctx) => handlers.onSessionShutdown(event, ctx));
};

module.exports.handlers = handlers;
