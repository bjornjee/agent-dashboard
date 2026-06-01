#!/usr/bin/env node
'use strict';

// Explicit ownership command for skill setup. Use this after a skill creates
// and enters a linked worktree. Automatic hook recovery stays in
// reconcileWorktree(); this wrapper only wires pane/session state into the
// shared claimWorktreeForPane helper.

const path = require('path');

const pluginRoot = process.env.CLAUDE_PLUGIN_ROOT || path.resolve(__dirname, '..', '..');
const { readAllState, writeState } = require(path.join(pluginRoot, 'packages', 'agent-state'));
const { claimWorktreeForPane } = require(path.join(pluginRoot, 'packages', 'worktree-reconcile'));

function stateDirFromEnv(env = process.env) {
  return env.AGENT_DASHBOARD_DIR || path.join(env.HOME || env.USERPROFILE || '/tmp', '.agent-dashboard');
}

function main(argv = process.argv, env = process.env) {
  if (argv.length > 3) throw new Error('usage: node claim-worktree.js [worktree-path]');
  const worktreePath = argv[2] || process.cwd();
  claimWorktreeForPane({
    worktreePath,
    paneId: env.TMUX_PANE,
    stateDir: stateDirFromEnv(env),
    readAllState,
    writeState,
  });
}

if (require.main === module) {
  try {
    main();
  } catch (err) {
    console.error(`error: ${err.message}`);
    process.exit(1);
  }
}
