#!/usr/bin/env node
/**
 * PreToolUse hook for Bash — blocks destructive commands.
 *
 * Exit code 2 blocks the tool call. Writes reason to stderr.
 */

'use strict';

const path = require('path');

const pluginRoot = process.env.CLAUDE_PLUGIN_ROOT || path.resolve(__dirname, '..', '..');
const { writeState } = require(path.join(pluginRoot, 'packages', 'agent-state'));

function hasRmRF(cmd) {
  const segments = cmd.split(/[|;&\n]+/);
  for (const seg of segments) {
    const tokens = seg.trim().split(/\s+/);
    const rmIdx = tokens.indexOf('rm');
    if (rmIdx === -1) continue;

    let flags = '';
    for (let i = rmIdx + 1; i < tokens.length; i++) {
      if (tokens[i] === '--') break;
      if (tokens[i].startsWith('--')) continue;
      if (tokens[i].startsWith('-')) flags += tokens[i];
    }

    if (/[rR]/.test(flags) && /f/.test(flags)) return true;
  }
  return false;
}

const DESTRUCTIVE_PATTERNS = [
  { test: hasRmRF,                                   label: 'rm -rf' },
  { pattern: /\bgit\s+reset\s+--hard\b/,             label: 'git reset --hard' },
  { pattern: /\bgit\s+push\s+.*--force\b/,           label: 'git push --force' },
  { pattern: /\bgit\s+push\s+-f\b/,                  label: 'git push -f' },
  { pattern: /\bgit\s+clean\s+(-[^\s]*f[^\s]*|-f)\b/, label: 'git clean -f' },
  { pattern: /\bgit\s+checkout\s+\.\s*([;&|]|$)/,    label: 'git checkout .' },
  { pattern: /\bgit\s+restore\s+\.\s*([;&|]|$)/,     label: 'git restore .' },
  { pattern: /\bdrop\s+table\b/i,                     label: 'DROP TABLE' },
  { pattern: /\bdrop\s+database\b/i,                  label: 'DROP DATABASE' },
  { pattern: /\btruncate\s+table\b/i,                 label: 'TRUNCATE TABLE' },
  { pattern: /\btmux\s+send-keys\b/,                  label: 'tmux send-keys (cross-pane injection)' },
];

module.exports = { hasRmRF, DESTRUCTIVE_PATTERNS };

if (require.main === module && !process.stdin.isTTY) {
  let data = '';

  process.stdin.setEncoding('utf8');
  process.stdin.on('data', chunk => { data += chunk; });
  process.stdin.on('end', () => {
    try {
      const input = JSON.parse(data);
      const command = (input.tool_input && input.tool_input.command) || '';

      for (const { pattern, test, label } of DESTRUCTIVE_PATTERNS) {
        const match = test ? test(command) : pattern.test(command);
        if (match) {
          const reason = `Blocked: "${label}" is a destructive command. If intentional, ask the user to run it manually.`;
          const sessionId = input.session_id;
          if (sessionId) {
            try { writeState(sessionId, { hook_blocked: reason }); } catch { /* don't break Claude Code */ }
          }
          process.stderr.write(reason + '\n');
          process.exit(2);
        }
      }

      // Pass through — print original input to stdout
      process.stdout.write(data);
    } catch {
      // On parse error, don't block — let it through
      process.stdout.write(data);
    }
  });
}
