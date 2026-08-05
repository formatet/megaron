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
