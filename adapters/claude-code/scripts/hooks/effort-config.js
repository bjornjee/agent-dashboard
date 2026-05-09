'use strict';

/**
 * Reads the [effort] section from the dashboard's settings.toml. Defaults
 * (plan="max", default="high") match the Go side's DefaultSettings() so a
 * missing file or section yields identical behavior on both sides.
 */

const fs = require('fs');
const os = require('os');
const path = require('path');

const pluginRoot = process.env.CLAUDE_PLUGIN_ROOT || path.resolve(__dirname, '..', '..');
const toml = require(path.join(pluginRoot, 'packages', 'toml-lite'));

const DEFAULTS = Object.freeze({ plan: 'max', default: 'high' });

function dashboardDir() {
  return process.env.AGENT_DASHBOARD_DIR
    || path.join(process.env.HOME || process.env.USERPROFILE || os.homedir(), '.agent-dashboard');
}

function readEffortConfig() {
  try {
    const tomlPath = path.join(dashboardDir(), 'settings.toml');
    const content = fs.readFileSync(tomlPath, 'utf8');
    const section = toml.parse(content).effort || {};
    return {
      plan: typeof section.plan === 'string' && section.plan ? section.plan : DEFAULTS.plan,
      default: typeof section.default === 'string' && section.default ? section.default : DEFAULTS.default,
    };
  } catch {
    return { ...DEFAULTS };
  }
}

module.exports = { readEffortConfig };
