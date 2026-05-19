#!/usr/bin/env node
/**
 * Mermaid diagram extractor hook (Claude Code Stop event).
 *
 * Sole writer of ~/.agent-dashboard/diagrams/<session-id>/<unix-ts>-<hash>.mmd.
 * The dashboard is a pure reader — there is no fallback scanner.
 *
 * For each ```mermaid``` fenced block in the last assistant message:
 *   1. Derive a human-readable title from the prose preceding the fence
 *      (or use a `%% title:` line / frontmatter if the agent supplied one).
 *   2. Prepend `%% title: <derived>\n` to the source if missing.
 *   3. Compute hash8 = sha256(source)[:8], stat-then-write to dedup.
 *   4. Atomic write via tmp + rename.
 *
 * After processing, write {diagram_count, diagram_latest_ts} into the
 * agent state JSON to nudge the dashboard's fsnotify watcher.
 */

'use strict';

const path = require('path');
const fs = require('fs');
const os = require('os');
const crypto = require('crypto');

const pluginRoot = process.env.CLAUDE_PLUGIN_ROOT || path.resolve(__dirname, '..', '..');
const { writeState } = require(path.join(pluginRoot, 'packages', 'agent-state'));

const STATE_DIR = process.env.AGENT_DASHBOARD_DIR
  || path.join(process.env.HOME || process.env.USERPROFILE || '/tmp', '.agent-dashboard');

const FENCE_RE = /```mermaid\r?\n([\s\S]*?)\r?\n```/g;

// Restrict session IDs to a safe character set so they can't escape the
// diagrams root directory via path traversal or absolute paths.
const SAFE_SESSION_ID_RE = /^[a-zA-Z0-9_-]{1,128}$/;
function isValidSessionId(s) {
  return typeof s === 'string' && SAFE_SESSION_ID_RE.test(s);
}

const KNOWN_TYPES = [
  'flowchart', 'sequenceDiagram', 'stateDiagram-v2', 'stateDiagram',
  'classDiagram', 'erDiagram', 'gantt', 'pie', 'journey', 'gitGraph',
  'mindmap', 'timeline', 'quadrantChart', 'requirementDiagram', 'C4Context',
];

const GENERIC_TITLES = new Set([
  '', 'here', "here's", 'here is', 'below', 'this diagram', 'as follows',
  'this', 'see', 'example', 'diagram',
]);

function findSessionId() {
  const sessDir = path.join(os.homedir(), '.claude', 'sessions');
  let pid = process.ppid;
  for (let i = 0; i < 3 && pid > 1; i++) {
    try {
      const file = path.join(sessDir, `${pid}.json`);
      const data = JSON.parse(fs.readFileSync(file, 'utf8'));
      if (data.sessionId) return data.sessionId;
    } catch { /* not found, try parent */ }
    try {
      const { spawnSync } = require('child_process');
      const r = spawnSync('ps', ['-o', 'ppid=', '-p', String(pid)], { timeout: 1000 });
      pid = parseInt(r.stdout.toString().trim(), 10);
      if (isNaN(pid)) break;
    } catch { break; }
  }
  return null;
}

function hash8(src) {
  return crypto.createHash('sha256').update(src).digest('hex').slice(0, 8);
}

function detectType(src) {
  const lines = src.split('\n');
  let i = 0;
  if (lines[i] && lines[i].trim() === '---') {
    i++;
    while (i < lines.length && lines[i].trim() !== '---') i++;
    if (i < lines.length) i++;
  }
  for (; i < lines.length; i++) {
    const line = lines[i].trim();
    if (!line || line.startsWith('%%')) continue;
    for (const t of KNOWN_TYPES) {
      if (line === t || line.startsWith(t + ' ') || line.startsWith(t + '\t')) return t;
    }
    return 'diagram';
  }
  return 'diagram';
}

function hasExplicitTitle(src) {
  const lines = src.split('\n');
  if (lines[0] && lines[0].trim() === '---') {
    for (let i = 1; i < lines.length; i++) {
      if (lines[i].trim() === '---') break;
      if (/^title:\s*\S/.test(lines[i].trim())) return true;
    }
  }
  for (const line of lines) {
    const t = line.trim();
    if (!t) continue;
    if (/^%%\s*title:\s*\S/.test(t)) return true;
    if (!t.startsWith('%%') && t !== '---') break;
  }
  return false;
}

/**
 * Walk backwards from `fenceOffset` in `text`, find the last sentence,
 * strip markdown, return a clean title or '' if it's too generic.
 */
function derivePrecedingTitle(text, fenceOffset) {
  let end = fenceOffset;
  // Skip back over the ```mermaid line itself + the trailing newline.
  // (The caller passes the offset of the opening ``` so go back to find prose.)
  // Trim trailing whitespace before the fence.
  while (end > 0 && /\s/.test(text[end - 1])) end--;

  // Walk backwards to find a sentence boundary.
  let start = end;
  let sawBlank = false;
  while (start > 0) {
    const ch = text[start - 1];
    // Stop at sentence terminators.
    if (ch === '.' || ch === '!' || ch === '?') break;
    // Stop at a blank line (two consecutive newlines).
    if (ch === '\n' && start > 1 && /[\n]/.test(text[start - 2])) {
      sawBlank = true;
      break;
    }
    start--;
    // Cap walk-back length.
    if (end - start > 400) break;
  }

  let snippet = text.slice(start, end);
  // Strip markdown.
  snippet = snippet
    .replace(/```[\s\S]*?```/g, ' ') // any other code fences
    .replace(/`([^`]+)`/g, '$1')
    .replace(/\*\*([^*]+)\*\*/g, '$1')
    .replace(/\*([^*]+)\*/g, '$1')
    .replace(/_([^_]+)_/g, '$1')
    .replace(/^\s*#{1,6}\s+/gm, '')
    .replace(/^\s*[-*]\s+/gm, '')
    .replace(/^\s*\d+\.\s+/gm, '')
    .replace(/\[([^\]]+)\]\([^)]*\)/g, '$1');

  // Take the last line of the snippet (closest to the fence).
  const parts = snippet.split('\n').map(s => s.trim()).filter(Boolean);
  if (parts.length === 0) return '';
  let last = parts[parts.length - 1];

  // Trim trailing punctuation/colons.
  last = last.replace(/[\s:.;,]+$/, '').replace(/\s+/g, ' ').trim();

  // Truncate to 80 chars with ellipsis.
  if (last.length > 80) last = last.slice(0, 79).trim() + '\u2026';

  if (GENERIC_TITLES.has(last.toLowerCase())) return '';
  if (last.length < 3) return '';
  return last;
}

/**
 * Inject a `%% title: ...` line at the top of the source if not present.
 * Respects frontmatter — the title goes after the closing `---`.
 */
function injectTitle(src, title) {
  if (!title) return src;
  const lines = src.split('\n');
  if (lines[0] && lines[0].trim() === '---') {
    let i = 1;
    while (i < lines.length && lines[i].trim() !== '---') i++;
    if (i < lines.length) {
      // Insert after the closing ---.
      const before = lines.slice(0, i + 1).join('\n');
      const after = lines.slice(i + 1).join('\n');
      return before + '\n%% title: ' + title + '\n' + after;
    }
  }
  return '%% title: ' + title + '\n' + src;
}

function atomicWrite(filePath, content) {
  fs.mkdirSync(path.dirname(filePath), { recursive: true });
  const tmp = filePath + `.tmp.${process.pid}.${crypto.randomBytes(4).toString('hex')}`;
  try {
    fs.writeFileSync(tmp, content);
    fs.renameSync(tmp, filePath);
  } catch (err) {
    try { fs.unlinkSync(tmp); } catch { /* ignore */ }
    throw err;
  }
}

function existsForHash(dir, hash) {
  try {
    const files = fs.readdirSync(dir);
    return files.some(f => f.endsWith(`-${hash}.mmd`));
  } catch {
    return false;
  }
}

/**
 * Extract mermaid diagrams from a single text blob and return the
 * list of new {hash, source} that were freshly written.
 *
 * Pure I/O wrapper exposed for testability.
 */
function extractFromText(text, sessionDir, nowSec, seqStart) {
  const written = [];
  let seq = seqStart;
  let m;
  FENCE_RE.lastIndex = 0;
  while ((m = FENCE_RE.exec(text)) !== null) {
    const rawSource = m[1];
    const fenceStart = m.index;

    let source = rawSource;
    if (!hasExplicitTitle(source)) {
      let title = derivePrecedingTitle(text, fenceStart);
      if (!title) {
        const type = detectType(source);
        title = `${type} #${seq}`;
      }
      source = injectTitle(source, title);
    }

    const h = hash8(source);
    if (existsForHash(sessionDir, h)) {
      seq++;
      continue;
    }

    const filename = `${nowSec}-${h}.mmd`;
    const filepath = path.join(sessionDir, filename);
    try {
      atomicWrite(filepath, source);
      written.push({ hash: h, path: filepath });
    } catch { /* ignore */ }
    seq++;
  }
  return written;
}

function countMmdFiles(dir) {
  try {
    return fs.readdirSync(dir).filter(f => /^\d+-[0-9a-f]{8}\.mmd$/.test(f)).length;
  } catch {
    return 0;
  }
}

function run(input) {
  const sessionId = input.session_id || findSessionId();
  if (!sessionId || !isValidSessionId(sessionId)) return;

  // Resolve the assistant text. Prefer last_assistant_message; fall back
  // to reading the transcript file if Claude Code provided one.
  let text = '';
  if (typeof input.last_assistant_message === 'string') {
    text = input.last_assistant_message;
  } else if (input.transcript_path) {
    try {
      const lines = fs.readFileSync(input.transcript_path, 'utf8').trim().split('\n');
      // Walk back to find the last assistant text content.
      for (let i = lines.length - 1; i >= 0; i--) {
        try {
          const ev = JSON.parse(lines[i]);
          if (ev.role !== 'assistant' && ev.type !== 'assistant') continue;
          const content = ev.message ? ev.message.content : ev.content;
          if (Array.isArray(content)) {
            text = content
              .filter(b => b && b.type === 'text' && typeof b.text === 'string')
              .map(b => b.text)
              .join('\n');
          } else if (typeof content === 'string') {
            text = content;
          }
          if (text) break;
        } catch { /* skip line */ }
      }
    } catch { /* ignore */ }
  }
  if (!text || text.indexOf('```mermaid') === -1) return;

  const sessionDir = path.join(STATE_DIR, 'diagrams', sessionId);
  const nowSec = Math.floor(Date.now() / 1000);
  const existingCount = countMmdFiles(sessionDir);
  const written = extractFromText(text, sessionDir, nowSec, existingCount + 1);

  if (written.length > 0) {
    const total = countMmdFiles(sessionDir);
    try {
      writeState(sessionId, {
        diagram_count: total,
        diagram_latest_ts: nowSec,
      });
    } catch { /* ignore */ }
  }
}

if (require.main === module) {
  const MAX_STDIN = 4 * 1024 * 1024;
  let data = '';
  process.stdin.setEncoding('utf8');
  process.stdin.on('data', chunk => {
    if (data.length < MAX_STDIN) data += chunk.substring(0, MAX_STDIN - data.length);
  });
  process.stdin.on('end', () => {
    try {
      const input = data.trim() ? JSON.parse(data) : {};
      run(input);
    } catch { /* silent */ }
  });
}

module.exports = {
  hash8,
  detectType,
  hasExplicitTitle,
  derivePrecedingTitle,
  injectTitle,
  extractFromText,
  isValidSessionId,
};
