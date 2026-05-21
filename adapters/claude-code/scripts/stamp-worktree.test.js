#!/usr/bin/env node
'use strict';

const { describe, it, beforeEach, afterEach } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('fs');
const path = require('path');
const os = require('os');
const { spawnSync } = require('child_process');

const SCRIPT = path.join(__dirname, 'stamp-worktree.js');

let tmpDir;
let agentsDir;

beforeEach(() => {
  tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'stamp-worktree-test-'));
  agentsDir = path.join(tmpDir, 'agents');
});

afterEach(() => {
  fs.rmSync(tmpDir, { recursive: true, force: true });
});

function run(args, env = {}) {
  const baseEnv = { ...process.env };
  delete baseEnv.CLAUDE_PLUGIN_ROOT;
  delete baseEnv.PLUGIN_ROOT;
  delete baseEnv.CLAUDE_SESSION_ID;
  delete baseEnv.CLAUDE_CODE_SESSION_ID;
  return spawnSync('node', [SCRIPT, ...args], {
    env: { ...baseEnv, AGENT_DASHBOARD_DIR: tmpDir, ...env },
    encoding: 'utf8',
  });
}

describe('stamp-worktree.js', () => {
  it('writes worktree_cwd when agent state is empty', () => {
    const sessionId = 'sess-1';
    const wt = path.join(tmpDir, 'wt');
    fs.mkdirSync(wt);
    const r = run([wt], { CLAUDE_SESSION_ID: sessionId });
    assert.equal(r.status, 0, r.stderr);
    const written = JSON.parse(fs.readFileSync(path.join(agentsDir, sessionId + '.json'), 'utf8'));
    assert.equal(written.worktree_cwd, wt);
  });

  it('preserves existing worktree_cwd (first-write-wins)', () => {
    const sessionId = 'sess-2';
    fs.mkdirSync(agentsDir, { recursive: true });
    fs.writeFileSync(path.join(agentsDir, sessionId + '.json'),
      JSON.stringify({ worktree_cwd: '/already/set' }));
    const r = run(['/some/other/path'], { CLAUDE_SESSION_ID: sessionId });
    assert.equal(r.status, 0, r.stderr);
    const written = JSON.parse(fs.readFileSync(path.join(agentsDir, sessionId + '.json'), 'utf8'));
    assert.equal(written.worktree_cwd, '/already/set');
  });

  it('exits 2 when worktree path arg is missing', () => {
    const r = run([], { CLAUDE_SESSION_ID: 'sess-3' });
    assert.equal(r.status, 2);
  });

  it('exits 2 when session id is missing (no env, no arg)', () => {
    const r = run(['/some/path']);
    assert.equal(r.status, 2);
  });

  it('resolves relative worktree paths to absolute', () => {
    const sessionId = 'sess-4';
    const r = run(['./foo'], { CLAUDE_SESSION_ID: sessionId });
    assert.equal(r.status, 0, r.stderr);
    const written = JSON.parse(fs.readFileSync(path.join(agentsDir, sessionId + '.json'), 'utf8'));
    assert.equal(written.worktree_cwd, path.resolve('./foo'));
  });

  it('accepts session id as positional arg when env unset', () => {
    const sessionId = 'sess-5';
    const r = run(['/tmp/x', sessionId]);
    assert.equal(r.status, 0, r.stderr);
    const written = JSON.parse(fs.readFileSync(path.join(agentsDir, sessionId + '.json'), 'utf8'));
    assert.equal(written.worktree_cwd, '/tmp/x');
  });

  it('reads CLAUDE_CODE_SESSION_ID env var (the name Claude Code actually exports)', () => {
    const sessionId = 'sess-6';
    const r = run(['/tmp/y'], { CLAUDE_CODE_SESSION_ID: sessionId });
    assert.equal(r.status, 0, r.stderr);
    const written = JSON.parse(fs.readFileSync(path.join(agentsDir, sessionId + '.json'), 'utf8'));
    assert.equal(written.worktree_cwd, '/tmp/y');
  });
});
