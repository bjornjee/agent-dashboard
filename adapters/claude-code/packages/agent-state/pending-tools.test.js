'use strict';

const { describe, it } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('fs');
const os = require('os');
const path = require('path');

const { hasPendingParentToolUse } = require('./pending-tools');

function writeJsonl(lines) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'pending-tools-'));
  const file = path.join(dir, 'session.jsonl');
  fs.writeFileSync(file, lines.map(l => JSON.stringify(l)).join('\n') + '\n');
  return { file, dir };
}

function cleanup(dir) {
  fs.rmSync(dir, { recursive: true, force: true });
}

const ASSISTANT_TOOL_USE = (id, name = 'Bash') => ({
  type: 'assistant',
  isSidechain: false,
  message: {
    role: 'assistant',
    content: [{ type: 'tool_use', id, name, input: {} }],
  },
});

const USER_TOOL_RESULT = (toolUseId) => ({
  type: 'user',
  isSidechain: false,
  message: {
    role: 'user',
    content: [{ type: 'tool_result', tool_use_id: toolUseId, content: 'ok' }],
  },
});

describe('hasPendingParentToolUse', () => {
  it('returns true when last tool_use has no matching tool_result', () => {
    const { file, dir } = writeJsonl([
      ASSISTANT_TOOL_USE('toolu_001'),
      USER_TOOL_RESULT('toolu_001'),
      ASSISTANT_TOOL_USE('toolu_002'),
      // toolu_002 has no tool_result yet
    ]);
    try {
      assert.equal(hasPendingParentToolUse(file), true);
    } finally {
      cleanup(dir);
    }
  });

  it('returns false when every tool_use has a matching tool_result', () => {
    const { file, dir } = writeJsonl([
      ASSISTANT_TOOL_USE('toolu_001'),
      USER_TOOL_RESULT('toolu_001'),
      ASSISTANT_TOOL_USE('toolu_002'),
      USER_TOOL_RESULT('toolu_002'),
    ]);
    try {
      assert.equal(hasPendingParentToolUse(file), false);
    } finally {
      cleanup(dir);
    }
  });

  it('handles parallel tool_use blocks in one assistant message', () => {
    const { file, dir } = writeJsonl([
      {
        type: 'assistant',
        isSidechain: false,
        message: {
          role: 'assistant',
          content: [
            { type: 'tool_use', id: 'toolu_p1', name: 'Read', input: {} },
            { type: 'tool_use', id: 'toolu_p2', name: 'Read', input: {} },
          ],
        },
      },
      USER_TOOL_RESULT('toolu_p1'),
      // toolu_p2 still pending
    ]);
    try {
      assert.equal(hasPendingParentToolUse(file), true);
    } finally {
      cleanup(dir);
    }
  });

  it('skips malformed JSONL lines and continues', () => {
    const { file, dir } = writeJsonl([
      ASSISTANT_TOOL_USE('toolu_001'),
      USER_TOOL_RESULT('toolu_001'),
      ASSISTANT_TOOL_USE('toolu_002'),
    ]);
    // Inject malformed line
    fs.appendFileSync(file, 'not-json-at-all\n');
    fs.appendFileSync(file, JSON.stringify(USER_TOOL_RESULT('toolu_002')) + '\n');
    try {
      assert.equal(hasPendingParentToolUse(file), false);
    } finally {
      cleanup(dir);
    }
  });

  it('returns false when transcriptPath is empty or missing', () => {
    assert.equal(hasPendingParentToolUse(''), false);
    assert.equal(hasPendingParentToolUse(null), false);
    assert.equal(hasPendingParentToolUse(undefined), false);
    assert.equal(hasPendingParentToolUse('/nonexistent/path/file.jsonl'), false);
  });

  it('returns false on empty file', () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'pending-tools-'));
    const file = path.join(dir, 'empty.jsonl');
    fs.writeFileSync(file, '');
    try {
      assert.equal(hasPendingParentToolUse(file), false);
    } finally {
      cleanup(dir);
    }
  });

  it('correctly handles file larger than tail window', () => {
    const lines = [];
    for (let i = 0; i < 500; i++) {
      lines.push(ASSISTANT_TOOL_USE(`toolu_pad_${i}`));
      lines.push(USER_TOOL_RESULT(`toolu_pad_${i}`));
    }
    lines.push(ASSISTANT_TOOL_USE('toolu_final'));
    const { file, dir } = writeJsonl(lines);
    try {
      assert.equal(hasPendingParentToolUse(file), true);
    } finally {
      cleanup(dir);
    }
  });

  it('ignores orphan tool_result whose tool_use scrolled past the tail', () => {
    const lines = [];
    for (let i = 0; i < 500; i++) {
      lines.push(ASSISTANT_TOOL_USE(`toolu_pad_${i}`));
      lines.push(USER_TOOL_RESULT(`toolu_pad_${i}`));
    }
    lines.push(USER_TOOL_RESULT('toolu_orphan_far_back'));
    const { file, dir } = writeJsonl(lines);
    try {
      assert.equal(hasPendingParentToolUse(file), false);
    } finally {
      cleanup(dir);
    }
  });
});
