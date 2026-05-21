'use strict';

const { spawnSync } = require('child_process');

const TIMEOUT = 2000;

/**
 * Find the merge-base commit between HEAD and the default branch.
 * Prefers origin/main over local main (and origin/master over local master)
 * to avoid stale results when the local default branch is behind the remote.
 * Returns the commit hash, or null if not found.
 * @param {string} cwd - working directory
 * @returns {string|null}
 */
function findMergeBase(cwd) {
  for (const base of ['origin/main', 'main', 'origin/master', 'master']) {
    const result = spawnSync('git', ['merge-base', 'HEAD', base], {
      encoding: 'utf8',
      timeout: TIMEOUT,
      cwd,
      stdio: ['ignore', 'pipe', 'ignore'],
    });
    if (result.status === 0 && result.stdout.trim()) {
      return result.stdout.trim();
    }
  }
  return null;
}

/**
 * Get files changed on the current branch (committed + uncommitted).
 * Diffs from the merge-base with main/master to capture all branch changes.
 * Falls back to diffing against HEAD if no merge-base is found.
 * Returns array of strings with prefix: '+' added, '~' modified, '-' deleted.
 * @param {string} cwd - working directory
 * @returns {string[]}
 */
function getChangedFiles(cwd) {
  if (!cwd) return [];

  const diffFrom = findMergeBase(cwd) || 'HEAD';

  const result = spawnSync('git', ['diff', '--name-status', diffFrom], {
    encoding: 'utf8',
    timeout: TIMEOUT,
    cwd,
    stdio: ['ignore', 'pipe', 'ignore'],
  });

  if (result.status !== 0 || !result.stdout) return [];

  const prefixMap = { A: '+', M: '~', D: '-', R: '~', C: '~' };

  return result.stdout.trim().split('\n').map(line => {
    const [status, ...fileParts] = line.split('\t');
    const file = fileParts[fileParts.length - 1]; // handle renames (R100\told\tnew)
    const prefix = prefixMap[status?.[0]] || '~';
    return `${prefix}${file}`;
  }).filter(Boolean);
}

/**
 * Get the current git branch name.
 * @param {string} cwd - working directory
 * @returns {string|null}
 */
function getBranch(cwd) {
  if (!cwd) return null;

  const result = spawnSync('git', ['branch', '--show-current'], {
    encoding: 'utf8',
    timeout: TIMEOUT,
    cwd,
    stdio: ['ignore', 'pipe', 'ignore'],
  });

  if (result.status !== 0 || !result.stdout) return null;
  return result.stdout.trim() || null;
}

/**
 * Extract the effective working directory from a Bash command string.
 * Handles the worktree pattern: cd /absolute/path && ...
 * @param {string} command
 * @returns {string|null} absolute path or null
 */
function extractCwdFromCommand(command) {
  if (!command) return null;
  const match = command.match(/^\s*cd\s+(?:"([^"]+)"|'([^']+)'|(\S+))\s*(?:&&|;|\|\||$)/);
  if (!match) return null;
  const dir = match[1] || match[2] || match[3];
  if (!dir || !dir.startsWith('/')) return null;
  return dir;
}

module.exports = { getChangedFiles, getBranch, extractCwdFromCommand, findMergeBase };
