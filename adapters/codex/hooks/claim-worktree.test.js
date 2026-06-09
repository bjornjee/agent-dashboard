'use strict';

const { describe, it } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('fs');
const path = require('path');
const os = require('os');

const { readAllState, writeState } = require('./packages/agent-state');
const { claimWorktreeForPane, MARKER_NAME } = require('./packages/worktree-reconcile');

function tempDir() {
  return fs.mkdtempSync(path.join(os.tmpdir(), 'claim-wt-'));
}

function makeLinkedWorktree() {
  const source = tempDir();
  fs.mkdirSync(path.join(source, '.git', 'worktrees'), { recursive: true });
  const wtRoot = tempDir();
  const perWorktreeDir = path.join(source, '.git', 'worktrees', path.basename(wtRoot));
  fs.mkdirSync(perWorktreeDir, { recursive: true });
  fs.writeFileSync(path.join(wtRoot, '.git'), `gitdir: ${perWorktreeDir}\n`);
  return { source, wtRoot, perWorktreeDir };
}

function recordingSpawn(handlers) {
  const spawnSync = (cmd, args) => {
    const key = `${cmd} ${args.join(' ')}`;
    const stdout = handlers[key];
    if (stdout === undefined) return { status: 1, stdout: '' };
    return { status: 0, stdout };
  };
  return spawnSync;
}

describe('claim-worktree', () => {
  it('claims the linked worktree marker and patches the current pane state', () => {
    const stateDir = tempDir();
    const agentsDir = path.join(stateDir, 'agents');
    fs.mkdirSync(agentsDir, { recursive: true });
    const agentPath = path.join(agentsDir, 'sess-claim.json');
    fs.writeFileSync(agentPath, JSON.stringify({
      session_id: 'sess-claim',
      target: 'main:1.0',
      tmux_pane_id: '%12',
      cwd: '/repo/main',
      branch: 'main',
      state: 'running',
    }));

    const { source, wtRoot, perWorktreeDir } = makeLinkedWorktree();
    const spawnSync = recordingSpawn({
      [`git -C ${wtRoot} worktree list --porcelain`]:
        `worktree ${source}\nbranch refs/heads/main\n\nworktree ${wtRoot}\nbranch refs/heads/feat/claim\n\n`,
      [`git -C ${wtRoot} rev-parse --absolute-git-dir`]: `${perWorktreeDir}\n`,
    });

    const update = claimWorktreeForPane({
      worktreePath: wtRoot,
      paneId: '%12',
      stateDir,
      readAllState,
      writeState,
    }, {
      spawnSync,
    });

    assert.deepEqual(update, { worktree_cwd: wtRoot, branch: 'feat/claim' });
    assert.equal(fs.readFileSync(path.join(perWorktreeDir, MARKER_NAME), 'utf8'), 'sess-claim');

    const patched = JSON.parse(fs.readFileSync(agentPath, 'utf8'));
    assert.equal(patched.worktree_cwd, wtRoot);
    assert.equal(patched.branch, 'feat/claim');
  });

  it('reaps stale markers before claiming', () => {
    const stateDir = tempDir();
    const agentsDir = path.join(stateDir, 'agents');
    fs.mkdirSync(agentsDir, { recursive: true });
    fs.writeFileSync(path.join(agentsDir, 'sess-claim.json'), JSON.stringify({
      session_id: 'sess-claim',
      target: 'main:1.0',
      tmux_pane_id: '%12',
      state: 'running',
    }));

    const { source, wtRoot, perWorktreeDir } = makeLinkedWorktree();
    fs.writeFileSync(path.join(perWorktreeDir, MARKER_NAME), 'ghost-session');
    const spawnSync = recordingSpawn({
      [`git -C ${wtRoot} worktree list --porcelain`]:
        `worktree ${source}\nbranch refs/heads/main\n\nworktree ${wtRoot}\nbranch refs/heads/feat/claim\n\n`,
      [`git -C ${wtRoot} rev-parse --absolute-git-dir`]: `${perWorktreeDir}\n`,
    });

    claimWorktreeForPane({
      worktreePath: wtRoot,
      paneId: '%12',
      stateDir,
      readAllState,
      writeState,
    }, {
      spawnSync,
    });

    assert.equal(fs.readFileSync(path.join(perWorktreeDir, MARKER_NAME), 'utf8'), 'sess-claim');
  });

  it('overwrites a misaligned marker whose owner pinned a different worktree', () => {
    const stateDir = tempDir();
    const agentsDir = path.join(stateDir, 'agents');
    fs.mkdirSync(agentsDir, { recursive: true });
    fs.writeFileSync(path.join(agentsDir, 'sess-me.json'), JSON.stringify({
      session_id: 'sess-me',
      target: 'main:1.0',
      tmux_pane_id: '%91',
      cwd: '/tmp/gone',
      worktree_cwd: '/tmp/gone',
      branch: 'HEAD',
      state: 'running',
    }));
    fs.writeFileSync(path.join(agentsDir, 'sess-other.json'), JSON.stringify({
      session_id: 'sess-other',
      target: 'main:2.0',
      tmux_pane_id: '%92',
      cwd: '/some/other/wt',
      worktree_cwd: '/some/other/wt',
      branch: 'feat/other',
      state: 'running',
    }));

    const { source, wtRoot, perWorktreeDir } = makeLinkedWorktree();
    fs.writeFileSync(path.join(perWorktreeDir, MARKER_NAME), 'sess-other');

    const spawnSync = recordingSpawn({
      [`git -C ${wtRoot} worktree list --porcelain`]:
        `worktree ${source}\nbranch refs/heads/main\n\nworktree ${wtRoot}\nbranch refs/heads/feat/heal\n\n`,
      [`git -C ${wtRoot} rev-parse --absolute-git-dir`]: `${perWorktreeDir}\n`,
    });

    const update = claimWorktreeForPane({
      worktreePath: wtRoot,
      paneId: '%91',
      stateDir,
      readAllState,
      writeState,
    }, { spawnSync });

    assert.deepEqual(update, { worktree_cwd: wtRoot, branch: 'feat/heal' });
    assert.equal(fs.readFileSync(path.join(perWorktreeDir, MARKER_NAME), 'utf8'), 'sess-me');
    const patched = JSON.parse(fs.readFileSync(path.join(agentsDir, 'sess-me.json'), 'utf8'));
    assert.equal(patched.worktree_cwd, wtRoot);
    assert.equal(patched.branch, 'feat/heal');
  });

  it('refuses to overwrite a marker whose owner legitimately pins this worktree', () => {
    const stateDir = tempDir();
    const agentsDir = path.join(stateDir, 'agents');
    fs.mkdirSync(agentsDir, { recursive: true });
    fs.writeFileSync(path.join(agentsDir, 'sess-me.json'), JSON.stringify({
      session_id: 'sess-me',
      target: 'main:1.0',
      tmux_pane_id: '%91',
      cwd: '/tmp/gone',
      branch: 'HEAD',
      state: 'running',
    }));

    const { source, wtRoot, perWorktreeDir } = makeLinkedWorktree();
    fs.writeFileSync(path.join(perWorktreeDir, MARKER_NAME), 'sess-other');
    fs.writeFileSync(path.join(agentsDir, 'sess-other.json'), JSON.stringify({
      session_id: 'sess-other',
      target: 'main:2.0',
      tmux_pane_id: '%92',
      cwd: wtRoot,
      worktree_cwd: wtRoot,
      branch: 'feat/other',
      state: 'running',
    }));

    const spawnSync = recordingSpawn({
      [`git -C ${wtRoot} worktree list --porcelain`]:
        `worktree ${source}\nbranch refs/heads/main\n\nworktree ${wtRoot}\nbranch refs/heads/feat/other\n\n`,
      [`git -C ${wtRoot} rev-parse --absolute-git-dir`]: `${perWorktreeDir}\n`,
    });

    assert.throws(
      () => claimWorktreeForPane({
        worktreePath: wtRoot,
        paneId: '%91',
        stateDir,
        readAllState,
        writeState,
      }, { spawnSync }),
      /worktree marker is owned by another session/,
    );
    assert.equal(fs.readFileSync(path.join(perWorktreeDir, MARKER_NAME), 'utf8'), 'sess-other');
  });

  it('clears stale branch when claiming a detached linked worktree', () => {
    const stateDir = tempDir();
    const agentsDir = path.join(stateDir, 'agents');
    fs.mkdirSync(agentsDir, { recursive: true });
    const agentPath = path.join(agentsDir, 'sess-claim.json');
    fs.writeFileSync(agentPath, JSON.stringify({
      session_id: 'sess-claim',
      target: 'main:1.0',
      tmux_pane_id: '%12',
      cwd: '/repo/main',
      branch: 'main',
      state: 'running',
    }));

    const { source, wtRoot, perWorktreeDir } = makeLinkedWorktree();
    const spawnSync = recordingSpawn({
      [`git -C ${wtRoot} worktree list --porcelain`]:
        `worktree ${source}\nbranch refs/heads/main\n\nworktree ${wtRoot}\nHEAD abc\ndetached\n\n`,
      [`git -C ${wtRoot} rev-parse --absolute-git-dir`]: `${perWorktreeDir}\n`,
    });

    const update = claimWorktreeForPane({
      worktreePath: wtRoot,
      paneId: '%12',
      stateDir,
      readAllState,
      writeState,
    }, {
      spawnSync,
    });

    assert.deepEqual(update, { worktree_cwd: wtRoot, branch: '' });
    const patched = JSON.parse(fs.readFileSync(agentPath, 'utf8'));
    assert.equal(patched.branch, '');
  });
});
