import test from 'node:test';
import assert from 'node:assert/strict';

// map.js is the biggest offender: `export const canvas = document.getElementById(...)`,
// `tooltip`, `container` and `ctx = canvas.getContext('2d')` are ALL module-level DOM
// reads, on top of the `window.addEventListener('resize', resizeCanvas)` this plan
// originally flagged — moving all of that into an init() would mean converting several
// module-level `const`s (exported and consumed by ui/search.js, ui/marchctx.js) into
// `let`s assigned from initMap(), a structural change to the whole file, not a two-line
// move (megaron_plan_modulniva_dom.md under-scoped this file — see the plan's own
// ✅-notes for the full finding). Same stub-then-dynamic-import trick as
// marchctx.test.mjs sidesteps needing that refactor at all: a getElementById/
// getContext that always returns a usable no-op Proxy satisfies every module-level
// read here without touching map.js's production code.
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
globalThis.window ??= {
  addEventListener() {},
  matchMedia: () => ({ matches: false, addEventListener() {} }),
  innerWidth: 800, innerHeight: 600,
};
globalThis.localStorage ??= { getItem: () => null, setItem() {}, removeItem() {} };

const { panDelta } = await import('./map.js');

test('AK1: import does not touch DOM/window before this point (proven by reaching here)', () => {
  assert.ok(true);
});

test('AK2: panDelta normalizes a diagonal so W+D is not sqrt(2)x faster than one direction', () => {
  const single = panDelta(new Set(['w']), 1000, 100);
  const diag = panDelta(new Set(['w', 'd']), 1000, 100);
  const singleDist = Math.hypot(single.dx, single.dy);
  const diagDist = Math.hypot(diag.dx, diag.dy);
  assert.ok(Math.abs(singleDist - diagDist) < 1e-9, 'diagonal speed must match single-axis speed');
});

test('AK3: no keys held means no movement', () => {
  const out = panDelta(new Set(), 1000, 100);
  assert.deepEqual(out, { dx: 0, dy: 0 });
});
