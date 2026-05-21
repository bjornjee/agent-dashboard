#!/usr/bin/env node
/**
 * Desktop notification hook for Stop, Notification, and StopFailure events.
 *
 * Plays the "Blow" sound when Claude needs user attention:
 *   - Notification: permission_prompt, idle_prompt, elicitation_dialog
 *   - Stop: last assistant turn used AskUserQuestion or ExitPlanMode
 *   - StopFailure: rate_limit error
 *
 * All other stops show a silent notification.
 *
 * When running inside tmux, clicking the notification switches to the
 * correct window and pane.
 *
 * Fallback: osascript if terminal-notifier is not installed.
 *
 * Dependencies (optional): terminal-notifier (brew install terminal-notifier)
 */

'use strict';

const { spawnSync } = require('child_process');
const { readFileSync } = require('fs');
const { basename, resolve } = require('path');

const pluginRoot = process.env.CLAUDE_PLUGIN_ROOT || resolve(__dirname, '..', '..');
const tmuxPkg = require(resolve(pluginRoot, 'packages', 'tmux'));

const TITLE = 'Claude Code';
const SOUND = 'Blow';
const MAX_BODY = 100;

// Notification types that require user attention.
const ALERTING_NOTIFICATION_TYPES = new Set([
  'permission_prompt',
  'idle_prompt',
  'elicitation_dialog',
]);

// Tool names in the last assistant turn that mean "waiting for user".
const ALERTING_TOOL_NAMES = new Set([
  'AskUserQuestion',
  'ExitPlanMode',
]);

// StopFailure error types that warrant a sound.
const ALERTING_ERRORS = new Set([
  'rate_limit',
]);

/**
 * Read the last assistant message from the transcript JSONL and check
 * whether it contains a tool_use block matching ALERTING_TOOL_NAMES.
 */
function lastTurnHasAlertingTool(transcriptPath) {
  return findAlertingToolName(transcriptPath) !== undefined;
}

/**
 * Determine whether to play the alert sound based on the hook event.
 */
function shouldAlert(input) {
  const event = input.hook_event_name;

  // Tier 1: Notification hook — permission prompt, idle, elicitation
  if (event === 'Notification') {
    return ALERTING_NOTIFICATION_TYPES.has(input.notification_type);
  }

  // Tier 1: StopFailure — rate limit
  if (event === 'StopFailure') {
    return ALERTING_ERRORS.has(input.error);
  }

  // Tier 2: Stop — check transcript for AskUserQuestion / ExitPlanMode
  if (event === 'Stop') {
    return lastTurnHasAlertingTool(input.transcript_path);
  }

  return false;
}

function stripMarkdown(text) {
  return text
    .replace(/#{1,6}\s+/g, '')       // headings
    .replace(/\*\*([^*]+)\*\*/g, '$1') // bold
    .replace(/\*([^*]+)\*/g, '$1')     // italic
    .replace(/`([^`]+)`/g, '$1')       // inline code
    .replace(/```[\s\S]*?```/g, '')    // code blocks
    .replace(/\[([^\]]+)\]\([^)]+\)/g, '$1') // links
    .replace(/^[-*]\s+/gm, '')        // list markers
    .trim();
}

function extractSummary(message) {
  if (!message || typeof message !== 'string') return 'Done';

  const cleaned = stripMarkdown(message);
  const line = cleaned
    .split('\n')
    .map(l => l.trim())
    .find(l => l.length > 0);

  if (!line) return 'Done';
  return line.length > MAX_BODY ? `${line.slice(0, MAX_BODY)}...` : line;
}

/**
 * Build the notification body text based on the hook event.
 */
function buildBody(input) {
  const event = input.hook_event_name;

  if (event === 'Notification') {
    return input.message || input.title || 'Notification';
  }

  if (event === 'StopFailure') {
    return input.error_details || input.error || 'Error';
  }

  return extractSummary(input.last_assistant_message);
}

function getSubtitle(cwd, input) {
  const parts = [];

  if (cwd) {
    parts.push(basename(cwd));
  }

  const branch = spawnSync('git', ['branch', '--show-current'], {
    encoding: 'utf8',
    timeout: 2000,
    cwd: cwd || undefined,
    stdio: ['ignore', 'pipe', 'ignore'],
  });
  if (branch.status === 0 && branch.stdout.trim()) {
    parts.push(branch.stdout.trim());
  }

  if (input) {
    const state = getAgentState(input);
    if (state) parts.push(state);
  }

  return parts.join(' | ') || undefined;
}

function hasCommand(cmd) {
  const result = spawnSync('which', [cmd], { stdio: 'ignore', timeout: 2000 });
  return result.status === 0;
}

const TERMINAL_BUNDLE_IDS = {
  ghostty: 'com.mitchellh.ghostty',
  'iTerm.app': 'com.googlecode.iterm2',
  Apple_Terminal: 'com.apple.Terminal',
  WezTerm: 'com.github.wez.wezterm',
};

function getTerminalBundleId(termProgram) {
  return TERMINAL_BUNDLE_IDS[termProgram];
}

const NOTIFICATION_STATE_MAP = {
  permission_prompt: 'needs permission',
  idle_prompt: 'idle',
  elicitation_dialog: 'needs input',
};

/**
 * Find the name of the alerting tool in the last assistant turn, if any.
 * Returns the tool name string or undefined.
 */
function findAlertingToolName(transcriptPath) {
  if (!transcriptPath) return undefined;
  try {
    const raw = readFileSync(transcriptPath, 'utf8');
    const lines = raw.trimEnd().split('\n');

    for (let i = lines.length - 1; i >= 0; i--) {
      const entry = JSON.parse(lines[i]);
      if (entry.type !== 'assistant' || !entry.message) continue;

      const content = entry.message.content;
      if (!Array.isArray(content)) return undefined;

      const tool = content.find(
        block => block.type === 'tool_use' && ALERTING_TOOL_NAMES.has(block.name)
      );
      return tool ? tool.name : undefined;
    }
  } catch {
    // Transcript unreadable — fall through silently.
  }
  return undefined;
}

function getAgentState(input) {
  const event = input.hook_event_name;

  if (event === 'Notification') {
    return NOTIFICATION_STATE_MAP[input.notification_type] || 'notification';
  }

  if (event === 'StopFailure') {
    return input.error === 'rate_limit' ? 'rate limited' : 'error';
  }

  if (event === 'Stop') {
    const toolName = findAlertingToolName(input.transcript_path);
    if (toolName === 'AskUserQuestion') return 'asked a question';
    if (toolName === 'ExitPlanMode') return 'plan ready';
    return 'done';
  }

  return undefined;
}

function sanitizeShellArg(str) {
  return str.replace(/[^a-zA-Z0-9_.:@/-]/g, '');
}

function getTmuxAction() {
  const tmuxPane = process.env.TMUX_PANE;
  if (!tmuxPane) return undefined;

  const target = spawnSync('tmux', [
    'display-message', '-t', tmuxPane, '-p',
    '#{session_name}:#{window_index}.#{pane_index}',
  ], { encoding: 'utf8', timeout: 2000 });

  if (!target.stdout) return undefined;

  const t = sanitizeShellArg(target.stdout.trim());
  const sessionWindow = tmuxPkg.extractSessionWindow(t);
  return `tmux select-window -t '${sessionWindow}' && tmux select-pane -t '${t}'`;
}

function notifyWithTerminalNotifier(title, subtitle, body, sound) {
  const args = ['-title', title, '-message', body, '-group', `claude-${process.pid}`];

  if (subtitle) args.push('-subtitle', subtitle);
  if (sound) args.push('-sound', sound);

  const action = getTmuxAction();
  if (action) args.push('-execute', action);

  const bundleId = getTerminalBundleId(process.env.TERM_PROGRAM);
  if (bundleId) args.push('-activate', bundleId);

  spawnSync('terminal-notifier', args, { stdio: 'ignore', timeout: 5000 });
}

function escapeAppleScript(str) {
  return str.replace(/\\/g, '\\\\').replace(/"/g, '\\"');
}

function notifyWithOsascript(title, subtitle, body, sound) {
  const subtitlePart = subtitle ? ` subtitle "${escapeAppleScript(subtitle)}"` : '';
  const soundPart = sound ? ` sound name "${escapeAppleScript(sound)}"` : '';
  const script = `display notification "${escapeAppleScript(body)}" with title "${escapeAppleScript(title)}"${subtitlePart}${soundPart}`;

  spawnSync('osascript', ['-e', script], { stdio: 'ignore', timeout: 5000 });
}

function notify(title, subtitle, body, sound) {
  if (process.platform !== 'darwin') return;

  if (hasCommand('terminal-notifier')) {
    notifyWithTerminalNotifier(title, subtitle, body, sound);
  } else {
    notifyWithOsascript(title, subtitle, body, sound);
  }
}

// Export for testing
if (typeof module !== 'undefined') {
  module.exports = { stripMarkdown, extractSummary, escapeAppleScript, sanitizeShellArg, shouldAlert, lastTurnHasAlertingTool, getTerminalBundleId, getAgentState, buildBody, ALERTING_NOTIFICATION_TYPES, ALERTING_TOOL_NAMES, ALERTING_ERRORS };
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
      const sound = shouldAlert(input) ? SOUND : undefined;
      const body = buildBody(input);
      const subtitle = getSubtitle(input.cwd, input);
      notify(TITLE, subtitle, body, sound);
    } catch {
      // Silent — don't break Claude Code if notification fails
    }
  });
}
