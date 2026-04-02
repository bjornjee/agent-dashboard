#!/usr/bin/env node
'use strict';

const { describe, it, beforeEach, afterEach } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('fs');
const path = require('path');
const os = require('os');

// Import the module under test
const { resolveState, buildUpdate } = require('./agent-state-fast');

// Import shared packages
const pluginRoot = path.resolve(__dirname, '..', '..');
const { readAgentState, writeState } = require(path.join(pluginRoot, 'packages', 'agent-state'));

let tmpDir;
let agentsDir;

beforeEach(() => {
  tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'fast-hook-test-'));
  agentsDir = path.join(tmpDir, 'agents');
});

afterEach(() => {
  fs.rmSync(tmpDir, { recursive: true, force: true });
});

describe('resolveState', () => {
  it('returns "permission" for PermissionRequest', () => {
    assert.equal(resolveState('PermissionRequest', 'Bash'), 'permission');
  });

  it('returns "running" for PreToolUse with normal tools', () => {
    assert.equal(resolveState('PreToolUse', 'Bash'), 'running');
    assert.equal(resolveState('PreToolUse', 'Read'), 'running');
    assert.equal(resolveState('PreToolUse', 'Edit'), 'running');
  });

  it('returns "question" for PreToolUse with AskUserQuestion', () => {
    assert.equal(resolveState('PreToolUse', 'AskUserQuestion'), 'question');
  });

  it('returns "running" for PostToolUse', () => {
    assert.equal(resolveState('PostToolUse', 'Bash'), 'running');
  });

  it('returns "running" for unknown events', () => {
    assert.equal(resolveState('SomeOther', 'Bash'), 'running');
  });
});

describe('fast hook state updates (per-agent files)', () => {
  it('PermissionRequest sets state to input with current_tool', () => {
    writeState('main:1.0', {
      target: 'main:1.0',
      state: 'running',
      current_tool: '',
      permission_mode: 'default',
    }, agentsDir);

    // Simulate PermissionRequest update
    const existing = readAgentState('main:1.0', agentsDir);
    const update = {
      ...existing,
      state: 'input',
      current_tool: 'Edit',
      permission_mode: 'acceptEdits',
      last_hook_event: 'PermissionRequest',
    };
    writeState('main:1.0', update, agentsDir);

    const result = readAgentState('main:1.0', agentsDir);
    assert.equal(result.state, 'input');
    assert.equal(result.current_tool, 'Edit');
    assert.equal(result.permission_mode, 'acceptEdits');
    assert.equal(result.last_hook_event, 'PermissionRequest');
  });

  it('PostToolUse sets state to running and clears current_tool', () => {
    writeState('main:1.0', {
      target: 'main:1.0',
      state: 'input',
      current_tool: 'Edit',
      permission_mode: 'acceptEdits',
      last_hook_event: 'PermissionRequest',
    }, agentsDir);

    const existing = readAgentState('main:1.0', agentsDir);
    const update = {
      ...existing,
      state: 'running',
      current_tool: '',
      last_hook_event: 'PostToolUse',
    };
    writeState('main:1.0', update, agentsDir);

    const result = readAgentState('main:1.0', agentsDir);
    assert.equal(result.state, 'running');
    assert.equal(result.current_tool, '');
    assert.equal(result.last_hook_event, 'PostToolUse');
  });

  it('PreToolUse sets current_tool but keeps state running', () => {
    writeState('main:1.0', {
      target: 'main:1.0',
      state: 'running',
      current_tool: '',
    }, agentsDir);

    const existing = readAgentState('main:1.0', agentsDir);
    const update = {
      ...existing,
      state: 'running',
      current_tool: 'Bash',
      last_hook_event: 'PreToolUse',
    };
    writeState('main:1.0', update, agentsDir);

    const result = readAgentState('main:1.0', agentsDir);
    assert.equal(result.state, 'running');
    assert.equal(result.current_tool, 'Bash');
  });

  it('buildUpdate does not include cwd or branch in update', () => {
    const existing = {
      target: 'main:1.0',
      state: 'running',
      current_tool: 'Bash',
    };

    const { update } = buildUpdate({
      input: {
        session_id: 'abc123',
        hook_event_name: 'PostToolUse',
        tool_name: 'Bash',
        cwd: '/Users/bjornjee/Code/bjornjee/skills',
      },
      existing,
      target: 'main:1.0',
      tmuxPane: '%0',
      worktreeCwd: null,
    });

    assert.equal(update.cwd, undefined, 'fast hook should not set cwd');
    assert.equal(update.branch, undefined, 'fast hook should not set branch');
  });

  it('sets worktree_cwd when Bash cd targets a worktree path', () => {
    const existing = {
      target: 'main:1.0',
      state: 'running',
      current_tool: 'Bash',
    };

    const { changed, update } = buildUpdate({
      input: {
        session_id: 'abc123',
        hook_event_name: 'PostToolUse',
        tool_name: 'Bash',
        cwd: '/Users/bjornjee/Code/bjornjee/skills',
      },
      existing,
      target: 'main:1.0',
      tmuxPane: '%0',
      worktreeCwd: '/Users/bjornjee/Code/bjornjee/worktrees/skills/my-feature',
    });

    assert.equal(changed, true);
    assert.equal(update.worktree_cwd, '/Users/bjornjee/Code/bjornjee/worktrees/skills/my-feature');
  });

  it('does not set worktree_cwd for non-worktree cd', () => {
    const existing = {
      target: 'main:1.0',
      state: 'running',
      current_tool: 'Bash',
    };

    const { update } = buildUpdate({
      input: {
        session_id: 'abc123',
        hook_event_name: 'PostToolUse',
        tool_name: 'Bash',
        cwd: '/Users/bjornjee/Code/bjornjee/skills',
      },
      existing,
      target: 'main:1.0',
      tmuxPane: '%0',
      worktreeCwd: null,
    });

    assert.equal(update.worktree_cwd, undefined);
  });

  it('preserves existing worktree_cwd when no new worktree cd detected', () => {
    const existing = {
      target: 'main:1.0',
      state: 'running',
      current_tool: 'Read',
      worktree_cwd: '/Users/bjornjee/Code/bjornjee/worktrees/skills/my-feature',
    };

    const { update } = buildUpdate({
      input: {
        session_id: 'abc123',
        hook_event_name: 'PostToolUse',
        tool_name: 'Read',
        cwd: '/Users/bjornjee/Code/bjornjee/skills',
      },
      existing,
      target: 'main:1.0',
      tmuxPane: '%0',
      worktreeCwd: null,
    });

    // worktree_cwd should NOT be in the update — it's preserved via merge in writeState
    assert.equal(update.worktree_cwd, undefined);
  });

  it('preserves existing fields not updated by fast hook', () => {
    writeState('main:1.0', {
      target: 'main:1.0',
      state: 'running',
      branch: 'feat/something',
      model: 'claude-opus-4-6',
      session_id: 'abc123',
      files_changed: ['file1.go', 'file2.go'],
    }, agentsDir);

    // Fast hook only updates state, current_tool, permission_mode, last_hook_event
    const existing = readAgentState('main:1.0', agentsDir);
    const update = {
      ...existing,
      state: 'input',
      current_tool: 'Bash',
      permission_mode: 'default',
      last_hook_event: 'PermissionRequest',
    };
    writeState('main:1.0', update, agentsDir);

    const result = readAgentState('main:1.0', agentsDir);
    // Fast fields updated
    assert.equal(result.state, 'input');
    assert.equal(result.current_tool, 'Bash');
    // Existing fields preserved
    assert.equal(result.branch, 'feat/something');
    assert.equal(result.model, 'claude-opus-4-6');
    assert.equal(result.session_id, 'abc123');
    assert.deepEqual(result.files_changed, ['file1.go', 'file2.go']);
  });
});
