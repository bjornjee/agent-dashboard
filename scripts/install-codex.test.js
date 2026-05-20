#!/usr/bin/env node
'use strict';

const { describe, it } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const REPO_DIR = path.resolve(__dirname, '..');

describe('codex installer support', () => {
  const installScript = fs.readFileSync(path.join(REPO_DIR, 'install.sh'), 'utf8');

  it('installs the codex hook bundle into the durable codex hooks directory', () => {
    assert.match(installScript, /install_codex_hooks\(\)/);
    assert.match(installScript, /CODEX_HOOKS_DIR="\$CODEX_DIR\/hooks\/agent-dashboard"/);
    assert.match(installScript, /adapters\/codex\/hooks/);
  });

  it('updates the global codex hooks file instead of relying on plugin cache hooks', () => {
    assert.match(installScript, /CODEX_DIR="\$\{CODEX_HOME:-\$HOME\/\.codex\}"/);
    assert.match(installScript, /CODEX_HOOKS_FILE="\$CODEX_DIR\/hooks\.json"/);
    assert.doesNotMatch(installScript, /\.codex\/plugins\/cache/);
  });

  it('uses copy-if-missing actions for codex hooks', () => {
    assert.match(installScript, /copy_dir_if_missing "\$CODEX_HOOKS_SOURCE" "\$CODEX_HOOKS_DIR"/);
    assert.match(installScript, /copy_file_if_missing "\$CODEX_HOOKS_SOURCE\/hooks\.json" "\$CODEX_HOOKS_FILE"/);
    assert.doesNotMatch(installScript, /merge_codex_hooks_config/);
    assert.doesNotMatch(installScript, /node <<['"]?NODE/);
    assert.doesNotMatch(installScript, /writeFileSync/);
  });

  it('does not print shell-profile append commands', () => {
    assert.doesNotMatch(installScript, />> ~\/\.(zshrc|bashrc)/);
    assert.doesNotMatch(installScript, /cat > "\$settings_file"/);
  });

  it('can install codex hooks during curl-based release installs', () => {
    assert.match(installScript, /archive\/refs\/tags\/v\$\{VERSION\}\.tar\.gz/);
    assert.match(installScript, /adapters\/codex\/hooks\/hooks\.json/);
  });
});
