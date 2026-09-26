import test from 'node:test';
import assert from 'node:assert/strict';
import { warMovements, incomingTargetKeys } from './movements.js';

const provinces = [
  { q: 0, r: 0, own: true, name: 'Knossos', settlement_id: 's-knossos' },
  { q: 5, r: 5, own: false, name: 'Mycenae', settlement_id: 's-mycenae' },
];

test('an ordinary own march is outgoing — read from the unit layer, not marching_armies', () => {
  const { outgoing, incoming } = warMovements({
    provinces,
    units: [
      { status: 'marching', display_name: '1st Spearmen of Knossos', q: 0, r: 0, target_q: 5, target_r: 5, arrives_at: '2026-09-27T10:00:00Z' },
      { status: 'garrison', display_name: 'Home guard', q: 0, r: 0 },
    ],
  });
  assert.equal(outgoing.length, 1);
  assert.equal(outgoing[0].title, '1st Spearmen of Knossos');
  assert.deepEqual([outgoing[0].target_q, outgoing[0].target_r], [5, 5]);
  assert.equal(incoming.length, 0);
});

// Since 2026-09-26 a foreign march discloses no target — only which of YOUR
// cities' lands it seems bound for (server `toward`), with an arrival-if.
test('a sighted foreign march that seems bound for an own city is incoming, as an estimate', () => {
  const { incoming } = warMovements({
    provinces,
    foreign: [
      { status: 'marching', type: 'spearman', size: 100, owner: 'Minos', q: 3, r: 3, heading: 'north-west',
        toward: { settlement_id: 's-knossos', name: 'Knossos', eta_at: '2026-09-27T10:00:00Z' } },
      { status: 'marching', type: 'galley', size: 1, owner: 'Minos', q: 3, r: 3, heading: 'east' },
      { status: 'positioned', type: 'spearman', size: 100, owner: 'Minos', q: 1, r: 0 },
    ],
  });
  assert.equal(incoming.length, 1);
  assert.equal(incoming[0].title, 'Spearmen (100) of Minos');
  assert.deepEqual([incoming[0].target_q, incoming[0].target_r], [0, 0]);
  assert.equal(incoming[0].arrives_at, '2026-09-27T10:00:00Z');
  assert.equal(incoming[0].ifBound, true);
});

test('legacy marching_armies rows (recall) still show, sorted by arrival with unit marches', () => {
  const { outgoing, incoming } = warMovements({
    provinces,
    units: [{ status: 'marching', display_name: 'A', q: 0, r: 0, target_q: 5, target_r: 5, arrives_at: '2026-09-28' }],
    marches: [
      { intent: 'recall', origin_q: 0, origin_r: 0, target_q: 5, target_r: 5, arrives_at: '2026-09-27' },
      { intent: 'attack', origin_q: 5, origin_r: 5, target_q: 0, target_r: 0, arrives_at: '2026-09-27' },
    ],
  });
  assert.deepEqual(outgoing.map(m => m.title), ['Recall', 'A']);
  assert.deepEqual(incoming.map(m => m.title), ['Attack']);
});

test('the map glow lights own cities a foreign march seems bound for — never a march elsewhere', () => {
  const keys = incomingTargetKeys({
    provinces,
    foreign: [
      { status: 'marching', type: 'infantry', size: 40, owner: 'Minos', q: 3, r: 3,
        toward: { settlement_id: 's-knossos', name: 'Knossos', eta_at: 'x' } },
      { status: 'marching', type: 'infantry', size: 40, owner: 'Minos', q: 3, r: 3, heading: 'south' },
      { status: 'positioned', type: 'infantry', size: 40, owner: 'Minos', q: 1, r: 0 },
    ],
    marches: [
      // own legacy attack on someone else — not an incoming threat
      { intent: 'attack', origin_q: 0, origin_r: 0, target_q: 5, target_r: 5 },
    ],
  });
  assert.deepEqual([...keys], ['0,0']);
});

test('a legacy recall column onto an own province is no threat', () => {
  const keys = incomingTargetKeys({
    provinces,
    marches: [{ intent: 'recall', origin_q: 5, origin_r: 5, target_q: 0, target_r: 0 }],
  });
  assert.equal(keys.size, 0);
});
