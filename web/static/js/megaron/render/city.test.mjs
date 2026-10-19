import test from 'node:test';
import assert from 'node:assert/strict';
import { buildCityScene, SCENE_CANVAS } from './city.js';
const buildings = [
  { type: 'temple', level: 1 },
  { type: 'farm', level: 2 },
  { type: 'harbour', level: 1 },
  { type: 'shipyard', level: 1 },
  { type: 'mine', level: 1 }
];
const scene = (population = 4500, extra = {}, bs = buildings, queue = []) =>
  buildCityScene({ terrain: 'hills' }, bs, queue, {
    population,
    walls: 1,
    food: { growth_state: 'growing' },
    ...extra
  });
const count = (a) => a.reduce((n, v) => n + Number(Boolean(v)), 0);
test('terraced housing grows geometrically without figures or smoke', () => {
  const scenes = [1000, 4500, 10000].map((p) => scene(p));
  assert.deepEqual(SCENE_CANVAS, { w: 320, h: 192 });
  assert.deepEqual(
    scenes.map((s) => s.terraceLevels),
    [1, 3, 5]
  );
  const mass = scenes.map((s) => count(s.housing));
  assert.ok(mass[0] < mass[1] && mass[1] < mass[2], `residential projection must grow: ${mass}`);
  assert.equal(new Set(scenes.map((s) => s.structure.join(','))).size, 3);
  for (const s of scenes) {
    assert.equal(s.structure.length, 160 * 96);
    assert.equal(s.structure[4 * 160 + 80], '#E3C6A0');
    assert.equal(s.structure[11 * 160 + 80], '#B59B89');
  }
});
test('all statuses stay in separate layers and can coexist', () => {
  const base = scene(),
    siege = scene(4500, { besieged: true }),
    hunger = scene(4500, { food: { growth_state: 'shrinking' } }),
    build = scene(4500, {}, buildings, [{ type: 'temple' }]);
  for (const s of [siege, hunger, build]) assert.deepEqual(s.structure, base.structure);
  assert.ok(count(siege.status) > 0);
  assert.ok(count(hunger.status) > 0);
  assert.ok(count(build.status) > 0);
  const combined = scene(4500, { besieged: true, food: { growth_state: 'shrinking' } }, buildings, [
    { type: 'temple' }
  ]);
  assert.equal(combined.starving, true);
  assert.equal(combined.besieged, true);
  assert.equal(combined.construction.length, 1);
  assert.ok(count(combined.status) > Math.max(count(siege.status), count(hunger.status)));
});
test('food truth comes from growth state, not low grain or negative balance', () => {
  assert.equal(
    scene(4500, { grain: 0, food: { growth_state: 'holding', food_net_per_tick: -100 } }).starving,
    false
  );
  assert.equal(
    scene(4500, { food: { growth_state: 'shrinking', coverage_ticks: 30 } }).starving,
    true
  );
});
test('wall silhouette and gate change independently of population', () => {
  const variants = [0, 1, 2, 3].map((walls) => scene(4500, { walls }));
  assert.equal(new Set(variants.map((s) => s.structure.join(','))).size, 4);
  assert.deepEqual(
    variants.map((s) => s.terraceLevels),
    [3, 3, 3, 3]
  );
});
test('no fabricated temple, harbour, mine or construction on an empty city', () => {
  const empty = scene(1000, {}, []);
  assert.deepEqual(Object.keys(empty.anchors).sort(), ['gate']);
  assert.deepEqual(empty.construction, []);
  for (const b of buildings) {
    const s = scene(1000, {}, [b]);
    assert.ok(s.anchors[b.type], b.type);
    assert.notDeepEqual(s.structure, empty.structure);
  }
  const queue = scene(1000, {}, [], [{ type: 'temple' }]);
  assert.ok(queue.construction[0]);
  assert.equal(queue.anchors.temple, undefined);
});
test('raster deterministic and all colours valid; order of building payload irrelevant', () => {
  const a = scene(),
    b = scene(4500, {}, [...buildings].reverse());
  assert.deepEqual(a, b);
  for (const layer of [a.structure, a.status, a.life])
    for (const px of layer) assert.ok(px === null || /^#[0-9a-f]{6}$/i.test(px), px);
});

test('every functional building is drawn from the actual inventory', () => {
  const empty = scene(4500, {}, []);
  for (const type of [
    'olive_press',
    'winery',
    'market',
    'lumbermill',
    'stonequarry',
    'silver_mine',
    'foundry',
    'barracks',
    'stable'
  ]) {
    const s = scene(4500, {}, [{ type, level: 3 }]);
    assert.ok(s.anchors[type], type);
    assert.notDeepEqual(s.structure, empty.structure, type);
  }
});

test('missing settlement payload never fabricates population, buildings or life', () => {
  const s = buildCityScene(null, [], [], null);
  assert.equal(s.terraceLevels, 0);
  assert.equal(count(s.housing), 0);
  assert.equal(count(s.life), 0);
  assert.deepEqual(s.anchors, {});
});

test('siege of an unfortified city never invents a closed wall gate', () => {
  const s = scene(4500, { walls: 0, besieged: true });
  assert.ok(count(s.status) > 0);
  assert.equal(s.status[62 * 160 + 80], null);
});
