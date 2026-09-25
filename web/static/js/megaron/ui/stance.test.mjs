import test from 'node:test';
import assert from 'node:assert/strict';
import { canTakeStance, stanceSentLine } from './stance.js';

test('canTakeStance: a marching land unit can take a stance, a ship never', () => {
  assert.equal(canTakeStance({ category: 'land', status: 'marching' }), true);
  assert.equal(canTakeStance({ category: 'land', status: 'garrison' }), true);
  assert.equal(canTakeStance({ category: 'land', status: 'positioned' }), true);
  assert.equal(canTakeStance({ category: 'land', status: 'forming' }), false);
  assert.equal(canTakeStance({ category: 'naval', status: 'marching' }), false);
});

test('stanceSentLine: a marching unit — the Runner must catch up, where and when', () => {
  const l = stanceSentLine({ stance: 'sentry', catch_up: 'on_the_march', intercept_q: 2, intercept_r: 0 }, '14:00 · in 3h');
  assert.match(l, /must catch up/);
  assert.match(l, /at \(2,0\) ~14:00 · in 3h/);
  assert.match(l, /bites where the unit stops/);
});

test('stanceSentLine: no Runner can overtake — it follows the unit to its destination', () => {
  const l = stanceSentLine({ stance: 'fortify', catch_up: 'at_destination', intercept_q: 8, intercept_r: 0 }, 'x');
  assert.match(l, /no Runner can overtake/);
  assert.match(l, /destination at \(8,0\)/);
  assert.match(l, /applies where it stopped/);
});

test('stanceSentLine: a standing field unit keeps the plain receipt', () => {
  assert.match(stanceSentLine({ stance: 'storm' }, 'y'), /reaches the unit ~y and applies on delivery/);
});
