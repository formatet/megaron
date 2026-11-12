import test from 'node:test';
import assert from 'node:assert/strict';
import { foodAnswer, foodAnswerValue, foodAnswerText } from './food_line.js';
const pd = (net, days = null) => ({ forecast: { food: { net, days } } });

test('food answer: the net after everything, from the forecast owner only', () => {
  assert.deepEqual(foodAnswer(pd(0.4)), { net: 0.4, days: null });
  assert.deepEqual(foodAnswer(pd(-2, 9.5)), { net: -2, days: 9.5 });
  // the other server fields are different quantities and are never the answer
  assert.equal(foodAnswer({ food_net_per_tick_after_upkeep: 1, food: { food_net_per_tick: 2, coverage_ticks: 240 } }), null);
  for (const x of [null, {}, { forecast: {} }, { forecast: { food: {} } }, { forecast: { food: { net: 'x' } } }]) assert.equal(foodAnswer(x), null);
});
test('food answer: one wording, a day count only when running down', () => {
  assert.equal(foodAnswerText(foodAnswer(pd(0.42))), 'Food +0.4 per game day');
  assert.equal(foodAnswerText(foodAnswer(pd(-2.04, 5.9))), 'Food −2 per game day · lasts 5.9 game days');
  assert.equal(foodAnswerText(foodAnswer(pd(-1, 1))), 'Food −1 per game day · lasts 1 game day');
  assert.equal(foodAnswerText(foodAnswer(pd(0))), 'Food about even');
  assert.equal(foodAnswerText(foodAnswer(pd(-0.01, 120000))), 'Food about even');
  assert.equal(foodAnswerText(null), '');
});
test('food answer: never a silly day count', () => {
  assert.equal(foodAnswerValue(foodAnswer(pd(-0.15, 8000))), '−0.2 per game day · lasts over 1,000 game days');
  assert.doesNotMatch(foodAnswerText(foodAnswer(pd(3, 8000))), /game days/, 'positive net has no day count even if a stale days slipped in');
});
