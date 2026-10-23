import test from 'node:test';
import assert from 'node:assert/strict';
import { State } from '../../state.js';
import {
  loadEconomyDrawer, loadTransferGoods, startTransfer, atStorageCeiling, goodsRateCell,
  parseGoodAmountPairs, renderStandingOrdersHTML,
  settlementFoodRow, sortSettlementRows, renderSettlementsOverviewHTML,
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
  assert.match(html, /\+12.3 lost per day/);           // the wasted labour is visible
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
  assert.match(html, /var\(--safe-text\)/);
  assert.match(html, /\+4.5 per day/);
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

// megaron_plan_sjohandel_kraver_skepp.md R4: a naval route names its bound
// ship; a route with no ship yet (never dispatched, or still land) shows
// nothing extra.
test('renderStandingOrdersHTML: a naval route names its bound ship', () => {
  const withShip = renderStandingOrdersHTML([{
    id: 'n1', from_name: 'Byblos', to_name: 'Ugarit', status: 'active', ship_name: 'White Dolphin, Merchantman',
  }]);
  assert.match(withShip, /White Dolphin, Merchantman/);

  const noShip = renderStandingOrdersHTML([{ id: 'l1', from_name: 'Mycenae', to_name: 'Tiryns', status: 'active' }]);
  assert.doesNotMatch(noShip, /⛵/);
});

// ── S1: settlements overview (megaron_plan_stad_vs_ekonomi.md §3) ──────────

test('settlementFoodRow: a self-sufficient, well-stocked settlement reads "stocks growing"', () => {
  const pd = {
    population: 800, grain_prod_rate: 10, grain_consum_rate: 8, food_self_sufficient: true,
    food: { coverage_ticks: 20, food_net_per_tick: 2 },
  };
  const row = settlementFoodRow({ id: 's1', name: 'Petras', is_capital: true }, pd);
  assert.equal(row.population, 800);
  assert.equal(row.grainRate, 2);
  assert.equal(row.severity, 'growing');
  assert.equal(row.label, 'stocks growing');
  assert.equal(row.isCapital, true);
});

test('settlementFoodRow: server reports shrinking reads "starving" and ranks worst (except cannot-feed)', () => {
  const pd = {
    population: 300, grain_prod_rate: 1, grain_consum_rate: 6, food_self_sufficient: true,
    food: { coverage_ticks: 1, food_net_per_tick: -5, growth_state: 'shrinking' },
  };
  const row = settlementFoodRow({ id: 's2', name: 'Zakros' }, pd);
  assert.equal(row.severity, 'starving');
  assert.equal(row.label, 'starving');
  assert.equal(row.rank, 0);
});

test('settlementFoodRow: food_self_sufficient===false overrides stock direction — always the worst, even with full stores', () => {
  const pd = {
    population: 300, grain_prod_rate: 5, grain_consum_rate: 20, food_self_sufficient: false,
    food: { coverage_ticks: 40, food_net_per_tick: 5 },
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
  const rows = [{ id: 'p1', name: 'Petras', isCapital: true, population: 800, grainRate: -2.5, coverage: 3.2, severity: 'declining', label: 'stocks falling', rank: 1 }];
  const html = renderSettlementsOverviewHTML(rows, 'brist', 'asc');
  assert.match(html, /openCitySettlement\('p1'\)/);
  assert.match(html, /Petras/);
  assert.match(html, /★/);
  assert.match(html, />800</);
  assert.match(html, /-2.5 per day/);
  assert.match(html, /var\(--danger\)/); // negative rate reads as danger
  assert.match(html, /food-state-declining/);
  assert.match(html, /stocks falling/);
});

test('renderSettlementsOverviewHTML: the active sort column shows its arrow, others do not', () => {
  const rows = [{ id: 'p1', name: 'Petras', isCapital: false, population: 1, grainRate: 0, coverage: 0, severity: 'stable', label: 'stable', rank: 3 }];
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

test('settlement food column follows full fish-inclusive authoritative net',()=>{
 const row=settlementFoodRow({id:'p',name:'Coast'},{population:1000,grain_prod_rate:0.5,grain_consum_rate:5,food:{coverage_ticks:2,food_net_per_tick:0.5}});
 assert.equal(row.grainRate,0.5);
});

test('AIRound3: overview sorts and displays food after army, keeps people-only fallback',()=>{
 const city={id:'s',name:'Scheria'};
 const data={population:1000,food_net_per_tick_after_upkeep:-1.5,food:{food_net_per_tick:0.5,coverage_ticks:1}};
 assert.equal(settlementFoodRow(city,data).grainRate,-1.5);
 const html=renderSettlementsOverviewHTML([settlementFoodRow(city,data)]);
 assert.match(html,/Food after army/);assert.match(html,/-1.5 per day/);
 delete data.food_net_per_tick_after_upkeep;assert.equal(settlementFoodRow(city,data).grainRate,0.5);
});


test('food status separates declining stock from hunger and never invents missing balances', () => {
  const city = {id: 'p', name: 'Coast'};
  const data = {food: {coverage_ticks: 0, food_net_per_tick: 2, growth_state: 'growing'}};
  assert.equal(settlementFoodRow(city, data).label, 'stocks growing');
  data.food.food_net_per_tick = -2;
  data.food.growth_state = 'holding';
  assert.equal(settlementFoodRow(city, data).label, 'stocks falling');
  data.food.food_net_per_tick = 0;
  assert.equal(settlementFoodRow(city, data).label, 'stable');
  data.food.growth_state = 'shrinking';
  assert.equal(settlementFoodRow(city, data).label, 'starving');
  for (const food of [undefined, {}, {coverage_ticks: NaN}, {coverage_ticks: 3}]) {
    const row = settlementFoodRow(city, {food});
    assert.equal(row.label, 'no data');
    assert.equal(row.grainRate, null);
    assert.match(renderSettlementsOverviewHTML([row]), /not reported/);
  }
});

test('standing route list shows configured stock targets and return floors, including zero and decimals',()=>{
 const html=renderStandingOrdersHTML([{id:'route',from_name:'Source',to_name:'Colony',status:'active',outbound:[{good_key:'grain',threshold:200},{good_key:'fish',threshold:50.5}],return:[{good_key:'silver',floor:0},{good_key:'stone',floor:20}]}]);
 assert.match(html,/Keep at destination: grain 200, fish 50\.5/);
 assert.match(html,/Bring home above: silver 0, stone 20/);
 const empty=renderStandingOrdersHTML([{id:'empty',from_name:'Source',to_name:'Other',status:'paused',outbound:[{good_key:'grain',threshold:0}],return:[]}]);
 assert.match(empty,/Keep at destination: grain 0/);assert.match(empty,/Return cargo: none/);
 const small=renderStandingOrdersHTML([{id:'small',from_name:'Source',to_name:'Other',status:'active',outbound:[{good_key:'fish',threshold:0.04}],return:[{good_key:'silver',floor:0.125}]}]);
 assert.match(small,/fish 0\.04/);assert.match(small,/silver 0\.125/);
 const escaped=renderStandingOrdersHTML([{id:'escape',from_name:'Source',to_name:'Other',status:'active',outbound:[{good_key:'<script>',threshold:2}]}]);
 assert.ok(!escaped.includes('<script>'));assert.match(escaped,/&lt;script&gt;/);
});
