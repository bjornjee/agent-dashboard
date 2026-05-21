'use strict';

/**
 * Tiny TOML reader covering the subset agent-dashboard's hooks need to
 * read settings.toml without pulling in a runtime dependency. Returns a
 * plain object: { section: { key: value, ... }, ... }. Top-level keys
 * before any section header live on the returned object directly.
 *
 * Supported:
 *   - [section] headers
 *   - key = "string"  (basic strings; supports \\ \" \n \t escapes)
 *   - key = 'literal' (literal strings; no escapes)
 *   - key = 123 / -7  (signed integers)
 *   - key = true | false
 *   - # comments (line and trailing, # inside strings is preserved)
 *
 * Unsupported lines (arrays, inline tables, dotted keys, multi-line
 * strings, datetimes, floats, etc.) are silently skipped — callers must
 * not rely on this parser for fields it doesn't claim to handle. The Go
 * side uses a real TOML library; this package exists only for hooks that
 * need a small subset.
 */

function parse(text) {
  const result = {};
  let current = result;
  const lines = text.split(/\r?\n/);
  for (const raw of lines) {
    const line = stripComment(raw).trim();
    if (!line) continue;
    if (line[0] === '[') {
      const end = line.indexOf(']');
      if (end < 0) continue;
      const name = line.slice(1, end).trim();
      if (!name) continue;
      current = result[name] || (result[name] = {});
      continue;
    }
    const eq = line.indexOf('=');
    if (eq < 0) continue;
    const key = line.slice(0, eq).trim();
    if (!key) continue;
    const value = parseValue(line.slice(eq + 1).trim());
    if (value !== undefined) current[key] = value;
  }
  return result;
}

// Strip a `# ...` trailing comment, but treat # inside a string literal
// as data. Single- and double-quoted strings both protect their contents;
// inside a basic string a `\` escapes the next character.
function stripComment(line) {
  let inStr = null;
  for (let i = 0; i < line.length; i++) {
    const c = line[i];
    if (inStr) {
      if (c === '\\' && inStr === '"') { i++; continue; }
      if (c === inStr) inStr = null;
      continue;
    }
    if (c === '"' || c === "'") inStr = c;
    else if (c === '#') return line.slice(0, i);
  }
  return line;
}

function parseValue(raw) {
  if (raw === 'true') return true;
  if (raw === 'false') return false;
  if (/^-?\d+$/.test(raw)) return Number(raw);
  if (raw[0] === '"') {
    const end = findStringEnd(raw, '"', true);
    if (end < 0) return undefined;
    return unescapeBasic(raw.slice(1, end));
  }
  if (raw[0] === "'") {
    const end = findStringEnd(raw, "'", false);
    if (end < 0) return undefined;
    return raw.slice(1, end);
  }
  return undefined;
}

function findStringEnd(s, quote, allowEscapes) {
  for (let i = 1; i < s.length; i++) {
    if (allowEscapes && s[i] === '\\') { i++; continue; }
    if (s[i] === quote) return i;
  }
  return -1;
}

function unescapeBasic(s) {
  return s.replace(/\\(["\\nt])/g, (_, c) => {
    if (c === 'n') return '\n';
    if (c === 't') return '\t';
    return c;
  });
}

module.exports = { parse };
