import test from 'node:test';
import assert from 'node:assert/strict';
import { State } from '../state.js';
import { arrivalHTML, fmtArrival, fmtEta, fmtClock } from './time.js';

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
test('wall-clock estimate after several real dates stays clock/date, not game days or d',()=>{
 State.TICK_ANCHOR_MS=null;
 const at=new Date(Date.now()+3*86400000).toISOString();
 for(const text of [fmtEta(at),fmtArrival(at)]) {
  assert.match(text,/≈.*\d[:.]\d/);
  assert.doesNotMatch(text,/\bdays?\b|\d+d\b|\d+h\b/);
 }
});

test('clock is always 24 h, whatever the browser locale (Timothy 2026-10-11)', () => {
  const evening = new Date();
  evening.setHours(19, 4, 0, 0);
  assert.match(fmtClock(evening.getTime()), /^today 19:04$/);
  State.TICK_SECONDS = 0;
  const html = arrivalHTML(new Date(Date.now() + 3600e3).toISOString());
  assert.doesNotMatch(html, /\b(AM|PM)\b/i, 'hover title must be 24 h too');
});
