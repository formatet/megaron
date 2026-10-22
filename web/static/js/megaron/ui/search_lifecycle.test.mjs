import test from 'node:test';
import assert from 'node:assert/strict';

const elements = new Map();
function element() {
  const classes = new Set();
  return {
    classList: { add: c => classes.add(c), remove: c => classes.delete(c),
      contains: c => classes.has(c), toggle(c) { if (classes.has(c)) classes.delete(c); else classes.add(c); } },
    style: {}, value: '', innerHTML: '', addEventListener() {}, focus() {},
    querySelectorAll: () => [], getContext: () => ({}),
  };
}
globalThis.ResizeObserver = class { observe() {} };
globalThis.document = { body: element(), addEventListener() {},
  getElementById(id) { if (!elements.has(id)) elements.set(id, element()); return elements.get(id); },
  createElement: element, querySelectorAll: () => [], querySelector: () => null };
globalThis.window = { addEventListener() {}, matchMedia: () => ({ matches: true }) };
globalThis.localStorage = { getItem: () => null };
globalThis.fetch = async () => ({ ok: true, json: async () => [] });
const { toggleSearch } = await import('./search.js');
const { State } = await import('../state.js');
State.provinceData = []; State.unitsData = []; State.marchData = [];

test('mobile search toggle closes through cleanup and restores map input', () => {
  const search = document.getElementById('search-overlay');
  const canvas = document.getElementById('hex-canvas');
  const map = document.getElementById('map-root');
  toggleSearch();
  assert.equal(search.classList.contains('open'), true);
  assert.equal(canvas.inert, true);
  assert.equal(map.classList.contains('mobile-panel-blocked'), true);
  toggleSearch();
  assert.equal(search.classList.contains('open'), false);
  assert.equal(canvas.inert, false);
  assert.equal(map.classList.contains('mobile-panel-blocked'), false);
});
