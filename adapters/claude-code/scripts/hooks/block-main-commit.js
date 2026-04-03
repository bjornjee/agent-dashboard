#!/usr/bin/env node
/**
 * PreToolUse hook for Bash — blocks git commit on main/master.
 *
 * Exit code 2 blocks the tool call. Writes reason to stderr.
 */

'use strict';

const { execSync } = require('node:child_process');
const path = require('node:path');

const pluginRoot = path.resolve(__dirname, '..', '..');
const { extractCwdFromCommand } = require(path.join(pluginRoot, 'packages', 'git-status'));

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
      const input = JSON.parse(data);
      const command = (input.tool_input && input.tool_input.command) || '';

      // Quick check: bail early if no git commit in command
      if (!/git\s+commit/.test(command)) {
        process.stdout.write(data);
        return;
      }

      // Get current branch — use cwd from command's cd prefix if present (worktree support)
      const effectiveCwd = extractCwdFromCommand(command);
      let branch;
      try {
        const opts = { encoding: 'utf8', timeout: 3000 };
        if (effectiveCwd) opts.cwd = effectiveCwd;
        branch = execSync('git branch --show-current', opts).trim();
      } catch {
        // Can't determine branch — let it through
        process.stdout.write(data);
        return;
      }

      if (isCommitOnMain(command, branch)) {
        process.stderr.write(
          'Blocked: git commit on main/master is not allowed. ' +
          'Create a feature branch first.\n'
        );
        process.exit(2);
      }

      process.stdout.write(data);
    } catch {
      process.stdout.write(data);
    }
  });
}
