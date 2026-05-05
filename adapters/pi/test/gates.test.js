'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');

const { runGate, runGates } = require('../lib/gates');

function fakeSpawnSync(scripts) {
  return (cmd, args /* , opts */) => {
    const scriptPath = args[0];
    const handler = scripts[scriptPath];
    if (!handler) return { status: 0, stderr: Buffer.from('') };
    return handler();
  };
}

test('runGate: exit 0 → allow (no result)', () => {
  const spawn = fakeSpawnSync({
    '/g/foo.js': () => ({ status: 0, stderr: Buffer.from('') }),
  });

  const result = runGate('/g/foo.js', { tool_name: 'Bash' }, { spawn });
  assert.equal(result, null);
});

test('runGate: exit 2 → block with stderr as reason', () => {
  const spawn = fakeSpawnSync({
    '/g/destructive.js': () => ({
      status: 2,
      stderr: Buffer.from('Blocked: rm -rf is dangerous\n'),
    }),
  });

  const result = runGate('/g/destructive.js', { tool_name: 'Bash' }, { spawn });
  assert.deepEqual(result, {
    block: true,
    reason: 'Blocked: rm -rf is dangerous',
  });
});

test('runGate: non-zero non-2 exit → null (not a gate failure, just script error)', () => {
  const spawn = fakeSpawnSync({
    '/g/broken.js': () => ({ status: 1, stderr: Buffer.from('crashed') }),
  });

  const result = runGate('/g/broken.js', {}, { spawn });
  assert.equal(result, null);
});

test('runGate: spawn throws → null (gate failure must not break pi)', () => {
  const spawn = () => { throw new Error('ENOENT'); };

  const result = runGate('/g/missing.js', {}, { spawn });
  assert.equal(result, null);
});

test('runGate: stdin payload is JSON-stringified', () => {
  let captured = null;
  const spawn = (cmd, args, opts) => {
    captured = { cmd, args, opts };
    return { status: 0, stderr: Buffer.from('') };
  };

  runGate('/g/foo.js', { tool_name: 'Bash', cwd: '/x' }, { spawn });

  assert.equal(captured.cmd, 'node');
  assert.equal(captured.args[0], '/g/foo.js');
  assert.equal(captured.opts.input, JSON.stringify({ tool_name: 'Bash', cwd: '/x' }));
});

test('runGates: runs scripts in order, returns first block', () => {
  const calls = [];
  const spawn = (cmd, args) => {
    calls.push(args[0]);
    if (args[0] === '/g/second.js') {
      return { status: 2, stderr: Buffer.from('block by second') };
    }
    return { status: 0, stderr: Buffer.from('') };
  };

  const result = runGates(
    ['/g/first.js', '/g/second.js', '/g/third.js'],
    { tool_name: 'Bash' },
    { spawn },
  );

  assert.deepEqual(result, { block: true, reason: 'block by second' });
  assert.deepEqual(calls, ['/g/first.js', '/g/second.js']);
});

test('runGates: all pass → null', () => {
  const spawn = () => ({ status: 0, stderr: Buffer.from('') });

  const result = runGates(['/a.js', '/b.js'], {}, { spawn });
  assert.equal(result, null);
});

test('POST_BASH_GATES: includes commit-lint and pr-detect', () => {
  const { POST_BASH_GATES } = require('../lib/gates');
  const names = POST_BASH_GATES.map(p => p.split('/').pop());
  assert.ok(names.includes('commit-lint.js'), `missing commit-lint.js in: ${names.join(',')}`);
  assert.ok(names.includes('pr-detect.js'), `missing pr-detect.js in: ${names.join(',')}`);
});
