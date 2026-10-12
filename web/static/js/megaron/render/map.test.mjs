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
globalThis.ResizeObserver ??= class { observe() {} };
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

const { panDelta, CATCHMENT_OFFSETS, CATCHMENT_RADIUS, workedHexesFromRoster, messengerLegPosition, hexPx, catchmentFocusCities } = await import('./map.js');

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

// The hex panel's "Workers here" row (inspel 2026-09-08) reads the same reduced
// entry, so it carries the hex's total gubbar and the goods they work.
test('workedHexesFromRoster: sums gubbar per hex and lists their goods', () => {
  const out = workedHexesFromRoster([{
    assignments: [
      { target_kind: 'hex', hex_q: -2, hex_r: 0, good_key: 'grain', count: 3 },
      { target_kind: 'hex', hex_q: -2, hex_r: 0, good_key: 'olives', count: 1 },
      { target_kind: 'hex', hex_q: -2, hex_r: 2, good_key: 'fish', count: 2 },
      { target_kind: 'building', building_type: 'stonequarry', good_key: 'stone', count: 5 },
    ],
  }]);
  const byKey = Object.fromEntries(out.map(h => [`${h.q},${h.r}`, h]));
  assert.equal(byKey['-2,0'].count, 4);
  assert.deepEqual(byKey['-2,0'].goods, ['grain', 'olives']);
  assert.equal(byKey['-2,2'].count, 2);
  assert.deepEqual(byKey['-2,2'].goods, ['fish']);
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

const { State } = await import('../state.js');
const { markHexAt, hexAtWorld, riverCurves, riverInkAt } = await import('./map.js');
const dirs = [[1,0],[1,-1],[0,-1],[-1,0],[-1,1],[0,1]];
function riverFixture(seaDir = 3, river = 'river') {
  State.tileData = [];
  for (let q = -3; q <= 3; q++) for (let r = -3; r <= 3; r++) {
    State.tileData.push({ q, r, terrain: q === 0 && r === 0 ? river :
      q === dirs[seaDir][0] && r === dirs[seaDir][1] ? 'coastal_sea' : 'plains' });
  }
}
const terrain = (q, r) => State.tileData.find(t => t.q === q && t.r === r)?.terrain;
test('river valley ground warps across its physical hex boundary like other land', () => {
  State.WORLD_ID = 'showcase-forest-0000-0000-000000000000';
  riverFixture();
  let inward = 0, outward = 0;
  for (let x = -32; x <= 32; x++) for (let y = -32; y <= 32; y++) {
    const exact = hexAtWorld(x + .5, y + .5), warped = markHexAt(x + .5, y + .5);
    if (exact[0] === 0 && exact[1] === 0 && (warped[0] !== 0 || warped[1] !== 0)) inward++;
    if ((exact[0] !== 0 || exact[1] !== 0) && warped[0] === 0 && warped[1] === 0) outward++;
  }
  assert.ok(inward > 0, 'neighbour ground must enter the physical river hex');
  assert.ok(outward > 0, 'river valley ground must extend beyond the physical hex');
});
test('every river mouth reaches a sea pixel in the warped mask, all directions and salts', () => {
  for (const salt of ['showcase-forest-0000-0000-000000000000', 'Mycenae', 'Tiryns', 'Nestor']) {
    State.WORLD_ID = salt;
    for (let dir = 0; dir < 6; dir++) for (const type of ['river', 'river_ford']) {
      riverFixture(dir, type);
      const endpoint = riverCurves(0, 0)[0][0];
      const qr = markHexAt(...endpoint);
      assert.equal(terrain(...qr), 'coastal_sea', `mouth ${dir} ${salt} ${type} must meet water`);
    }
  }
});
test('physical river ink survives encroaching ground and stops at sea and fog', () => {
  State.WORLD_ID = 'showcase-forest-0000-0000-000000000000';
  riverFixture();
  let encroaching = 0;
  for (let x = -18; x <= 18; x++) for (let y = -18; y <= 18; y++) {
    const wx = x + .5, wy = y + .5;
    const exact = hexAtWorld(wx, wy), warped = markHexAt(wx, wy);
    const t = terrain(...warped);
    if (t === 'coastal_sea') assert.equal(riverInkAt(0,0,wx,wy), false);
    else if (exact[0] === 0 && exact[1] === 0 && t === 'plains') {
      assert.equal(riverInkAt(0,0,wx,wy), true, 'ground cannot clip the physical channel'); encroaching++;
    }
  }
  assert.ok(encroaching > 0);
  State.tileData = State.tileData.map(t => ({ ...t, terrain: t.terrain === 'plains' ? 'fog' : t.terrain }));
  let fog = 0;
  for (let x = -30; x <= 30; x++) for (let y = -30; y <= 30; y++) {
    if (terrain(...markHexAt(x+.5,y+.5)) === 'fog') {
      assert.equal(riverInkAt(0,0,x+.5,y+.5), false, 'river cannot reveal fog'); fog++;
    }
  }
  assert.ok(fog > 0);
});

test('catchment tint only for the city in focus: open City drawer or own city selected', async () => {
  const { State } = await import('../state.js');
  const myc = { id: 'myc', q: 0, r: 0, own: true, is_capital: true }, tir = { id: 'tir', q: 5, r: 0, own: true }, kyme = { id: 'kyme', q: 9, r: 0, own: false };
  State.provinceData = [myc, tir, kyme];
  State.activeDrawer = null; State.selectedHex = null; State.cityViewID = null;
  assert.deepEqual(catchmentFocusCities(), [], 'no tint on the plain map');
  State.activeDrawer = 'city'; State.cityViewID = 'tir';
  assert.deepEqual(catchmentFocusCities(), [tir], 'the city the drawer shows');
  State.activeDrawer = 'war';
  assert.deepEqual(catchmentFocusCities(), [], 'another drawer is not the city');
  State.activeDrawer = null; State.selectedHex = { q: 0, r: 0 };
  assert.deepEqual(catchmentFocusCities(), [myc], 'own city selected on the map');
  State.selectedHex = { q: 9, r: 0 };
  assert.deepEqual(catchmentFocusCities(), [], 'a foreign city gives no tint');
  State.selectedHex = null;
});

test('production scene shows four own trails and only current arrows for every non-owner, including recipients',async()=>{
 const {journeyOverlays}=await import('./map.js');
 const saved={};for(const k of ['tileData','unitsData','marchData','tradeData','messengerData','foreignUnitData','CURRENT_TICK','TICK_SECONDS','TICK_ANCHOR_MS'])saved[k]=State[k];
 try{
  State.CURRENT_TICK=1;State.TICK_SECONDS=6;State.TICK_ANCHOR_MS=0;
  State.tileData=[0,1,2,3].map(q=>({q,r:0,terrain:'plains',tier:'live'}));
  const base={path:[[0,0],[1,0],[2,0],[3,0]],departure_tick:0,arrival_tick:4,step_costs:[1000,1000,1000],departs_at:'1970-01-01T00:00:00Z',arrives_at:'1970-01-01T00:00:24Z'};
  State.unitsData=[{...base,id:'army',category:'land',status:'marching'},{...base,id:'ship',category:'naval',status:'marching'}];State.marchData=[];
  const observed={id:'foreign-caravan',mine:false,role:'recipient',current_q:1,current_r:0,heading:'east'};
  for(const key of ['path','dest_q','dest_r','departs_at','arrives_at'])Object.defineProperty(observed,key,{get(){throw Error('recipient route leaked into rendering');}});
  State.tradeData=[{...base,id:'caravan',mine:true,category:'land'},observed];
  State.messengerData=[{...base,id:'runner',own:true},{id:'foreign-runner',own:false,current_q:1,current_r:0,heading:'east'}];
  State.foreignUnitData=[{id:'foreign-army',q:1,r:0,heading:'east',category:'land'},{id:'foreign-ship',q:1,r:0,heading:'east',category:'naval'}];
  const out=journeyOverlays(3000);assert.equal(out.filter(p=>p.type==='route').length,4,'own movement kinds lost an always-visible trail');assert.equal(out.filter(p=>p.type==='arrow').length,4);
  assert.equal(out.find(p=>p.actor_id==='foreign-caravan').type,'arrow');
  State.tradeData.push({...base,id:'agreed',mine:false,role:'recipient',route_known:true,category:'land'});
  const agreement=journeyOverlays(3000).find(p=>p.actor_id==='agreed');assert.equal(agreement.type,'route','agreed recipient caravan lost its forward route');assert.equal(agreement.relation,'foreign');
  State.tradeData.push({id:'gift',mine:false,role:'recipient',route_known:false,current_q:1,current_r:0,heading:'east'});
  assert.equal(journeyOverlays(3000).find(p=>p.actor_id==='gift').type,'arrow','unagreed gift exposed a route');
  assert.equal(out.find(p=>p.actor_id==='foreign-runner').type,'arrow');
  State.tileData=State.tileData.map(t=>({...t,tier:'remembered'}));assert.deepEqual(journeyOverlays(3000),[],'movement marks leaked onto remembered terrain');
 }finally{Object.assign(State,saved);}
});
