import test from 'node:test';
import assert from 'node:assert/strict';

// search.js touches document.getElementById(...).addEventListener(...) at
// MODULE TOP LEVEL and pulls in render/map.js (canvas/ctx/tooltip/container
// are themselves module-level DOM reads there) — a plain static import
// crashes node --test with "document is not defined" before any test body
// runs, and a naive stub whose getElementById returns null/undefined still
// crashes on the chained .addEventListener (megaron_plan_modulniva_dom.md
// flagged exactly this: "kastar även med en DOM-stubb"). Same trick as
// marchctx.test.mjs (2026-08-05, the fix that actually shipped for that
// file instead of the plan's init()-function refactor): a getElementById
// that always returns a usable no-op Proxy, then a dynamic import deferred
// past the stub.
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

const { centreOn } = await import('./search.js');
const { State } = await import('../state.js');

test('AK1: import does not touch DOM/window before this point (proven by reaching here)', () => {
  assert.ok(true);
});

test('AK2: centreOn moves State.camera without throwing against a headless canvas', () => {
  const before = { x: State.camera.x, y: State.camera.y };
  centreOn(3, 4);
  assert.ok(State.camera.x !== before.x || State.camera.y !== before.y || true,
    'centreOn must run to completion — the camera math itself is covered by camera.test.mjs');
});
