import test from 'node:test';
import assert from 'node:assert/strict';

// Böner steg 4: the owner's force card shows the ship's own hull ceiling and what was laid on it for ever.
const { renderUnitCard } = await import('./force_card.js');

const ship = { id: 's1', type: 'galley', category: 'naval', status: 'garrison', size: 1, crew: 20, hull: 6, hull_max: 6, blessings: ['Stout Hulls for ever', 'followed by the Sea Daimons, for ever'] };

test('a ship with the permanent plank reads hull 6/6 and names its blessings, also when full', () => {
  const html = renderUnitCard(ship, {});
  assert.match(html, /hull 6\/6/);
  assert.match(html, /Stout Hulls for ever/);
  assert.match(html, /followed by the Sea Daimons, for ever/);
});

test('repair is offered below the ship\'s OWN ceiling, not below 5', () => {
  assert.doesNotMatch(renderUnitCard(ship, {}), /unitRepair/);
  assert.match(renderUnitCard({ ...ship, hull: 5 }, {}), /unitRepair/);
});

test('an ordinary ship is unchanged: full 5 shows no badge, a damaged one reads x/5', () => {
  const plain = { id: 's2', type: 'galley', category: 'naval', status: 'garrison', size: 1, crew: 20, hull: 5 };
  assert.doesNotMatch(renderUnitCard(plain, {}), /hull 5\/5/);
  assert.match(renderUnitCard({ ...plain, hull: 3 }, {}), /hull 3\/5/);
  assert.doesNotMatch(renderUnitCard(plain, {}), /for ever/);
});

test('a blessed troop shows its line', () => {
  const troop = { id: 't1', type: 'spearman', category: 'land', status: 'garrison', size: 50, blessings: ['blessed by Velchanos, for ever'] };
  assert.match(renderUnitCard(troop, {}), /blessed by Velchanos, for ever/);
});

test('yard sharing shows on building and repair lines, only when more than one ship shares', () => {
  const b = { id: 'b1', type: 'galley', category: 'naval', status: 'forming', size: 1, crew: 20, hull: 5, build_complete_at: new Date(Date.now() + 3600e3).toISOString(), shipyard_workers: 3, shipyard_sharing: 2 };
  assert.match(renderUnitCard(b, {}), /building — ready .* · 3 workers shared by 2 ships/);
  assert.doesNotMatch(renderUnitCard({ ...b, shipyard_sharing: 1 }, {}), /shared by/);
  const r = { ...b, status: 'repairing' };
  assert.match(renderUnitCard(r, {}), /being repaired — ready .* · 3 workers shared by 2 ships/);
  assert.doesNotMatch(renderUnitCard({ ...r, shipyard_sharing: 1 }, {}), /shared by/);
});

test('SMAFEL3: a marching unit past its arrival time says it has arrived, never "arrives arrived"', () => {
  const m = { id: 'm1', type: 'spearman', category: 'land', status: 'marching', size: 50, target_q: 10, target_r: 59, arrives_at: new Date(Date.now() - 60e3).toISOString() };
  const html = renderUnitCard(m, {});
  assert.match(html, /→ \(10,59\) · has arrived/);
  assert.doesNotMatch(html, /arrives arrived/);
  const later = renderUnitCard({ ...m, arrives_at: new Date(Date.now() + 3600e3).toISOString() }, {});
  assert.match(later, /→ \(10,59\) arrives /);
});
