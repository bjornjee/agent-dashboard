'use strict';

const { spawnSync } = require('child_process');

const TIMEOUT = 2000;

/**
 * Run a tmux command safely. Returns the result or null on failure.
 * @param {string[]} args - tmux command arguments
 * @param {Object} [opts] - additional spawnSync options
 * @returns {{status: number, stdout: string}|null}
 */
function tmux(args, opts = {}) {
  try {
    return spawnSync('tmux', args, {
      encoding: 'utf8',
      timeout: TIMEOUT,
      stdio: ['ignore', 'pipe', 'ignore'],
      ...opts,
    });
  } catch {
    return null;
  }
}

/**
 * Check whether tmux is available and a server is running.
 * @returns {boolean}
 */
function isAvailable() {
  const result = tmux(['list-sessions'], { stdio: 'ignore' });
  return result !== null && result.status === 0;
}

/**
 * Extract session:window from a full session:window.pane target.
 * @param {string} tmuxTarget
 * @returns {string}
 */
function extractSessionWindow(tmuxTarget) {
  const lastDot = tmuxTarget.lastIndexOf('.');
  return lastDot !== -1 ? tmuxTarget.substring(0, lastDot) : tmuxTarget;
}

/**
 * Parse a tmux target string into its components.
 * @param {string} target - e.g. 'session:0.1'
 * @returns {{session: string, window: number, pane: number}}
 */
function parseTarget(target) {
  const colonIdx = target.indexOf(':');
  const lastDot = target.lastIndexOf('.');

  const session = colonIdx !== -1 ? target.substring(0, colonIdx) : target;
  const window = lastDot !== -1
    ? parseInt(target.substring(colonIdx + 1, lastDot), 10)
    : parseInt(target.substring(colonIdx + 1), 10);
  const pane = lastDot !== -1 ? parseInt(target.substring(lastDot + 1), 10) : 0;

  return {
    session,
    window: isNaN(window) ? 0 : window,
    pane: isNaN(pane) ? 0 : pane,
  };
}

/**
 * Capture the last N lines from a tmux pane buffer.
 * @param {string} target - tmux target (session:window.pane)
 * @param {number} [lines=15] - number of lines to capture
 * @returns {string[]} array of lines (empty on failure)
 */
function capture(target, lines = 15) {
  const result = tmux(['capture-pane', '-p', '-t', target, '-S', `-${lines}`]);
  if (!result || result.status !== 0 || !result.stdout) return [];
  return result.stdout.split('\n');
}

/**
 * Jump to a tmux target (select window + pane).
 * @param {string} target - full target (session:window.pane)
 * @returns {boolean} true if both commands succeeded
 */
function jump(target) {
  const sessionWindow = extractSessionWindow(target);
  const w = tmux(['select-window', '-t', sessionWindow], { stdio: 'ignore' });
  const p = tmux(['select-pane', '-t', target], { stdio: 'ignore' });
  return (w !== null && w.status === 0) && (p !== null && p.status === 0);
}

/**
 * Send keystrokes to a tmux pane.
 * @param {string} target - tmux target
 * @param {string} text - text to send
 * @returns {boolean} true if command succeeded
 */
function sendKeys(target, text) {
  const result = tmux(['send-keys', '-t', target, text, 'Enter'], { stdio: 'ignore' });
  return result !== null && result.status === 0;
}

/**
 * List all tmux panes with their metadata.
 * @returns {Array<{target: string, session: string, window: number, pane: number, command: string}>}
 */
function listPanes() {
  const format = '#{session_name}:#{window_index}.#{pane_index}\t#{session_name}\t#{window_index}\t#{pane_index}\t#{pane_current_command}';
  const result = tmux(['list-panes', '-a', '-F', format]);
  if (!result || result.status !== 0 || !result.stdout) return [];

  return result.stdout.trim().split('\n').map(line => {
    const [target, session, window, pane, command] = line.split('\t');
    return {
      target,
      session,
      window: parseInt(window, 10),
      pane: parseInt(pane, 10),
      command: command || '',
    };
  });
}

/**
 * Resolve a TMUX_PANE id to a full session:window.pane target.
 * @param {string} paneId - e.g. '%0'
 * @returns {string|null}
 */
function getTarget(paneId) {
  if (!paneId) return null;
  const result = tmux([
    'display-message', '-t', paneId, '-p',
    '#{session_name}:#{window_index}.#{pane_index}',
  ]);
  if (!result || result.status !== 0 || !result.stdout) return null;
  return result.stdout.trim();
}

module.exports = { isAvailable, capture, jump, sendKeys, listPanes, getTarget, extractSessionWindow, parseTarget };
