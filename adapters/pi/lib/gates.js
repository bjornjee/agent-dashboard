'use strict';

const path = require('path');
const childProcess = require('child_process');

const CLAUDE_HOOKS_DIR = path.resolve(__dirname, '..', '..', 'claude-code', 'scripts', 'hooks');
const CLAUDE_PLUGIN_ROOT = path.resolve(__dirname, '..', '..', 'claude-code');

function runGate(scriptPath, payload, opts = {}) {
  const spawn = opts.spawn || childProcess.spawnSync;
  let result;
  try {
    result = spawn('node', [scriptPath], {
      input: JSON.stringify(payload),
      env: { ...process.env, CLAUDE_PLUGIN_ROOT },
      timeout: 30000,
    });
  } catch {
    return null;
  }

  if (result && result.status === 2) {
    const stderr = result.stderr ? result.stderr.toString().trim() : '';
    return { block: true, reason: stderr || 'Blocked by gate' };
  }
  return null;
}

function runGates(scriptPaths, payload, opts = {}) {
  for (const scriptPath of scriptPaths) {
    const result = runGate(scriptPath, payload, opts);
    if (result) return result;
  }
  return null;
}

function gateScript(name) {
  return path.join(CLAUDE_HOOKS_DIR, name);
}

const PRE_BASH_GATES = [
  gateScript('warn-destructive.js'),
  gateScript('block-main-commit.js'),
  gateScript('test-gate.js'),
];

const POST_BASH_GATES = [
  gateScript('commit-lint.js'),
];

const STOP_HOOKS = [
  gateScript('desktop-notify.js'),
];

module.exports = {
  runGate,
  runGates,
  gateScript,
  PRE_BASH_GATES,
  POST_BASH_GATES,
  STOP_HOOKS,
  CLAUDE_PLUGIN_ROOT,
};
