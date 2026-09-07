import test from 'node:test';
import assert from 'node:assert/strict';
import { State } from '../../state.js';
import {
  loadEconomyDrawer, loadTransferGoods, startTransfer, atStorageCeiling, goodsRateCell,
  parseGoodAmountPairs, renderStandingOrdersHTML,
  settlementFoodRow, sortSettlementRows, renderSettlementsOverviewHTML,
  rosterLocation, renderRosterHTML,
} from './economy.js';

test('atStorageCeiling: mirrors keryx — >=99% of a positive cap, nothing without a cap', () => {
  assert.equal(atStorageCeiling(1000000, 1000000), true);
  assert.equal(atStorageCeiling(990000, 1000000), true);   // exactly 99%
  assert.equal(atStorageCeiling(989999, 1000000), false);  // just under
  assert.equal(atStorageCeiling(1000000, 0), false);       // no cap = never full
  assert.equal(atStorageCeiling(50, -1), false);
});

test('goodsRateCell: a good at cap reads "full" (dimmed) not green growth, and names the wasted rate', () => {
  const html = goodsRateCell({ amount: 1000000, cap: 1000000, rate_per_tick: 12.3 });
  assert.match(html, /class="goods-atcap"/);   // dimmed, not --safe green
  assert.match(html, /full/);
  assert.match(html, /\+12\.3 lost/);           // the wasted labour is visible
  assert.doesNotMatch(html, /--safe/);
});

test('goodsRateCell: at cap but idle (rate 0) reads plain "full", no wasted-rate tail', () => {
  const html = goodsRateCell({ amount: 1000000, cap: 1000000, rate_per_tick: 0 });
  assert.match(html, /class="goods-atcap"/);
  assert.match(html, />full</);
  assert.doesNotMatch(html, /lost/);
});

test('goodsRateCell: below cap and producing keeps the green growth cell', () => {
  const html = goodsRateCell({ amount: 500, cap: 1000000, rate_per_tick: 4.5 });
  assert.match(html, /var\(--safe\)/);
  assert.match(html, /\+4\.5\/tick/);
  assert.doesNotMatch(html, /goods-atcap/);
});

test('goodsRateCell: below cap and idle renders an empty cell', () => {
  assert.equal(goodsRateCell({ amount: 500, cap: 1000000, rate_per_tick: 0 }), '<td></td>');
});

test('AK1: economy.js imports under node --test without a DOM (proves the misc.js pointerdown listener no longer runs at module scope)', () => {
  assert.equal(typeof loadEconomyDrawer, 'function');
  assert.equal(typeof loadTransferGoods, 'function');
  assert.equal(typeof startTransfer, 'function');
});

test('parseGoodAmountPairs: matches keryx cmd_route.go\'s good:amount,good:amount shape', () => {
  assert.deepEqual(parseGoodAmountPairs('grain:200,fish:50'), [
    { good_key: 'grain', amount: 200 },
    { good_key: 'fish', amount: 50 },
  ]);
  assert.deepEqual(parseGoodAmountPairs(' grain : 200 , fish:50 '), [
    { good_key: 'grain', amount: 200 },
    { good_key: 'fish', amount: 50 },
  ]);
});

test('parseGoodAmountPairs: empty/blank input is an empty list, not an error', () => {
  assert.deepEqual(parseGoodAmountPairs(''), []);
  assert.deepEqual(parseGoodAmountPairs('   '), []);
});

test('parseGoodAmountPairs: a malformed pair is dropped, not thrown — a typo must not crash the form', () => {
  assert.deepEqual(parseGoodAmountPairs('grain:200,nonsense,fish:abc,stone:20'), [
    { good_key: 'grain', amount: 200 },
    { good_key: 'stone', amount: 20 },
  ]);
});

test('renderStandingOrdersHTML: empty list renders the empty-state message', () => {
  assert.match(renderStandingOrdersHTML([]), /No standing orders yet/);
});

test('renderStandingOrdersHTML: an active order offers Pause, a paused one offers Resume and shows its reason', () => {
  const active = renderStandingOrdersHTML([{ id: 'a1', from_name: 'Petras', to_name: 'Colony', status: 'active' }]);
  assert.match(active, /Petras/);
  assert.match(active, /Colony/);
  assert.match(active, /pauseStandingOrder\('a1'\)/);
  assert.doesNotMatch(active, /resumeStandingOrder/);

  const paused = renderStandingOrdersHTML([{
    id: 'p1', from_name: 'Petras', to_name: 'Colony', status: 'paused',
    pause_reason: 'no spare workforce at the crewing settlement',
  }]);
  assert.match(paused, /resumeStandingOrder\('p1'\)/);
  assert.match(paused, /no spare workforce/);
});

// ── S1: settlements overview (megaron_plan_stad_vs_ekonomi.md §3) ──────────

test('settlementFoodRow: a self-sufficient, well-stocked settlement reads "stable"', () => {
  const pd = {
    population: 800, grain_prod_rate: 10, grain_consum_rate: 8, food_self_sufficient: true,
    sitos: { coverage_ticks: 20, low_ticks: 10, high_ticks: 30, granary_total: 500, food_net_per_tick: 2 },
  };
  const row = settlementFoodRow({ id: 's1', name: 'Petras', is_capital: true }, pd);
  assert.equal(row.population, 800);
  assert.equal(row.grainRate, 2);
  assert.equal(row.severity, 'rest');
  assert.equal(row.label, 'stable');
  assert.equal(row.isCapital, true);
});

test('settlementFoodRow: empty granary and shrinking reads "starving" and ranks worst (except cannot-feed)', () => {
  const pd = {
    population: 300, grain_prod_rate: 1, grain_consum_rate: 6, food_self_sufficient: true,
    sitos: { coverage_ticks: 1, low_ticks: 10, high_ticks: 30, granary_total: 0, food_net_per_tick: -5 },
  };
  const row = settlementFoodRow({ id: 's2', name: 'Zakros' }, pd);
  assert.equal(row.severity, 'empty-shrinking');
  assert.equal(row.label, 'starving');
  assert.equal(row.rank, 0);
});

test('settlementFoodRow: food_self_sufficient===false overrides the granary state — always the worst, even mid-store', () => {
  const pd = {
    population: 300, grain_prod_rate: 5, grain_consum_rate: 20, food_self_sufficient: false,
    sitos: { coverage_ticks: 40, low_ticks: 10, high_ticks: 30, granary_total: 9000, food_net_per_tick: 5 },
  };
  const row = settlementFoodRow({ id: 's3', name: 'Gournia' }, pd);
  assert.equal(row.label, 'cannot feed itself');
  assert.equal(row.rank, -1);
});

test('settlementFoodRow: no province data (failed fetch) renders "no data" instead of throwing or guessing', () => {
  const row = settlementFoodRow({ id: 's4', name: 'Unknown' }, null);
  assert.equal(row.label, 'no data');
  assert.equal(row.population, 0);
});

test('sortSettlementRows: default brist sort puts cannot-feed first, then starving, then stable', () => {
  const rows = [
    { name: 'Stable', population: 1, grainRate: 1, coverage: 20, rank: 3 },
    { name: 'Starving', population: 1, grainRate: -1, coverage: 1, rank: 0 },
    { name: 'CannotFeed', population: 1, grainRate: -1, coverage: 40, rank: -1 },
  ];
  const sorted = sortSettlementRows(rows, 'brist', 'asc');
  assert.deepEqual(sorted.map(r => r.name), ['CannotFeed', 'Starving', 'Stable']);
});

test('sortSettlementRows: sorting by population desc does not mutate the input array', () => {
  const rows = [{ name: 'A', population: 100 }, { name: 'B', population: 900 }];
  const sorted = sortSettlementRows(rows, 'population', 'desc');
  assert.deepEqual(sorted.map(r => r.name), ['B', 'A']);
  assert.deepEqual(rows.map(r => r.name), ['A', 'B']); // original order untouched
});

test('renderSettlementsOverviewHTML: empty rows render nothing', () => {
  assert.equal(renderSettlementsOverviewHTML([], 'brist', 'asc'), '');
});

test('renderSettlementsOverviewHTML: each row links to openCitySettlement and shows name/pop/rate/status', () => {
  const rows = [{ id: 'p1', name: 'Petras', isCapital: true, population: 800, grainRate: -2.5, coverage: 3.2, severity: 'release', label: 'drawing down reserve', rank: 1 }];
  const html = renderSettlementsOverviewHTML(rows, 'brist', 'asc');
  assert.match(html, /openCitySettlement\('p1'\)/);
  assert.match(html, /Petras/);
  assert.match(html, /★/);
  assert.match(html, />800</);
  assert.match(html, /-2\.5\/tick/);
  assert.match(html, /var\(--danger\)/); // negative rate reads as danger
  assert.match(html, /sitos-state-release/);
  assert.match(html, /drawing down reserve/);
});

test('renderSettlementsOverviewHTML: the active sort column shows its arrow, others do not', () => {
  const rows = [{ id: 'p1', name: 'Petras', isCapital: false, population: 1, grainRate: 0, coverage: 0, severity: 'rest', label: 'stable', rank: 3 }];
  const html = renderSettlementsOverviewHTML(rows, 'population', 'desc');
  assert.match(html, /Pop ▼/);
  assert.doesNotMatch(html, /Settlement ▼/);
  assert.doesNotMatch(html, /Settlement ▲/);
});

test('loadEconomyDrawer: with no owned settlements, renders the empty-state message and returns before touching anything else', async () => {
  const bodyStub = { innerHTML: '' };
  const calls = [];
  globalThis.document = {
    getElementById(id) {
      calls.push(id);
      return id === 'economy-body' ? bodyStub : null;
    },
  };
  State.provinceData = []; // no own settlements at all
  await loadEconomyDrawer();
  assert.match(bodyStub.innerHTML, /No settlements/);
  // Only the one lookup for #economy-body — the early return means it never
  // goes on to build tabs or fetch goods.
  assert.deepEqual(calls, ['economy-body']);
  delete globalThis.document;
});

// ── Gubbrulla (megaron_plan_webb_gubbrulla.md) — webb-syskon till `keryx
// roster` ────────────────────────────────────────────────────────────────
// Fixture is the exact payload documented for
// GET .../settlements/placement-roster in the plan.
const ROSTER_FIXTURE = [
  {
    id: 's-knossos', province_id: 'p-knossos', name: 'Knossos', is_capital: true,
    total_gubbar: 8, placed: 5, idle: 3,
    assignments: [
      { good_key: 'stone', target_kind: 'building', building_type: 'stonequarry', count: 1 },
      { good_key: 'grain', target_kind: 'hex', hex_ordinal: 1, hex_q: -2, hex_r: 0, count: 3 },
      { good_key: 'fish', target_kind: 'hex', hex_ordinal: 2, hex_q: -2, hex_r: 1, count: 1 },
    ],
  },
];

test('rosterLocation: mirrors keryx cmd_roster.go — hex #ordinal, hex (q,r) fallback, or building type', () => {
  assert.equal(rosterLocation({ target_kind: 'hex', hex_ordinal: 5 }), 'hex #5');
  assert.equal(rosterLocation({ target_kind: 'hex', hex_ordinal: null, hex_q: -2, hex_r: 1 }), 'hex (-2,1)');
  assert.equal(rosterLocation({ target_kind: 'hex' }), 'hex');
  assert.equal(rosterLocation({ target_kind: 'building', building_type: 'stonequarry' }), 'stonequarry');
  assert.equal(rosterLocation({ target_kind: 'weird' }), 'weird');
});

test('renderRosterHTML: empty roster (founder phase) renders an honest empty state, no crash', () => {
  const html = renderRosterHTML([]);
  assert.match(html, /No settlements yet/);
});

test('renderRosterHTML: realm summary sums placed/idle/total across settlements', () => {
  const html = renderRosterHTML(ROSTER_FIXTURE);
  assert.match(html, /Gubbar across 1 settlement — 5\/8 placed/);
  assert.match(html, /class="roster-idle">3 idle</);
});

test('renderRosterHTML: a settlement with zero idle shows no idle note (mirrors keryx, which omits it too)', () => {
  const html = renderRosterHTML([{ ...ROSTER_FIXTURE[0], idle: 0, placed: 8 }]);
  assert.doesNotMatch(html, /idle<\/span>/);
});

test('renderRosterHTML: renders capital marker, placed/total, and a hex #N assignment row', () => {
  const html = renderRosterHTML(ROSTER_FIXTURE);
  assert.match(html, /Knossos/);
  assert.match(html, /★/);
  assert.match(html, /5\/8 placed/);
  assert.match(html, /hex #1/);
  assert.match(html, /3× grain/);
  assert.match(html, /1× stone/);
  assert.match(html, /stonequarry/);
});

test('renderRosterHTML: a settlement with no assignments reads "every citizen is idle", not an empty table', () => {
  const html = renderRosterHTML([{ id: 's2', name: 'Founding', is_capital: false, total_gubbar: 2, placed: 0, idle: 2, assignments: [] }]);
  assert.match(html, /every citizen is idle/);
});

test('renderRosterHTML: no hardcoded hex colors — only CSS custom properties or classes', () => {
  const html = renderRosterHTML(ROSTER_FIXTURE);
  assert.doesNotMatch(html, /#[0-9a-fA-F]{3,6}(?!\d)/); // no literal hex color codes (hex #N location strings use a space before '#', not this pattern)
});
