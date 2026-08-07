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
