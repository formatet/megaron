import test from 'node:test';
import assert from 'node:assert/strict';
import { buildingOptionsHTML, hexBuildOptionsHTML, buildQuoteHTML } from './city.js';

// megaron_plan_byggkatalogen_i_webben.md: the Construct dropdown used to be
// 14 hardcoded <option> rows (silver_mine missing entirely, every cost
// pre-mig-136). buildingOptionsHTML renders it from a GET /api/v1/buildings
// fixture instead — same "pure string builder" testability as
// other pure HTML builders.

test('BK1: mine is buildable from the catalogue (was missing entirely from the old hardcoded list)', () => {
  const html = buildingOptionsHTML([
    { type: 'mine', costs: { timber: 1.429, stone: 28.571 }, purpose: 'Extracts copper/tin/silver from a deposit hex in catchment', requires_deposits: ['copper', 'tin', 'silver'] },
  ]);
  assert.match(html, /value="mine"/);
  assert.match(html, /Mine/);
});

// megaron_plan_byggnad_pa_hex.md §A: silver_mine is retired — a mine on a
// silver-deposit hex now produces silver, exactly like copper/tin on their
// own hexes. Nothing in the web client should still name silver_mine.
test('silver_mine is gone from the Construct label map — an unlabelled fallback would betray a stale catalogue entry', () => {
  const html = buildingOptionsHTML([
    { type: 'silver_mine', costs: { timber: 1, stone: 9 }, purpose: 'x' },
  ]);
  assert.doesNotMatch(html, /Silver Mine/);
});

test('BK2: costs are read from the catalogue, not hardcoded — mutating the fixture mutates the rendered string', () => {
  const low = buildingOptionsHTML([{ type: 'farm', costs: { timber: 6, stone: 4 }, purpose: 'x' }]);
  const high = buildingOptionsHTML([{ type: 'farm', costs: { timber: 5, stone: 40 }, purpose: 'x' }]);
  assert.match(low, /6 timber 4 stone/);
  assert.match(high, /5 timber 40 stone/);
  assert.notEqual(low, high);
});

test('BK3: cost rounding matches the CLI (%.0f per good, cmd_build.go), not the pre-mig-136 hardcoded values', () => {
  // Farm cost under the recipe catalogue (byggkostnader, 6 timber 4 stone) (province/building.go BuildingSpecs) — the OLD
  // hardcoded string said "50 timber 20 stone".
  const html = buildingOptionsHTML([{ type: 'farm', costs: { timber: 6, stone: 4 }, purpose: 'x' }]);
  assert.match(html, /6 timber 4 stone/);
  assert.doesNotMatch(html, /50 timber/);
});

test('BK4: an option is name and price only — no purpose, no requires tail (Timothy 2026-09-30: too long)', () => {
  const html = buildingOptionsHTML([
    { type: 'mine', costs: { timber: 1, stone: 9 }, purpose: 'Extracts ore', requires_deposits: ['silver'] },
    { type: 'harbour', costs: { timber: 3, stone: 37 }, purpose: 'Enables fish production', requires_coastal: true },
  ]);
  assert.match(html, /<option value="mine">Mine — 1 timber 9 stone<\/option>/);
  assert.match(html, /<option value="harbour">Harbour — 3 timber 37 stone<\/option>/);
  assert.doesNotMatch(html, /requires|Extracts|Enables/);
});

test('BK6: wall keeps its old hardcoded upgrade-ladder copy — the catalogue entry has no upgrade_costs for it (stop condition, megaron_plan_byggkatalogen_i_webben.md §7)', () => {
  const html = buildingOptionsHTML([
    { type: 'wall', costs: { timber: 0.541, stone: 19.459 }, purpose: 'Adds a wall tier', max_level: 1 },
  ]);
  assert.match(html, /Wall — upgrade \(Palisade→Stone Wall→Bronze Wall\)/);
  assert.doesNotMatch(html, /19 stone/);
});

// hexBuildOptionsHTML: the Construct flow's hex picker for a hex-bound type
// (megaron_plan_byggnad_pa_hex.md §A2). Merges valid_hexes_for_building
// (server-computed "could build here now") with hexes[] entries that already
// carry a building of the SAME type — those are excluded from
// valid_hexes_for_building (they're the upgrade path, not a new build) so
// this function has to add them back in labelled as upgrades.

test('HX1: a valid, empty hex renders as a build option with its coords and terrain', () => {
  const html = hexBuildOptionsHTML('farm', {
    valid_hexes_for_building: { farm: [{ q: 3, r: -1, effect: 'this hex\'s grain output ×1.7 · space for 4 more workers' }] },
    hexes: [{ hex_q: 3, hex_r: -1, terrain: 'plains' }],
  });
  assert.match(html, /value="3,-1"/);
  assert.match(html, />\(3,-1\) Plains — this hex\'s grain output ×1\.7 · space for 4 more workers</);
});

test('HX2: a hex missing from valid_hexes_for_building but already carrying a SAME-type building is offered as an upgrade', () => {
  const html = hexBuildOptionsHTML('farm', {
    valid_hexes_for_building: { farm: [] },
    hexes: [{ hex_q: 5, hex_r: 2, terrain: 'plains', building: { type: 'farm', level: 1, upgrade_effect: 'this hex\'s grain output ×1.7 → ×2.4' } }],
  });
  assert.match(html, /value="5,2"/);
  assert.match(html, />\(5,2\) Plains — upgrade to L2: this hex\'s grain output ×1\.7 → ×2\.4</);
});

test('HX3: a hex carrying a DIFFERENT-type building is not offered at all', () => {
  const html = hexBuildOptionsHTML('farm', {
    valid_hexes_for_building: { farm: [] },
    hexes: [{ hex_q: 5, hex_r: 2, terrain: 'hills', building: { type: 'mine', level: 1 } }],
  });
  assert.equal(html, '');
});

test('HX4: a hex present in both lists is rendered once, as a build option (valid_hexes_for_building wins)', () => {
  const html = hexBuildOptionsHTML('farm', {
    valid_hexes_for_building: { farm: [{ q: 1, r: 1 }] },
    hexes: [{ hex_q: 1, hex_r: 1, terrain: 'plains' }],
  });
  assert.equal((html.match(/value="1,1"/g) || []).length, 1);
});

test('HX4b: server effect strings are escaped, and a missing effect leaves no dangling dash', () => {
  const html = hexBuildOptionsHTML('mine', {
    valid_hexes_for_building: { mine: [{ q: 1, r: 2, effect: '<b>x</b>' }, { q: 2, r: 2 }] },
    hexes: [{ hex_q: 1, hex_r: 2, terrain: 'hills' }, { hex_q: 2, hex_r: 2, terrain: 'hills' }],
  });
  assert.match(html, /&lt;b&gt;x&lt;\/b&gt;/);
  assert.match(html, />\(2,2\) Hills</);
});

test('HX5: no placement-options data yet renders no options, not an error', () => {
  assert.equal(hexBuildOptionsHTML('farm', null), '');
});

test('B1: the wall option shows the next rung, greyed with its reason until the Granary is high enough', async () => {
  const { wallOptionHTML } = await import('./city.js');
  const wall = { type: 'wall', wall_ladder: [
    { level: 1, name: 'Palisade', costs: { timber: 12, stone: 4 }, requires_silo: 1, population_cap: 5000 },
    { level: 2, name: 'Stone Wall', costs: { timber: 20, stone: 40 }, requires_silo: 2, population_cap: 10000 },
    { level: 3, name: 'Bronze Wall', costs: { stone: 80, bronze: 20 }, requires_silo: 3, population_cap: 20000 },
  ] };
  const locked = wallOptionHTML(wall, { walls: 1, siloLevel: 1 });
  assert.match(locked, /disabled/);
  assert.match(locked, /Stone Wall — needs a Granary of level 2/);
  const open = wallOptionHTML(wall, { walls: 1, siloLevel: 2 });
  assert.doesNotMatch(open, /disabled/);
  assert.match(open, /20 timber 40 stone/);
  assert.match(wallOptionHTML(wall, { walls: 3, siloLevel: 3 }), /highest level/);
});

test('ORDER17: standing shipyard quotes its next level instead of L1',()=>{
 const html=buildingOptionsHTML([{type:'shipyard',costs:{timber:24,stone:12}}],{buildQuotes:[{type:'shipyard',current_level:1,next_level:2,costs:{timber:48,stone:24,cedar:4}}]});
 assert.match(html,/L1.*L2/);assert.match(html,/4 cedar/);assert.match(html,/48 timber/);
});

test('ORDER17: each farm hex has its own next price and max does not offer L4',()=>{
 const quotes=[{type:'farm',new_site:true,current_level:0,next_level:1,costs:{timber:10}},{type:'farm',hex_q:1,hex_r:0,current_level:1,next_level:2,costs:{timber:20}},{type:'farm',hex_q:0,hex_r:1,current_level:3,next_level:null,at_max_level:true}];
 const html=hexBuildOptionsHTML('farm',{valid_hexes_for_building:{farm:[{q:-1,r:0}]},hexes:[{hex_q:1,hex_r:0,terrain:'plains',building:{type:'farm',level:1}},{hex_q:0,hex_r:1,terrain:'plains',building:{type:'farm',level:3}}]},quotes);
 assert.match(html,/L0 → L1: 10 timber/);assert.match(html,/L1 → L2: 20 timber/);assert.match(html,/value="0,1" disabled/);assert.doesNotMatch(html,/L4/);
});

test('ORDER17: selected build price is readable below the picker with server duration',()=>{
 const html=buildQuoteHTML({type:'shipyard',current_level:1,next_level:2,costs:{timber:48,stone:24,cedar:4},duration_game_days:24});
 assert.match(html,/L1 → L2/);assert.match(html,/4 cedar/);assert.match(html,/24 days/);assert.doesNotMatch(buildQuoteHTML({type:'farm',current_level:3,at_max_level:true}),/L4/);
});
