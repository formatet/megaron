import test from 'node:test';
import assert from 'node:assert/strict';
import { loyaltyLogRowsHTML, buildingOptionsHTML } from './city.js';

// megaron_plan_byggkatalogen_i_webben.md: the Construct dropdown used to be
// 14 hardcoded <option> rows (silver_mine missing entirely, every cost
// pre-mig-136). buildingOptionsHTML renders it from a GET /api/v1/buildings
// fixture instead — same "pure string builder" testability as
// loyaltyLogRowsHTML above.

test('BK1: silver_mine is buildable from the catalogue (was missing entirely from the hardcoded list)', () => {
  const html = buildingOptionsHTML([
    { type: 'silver_mine', costs: { timber: 1.429, stone: 28.571 }, purpose: 'Extracts silver from silver deposits in catchment (requires deposit)', requires_deposits: ['silver'] },
  ]);
  assert.match(html, /value="silver_mine"/);
  assert.match(html, /Silver Mine/);
});

test('BK2: costs are read from the catalogue, not hardcoded — mutating the fixture mutates the rendered string', () => {
  const low = buildingOptionsHTML([{ type: 'farm', costs: { timber: 0.769, stone: 9.231 }, purpose: 'x' }]);
  const high = buildingOptionsHTML([{ type: 'farm', costs: { timber: 5, stone: 40 }, purpose: 'x' }]);
  assert.match(low, /1 timber 9 stone/);
  assert.match(high, /5 timber 40 stone/);
  assert.notEqual(low, high);
});

test('BK3: cost rounding matches the CLI (%.0f per good, cmd_build.go), not the pre-mig-136 hardcoded values', () => {
  // Post-mig-136 farm cost (province/building.go BuildingSpecs) — the OLD
  // hardcoded string said "50 timber 20 stone", off by ~65×/~2×.
  const html = buildingOptionsHTML([{ type: 'farm', costs: { timber: 0.769, stone: 9.231 }, purpose: 'x' }]);
  assert.match(html, /1 timber 9 stone/);
  assert.doesNotMatch(html, /50 timber/);
});

test('BK4: a deposit gate is rendered so a player can see why a building is/isn\'t available', () => {
  const html = buildingOptionsHTML([
    { type: 'silver_mine', costs: { timber: 1, stone: 9 }, purpose: 'Extracts silver', requires_deposits: ['silver'] },
  ]);
  assert.match(html, /requires silver deposit/);
});

test('BK5: a coastal gate and a terrain gate are both rendered', () => {
  const html = buildingOptionsHTML([
    { type: 'harbour', costs: { timber: 3, stone: 37 }, purpose: 'Enables fish production', requires_coastal: true },
    { type: 'winery', costs: { timber: 1, stone: 23 }, purpose: 'Increases wine production', requires_terrain: ['hills', 'plains', 'scrub_maquis'] },
  ]);
  assert.match(html, /requires coastal/);
  assert.match(html, /hills\/plains\/scrub_maquis terrain/);
});

test('BK6: wall keeps its old hardcoded upgrade-ladder copy — the catalogue entry has no upgrade_costs for it (stop condition, megaron_plan_byggkatalogen_i_webben.md §7)', () => {
  const html = buildingOptionsHTML([
    { type: 'wall', costs: { timber: 0.541, stone: 19.459 }, purpose: 'Adds a wall tier', max_level: 1 },
  ]);
  assert.match(html, /Wall — upgrade \(Palisade→Stone Wall→Bronze Wall\)/);
  assert.doesNotMatch(html, /19 stone/);
});

test('BK7: an entry with no gates renders no "requires" clause', () => {
  const html = buildingOptionsHTML([{ type: 'farm', costs: { timber: 1, stone: 9 }, purpose: 'Raises grain' }]);
  assert.doesNotMatch(html, /requires/);
});

// megaron_plan_webbytor_keryx_paritet.md, Slice LOYALTY-LOG: the city drawer
// shows the loyalty VALUE but never WHY it changed. loyaltyLogRowsHTML is the
// pure string builder for the new "Lojalitetslogg" section (same pattern as
// economy.js's goodsRateCell) — a fake loyalty-log response in, HTML out, no
// DOM or fetch needed to test it.

test('AK1: city.js imports under node --test without a DOM', () => {
  assert.equal(typeof loyaltyLogRowsHTML, 'function');
});

test('empty loyalty-log renders a friendly empty-state, not a bare table', () => {
  const html = loyaltyLogRowsHTML([]);
  assert.match(html, /empty-state/);
  assert.doesNotMatch(html, /<table/);
});

test('a positive delta is signed with a leading + and the --safe tone', () => {
  const html = loyaltyLogRowsHTML([
    { id: 1, event_type: 'gift', loyalty_delta: 1, reason: 'Received a significant gift', created_at: '2026-08-16T10:00:00Z' },
  ]);
  assert.match(html, /\+1/);
  assert.match(html, /var\(--safe\)/);
  assert.match(html, /Received a significant gift/);
});

test('a negative delta keeps its own minus sign (no double sign) and the --accent tone', () => {
  const html = loyaltyLogRowsHTML([
    { id: 2, event_type: 'revolt_risk', loyalty_delta: -2, reason: 'Garrison dominated by foreign troops', created_at: '2026-08-15T09:00:00Z' },
  ]);
  assert.match(html, />-2</);
  assert.doesNotMatch(html, /\+-2/);
  assert.match(html, /var\(--accent\)/);
});

test('rows are rendered in the order given (server already sorts newest-first) — not re-sorted client-side', () => {
  const html = loyaltyLogRowsHTML([
    { id: 3, event_type: 'a', loyalty_delta: 1, reason: 'newest', created_at: '2026-08-16T10:00:00Z' },
    { id: 2, event_type: 'b', loyalty_delta: -1, reason: 'older', created_at: '2026-08-15T10:00:00Z' },
  ]);
  assert.ok(html.indexOf('newest') < html.indexOf('older'));
});

test('a hostile reason string is escaped, not injected as markup', () => {
  const html = loyaltyLogRowsHTML([
    { id: 4, event_type: 'x', loyalty_delta: 1, reason: '<img src=x onerror=alert(1)>', created_at: '2026-08-16T10:00:00Z' },
  ]);
  assert.doesNotMatch(html, /<img/);
  assert.match(html, /&lt;img/);
});

test('a zero delta (if it ever occurs) gets no sign and the neutral --text-dim tone', () => {
  const html = loyaltyLogRowsHTML([
    { id: 5, event_type: 'noop', loyalty_delta: 0, reason: 'no change', created_at: '2026-08-16T10:00:00Z' },
  ]);
  assert.match(html, />0</);
  assert.match(html, /var\(--text-dim\)/);
});
