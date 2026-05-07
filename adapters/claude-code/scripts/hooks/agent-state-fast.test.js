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
    assert.equal(resolveState('PermissionRequest', 'Bash', ''), 'permission');
  });

  it('returns "running" for PreToolUse with normal tools', () => {
    assert.equal(resolveState('PreToolUse', 'Bash', ''), 'running');
    assert.equal(resolveState('PreToolUse', 'Read', ''), 'running');
    assert.equal(resolveState('PreToolUse', 'Edit', ''), 'running');
  });

  it('returns "question" for PreToolUse with AskUserQuestion', () => {
    assert.equal(resolveState('PreToolUse', 'AskUserQuestion', ''), 'question');
  });

  it('returns "plan" for PreToolUse with ExitPlanMode', () => {
    assert.equal(resolveState('PreToolUse', 'ExitPlanMode'), 'plan');
  });

  it('returns "running" for PostToolUse', () => {
    assert.equal(resolveState('PostToolUse', 'Bash', ''), 'running');
  });

  it('returns "running" for unknown events', () => {
    assert.equal(resolveState('SomeOther', 'Bash', ''), 'running');
  });

  it('does NOT return "plan" while permission_mode is plan but tool is busy', () => {
    // The "plan" badge means "plan ready for review" — only ExitPlanMode triggers it.
    // While the agent is researching/asking inside plan mode, state flows from the tool.
    assert.equal(resolveState('PreToolUse', 'Bash', 'plan'), 'running');
    assert.equal(resolveState('PostToolUse', 'Edit', 'plan'), 'running');
    assert.equal(resolveState('PreToolUse', 'AskUserQuestion', 'plan'), 'question');
  });

  it('PermissionRequest returns "permission" regardless of permission_mode', () => {
    assert.equal(resolveState('PermissionRequest', 'Edit', 'plan'), 'permission');
  });

  it('returns "plan" for PermissionRequest with ExitPlanMode (non-bypass plan mode)', () => {
    // In permission_mode='plan' (not bypassPermissions), Claude Code fires
    // PermissionRequest for ExitPlanMode. The tool-specific signal must win
    // over the generic permission fallback so the dashboard groups the agent
    // under PLAN, not BLOCKED.
    assert.equal(resolveState('PermissionRequest', 'ExitPlanMode', 'plan'), 'plan');
  });

  it('returns "question" for PermissionRequest with AskUserQuestion', () => {
    // Same race as ExitPlanMode: PermissionRequest can swallow AskUserQuestion
    // before its PreToolUse branch runs. Tool-specific classification wins.
    assert.equal(resolveState('PermissionRequest', 'AskUserQuestion', ''), 'question');
  });

  it('returns "running" when permission_mode is bypassPermissions', () => {
    assert.equal(resolveState('PreToolUse', 'Bash', 'bypassPermissions'), 'running');
  });

  it('returns "plan" for PreToolUse Agent with subagent_type=Plan', () => {
    // Orchestrator delegates planning to the Plan subagent — main agent
    // permission_mode stays bypassPermissions, so we detect via the tool call.
    assert.equal(
      resolveState('PreToolUse', 'Agent', '', { subagent_type: 'Plan' }),
      'plan'
    );
  });

  it('returns "running" for PreToolUse Agent with non-Plan subagent_type', () => {
    assert.equal(
      resolveState('PreToolUse', 'Agent', '', { subagent_type: 'Explore' }),
      'running'
    );
    assert.equal(
      resolveState('PreToolUse', 'Agent', '', { subagent_type: 'general-purpose' }),
      'running'
    );
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

  it('sets worktree_cwd when input.cwd is a worktree path', () => {
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
        cwd: '/Users/bjornjee/Code/bjornjee/worktrees/skills/my-feature',
      },
      existing,
      target: 'main:1.0',
      tmuxPane: '%0',
    });

    assert.equal(changed, true);
    assert.equal(update.worktree_cwd, '/Users/bjornjee/Code/bjornjee/worktrees/skills/my-feature');
  });

  it('does not set worktree_cwd when input.cwd is not a worktree path', () => {
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
    });

    assert.equal(update.worktree_cwd, undefined);
  });

  it('preserves existing worktree_cwd when current input.cwd is the source repo', () => {
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
    });

    // worktree_cwd should NOT be in the update — it's preserved via merge in writeState
    assert.equal(update.worktree_cwd, undefined);
  });

  it('detects worktree from input.cwd even when Bash command used a relative cd (regression)', () => {
    // Regression for the dashboard-shows-wrong-branch bug: previously the hook
    // parsed Bash commands for `cd /abs/path && ...` and rejected relative paths.
    // Now we read input.cwd directly, so any cd form (relative, $(...), pushd)
    // resolves correctly because Claude Code reports the live cwd.
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
        tool_input: { command: 'cd ../worktrees/skills/my-feature && pwd' },
        cwd: '/Users/bjornjee/Code/bjornjee/worktrees/skills/my-feature',
      },
      existing,
      target: 'main:1.0',
      tmuxPane: '%0',
    });

    assert.equal(changed, true);
    assert.equal(update.worktree_cwd, '/Users/bjornjee/Code/bjornjee/worktrees/skills/my-feature');
  });

  it('does not overwrite an already-stamped worktree_cwd (static dir semantic)', () => {
    // Once worktree_cwd is set it should be treated as the agent's static home
    // for the rest of the session — diff viewer, PR creation, and cleanup all
    // trust this dir. Even if input.cwd reports a different worktree path
    // (e.g. agent cd'd into a different worktree), don't update.
    const existing = {
      target: 'main:1.0',
      state: 'running',
      current_tool: 'Bash',
      worktree_cwd: '/Users/bjornjee/Code/bjornjee/worktrees/skills/feature-a',
    };

    const { update } = buildUpdate({
      input: {
        session_id: 'abc123',
        hook_event_name: 'PostToolUse',
        tool_name: 'Bash',
        cwd: '/Users/bjornjee/Code/bjornjee/worktrees/skills/feature-b',
      },
      existing,
      target: 'main:1.0',
      tmuxPane: '%0',
    });

    assert.equal(update.worktree_cwd, undefined,
      'existing worktree_cwd must not be overwritten when agent visits another worktree path');
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

  it('buildUpdate keeps state "running" while permission_mode is plan but tool is busy', () => {
    // While planning, permission_mode='plan' is captured as a field but does not
    // drive state — state still reflects the active tool. Only ExitPlanMode flips
    // state to "plan" (meaning: plan ready for review).
    const existing = {
      target: 'main:1.0',
      state: 'running',
      current_tool: '',
    };

    const { changed, update } = buildUpdate({
      input: {
        session_id: 'abc123',
        hook_event_name: 'PreToolUse',
        tool_name: 'Read',
        permission_mode: 'plan',
      },
      existing,
      target: 'main:1.0',
      tmuxPane: '%0',
      worktreeCwd: null,
    });

    assert.equal(changed, true);
    assert.equal(update.state, 'running');
    assert.equal(update.permission_mode, 'plan',
      'permission_mode field must still be captured for display');
  });

  it('buildUpdate sets state to "plan" on PreToolUse ExitPlanMode', () => {
    // ExitPlanMode is the canonical "plan ready for review" signal.
    const existing = {
      target: 'main:1.0',
      state: 'running',
      current_tool: '',
    };

    const { changed, update } = buildUpdate({
      input: {
        session_id: 'abc123',
        hook_event_name: 'PreToolUse',
        tool_name: 'ExitPlanMode',
        permission_mode: 'plan',
      },
      existing,
      target: 'main:1.0',
      tmuxPane: '%0',
      worktreeCwd: null,
    });

    assert.equal(changed, true);
    assert.equal(update.state, 'plan');
  });

  it('PostToolUse does not overwrite existing "plan" state', () => {
    // STOP_STATES guard: once state=plan (set by ExitPlanMode), a stale
    // PostToolUse from any tool must not clobber it.
    const existing = {
      target: 'main:1.0',
      state: 'plan',
      current_tool: '',
      permission_mode: 'plan',
    };

    const { changed, update } = buildUpdate({
      input: {
        session_id: 'abc123',
        hook_event_name: 'PostToolUse',
        tool_name: 'Read',
        permission_mode: 'plan',
      },
      existing,
      target: 'main:1.0',
      tmuxPane: '%0',
      worktreeCwd: null,
    });

    assert.equal(changed, false, 'PostToolUse must not overwrite plan');
    assert.equal(update, null);
  });

  it('PreToolUse Agent+Plan stamps delegated_plan_tool_use_id and sets state=plan', () => {
    const existing = {
      target: 'main:1.0',
      state: 'running',
      current_tool: '',
    };

    const { changed, update } = buildUpdate({
      input: {
        session_id: 'abc123',
        hook_event_name: 'PreToolUse',
        tool_name: 'Agent',
        tool_input: { subagent_type: 'Plan', prompt: '...' },
        tool_use_id: 'toolu_01ABC',
      },
      existing,
      target: 'main:1.0',
      tmuxPane: '%0',
      worktreeCwd: null,
    });

    assert.equal(changed, true);
    assert.equal(update.state, 'plan');
    assert.equal(update.delegated_plan_tool_use_id, 'toolu_01ABC');
  });

  it('PreToolUse Agent with non-Plan subagent_type does NOT stamp the id', () => {
    const existing = {
      target: 'main:1.0',
      state: 'running',
      current_tool: '',
    };

    const { update } = buildUpdate({
      input: {
        session_id: 'abc123',
        hook_event_name: 'PreToolUse',
        tool_name: 'Agent',
        tool_input: { subagent_type: 'Explore', prompt: '...' },
        tool_use_id: 'toolu_01XYZ',
      },
      existing,
      target: 'main:1.0',
      tmuxPane: '%0',
      worktreeCwd: null,
    });

    assert.equal(update.state, 'running');
    assert.equal(update.delegated_plan_tool_use_id, undefined);
  });

  it('PostToolUse Agent+Plan keeps state=plan via STOP_STATES guard', () => {
    // After Plan PreToolUse stamped the id and set state=plan, the matching
    // PostToolUse must not clobber state — STOP_STATES already includes plan.
    const existing = {
      target: 'main:1.0',
      state: 'plan',
      current_tool: '',
      delegated_plan_tool_use_id: 'toolu_01ABC',
    };

    const { changed, update } = buildUpdate({
      input: {
        session_id: 'abc123',
        hook_event_name: 'PostToolUse',
        tool_name: 'Agent',
        tool_input: { subagent_type: 'Plan' },
        tool_use_id: 'toolu_01ABC',
      },
      existing,
      target: 'main:1.0',
      tmuxPane: '%0',
      worktreeCwd: null,
    });

    assert.equal(changed, false, 'PostToolUse must not overwrite plan state');
    assert.equal(update, null);
  });

  it('PreToolUse Bash after delegated plan clears the id and transitions to running', () => {
    // User approved the plan and the agent resumed work — next non-Agent
    // PreToolUse must clear the pointer so the dashboard stops showing the plan.
    const existing = {
      target: 'main:1.0',
      state: 'plan',
      current_tool: '',
      delegated_plan_tool_use_id: 'toolu_01ABC',
    };

    const { changed, update } = buildUpdate({
      input: {
        session_id: 'abc123',
        hook_event_name: 'PreToolUse',
        tool_name: 'Bash',
        tool_input: { command: 'go test ./...' },
      },
      existing,
      target: 'main:1.0',
      tmuxPane: '%0',
      worktreeCwd: null,
    });

    assert.equal(changed, true);
    assert.equal(update.state, 'running');
    assert.equal(update.delegated_plan_tool_use_id, '', 'pointer must be cleared on transition out of plan');
  });

  it('PreToolUse Agent+Plan with missing tool_use_id sets state=plan but does not stamp id', () => {
    // Defensive: if a future CC version renames or omits tool_use_id from
    // hook stdin, state still flips to plan (consumers fall back to
    // ReadPlanSlug) and we don't write a bogus empty pointer.
    const existing = {
      target: 'main:1.0',
      state: 'running',
      current_tool: '',
    };

    const { changed, update } = buildUpdate({
      input: {
        session_id: 'abc123',
        hook_event_name: 'PreToolUse',
        tool_name: 'Agent',
        tool_input: { subagent_type: 'Plan' },
        // tool_use_id intentionally omitted
      },
      existing,
      target: 'main:1.0',
      tmuxPane: '%0',
      worktreeCwd: null,
    });

    assert.equal(changed, true);
    assert.equal(update.state, 'plan');
    assert.equal(update.delegated_plan_tool_use_id, undefined);
  });

  it('does not clear delegated_plan_tool_use_id while state stays plan', () => {
    // A subsequent PreToolUse for ExitPlanMode (e.g., dual-source scenario)
    // keeps state=plan, so the existing pointer must be preserved.
    const existing = {
      target: 'main:1.0',
      state: 'plan',
      current_tool: '',
      delegated_plan_tool_use_id: 'toolu_01ABC',
    };

    const { update } = buildUpdate({
      input: {
        session_id: 'abc123',
        hook_event_name: 'PreToolUse',
        tool_name: 'ExitPlanMode',
      },
      existing,
      target: 'main:1.0',
      tmuxPane: '%0',
      worktreeCwd: null,
    });

    assert.equal(update.state, 'plan');
    assert.equal(update.delegated_plan_tool_use_id, undefined,
      'should not write a clear when state stays plan');
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
