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

  it('returns "plan" for PreToolUse with ExitPlanMode', () => {
    assert.equal(resolveState('PreToolUse', 'ExitPlanMode'), 'plan');
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

  it('allows transition out of "pr" state when not pinned', () => {
    // Unpinned "pr" state (e.g. from an older hook version) is overridable
    // by subsequent tool activity.
    const existing = {
      target: 'main:1.0',
      state: 'pr',
      pr_url: 'https://github.com/bjornjee/agent-dashboard/pull/86',
      current_tool: '',
    };

    const { changed, update } = buildUpdate({
      input: {
        session_id: 'abc123',
        hook_event_name: 'PostToolUse',
        tool_name: 'Bash',
      },
      existing,
      target: 'main:1.0',
      tmuxPane: '%0',
      worktreeCwd: null,
    });

    assert.equal(changed, true, 'unpinned pr state should not stick');
    assert.equal(update.state, 'running');
  });

  it('allows transition out of "merged" state when not pinned', () => {
    const existing = {
      target: 'main:1.0',
      state: 'merged',
      pr_url: 'https://github.com/bjornjee/agent-dashboard/pull/86',
      current_tool: '',
    };

    const { changed, update } = buildUpdate({
      input: {
        session_id: 'abc123',
        hook_event_name: 'PostToolUse',
        tool_name: 'Read',
      },
      existing,
      target: 'main:1.0',
      tmuxPane: '%0',
      worktreeCwd: null,
    });

    assert.equal(changed, true, 'unpinned merged state should not stick');
    assert.equal(update.state, 'running');
  });

  it('preserves pinned_state "pr" even when state differs', () => {
    // Dashboard pins set pinned_state but state may have been overwritten.
    // The guard should check pinned_state too.
    const existing = {
      target: 'main:1.0',
      state: 'idle_prompt',
      pinned_state: 'pr',
      current_tool: '',
    };

    const { changed, update } = buildUpdate({
      input: {
        session_id: 'abc123',
        hook_event_name: 'PostToolUse',
        tool_name: 'Read',
      },
      existing,
      target: 'main:1.0',
      tmuxPane: '%0',
      worktreeCwd: null,
    });

    assert.equal(changed, false, 'should not overwrite when pinned_state is pr');
    assert.equal(update, null);
  });

  it('allows "permission" to override "pr" state', () => {
    const existing = {
      target: 'main:1.0',
      state: 'pr',
      current_tool: '',
    };

    const { changed, update } = buildUpdate({
      input: {
        session_id: 'abc123',
        hook_event_name: 'PermissionRequest',
        tool_name: 'Bash',
        permission_mode: 'default',
      },
      existing,
      target: 'main:1.0',
      tmuxPane: '%0',
      worktreeCwd: null,
    });

    assert.equal(changed, true);
    assert.equal(update.state, 'permission');
  });

  it('hook_blocked overrides state to "permission" on PreToolUse', () => {
    const existing = {
      target: 'main:1.0',
      state: 'running',
      current_tool: '',
      hook_blocked: 'Blocked: "git push --force" is a destructive command.',
    };

    const { changed, update } = buildUpdate({
      input: {
        session_id: 'abc123',
        hook_event_name: 'PreToolUse',
        tool_name: 'Bash',
      },
      existing,
      target: 'main:1.0',
      tmuxPane: '%0',
      worktreeCwd: null,
    });

    assert.equal(changed, true);
    assert.equal(update.state, 'permission');
    assert.equal(update.hook_blocked, '', 'hook_blocked should be cleared after consuming');
  });

  it('hook_blocked is NOT consumed on PermissionRequest (only PreToolUse)', () => {
    // PermissionRequest resolves to "permission" on its own — hook_blocked
    // should not be consumed here, only on PreToolUse.
    const existing = {
      target: 'main:1.0',
      state: 'running',
      current_tool: '',
      hook_blocked: 'Blocked: some reason',
    };

    const { update } = buildUpdate({
      input: {
        session_id: 'abc123',
        hook_event_name: 'PermissionRequest',
        tool_name: 'Bash',
        permission_mode: 'default',
      },
      existing,
      target: 'main:1.0',
      tmuxPane: '%0',
      worktreeCwd: null,
    });

    assert.equal(update.state, 'permission');
    assert.equal(update.hook_blocked, undefined, 'PermissionRequest should not consume hook_blocked');
  });

  it('hook_blocked is NOT consumed on PostToolUse (prevents race)', () => {
    const existing = {
      target: 'main:1.0',
      state: 'running',
      current_tool: 'Bash',
      hook_blocked: 'Blocked: "git push --force" is a destructive command.',
    };

    const { update } = buildUpdate({
      input: {
        session_id: 'abc123',
        hook_event_name: 'PostToolUse',
        tool_name: 'Bash',
      },
      existing,
      target: 'main:1.0',
      tmuxPane: '%0',
      worktreeCwd: null,
    });

    // PostToolUse should NOT clear hook_blocked — it must survive until next PreToolUse
    assert.equal(update.state, 'running');
    assert.equal(update.hook_blocked, undefined, 'PostToolUse should not clear hook_blocked');
  });

  it('hook_blocked is cleared on PreToolUse even when state is pr', () => {
    const existing = {
      target: 'main:1.0',
      state: 'pr',
      current_tool: '',
      hook_blocked: 'Blocked: some reason',
    };

    const { changed, update } = buildUpdate({
      input: {
        session_id: 'abc123',
        hook_event_name: 'PreToolUse',
        tool_name: 'Bash',
      },
      existing,
      target: 'main:1.0',
      tmuxPane: '%0',
      worktreeCwd: null,
    });

    // hook_blocked triggers permission state, which differs from pr — changed
    assert.equal(changed, true);
    assert.equal(update.hook_blocked, '', 'hook_blocked should be cleared even with PR state');
  });

  it('no hook_blocked means normal "running" state on PreToolUse', () => {
    const existing = {
      target: 'main:1.0',
      state: 'permission',
      current_tool: 'Bash',
    };

    const { update } = buildUpdate({
      input: {
        session_id: 'abc123',
        hook_event_name: 'PreToolUse',
        tool_name: 'Read',
      },
      existing,
      target: 'main:1.0',
      tmuxPane: '%0',
      worktreeCwd: null,
    });

    assert.equal(update.state, 'running');
    assert.equal(update.hook_blocked, undefined, 'should not set hook_blocked when absent');
  });

  it('PostToolUse skips when existing state is idle_prompt (stop-state guard)', () => {
    const existing = {
      target: 'main:1.0',
      state: 'idle_prompt',
      current_tool: '',
    };

    const { changed, update } = buildUpdate({
      input: {
        session_id: 'abc123',
        hook_event_name: 'PostToolUse',
        tool_name: 'Bash',
      },
      existing,
      target: 'main:1.0',
      tmuxPane: '%0',
      worktreeCwd: null,
    });

    assert.equal(changed, false, 'PostToolUse should not overwrite idle_prompt');
    assert.equal(update, null);
  });

  it('PostToolUse skips when existing state is plan (stop-state guard)', () => {
    const existing = {
      target: 'main:1.0',
      state: 'plan',
      current_tool: '',
    };

    const { changed, update } = buildUpdate({
      input: {
        session_id: 'abc123',
        hook_event_name: 'PostToolUse',
        tool_name: 'ExitPlanMode',
      },
      existing,
      target: 'main:1.0',
      tmuxPane: '%0',
      worktreeCwd: null,
    });

    assert.equal(changed, false, 'PostToolUse should not overwrite plan');
    assert.equal(update, null);
  });

  it('PostToolUse skips when existing state is done (stop-state guard)', () => {
    const existing = {
      target: 'main:1.0',
      state: 'done',
      current_tool: '',
    };

    const { changed, update } = buildUpdate({
      input: {
        session_id: 'abc123',
        hook_event_name: 'PostToolUse',
        tool_name: 'Read',
      },
      existing,
      target: 'main:1.0',
      tmuxPane: '%0',
      worktreeCwd: null,
    });

    assert.equal(changed, false, 'PostToolUse should not overwrite done');
    assert.equal(update, null);
  });

  it('PreToolUse is NOT blocked by stop states (next turn resumes running)', () => {
    const existing = {
      target: 'main:1.0',
      state: 'idle_prompt',
      current_tool: '',
    };

    const { changed, update } = buildUpdate({
      input: {
        session_id: 'abc123',
        hook_event_name: 'PreToolUse',
        tool_name: 'Bash',
      },
      existing,
      target: 'main:1.0',
      tmuxPane: '%0',
      worktreeCwd: null,
    });

    assert.equal(changed, true, 'PreToolUse should transition from idle_prompt to running');
    assert.equal(update.state, 'running');
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
