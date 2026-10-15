import test from 'node:test';
import assert from 'node:assert/strict';
import { roadNeighbours, roadPanelHTML, roadOrderBody, parseHex } from './roadorder.js';

const tiles = [
  { q: 0, r: 0, terrain: 'plains' },
  { q: 1, r: 0, terrain: 'hills' },
  { q: -1, r: 0, terrain: 'river_ford' },
  { q: 0, r: 1, terrain: 'mountain_red' },
  { q: 0, r: -1, terrain: 'coastal_sea' },
  { q: 1, r: -1, terrain: 'forest_olive_grove' },
  { q: -1, r: 1, terrain: 'plains' },
];

test('roadorder: only roadable neighbours are offered (no ford, mountain or sea)', () => {
  const got = roadNeighbours({ q: 0, r: 0 }, tiles, []).map(n => `${n.q},${n.r}`).sort();
  assert.deepEqual(got, ['-1,1', '1,-1', '1,0']);
});

test('roadorder: an existing edge is not offered again, in either direction', () => {
  const edges = [{ q1: 0, r1: 0, q2: 1, r2: 0 }, { q1: -1, r1: 1, q2: 0, r2: 0 }];
  const got = roadNeighbours({ q: 0, r: 0 }, tiles, edges).map(n => `${n.q},${n.r}`);
  assert.deepEqual(got, ['1,-1']);
});

test('roadorder: an army standing where no road can start gets no choices', () => {
  assert.deepEqual(roadNeighbours({ q: -1, r: 0 }, tiles, []), []);
  assert.deepEqual(roadNeighbours({ q: 9, r: 9 }, tiles, []), []);
});

test('roadorder: panel wires the order button and names the from hex', () => {
  const html = roadPanelHTML('abc', { q: 0, r: 0 }, tiles, []);
  assert.match(html, /unitBuildRoad\('abc'\)/);
  assert.match(html, /data-from="0,0"/);
  assert.match(html, /value="1,0">\(1,0\) hills/);
  assert.match(roadPanelHTML('abc', { q: -1, r: 0 }, tiles, []), /No neighbouring hex/);
});

test('roadorder: the body carries from and to; a bad choice yields none', () => {
  assert.deepEqual(roadOrderBody({ q: 5, r: -2 }, '6,-2'), { from_q: 5, from_r: -2, to_q: 6, to_r: -2 });
  assert.equal(roadOrderBody({ q: 5, r: -2 }, 'nope'), null);
  assert.equal(roadOrderBody(null, '1,1'), null);
  assert.deepEqual(parseHex('-3,4'), { q: -3, r: 4 });
});
