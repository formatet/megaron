import test from 'node:test';
import assert from 'node:assert/strict';

const noopEl = new Proxy({}, { get: (_t, k) => (k === 'style' ? {} : (k === 'value' ? '' : () => noopEl)), set: () => true });
globalThis.ResizeObserver ??= class { observe() {} };
globalThis.document ??= { addEventListener() {}, getElementById: () => noopEl, createElement: () => noopEl, querySelector: () => noopEl, querySelectorAll: () => [], body: noopEl };
globalThis.window ??= { addEventListener() {}, matchMedia: () => ({ matches: false, addEventListener() {} }) };
globalThis.localStorage ??= { getItem: () => null, setItem() {}, removeItem() {} };

const { createPicker, barHTML, sheetHTML } = await import('./hexpick.js');
const { chooseMarchTarget } = await import('./marchctx.js');
const { State } = await import('../state.js');
const { hexAtScreen, hexClientPos } = await import('../render/map.js');

function fakeEnv({ cities = [], tiles = { '3,4': { terrain: 'plains' } }, answer = { q: 3, r: 4 } } = {}) {
  const calls = { opened: [], picks: [], shortcuts: null, domTouched: false };
  return {
    calls,
    env: {
      pickHex(opts) { calls.picks.push(opts); return Promise.resolve(answer); },
      setPickShortcuts(list) { calls.shortcuts = list; },
      fetchCities: async () => cities,
      tileAt: (q, r) => tiles[q + ',' + r],
      destFromHex: (h, tile) => ({ q: h.q, r: h.r, terrain: tile.terrain }),
      hexClientPos: () => ({ x: 900, y: 300 }),
      openMarchCtx(dest, x, y) { calls.opened.push({ dest, x, y, unit: State.marchCtxUnitID }); },
    },
  };
}

test('March starts a pick at once, names the unit, and opens no menu while picking', async () => {
  State.unitsData = [{ id: 'u1', display_name: '2nd Spearmen of Phaistos' }];
  let release; const { calls, env } = fakeEnv();
  env.pickHex = opts => { calls.picks.push(opts); return new Promise(r => { release = r; }); };
  const done = chooseMarchTarget('u1', 'March', env);
  await Promise.resolve();
  assert.equal(calls.picks.length, 1);
  assert.equal(calls.picks[0].prompt, 'March 2nd Spearmen of Phaistos: tap the destination');
  assert.equal(calls.opened.length, 0, 'no menu while picking');
  assert.equal(State.marchCtxUnitID ?? null, null, 'the unit is pinned only once a hex is chosen');
  release(null); await done;
});

test('Redirect names the verb', async () => {
  State.unitsData = [{ id: 'u1', display_name: 'Chariots' }];
  const { calls, env } = fakeEnv({ answer: null });
  await chooseMarchTarget('u1', 'Redirect', env);
  assert.equal(calls.picks[0].prompt, 'Redirect Chariots: tap the destination');
});

test('confirm opens the real menu for that hex with the unit preselected, away from the hex', async () => {
  State.unitsData = [{ id: 'u1', display_name: 'Chariots' }];
  const { calls, env } = fakeEnv();
  await chooseMarchTarget('u1', 'March', env);
  assert.equal(calls.opened.length, 1);
  const o = calls.opened[0];
  assert.deepEqual(o.dest, { q: 3, r: 4, terrain: 'plains' });
  assert.equal(o.unit, 'u1', 'unit pinned when the menu opens');
  assert.ok(o.x + 8 + 210 <= 900 - 20 || o.x >= 900 + 20, 'menu does not sit on the chosen hex: x=' + o.x);
});

test('cancel opens nothing and leaves no armed unit', async () => {
  const { calls, env } = fakeEnv({ answer: null });
  State.marchCtxUnitID = 'stale';
  await chooseMarchTarget('u1', 'March', env);
  assert.equal(calls.opened.length, 0);
  assert.equal(State.marchCtxUnitID, null);
});

test('accept refuses only hexes off the map', async () => {
  const { calls, env } = fakeEnv();
  await chooseMarchTarget('u1', 'March', env);
  const accept = calls.picks[0].accept;
  assert.equal(accept({ q: 3, r: 4 }), null);
  assert.match(accept({ q: 99, r: 99 }), /not on the map/i);
});

test('known cities become shortcuts (name — ruler, or your city); unknown ones are dropped', async () => {
  const cities = [
    { settlement_id: 's1', name: 'Phaistos', own: true, knowledge: 'known', q: 1, r: 2 },
    { settlement_id: 's2', name: 'Knossos', owner: 'Minos', knowledge: 'known', q: -2, r: 0 },
    { settlement_id: 's3', name: 'Rumour', knowledge: 'rumour', q: 5, r: 5 },
  ];
  const { calls, env } = fakeEnv({ cities });
  await chooseMarchTarget('u1', 'March', env);
  assert.deepEqual(calls.shortcuts, [
    { label: 'Phaistos — your city', q: 1, r: 2 },
    { label: 'Knossos — Minos', q: -2, r: 0 },
  ]);
});

test('no known cities: no shortcuts handed over', async () => {
  const { calls, env } = fakeEnv({ cities: [] });
  await chooseMarchTarget('u1', 'March', env);
  assert.equal(calls.shortcuts, null);
});

// ── the picker ──
function rig() {
  const env = { hide: () => 1, restore() {}, render(v) { rig.last = v; }, describe: () => ({}) };
  return createPicker(env);
}

test('a shortcut confirms its hex at once', async () => {
  const p = rig();
  const done = p.pick({ prompt: 'x' });
  p.setShortcuts([{ label: 'A', q: 1, r: 2 }, { label: 'B', q: -2, r: 0 }]);
  assert.equal(rig.last.shortcuts.length, 2);
  p.shortcut(1);
  assert.deepEqual(await done, { q: -2, r: 0 });
  assert.equal(p.active(), false);
});

test('a shortcut the accept rule refuses selects the hex and shows the reason', async () => {
  const p = rig();
  const done = p.pick({ prompt: 'x', accept: () => 'No.' });
  p.setShortcuts([{ label: 'A', q: 1, r: 2 }]);
  p.shortcut(0);
  assert.equal(p.active(), true);
  assert.equal(rig.last.reason, 'No.');
  p.cancel(); assert.equal(await done, null);
});

test('the bar shows Cities only when shortcuts exist; the sheet lists them as buttons', () => {
  assert.doesNotMatch(barHTML({ prompt: 'p', sel: null, reason: null, shortcuts: [] }), /Cities/);
  assert.match(barHTML({ prompt: 'p', sel: null, reason: null, shortcuts: [{ label: 'A', q: 1, r: 2 }] }), /data-pick-cities[^>]*>Cities/);
  const html = sheetHTML({ shortcuts: [{ label: 'Phaistos — your city', q: 1, r: 2 }] });
  assert.match(html, /data-pick-shortcut="0"[^>]*>Phaistos — your city/);
});

test('hexClientPos is the inverse of hexAtScreen under any camera', () => {
  const cam = { x: 120, y: -40, zoom: 1.5 };
  const rect = { left: 10, top: 20 };
  for (const [q, r] of [[0, 0], [5, -3], [-7, 9]]) {
    const { x, y } = hexClientPos(q, r, cam, rect);
    assert.deepEqual(hexAtScreen(x - rect.left, y - rect.top, cam), { q, r });
  }
});

test('the old armed empty menu strings are gone', async () => {
  const fs = await import('node:fs');
  const src = fs.readFileSync(new URL('./marchctx.js', import.meta.url), 'utf8');
  assert.doesNotMatch(src, /mctx-hint'\)\.textContent = 'Choose a destination/);
});

test('chooseMarchTarget answers null when the pick is cancelled and the hex when it opens the menu', async () => {
  State.unitsData = [{ id: 'u1', display_name: 'Chariots' }];
  assert.equal(await chooseMarchTarget('u1', 'March', fakeEnv({ answer: null }).env), null);
  assert.deepEqual(await chooseMarchTarget('u1', 'March', fakeEnv().env), { q: 3, r: 4 });
});
