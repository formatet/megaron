import test from 'node:test';
import assert from 'node:assert/strict';
import { isSea, isNavigable, isWaterBarrier } from './terrain.js';

// Mirrors server/internal/province/water_test.go waterClasses.
const classes = [
  ['coastal_sea', true, true, true],
  ['deep_sea', true, true, true],
  ['river', false, true, true],
  ['river_ford', false, true, false],
  ['plains', false, false, false],
  ['mountain_red', false, false, false],
  ['river_valley', false, false, false],
  [undefined, false, false, false],
];

test('water classes match the server owner', () => {
  for (const [t, sea, nav, wall] of classes) {
    assert.equal(isSea(t), sea, `isSea(${t})`);
    assert.equal(isNavigable(t), nav, `isNavigable(${t})`);
    assert.equal(isWaterBarrier(t), wall, `isWaterBarrier(${t})`);
  }
});
