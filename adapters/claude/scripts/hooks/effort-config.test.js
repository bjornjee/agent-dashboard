#!/usr/bin/env node
'use strict';

const { describe, it, beforeEach, afterEach } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('fs');
const path = require('path');
const os = require('os');

const { readEffortConfig } = require('./effort-config');

let tmpDir;
let originalDir;

beforeEach(() => {
  tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'effort-config-test-'));
  originalDir = process.env.AGENT_DASHBOARD_DIR;
  process.env.AGENT_DASHBOARD_DIR = tmpDir;
});

afterEach(() => {
  if (originalDir === undefined) {
    delete process.env.AGENT_DASHBOARD_DIR;
  } else {
    process.env.AGENT_DASHBOARD_DIR = originalDir;
  }
  fs.rmSync(tmpDir, { recursive: true, force: true });
});

describe('readEffortConfig', () => {
  it('returns defaults when settings.toml is missing', () => {
    const cfg = readEffortConfig();
    assert.deepEqual(cfg, { plan: 'max', default: 'high' });
  });

  it('returns defaults when [effort] section is absent', () => {
    fs.writeFileSync(path.join(tmpDir, 'settings.toml'),
      '[banner]\nshow_mascot = true\n');
    const cfg = readEffortConfig();
    assert.deepEqual(cfg, { plan: 'max', default: 'high' });
  });

  it('returns the user-set plan and default from [effort]', () => {
    fs.writeFileSync(path.join(tmpDir, 'settings.toml'),
      '[effort]\nplan = "high"\ndefault = "medium"\n');
    const cfg = readEffortConfig();
    assert.equal(cfg.plan, 'high');
    assert.equal(cfg.default, 'medium');
  });

  it('falls back per-key when [effort] is partial', () => {
    fs.writeFileSync(path.join(tmpDir, 'settings.toml'),
      '[effort]\nplan = "low"\n');
    const cfg = readEffortConfig();
    assert.equal(cfg.plan, 'low');
    assert.equal(cfg.default, 'high');
  });

  it('isolates [effort] keys from later sections', () => {
    // A `default` key in a later section must not be picked up by [effort].
    fs.writeFileSync(path.join(tmpDir, 'settings.toml'),
      '[effort]\nplan = "max"\n\n[other]\ndefault = "medium"\n');
    const cfg = readEffortConfig();
    assert.equal(cfg.plan, 'max');
    assert.equal(cfg.default, 'high');
  });

  it('returns defaults when [effort] keys appear before the section header', () => {
    // Stray `default = "medium"` in a different section must not leak in.
    fs.writeFileSync(path.join(tmpDir, 'settings.toml'),
      '[banner]\ndefault = "medium"\n\n[effort]\nplan = "high"\n');
    const cfg = readEffortConfig();
    assert.equal(cfg.plan, 'high');
    assert.equal(cfg.default, 'high');
  });

  it('returns defaults on read error', () => {
    // Write a directory at settings.toml path → readFileSync throws.
    fs.mkdirSync(path.join(tmpDir, 'settings.toml'));
    const cfg = readEffortConfig();
    assert.deepEqual(cfg, { plan: 'max', default: 'high' });
  });
});
