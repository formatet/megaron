import test from 'node:test';
import assert from 'node:assert/strict';
import { FIRE_W, FIRE_H, FIRE_PALETTE, newFire, stepFire, fireAt } from './doomfire.js';

const HOT = FIRE_PALETTE.length - 1;

test('fire rises from the hot source row and stays inside the palette', () => {
  const f = newFire('city-a');
  for (let i = 0; i < 60; i++) stepFire(f);
  const bottom = f.buf.subarray((FIRE_H - 1) * FIRE_W);
  assert.equal(bottom[FIRE_W >> 1], HOT, 'source row is held hot');
  assert.equal(bottom[0], 0, 'source is cold at the edge (a pyre, not a wall)');
  const mid = f.buf.subarray((FIRE_H >> 1) * FIRE_W, ((FIRE_H >> 1) + 1) * FIRE_W);
  assert.ok(mid.some(v => v > 0), 'flames reach the middle of the field');
  assert.ok(f.buf.every(v => v >= 0 && v <= HOT));
});

test('same city, same frame → same picture (frozen-frame rigs stay deterministic)', () => {
  const a = Array.from(fireAt('city-b', 25).buf);
  fireAt('city-b', 40);
  const again = Array.from(fireAt('city-b', 25).buf); // backwards → restart from seed
  assert.deepEqual(again, a);
  const other = Array.from(fireAt('city-c', 25).buf);
  assert.notDeepEqual(other, a, 'each city burns with its own flames');
});
