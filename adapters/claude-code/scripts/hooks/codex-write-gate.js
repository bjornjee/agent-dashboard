#!/usr/bin/env node
/**
 * PreToolUse hook for Bash — blocks `codex-companion.mjs task` without --write.
 *
 * When a Bash tool call invokes `codex-companion.mjs task` without the --write
 * flag, this hook blocks execution (exit 2) so the agent retries with --write.
 * Without --write, Codex runs in read-only sandbox mode and cannot modify files.
 *
 * Set SKIP_CODEX_WRITE_GATE=1 to suppress.
 *
 * Stdin: JSON from Claude Code hook system (PreToolUse, Bash)
 */

'use strict';

const BLOCK_MESSAGE = [
  'Codex task requires --write for workspace write access.',
  'Add --write to the codex-companion.mjs task command.',
  'Without it, Codex runs in read-only sandbox mode and cannot modify files.',
].join('\n');

function isCodexTask(command) {
  if (!command || typeof command !== 'string') return false;
  return /codex-companion\.mjs\s+task\b/.test(command);
}

function hasWriteFlag(command) {
  if (!command || typeof command !== 'string') return false;
  return /(^|\s)--write\b/.test(command);
}

function shouldSkip() {
  return process.env.SKIP_CODEX_WRITE_GATE === '1';
}

module.exports = { isCodexTask, hasWriteFlag, shouldSkip, BLOCK_MESSAGE };

if (require.main === module && !process.stdin.isTTY) {
  let data = '';

  process.stdin.setEncoding('utf8');
  process.stdin.on('data', chunk => { data += chunk; });

  process.stdin.on('end', () => {
    try {
      if (shouldSkip()) {
        process.stdout.write(data);
        return;
      }

      const input = data.trim() ? JSON.parse(data) : {};
      const command = input.tool_input?.command ?? '';

      if (!isCodexTask(command)) {
        process.stdout.write(data);
        return;
      }

      if (hasWriteFlag(command)) {
        process.stdout.write(data);
        return;
      }

      process.stderr.write(BLOCK_MESSAGE + '\n');
      process.exit(2);
    } catch {
      process.stdout.write(data);
    }
  });
}
