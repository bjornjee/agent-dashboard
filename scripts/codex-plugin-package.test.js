#!/usr/bin/env node
'use strict';

const { describe, it } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const REPO = path.resolve(__dirname, '..');

function readJson(filePath) {
  return JSON.parse(fs.readFileSync(filePath, 'utf8'));
}

function skillNames(root) {
  return fs.readdirSync(root, { withFileTypes: true })
    .filter(entry => entry.isDirectory())
    .map(entry => entry.name)
    .sort();
}

function relativeFiles(root) {
  const files = [];

  function walk(current) {
    for (const entry of fs.readdirSync(current, { withFileTypes: true })) {
      const fullPath = path.join(current, entry.name);
      if (entry.isDirectory()) {
        walk(fullPath);
      } else if (entry.isFile()) {
        files.push(path.relative(root, fullPath));
      }
    }
  }

  walk(root);
  return files.sort();
}

describe('codex plugin package', () => {
  it('publishes a Codex marketplace entry that points at the Codex adapter', () => {
    const marketplace = readJson(path.join(REPO, '.agents/plugins/marketplace.json'));
    const plugin = marketplace.plugins.find(entry => entry.name === 'agent-dashboard');

    assert.ok(plugin, 'agent-dashboard plugin entry should exist');
    assert.equal(plugin.source.source, 'local');
    assert.equal(plugin.source.path, './adapters/codex');
    assert.deepEqual(plugin.policy, {
      installation: 'AVAILABLE',
      authentication: 'ON_INSTALL',
    });
    assert.equal(plugin.category, 'Engineering');
  });

  it('has a Codex manifest under the Codex adapter', () => {
    const manifest = readJson(path.join(REPO, 'adapters/codex/.codex-plugin/plugin.json'));

    assert.equal(manifest.name, 'agent-dashboard');
    assert.equal(manifest.version, '0.24.0');
    assert.equal(manifest.skills, './skills/');
    assert.equal(manifest.hooks, './hooks/plugin-hooks.json');
    assert.equal(manifest.interface.developerName, 'bjornjee');
    assert.equal(manifest.interface.category, 'Engineering');
  });

  it('uses plugin-local Codex hook commands, not Claude adapter or global hooks', () => {
    const hooks = readJson(path.join(REPO, 'adapters/codex/hooks/plugin-hooks.json'));
    const commands = Object.values(hooks.hooks)
      .flatMap(entries => entries)
      .flatMap(entry => entry.hooks)
      .map(hook => hook.command);

    assert.ok(commands.length > 0, 'expected hook commands');
    for (const command of commands) {
      assert.match(command, /\$\{PLUGIN_ROOT\}/);
      assert.doesNotMatch(command, /adapters\/claude-code/);
      assert.doesNotMatch(command, /\$HOME\/\.codex\/hooks/);
    }
  });

  it('packages the agent-dashboard skills inside the Codex plugin root', () => {
    const codexSkills = path.join(REPO, 'adapters/codex/skills');
    const claudeSkills = path.join(REPO, 'adapters/claude-code/skills');

    assert.deepEqual(skillNames(codexSkills), skillNames(claudeSkills));
    for (const relativeFile of relativeFiles(claudeSkills)) {
      assert.equal(
        fs.readFileSync(path.join(codexSkills, relativeFile), 'utf8'),
        fs.readFileSync(path.join(claudeSkills, relativeFile), 'utf8'),
        `${relativeFile} should match the Claude skill source`,
      );
    }
  });
});
