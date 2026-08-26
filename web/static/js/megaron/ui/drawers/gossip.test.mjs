import test from 'node:test';
import assert from 'node:assert/strict';

// gossip.js's whole import graph (state.js, api.js, ui/format.js) is
// side-effect-free at module scope — no document/window touched until a
// function actually runs — so, unlike economy.js (ui/misc.js's pointerdown
// listener at top level, see cargo.test.mjs), a plain static import is safe
// here. Same pure-render-function convention as cargo.test.mjs
// (formatCargoRows/renderCargoHTML) and economy.test.mjs (goodsRateCell):
// exercise the pure renderers directly rather than stubbing fetch to drive
// loadGossipDrawer end-to-end.
import {
  hopsLabel, renderGossipRowsHTML, groupWanaxes, renderWanaxesHTML, loadGossipDrawer,
} from './gossip.js';

const GOSSIP_ROW = {
  id: 'g1',
  source_region: 'Argolid',
  category: 'harvest',
  text: 'Mycenae\'s granaries overflow this season.',
  generated_at: '2026-08-16T10:00:00.000Z',
  importance: 'minor',
  hops: 2,
};

test('hopsLabel: 0 or missing hops → no qualifier (matches keryx cmd_gossip.go hopLabel)', () => {
  assert.equal(hopsLabel(0), '');
  assert.equal(hopsLabel(undefined), '');
  assert.equal(hopsLabel(null), '');
});

test('hopsLabel: 1 hop is singular, N hops is plural', () => {
  assert.equal(hopsLabel(1), '1 hop away');
  assert.equal(hopsLabel(2), '2 hops away');
  assert.equal(hopsLabel(5), '5 hops away');
});

test('renderGossipRowsHTML: empty list renders the empty-state, not a blank list', () => {
  const html = renderGossipRowsHTML([]);
  assert.match(html, /empty-state/);
  assert.match(html, /No rumours/);
  assert.doesNotMatch(html, /inbox-item/);
});

test('renderGossipRowsHTML: null/undefined also renders the empty-state, crashes nothing', () => {
  assert.match(renderGossipRowsHTML(null), /empty-state/);
  assert.match(renderGossipRowsHTML(undefined), /empty-state/);
});

test('renderGossipRowsHTML: a row renders region, category, hops and text', () => {
  const html = renderGossipRowsHTML([GOSSIP_ROW]);
  assert.match(html, /Argolid/);
  assert.match(html, /harvest/);
  assert.match(html, /2 hops away/);
  assert.match(html, /Mycenae/);
  assert.doesNotMatch(html, /gossip-major/, 'importance=minor must not get the major-highlight class');
});

test('renderGossipRowsHTML: importance=major gets the highlight class', () => {
  const html = renderGossipRowsHTML([{ ...GOSSIP_ROW, importance: 'major' }]);
  assert.match(html, /gossip-major/);
});

test('renderGossipRowsHTML: escapes rumour text so a hostile string cannot inject markup', () => {
  const html = renderGossipRowsHTML([{ ...GOSSIP_ROW, text: '<img src=x onerror=alert(1)>' }]);
  assert.doesNotMatch(html, /<img/);
});

const WANAX_ROWS = [
  { settlement_id: 's1', name: 'Mycenae', owner: 'Agamemnon', kingdom: 'Achaea', own: true },
  { settlement_id: 's2', name: 'Tiryns', owner: 'Agamemnon', kingdom: 'Achaea', own: false },
  { settlement_id: 's3', name: 'Pylos', owner: 'Nestor', kingdom: '', own: false },
];

test('groupWanaxes: aggregates the per-settlement /wanaxes rows by owner', () => {
  const groups = groupWanaxes(WANAX_ROWS);
  assert.equal(groups.length, 2);
  const agamemnon = groups.find(g => g.owner === 'Agamemnon');
  assert.deepEqual(agamemnon.settlements, ['Mycenae', 'Tiryns']);
  assert.equal(agamemnon.kingdom, 'Achaea');
  assert.equal(agamemnon.own, true, 'own=true on any one settlement marks the whole wanax as own');
});

test('groupWanaxes: no kingdom stays empty, not "undefined"', () => {
  const nestor = groupWanaxes(WANAX_ROWS).find(g => g.owner === 'Nestor');
  assert.equal(nestor.kingdom, '');
});

test('groupWanaxes: empty/null input yields no groups', () => {
  assert.deepEqual(groupWanaxes([]), []);
  assert.deepEqual(groupWanaxes(null), []);
});

test('renderWanaxesHTML: empty list renders the empty-state, not a blank table', () => {
  const html = renderWanaxesHTML([]);
  assert.match(html, /empty-state/);
  assert.match(html, /No wanaxes known/);
  assert.doesNotMatch(html, /<table/);
});

test('renderWanaxesHTML: a known wanax renders name, own-star, kingdom and cities', () => {
  const html = renderWanaxesHTML(WANAX_ROWS);
  assert.match(html, /Agamemnon/);
  assert.match(html, /★/);
  assert.match(html, /Achaea/);
  assert.match(html, /Mycenae, Tiryns/);
});

test('renderWanaxesHTML: escapes a hostile owner/name so markup cannot be injected', () => {
  const html = renderWanaxesHTML([{ name: '<img src=x onerror=alert(1)>', owner: '<script>', kingdom: '' }]);
  assert.doesNotMatch(html, /<img/);
  assert.doesNotMatch(html, /<script>/);
});

test('AK: gossip.js imports under node --test without a DOM (proves the module graph is side-effect-free)', () => {
  assert.equal(typeof loadGossipDrawer, 'function');
});

test('loadGossipDrawer: missing #gossip-body returns early without touching anything else', async () => {
  const calls = [];
  globalThis.document = { getElementById(id) { calls.push(id); return null; } };
  await loadGossipDrawer();
  assert.deepEqual(calls, ['gossip-body']);
  delete globalThis.document;
});
