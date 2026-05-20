'use strict';

const { describe, it } = require('node:test');
const assert = require('node:assert/strict');

const { parse } = require('./index');

describe('toml-lite parse', () => {
  it('returns an empty object for empty input', () => {
    assert.deepEqual(parse(''), {});
  });

  it('parses top-level keys before any section', () => {
    const r = parse('name = "demo"\ncount = 3\n');
    assert.deepEqual(r, { name: 'demo', count: 3 });
  });

  it('parses string, integer, and boolean values', () => {
    const r = parse([
      '[s]',
      'str    = "hello"',
      'num    = 42',
      'neg    = -7',
      'on     = true',
      'off    = false',
    ].join('\n'));
    assert.deepEqual(r.s, { str: 'hello', num: 42, neg: -7, on: true, off: false });
  });

  it('parses literal (single-quoted) strings without unescaping', () => {
    const r = parse(`[s]\npath = 'C:\\Users\\demo'\n`);
    assert.equal(r.s.path, 'C:\\Users\\demo');
  });

  it('unescapes basic strings (\\n \\t \\\\ \\")', () => {
    const r = parse('[s]\nmsg = "line1\\nline2\\t\\"quoted\\"\\\\"\n');
    assert.equal(r.s.msg, 'line1\nline2\t"quoted"\\');
  });

  it('isolates keys per section', () => {
    const r = parse([
      '[a]',
      'k = "from-a"',
      '[b]',
      'k = "from-b"',
    ].join('\n'));
    assert.equal(r.a.k, 'from-a');
    assert.equal(r.b.k, 'from-b');
  });

  it('strips trailing line comments', () => {
    const r = parse('[s]\nk = "v"  # trailing comment\n');
    assert.equal(r.s.k, 'v');
  });

  it('does not treat # inside a string literal as a comment', () => {
    const r = parse('[s]\nk = "a # b"\n');
    assert.equal(r.s.k, 'a # b');
  });

  it('skips full-line comments and blank lines', () => {
    const r = parse([
      '# top-level comment',
      '',
      '[s]',
      '# section comment',
      '',
      'k = "v"',
    ].join('\n'));
    assert.equal(r.s.k, 'v');
  });

  it('tolerates whitespace around = and around section names', () => {
    const r = parse('[ s ]\n  k    =    "v"\n');
    assert.equal(r.s.k, 'v');
  });

  it('ignores malformed lines (no =, missing close quote, unsupported value)', () => {
    const r = parse([
      '[s]',
      'no_equals',
      'unterminated = "oops',
      'array = [1, 2]',
      'ok = "yes"',
    ].join('\n'));
    assert.equal(r.s.ok, 'yes');
    assert.equal(r.s.no_equals, undefined);
    assert.equal(r.s.unterminated, undefined);
    assert.equal(r.s.array, undefined);
  });

  it('parses the project settings.example.toml shape', () => {
    const r = parse([
      '[banner]',
      'show_mascot = true',
      'show_quote  = true',
      '',
      '[usage]',
      'rate_limit_poll_seconds = 60',
      '',
      '[effort]',
      'plan    = "max"',
      'default = "high"',
    ].join('\n'));
    assert.equal(r.banner.show_mascot, true);
    assert.equal(r.banner.show_quote, true);
    assert.equal(r.usage.rate_limit_poll_seconds, 60);
    assert.equal(r.effort.plan, 'max');
    assert.equal(r.effort.default, 'high');
  });
});
