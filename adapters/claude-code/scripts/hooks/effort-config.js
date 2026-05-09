'use strict';

/**
 * Reads the [effort] section from the dashboard's settings.toml.
 *
 * The dashboard's Go side parses settings.toml with a real TOML library
 * (BurntSushi/toml). Hooks need the same two scalar string keys but run as
 * short-lived Node processes that shouldn't pull in a TOML dependency, so
 * we inline a tiny scalar reader: locate the [effort] section by header,
 * pluck plan/default values via per-key regex, and fall back to the same
 * defaults the Go side uses when a key is missing or malformed.
 */

const fs = require('fs');
const os = require('os');
const path = require('path');

const DEFAULTS = Object.freeze({ plan: 'max', default: 'high' });

function dashboardDir() {
  return process.env.AGENT_DASHBOARD_DIR
    || path.join(process.env.HOME || process.env.USERPROFILE || os.homedir(), '.agent-dashboard');
}

// extractSection slices the [effort] block out of a TOML document: from
// the line after the [effort] header to the line before the next [section]
// header (or end-of-file). Returns '' when [effort] is absent so the caller
// applies defaults.
function extractSection(content) {
  const headerMatch = content.match(/^\[effort\][^\S\n]*$/m);
  if (!headerMatch) return '';
  const after = content.slice(headerMatch.index + headerMatch[0].length);
  const nextHeader = after.match(/^\[[^\]\n]+\][^\S\n]*$/m);
  return nextHeader ? after.slice(0, nextHeader.index) : after;
}

function readEffortConfig() {
  try {
    const tomlPath = path.join(dashboardDir(), 'settings.toml');
    const content = fs.readFileSync(tomlPath, 'utf8');
    const section = extractSection(content);
    const planMatch = section.match(/^\s*plan\s*=\s*"([^"]*)"/m);
    const defaultMatch = section.match(/^\s*default\s*=\s*"([^"]*)"/m);
    return {
      plan: (planMatch && planMatch[1]) || DEFAULTS.plan,
      default: (defaultMatch && defaultMatch[1]) || DEFAULTS.default,
    };
  } catch {
    return { ...DEFAULTS };
  }
}

module.exports = { readEffortConfig };
