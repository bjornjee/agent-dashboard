#!/usr/bin/env node
'use strict';

// Integration tests for `install.sh --sync-adapters`. The grep-based assertions
// in install-codex.test.js verify that the right pieces are written in the
// script; these run the script end-to-end against a temp CODEX_HOME to verify
// the actual upgrade behavior — fresh install, idempotent re-run, version-stamp
// drift, and user-modified hooks.json handling.

const { describe, it, before, after } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const crypto = require('node:crypto');
const { execFileSync } = require('node:child_process');

const REPO_DIR = path.resolve(__dirname, '..');
const INSTALL_SH = path.join(REPO_DIR, 'install.sh');
const HOOKS_SRC = path.join(REPO_DIR, 'adapters/codex/hooks');

function sha256(buf) {
  return crypto.createHash('sha256').update(buf).digest('hex');
}

function runSync({ codexHome, stateDir, input }) {
  // --sync-adapters must skip binary install, settings bootstrap, and PATH
  // checks — exit cleanly with adapters synced.
  return execFileSync('sh', [INSTALL_SH, '--sync-adapters'], {
    env: {
      ...process.env,
      CODEX_HOME: codexHome,
      AGENT_DASHBOARD_DIR: stateDir,
      // Prevent any interactive prompts from blocking the test runner.
      AGENT_DASHBOARD_NONINTERACTIVE: input == null ? '1' : '',
    },
    input: input == null ? '' : input,
    encoding: 'utf8',
    stdio: ['pipe', 'pipe', 'pipe'],
  });
}

describe('install.sh --sync-adapters (integration)', () => {
  let workRoot;
  let codexHome;
  let stateDir;
  let hooksDir;
  let hooksJsonPath;

  before(() => {
    workRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'ad-sync-'));
  });

  after(() => {
    fs.rmSync(workRoot, { recursive: true, force: true });
  });

  function freshEnv(name) {
    codexHome = path.join(workRoot, name, '.codex');
    stateDir = path.join(workRoot, name, '.agent-dashboard');
    hooksDir = path.join(codexHome, 'hooks', 'agent-dashboard');
    hooksJsonPath = path.join(codexHome, 'hooks.json');
  }

  it('fresh install: copies the bundle and writes a version stamp', () => {
    freshEnv('fresh');
    runSync({ codexHome, stateDir });

    assert.ok(fs.existsSync(hooksDir), 'bundle dir should be created');
    assert.ok(fs.existsSync(hooksJsonPath), 'hooks.json should be created');

    const stamp = fs.readFileSync(
      path.join(hooksDir, '.agent-dashboard-installed'),
      'utf8'
    );
    assert.match(stamp, /^[a-f0-9]{64}$/m, 'stamp must record a sha256 hash');
  });

  it('idempotent: re-running with the same source is a no-op for the bundle', () => {
    freshEnv('idempotent');
    runSync({ codexHome, stateDir });
    const stampBefore = fs.readFileSync(
      path.join(hooksDir, '.agent-dashboard-installed'),
      'utf8'
    );
    const mtimeBefore = fs.statSync(
      path.join(hooksDir, 'agent-state-fast.sh')
    ).mtimeMs;

    // Bump time forward so a re-copy would be detectable on filesystems
    // with low mtime resolution.
    execFileSync('sleep', ['0.05']);

    runSync({ codexHome, stateDir });

    const stampAfter = fs.readFileSync(
      path.join(hooksDir, '.agent-dashboard-installed'),
      'utf8'
    );
    const mtimeAfter = fs.statSync(
      path.join(hooksDir, 'agent-state-fast.sh')
    ).mtimeMs;

    assert.equal(stampBefore, stampAfter);
    assert.equal(
      mtimeBefore,
      mtimeAfter,
      're-sync must not re-copy when hash matches'
    );
  });

  it('drift: replaces the bundle when the stamp hash no longer matches', () => {
    freshEnv('drift');
    runSync({ codexHome, stateDir });

    // Tamper with the installed bundle by editing a hook file — also rewrite
    // the stamp to a bogus hash to simulate "installed by an older release".
    const stampPath = path.join(hooksDir, '.agent-dashboard-installed');
    fs.writeFileSync(stampPath, '0'.repeat(64) + '\n');
    fs.writeFileSync(
      path.join(hooksDir, 'agent-state-fast.sh'),
      '#!/bin/sh\necho tampered\n'
    );

    runSync({ codexHome, stateDir });

    const stampAfter = fs
      .readFileSync(stampPath, 'utf8')
      .trim();
    assert.notEqual(stampAfter, '0'.repeat(64), 'stamp should be rewritten');
    assert.match(stampAfter, /^[a-f0-9]{64}$/);

    const tampered = fs.readFileSync(
      path.join(hooksDir, 'agent-state-fast.sh'),
      'utf8'
    );
    assert.doesNotMatch(
      tampered,
      /echo tampered/,
      'tampered hook script must be replaced on drift'
    );
  });

  it('hooks.json: replaces when installed hash is in .shipped-hashes', () => {
    freshEnv('hooks-shipped');
    runSync({ codexHome, stateDir });

    // Simulate "user is on the shipped previous version" by writing an entry
    // to the allowlist that matches a sentinel hooks.json content. Confirms
    // the upgrade path replaces a known-shipped file even when its content
    // differs from the latest source.
    const sentinel = '{"hooks":{"SessionStart":[]}}\n';
    fs.writeFileSync(hooksJsonPath, sentinel);
    const sentinelHash = sha256(Buffer.from(sentinel));

    // Append sentinel hash to a TEST-LOCAL shipped-hashes via env-overridable
    // path. The installer must respect AGENT_DASHBOARD_SHIPPED_HASHES if set.
    const testShipped = path.join(workRoot, 'shipped-hashes-test.txt');
    const existing = fs.readFileSync(
      path.join(HOOKS_SRC, '.shipped-hashes'),
      'utf8'
    );
    fs.writeFileSync(testShipped, existing + sentinelHash + '\n');

    execFileSync('sh', [INSTALL_SH, '--sync-adapters'], {
      env: {
        ...process.env,
        CODEX_HOME: codexHome,
        AGENT_DASHBOARD_DIR: stateDir,
        AGENT_DASHBOARD_SHIPPED_HASHES: testShipped,
        AGENT_DASHBOARD_NONINTERACTIVE: '1',
      },
      encoding: 'utf8',
      stdio: ['pipe', 'pipe', 'pipe'],
    });

    const sourceHooksJson = fs.readFileSync(
      path.join(HOOKS_SRC, 'hooks.json'),
      'utf8'
    );
    assert.equal(
      fs.readFileSync(hooksJsonPath, 'utf8'),
      sourceHooksJson,
      'shipped-hash hooks.json must be upgraded to current source'
    );
  });

  it('hooks.json: leaves user-modified file alone in non-interactive mode', () => {
    freshEnv('hooks-user-modified');
    runSync({ codexHome, stateDir });

    const userContent = '{"hooks":{"SessionStart":[{"matcher":"*"}]},"// user":"hand-edited"}\n';
    fs.writeFileSync(hooksJsonPath, userContent);

    const out = runSync({ codexHome, stateDir });

    assert.equal(
      fs.readFileSync(hooksJsonPath, 'utf8'),
      userContent,
      'user-modified hooks.json must not be overwritten in non-interactive mode'
    );
    assert.match(
      out,
      /hooks\.json.*modified|locally modified|user.modified/i,
      'installer should warn that hooks.json was left in place'
    );
  });

  it('hooks.json: piping "y" through stdin overwrites in interactive mode', () => {
    freshEnv('hooks-prompt-yes');
    runSync({ codexHome, stateDir });

    fs.writeFileSync(hooksJsonPath, '{"// user":"edited"}\n');

    // The installer's TTY check is [ -t 0 ]; piping input makes that false,
    // so the script must take the non-interactive path. To exercise the
    // "yes" branch deterministically without a PTY, the installer should
    // accept AGENT_DASHBOARD_ASSUME_YES=1 as an opt-in override.
    execFileSync('sh', [INSTALL_SH, '--sync-adapters'], {
      env: {
        ...process.env,
        CODEX_HOME: codexHome,
        AGENT_DASHBOARD_DIR: stateDir,
        AGENT_DASHBOARD_ASSUME_YES: '1',
      },
      encoding: 'utf8',
      stdio: ['pipe', 'pipe', 'pipe'],
    });

    const sourceHooksJson = fs.readFileSync(
      path.join(HOOKS_SRC, 'hooks.json'),
      'utf8'
    );
    assert.equal(
      fs.readFileSync(hooksJsonPath, 'utf8'),
      sourceHooksJson,
      'AGENT_DASHBOARD_ASSUME_YES must overwrite a user-modified hooks.json'
    );
  });
});
