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

test('temple confirmation and result use actual server loss and ceiling, including floor', () => {
  const quote = {god_id:'potnia', level:3, kharis_loss:4, kharis_before:5, kharis_after:1, cap_before:100, cap_after:50};
  for (const text of [demolishPrompt('Temple of Potnia', quote), demolishResultLine('Temple of Potnia', 2, quote)]) {
    assert.match(text, /potnia is angered/);
    assert.match(text, /−4 \(5 → 1; floor 1\)/);
    assert.match(text, /100 → 50/);
    assert.match(text, /does not cut existing kharis/);
    assert.doesNotMatch(text, /−30/);
  }
});
