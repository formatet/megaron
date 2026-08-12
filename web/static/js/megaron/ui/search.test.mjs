import test from 'node:test';
import assert from 'node:assert/strict';

// search.js registers keydown/input listeners on getElementById('search-input')
// at module top level and imports render/map.js (DOM-at-import) — so, same trick
// as marchctx.test.mjs, stub the globals its import graph reaches THEN import
// dynamically. enterTargetIndex is the pure half of the Enter-to-go fix
// (Timothy: "⌕ → skriv → Enter gör ingenting; man måste klicka").
const noopEl = new Proxy({}, {
  get: (_t, k) => (k === 'style' ? {} : (k === 'value' ? '' : () => noopEl)),
  set: () => true,
});
globalThis.document ??= {
  addEventListener() {},
  getElementById: () => noopEl,
  createElement: () => noopEl,
  querySelector: () => noopEl,
  querySelectorAll: () => [],
  body: noopEl,
};
globalThis.window ??= { addEventListener() {}, matchMedia: () => ({ matches: false, addEventListener() {} }) };
globalThis.localStorage ??= { getItem: () => null, setItem() {}, removeItem() {} };

const { enterTargetIndex } = await import('./search.js');

test('Enter with nothing selected (idx -1) acts on the first hit', () => {
  assert.equal(enterTargetIndex(-1, 3), 0);
});

test('Enter with an arrow-selected row acts on that row', () => {
  assert.equal(enterTargetIndex(2, 5), 2);
});

test('Enter with no results acts on nothing', () => {
  assert.equal(enterTargetIndex(-1, 0), -1);
  assert.equal(enterTargetIndex(0, 0), -1);
});

test('a stale selection past the end falls back to the first hit', () => {
  // e.g. the list shrank after typing but searchFocusIdx was left high.
  assert.equal(enterTargetIndex(7, 3), 0);
});
