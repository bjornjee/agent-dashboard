#!/usr/bin/env node
'use strict';

const { describe, it } = require('node:test');
const assert = require('node:assert/strict');
const path = require('node:path');
const fs = require('node:fs');

const REPO_DIR = path.resolve(__dirname, '..');

describe('install version resolution', () => {
  it('should resolve version from plugin.json as primary source', () => {
    const installScript = fs.readFileSync(
      path.join(REPO_DIR, 'install.sh'),
      'utf8'
    );

    assert.ok(
      installScript.includes('plugin.json'),
      'install.sh should reference plugin.json for version resolution'
    );
  });

  it('should prefer plugin.json version over VERSION file', () => {
    const installScript = fs.readFileSync(
      path.join(REPO_DIR, 'install.sh'),
      'utf8'
    );

    const versionLine = installScript
      .split('\n')
      .find(l => l.includes('version=') && l.includes('plugin.json'));

    assert.ok(
      versionLine,
      'install.sh should have a version= line that references plugin.json'
    );

    // plugin.json should appear before VERSION fallback (primary source)
    const pluginJsonIdx = versionLine.indexOf('plugin.json');
    const versionFileIdx = versionLine.indexOf('VERSION');
    if (versionFileIdx !== -1) {
      assert.ok(
        pluginJsonIdx < versionFileIdx,
        'plugin.json should be checked before VERSION file (primary source)'
      );
    }
  });

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
