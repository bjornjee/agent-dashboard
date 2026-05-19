#!/usr/bin/env node
/**
 * PostToolUse hook for ExitPlanMode — nudges Claude to delegate to Codex.
 *
 * When in a git worktree and `codex` is available in PATH, writes a reminder
 * to stderr so Claude delegates via /codex-delegate. Always exits 0.
 * Set SKIP_CODEX_GATE=1 to suppress.
 */

'use strict';

const { statSync } = require('node:fs');
const { spawnSync } = require('node:child_process');
const path = require('node:path');

const NUDGE_MESSAGE = [
  'You are in a git worktree and Codex CLI is available.',
  'Only delegate to Codex via /codex-delegate if:',
  '  1. The user explicitly asked for Codex delegation, OR',
  '  2. The plan touches 10+ files or ~3,000+ lines of implementation — below that threshold the orchestration overhead costs more tokens than Claude implementing directly.',
  'If delegating, use --write flag to ensure Codex has workspace write access.',
  'To skip this nudge, set SKIP_CODEX_GATE=1.',
].join('\n');

function isWorktree(cwd) {
  const dir = cwd || process.cwd();
  try {
    return statSync(path.join(dir, '.git')).isFile();
  } catch {
    return false;
  }
}

function isCodexAvailable() {
  try {
    const result = spawnSync('which', ['codex'], {
      stdio: ['ignore', 'pipe', 'ignore'],
      timeout: 2000,
      encoding: 'utf8',
    });
    return result.status === 0;
  } catch {
    return false;
  }
}

function shouldNudge(cwd) {
  return isWorktree(cwd) && isCodexAvailable();
}

function shouldSkip() {
  return process.env.SKIP_CODEX_GATE === '1';
}

module.exports = { isWorktree, isCodexAvailable, shouldNudge, shouldSkip, NUDGE_MESSAGE };

if (require.main === module && !process.stdin.isTTY) {
  let data = '';

  process.stdin.setEncoding('utf8');
  process.stdin.on('data', chunk => { data += chunk; });

  process.stdin.on('end', () => {
    try {
      if (shouldSkip()) return;

      const input = data.trim() ? JSON.parse(data) : {};
      const cwd = input.cwd || null;

      if (shouldNudge(cwd)) {
        process.stderr.write(NUDGE_MESSAGE + '\n');
      }
    } catch {
      // Silent — don't break Claude Code
    }
  });
}
