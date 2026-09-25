import test from 'node:test';
import assert from 'node:assert/strict';
import { eyeSees, hexLine, seaSightline } from './sight.js';

const lookup = (fn) => (q, r) => fn(q, r);

test('enclosed lake behind the shore is not previewed (acceptance world (28,18))', () => {
  const over = new Map([['24,18', 'coastal_sea'], ['24,19', 'coastal_sea'],
    ['25,18', 'mountain_limestone'], ['26,18', 'hills'], ['27,18', 'forest_olive_grove']]);
  const t = lookup((q, r) => over.get(`${q},${r}`) ?? (q >= 29 ? 'deep_sea' : 'plains'));
  assert.equal(eyeSees('land', 28, 18, 24, 18, t), false);
  assert.equal(eyeSees('land', 28, 18, 24, 19, t), false);
  assert.equal(eyeSees('land', 28, 18, 32, 18, t), true, 'open sea 4 east is seen');
});

test('one land hex on the line blocks; open water reaches 4', () => {
  const t = lookup((q, r) => (q === 2 && r === 0 ? 'plains' : 'deep_sea'));
  assert.equal(eyeSees('ship', 0, 0, 4, 0, t), false);
  assert.equal(eyeSees('ship', 0, 0, 0, 4, t), true);
});

test('fog between eye and sea blocks (never over-draw)', () => {
  const t = lookup((q) => (q === 2 ? undefined : q >= 1 ? 'coastal_sea' : 'plains'));
  assert.equal(eyeSees('land', 0, 0, 4, 0, t), false);
});

test('edge tie holds if either side is sea', () => {
  const p = hexLine(0, 0, 1, 1, 1)[1], m = hexLine(0, 0, 1, 1, -1)[1];
  assert.notDeepEqual(p, m);
  const seaAt = (...hexes) => (q, r) =>
    (q === 1 && r === 1) || hexes.some(([hq, hr]) => hq === q && hr === r) ? 'deep_sea' : 'plains';
  assert.equal(seaSightline(0, 0, 1, 1, seaAt([1, 0])), true);
  assert.equal(seaSightline(0, 0, 1, 1, seaAt([0, 1])), true);
  assert.equal(seaSightline(0, 0, 1, 1, seaAt()), false);
});
