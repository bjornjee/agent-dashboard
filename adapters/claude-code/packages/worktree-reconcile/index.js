'use strict';

/**
 * Deterministic worktree ownership for JS hooks and skill setup scripts.
 *
 * Hooks call reconcileWorktree() on PreToolUse / PostToolUse / SessionStart.
 * The function asks the filesystem + git (NOT the agent's Bash command) where
 * this session's worktree is, and atomically claims it via a marker file at
 * <git-dir>/agent-dashboard-session.
 *
 * Skills call claimWorktreeForPane() once after creating and entering a linked
 * worktree. That explicit path shares marker/state semantics with reconcile
 * but does not try to infer intent from hook timing.
 *
 * Determinism guarantees:
 *   • POSIX mtime updates atomically on directory adds/removes.
 *   • `.git` is a dir (main worktree) or a text file `gitdir: <abs>` (linked).
 *   • O_CREAT|O_EXCL gives one-marker-per-session atomic ownership.
 *   • `git worktree list --porcelain` is git's stable machine format.
 *   • No regex on agent commands. The only string parsing is the
 *     `gitdir: <path>` line out of `.git`.
 */

const realFs = require('fs');
const realPath = require('path');
const { spawnSync: realSpawnSync } = require('child_process');

const MARKER_NAME = 'agent-dashboard-session';
const GIT_TIMEOUT_MS = 2000;

/**
 * Look up and consume the dashboard's spawn-pin staging file for this pane.
 * Returns `{ worktree_cwd, branch }` and deletes the file on hit, or null on
 * any miss (no TMUX_PANE, no state dir, no file, existing already pinned,
 * malformed JSON, or empty worktree_cwd).
 *
 * Mirrors the Go-side ApplySpawnPins logic in internal/state/spawnpin.go so
 * whichever side (JS hook or Go refresh) reaches the staging file first wins
 * deterministically.
 */
function consumeSpawnPin({ fs, path, env, existing }) {
  if (existing && existing.worktree_cwd && isLinkedWorktreePath(fs, path, existing.worktree_cwd)) return null;
  const paneId = env.TMUX_PANE;
  if (!paneId) return null;
  const home = env.HOME || env.USERPROFILE || '/tmp';
  const stateDir = env.AGENT_DASHBOARD_DIR || path.join(home, '.agent-dashboard');
  const filename = paneId.replace(/%/g, '_') + '.json';
  const pinPath = path.join(stateDir, 'spawn-pins', filename);
  let raw;
  try { raw = fs.readFileSync(pinPath, 'utf8'); }
  catch { return null; }
  let parsed;
  try { parsed = JSON.parse(raw); }
  catch {
    try { fs.unlinkSync(pinPath); } catch {} // malformed; clean up
    return null;
  }
  if (!parsed || !parsed.worktree_cwd) {
    try { fs.unlinkSync(pinPath); } catch {}
    return null;
  }
  try { fs.unlinkSync(pinPath); } catch {}
  return { worktree_cwd: parsed.worktree_cwd, branch: parsed.branch || '' };
}

/**
 * Find the .git for the working tree at `cwd` by walking up the directory tree.
 * Returns null when no .git is found (cwd isn't inside any git repo).
 *
 * For the main worktree, `.git` is a directory and we return
 *   { type: 'main', source, gitDir }.
 * For a linked worktree, `.git` is a text file containing `gitdir: <abs>`
 * pointing into the source repo's .git/worktrees/<name>/ — we return
 *   { type: 'linked', worktreeRoot, gitDir }
 * where gitDir is the per-worktree dir (where the marker lives).
 */
function findGitDir(fs, path, startCwd) {
  let dir = startCwd;
  // Bound the walk so a path outside any repo can't loop forever.
  for (let i = 0; i < 64; i++) {
    const candidate = path.join(dir, '.git');
    let stat;
    try { stat = fs.lstatSync(candidate); }
    catch { stat = null; }
    if (stat) {
      if (stat.isDirectory()) {
        return { type: 'main', source: dir, gitDir: candidate };
      }
      if (stat.isFile()) {
        let body;
        try { body = fs.readFileSync(candidate, 'utf8'); }
        catch { return null; }
        const m = body.match(/^gitdir:\s+(.+?)\s*$/m);
        if (!m) return null;
        return { type: 'linked', worktreeRoot: dir, gitDir: m[1].trim() };
      }
    }
    const parent = path.dirname(dir);
    if (parent === dir) return null;
    dir = parent;
  }
  return null;
}

/**
 * Try to claim a worktree for `sessionId` by reading or atomically creating
 * the marker file at <gitDir>/agent-dashboard-session.
 *
 * Returns one of:
 *   { match: true,  owner: sessionId }   — marker exists and matches us
 *   { match: false, owner: <other> }     — marker exists, owned by another session
 *   { match: true,  owner: sessionId,
 *     claimed: true }                    — marker did not exist; we created it
 *   { match: false, error: <err> }       — IO failure other than EEXIST/ENOENT
 */
function claimMarker(fs, path, gitDir, sessionId) {
  const marker = path.join(gitDir, MARKER_NAME);
  let existing;
  try { existing = fs.readFileSync(marker, 'utf8'); }
  catch (err) {
    if (err.code !== 'ENOENT') {
      return { match: false, error: err };
    }
    existing = null;
  }
  if (existing !== null) {
    const owner = existing.trim();
    return { match: owner === sessionId, owner };
  }
  // Marker doesn't exist — atomic claim.
  try {
    const fd = fs.openSync(marker, 'wx', 0o600);
    try { fs.writeSync(fd, sessionId); }
    finally { fs.closeSync(fd); }
    return { match: true, owner: sessionId, claimed: true };
  } catch (err) {
    if (err.code === 'EEXIST') {
      // Someone else won the race in the microsecond between our read and
      // open. Re-read to see who owns it now.
      try {
        const after = fs.readFileSync(marker, 'utf8').trim();
        return { match: after === sessionId, owner: after };
      } catch {
        return { match: false };
      }
    }
    return { match: false, error: err };
  }
}

/**
 * Get the current branch name for `dir`, or empty string on failure (detached
 * HEAD, missing dir, git error). Synchronous spawn — same pattern as the
 * existing packages/git-status helpers.
 */
function getBranch(spawnSync, dir) {
  const r = spawnSync('git', ['-C', dir, 'branch', '--show-current'], {
    encoding: 'utf8', timeout: GIT_TIMEOUT_MS, stdio: ['ignore', 'pipe', 'ignore'],
  });
  if (r.status !== 0 || !r.stdout) return '';
  return r.stdout.trim();
}

/**
 * Run `git -C <source> worktree list --porcelain` and parse the line-oriented
 * blocks. Returns [{ path, branch }, ...]. Blank lines separate entries.
 * The branch line's `refs/heads/` prefix is stripped; non-heads refs are
 * kept verbatim; `detached` yields branch=''.
 */
function listWorktrees(spawnSync, source) {
  const r = spawnSync('git', ['-C', source, 'worktree', 'list', '--porcelain'], {
    encoding: 'utf8', timeout: GIT_TIMEOUT_MS, stdio: ['ignore', 'pipe', 'ignore'],
  });
  if (r.status !== 0 || !r.stdout) return [];
  const out = [];
  let cur = null;
  for (const line of r.stdout.split('\n')) {
    if (line === '') {
      if (cur && cur.path) out.push(cur);
      cur = null;
      continue;
    }
    if (!cur) cur = { path: '', branch: '' };
    if (line.startsWith('worktree ')) {
      cur.path = line.slice('worktree '.length);
    } else if (line.startsWith('branch ')) {
      const ref = line.slice('branch '.length);
      cur.branch = ref.startsWith('refs/heads/') ? ref.slice('refs/heads/'.length) : ref;
    }
    // HEAD / detached / locked / prunable lines are intentionally ignored.
  }
  if (cur && cur.path) out.push(cur);
  return out;
}

/**
 * Resolve the per-worktree git-dir for a worktree path. `git rev-parse
 * --absolute-git-dir` from inside the worktree yields the path to
 * <source>/.git/worktrees/<name>/ for linked worktrees, or <source>/.git
 * for the main worktree.
 */
function resolveWorktreeGitDir(spawnSync, worktreePath) {
  const r = spawnSync('git', ['-C', worktreePath, 'rev-parse', '--absolute-git-dir'], {
    encoding: 'utf8', timeout: GIT_TIMEOUT_MS, stdio: ['ignore', 'pipe', 'ignore'],
  });
  if (r.status !== 0 || !r.stdout) return '';
  return r.stdout.trim();
}

function canonicalPath(fs, path, p) {
  try { return fs.realpathSync(p); } catch {}
  try { return path.resolve(p); } catch {}
  return p;
}

function findAgentByPane(readAllState, agentsDir, paneId) {
  const state = readAllState(agentsDir);
  for (const agent of Object.values(state.agents || {})) {
    if (agent && agent.tmux_pane_id === paneId) return agent;
  }
  return null;
}

function reapStaleMarker(fs, path, stateDir, gitDir) {
  const marker = path.join(gitDir, MARKER_NAME);
  let owner;
  try { owner = fs.readFileSync(marker, 'utf8').trim(); } catch { return; }
  if (!owner) return;
  if (fs.existsSync(path.join(stateDir, 'agents', owner + '.json'))) return;
  try { fs.unlinkSync(marker); } catch {}
}

function isLinkedWorktreePath(fs, path, p) {
  const info = findGitDir(fs, path, p);
  return !!(info && info.type === 'linked');
}

/**
 * Explicitly claim a linked worktree for the agent currently running in a
 * tmux pane. This is the skill-directed path used immediately after a
 * worktree is created and entered. It differs from reconcileWorktree(), which
 * is the automatic hook recovery path.
 */
function claimWorktreeForPane({ worktreePath, paneId, stateDir, readAllState, writeState }, opts) {
  if (!paneId) throw new Error('TMUX_PANE is required');
  if (!worktreePath) throw new Error('worktree path is required');
  if (!stateDir) throw new Error('state directory is required');
  if (typeof readAllState !== 'function') throw new Error('readAllState is required');
  if (typeof writeState !== 'function') throw new Error('writeState is required');

  const o = opts || {};
  const fs = o.fs || realFs;
  const path = o.path || realPath;
  const spawnSync = o.spawnSync || realSpawnSync;

  const agentsDir = path.join(stateDir, 'agents');
  const agent = findAgentByPane(readAllState, agentsDir, paneId);
  if (!agent) throw new Error(`no agent state found for pane ${paneId}`);
  if (!agent.session_id) throw new Error(`agent for pane ${paneId} has no session_id`);

  const absPath = path.resolve(worktreePath);
  const info = findGitDir(fs, path, absPath);
  if (!info || info.type !== 'linked') throw new Error(`${absPath} is not a linked worktree`);

  const want = canonicalPath(fs, path, info.worktreeRoot);
  const wts = listWorktrees(spawnSync, info.worktreeRoot);
  for (const wt of wts) {
    if (canonicalPath(fs, path, wt.path) !== want) continue;
    const gitDir = resolveWorktreeGitDir(spawnSync, wt.path);
    if (!gitDir) throw new Error(`worktree git dir not found for ${wt.path}`);
    reapStaleMarker(fs, path, stateDir, gitDir);
    const claimed = claimMarker(fs, path, gitDir, agent.session_id);
    if (!claimed.match) throw new Error('worktree marker is owned by another session');

    const update = { worktree_cwd: wt.path, branch: wt.branch || '' };
    writeState(agent.session_id, update, agentsDir);
    return update;
  }

  throw new Error(`worktree ${absPath} not found in git worktree list`);
}

/**
 * The 5-step reconciliation flow. Returns a partial update object to merge
 * into the agent's state file, or null when there is nothing to write.
 *
 *   1. existing.worktree_cwd points at a linked worktree → null.
 *   2. fs walk to .git. Linked worktree → claim and return.
 *   3. main worktree → stat <source>/.git/worktrees/ mtime.
 *      ENOENT → null. Never invoke git when no linked worktrees exist.
 *   4. mtime matches cached existing.worktrees_seen_mtime → null.
 *   5. git worktree list --porcelain. For each linked worktree, claim its
 *      marker. First match wins; persist the mtime either way.
 */
function reconcileWorktree({ input, existing, sessionId }, opts) {
  if (!sessionId) return null;
  const cwd = input && input.cwd;
  if (!cwd) return null;

  const o = opts || {};
  const fs = o.fs || realFs;
  const path = o.path || realPath;
  const spawnSync = o.spawnSync || realSpawnSync;
  const env = o.env || process.env;

  // Steady state — fully pinned to a linked worktree. Nothing to do.
  if (existing && existing.worktree_cwd && existing.branch && isLinkedWorktreePath(fs, path, existing.worktree_cwd)) return null;

  // Dashboard-staged spawn-pin path. When the dashboard spawned this agent
  // it left a record at <stateDir>/spawn-pins/<pane_id>.json with the
  // worktree + branch already resolved. Consuming it here means the agent's
  // own state file lands correct on the very first hook event, even when a
  // sibling worktree has a stale marker that would block the marker-claim
  // path below. No-op when the agent is already pinned (handled above).
  const stagedPin = consumeSpawnPin({ fs, path, env, existing });
  if (stagedPin) return stagedPin;

  // Branch backfill: worktree_cwd already pinned but branch is missing
  // (legacy state from before the atomic pin, or a partial write). One git
  // call. Also drop the marker if absent so future state-file wipes can
  // recover via Go's ResolveAgentWorktree.
  if (existing && existing.worktree_cwd && !existing.branch) {
    const branch = getBranch(spawnSync, existing.worktree_cwd);
    const info = findGitDir(fs, path, existing.worktree_cwd);
    if (info && info.type === 'linked') {
      claimMarker(fs, path, info.gitDir, sessionId); // best-effort
    }
    return branch ? { branch } : null;
  }

  const info = findGitDir(fs, path, cwd);
  if (!info) return null;

  if (info.type === 'linked') {
    // Scenario C: agent launched directly in a linked worktree.
    const r = claimMarker(fs, path, info.gitDir, sessionId);
    if (!r.match) return null;
    return {
      worktree_cwd: info.worktreeRoot,
      branch: getBranch(spawnSync, info.worktreeRoot),
    };
  }

  // Main worktree — check whether linked-worktrees set changed since last scan.
  const worktreesDir = path.join(info.gitDir, 'worktrees');
  let mtime;
  try { mtime = fs.statSync(worktreesDir).mtimeMs; }
  catch { return null; }

  const cached = existing && existing.worktrees_seen_mtime;
  if (cached === mtime) return null;

  const wts = listWorktrees(spawnSync, info.source);
  for (const wt of wts) {
    if (wt.path === info.source) continue; // skip main worktree
    const wtGitDir = resolveWorktreeGitDir(spawnSync, wt.path);
    if (!wtGitDir) continue;
    const r = claimMarker(fs, path, wtGitDir, sessionId);
    if (r.match) {
      return {
        worktree_cwd: wt.path,
        branch: wt.branch,
        worktrees_seen_mtime: mtime,
      };
    }
  }
  // Cache the mtime even when no match — short-circuits future events
  // until the worktree set changes again.
  return { worktrees_seen_mtime: mtime };
}

module.exports = {
  reconcileWorktree,
  claimWorktreeForPane,
  // Exported for direct unit tests.
  findGitDir,
  claimMarker,
  listWorktrees,
  MARKER_NAME,
};
