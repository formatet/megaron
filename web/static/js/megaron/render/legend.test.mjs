import test from 'node:test';
import assert from 'node:assert/strict';

const noopEl = new Proxy({}, {
  get: (_t, k) => (k === 'style' ? {} : (k === 'value' ? '' : () => noopEl)),
  set: () => true,
});
globalThis.ResizeObserver ??= class { observe() {} };
globalThis.document ??= {
  addEventListener() {}, getElementById: () => noopEl, createElement: () => noopEl,
  querySelector: () => noopEl, querySelectorAll: () => [], body: noopEl,
};
globalThis.window ??= {
  addEventListener() {}, matchMedia: () => ({ matches: false, addEventListener() {} }),
  innerWidth: 800, innerHeight: 600,
};
globalThis.localStorage ??= { getItem: () => null, setItem() {}, removeItem() {} };

const { LEGEND_ENTRIES } = await import('./legend.js');
const { DEPOSIT_KINDS } = await import('./map.js');

test('legend has one deposit row per kind in the renderer table', () => {
  for (const k of DEPOSIT_KINDS) {
    const e = LEGEND_ENTRIES.find(x => x.id === 'deposit_' + k.key);
    assert.ok(e, k.label);
    assert.equal(e.label, k.label);
  }
});

test('legend names the things the first-hour tester could not read', () => {
  const labels = LEGEND_ENTRIES.map(e => e.label).join('|');
  for (const w of ['Copper', 'Silver', 'Tin', 'Your city', 'Host']) assert.ok(labels.includes(w), w);
});

test('every legend entry has a unique id and draws something', () => {
  const ids = new Set();
  for (const e of LEGEND_ENTRIES) {
    assert.ok(!ids.has(e.id)); ids.add(e.id);
    let calls = 0;
    const ctx = new Proxy({}, {
      get: (_t, k) => (k === 'canvas' ? {} : () => { calls++; }),
      set: () => true,
    });
    e.draw(ctx);
    assert.ok(calls > 0, e.id + ' drew nothing');
  }
});
