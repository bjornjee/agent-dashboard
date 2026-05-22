#!/usr/bin/env node
'use strict';

const { spawn, spawnSync } = require('child_process');
const path = require('path');

const hookRoot = __dirname;
const { readAgentState, writeState } = require(path.join(hookRoot, 'packages', 'agent-state'));

const defaultInitialDelayMs = 400;
const defaultPollDelayMs = 200;
const defaultMaxAttempts = 25;

function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

function sendLine(tmuxPane, text) {
  const textResult = spawnSync('tmux', ['send-keys', '-l', '-t', tmuxPane, text], {
    timeout: 1000,
    stdio: 'ignore',
  });
  if (textResult.status !== 0) return false;

  const enterResult = spawnSync('tmux', ['send-keys', '-t', tmuxPane, 'Enter'], {
    timeout: 1000,
    stdio: 'ignore',
  });
  return enterResult.status === 0;
}

async function runAutoPlan({
  sessionId,
  tmuxPane,
  deferredPrompt,
  readAgentState: readState = readAgentState,
  writeState: write = writeState,
  sendLine: send = sendLine,
  sleep: wait = sleep,
  maxAttempts = defaultMaxAttempts,
  initialDelayMs = defaultInitialDelayMs,
  pollDelayMs = defaultPollDelayMs,
  now = () => new Date().toISOString(),
}) {
  if (!sessionId || !tmuxPane || !deferredPrompt) {
    return { status: 'skipped' };
  }

  const existing = readState(sessionId) || {};
  if (existing.auto_plan_injected_at) {
    return { status: 'already-injected' };
  }

  write(sessionId, {
    auto_plan_injected_at: now(),
    auto_plan_status: 'starting',
  });

  await wait(initialDelayMs);
  if (!send(tmuxPane, '/plan')) {
    write(sessionId, {
      auto_plan_status: 'send-plan-failed',
      auto_plan_error: 'failed to send /plan to tmux pane',
    });
    return { status: 'send-plan-failed' };
  }

  for (let attempt = 0; attempt < maxAttempts; attempt++) {
    const state = readState(sessionId) || {};
    if (state.permission_mode === 'plan') {
      if (!send(tmuxPane, deferredPrompt)) {
        write(sessionId, {
          auto_plan_status: 'send-prompt-failed',
          auto_plan_error: 'failed to send deferred prompt to tmux pane',
        });
        return { status: 'send-prompt-failed' };
      }
      write(sessionId, { auto_plan_status: 'done' });
      return { status: 'done' };
    }
    await wait(pollDelayMs);
  }

  write(sessionId, {
    auto_plan_status: 'timeout',
    auto_plan_error: 'permission_mode did not become plan',
  });
  return { status: 'timeout' };
}

function spawnWorker(input) {
  if (process.env.AGENT_DASHBOARD_AUTO_PLAN !== '1') return;
  const sessionId = input.session_id || '';
  const deferredPrompt = process.env.AGENT_DASHBOARD_DEFERRED_PROMPT || '';
  const tmuxPane = process.env.TMUX_PANE || '';
  if (!sessionId || !deferredPrompt || !tmuxPane) return;

  const child = spawn(process.execPath, [__filename, '--worker'], {
    detached: true,
    stdio: 'ignore',
    env: {
      ...process.env,
      AGENT_DASHBOARD_AUTO_PLAN_SESSION_ID: sessionId,
    },
  });
  child.unref();
}

async function runWorkerFromEnv() {
  await runAutoPlan({
    sessionId: process.env.AGENT_DASHBOARD_AUTO_PLAN_SESSION_ID || '',
    tmuxPane: process.env.TMUX_PANE || '',
    deferredPrompt: process.env.AGENT_DASHBOARD_DEFERRED_PROMPT || '',
  });
}

if (require.main === module) {
  if (process.argv[2] === '--worker') {
    runWorkerFromEnv()
      .catch(() => {});
  } else {
    let data = '';
    process.stdin.setEncoding('utf8');
    process.stdin.on('data', chunk => { data += chunk; });
    process.stdin.on('end', () => {
      try {
        const input = data.trim() ? JSON.parse(data) : {};
        spawnWorker(input);
      } catch { /* hooks must not break Codex */ }
      process.stdout.write('{}\n');
    });
  }
}

module.exports = { runAutoPlan };
