#!/usr/bin/env node
/**
 * PreToolUse hook for Bash — blocks git commit on main/master.
 *
 * Exit code 2 blocks the tool call (Codex respects this for PreToolUse,
 * matching Claude Code semantics). On pass-through emit `{}` so Codex's
 * stdout JSON contract is satisfied.
 */

'use strict';

const { execSync } = require('node:child_process');
const path = require('node:path');

const { extractCwdFromCommand } = require(path.join(__dirname, 'packages', 'git-status'));

function isCommitOnMain(command, branch) {
  if (branch !== 'main' && branch !== 'master') return false;

  const segments = command.split(/[;&]+/);
  for (const seg of segments) {
    const trimmed = seg.trim();
    if (/^\s*git\s+commit\b/.test(trimmed)) return true;
  }
  return false;
}

module.exports = { isCommitOnMain };

if (require.main === module && !process.stdin.isTTY) {
  let data = '';

  process.stdin.setEncoding('utf8');
  process.stdin.on('data', chunk => { data += chunk; });
  process.stdin.on('end', () => {
    try {
      const input = data.trim() ? JSON.parse(data) : {};
      const command = (input.tool_input && input.tool_input.command) || '';

      if (!/git\s+commit/.test(command)) {
        process.stdout.write('{}\n');
        return;
      }

      // Resolve cwd from the command's `cd ... && ...` prefix first, then the
      // hook payload's input.cwd. Without this the hook would run git in its
      // own cwd (the codex install root), causing false positives in worktrees.
      const effectiveCwd = extractCwdFromCommand(command) || input.cwd || null;
      let branch;
      try {
        const opts = { encoding: 'utf8', timeout: 3000 };
        if (effectiveCwd) opts.cwd = effectiveCwd;
        branch = execSync('git branch --show-current', opts).trim();
      } catch {
        process.stdout.write('{}\n');
        return;
      }

      if (isCommitOnMain(command, branch)) {
        process.stderr.write(
          'Blocked: git commit on main/master is not allowed. ' +
          'Create a feature branch first.\n'
        );
        process.exit(2);
      }

      process.stdout.write('{}\n');
    } catch {
      process.stdout.write('{}\n');
    }
  });
}
