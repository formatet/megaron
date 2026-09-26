import test from 'node:test';
import assert from 'node:assert/strict';
import { unitHoverLines } from './hover.js';

const names = { '0,0': 'Knossos', '5,0': 'Phaistos' };
const placeName = (q, r) => names[q + ',' + r] || `(${q},${r})`;

test('an own unit on the march says where it left from and where it is going', () => {
  const lines = unitHoverLines({
    placeName,
    own: [{ display_name: '1st Spearmen of Knossos', status: 'marching', q: 0, r: 0, target_q: 5, target_r: 0 }],
  });
  assert.deepEqual(lines, ['1st Spearmen of Knossos — from Knossos to Phaistos']);
});

test('an own unit standing still is just its name', () => {
  const lines = unitHoverLines({ placeName, own: [{ display_name: '1st Spearmen of Knossos', status: 'positioned', q: 2, r: 0 }] });
  assert.deepEqual(lines, ['1st Spearmen of Knossos']);
});

test('a foreign march shows its heading, never its destination', () => {
  const lines = unitHoverLines({
    placeName,
    foreign: [{ name: '2nd Spearmen of Mallia', owner: 'Minos', status: 'marching', heading: 'north-west', target_q: 0, target_r: 0 }],
  });
  assert.deepEqual(lines, ['2nd Spearmen of Mallia (Minos) — heading north-west']);
});

test('a foreign unit says what it is called and whose it is', () => {
  const lines = unitHoverLines({
    placeName,
    foreign: [
      { name: '2nd Spearmen of Mallia', owner: 'Minos', type: 'spearman', status: 'marching', target_q: 0, target_r: 0 },
      { owner: 'Minos', type: 'war_chariot', status: 'positioned' },
    ],
  });
  assert.equal(lines[0], '2nd Spearmen of Mallia (Minos)');
  assert.match(lines[1], /\(Minos\)$/);
});

test("a caravan's cargo is shown only to its sender and recipient", () => {
  const route = { origin_q: 0, origin_r: 0, dest_q: 5, dest_r: 0 };
  const lines = unitHoverLines({
    placeName,
    caravans: [
      { ...route, role: 'sender', owner: 'Ariadne', good_key: 'grain', quantity: 250 },
      { ...route, role: 'recipient', owner: 'Minos', good_key: 'copper', quantity: 7 },
      { ...route, role: '', owner: 'Minos', good_key: '', quantity: 0 },
    ],
  });
  assert.deepEqual(lines, [
    'Your caravan: 250 grain — from Knossos to Phaistos',
    'Caravan to you from Minos: 7 copper',
    'Caravan of Minos',
  ]);
});

test('runners: your own shows its route, a stranger\'s only whose it is', () => {
  const lines = unitHoverLines({
    placeName,
    runners: [
      { own: true, origin_q: 0, origin_r: 0, dest_q: 5, dest_r: 0 },
      { own: false, sender: 'Minos', origin_q: 5, origin_r: 0, dest_q: 0, dest_r: 0 },
    ],
  });
  assert.deepEqual(lines, ['Your Runner — from Knossos to Phaistos', 'Runner of Minos']);
});
