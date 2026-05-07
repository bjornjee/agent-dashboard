#!/usr/bin/env node
/**
 * PreToolUse hook for Bash — blocks direct `gh pr create`.
 *
 * The /agent-dashboard:pr skill is the only sanctioned path to PR creation:
 * it runs refactor-cleaner, `make fmt`, and `make test` before opening the
 * PR. To bypass this gate, the skill prefixes its `gh pr create` invocation
 * with `AGENT_DASHBOARD_PR_SKILL=1` — the hook detects that marker in the
 * command string and lets it through.
 *
 * Set SKIP_PR_SKILL_GATE=1 to bypass entirely (emergencies only).
 * Exit code 2 blocks the tool call. Writes reason to stderr.
 */

'use strict';

const BYPASS_MARKER = /\bAGENT_DASHBOARD_PR_SKILL=1\b/;
// `gh pr create` at the start of a shell segment, optionally preceded by
// `VAR=value ` env-var assignments. Split on `[;&|]+` (pipes included) so
// `echo "gh pr create"` doesn't trigger.
const PR_CREATE_RE = /^\s*(?:[A-Za-z_][A-Za-z0-9_]*=\S*\s+)*gh\s+pr\s+create\b/;

function isUngatedPRCreate(command) {
  if (!command || typeof command !== 'string') return false;
  if (process.env.SKIP_PR_SKILL_GATE === '1') return false;

  const segments = command.split(/[;&|]+/);
  const matched = segments.some(seg => PR_CREATE_RE.test(seg));
  if (!matched) return false;

  if (BYPASS_MARKER.test(command)) return false;
  return true;
}

module.exports = { isUngatedPRCreate };

if (require.main === module && !process.stdin.isTTY) {
  let data = '';

  process.stdin.setEncoding('utf8');
  process.stdin.on('data', chunk => { data += chunk; });
  process.stdin.on('end', () => {
    try {
      const input = JSON.parse(data);
      const command = (input.tool_input && input.tool_input.command) || '';

      if (isUngatedPRCreate(command)) {
        process.stderr.write(
          'Blocked: direct `gh pr create` is disabled. ' +
          'Use /agent-dashboard:pr to run cleanup, fmt, and test gates ' +
          'before opening the PR.\n' +
          'Set SKIP_PR_SKILL_GATE=1 to bypass (emergencies only).\n'
        );
        process.exit(2);
      }

      process.stdout.write(data);
    } catch {
      process.stdout.write(data);
    }
  });
}
