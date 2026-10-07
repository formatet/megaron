import test from 'node:test';
import assert from 'node:assert/strict';
import { State } from '../state.js';
import { arrivalHTML, fmtArrival, fmtEta } from './time.js';

test('I: missing or invalid arrival never prints Invalid Date or NaNm', () => {
  State.TICK_SECONDS = 6;
  State.TICK_ANCHOR_MS = Date.now();
  State.CURRENT_TICK = 100;
  for (const value of [undefined, null, '', 'bad-date']) {
    assert.equal(arrivalHTML(value), '', 'unknown arrival must be empty');
    assert.equal(fmtArrival(value), '');
    assert.equal(fmtEta(value), '');
  }
  assert.equal(arrivalHTML(undefined, NaN), '');
  assert.equal(arrivalHTML(undefined, Infinity), '');
});

test('I: authoritative tick still works without ISO and overrides invalid ISO', () => {
  State.TICK_SECONDS = 3600;
  State.TICK_ANCHOR_MS = Date.now();
  State.CURRENT_TICK = 100;
  for (const iso of [undefined, 'bad-date']) {
    assert.match(arrivalHTML(iso, 102), /<span title=/);
    assert.doesNotMatch(arrivalHTML(iso, 102), /Invalid Date|NaN/);
    assert.equal(arrivalHTML(iso, 99), 'arrived');
    assert.equal(arrivalHTML(iso, 99, 'ready'), 'ready');
  }
});

test('I: valid ISO fallback still has arrival and completed wording', () => {
  State.TICK_ANCHOR_MS = null;
  assert.match(arrivalHTML(new Date(Date.now()+86400000).toISOString()), /<span title=/);
  assert.equal(arrivalHTML('2000-01-01T00:00:00Z'), 'arrived');
});
