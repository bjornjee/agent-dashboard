'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');

const { writePiState, readPiState } = require('../lib/state-bridge');

function tmpDir() {
  return fs.mkdtempSync(path.join(os.tmpdir(), 'pi-bridge-test-'));
}

test('writePiState: writes JSON with agent_id="pi" and required fields', () => {
  const dir = tmpDir();
  writePiState('sess-1', { target: 'a:0.1', state: 'running' }, dir);

  const raw = fs.readFileSync(path.join(dir, 'sess-1.json'), 'utf8');
  const parsed = JSON.parse(raw);

  assert.equal(parsed.agent_id, 'pi');
  assert.equal(parsed.session_id, 'sess-1');
  assert.equal(parsed.target, 'a:0.1');
  assert.equal(parsed.state, 'running');
  assert.ok(parsed.updated_at);
});

test('writePiState: merges with existing state, preserves agent_id', () => {
  const dir = tmpDir();
  writePiState('sess-1', { target: 'a:0.1', state: 'running' }, dir);
  writePiState('sess-1', { state: 'done' }, dir);

  const parsed = JSON.parse(fs.readFileSync(path.join(dir, 'sess-1.json'), 'utf8'));
  assert.equal(parsed.agent_id, 'pi');
  assert.equal(parsed.target, 'a:0.1');
  assert.equal(parsed.state, 'done');
});

test('writePiState: caller cannot override agent_id away from "pi"', () => {
  const dir = tmpDir();
  writePiState('sess-1', { target: 't', state: 'running', agent_id: 'claude' }, dir);

  const parsed = JSON.parse(fs.readFileSync(path.join(dir, 'sess-1.json'), 'utf8'));
  assert.equal(parsed.agent_id, 'pi');
});

test('readPiState: returns null when no file', () => {
  const dir = tmpDir();
  assert.equal(readPiState('missing', dir), null);
});

test('readPiState: round-trips written state', () => {
  const dir = tmpDir();
  writePiState('s', { target: 'a:0.1', state: 'idle_prompt' }, dir);
  const got = readPiState('s', dir);
  assert.equal(got.state, 'idle_prompt');
  assert.equal(got.agent_id, 'pi');
});
