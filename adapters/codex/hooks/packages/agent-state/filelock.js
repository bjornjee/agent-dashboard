'use strict';

const fs = require('fs');

// Lock protocol parameters MUST match internal/state/filelock.go so Go and
// Node writers on the same agent file rendezvous on the same sidecar.
const LOCK_SUFFIX = '.lock';
const LOCK_STALE_MS = 5000;
const LOCK_RETRY_MS = 5;
const LOCK_MAX_RETRIES = 400; // ~2s budget

function sleepSync(ms) {
  // Atomics.wait is the only built-in synchronous sleep in Node — no spawn,
  // no busy-wait. writeState is synchronous so we cannot await/setTimeout.
  const sab = new SharedArrayBuffer(4);
  const view = new Int32Array(sab);
  Atomics.wait(view, 0, 0, ms);
}

/**
 * Run fn while holding an exclusive sidecar lock on filePath.
 * Created with O_CREAT|O_EXCL — the same atomic primitive used by Go's
 * withFileLock, so cross-language writers coordinate.
 *
 * A lock older than LOCK_STALE_MS is treated as abandoned (writer crashed)
 * and removed. After LOCK_MAX_RETRIES the call returns the lock-acquire
 * error to the caller rather than blocking indefinitely.
 *
 * @param {string} filePath - the data file being protected
 * @param {function} fn - synchronous critical section
 * @returns whatever fn returns
 */
function withFileLock(filePath, fn) {
  const lockPath = filePath + LOCK_SUFFIX;
  let fd = -1;
  for (let attempt = 0; attempt < LOCK_MAX_RETRIES; attempt++) {
    try {
      fd = fs.openSync(lockPath, 'wx', 0o600);
      break;
    } catch (err) {
      if (err.code !== 'EEXIST') {
        throw err;
      }
      try {
        const stat = fs.statSync(lockPath);
        if (Date.now() - stat.mtimeMs > LOCK_STALE_MS) {
          try { fs.unlinkSync(lockPath); } catch { /* race with another acquirer */ }
          continue;
        }
      } catch { /* lock vanished between EEXIST and stat — retry */ }
      sleepSync(LOCK_RETRY_MS);
    }
  }
  if (fd === -1) {
    throw new Error(`acquire lock for ${filePath}: timed out`);
  }
  try {
    try { fs.writeSync(fd, String(process.pid)); } catch { /* best-effort */ }
    fs.closeSync(fd);
    return fn();
  } finally {
    try { fs.unlinkSync(lockPath); } catch { /* already removed */ }
  }
}

module.exports = { withFileLock };
