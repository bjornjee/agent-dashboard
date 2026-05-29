'use strict';

const { describe, it } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('fs');
const path = require('path');
const os = require('os');

const { reconcileWorktree, findGitDir, claimMarker, listWorktrees, MARKER_NAME } = require('./index');

function tempDir() {
  return fs.mkdtempSync(path.join(os.tmpdir(), 'wtrecon-'));
}

// makeMainRepo creates a minimal "main worktree" layout — a directory with a
// real .git directory (so findGitDir's lstat returns isDirectory()).
function makeMainRepo() {
  const root = tempDir();
  fs.mkdirSync(path.join(root, '.git'), { recursive: true });
  return { root, gitDir: path.join(root, '.git') };
}

// makeLinkedWorktree creates a directory whose .git is a file pointing to a
// per-worktree dir under the source repo.
function makeLinkedWorktree(source) {
  const wtRoot = tempDir();
  const wtName = path.basename(wtRoot);
  const perWorktreeDir = path.join(source, '.git', 'worktrees', wtName);
  fs.mkdirSync(perWorktreeDir, { recursive: true });
  fs.writeFileSync(path.join(wtRoot, '.git'), `gitdir: ${perWorktreeDir}\n`);
  return { wtRoot, perWorktreeDir };
}

// recordingSpawn returns a spawnSync stub plus the recorded call list.
function recordingSpawn(handlers) {
  const calls = [];
  const spawnSync = (cmd, args, _opts) => {
    calls.push({ cmd, args });
    const key = `${cmd} ${args.join(' ')}`;
    const h = handlers[key];
    if (!h) return { status: 1, stdout: '' };
    return { status: 0, stdout: h };
  };
  return { spawnSync, calls };
}

describe('reconcileWorktree', () => {
  it('returns null when fully pinned (worktree_cwd + branch) — no syscalls', () => {
    const { spawnSync, calls } = recordingSpawn({});
    const fsSpy = { calls: [] };
    const proxiedFs = new Proxy(fs, {
      get(target, prop) {
        fsSpy.calls.push(prop);
        return target[prop];
      },
    });
    const out = reconcileWorktree({
      input: { cwd: '/whatever' },
      existing: { worktree_cwd: '/already/pinned', branch: 'feat/x' },
      sessionId: 'sess-1',
    }, { spawnSync, fs: proxiedFs });
    assert.equal(out, null);
    assert.equal(calls.length, 0, 'no git subprocess fired');
    assert.equal(fsSpy.calls.length, 0, 'no fs syscalls fired');
  });

  it('branch backfill: worktree_cwd set, branch empty → returns {branch} and drops marker', () => {
    const { root: source } = makeMainRepo();
    const { wtRoot, perWorktreeDir } = makeLinkedWorktree(source);

    const { spawnSync } = recordingSpawn({
      [`git -C ${wtRoot} branch --show-current`]: 'feat/backfilled\n',
    });

    const out = reconcileWorktree({
      input: { cwd: source },
      existing: { worktree_cwd: wtRoot }, // branch missing
      sessionId: 'sess-backfill',
    }, { spawnSync });

    assert.deepEqual(out, { branch: 'feat/backfilled' });
    // Marker dropped so a future state-file wipe can be recovered.
    const marker = fs.readFileSync(path.join(perWorktreeDir, MARKER_NAME), 'utf8');
    assert.equal(marker.trim(), 'sess-backfill');
  });

  it('branch backfill: branch lookup fails → returns null', () => {
    const { root: source } = makeMainRepo();
    const { wtRoot } = makeLinkedWorktree(source);
    const spawnSync = () => ({ status: 128, stdout: '' });
    const out = reconcileWorktree({
      input: { cwd: source },
      existing: { worktree_cwd: wtRoot },
      sessionId: 'sess-1',
    }, { spawnSync });
    assert.equal(out, null);
  });

  it('returns null when sessionId is empty', () => {
    const out = reconcileWorktree({
      input: { cwd: '/x' }, existing: {}, sessionId: '',
    });
    assert.equal(out, null);
  });

  it('returns null when input.cwd is empty', () => {
    const out = reconcileWorktree({
      input: {}, existing: {}, sessionId: 'sess-1',
    });
    assert.equal(out, null);
  });

  it('Scenario C: agent launched inside a linked worktree → pins via fs walk, no git porcelain', () => {
    const { root: source } = makeMainRepo();
    const { wtRoot, perWorktreeDir } = makeLinkedWorktree(source);

    const { spawnSync, calls } = recordingSpawn({
      [`git -C ${wtRoot} branch --show-current`]: 'feat/x\n',
    });

    const out = reconcileWorktree({
      input: { cwd: wtRoot },
      existing: {},
      sessionId: 'sess-launched-in-wt',
    }, { spawnSync });

    assert.equal(out.worktree_cwd, wtRoot);
    assert.equal(out.branch, 'feat/x');
    // The marker should now exist with our session id.
    const marker = fs.readFileSync(path.join(perWorktreeDir, MARKER_NAME), 'utf8');
    assert.equal(marker.trim(), 'sess-launched-in-wt');
    // Only the branch-lookup spawn fired; no porcelain call.
    const porcelainCalls = calls.filter(c => c.args.includes('--porcelain'));
    assert.equal(porcelainCalls.length, 0, 'no porcelain spawn');
  });

  it('Scenario A: main repo, worktrees dir does not exist → returns null without spawning git', () => {
    const { root: source } = makeMainRepo();
    const { spawnSync, calls } = recordingSpawn({});
    const out = reconcileWorktree({
      input: { cwd: source },
      existing: {},
      sessionId: 'sess-1',
    }, { spawnSync });
    assert.equal(out, null);
    assert.equal(calls.length, 0);
  });

  it('mtime cached matches existing → returns null without spawning git', () => {
    const { root: source, gitDir } = makeMainRepo();
    // Create empty worktrees/ dir so the stat succeeds.
    fs.mkdirSync(path.join(gitDir, 'worktrees'));
    const cachedMtime = fs.statSync(path.join(gitDir, 'worktrees')).mtimeMs;

    const { spawnSync, calls } = recordingSpawn({});
    const out = reconcileWorktree({
      input: { cwd: source },
      existing: { worktrees_seen_mtime: cachedMtime },
      sessionId: 'sess-1',
    }, { spawnSync });
    assert.equal(out, null);
    assert.equal(calls.length, 0, 'mtime cache short-circuits before any git spawn');
  });

  it('mtime changed, marker absent → atomic claim + pin', () => {
    const { root: source } = makeMainRepo();
    const { wtRoot, perWorktreeDir } = makeLinkedWorktree(source);

    const { spawnSync } = recordingSpawn({
      [`git -C ${source} worktree list --porcelain`]:
        `worktree ${source}\nHEAD abc\nbranch refs/heads/main\n\nworktree ${wtRoot}\nHEAD def\nbranch refs/heads/feat/foo\n`,
      [`git -C ${wtRoot} rev-parse --absolute-git-dir`]: `${perWorktreeDir}\n`,
    });

    const out = reconcileWorktree({
      input: { cwd: source },
      existing: {},
      sessionId: 'sess-claim',
    }, { spawnSync });

    assert.equal(out.worktree_cwd, wtRoot);
    assert.equal(out.branch, 'feat/foo');
    assert.ok(out.worktrees_seen_mtime, 'mtime stamped');
    assert.equal(
      fs.readFileSync(path.join(perWorktreeDir, MARKER_NAME), 'utf8').trim(),
      'sess-claim',
    );
  });

  it('mtime changed, marker matches → pin without re-claiming', () => {
    const { root: source } = makeMainRepo();
    const { wtRoot, perWorktreeDir } = makeLinkedWorktree(source);
    fs.writeFileSync(path.join(perWorktreeDir, MARKER_NAME), 'sess-mine');

    const { spawnSync } = recordingSpawn({
      [`git -C ${source} worktree list --porcelain`]:
        `worktree ${source}\nHEAD abc\nbranch refs/heads/main\n\nworktree ${wtRoot}\nHEAD def\nbranch refs/heads/feat/foo\n`,
      [`git -C ${wtRoot} rev-parse --absolute-git-dir`]: `${perWorktreeDir}\n`,
    });

    const out = reconcileWorktree({
      input: { cwd: source },
      existing: {},
      sessionId: 'sess-mine',
    }, { spawnSync });

    assert.equal(out.worktree_cwd, wtRoot);
    assert.equal(out.branch, 'feat/foo');
  });

  it('mtime changed, marker owned by another session → skip', () => {
    const { root: source } = makeMainRepo();
    const { wtRoot, perWorktreeDir } = makeLinkedWorktree(source);
    fs.writeFileSync(path.join(perWorktreeDir, MARKER_NAME), 'sess-someone-else');

    const { spawnSync } = recordingSpawn({
      [`git -C ${source} worktree list --porcelain`]:
        `worktree ${source}\nHEAD abc\nbranch refs/heads/main\n\nworktree ${wtRoot}\nHEAD def\nbranch refs/heads/feat/foo\n`,
      [`git -C ${wtRoot} rev-parse --absolute-git-dir`]: `${perWorktreeDir}\n`,
    });

    const out = reconcileWorktree({
      input: { cwd: source },
      existing: {},
      sessionId: 'sess-mine',
    }, { spawnSync });

    // No pin (worktree_cwd not set), but mtime IS cached so we don't re-scan
    // on the next event.
    assert.equal(out.worktree_cwd, undefined);
    assert.ok(out.worktrees_seen_mtime, 'mtime stamped to short-circuit next time');
  });

  it('multiple linked worktrees, only one matches → correct one pinned', () => {
    const { root: source } = makeMainRepo();
    const a = makeLinkedWorktree(source);
    const b = makeLinkedWorktree(source);
    fs.writeFileSync(path.join(a.perWorktreeDir, MARKER_NAME), 'sess-other');
    fs.writeFileSync(path.join(b.perWorktreeDir, MARKER_NAME), 'sess-mine');

    const { spawnSync } = recordingSpawn({
      [`git -C ${source} worktree list --porcelain`]:
        `worktree ${source}\nHEAD abc\nbranch refs/heads/main\n\n` +
        `worktree ${a.wtRoot}\nHEAD def\nbranch refs/heads/feat/a\n\n` +
        `worktree ${b.wtRoot}\nHEAD ghi\nbranch refs/heads/feat/b\n`,
      [`git -C ${a.wtRoot} rev-parse --absolute-git-dir`]: `${a.perWorktreeDir}\n`,
      [`git -C ${b.wtRoot} rev-parse --absolute-git-dir`]: `${b.perWorktreeDir}\n`,
    });

    const out = reconcileWorktree({
      input: { cwd: source },
      existing: {},
      sessionId: 'sess-mine',
    }, { spawnSync });

    assert.equal(out.worktree_cwd, b.wtRoot);
    assert.equal(out.branch, 'feat/b');
  });

  it('walk halts at filesystem root for a non-git cwd', () => {
    const out = findGitDir(fs, path, '/tmp');
    // /tmp may or may not be inside a git repo depending on the system; the
    // assertion that matters is that findGitDir returns a sane value, not a
    // crash or infinite loop.
    assert.ok(out === null || (out && typeof out.type === 'string'));
  });
});

describe('claimMarker race semantics', () => {
  it('two concurrent claims: only one wins, the other observes the winner', () => {
    const { root: source } = makeMainRepo();
    const { perWorktreeDir } = makeLinkedWorktree(source);

    const a = claimMarker(fs, path, perWorktreeDir, 'sess-A');
    const b = claimMarker(fs, path, perWorktreeDir, 'sess-B');

    assert.equal(a.match, true);
    assert.equal(a.claimed, true);
    assert.equal(a.owner, 'sess-A');
    assert.equal(b.match, false);
    assert.equal(b.owner, 'sess-A');
  });
});

describe('reconcileWorktree spawn-pin consumer', () => {
  it('returns staged pin and deletes the staging file', () => {
    const dashboardDir = tempDir();
    const spawnPinsDir = path.join(dashboardDir, 'spawn-pins');
    fs.mkdirSync(spawnPinsDir, { recursive: true });
    const pinPath = path.join(spawnPinsDir, '_42.json'); // `%42` → `_42`
    fs.writeFileSync(pinPath, JSON.stringify({
      pane_id: '%42',
      worktree_cwd: '/tmp/wt/spawn',
      branch: 'feat/spawn',
      created_at: new Date().toISOString(),
    }));

    const { spawnSync, calls } = recordingSpawn({});
    const out = reconcileWorktree(
      { input: { cwd: '/somewhere' }, existing: {}, sessionId: 'sess-spawn' },
      { spawnSync, env: { TMUX_PANE: '%42', AGENT_DASHBOARD_DIR: dashboardDir } },
    );

    assert.equal(out.worktree_cwd, '/tmp/wt/spawn');
    assert.equal(out.branch, 'feat/spawn');
    assert.equal(fs.existsSync(pinPath), false, 'staging file should be deleted');
    assert.equal(calls.length, 0, 'no git subprocess needed when staged pin wins');
  });

  it('skips staged pin when existing.worktree_cwd is already set', () => {
    const dashboardDir = tempDir();
    fs.mkdirSync(path.join(dashboardDir, 'spawn-pins'), { recursive: true });
    const pinPath = path.join(dashboardDir, 'spawn-pins', '_99.json');
    fs.writeFileSync(pinPath, JSON.stringify({ pane_id: '%99', worktree_cwd: '/new', branch: 'b' }));

    const out = reconcileWorktree(
      { input: { cwd: '/x' }, existing: { worktree_cwd: '/old', branch: 'b' }, sessionId: 's' },
      { spawnSync: () => ({ status: 1 }), env: { TMUX_PANE: '%99', AGENT_DASHBOARD_DIR: dashboardDir } },
    );

    // Already fully pinned — the function returns null and never touches the staging file
    assert.equal(out, null);
    assert.equal(fs.existsSync(pinPath), true, 'staging file untouched when already pinned');
  });

  it('falls through to marker logic when no staged pin exists', () => {
    const dashboardDir = tempDir();
    const { root: source } = makeMainRepo();
    const { wtRoot } = makeLinkedWorktree(source);

    const { spawnSync } = recordingSpawn({
      [`git -C ${wtRoot} branch --show-current`]: 'feat/marker\n',
    });

    const out = reconcileWorktree(
      { input: { cwd: wtRoot }, existing: {}, sessionId: 'sess-marker' },
      { spawnSync, env: { TMUX_PANE: '%nonexistent', AGENT_DASHBOARD_DIR: dashboardDir } },
    );

    assert.equal(out.worktree_cwd, wtRoot);
    assert.equal(out.branch, 'feat/marker');
  });
});

describe('listWorktrees parser', () => {
  it('detached HEAD yields empty branch', () => {
    const spawnSync = () => ({
      status: 0,
      stdout: 'worktree /repo\nHEAD abc\nbranch refs/heads/main\n\nworktree /wt\nHEAD def\ndetached\n',
    });
    const out = listWorktrees(spawnSync, '/repo');
    assert.deepEqual(out, [
      { path: '/repo', branch: 'main' },
      { path: '/wt', branch: '' },
    ]);
  });

  it('tolerates trailing blank line', () => {
    const spawnSync = () => ({ status: 0, stdout: 'worktree /repo\nbranch refs/heads/main\n\n' });
    const out = listWorktrees(spawnSync, '/repo');
    assert.deepEqual(out, [{ path: '/repo', branch: 'main' }]);
  });
});
