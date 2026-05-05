'use strict';

const KNOWN_TOOLS = new Set(['bash', 'read', 'write', 'edit', 'grep', 'find', 'ls']);

function mapToolName(piToolName) {
  if (!piToolName) return '';
  if (KNOWN_TOOLS.has(piToolName)) {
    return piToolName.charAt(0).toUpperCase() + piToolName.slice(1);
  }
  return piToolName;
}

function mapToolCall(event, { sessionId, cwd }) {
  return {
    hook_event_name: 'PreToolUse',
    session_id: sessionId,
    cwd,
    tool_name: mapToolName(event.toolName),
    tool_input: event.input,
    tool_call_id: event.toolCallId,
  };
}

function mapToolResult(event, { sessionId, cwd }) {
  return {
    hook_event_name: 'PostToolUse',
    session_id: sessionId,
    cwd,
    tool_name: mapToolName(event.toolName),
    tool_input: event.input,
    tool_call_id: event.toolCallId,
    tool_response_is_error: !!event.isError,
  };
}

function mapSessionStart(event, { sessionId, cwd, model }) {
  return {
    hook_event_name: 'SessionStart',
    session_id: sessionId,
    cwd,
    model: model || '',
    source: event.reason || 'startup',
  };
}

function lastAssistantText(messages) {
  if (!Array.isArray(messages) || messages.length === 0) return null;
  for (let i = messages.length - 1; i >= 0; i--) {
    const m = messages[i];
    if (!m || m.role !== 'assistant') continue;
    if (typeof m.content === 'string') return m.content;
    if (Array.isArray(m.content)) {
      const texts = m.content
        .filter(b => b && b.type === 'text' && typeof b.text === 'string')
        .map(b => b.text);
      return texts.length ? texts.join('\n') : null;
    }
    return null;
  }
  return null;
}

function mapAgentEnd(event, { sessionId, cwd }) {
  return {
    hook_event_name: 'Stop',
    session_id: sessionId,
    cwd,
    last_assistant_message: lastAssistantText(event.messages),
  };
}

module.exports = {
  mapToolName,
  mapToolCall,
  mapToolResult,
  mapSessionStart,
  mapAgentEnd,
  lastAssistantText,
};
