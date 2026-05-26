#!/usr/bin/env node
'use strict';

const path = require('path');

const pluginRoot = process.env.CLAUDE_PLUGIN_ROOT || path.resolve(__dirname, '..');
const { readAgentState, writeState } = require(path.join(pluginRoot, 'packages', 'agent-state'));
const { getBranch } = require(path.join(pluginRoot, 'packages', 'git-status'));

const worktreePath = process.argv[2];
// CLAUDE_SESSION_ID kept as alias so callers from earlier docs still work.
const sessionId = process.env.CLAUDE_CODE_SESSION_ID
  || process.env.CLAUDE_SESSION_ID
  || process.argv[3];

if (!worktreePath || !sessionId) {
  console.error('usage: stamp-worktree.js <worktree-path> [session-id]');
  process.exit(2);
}

const abs = path.resolve(worktreePath);
const existing = readAgentState(sessionId) || {};
// Pin branch against the already-stamped worktree_cwd when present so a
// later invocation with a different worktreePath arg can't poison the pin.
const pinnedCwd = existing.worktree_cwd || abs;
const branch = getBranch(pinnedCwd);
const update = {};

if (!existing.worktree_cwd) {
  update.worktree_cwd = abs;
}
if (!existing.branch && branch) {
  update.branch = branch;
}

if (Object.keys(update).length > 0) {
  writeState(sessionId, update);
}
process.stdout.write('{}\n');
