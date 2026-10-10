import test from 'node:test';
import assert from 'node:assert/strict';
import { stormOuterEdges, stormTone, EDGE_NEIGHBOUR } from './storms.js';

// Axial → pixel centre in S units, flat-top (the same as hexPx in map.js).
const centre = (q, r) => [1.5 * q, Math.sqrt(3) * (r + q / 2)];

test('edge i of a hex faces the neighbour EDGE_NEIGHBOUR[i], at angle i·60°+30°', () => {
  EDGE_NEIGHBOUR.forEach(([dq, dr], i) => {
    const [x, y] = centre(dq, dr);
    const angle = (Math.atan2(y, x) * 180 / Math.PI + 360) % 360;
    assert.ok(Math.abs(angle - ((i * 60 + 30) % 360)) < 1e-9, `edge ${i}: neighbour at ${angle}°`);
  });
});

test('a triangle of three storm hexes has 12 outer edges, shared edges are not drawn', () => {
  const tri = [{ q: 0, r: 0 }, { q: 1, r: 0 }, { q: 0, r: 1 }];
  const outer = stormOuterEdges(tri);
  assert.equal(outer.flat().filter(Boolean).length, 12); // 18 sides − 3 shared edges × 2
  // hex (0,0): its (1,0) edge is shared with hex (1,0); its (0,1) edge with hex (0,1)
  assert.equal(outer[0][0], false);
  assert.equal(outer[0][1], false);
  assert.equal(outer[0][2], true);
});

test('a straight line of three has 14 outer edges', () => {
  const line = [{ q: 0, r: 0 }, { q: 0, r: 1 }, { q: 0, r: 2 }];
  assert.equal(stormOuterEdges(line).flat().filter(Boolean).length, 14);
});

test('the cloud is deterministic per pixel and phase, always one of three tones, and it churns', () => {
  const tones = new Set();
  let differs = 0;
  for (let y = 0; y < 40; y++) for (let x = 0; x < 40; x++) {
    const t = stormTone(x, y, 1);
    assert.equal(t, stormTone(x, y, 1));
    assert.ok(t >= 0 && t <= 2);
    tones.add(t);
    if (t !== stormTone(x, y, 2)) differs++;
  }
  assert.equal(tones.size, 3);
  assert.ok(differs > 100, 'a new phase must change the picture');
});
