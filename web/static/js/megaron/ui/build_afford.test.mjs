import test from 'node:test';
import assert from 'node:assert/strict';
import { shortfalls, shortfallText, stockFromGoods, buildRows, constructListHTML, hexBuildRowHTML, hexRowsHTML, buildRowSelector, showBuildResult } from './build_rows.js';
import { warehouseHTML } from './drawers/city_warehouse.js';

const farm = { type: 'farm', hex_bound: true, costs: { timber: 6, stone: 4 }, duration_game_days: 4 };
const silo = { type: 'silo', costs: { timber: 6, stone: 4 }, duration_game_days: 6 };

test('BA1: shortfalls compares cost with stock, silver included, spare cedar covers timber', () => {
  assert.deepEqual(shortfalls({ timber: 6, stone: 4 }, 0, { timber: 2, stone: 4 }), [{ good: 'timber', need: 6, have: 2 }]);
  assert.deepEqual(shortfalls({ timber: 6 }, 0, { timber: 2, cedar: 5 }), []);
  assert.deepEqual(shortfalls({ timber: 6 }, 3, { timber: 6 }), [{ good: 'silver', need: 3, have: 0 }]);
  assert.deepEqual(shortfalls({ timber: 6 }, 0, { timber: 5.9999999 }), [], 'server epsilon');
  assert.equal(shortfallText([{ good: 'timber', need: 6, have: 2 }]), 'needs 6 timber, city has 2');
  assert.equal(shortfallText([{ good: 'stone', need: 4, have: 0 }, { good: 'timber', need: 6, have: 2 }]), 'needs 4 stone, city has 0; needs 6 timber, city has 2');
  assert.deepEqual(stockFromGoods([{ key: 'timber', amount: 2.4 }]), { timber: 2.4 });
  assert.equal(stockFromGoods(null), null);
});

test('BA2: a Construct row the city cannot pay has a disabled Build and a btn-reason; a payable one is unchanged', () => {
  const ctx = stock => ({ stock, buildQuotes: [{ type: 'silo', current_level: 0, next_level: 1, costs: { timber: 6, stone: 4 }, duration_game_days: 6 }] });
  const poor = constructListHTML([silo], ctx({ timber: 2, stone: 0 }));
  assert.match(poor, /<button[^>]*build-row-btn[^>]*data-build-act="build"[^>]* disabled/);
  assert.match(poor, /<span class="btn-reason">needs 4 stone, city has 0; needs 6 timber, city has 2<\/span>/);
  const rich = constructListHTML([silo], ctx({ timber: 6, stone: 4 }));
  assert.doesNotMatch(rich, /disabled|btn-reason/);
  const unknown = constructListHTML([silo], ctx(null));
  assert.doesNotMatch(unknown, /disabled|btn-reason/, 'no stock loaded: no claim');
  assert.equal(buildRows([silo], ctx({ timber: 2, stone: 0 }))[0].payable, false);
});

test('BA3: hex-bound Construct row is disabled like Build when short, and tells the shortage (round two item D)', () => {
  const html = constructListHTML([farm], { stock: { timber: 2, stone: 0 }, buildQuotes: [{ type: 'farm', new_site: true, current_level: 0, next_level: 1, costs: { timber: 6, stone: 4 }, duration_game_days: 4 }] });
  assert.match(html, /data-build-act="pick"[^>]* disabled/);
  const rich = constructListHTML([farm], { stock: { timber: 6, stone: 4 }, buildQuotes: [{ type: 'farm', new_site: true, current_level: 0, next_level: 1, costs: { timber: 6, stone: 4 }, duration_game_days: 4 }] });
  assert.doesNotMatch(rich, /disabled/);
  assert.match(html, /btn-reason">needs 4 stone, city has 0; needs 6 timber, city has 2/);
});

test('BA4: hex Build here rows: disabled + reason when short, plain when payable', () => {
  const c = { q: 1, r: 2, effect: '', upgrade: false, quote: { costs: { timber: 6 }, duration_game_days: 4, current_level: 0, next_level: 1 } };
  const poor = hexBuildRowHTML('farm', c, { timber: 2 });
  assert.match(poor, /data-build-act="build-hex"[^>]* disabled/); assert.match(poor, /btn-reason">needs 6 timber, city has 2/);
  assert.doesNotMatch(hexBuildRowHTML('farm', c, { timber: 6 }), /disabled|btn-reason/);
  assert.doesNotMatch(hexBuildRowHTML('farm', c), /disabled|btn-reason/);
  const opts = { build_quotes: [{ type: 'farm', new_site: true, costs: { timber: 6 }, duration_game_days: 4, current_level: 0, next_level: 1 }], valid_hexes_for_building: { farm: [{ q: 1, r: 2 }] }, hexes: [] };
  assert.match(hexRowsHTML([farm], opts, 1, 2, undefined, { timber: 0 }), /btn-reason/);
});

// A tiny DOM: enough for insert-after, class, text, scrollIntoView.
function fakeDoc() {
  const mk = tag => { const e = { tag, children: [], className: '', textContent: '', attrs: {}, scrolled: null, parent: null,
    setAttribute(k, v) { this.attrs[k] = v; }, scrollIntoView(o) { this.scrolled = o; },
    after(n) { const i = this.parent.children.indexOf(this); n.parent = this.parent; this.parent.children.splice(i + 1, 0, n); },
    remove() { this.parent.children.splice(this.parent.children.indexOf(this), 1); },
    get nextElementSibling() { return this.parent.children[this.parent.children.indexOf(this) + 1] || null; },
    get ownerDocument() { return doc; } }; return e; };
  const doc = { createElement: mk };
  const root = mk('div'); const row = mk('div'); row.className = 'build-row'; row.parent = root; root.children.push(row);
  root.querySelector = sel => (sel === 'ROW' ? row : null);
  return { doc, root, row };
}

test('BA5: buildRowSelector addresses the clicked row by type, god and hex', () => {
  assert.equal(buildRowSelector({ type: 'silo' }), '.build-row[data-build-type="silo"]:not([data-build-q])');
  assert.equal(buildRowSelector({ type: 'temple', god: 'potnia' }), '.build-row[data-build-type="temple"][data-build-god="potnia"]:not([data-build-q])');
  assert.equal(buildRowSelector({ type: 'farm', hex_q: 1, hex_r: -2 }), '.build-row[data-build-type="farm"][data-build-q="1"][data-build-r="-2"]');
});

test('BA6: showBuildResult puts the answer right under the row, replaces a stale one, scrolls it into view', () => {
  const { root, row } = fakeDoc();
  root.querySelector = () => row;
  const el = showBuildResult(root, { type: 'farm' }, 'Needs timber.', false);
  assert.equal(row.nextElementSibling, el);
  assert.match(el.className, /build-row-result/); assert.doesNotMatch(el.className, /ok/);
  assert.equal(el.textContent, 'Needs timber.'); assert.equal(el.attrs.role, 'status');
  assert.ok(el.scrolled, 'scrolled into view'); assert.equal(el.scrolled.block, 'nearest');
  const el2 = showBuildResult(root, { type: 'farm' }, 'Farm queued.', true);
  assert.equal(root.children.length, 2, 'one result per row, stale removed');
  assert.match(el2.className, /build-row-ok/); assert.equal(row.nextElementSibling, el2);
  assert.equal(showBuildResult({ querySelector: () => null }, { type: 'x' }, 't', false), null, 'row gone: caller falls back');
});

test('BA7: Warehouse lists stone and timber even at 0, other empty goods stay hidden', () => {
  const html = warehouseHTML({}, [{ key: 'stone', name: 'Stone', amount: 0, cap: 0 }, { key: 'wine', name: 'Wine', amount: 0, cap: 0 }]);
  const goodsPart = html.split('dsec-title">Goods')[1];
  assert.match(goodsPart, /Stone<\/span><span class="sr-val">0</);
  assert.match(goodsPart, /Timber<\/span><span class="sr-val">0</, 'missing row synthesised');
  assert.doesNotMatch(goodsPart, /Wine/);
});
