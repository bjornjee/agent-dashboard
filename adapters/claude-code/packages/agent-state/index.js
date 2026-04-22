'use strict';

const fs = require('fs');
const path = require('path');
const crypto = require('crypto');
const { validateAgent } = require('./schema');
const { detectState } = require('./detect');

const DEFAULT_AGENTS_DIR = path.join(
  process.env.AGENT_DASHBOARD_DIR || path.join(process.env.HOME || process.env.USERPROFILE || '/tmp', '.agent-dashboard'),
  'agents',
);

/**
 * Get the file path for a specific agent by session_id.
 * UUIDs are filesystem-safe, so no encoding is needed.
 * @param {string} sessionId - Claude session_id (UUID)
 * @param {string} [agentsDir] - directory containing per-agent files
 * @returns {string}
 */
function agentFilePath(sessionId, agentsDir = DEFAULT_AGENTS_DIR) {
  return path.join(agentsDir, sessionId + '.json');
}

/**
 * Read a single agent's state from its per-agent file.
 * @param {string} sessionId - Claude session_id (UUID)
 * @param {string} [agentsDir] - directory containing per-agent files
 * @returns {Object|null} - agent state or null if not found/invalid
 */
function readAgentState(sessionId, agentsDir = DEFAULT_AGENTS_DIR) {
  try {
    const raw = fs.readFileSync(agentFilePath(sessionId, agentsDir), 'utf8');
    const parsed = JSON.parse(raw);
    return (parsed && typeof parsed === 'object') ? parsed : null;
  } catch {
    return null;
  }
}

/**
 * Read all agent state files from the agents directory.
 * Keys are session_id (filename stem).
 * @param {string} [agentsDir] - directory containing per-agent files
 * @returns {{agents: Object}} - state object with all agents keyed by session_id
 */
function readAllState(agentsDir = DEFAULT_AGENTS_DIR) {
  const result = { agents: {} };

  try {
    const files = fs.readdirSync(agentsDir);
    for (const file of files) {
      if (!file.endsWith('.json')) continue;

      try {
        const raw = fs.readFileSync(path.join(agentsDir, file), 'utf8');
        const agent = JSON.parse(raw);
        const sessionId = file.slice(0, -5); // strip .json
        if (validateAgent(agent)) {
          result.agents[sessionId] = agent;
        }
      } catch {
        // Skip corrupted files
      }
    }
  } catch {
    // Directory doesn't exist yet
  }

  return result;
}

/**
 * Write/merge an agent update into its per-agent file (atomic).
 * No cross-agent locking needed — each session writes only its own file.
 * @param {string} sessionId - Claude session_id (UUID)
 * @param {Object} update - fields to merge into the agent entry
 * @param {string} [agentsDir] - directory containing per-agent files
 */
function writeState(sessionId, update, agentsDir = DEFAULT_AGENTS_DIR, opts = {}) {
  fs.mkdirSync(agentsDir, { recursive: true });

  const filePath = agentFilePath(sessionId, agentsDir);
  const existing = readAgentState(sessionId, agentsDir) || {};

  // Guard: skip write if current on-disk state is protected.
  // This eliminates the TOCTOU race where a caller checks stale state and then
  // writeState re-reads fresh state — the guard now runs against the fresh read.
  if (opts.guardStates && opts.guardStates.has(existing.state)) {
    return;
  }

  const merged = {
    ...existing,
    ...update,
    updated_at: new Date().toISOString(),
  };

  // Atomic write via tmp file + rename. Clean up tmp on failure.
  const tmp = filePath + `.tmp.${process.pid}.${crypto.randomBytes(4).toString('hex')}`;
  try {
    fs.writeFileSync(tmp, JSON.stringify(merged, null, 2));
    fs.renameSync(tmp, filePath);
  } catch (err) {
    try { fs.unlinkSync(tmp); } catch { /* ignore */ }
    throw err;
  }
}

/**
 * Watch the agents directory for changes with debounce.
 * @param {function} callback - called with all agents state on change
 * @param {string} [agentsDir] - directory containing per-agent files
 * @param {number} [debounceMs=300] - debounce interval
 * @returns {function} stop - call to stop watching
 */
function watchState(callback, agentsDir = DEFAULT_AGENTS_DIR, debounceMs = 300) {
  let timer = null;
  let watcher = null;

  fs.mkdirSync(agentsDir, { recursive: true });

  try {
    watcher = fs.watch(agentsDir, () => {
      if (timer) clearTimeout(timer);
      timer = setTimeout(() => {
        callback(readAllState(agentsDir));
      }, debounceMs);
    });
  } catch {
    // Fallback to polling if fs.watch isn't available
    const interval = setInterval(() => {
      callback(readAllState(agentsDir));
    }, 1000);
    return () => clearInterval(interval);
  }

  return () => {
    if (timer) clearTimeout(timer);
    if (watcher) watcher.close();
  };
}

/**
 * Remove stale agent files that haven't been updated within the threshold.
 * Also cleans orphaned tmp files older than 60s.
 * @param {number} [maxAgeMs=300000] - max age in ms (default 5 min)
 * @param {string} [agentsDir] - directory containing per-agent files
 */
function cleanStale(maxAgeMs = 300000, agentsDir = DEFAULT_AGENTS_DIR) {
  let files;
  try {
    files = fs.readdirSync(agentsDir);
  } catch {
    return; // Directory doesn't exist
  }

  const now = Date.now();

  for (const file of files) {
    const filePath = path.join(agentsDir, file);

    // Clean up orphaned tmp files older than 60s
    if (file.includes('.tmp.')) {
      try {
        const stat = fs.statSync(filePath);
        if (now - stat.mtimeMs > 60000) {
          fs.unlinkSync(filePath);
        }
      } catch { /* ignore */ }
      continue;
    }

    if (!file.endsWith('.json')) continue;

    try {
      const raw = fs.readFileSync(filePath, 'utf8');
      const agent = JSON.parse(raw);
      const age = now - new Date(agent.updated_at || 0).getTime();
      if (age > maxAgeMs) {
        fs.unlinkSync(filePath);
      }
    } catch {
      // Skip files we can't read
    }
  }
}

/**
 * Remove a specific agent's state file.
 * @param {string} sessionId - Claude session_id (UUID)
 * @param {string} [agentsDir] - directory containing per-agent files
 */
function removeAgent(sessionId, agentsDir = DEFAULT_AGENTS_DIR) {
  try {
    fs.unlinkSync(agentFilePath(sessionId, agentsDir));
  } catch {
    // File already removed or never existed
  }
}

module.exports = {
  readAgentState,
  readAllState,
  writeState,
  watchState,
  cleanStale,
  removeAgent,
  detectState,
  DEFAULT_AGENTS_DIR,
};
