#!/usr/bin/env node
'use strict';

const { describe, it } = require('node:test');
const assert = require('node:assert/strict');
const path = require('node:path');
const fs = require('node:fs');

const REPO_DIR = path.resolve(__dirname, '..');

describe('install version resolution', () => {
  it('version from plugin.json can be extracted by node', () => {
    const pluginJson = JSON.parse(
      fs.readFileSync(
        path.join(REPO_DIR, 'adapters/claude-code/.claude-plugin/plugin.json'),
        'utf8'
      )
    );

    assert.ok(pluginJson.version, 'plugin.json should have a version field');
    assert.match(
      pluginJson.version,
      /^\d+\.\d+\.\d+/,
      'version should be semver'
    );
  });
});
