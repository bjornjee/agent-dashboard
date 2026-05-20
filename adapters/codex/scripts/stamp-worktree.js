#!/usr/bin/env node
'use strict';

const path = require('path');

const pluginRoot = process.env.CLAUDE_PLUGIN_ROOT || process.env.PLUGIN_ROOT || path.resolve(__dirname, '..', 'hooks');
const { readAgentState, writeState } = require(path.join(pluginRoot, 'packages', 'agent-state'));

const worktreePath = process.argv[2];
// Claude Code exports the session id as CLAUDE_CODE_SESSION_ID; codex's
// OOTB plugin compat (codex-rs/hooks/src/engine/discovery.rs) mirrors
// Claude-named env vars, so reading CLAUDE_CODE_SESSION_ID works under
// both harnesses. CLAUDE_SESSION_ID stays as a documented alias.
const sessionId = process.env.CLAUDE_CODE_SESSION_ID
  || process.env.CLAUDE_SESSION_ID
  || process.argv[3];

if (!worktreePath || !sessionId) {
  console.error('usage: stamp-worktree.js <worktree-path> [session-id]');
  process.exit(2);
}

const abs = path.resolve(worktreePath);
const existing = readAgentState(sessionId) || {};
if (!existing.worktree_cwd) {
  writeState(sessionId, { worktree_cwd: abs });
}
process.stdout.write('{}\n');
