'use strict';

const fs = require('fs');

const TAIL_SIZE = 32 * 1024;

/**
 * Returns true if the parent transcript JSONL has any tool_use whose id has
 * no matching tool_result in a later user entry. A "pending parent tool" is a
 * deterministic signal that the parent agent is still actively working — far
 * more reliable than tmux pane heuristics.
 *
 * Mirrors the tail-scan pattern in internal/conversation/conversation.go's
 * LastPendingBlockingTool: read the last 32 KB, discard the partial first line
 * after seeking, JSON.parse per line. Sidechain (subagent) entries do not
 * appear in the parent JSONL, so no isSidechain filter is needed.
 *
 * Orphan tool_result entries (whose tool_use scrolled past the tail) do not
 * mark anything as pending — the corresponding tool_use is simply not seen.
 *
 * @param {string|null|undefined} transcriptPath - path to the parent .jsonl
 * @returns {boolean}
 */
function hasPendingParentToolUse(transcriptPath) {
  if (!transcriptPath) return false;

  let fd;
  try {
    fd = fs.openSync(transcriptPath, 'r');
  } catch {
    return false;
  }

  try {
    const stat = fs.fstatSync(fd);
    if (stat.size === 0) return false;

    const readLen = Math.min(stat.size, TAIL_SIZE);
    const buf = Buffer.alloc(readLen);
    fs.readSync(fd, buf, 0, readLen, stat.size - readLen);

    let text = buf.toString('utf8');
    // If we seeked mid-file, drop the partial first line.
    if (stat.size > TAIL_SIZE) {
      const nl = text.indexOf('\n');
      if (nl === -1) return false;
      text = text.slice(nl + 1);
    }

    const pending = new Set();

    for (const line of text.split('\n')) {
      if (!line) continue;
      let entry;
      try {
        entry = JSON.parse(line);
      } catch {
        continue;
      }

      if (entry.type === 'assistant') {
        const content = entry.message && entry.message.content;
        if (!Array.isArray(content)) continue;
        for (const block of content) {
          if (block && block.type === 'tool_use' && block.id) {
            pending.add(block.id);
          }
        }
      } else if (entry.type === 'user') {
        const content = entry.message && entry.message.content;
        if (!Array.isArray(content)) continue;
        for (const block of content) {
          if (block && block.type === 'tool_result' && block.tool_use_id) {
            pending.delete(block.tool_use_id);
          }
        }
      }
    }

    return pending.size > 0;
  } finally {
    try { fs.closeSync(fd); } catch { /* ignore */ }
  }
}

module.exports = { hasPendingParentToolUse };
