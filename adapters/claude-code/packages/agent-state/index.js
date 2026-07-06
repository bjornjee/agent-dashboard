'use strict';

const fs = require('fs');
const path = require('path');
const crypto = require('crypto');
const { normalizeState, validateAgent } = require('./schema');
const { detectState } = require('./detect');
const { withFileLock } = require('./filelock');

const DEFAULT_AGENTS_DIR = path.join(
  process.env.AGENT_DASHBOARD_DIR || path.join(process.env.HOME || process.env.USERPROFILE || '/tmp', '.agent-dashboard'),
  'agents',
);

// report_seq is a scaled wall-clock stamp (Date.now() * 1000). Treat an on-disk
// value more than this far in the future as implausible (corrupt/restored file
// or a clock jump) and ignore it for ordering, so it can't freeze writes forever
// — the next write overwrites it and self-heals.
const REPORT_SEQ_FUTURE_SLACK = 60_000 * 1000; // ~60s in the same scaled units

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
          result.agents[sessionId] = { ...agent, state: normalizeState(agent.state) };
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
 * Write/merge an agent update into its per-agent file.
 *
 * Read → merge → atomic write inside a sidecar file lock so concurrent
 * hook subprocesses and the dashboard pin/stamp paths don't overwrite
 * each other's fields (lost-update race). The atomic rename keeps any
 * unlucky reader (dashboard refresh, sibling hook) from seeing a torn
 * intermediate state.
 *
 * @param {string} sessionId - Claude session_id (UUID)
 * @param {Object} update - fields to merge into the agent entry
 * @param {string} [agentsDir] - directory containing per-agent files
 */
function writeState(sessionId, update, agentsDir = DEFAULT_AGENTS_DIR, opts = {}) {
  fs.mkdirSync(agentsDir, { recursive: true });

  const filePath = agentFilePath(sessionId, agentsDir);

  withFileLock(filePath, () => {
    const existing = readAgentState(sessionId, agentsDir) || {};

    const guardHit = opts.guardStates && opts.guardStates.has(existing.state);
    const preserveGuardedState = guardHit && opts.preserveGuardedState;

    // Primary ordering authority: reject a strictly-older report_seq so a stale
    // write that lands late (e.g. a delayed PostToolUse->running arriving after
    // Stop->idle_prompt) can never clobber a newer one. report_seq is a scaled
    // wall-clock stamp taken at hook entry (≈ event time), so the earlier event
    // keeps the lower seq even if it writes last. A non-finite or implausibly-
    // future on-disk seq is ignored (self-healing) so a corrupt file or clock
    // jump can't freeze writes. Equal seqs and un-seq'd writes fall through to
    // the guardStates backstop below.
    const staleReportSeq = (
      Number.isFinite(update.report_seq) &&
      Number.isFinite(existing.report_seq) &&
      existing.report_seq <= Date.now() * 1000 + REPORT_SEQ_FUTURE_SLACK &&
      update.report_seq < existing.report_seq
    );
    if (staleReportSeq && !preserveGuardedState) {
      return;
    }

    // Backstop: skip write if current on-disk state is protected. Catches
    // equal-seq ties and un-seq'd writes. Runs INSIDE the lock so the check sees
    // the same fresh read the merge will use — no TOCTOU window between guard
    // and write.
    if (guardHit && !opts.preserveGuardedState) {
      return;
    }

    const merged = {
      ...existing,
      ...update,
      ...(preserveGuardedState ? { state: existing.state } : {}),
      ...(staleReportSeq ? { report_seq: existing.report_seq } : {}),
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
  });
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
