#!/usr/bin/env node
'use strict';

const { describe, it } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const REPO_DIR = path.resolve(__dirname, '..');
const installScript = fs.readFileSync(path.join(REPO_DIR, 'install.sh'), 'utf8');

describe('codex installer support', () => {

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

  it('does not print shell-profile append commands', () => {
    assert.doesNotMatch(installScript, />> ~\/\.(zshrc|bashrc)/);
    assert.doesNotMatch(installScript, /cat > "\$settings_file"/);
  });

  it('can install codex hooks during curl-based release installs', () => {
    assert.match(installScript, /archive\/refs\/tags\/v\$\{VERSION\}\.tar\.gz/);
    assert.match(installScript, /adapters\/codex\/hooks\/hooks\.json/);
  });
});

// Upgrade-aware sync of the codex adapter. Prior behavior used
// copy_*_if_missing which silently skipped on every re-install, leaving users
// with stale hook bundles. The new path stamps a version+hash file in the
// installed bundle so version changes drive an automatic replace, and gates
// global ~/.codex/hooks.json edits behind a known-shipped-hash allowlist.
describe('codex adapter upgrade sync', () => {
  it('exposes a --sync-adapters flag that skips the binary install', () => {
    assert.match(installScript, /--sync-adapters/);
    // The flag must short-circuit binary download/build — only adapters touched.
    assert.match(installScript, /SYNC_ADAPTERS_ONLY=true/);
  });

  it('stamps the installed bundle with a .agent-dashboard-installed file', () => {
    assert.match(installScript, /\.agent-dashboard-installed/);
  });

  it('detects bundle drift via sha256 over the source hooks dir', () => {
    // Bundle hash must drive the upgrade decision, not just version string —
    // dev checkouts may rev contents without bumping version.
    assert.match(installScript, /bundle_hash\(\)/);
    // shasum/sha256sum portability mirrors the existing checksum path.
    assert.match(installScript, /sha256sum|shasum -a 256/);
  });

  it('replaces the bundle dir when the stamped hash differs from the source hash', () => {
    // The decision function must compare installed stamp vs computed source
    // hash. Replace path uses rm -rf followed by cp -R (not cp -n / copy_if_missing).
    assert.match(installScript, /sync_codex_bundle\(\)/);
    assert.match(installScript, /rm -rf "\$CODEX_HOOKS_DIR"/);
  });

  it('reads a .shipped-hashes manifest to identify known-shipped hooks.json versions', () => {
    assert.match(installScript, /\.shipped-hashes/);
    // Helper that returns 0/1 based on hash allowlist membership.
    assert.match(installScript, /is_shipped_hooks_json\(\)/);
  });

  it('prompts y/N before overwriting a user-modified hooks.json on a TTY', () => {
    // TTY check (input redirect detection) — using `[ -t 0 ]` or equivalent.
    assert.match(installScript, /\[ -t 0 \]/);
    // The prompt itself must actually appear in the script.
    assert.match(installScript, /Overwrite.*\?.*\[y\/N\]/i);
  });

  it('skips overwriting when stdin is not a TTY (curl-pipe install case)', () => {
    // The non-TTY branch must NOT default to "yes"; it must skip and warn.
    assert.match(installScript, /stdin is not a TTY|not a TTY|non-interactive/i);
  });

  it('ships an initial allowlist file with at least the current hooks.json hash', () => {
    const shipped = fs.readFileSync(
      path.join(REPO_DIR, 'adapters/codex/hooks/.shipped-hashes'),
      'utf8'
    );
    // One sha256 (64 hex) per line, comments tolerated.
    const lines = shipped
      .split('\n')
      .map(l => l.trim())
      .filter(l => l && !l.startsWith('#'));
    assert.ok(lines.length >= 1, 'expected at least one shipped hash');
    for (const l of lines) {
      assert.match(l, /^[a-f0-9]{64}$/, `not a sha256 hex digest: ${l}`);
    }
    // The current hooks.json hash must be in the allowlist (a release without
    // its own entry would treat itself as "user modified" — a footgun).
    const crypto = require('node:crypto');
    const hooksJson = fs.readFileSync(
      path.join(REPO_DIR, 'adapters/codex/hooks/hooks.json')
    );
    const currentHash = crypto.createHash('sha256').update(hooksJson).digest('hex');
    assert.ok(
      lines.includes(currentHash),
      `current hooks.json hash ${currentHash} must appear in .shipped-hashes`
    );
  });

  it('exposes a Makefile sync-adapters target wired to install.sh --sync-adapters', () => {
    const makefile = fs.readFileSync(path.join(REPO_DIR, 'Makefile'), 'utf8');
    assert.match(makefile, /^sync-adapters:/m);
    assert.match(makefile, /install\.sh --sync-adapters/);
  });
});
