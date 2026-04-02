#!/usr/bin/env node
/**
 * PreToolUse hook for Bash — validates conventional commit messages.
 *
 * Exit code 2 blocks the tool call. Writes reason to stderr.
 */

'use strict';

const VALID_TYPES = ['feat', 'fix', 'refactor', 'docs', 'test', 'chore', 'perf', 'ci'];
const COMMIT_RE = /^(feat|fix|refactor|docs|test|chore|perf|ci):\s+\S/;

function extractCommitMessage(command) {
  // HEREDOC pattern first (most specific): -m "$(cat <<'EOF'\n...\nEOF\n)"
  const heredoc = command.match(/-m\s+"\$\(cat\s+<<'?EOF'?\n([\s\S]*?)\nEOF\n?\s*\)"/);
  if (heredoc) return heredoc[1].trim();

  // Match -m "msg", -m 'msg'
  const doubleQuote = command.match(/\bgit\s+commit\b[^]*?-m\s+"([^"]+)"/);
  if (doubleQuote) return doubleQuote[1];

  const singleQuote = command.match(/\bgit\s+commit\b[^]*?-m\s+'([^']+)'/);
  if (singleQuote) return singleQuote[1];

  return null;
}

function validateCommitMessage(message) {
  if (!message) return { valid: false, reason: 'Could not parse commit message from command.' };

  // Check first line only (strip Co-Authored-By and other trailers)
  const firstLine = message.split('\n')[0].trim();

  if (!COMMIT_RE.test(firstLine)) {
    return {
      valid: false,
      reason:
        `Commit message "${firstLine}" does not follow conventional format.\n` +
        `Expected: <type>: <description>\n` +
        `Valid types: ${VALID_TYPES.join(', ')}`
    };
  }

  return { valid: true };
}

// Export for testing
module.exports = { extractCommitMessage, validateCommitMessage, VALID_TYPES };

// Only run as hook when executed directly (not imported by test runner)
if (require.main === module && !process.stdin.isTTY) {
  let data = '';

  process.stdin.setEncoding('utf8');
  process.stdin.on('data', chunk => { data += chunk; });
  process.stdin.on('end', () => {
    try {
      const input = JSON.parse(data);
      const command = (input.tool_input && input.tool_input.command) || '';

      // Only check commands that contain git commit
      if (!/\bgit\s+commit\b/.test(command)) {
        process.stdout.write(data);
        return;
      }

      const message = extractCommitMessage(command);

      // No -m flag: amend, --reuse-message, etc. — pass through
      if (message === null) {
        process.stdout.write(data);
        return;
      }

      const result = validateCommitMessage(message);

      if (!result.valid) {
        process.stderr.write(`Blocked: ${result.reason}\n`);
        process.exit(2);
      }

      process.stdout.write(data);
    } catch {
      process.stdout.write(data);
    }
  });
}
