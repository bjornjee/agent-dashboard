'use strict';

const { describe, it, beforeEach, afterEach } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('fs');
const path = require('path');
const os = require('os');

const { validateState, validateAgent, sortAgentsByPriority, VALID_STATES } = require('./schema');
const { detectState, scoreMessage, scorePaneBuffer } = require('./detect');
const { readAgentState, writeState, readAllState, cleanStale, removeAgent } = require('./index');

// Temp dir for per-agent file tests
let tmpDir;
let agentsDir;

beforeEach(() => {
  tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'agent-state-test-'));
  agentsDir = path.join(tmpDir, 'agents');
});

afterEach(() => {
  fs.rmSync(tmpDir, { recursive: true, force: true });
});

// --- Schema ---

describe('schema/validateAgent', () => {
  it('rejects null and non-objects', () => {
    assert.equal(validateAgent(null), false);
    assert.equal(validateAgent('string'), false);
    assert.equal(validateAgent(42), false);
  });

  it('rejects missing target', () => {
    assert.equal(validateAgent({ session_id: 'abc-123', state: 'running' }), false);
  });

  it('rejects missing session_id', () => {
    assert.equal(validateAgent({ target: 'a:0.1', state: 'running' }), false);
  });

  it('rejects invalid state', () => {
    assert.equal(validateAgent({ target: 'a:0.1', session_id: 'abc-123', state: 'unknown' }), false);
  });

  it('accepts valid agent with both target and session_id', () => {
    for (const state of VALID_STATES) {
      assert.equal(validateAgent({ target: 'a:0.1', session_id: 'abc-123', state }), true);
    }
  });

  it('accepts waiting_input as an alias for question', () => {
    const result = validateState({
      agents: {
        waiting: { target: 'a:0.1', session_id: 'abc-123', state: 'waiting_input' },
      },
    });
    assert.equal(result.agents.waiting.state, 'question');
  });
});

describe('schema/validateState', () => {
  it('returns empty state for null input', () => {
    assert.deepEqual(validateState(null), { agents: {} });
  });

  it('filters out invalid agents', () => {
    const result = validateState({
      agents: {
        good: { target: 'a:0.1', session_id: 'abc-123', state: 'running' },
        bad: { state: 'invalid' },
        noSession: { target: 'b:0.1', state: 'running' },
      },
    });
    assert.equal(Object.keys(result.agents).length, 1);
    assert.ok(result.agents.good);
  });
});

describe('schema/sortAgentsByPriority', () => {
  it('sorts blocked, then waiting, then running, then review', () => {
    const agents = [
      { state: 'done', target: 'a' },
      { state: 'permission', target: 'b' },
      { state: 'running', target: 'c' },
      { state: 'error', target: 'd' },
      { state: 'idle_prompt', target: 'e' },
      { state: 'question', target: 'f' },
      { state: 'waiting_input', target: 'g' },
    ];
    const sorted = sortAgentsByPriority(agents);
    const states = sorted.map(a => a.state);
    // permission (1) → error+question aliases (2, stable order from input) → running (3) → done+idle_prompt (4, stable order from input)
    assert.deepEqual(states, ['permission', 'error', 'question', 'question', 'running', 'done', 'idle_prompt']);
  });
});

// --- Detect ---

describe('detect/scoreMessage', () => {
  it('returns 0 for null/empty', () => {
    assert.equal(scoreMessage(null), 0);
    assert.equal(scoreMessage(''), 0);
  });

  it('scores questions ending with ?', () => {
    assert.ok(scoreMessage('Which provider should I use?') > 0);
  });

  it('scores question patterns', () => {
    assert.ok(scoreMessage('Should I use Firebase?') > 0);
    assert.ok(scoreMessage('Do you want me to proceed?') > 0);
    assert.ok(scoreMessage('Please choose one') > 0);
  });

  it('returns 0 for non-question statements', () => {
    assert.equal(scoreMessage('I have completed the task.'), 0);
    assert.equal(scoreMessage('All tests pass.'), 0);
  });
});

describe('detect/scorePaneBuffer', () => {
  it('returns 0 for empty buffer', () => {
    assert.equal(scorePaneBuffer([]), 0);
    assert.equal(scorePaneBuffer(null), 0);
  });

  it('detects > prompt', () => {
    assert.ok(scorePaneBuffer(['some output', '>']) > 0);
  });

  it('detects $ prompt', () => {
    assert.ok(scorePaneBuffer(['output', '$']) > 0);
  });

  it('returns 0 for normal output lines', () => {
    assert.equal(scorePaneBuffer(['Running tests...', 'All 5 passed']), 0);
  });

  it('detects plan approval menu with \u276f selector', () => {
    const planApprovalPane = [
      'Claude has written up a plan and is ready to execute. Would you like to proceed?',
      '',
      ' \u276f 1. Yes, and bypass permissions',
      '   2. Yes, manually approve edits',
      '   3. Tell Claude what to change',
    ];
    assert.ok(scorePaneBuffer(planApprovalPane) > 0);
  });

  it('detects \u276f anywhere in line, not just at end', () => {
    assert.ok(scorePaneBuffer(['some text', '\u276f 1. Option A']) > 0);
  });

  it('detects human: prompt', () => {
    assert.ok(scorePaneBuffer(['output', 'human:']) > 0);
    assert.ok(scorePaneBuffer(['output', 'Human: type here']) > 0);
  });
});

describe('detect/detectState', () => {
  it('returns question when message has question and pane has prompt', () => {
    assert.equal(detectState('Which one?', ['output', '>']), 'question');
  });

  it('returns question when only message has question', () => {
    assert.equal(detectState('Which one?', ['still running...']), 'question');
  });

  it('returns idle_prompt when pane shows prompt without question', () => {
    // Prompt visible but no question = finished turn, sitting at ❯
    assert.equal(detectState('Task complete.', ['$']), 'idle_prompt');
    assert.equal(detectState('Here is my plan.', ['\u276f']), 'idle_prompt');
  });

  it('returns question when message is a question even with plan approval pane', () => {
    const planPane = [
      'Here is my implementation plan.',
      '',
      ' \u276f 1. Yes, and bypass permissions',
      '   2. Yes, manually approve edits',
    ];
    // The message itself doesn't match question patterns, so idle_prompt
    assert.equal(detectState('Here is my implementation plan.', planPane), 'idle_prompt');
    // But if the message asks a question, it's a question
    assert.equal(detectState('Would you like to proceed?', planPane), 'question');
  });

  it('returns done when no signals', () => {
    assert.equal(detectState('All done.', ['output line']), 'done');
  });
});

// --- Per-agent file I/O (keyed by session_id) ---

describe('readAgentState', () => {
  it('returns null when agent file does not exist', () => {
    const state = readAgentState('nonexistent-session-id', agentsDir);
    assert.equal(state, null);
  });

  it('reads agent state from per-agent file by session_id', () => {
    fs.mkdirSync(agentsDir, { recursive: true });
    const sessionId = 'abc-def-123';
    fs.writeFileSync(
      path.join(agentsDir, `${sessionId}.json`),
      JSON.stringify({ target: 'a:0.1', session_id: sessionId, state: 'running', branch: 'main' }),
    );

    const state = readAgentState(sessionId, agentsDir);
    assert.equal(state.target, 'a:0.1');
    assert.equal(state.session_id, sessionId);
    assert.equal(state.state, 'running');
    assert.equal(state.branch, 'main');
  });

  it('handles corrupted JSON gracefully', () => {
    fs.mkdirSync(agentsDir, { recursive: true });
    const sessionId = 'abc-def-123';
    fs.writeFileSync(path.join(agentsDir, `${sessionId}.json`), 'not json{{{');

    assert.equal(readAgentState(sessionId, agentsDir), null);
  });
});

describe('writeState', () => {
  it('creates agents directory and file if they do not exist', () => {
    const sessionId = 'sess-001';
    writeState(sessionId, { target: 'a:0.1', session_id: sessionId, state: 'running' }, agentsDir);

    const state = readAgentState(sessionId, agentsDir);
    assert.equal(state.state, 'running');
    assert.ok(state.updated_at);
  });

  it('merges updates into existing agent file', () => {
    const sessionId = 'sess-001';
    writeState(sessionId, { target: 'a:0.1', session_id: sessionId, state: 'running', branch: 'main' }, agentsDir);
    writeState(sessionId, { state: 'done' }, agentsDir);

    const state = readAgentState(sessionId, agentsDir);
    assert.equal(state.state, 'done');
    assert.equal(state.branch, 'main');
  });

  it('writes separate files for different sessions', () => {
    const sess1 = 'sess-001';
    const sess2 = 'sess-002';
    writeState(sess1, { target: 'a:0.1', session_id: sess1, state: 'running' }, agentsDir);
    writeState(sess2, { target: 'b:0.1', session_id: sess2, state: 'question' }, agentsDir);

    const all = readAllState(agentsDir);
    assert.equal(Object.keys(all.agents).length, 2);
    assert.equal(all.agents[sess1].state, 'running');
    assert.equal(all.agents[sess2].state, 'question');
  });
});

describe('readAllState', () => {
  it('returns empty state when directory does not exist', () => {
    const state = readAllState(agentsDir);
    assert.deepEqual(state, { agents: {} });
  });

  it('reads all agent files from directory keyed by session_id', () => {
    const sess1 = 'sess-001';
    const sess2 = 'sess-002';
    writeState(sess1, { target: 'a:0.1', session_id: sess1, state: 'running' }, agentsDir);
    writeState(sess2, { target: 'b:1.0', session_id: sess2, state: 'question' }, agentsDir);

    const state = readAllState(agentsDir);
    assert.equal(Object.keys(state.agents).length, 2);
    assert.equal(state.agents[sess1].state, 'running');
    assert.equal(state.agents[sess2].state, 'question');
  });

  it('skips non-json files and invalid agents', () => {
    fs.mkdirSync(agentsDir, { recursive: true });
    fs.writeFileSync(path.join(agentsDir, 'readme.txt'), 'not an agent');
    fs.writeFileSync(path.join(agentsDir, 'bad.json'), 'not json');
    // Agent missing session_id should be skipped
    fs.writeFileSync(path.join(agentsDir, 'no-session.json'), JSON.stringify({ target: 'x:0.1', state: 'running' }));
    const sess1 = 'sess-001';
    writeState(sess1, { target: 'a:0.1', session_id: sess1, state: 'running' }, agentsDir);

    const state = readAllState(agentsDir);
    assert.equal(Object.keys(state.agents).length, 1);
  });
});

describe('removeAgent', () => {
  it('removes an agent file by session_id', () => {
    const sessionId = 'sess-to-remove';
    writeState(sessionId, { target: 'a:0.1', session_id: sessionId, state: 'running' }, agentsDir);
    assert.ok(readAgentState(sessionId, agentsDir));

    removeAgent(sessionId, agentsDir);
    assert.equal(readAgentState(sessionId, agentsDir), null);
  });

  it('does not throw when file does not exist', () => {
    removeAgent('nonexistent-session', agentsDir);
  });
});

describe('writeState concurrent (per-agent files)', () => {
  it('does not lose updates under concurrent multi-process writes', async () => {
    const { execFile } = require('child_process');
    const { promisify } = require('util');
    const execFileP = promisify(execFile);

    const N = 10;
    const script = path.join(tmpDir, '_concurrent-write-helper.js');

    const indexPath = path.join(__dirname, 'index.js');
    fs.writeFileSync(script, `
      const { writeState } = require(${JSON.stringify(indexPath)});
      const [sessionId, branch, dir] = process.argv.slice(2);
      writeState(sessionId, { target: 'agent:0.' + sessionId.split('-')[1], session_id: sessionId, state: 'running', branch }, dir);
    `);

    // Launch all N processes simultaneously — each writes its OWN file
    const promises = [];
    for (let i = 0; i < N; i++) {
      const sessionId = `sess-${i}`;
      promises.push(execFileP(process.execPath, [script, sessionId, `branch-${i}`, agentsDir]));
    }
    await Promise.all(promises);

    const state = readAllState(agentsDir);
    const agentCount = Object.keys(state.agents).length;
    assert.equal(agentCount, N, `Expected ${N} agents but got ${agentCount}`);

    for (let i = 0; i < N; i++) {
      const sessionId = `sess-${i}`;
      assert.ok(state.agents[sessionId], `Agent ${sessionId} missing from state`);
      assert.equal(state.agents[sessionId].branch, `branch-${i}`);
    }
  });

  it('preserves every field under concurrent writes to the SAME session_id', async () => {
    // Regression: the dashboard pin button + claude hook + codex hook can all
    // hit the same agent file in the same millisecond. Without the sidecar
    // file lock each writer's stale snapshot overwrites the others, wiping
    // pinned_state / branch / worktree_cwd at random.
    const { execFile } = require('child_process');
    const { promisify } = require('util');
    const execFileP = promisify(execFile);

    const sessionId = 'sess-shared';
    // Seed with baseline so writers MERGE rather than create.
    writeState(sessionId, { target: 'a:0.1', session_id: sessionId, state: 'running' }, agentsDir);

    const ITERS = 100;
    const script = path.join(tmpDir, '_same-session-writer.js');
    const indexPath = path.join(__dirname, 'index.js');
    fs.writeFileSync(script, `
      const { writeState } = require(${JSON.stringify(indexPath)});
      const [sessionId, field, value, dir, iters] = process.argv.slice(2);
      const N = parseInt(iters, 10);
      for (let i = 0; i < N; i++) {
        writeState(sessionId, { [field]: value + '-' + i }, dir);
      }
    `);

    // Three workers each owning a distinct field, all hitting the same file.
    const writers = [
      ['pinned_state', 'review'],
      ['branch', 'feat/x'],
      ['worktree_cwd', '/wt'],
    ];
    await Promise.all(writers.map(([field, value]) =>
      execFileP(process.execPath, [script, sessionId, field, value, agentsDir, String(ITERS)])
    ));

    const state = readAgentState(sessionId, agentsDir);
    assert.ok(state, 'agent file gone after concurrent writes');
    // The race manifests as a stale snapshot clobbering a more recent write.
    // After every process returns, each field's LAST value (suffix N-1) must
    // be on disk — anything less means another writer's stale merge wiped it.
    const lastSuffix = `-${ITERS - 1}`;
    assert.equal(state.pinned_state, 'review' + lastSuffix,
      `pinned_state lost-update: got ${JSON.stringify(state.pinned_state)}`);
    assert.equal(state.branch, 'feat/x' + lastSuffix,
      `branch lost-update: got ${JSON.stringify(state.branch)}`);
    assert.equal(state.worktree_cwd, '/wt' + lastSuffix,
      `worktree_cwd lost-update: got ${JSON.stringify(state.worktree_cwd)}`);
    // Baseline must also survive.
    assert.equal(state.session_id, sessionId);
    assert.equal(state.target, 'a:0.1');
  });
});

describe('writeState guardStates', () => {
  it('skips write when on-disk state matches guardStates', () => {
    const sessionId = 'sess-guard';
    writeState(sessionId, { target: 'a:0.1', session_id: sessionId, state: 'idle_prompt' }, agentsDir);

    // Attempt to overwrite idle_prompt → running, guarded by STOP_STATES
    const guardStates = new Set(['idle_prompt', 'done', 'question', 'waiting_input', 'plan']);
    writeState(sessionId, { state: 'running' }, agentsDir, { guardStates });

    const state = readAgentState(sessionId, agentsDir);
    assert.equal(state.state, 'idle_prompt', 'guardStates should prevent overwriting idle_prompt');
  });

  it('allows write when on-disk state does not match guardStates', () => {
    const sessionId = 'sess-guard-pass';
    writeState(sessionId, { target: 'a:0.1', session_id: sessionId, state: 'running' }, agentsDir);

    const guardStates = new Set(['idle_prompt', 'done', 'question', 'waiting_input', 'plan']);
    writeState(sessionId, { state: 'permission' }, agentsDir, { guardStates });

    const state = readAgentState(sessionId, agentsDir);
    assert.equal(state.state, 'permission', 'write should proceed when state is not guarded');
  });

  it('allows write when guardStates is not provided', () => {
    const sessionId = 'sess-no-guard';
    writeState(sessionId, { target: 'a:0.1', session_id: sessionId, state: 'idle_prompt' }, agentsDir);

    writeState(sessionId, { state: 'running' }, agentsDir);

    const state = readAgentState(sessionId, agentsDir);
    assert.equal(state.state, 'running', 'without guardStates, write should proceed normally');
  });

  it('preserves guarded state while merging non-state fields when requested', () => {
    const sessionId = 'sess-guard-preserve';
    writeState(sessionId, {
      target: 'a:0.1',
      session_id: sessionId,
      state: 'idle_prompt',
      subagent_count: 1,
      files_changed: ['old'],
    }, agentsDir);

    const guardStates = new Set(['idle_prompt', 'done', 'question', 'waiting_input', 'plan']);
    writeState(sessionId, {
      state: 'running',
      subagent_count: 0,
      files_changed: ['fresh'],
    }, agentsDir, { guardStates, preserveGuardedState: true });

    const state = readAgentState(sessionId, agentsDir);
    assert.equal(state.state, 'idle_prompt', 'guarded stop state must remain authoritative');
    assert.equal(state.subagent_count, 0, 'subagent metadata should still merge');
    assert.deepEqual(state.files_changed, ['fresh'], 'fresh file changes should still merge');
  });
});

describe('writeState report_seq ordering', () => {
  it('drops a write with a strictly-older report_seq', () => {
    const sessionId = 'sess-seq-older';
    writeState(sessionId, { target: 'a:0.1', session_id: sessionId, state: 'idle_prompt', report_seq: 200 }, agentsDir);

    // A stale write (older event) that lands later must lose.
    writeState(sessionId, { state: 'running', report_seq: 100 }, agentsDir);

    const state = readAgentState(sessionId, agentsDir);
    assert.equal(state.state, 'idle_prompt', 'older report_seq must not overwrite a newer state');
    assert.equal(state.report_seq, 200, 'report_seq must remain the newest');
  });

  it('applies a write with a newer report_seq', () => {
    const sessionId = 'sess-seq-newer';
    writeState(sessionId, { target: 'a:0.1', session_id: sessionId, state: 'running', report_seq: 100 }, agentsDir);

    writeState(sessionId, { state: 'idle_prompt', report_seq: 200 }, agentsDir);

    const state = readAgentState(sessionId, agentsDir);
    assert.equal(state.state, 'idle_prompt', 'newer report_seq must apply');
    assert.equal(state.report_seq, 200);
  });

  it('rejects a stale running after a newer idle_prompt with no guardStates', () => {
    // The exact race seq is meant to kill: a late PostToolUse->running landing
    // after a Stop->idle_prompt, with NO content guard supplied.
    const sessionId = 'sess-seq-race';
    writeState(sessionId, { target: 'a:0.1', session_id: sessionId, state: 'idle_prompt', report_seq: 5000 }, agentsDir);

    writeState(sessionId, { state: 'running', report_seq: 4999 }, agentsDir);

    const state = readAgentState(sessionId, agentsDir);
    assert.equal(state.state, 'idle_prompt', 'seq alone (no guard) must reject the stale running');
  });

  it('merges older preserved guarded-state metadata without moving report_seq backward', () => {
    const sessionId = 'sess-seq-older-preserve';
    writeState(sessionId, {
      target: 'a:0.1',
      session_id: sessionId,
      state: 'idle_prompt',
      report_seq: 5000,
      subagent_count: 1,
      files_changed: ['old'],
    }, agentsDir);

    const guardStates = new Set(['idle_prompt', 'done', 'question', 'waiting_input', 'plan']);
    writeState(sessionId, {
      state: 'running',
      report_seq: 4999,
      subagent_count: 0,
      files_changed: ['fresh'],
    }, agentsDir, { guardStates, preserveGuardedState: true });

    const state = readAgentState(sessionId, agentsDir);
    assert.equal(state.state, 'idle_prompt', 'guarded stop state must remain authoritative');
    assert.equal(state.report_seq, 5000, 'older metadata refresh must not move report_seq backward');
    assert.equal(state.subagent_count, 0, 'final subagent metadata should still merge');
    assert.deepEqual(state.files_changed, ['fresh'], 'fresh final file changes should still merge');
  });

  it('allows a write with an equal report_seq (not strictly older)', () => {
    const sessionId = 'sess-seq-equal';
    writeState(sessionId, { target: 'a:0.1', session_id: sessionId, state: 'running', report_seq: 100 }, agentsDir);

    writeState(sessionId, { state: 'permission', report_seq: 100 }, agentsDir);

    const state = readAgentState(sessionId, agentsDir);
    assert.equal(state.state, 'permission', 'equal report_seq is not strictly older; the compare is skipped so the write proceeds');
  });

  it('allows a write when report_seq is absent (degrades to current behavior)', () => {
    const sessionId = 'sess-seq-missing';
    writeState(sessionId, { target: 'a:0.1', session_id: sessionId, state: 'idle_prompt', report_seq: 100 }, agentsDir);

    // No report_seq on the second write → compare skipped → normal merge.
    writeState(sessionId, { state: 'running' }, agentsDir);

    const state = readAgentState(sessionId, agentsDir);
    assert.equal(state.state, 'running', 'without report_seq the write proceeds (unchanged behavior)');
  });
});

describe('writeState report_seq freeze guard', () => {
  it('self-heals when the on-disk report_seq is implausibly far in the future', () => {
    const sessionId = 'sess-seq-future';
    const farFuture = Date.now() * 1000 + 3_600_000_000; // ~1h ahead, well past the slack
    writeState(sessionId, { target: 'a:0.1', session_id: sessionId, state: 'idle_prompt', report_seq: farFuture }, agentsDir);

    // A normal, current write must NOT be frozen out by the bogus future seq.
    writeState(sessionId, { state: 'running', report_seq: Date.now() * 1000 }, agentsDir);

    const state = readAgentState(sessionId, agentsDir);
    assert.equal(state.state, 'running', 'an implausibly-future on-disk seq must not freeze writes');
  });

  it('ignores a non-finite (Infinity) on-disk report_seq', () => {
    const sessionId = 'sess-seq-inf';
    // JSON.parse turns an out-of-range literal into Infinity; hooks can't write
    // this (JSON.stringify(Infinity)==='null'), but a hand-edited / restored
    // file can. Write the raw literal so the file genuinely parses to Infinity.
    fs.mkdirSync(agentsDir, { recursive: true });
    fs.writeFileSync(path.join(agentsDir, sessionId + '.json'),
      '{"target":"a:0.1","session_id":"' + sessionId + '","state":"idle_prompt","report_seq":1e309}');

    writeState(sessionId, { state: 'running', report_seq: Date.now() * 1000 }, agentsDir);

    const state = readAgentState(sessionId, agentsDir);
    assert.equal(state.state, 'running', 'a non-finite on-disk seq must not freeze writes');
  });
});

describe('cleanStale', () => {
  it('removes agent files older than threshold', () => {
    const old = new Date(Date.now() - 600000).toISOString(); // 10 min ago
    const fresh = new Date().toISOString();

    const sessOld = 'sess-old';
    const sessFresh = 'sess-fresh';
    writeState(sessOld, { target: 'old:0.1', session_id: sessOld, state: 'done', updated_at: old }, agentsDir);
    writeState(sessFresh, { target: 'fresh:0.1', session_id: sessFresh, state: 'running', updated_at: fresh }, agentsDir);

    // Force the old agent's updated_at to be old (writeState auto-sets updated_at)
    const oldFile = path.join(agentsDir, sessOld + '.json');
    const oldData = JSON.parse(fs.readFileSync(oldFile, 'utf8'));
    oldData.updated_at = old;
    fs.writeFileSync(oldFile, JSON.stringify(oldData));

    cleanStale(300000, agentsDir); // 5 min threshold

    const state = readAllState(agentsDir);
    assert.equal(Object.keys(state.agents).length, 1);
    assert.ok(state.agents[sessFresh]);
    assert.equal(state.agents[sessOld], undefined);
  });

  it('no-ops when no stale agents', () => {
    const sess = 'sess-001';
    writeState(sess, { target: 'a:0.1', session_id: sess, state: 'running' }, agentsDir);

    cleanStale(300000, agentsDir);
    const state = readAllState(agentsDir);
    assert.equal(Object.keys(state.agents).length, 1);
  });

  it('no-ops when directory does not exist', () => {
    // Should not throw
    cleanStale(300000, path.join(tmpDir, 'nonexistent'));
  });
});
