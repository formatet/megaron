import test from 'node:test';
import assert from 'node:assert/strict';
import { notifText } from './format.js';

// Plan B — default arm stops throwing away the payload for notification kinds
// notifText has no case for. See format.js's PAYLOAD_SUMMARY_MAX_* comment
// for why the cap exists and what it does.

test('unknown kind with payload: line contains the kind and every payload value', () => {
  const body = { foo: 'bar', n: 3 };
  const line = notifText('TotallyUnknownKind', body);
  assert.match(line, /TotallyUnknownKind/);
  assert.match(line, /bar/);
  assert.match(line, /3/);
});

test('unknown kind with empty payload: exactly the kind, nothing more', () => {
  assert.equal(notifText('TotallyUnknownKind', {}), 'TotallyUnknownKind');
});

test('unknown kind with undefined payload: does not throw, gives the kind', () => {
  assert.equal(notifText('TotallyUnknownKind', undefined), 'TotallyUnknownKind');
});

test('unknown kind: stable key order regardless of insertion order', () => {
  const a = notifText('TotallyUnknownKind', { q: 7, r: 40 });
  const b = notifText('TotallyUnknownKind', { r: 40, q: 7 });
  assert.equal(a, b);
});

test('unknown kind: payload with more keys than the cap is truncated with an ellipsis', () => {
  const body = { a: 1, b: 2, c: 3, d: 4, e: 5, f: 6, g: 7, h: 8 };
  const line = notifText('TotallyUnknownKind', body);
  assert.match(line, /…$/);
});

// No regression — strings captured from a run against unmodified master
// (32c16fa), not read off the switch statement:
//   BuildComplete: "Build complete: granary"
//   ScoutReport:   "Explored (7, 40) — hills, copper"
//   UpkeepUnpaid:  "infantry unpaid (period 2) — one more unpaid period and they desert"
test('known kinds are unaffected by the default-arm change', () => {
  assert.equal(
    notifText('BuildComplete', { building_type: 'granary' }),
    'Build complete: granary',
  );
  assert.equal(
    notifText('ScoutReport', { q: 7, r: 40, terrain: 'hills', copper_deposit: true }),
    'Explored (7, 40) — hills, copper',
  );
  assert.equal(
    notifText('UpkeepUnpaid', { unit_type: 'infantry', unpaid_periods: 2, periods_until_desertion: 1 }),
    'infantry unpaid (period 2) — one more unpaid period and they desert',
  );
});

// The actual notification Timothy received in the acceptance world 2026-08-05
// — this is the line the default arm must now produce for it.
test('FieldBattleWon with the real acceptance-world payload', () => {
  assert.equal(notifText('FieldBattleWon', { q: 7, r: 40 }), 'FieldBattleWon — q: 7, r: 40');
});

// Stridsrapport (megaron_plan_stridsrapport.md §S5): the KR3 battle-end
// notification kind is 'BattleWon'/'BattleLost' (BattleTickHandler.
// notifyBattleEnded), not the plan's original field-specific
// FieldBattleWon/Lost — the case above is untouched. Red-before this slice:
// BattleWon/BattleLost had no case, so this fell to the raw-payload default.
test('BattleLost renders opponent name and both sides losses, no place', () => {
  const line = notifText('BattleLost', {
    role: 'defender', outcome: 'attacker_wins', opponent_name: 'Wanax3',
    own_unit: { type: 'spearman', size_before: 100, size_after: 41, pop_lost: 59 },
    enemy_unit: { type: 'elite_infantry', size_before: 100, size_after: 77 },
    q: 7, r: 40,
  });
  assert.equal(
    line,
    "Defeat at (7, 40) — your spearman (100→41, −59 dead) vs Wanax3's elite_infantry (100→77). The field was lost.",
  );
});

test('BattleWon uses place name when the server resolved a settlement', () => {
  const line = notifText('BattleWon', {
    role: 'attacker', outcome: 'attacker_wins', opponent_name: 'Wanax3',
    own_unit: { type: 'spearman', size_before: 1000, size_after: 940, pop_lost: 60 },
    enemy_unit: { type: 'spearman', size_before: 10, size_after: 0 },
    q: 1, r: 0, place: 'PolisCm4',
  });
  assert.equal(
    line,
    "Victory at PolisCm4 — your spearman (1000→940, −60 dead) vs Wanax3's spearman (10→0). The field was taken.",
  );
});

test('mutual_wipe trailer reads as neither side holding', () => {
  const line = notifText('BattleLost', {
    role: 'attacker', outcome: 'mutual_wipe', opponent_name: 'Wanax3',
    own_unit: { type: 'spearman', size_before: 50, size_after: 0, pop_lost: 50 },
    enemy_unit: { type: 'spearman', size_before: 50, size_after: 0 },
    q: 1, r: 0,
  });
  assert.match(line, /No survivors on either side\.$/);
});

// megaron_plan_skeppsreparation.md Slice B point 6 — BattleTickHandler.
// notifyShipDamaged's payload. Red-before this slice: ShipDamaged had no
// case, so it fell to the raw-payload default.
test('ShipDamaged: sunk reads as a loss, not a hull number', () => {
  const line = notifText('ShipDamaged', { unit_type: 'galley', hull: 0, hull_max: 5, sunk: true, returning_home: false });
  assert.equal(line, 'Your galley was sunk in battle');
});

test('ShipDamaged: routed survivor is limping home for repair', () => {
  const line = notifText('ShipDamaged', { unit_type: 'war_galley', hull: 3, hull_max: 5, sunk: false, returning_home: true });
  assert.equal(line, 'Your war_galley took damage (hull 3/5) and is limping home for repair');
});

test('ShipDamaged: winning side keeps its orders', () => {
  const line = notifText('ShipDamaged', { unit_type: 'merchantman', hull: 4, hull_max: 5, sunk: false, returning_home: false });
  assert.equal(line, 'Your merchantman took damage (hull 4/5) but holds its orders');
});

// A7 (megaron_mvp_mandag.md §A7): 16 NotifyPlayer kinds fell through to the
// raw-payload default before this slice — real server payloads (grepped from
// each call site, not invented) now get real text. Red-before: every one of
// these produced "<Kind> — key: value, …" via the default arm.
test('A7: the sixteen previously-uncased kinds get real text, not the default arm', () => {
  assert.equal(
    notifText('SettlementCaptured', { settlement_id: 'x', role: 'defender' }),
    'One of your settlements has fallen to conquest',
  );
  assert.equal(
    notifText('SettlementCaptured', { settlement_id: 'x', role: 'attacker' }),
    'Your army has taken an enemy settlement',
  );
  assert.equal(
    notifText('SettlementDefended', { role: 'defender', outcome: 'attacker_routed' }),
    'Your settlement held — the attacking army was routed',
  );
  assert.equal(
    notifText('SettlementSacked', { role: 'attacker', looted: { copper: 12, grain: 300 } }),
    'You sacked and razed a settlement — looted 12 copper, 300 grain',
  );
  assert.equal(
    notifText('SettlementSacked', { role: 'defender' }),
    'Your settlement was sacked and razed',
  );
  assert.equal(
    notifText('CityCollapsed', { name: 'PolisCm4', last_settlement: true }),
    'PolisCm4 has collapsed — that was your last settlement',
  );
  assert.equal(
    notifText('UnitLostAtSea', { unit_type: 'infantry', lost: 40, reason: 'grain_shortage' }),
    'infantry lost at sea — the ship starved, 40 men gone',
  );
  assert.equal(
    notifText('CaravanSeized', { transport_id: 'x', q: 3, r: 4 }),
    'You seized an enemy caravan at (3, 4)',
  );
  assert.equal(
    notifText('CaravanRaided', { transport_id: 'x', q: 3, r: 4 }),
    'Your caravan was raided at (3, 4)',
  );
  assert.equal(
    notifText('MarchStalled', { unit_id: 'x', reason: 'system fault, reissue the order' }),
    'system fault, reissue the order',
  );
  assert.equal(
    notifText('UnitArrived', { unit_id: 'x', type: 'ship', q: 5, r: 6, status: 'positioned', stance: 'sentry' }),
    'ship arrived at (5, 6) — positioned, standing sentry',
  );
  assert.equal(
    notifText('UnitExploreReturned', { unit_id: 'x', q: 1, r: 1 }),
    'Scout returning home',
  );
  assert.equal(
    notifText('OrderFailed', { verb: 'recall', reason: 'the army had already resolved' }),
    'Order failed (recall): the army had already resolved',
  );
  assert.equal(
    notifText('UnitRecalled', { unit_id: 'x', target_q: 2, target_r: 3 }),
    'Unit recalled — new course to (2, 3)',
  );
  assert.equal(
    notifText('UnitRedirected', { unit_id: 'x', target_q: 2, target_r: 3 }),
    'Unit redirected — new course to (2, 3)',
  );
  assert.equal(
    notifText('SitosGranaryRelease', { food_released: 450, coverage_days: 3, granary_empty: true }),
    "Granary released 450 grain (3 days' coverage) — granary now empty",
  );
  assert.equal(
    notifText('TransferDelivered', { dest_name: 'PolisCm4', goods: [{ good_key: 'grain', quantity: 200 }] }),
    'Transfer delivered to PolisCm4: 200 grain',
  );
  assert.equal(
    notifText('SentryAlerted', { foreign_owner: 'Wanax3', foreign_type: 'chariot', q: 9, r: 9 }),
    "Sentry spotted Wanax3's chariot at (9, 9)",
  );
});
