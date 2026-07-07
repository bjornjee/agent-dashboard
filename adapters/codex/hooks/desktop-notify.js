#!/usr/bin/env node
'use strict';

const { spawnSync } = require('child_process');
const { basename, resolve } = require('path');

const { extractSessionWindow } = require(resolve(__dirname, 'packages', 'tmux'));

const TITLE = 'Codex';
const SOUND = 'Blow';
const ALERTING_NOTIFICATION_TYPES = new Set([
  'permission_prompt',
  'idle_prompt',
  'elicitation_dialog',
]);

const ALERTING_ERRORS = new Set();

function shouldAlert(input) {
  if (input.hook_event_name === 'Notification') {
    return ALERTING_NOTIFICATION_TYPES.has(input.notification_type);
  }

  if (input.hook_event_name === 'StopFailure') {
    return ALERTING_ERRORS.has(input.error);
  }

  return false;
}

function buildBody(input) {
  if (input.hook_event_name === 'Notification') {
    return input.message || input.title || 'Notification';
  }

  if (input.hook_event_name === 'StopFailure') {
    return input.error_details || input.error || 'Error';
  }

  return 'Notification';
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

  const state = getAgentState(input);
  if (state) parts.push(state);

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

function getAgentState(input) {
  if (input.hook_event_name === 'Notification') {
    return NOTIFICATION_STATE_MAP[input.notification_type] || 'notification';
  }

  if (input.hook_event_name === 'StopFailure') {
    return input.error === 'rate_limit' ? 'rate limited' : 'error';
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
  const sessionWindow = extractSessionWindow(t);
  return `tmux select-window -t '${sessionWindow}' && tmux select-pane -t '${t}'`;
}

function notifyWithTerminalNotifier(title, subtitle, body, sound) {
  const args = ['-title', title, '-message', body, '-group', `codex-${process.pid}`];

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

function emitCodexNoopOutput() {
  process.stdout.write('{}\n');
}

module.exports = {
  escapeAppleScript,
  sanitizeShellArg,
  shouldAlert,
  getTerminalBundleId,
  getAgentState,
  buildBody,
  ALERTING_NOTIFICATION_TYPES,
  ALERTING_ERRORS,
};

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
      if (shouldAlert(input)) {
        const body = buildBody(input);
        const subtitle = getSubtitle(input.cwd, input);
        notify(TITLE, subtitle, body, SOUND);
      }
    } catch {
      // Notification failures must not break Codex hook execution.
    } finally {
      emitCodexNoopOutput();
    }
  });
}
