#!/usr/bin/env node
/**
 * PreToolUse hook for Bash — blocks git commit unless `make test` passes.
 *
 * Delegates entirely to the project's Makefile. If no Makefile or test target
 * exists, nudges the user to add one (but does not block the commit).
 *
 * Set SKIP_TEST_GATE=1 to bypass (WIP commits, docs-only changes).
 * Exit code 2 blocks the tool call. Writes reason to stderr.
 */

'use strict';

const { execSync } = require('node:child_process');
const { existsSync } = require('node:fs');
const path = require('node:path');

const pluginRoot = path.resolve(__dirname, '..', '..');
const { extractCwdFromCommand } = require(path.join(pluginRoot, 'packages', 'git-status'));

function isGitCommit(command) {
  return /\bgit\s+commit\b/.test(command);
}

function shouldSkip() {
  return process.env.SKIP_TEST_GATE === '1';
}

function getRepoRoot(cwd) {
  try {
    const opts = { encoding: 'utf8', stdio: 'pipe' };
    if (cwd) opts.cwd = cwd;
    return execSync('git rev-parse --show-toplevel', opts).trim();
  } catch {
    return cwd || process.cwd();
  }
}

function hasMakefile(cwd) {
  return existsSync(path.join(cwd, 'Makefile'));
}

/**
 * Find the best test target: prefer test-fast, fall back to test, null if neither.
 * @param {string} cwd
 * @returns {string|null}
 */
function getTestTarget(cwd) {
  if (!hasMakefile(cwd)) return null;
  for (const target of ['test-fast', 'test']) {
    try {
      execSync(`make -n ${target} 2>/dev/null`, { stdio: 'pipe', cwd });
      return target;
    } catch { /* target doesn't exist, try next */ }
  }
  return null;
}

function runMakeTest(cwd) {
  const target = getTestTarget(cwd);
  if (!target) return { passed: true };
  try {
    execSync(`make ${target}`, { stdio: 'pipe', timeout: 60000, cwd });
    return { passed: true };
  } catch (err) {
    const stdout = (err.stdout || '').toString();
    const stderr = (err.stderr || '').toString();
    const output = stdout + stderr;
    return { passed: false, output: output.slice(Math.max(0, output.length - 2000)) };
  }
}

// Export for testing
module.exports = { isGitCommit, shouldSkip, getRepoRoot, hasMakefile, getTestTarget, runMakeTest };

// Only run as hook when executed directly (not imported by test runner)
if (require.main === module && !process.stdin.isTTY) {
  let data = '';

  process.stdin.setEncoding('utf8');
  process.stdin.on('data', chunk => { data += chunk; });
  process.stdin.on('end', () => {
    try {
      const input = JSON.parse(data);
      const command = (input.tool_input && input.tool_input.command) || '';

      if (!isGitCommit(command)) {
        process.stdout.write(data);
        return;
      }

      if (shouldSkip()) {
        process.stdout.write(data);
        return;
      }

      const effectiveCwd = extractCwdFromCommand(command);
      const repoRoot = getRepoRoot(effectiveCwd);
      const testTarget = getTestTarget(repoRoot);

      if (!testTarget) {
        process.stderr.write(
          `Warning: No Makefile with a "test" or "test-fast" target found. ` +
          `Add one to enable pre-commit test gating.\n`
        );
        process.stdout.write(data);
        return;
      }

      const result = runMakeTest(repoRoot);
      if (!result.passed) {
        process.stderr.write(
          `Blocked: tests failed. Fix failing tests before committing.\n\n` +
          `${result.output}\n\n` +
          `Set SKIP_TEST_GATE=1 to bypass (e.g., for WIP commits).\n`
        );
        process.exit(2);
      }

      process.stdout.write(data);
    } catch {
      process.stdout.write(data);
    }
  });
}
