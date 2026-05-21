'use strict';

const QUESTION_PATTERNS = [
  /\?\s*$/m,
  /which\s+(one|should|do|would)/i,
  /should\s+i/i,
  /do\s+you\s+(want|prefer|need)/i,
  /would\s+you\s+(like|prefer)/i,
  /please\s+(choose|select|pick|confirm|decide)/i,
  /let\s+me\s+know/i,
  /waiting\s+for\s+(your|input|response)/i,
];

// Claude Code's interactive prompt character (U+276F). Present in idle prompts,
// plan approval menus, tool permission dialogs, and option selectors.
const PROMPT_CHAR = '\u276f';

/**
 * Detect whether an agent asked a question, is idle at prompt, or done.
 * Uses a layered heuristic: message content + tmux pane buffer.
 *
 * Called on Stop events only — permission/question states from
 * PermissionRequest and AskUserQuestion are handled by agent-state-fast.js.
 *
 * @param {string|null} lastMessage - The agent's last assistant message
 * @param {string[]} paneBuffer - Lines from tmux capture-pane
 * @returns {'question'|'idle_prompt'|'done'}
 */
function detectState(lastMessage, paneBuffer) {
  const hasQuestion = scoreMessage(lastMessage) > 0;
  const hasPrompt = scorePaneBuffer(paneBuffer) > 0;

  // Message looks like a question → agent asked something specific
  if (hasQuestion) return 'question';

  // Pane shows a prompt but no question → sitting at ❯, finished turn
  if (hasPrompt) return 'idle_prompt';

  // Neither signal → assume done (Stop hook fired, Claude finished)
  return 'done';
}

function scoreMessage(message) {
  if (!message || typeof message !== 'string') return 0;

  let score = 0;
  for (const pattern of QUESTION_PATTERNS) {
    if (pattern.test(message)) score++;
  }
  return score;
}

function scorePaneBuffer(lines) {
  if (!Array.isArray(lines) || lines.length === 0) return 0;

  // Check last 5 lines for Claude Code's interactive prompt character.
  // Covers: idle prompt (❯), plan approval (❯ 1. Yes...), tool permissions.
  const tail = lines.slice(-5);
  for (const line of tail) {
    if (line.includes(PROMPT_CHAR)) return 1;
  }

  // Check last 3 lines for bare shell prompts (string checks, no regex).
  const shellTail = lines.slice(-3);
  for (const line of shellTail) {
    const trimmed = line.trim();
    if (trimmed === '>' || trimmed === '$') return 1;
    if (trimmed.toLowerCase().startsWith('human:')) return 1;
  }

  return 0;
}

module.exports = { detectState, scoreMessage, scorePaneBuffer, QUESTION_PATTERNS, PROMPT_CHAR };
