import test from 'node:test';
import assert from 'node:assert/strict';
import { demolishLabel, demolishPrompt, demolishResultLine } from './demolish.js';

test('label names the hex for hex buildings only', () => {
  assert.equal(demolishLabel('Farm', 1, 0), 'Farm @ (1,0)');
  assert.equal(demolishLabel('Farm', 0, 0), 'Farm @ (0,0)');
  assert.equal(demolishLabel('Market', null, undefined), 'Market');
});

test('prompt says no refund and that it cannot be undone', () => {
  const p = demolishPrompt('Farm @ (1,0)');
  assert.match(p, /Farm @ \(1,0\)/);
  assert.match(p, /Nothing is refunded/);
  assert.match(p, /cannot be undone/);
});

test('result line pluralises the idle citizens', () => {
  assert.match(demolishResultLine('Mine', 0), /No one was working there/);
  assert.match(demolishResultLine('Mine', 1), /1 citizen is idle now/);
  assert.match(demolishResultLine('Mine', 3), /3 citizens are idle now/);
  assert.match(demolishResultLine('Mine', undefined), /No one was working there/);
});
