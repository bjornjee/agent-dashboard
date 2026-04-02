'use strict';

const { describe, it } = require('node:test');
const assert = require('node:assert/strict');

describe('packages/tmux', () => {
  describe('capture', () => {
    it('returns empty array when tmux fails', () => {
      const cp = require('child_process');
      const orig = cp.spawnSync;
      cp.spawnSync = () => ({ status: 1, stdout: '' });

      delete require.cache[require.resolve('./index')];
      const { capture } = require('./index');

      assert.deepEqual(capture('test:0.1'), []);

      cp.spawnSync = orig;
      delete require.cache[require.resolve('./index')];
    });

    it('returns empty array when spawnSync throws', () => {
      const cp = require('child_process');
      const orig = cp.spawnSync;
      cp.spawnSync = () => { throw new Error('ENOENT'); };

      delete require.cache[require.resolve('./index')];
      const { capture } = require('./index');

      assert.deepEqual(capture('test:0.1'), []);

      cp.spawnSync = orig;
      delete require.cache[require.resolve('./index')];
    });

    it('splits stdout into lines', () => {
      const cp = require('child_process');
      const orig = cp.spawnSync;
      cp.spawnSync = () => ({ status: 0, stdout: 'line1\nline2\nline3' });

      delete require.cache[require.resolve('./index')];
      const { capture } = require('./index');

      assert.deepEqual(capture('test:0.1', 3), ['line1', 'line2', 'line3']);

      cp.spawnSync = orig;
      delete require.cache[require.resolve('./index')];
    });
  });

  describe('jump', () => {
    it('calls select-window and select-pane with correct targets', () => {
      const cp = require('child_process');
      const orig = cp.spawnSync;
      const calls = [];
      cp.spawnSync = (cmd, args, opts) => {
        calls.push({ cmd, args });
        return { status: 0 };
      };

      delete require.cache[require.resolve('./index')];
      const { jump } = require('./index');

      const ok = jump('my.session:0.1');

      assert.equal(ok, true);
      assert.equal(calls.length, 2);
      assert.deepEqual(calls[0].args, ['select-window', '-t', 'my.session:0']);
      assert.deepEqual(calls[1].args, ['select-pane', '-t', 'my.session:0.1']);

      cp.spawnSync = orig;
      delete require.cache[require.resolve('./index')];
    });

    it('returns false when target does not exist', () => {
      const cp = require('child_process');
      const orig = cp.spawnSync;
      cp.spawnSync = () => ({ status: 1 });

      delete require.cache[require.resolve('./index')];
      const { jump } = require('./index');

      assert.equal(jump('bad:0.1'), false);

      cp.spawnSync = orig;
      delete require.cache[require.resolve('./index')];
    });

    it('returns false when spawnSync throws', () => {
      const cp = require('child_process');
      const orig = cp.spawnSync;
      cp.spawnSync = () => { throw new Error('ENOENT'); };

      delete require.cache[require.resolve('./index')];
      const { jump } = require('./index');

      assert.equal(jump('bad:0.1'), false);

      cp.spawnSync = orig;
      delete require.cache[require.resolve('./index')];
    });

    it('handles target without pane index', () => {
      const cp = require('child_process');
      const orig = cp.spawnSync;
      const calls = [];
      cp.spawnSync = (cmd, args) => {
        calls.push({ cmd, args });
        return { status: 0 };
      };

      delete require.cache[require.resolve('./index')];
      const { jump } = require('./index');

      jump('main:0');

      assert.deepEqual(calls[0].args, ['select-window', '-t', 'main:0']);
      assert.deepEqual(calls[1].args, ['select-pane', '-t', 'main:0']);

      cp.spawnSync = orig;
      delete require.cache[require.resolve('./index')];
    });
  });

  describe('sendKeys', () => {
    it('sends text with Enter to target pane', () => {
      const cp = require('child_process');
      const orig = cp.spawnSync;
      const calls = [];
      cp.spawnSync = (cmd, args) => {
        calls.push({ cmd, args });
        return { status: 0 };
      };

      delete require.cache[require.resolve('./index')];
      const { sendKeys } = require('./index');

      const ok = sendKeys('api:0.1', 'use firebase');

      assert.equal(ok, true);
      assert.equal(calls.length, 1);
      assert.deepEqual(calls[0].args, ['send-keys', '-t', 'api:0.1', 'use firebase', 'Enter']);

      cp.spawnSync = orig;
      delete require.cache[require.resolve('./index')];
    });

    it('returns false when target does not exist', () => {
      const cp = require('child_process');
      const orig = cp.spawnSync;
      cp.spawnSync = () => ({ status: 1 });

      delete require.cache[require.resolve('./index')];
      const { sendKeys } = require('./index');

      assert.equal(sendKeys('bad:0.1', 'hello'), false);

      cp.spawnSync = orig;
      delete require.cache[require.resolve('./index')];
    });
  });

  describe('listPanes', () => {
    it('parses tmux list-panes output', () => {
      const cp = require('child_process');
      const orig = cp.spawnSync;
      cp.spawnSync = () => ({
        status: 0,
        stdout: 'main:0.0\tmain\t0\t0\tnode\napi:1.0\tapi\t1\t0\tclaude\n',
      });

      delete require.cache[require.resolve('./index')];
      const { listPanes } = require('./index');

      const panes = listPanes();
      assert.equal(panes.length, 2);
      assert.deepEqual(panes[0], {
        target: 'main:0.0',
        session: 'main',
        window: 0,
        pane: 0,
        command: 'node',
      });

      cp.spawnSync = orig;
      delete require.cache[require.resolve('./index')];
    });

    it('returns empty array when tmux not available', () => {
      const cp = require('child_process');
      const orig = cp.spawnSync;
      cp.spawnSync = () => ({ status: 1, stdout: '' });

      delete require.cache[require.resolve('./index')];
      const { listPanes } = require('./index');

      assert.deepEqual(listPanes(), []);

      cp.spawnSync = orig;
      delete require.cache[require.resolve('./index')];
    });
  });

  describe('getTarget', () => {
    it('returns null for empty paneId', () => {
      delete require.cache[require.resolve('./index')];
      const { getTarget } = require('./index');
      assert.equal(getTarget(null), null);
      assert.equal(getTarget(''), null);
    });

    it('resolves pane id to full target', () => {
      const cp = require('child_process');
      const orig = cp.spawnSync;
      cp.spawnSync = () => ({ status: 0, stdout: 'dev:2.1\n' });

      delete require.cache[require.resolve('./index')];
      const { getTarget } = require('./index');

      assert.equal(getTarget('%5'), 'dev:2.1');

      cp.spawnSync = orig;
      delete require.cache[require.resolve('./index')];
    });
  });

  describe('isAvailable', () => {
    it('returns false when tmux is not running', () => {
      const cp = require('child_process');
      const orig = cp.spawnSync;
      cp.spawnSync = () => ({ status: 1 });

      delete require.cache[require.resolve('./index')];
      const { isAvailable } = require('./index');

      assert.equal(isAvailable(), false);

      cp.spawnSync = orig;
      delete require.cache[require.resolve('./index')];
    });

    it('returns false when spawnSync throws', () => {
      const cp = require('child_process');
      const orig = cp.spawnSync;
      cp.spawnSync = () => { throw new Error('ENOENT'); };

      delete require.cache[require.resolve('./index')];
      const { isAvailable } = require('./index');

      assert.equal(isAvailable(), false);

      cp.spawnSync = orig;
      delete require.cache[require.resolve('./index')];
    });

    it('returns true when tmux is running', () => {
      const cp = require('child_process');
      const orig = cp.spawnSync;
      cp.spawnSync = () => ({ status: 0 });

      delete require.cache[require.resolve('./index')];
      const { isAvailable } = require('./index');

      assert.equal(isAvailable(), true);

      cp.spawnSync = orig;
      delete require.cache[require.resolve('./index')];
    });
  });

  describe('extractSessionWindow', () => {
    it('extracts session:window from simple target', () => {
      delete require.cache[require.resolve('./index')];
      const { extractSessionWindow } = require('./index');
      assert.equal(extractSessionWindow('main:0.1'), 'main:0');
    });

    it('handles session names with dots', () => {
      delete require.cache[require.resolve('./index')];
      const { extractSessionWindow } = require('./index');
      assert.equal(extractSessionWindow('my.project:0.1'), 'my.project:0');
    });

    it('returns input when no dot present', () => {
      delete require.cache[require.resolve('./index')];
      const { extractSessionWindow } = require('./index');
      assert.equal(extractSessionWindow('main:0'), 'main:0');
    });
  });

  describe('parseTarget', () => {
    it('parses simple target', () => {
      delete require.cache[require.resolve('./index')];
      const { parseTarget } = require('./index');
      assert.deepEqual(parseTarget('main:0.1'), { session: 'main', window: 0, pane: 1 });
    });

    it('parses dotted session name', () => {
      delete require.cache[require.resolve('./index')];
      const { parseTarget } = require('./index');
      assert.deepEqual(parseTarget('my.project:2.3'), { session: 'my.project', window: 2, pane: 3 });
    });

    it('handles target without pane', () => {
      delete require.cache[require.resolve('./index')];
      const { parseTarget } = require('./index');
      assert.deepEqual(parseTarget('main:0'), { session: 'main', window: 0, pane: 0 });
    });
  });
});
