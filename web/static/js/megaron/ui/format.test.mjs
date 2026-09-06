import test from 'node:test';
import assert from 'node:assert/strict';
import { notifText, fmtSilver, formatApiError } from './format.js';

// Decimal-silver formatter (Timothy 2026-08-13: sexagesimal shekel/mina/talang
// retired). One decimal, trailing ".0" dropped, always suffixed " silver".
test('fmtSilver: whole amounts render as plain integers', () => {
  assert.equal(fmtSilver(0), '0 silver');
  assert.equal(fmtSilver(42), '42 silver');
  assert.equal(fmtSilver(3600), '3600 silver');
});

test('fmtSilver: fractional amounts keep one decimal', () => {
  assert.equal(fmtSilver(3.24), '3.2 silver');
  assert.equal(fmtSilver(142.05), '142.1 silver');
});

test('fmtSilver: a value that rounds to a whole number drops the decimal', () => {
  assert.equal(fmtSilver(9.99), '10 silver');
});

test('fmtSilver: missing/invalid input is treated as zero, never NaN', () => {
  assert.equal(fmtSilver(undefined), '0 silver');
  assert.equal(fmtSilver(null), '0 silver');
});

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
    notifText('UnitReturnedStarving', { unit_id: 'x', q: 1, r: 1, crew_after: 10 }),
    "Ship's crew starved to half strength (crew down to 10) — turning home on its own, sailing slower",
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

// megaron_plan_tre_tysta_notiserna.md: DivinePunishment/DivineBlessing/
// FoodShortfall used to fall to the raw-JSON default arm and none of them
// named the settlement. amount's UNIT depends on `type` for both divine
// kinds — a generic "+N" line would be wrong for two of three branches.
test('DivinePunishment names the settlement and the right unit per type', () => {
  assert.equal(
    notifText('DivinePunishment', { type: 'chariot_loss', amount: 18, name: 'Phaistos' }),
    'The gods scattered your war chariots in the night at Phaistos — 18 lost',
  );
  assert.equal(
    notifText('DivinePunishment', { type: 'ship_loss', amount: 1, name: 'Phaistos' }),
    'A divine storm claimed a vessel from your harbour at Phaistos',
  );
  assert.equal(
    notifText('DivinePunishment', { type: 'harvest_failure', amount: 1200, name: 'Phaistos' }),
    'The fields lay fallow by divine will at Phaistos — 1200 grain rotted',
  );
  assert.equal(
    notifText('DivinePunishment', { type: 'garrison_plague', amount: 20, name: 'Phaistos' }),
    'A pestilence swept the barracks at Phaistos — 20 fell',
  );
});

test('DivinePunishment without a name (older persisted notification) omits the place clause, not an empty one', () => {
  const line = notifText('DivinePunishment', { type: 'ship_loss', amount: 1 });
  assert.equal(line, 'A divine storm claimed a vessel from your harbour');
  assert.doesNotMatch(line, / at $/);
});

test('DivinePunishment unknown type falls to the raw-payload default, invents nothing', () => {
  const line = notifText('DivinePunishment', { type: 'locust_swarm', amount: 40, name: 'Phaistos' });
  assert.match(line, /^DivinePunishment/);
  assert.match(line, /locust_swarm/);
});

test('DivineBlessing names the settlement and the right unit per type', () => {
  assert.equal(
    notifText('DivineBlessing', { type: 'harvest_blessing', amount: 100, name: 'Phaistos' }),
    'The gods fill your granaries at Phaistos — +100 grain',
  );
  assert.equal(
    notifText('DivineBlessing', { type: 'divine_recruits', amount: 4, name: 'Phaistos' }),
    'Warriors answer a divine call at Phaistos — +4 men',
  );
  assert.equal(
    notifText('DivineBlessing', { type: 'sea_blessing', amount: 1, name: 'Phaistos' }),
    'Poseidon guides a vessel to your harbour at Phaistos — +1 galley',
  );
});

test('DivineBlessing unknown type falls to the raw-payload default, invents nothing', () => {
  const line = notifText('DivineBlessing', { type: 'unknown_favour', amount: 40, name: 'Phaistos' });
  assert.match(line, /^DivineBlessing/);
  assert.match(line, /unknown_favour/);
});

test('FoodShortfall names the settlement and the unmet ration', () => {
  assert.equal(
    notifText('FoodShortfall', { unmet: 312, name: 'Phaistos' }),
    'Phaistos went hungry today — 312 grain unmet',
  );
});

test('FoodShortfall without a name falls back to a generic settlement label', () => {
  assert.equal(
    notifText('FoodShortfall', { unmet: 312 }),
    'A settlement went hungry today — 312 grain unmet',
  );
});

// Blockad med enhet (megaron_plan_blockad_med_enhet.md, Timothy 2026-09-05):
// HexBlockaded/HexUnblockaded must name the settlement, the worker count
// (singular/plural), and carry q/r in the body so resolveDestination
// (dispatch_window.js) can jump the map straight to it.
test('HexBlockaded names the settlement, the worker count, and the hex', () => {
  assert.equal(
    notifText('HexBlockaded', { name: 'Petras', workers: 4, q: 14, r: 33 }),
    '4 workers in Petras have stopped — a foreign unit stands on (14, 33)',
  );
});

test('HexBlockaded uses singular phrasing for exactly one worker', () => {
  assert.equal(
    notifText('HexBlockaded', { name: 'Petras', workers: 1, q: 14, r: 33 }),
    '1 worker in Petras has stopped — a foreign unit stands on (14, 33)',
  );
});

test('HexUnblockaded names the settlement, the worker count, and the hex', () => {
  assert.equal(
    notifText('HexUnblockaded', { name: 'Petras', workers: 4, q: 14, r: 33 }),
    '4 workers in Petras have resumed work — the foreign unit at (14, 33) is gone',
  );
});

// SiegeStarted/SiegeLifted (economy.SyncSiegeState,
// megaron_plan_belagringsdispatch.md, Timothy 2026-09-06): a Wanax away for
// nine hours must learn a siege started or lifted — asynchronicity gate 2.
test('SiegeStarted names the settlement and the besieger', () => {
  assert.equal(
    notifText('SiegeStarted', { name: 'Petras', besieger_name: 'Idomeneus', q: 14, r: 33 }),
    'Petras is under siege — Idomeneus holds the approaches',
  );
});

test('SiegeStarted falls back to "an enemy" when besieger_name is missing', () => {
  assert.equal(
    notifText('SiegeStarted', { name: 'Petras', q: 14, r: 33 }),
    'Petras is under siege — an enemy holds the approaches',
  );
});

test('SiegeLifted names the settlement', () => {
  assert.equal(
    notifText('SiegeLifted', { name: 'Petras', q: 14, r: 33 }),
    'The siege of Petras has lifted',
  );
});

// Dispatches naming (megaron_plan_dispatches.md §4, bug report da660376):
// "'Scout returning home — arrives in ~10 min' — is it the galley that's
// meant?" — every unit-carrying kind now gets a `name` field from the
// server (unit.LoadDisplayName), and notifText must prefer it over the bare
// category. Bodies without a name (persisted before this slice, or a failed
// lookup) fall back to the old generic wording — proven above, unchanged.

test('UnitExploreReturned names the returning unit instead of a bare category', () => {
  assert.equal(
    notifText('UnitExploreReturned', { unit_id: 'x', q: 1, r: 1, name: 'Nomadic Host of formatet' }),
    'Nomadic Host of formatet returning home',
  );
});

test('UnitArrived names the unit over the type', () => {
  assert.equal(
    notifText('UnitArrived', { unit_id: 'x', type: 'ship', name: 'White Dolphin, Galley of Kydonia', q: 5, r: 6, status: 'positioned' }),
    'White Dolphin, Galley of Kydonia arrived at (5, 6) — positioned',
  );
});

test('UnitReturnedStarving names the ship possessively', () => {
  assert.equal(
    notifText('UnitReturnedStarving', { unit_id: 'x', q: 1, r: 1, crew_after: 10, name: 'White Dolphin, Galley of Kydonia' }),
    "White Dolphin, Galley of Kydonia's crew starved to half strength (crew down to 10) — turning home on its own, sailing slower",
  );
});

test('UnitAttrition and UnitDeserted name the unit', () => {
  assert.equal(
    notifText('UnitAttrition', { unit_type: 'infantry', name: '2nd Spearmen of Knossos', lost: 12, disbanded: false }),
    '2nd Spearmen of Knossos starving — lost 12 to hunger',
  );
  assert.equal(
    notifText('UnitDeserted', { unit_type: 'infantry', name: '2nd Spearmen of Knossos', disbanded: true }),
    '2nd Spearmen of Knossos deserted — unpaid, unit lost',
  );
});

test('UpkeepUnpaid names the unit', () => {
  assert.equal(
    notifText('UpkeepUnpaid', { unit_type: 'infantry', name: '2nd Spearmen of Knossos', unpaid_periods: 2, periods_until_desertion: 1 }),
    '2nd Spearmen of Knossos unpaid (period 2) — one more unpaid period and they desert',
  );
});

test('UnitLostAtSea names the unit', () => {
  assert.equal(
    notifText('UnitLostAtSea', { unit_type: 'infantry', name: '2nd Spearmen of Knossos', lost: 40, reason: 'grain_shortage' }),
    '2nd Spearmen of Knossos lost at sea — the ship starved, 40 men gone',
  );
});

test('UnitRecalled and UnitRedirected name the unit', () => {
  assert.equal(
    notifText('UnitRecalled', { unit_id: 'x', name: '2nd Spearmen of Knossos', target_q: 2, target_r: 3 }),
    '2nd Spearmen of Knossos recalled — new course to (2, 3)',
  );
  assert.equal(
    notifText('UnitRedirected', { unit_id: 'x', name: '2nd Spearmen of Knossos', target_q: 2, target_r: 3 }),
    '2nd Spearmen of Knossos redirected — new course to (2, 3)',
  );
});

test('OrderFailed prefixes the named unit when present', () => {
  assert.equal(
    notifText('OrderFailed', { verb: 'recall', name: '2nd Spearmen of Knossos', reason: 'the army had already resolved' }),
    '2nd Spearmen of Knossos — Order failed (recall): the army had already resolved',
  );
});

test('ShipDamaged and ShipRepaired name the ship over the bare type', () => {
  assert.equal(
    notifText('ShipDamaged', { unit_type: 'galley', name: 'White Dolphin, Galley of Kydonia', hull: 0, hull_max: 5, sunk: true, returning_home: false }),
    'Your White Dolphin, Galley of Kydonia was sunk in battle',
  );
  assert.equal(
    notifText('ShipRepaired', { unit_type: 'galley', name: 'White Dolphin, Galley of Kydonia', hull: 5 }),
    'Your White Dolphin, Galley of Kydonia is repaired (hull 5/5) and ready to sail',
  );
});

// formatApiError — bug report bee16ca7 (tick 230): the web threw away the
// server's structured 422 {"error":"insufficient_goods","missing":[...]}
// (writeGoodsError, api/handlers/helpers.go) and showed only the raw string
// "insufficient_goods". Every good must be named with both need and have.

test('insufficient_goods with one missing good names it with need and have', () => {
  const data = { error: 'insufficient_goods', missing: [{ good: 'timber', need: 40, have: 12 }] };
  assert.equal(formatApiError(data, 'Build failed.'), 'Not enough: timber 40 needed, 12 in store.');
});

test('insufficient_goods with several missing goods lists every one', () => {
  const data = {
    error: 'insufficient_goods',
    missing: [
      { good: 'timber', need: 40, have: 12 },
      { good: 'copper', need: 10, have: 0 },
    ],
  };
  assert.equal(
    formatApiError(data, 'Build failed.'),
    'Not enough: timber 40 needed, 12 in store; copper 10 needed, 0 in store.',
  );
});

test('insufficient_goods rounds fractional need/have so a lazy-eval float never reaches the eye', () => {
  const data = { error: 'insufficient_goods', missing: [{ good: 'grain', need: 39.999999, have: 4.2 }] };
  assert.equal(formatApiError(data, 'Build failed.'), 'Not enough: grain 40 needed, 4 in store.');
});

test('insufficient_goods with an empty missing array falls back to data.error', () => {
  const data = { error: 'insufficient_goods', missing: [] };
  assert.equal(formatApiError(data, 'Build failed.'), 'insufficient_goods');
});

test('a different error code is passed through untouched', () => {
  const data = { error: 'not_owner' };
  assert.equal(formatApiError(data, 'Build failed.'), 'not_owner');
});

test('an empty response body (fetchAuth catch-guard) falls back to the fallback string', () => {
  assert.equal(formatApiError({}, 'Build failed.'), 'Build failed.');
  assert.equal(formatApiError(null, 'Build failed.'), 'Build failed.');
  assert.equal(formatApiError(undefined, 'Build failed.'), 'Build failed.');
});
