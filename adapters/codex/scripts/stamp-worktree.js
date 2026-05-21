#!/usr/bin/env node
'use strict';

const path = require('path');

const hookRoot = path.resolve(__dirname, '..', 'hooks');
const { readAgentState, writeState } = require(path.join(hookRoot, 'packages', 'agent-state'));
const { getBranch } = require(path.join(hookRoot, 'packages', 'git-status'));

const worktreePath = process.argv[2];
// CLAUDE_SESSION_ID kept as alias; codex mirrors Claude-named env vars so
// CLAUDE_CODE_SESSION_ID works under both harnesses.
const sessionId = process.env.CLAUDE_CODE_SESSION_ID
  || process.env.CLAUDE_SESSION_ID
  || process.argv[3];

if (!worktreePath || !sessionId) {
  console.error('usage: stamp-worktree.js <worktree-path> [session-id]');
  process.exit(2);
}

const abs = path.resolve(worktreePath);
const existing = readAgentState(sessionId) || {};
const pinnedCwd = existing.worktree_cwd || abs;
const branch = getBranch(pinnedCwd);
const update = {};

if (!existing.worktree_cwd) {
  update.worktree_cwd = abs;
}
if (!existing.cwd) {
  update.cwd = pinnedCwd;
}
if (!existing.branch && branch) {
  update.branch = branch;
}

if (Object.keys(update).length > 0) {
  writeState(sessionId, update);
}
process.stdout.write('{}\n');
