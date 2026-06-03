// Unit tests for pure helpers in pages/detail.js.

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { pathToFileURL } = require('node:url');
const path = require('node:path');

let questionCardId;
let renderQuestionCard;

test('load module', async () => {
  const url = pathToFileURL(path.join(__dirname, 'detail.js')).href;
  const mod = await import(url);
  questionCardId = mod.questionCardId;
  renderQuestionCard = mod.renderQuestionCard;
  assert.equal(typeof questionCardId, 'function');
  assert.equal(typeof renderQuestionCard, 'function');
});

test('questionCardId prefers real tool_use_id', () => {
  const pending = {
    tool_use_id: 'toolu_123',
    questions: [{ question: 'Which?', options: [{ label: 'A' }] }],
  };
  assert.equal(questionCardId(pending), 'toolu_123');
});

test('questionCardId is stable when tool_use_id is blank', () => {
  const pending = {
    tool_use_id: '',
    questions: [{
      question: 'Plan has 3 phases. Continue inline here, or hand off?',
      header: 'Dispatch',
      multi_select: false,
      options: [
        { label: 'Continue inline (Recommended)', description: 'Stay here.' },
        { label: 'Hand off', description: 'Use implementation dispatch.' },
      ],
    }],
  };
  assert.equal(questionCardId(pending), questionCardId({ ...pending }));
  assert.match(questionCardId(pending), /^qc-[a-z0-9]+$/);
});

test('renderQuestionCard renders a card when tool_use_id is blank', () => {
  const pending = {
    tool_use_id: '',
    questions: [{
      question: 'Plan has 3 phases. Continue inline here, or hand off?',
      header: 'Dispatch',
      multi_select: false,
      options: [
        { label: 'Continue inline (Recommended)', description: 'Stay here.' },
        { label: 'Hand off', description: 'Use implementation dispatch.' },
      ],
    }],
  };

  const html = renderQuestionCard(pending, 'agent-1');
  assert.match(html, /class="question-card"/);
  assert.match(html, /data-tool-use-id="[^"]+"/);
  assert.match(html, /Plan has 3 phases/);
});
