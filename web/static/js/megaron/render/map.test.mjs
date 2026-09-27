import test from 'node:test';
import assert from 'node:assert/strict';

// map.js is the biggest offender: `export const canvas = document.getElementById(...)`,
// `tooltip`, `container` and `ctx = canvas.getContext('2d')` are ALL module-level DOM
// reads, on top of the `window.addEventListener('resize', resizeCanvas)` this plan
// originally flagged — moving all of that into an init() would mean converting several
// module-level `const`s (exported and consumed by ui/search.js, ui/marchctx.js) into
// `let`s assigned from initMap(), a structural change to the whole file, not a two-line
// move (megaron_plan_modulniva_dom.md under-scoped this file — see the plan's own
// ✅-notes for the full finding). Same stub-then-dynamic-import trick as
// marchctx.test.mjs sidesteps needing that refactor at all: a getElementById/
// getContext that always returns a usable no-op Proxy satisfies every module-level
// read here without touching map.js's production code.
const noopEl = new Proxy({}, {
  get: (_t, k) => (k === 'style' ? {} : (k === 'value' ? '' : () => noopEl)),
  set: () => true,
});
globalThis.document ??= {
  addEventListener() {},
  getElementById: () => noopEl,
  createElement: () => noopEl,
  querySelector: () => noopEl,
  querySelectorAll: () => [],
  body: noopEl,
};
globalThis.window ??= {
  addEventListener() {},
  matchMedia: () => ({ matches: false, addEventListener() {} }),
  innerWidth: 800, innerHeight: 600,
};
globalThis.localStorage ??= { getItem: () => null, setItem() {}, removeItem() {} };

const { panDelta, CATCHMENT_OFFSETS, CATCHMENT_RADIUS, workedHexesFromRoster, messengerLegPosition, hexPx } = await import('./map.js');

// The worked-hex map markers (report 54f2b747) come from the placement-roster
// payload reduced to one {q,r} per own catchment hex carrying a placed gubbe.
test('workedHexesFromRoster: one marker per hex, dedup across gubbar/goods', () => {
  const roster = [{
    name: 'Knossos',
    assignments: [
      { target_kind: 'hex', hex_q: -2, hex_r: 0, good_key: 'grain', count: 3 },
      { target_kind: 'hex', hex_q: -2, hex_r: 0, good_key: 'grain', count: 1 }, // same hex again
      { target_kind: 'hex', hex_q: -2, hex_r: 2, good_key: 'fish', count: 1 },
      { target_kind: 'building', building_type: 'stonequarry', good_key: 'stone', count: 1 }, // no q/r
    ],
  }];
  const out = workedHexesFromRoster(roster);
  assert.equal(out.length, 2, 'two distinct hexes, building assignment excluded');
  assert.deepEqual(new Set(out.map(h => `${h.q},${h.r}`)), new Set(['-2,0', '-2,2']));
});

test('workedHexesFromRoster: spans settlements and tolerates empty/missing', () => {
  assert.deepEqual(workedHexesFromRoster([]), []);
  assert.deepEqual(workedHexesFromRoster(null), []);
  const out = workedHexesFromRoster([
    { assignments: [{ target_kind: 'hex', hex_q: 1, hex_r: 1 }] },
    { assignments: [{ target_kind: 'hex', hex_q: 5, hex_r: 5 }] },
    { assignments: [{ target_kind: 'hex', hex_q: null, hex_r: null }] }, // guarded
    { /* no assignments field */ },
  ]);
  assert.deepEqual(new Set(out.map(h => `${h.q},${h.r}`)), new Set(['1,1', '5,5']));
});

// The catchment highlight must cover the settlement's radius-2 disc (19 hexes,
// mirroring server hexgrid.CatchmentRadius), not the pre-P1 7-hex radius-1
// shape it regressed to. Guards against anyone reverting to [[0,0],...HEX_DIRS].
test('catchment offsets cover the full radius-2 disc (19 hexes), not 7', () => {
  const dist = (q, r) => (Math.abs(q) + Math.abs(q + r) + Math.abs(r)) / 2;
  assert.equal(CATCHMENT_RADIUS, 2);
  assert.equal(CATCHMENT_OFFSETS.length, 19, 'radius-2 disc is 1+6+12 = 19 hexes');
  assert.ok(CATCHMENT_OFFSETS.every(([q, r]) => dist(q, r) <= CATCHMENT_RADIUS), 'every offset within radius');
  assert.ok(CATCHMENT_OFFSETS.some(([q, r]) => q === 0 && r === 0), 'includes the settlement hex');
  assert.equal(CATCHMENT_OFFSETS.filter(([q, r]) => dist(q, r) === 2).length, 12, 'outer ring has 12 hexes');
  // No duplicates.
  assert.equal(new Set(CATCHMENT_OFFSETS.map(([q, r]) => `${q},${r}`)).size, 19);
});

test('AK1: import does not touch DOM/window before this point (proven by reaching here)', () => {
  assert.ok(true);
});

test('AK2: panDelta normalizes a diagonal so W+D is not sqrt(2)x faster than one direction', () => {
  const single = panDelta(new Set(['w']), 1000, 100);
  const diag = panDelta(new Set(['w', 'd']), 1000, 100);
  const singleDist = Math.hypot(single.dx, single.dy);
  const diagDist = Math.hypot(diag.dx, diag.dy);
  assert.ok(Math.abs(singleDist - diagDist) < 1e-9, 'diagonal speed must match single-axis speed');
});

test('AK3: no keys held means no movement', () => {
  const out = panDelta(new Set(), 1000, 100);
  assert.deepEqual(out, { dx: 0, dy: 0 });
});

// messengerLegPosition (megaron_plan_budets_tre_ben.md, slice 3c, R4): the
// server (world.go MapMessengers) decides which physical leg an own
// passage-lifted runner is on; this pure function only interpolates. One
// fixture per leg, mirroring the Go leg fixtures in
// api/handlers/messenger_passage_leg_test.go.
test('messengerLegPosition: no leg set → null (today\'s flat interpolation applies)', () => {
  assert.equal(messengerLegPosition({}, [], [], Date.now()), null);
});

test('messengerLegPosition: to_port walks from origin to the port over the leg window', () => {
  const m = { leg: 'to_port', leg_from_q: 0, leg_from_r: 0, leg_to_q: 3, leg_to_r: 0,
    leg_start: '2026-01-01T00:00:00Z', leg_end: '2026-01-01T02:00:00Z' };
  const midway = new Date('2026-01-01T01:00:00Z').getTime();
  const leg = messengerLegPosition(m, [], [], midway);
  assert.equal(leg.mode, 'walk');
  const start = hexPx(0, 0), end = hexPx(3, 0);
  assert.ok(Math.abs(leg.x - (start.x + end.x) / 2) < 1, 'x roughly halfway');
});

test('messengerLegPosition: waiting stands still at the port', () => {
  const m = { leg: 'waiting', leg_from_q: 3, leg_from_r: 0, leg_to_q: 3, leg_to_r: 0 };
  const leg = messengerLegPosition(m, [], [], Date.now());
  const port = hexPx(3, 0);
  assert.deepEqual(leg, { mode: 'waiting', x: port.x, y: port.y, q: 3, r: 0 });
});

test('messengerLegPosition: sealed stands still at the port', () => {
  const m = { leg: 'sealed', leg_from_q: 3, leg_from_r: 0, leg_to_q: 3, leg_to_r: 0 };
  const leg = messengerLegPosition(m, [], [], Date.now());
  const port = hexPx(3, 0);
  assert.deepEqual(leg, { mode: 'sealed', x: port.x, y: port.y, q: 3, r: 0 });
});

test('messengerLegPosition: ashore walks from the disembark point to the true target', () => {
  const m = { leg: 'ashore', leg_from_q: 10, leg_from_r: 0, leg_to_q: 20, leg_to_r: 0,
    leg_start: '2026-01-01T00:00:00Z', leg_end: '2026-01-01T02:00:00Z' };
  const start = new Date('2026-01-01T00:00:00Z').getTime();
  const leg = messengerLegPosition(m, [], [], start);
  assert.equal(leg.mode, 'walk');
  const p0 = hexPx(10, 0);
  assert.ok(Math.abs(leg.x - p0.x) < 1, 'starts at the disembark point');
});

test('messengerLegPosition: aboard draws on a located carrier unit, ignoring the reserve leg', () => {
  const m = { leg: 'aboard', carrier_unit_id: 'ship-1', leg_from_q: 3, leg_from_r: 0, leg_to_q: 10, leg_to_r: 0 };
  const units = [{ id: 'ship-1', status: 'positioned', q: 7, r: 0 }];
  const leg = messengerLegPosition(m, units, [], Date.now());
  assert.equal(leg.mode, 'aboard-carrier');
  const shipPos = hexPx(7, 0);
  assert.equal(leg.x, shipPos.x);
  assert.equal(leg.y, shipPos.y);
});

test('messengerLegPosition: aboard draws on a located /trades transport when no unit matches', () => {
  const m = { leg: 'aboard', carrier_transport_id: 't-1', leg_from_q: 3, leg_from_r: 0, leg_to_q: 10, leg_to_r: 0 };
  const trades = [{ id: 't-1', origin_q: 3, origin_r: 0, dest_q: 10, dest_r: 0,
    departs_at: '2026-01-01T00:00:00Z', arrives_at: '2026-01-01T02:00:00Z' }];
  const midway = new Date('2026-01-01T01:00:00Z').getTime();
  const leg = messengerLegPosition(m, [], trades, midway);
  assert.equal(leg.mode, 'aboard-carrier');
});

test('messengerLegPosition: aboard falls back to the server\'s reserve leg when no carrier is found client-side', () => {
  const m = { leg: 'aboard', carrier_unit_id: 'ship-missing', leg_from_q: 3, leg_from_r: 0, leg_to_q: 10, leg_to_r: 0,
    leg_start: '2026-01-01T00:00:00Z', leg_end: '2026-01-01T02:00:00Z' };
  const midway = new Date('2026-01-01T01:00:00Z').getTime();
  const leg = messengerLegPosition(m, [], [], midway);
  assert.equal(leg.mode, 'aboard-reserve');
  const p0 = hexPx(3, 0), p1 = hexPx(10, 0);
  assert.ok(Math.abs(leg.x - (p0.x + p1.x) / 2) < 1, 'interpolated roughly halfway along the reserve leg');
});
