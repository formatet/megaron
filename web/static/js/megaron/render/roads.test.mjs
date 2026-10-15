import test from 'node:test';
import assert from 'node:assert/strict';
import { linePixels, roadSegments, plannedSegments, drawRoads, ROAD_DUST, ROAD_SHADOW } from './roads.js';

const hexPx = (q, r) => ({ x: q * 10, y: r * 8 });

test('roads: a pixel line keeps both endpoints and only whole pixels', () => {
  const px = linePixels(0.2, 0.2, 10.4, 7.6);
  assert.deepEqual(px[0], [0, 0]);
  assert.deepEqual(px.at(-1), [10, 8]);
  for (const [x, y] of px) { assert.ok(Number.isInteger(x) && Number.isInteger(y)); }
  for (let i = 1; i < px.length; i++) {
    assert.ok(Math.abs(px[i][0] - px[i - 1][0]) <= 1 && Math.abs(px[i][1] - px[i - 1][1]) <= 1, 'no gaps');
  }
});

test('roads: segments come from edges, malformed edges are dropped', () => {
  const segs = roadSegments([{ q1: 0, r1: 0, q2: 1, r2: 0 }, { q1: 0, r1: 0 }], hexPx);
  assert.equal(segs.length, 1);
  assert.deepEqual(segs[0], [{ x: 0, y: 0 }, { x: 10, y: 0 }]);
  assert.deepEqual(roadSegments(undefined, hexPx), []);
  assert.equal(plannedSegments([{ from_q: 1, from_r: 1, to_q: 2, to_r: 1 }, { from_q: 1 }], hexPx).length, 1);
});

test('roads: a finished edge draws a dark shadow row under a dust row', () => {
  const calls = [];
  const ctx = { save() {}, restore() {}, set fillStyle(v) { this._f = v; }, fillRect(x, y) { calls.push([this._f, x, y]); } };
  drawRoads(ctx, [[{ x: 0, y: 0 }, { x: 3, y: 0 }]]);
  const dust = calls.filter(c => c[0] === ROAD_DUST).map(c => [c[1], c[2]]);
  const shadow = calls.filter(c => c[0] === ROAD_SHADOW).map(c => [c[1], c[2]]);
  assert.equal(dust.length, 4);
  assert.deepEqual(shadow, dust.map(([x, y]) => [x, y + 1]));
});
