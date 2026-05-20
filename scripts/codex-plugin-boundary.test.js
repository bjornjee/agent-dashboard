#!/usr/bin/env node
'use strict';

const { describe, it } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const REPO_DIR = path.resolve(__dirname, '..');

function readJson(relPath) {
  return JSON.parse(fs.readFileSync(path.join(REPO_DIR, relPath), 'utf8'));
}

describe('Codex plugin boundary', () => {
  it('publishes a Codex marketplace entry for the Codex adapter, not the Claude adapter', () => {
    const marketplace = readJson('.agents/plugins/marketplace.json');
    const plugin = marketplace.plugins.find(entry => entry.name === 'agent-dashboard');

    assert.ok(plugin, 'agent-dashboard marketplace entry must exist');
    assert.equal(plugin.source.source, 'local');
    assert.equal(plugin.source.path, './adapters/codex');
  });

  it('has a Codex plugin manifest that points at Codex-safe hooks', () => {
    const manifest = readJson('adapters/codex/.codex-plugin/plugin.json');

    assert.equal(manifest.name, 'agent-dashboard');
    assert.equal(manifest.hooks, './hooks/hooks.json');
  });
});
