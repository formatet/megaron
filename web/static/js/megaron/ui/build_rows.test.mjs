import test from 'node:test';
import assert from 'node:assert/strict';
import { buildRows, constructListHTML, buildBody, hexBuildChoices, pickHexes, hexRowsHTML, hexBuildRowHTML, fmtBuildCost } from './build_rows.js';

const names = html => [...html.matchAll(/<b>([^<]+)<\/b>/g)].map(m => m[1]);

test('BR1: Construct markup has no <select; hex-bound row says Choose a hex, city row says Build', () => {
  const html = constructListHTML([
    { type: 'farm', hex_bound: true, costs: { timber: 6, stone: 4 }, duration_game_days: 4 },
    { type: 'silo', costs: { timber: 4, stone: 8 }, duration_game_days: 6 },
  ], { buildQuotes: [
    { type: 'farm', new_site: true, current_level: 0, next_level: 1, costs: { timber: 6, stone: 4 }, duration_game_days: 4 },
    { type: 'silo', current_level: 1, next_level: 2, costs: { timber: 4, stone: 8 }, duration_game_days: 6 }] });
  assert.doesNotMatch(html, /<select/);
  assert.match(html, /<b>Farm<\/b> L0 → L1 · 4 stone 6 timber · 4 game days/);
  assert.match(html, /data-build-act="pick" data-build-type="farm"/);
  assert.match(html, /<b>Granary<\/b> L1 → L2 · 8 stone 4 timber · 6 game days/);
  assert.match(html, /data-build-act="build" data-build-type="silo"/);
});

test('BR2: costs come from the catalogue, rounded like the CLI, silver included', () => {
  assert.equal(fmtBuildCost({ costs: { timber: 1.429, stone: 28.571 }, cost_silver: 3 }), '3 silver 29 stone 1 timber');
  const rows = buildRows([{ type: 'mine', costs: { timber: 1.429, stone: 28.571 } }]);
  assert.equal(rows[0].cost, '29 stone 1 timber');
});

test('BR3: a locked row has ONE visible reason and no button (max level, price unavailable, queue, goods, wall)', () => {
  const cat = [{ type: 'silo', costs: { timber: 1 } }, { type: 'stable', costs: { timber: 1 } }, { type: 'market', costs: { timber: 1 } }];
  const html = constructListHTML(cat, { buildQuotes: [
    { type: 'silo', current_level: 3, at_max_level: true },
    { type: 'stable', current_level: 0, next_level: 1, costs: { timber: 9 }, duration_game_days: 2 }],
    canAfford: [{ type: 'stable', can_afford: false }] });
  const row = t => html.match(new RegExp(`<div class="build-row[^"]*" data-build-type="${t}">[\\s\\S]*?</div></div>`))[0];
  assert.match(row('silo'), /locked/); assert.match(row('silo'), /build-row-reason">maximum level/); assert.doesNotMatch(row('silo'), /<button/);
  assert.match(row('stable'), /not enough goods in store/); assert.doesNotMatch(row('stable'), /<button/);
  assert.match(row('market'), /price unavailable/); assert.doesNotMatch(row('market'), /<button/);
  const full = constructListHTML([{ type: 'silo', costs: { timber: 1 } }], { queueFull: true });
  assert.match(full, /the build queue is full/); assert.doesNotMatch(full, /<button/);
  const wall = { type: 'wall', wall_ladder: [
    { level: 1, name: 'Palisade', costs: { timber: 12, stone: 4 }, requires_silo: 1, population_cap: 5000 },
    { level: 2, name: 'Stone Wall', costs: { timber: 20, stone: 40 }, requires_silo: 2, population_cap: 10000 }] };
  const w = constructListHTML([wall], { walls: 1, siloLevel: 1 });
  assert.match(w, /needs a Granary of level 2/); assert.doesNotMatch(w, /<button/);
  const open = constructListHTML([wall], { walls: 1, siloLevel: 2 });
  assert.match(open, /40 stone 20 timber/); assert.match(open, /<button/);
  assert.match(constructListHTML([{ ...wall, wall_ladder: [] }], { walls: 3, siloLevel: 3 }), /at the highest level/);
});

test('BR4: catalogue failure is one line and no button', () => {
  const html = constructListHTML(null, {});
  assert.equal(html, '<p class="empty-state">Could not load the building list. Reopen the city to retry.</p>');
});

test('BR5: temples are one row per god with their own quote; list is alphabetical by the name read', () => {
  const html = constructListHTML([{ type: 'temple', costs: { timber: 5 } }, { type: 'silo', costs: { timber: 1 } }, { type: 'farm', hex_bound: true, costs: { timber: 1 } }, { type: 'barracks', costs: { timber: 1 } }], { buildQuotes: [
    { type: 'temple', god_id: 'potnia', current_level: 1, next_level: 2, costs: { timber: 10 } },
    { type: 'temple', god_id: 'velchanos', current_level: 0, next_level: 1, costs: { timber: 5 } }] });
  assert.match(html, /<b>Temple of Potnia<\/b> L1 → L2 · 10 timber/);
  assert.match(html, /data-build-act="build" data-build-type="temple" data-build-god="velchanos"/);
  assert.match(html, /Temple of Asterion<\/b>[\s\S]*?price unavailable/);
  assert.match(html, /the Sea Daimons of Stillness/);
  const n = names(html); assert.deepEqual(n, [...n].sort((a, b) => a.localeCompare(b, 'en')));
});

test('BR6: the three POST bodies are unchanged byte for byte', () => {
  assert.equal(JSON.stringify(buildBody({ type: 'silo' })), '{"building_type":"silo"}');
  assert.equal(JSON.stringify(buildBody({ type: 'farm', hex_q: 3, hex_r: -1 })), '{"building_type":"farm","hex_q":3,"hex_r":-1}');
  assert.equal(JSON.stringify(buildBody({ type: 'temple', god: 'velchanos' })), '{"building_type":"temple","god":"velchanos"}');
});

const quotes = [{ type: 'farm', new_site: true, current_level: 0, next_level: 1, costs: { timber: 10, stone: 4 }, duration_game_days: 4 },
  { type: 'farm', hex_q: 1, hex_r: 0, current_level: 1, next_level: 2, costs: { timber: 20 }, duration_game_days: 6 },
  { type: 'farm', hex_q: 0, hex_r: 1, current_level: 3, next_level: null, at_max_level: true }];
const opts = { build_quotes: quotes, valid_hexes_for_building: { farm: [{ q: -1, r: 0, effect: 'grain ×1.7' }], mine: [{ q: 2, r: 2 }] },
  hexes: [{ hex_q: -1, hex_r: 0, terrain: 'plains' }, { hex_q: 1, hex_r: 0, terrain: 'plains', building: { type: 'farm', level: 1 } },
    { hex_q: 0, hex_r: 1, terrain: 'plains', building: { type: 'farm', level: 3 } }, { hex_q: 3, hex_r: 3, terrain: 'sea' }, { hex_q: 2, hex_r: 2, terrain: 'hills' }] };

test('HX1: pick mode marks exactly the eligible hexes with the level step; max level and others are not marked', () => {
  const m = pickHexes('farm', opts);
  assert.deepEqual([...m.entries()].sort(), [['-1,0', 'L0 → L1'], ['1,0', 'L1 → L2']]);
  assert.equal(pickHexes('farm', null).size, 0);
});

test('HX2: choices merge valid and same-type hexes once, a different-type building is not offered', () => {
  const c = hexBuildChoices('farm', { valid_hexes_for_building: { farm: [{ q: 1, r: 1 }] }, hexes: [{ hex_q: 1, hex_r: 1, terrain: 'plains' }, { hex_q: 5, hex_r: 2, building: { type: 'mine', level: 1 } }] }, []);
  assert.equal(c.length, 1);
  assert.equal(hexBuildChoices('farm', null, []).length, 0);
});

test('HX3: "Build here" rows for a hex come from the same data (price, step, effect), escaped; none when nothing fits', () => {
  const cat = [{ type: 'farm', hex_bound: true }, { type: 'mine', hex_bound: true }, { type: 'silo' }];
  const free = hexRowsHTML(cat, opts, -1, 0);
  assert.match(free, /<b>Farm<\/b> L0 → L1 · 4 stone 10 timber · 4 game days/);
  assert.match(free, /grain ×1\.7/);
  assert.match(free, /data-build-act="build-hex" data-build-type="farm" data-build-q="-1" data-build-r="0"/);
  assert.doesNotMatch(free, /Mine/);
  assert.match(hexRowsHTML(cat, opts, 1, 0), /L1 → L2 · 20 timber/);
  assert.equal(hexRowsHTML(cat, opts, 0, 1), '');   // at maximum level
  assert.equal(hexRowsHTML(cat, opts, 3, 3), '');   // nothing fits
  assert.match(hexBuildRowHTML('farm', { q: 1, r: 1, effect: '<b>x</b>', quote: quotes[0] }), /&lt;b&gt;x&lt;\/b&gt;/);
  assert.match(hexRowsHTML(cat, opts, 2, 2), /Mine<\/b> L0 → L1/);
});
