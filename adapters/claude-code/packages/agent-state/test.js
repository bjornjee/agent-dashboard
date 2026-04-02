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
    ];
    const sorted = sortAgentsByPriority(agents);
    const states = sorted.map(a => a.state);
    // permission (1) → error+question (2, stable order from input) → running (3) → done+idle_prompt (4, stable order from input)
    assert.deepEqual(states, ['permission', 'error', 'question', 'running', 'done', 'idle_prompt']);
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
