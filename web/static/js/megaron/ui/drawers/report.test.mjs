import test from 'node:test';
import assert from 'node:assert/strict';

// report.js pulls in api.js (fetchAuth) whose own import graph touches
// browser globals at module load — same stub-then-dynamic-import trick as
// marchctx.test.mjs, so this file can exercise buildContext() (the pure half
// of mig 123's context payload, temenos_buggrapporter.md) without a real DOM.
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

const { buildContext } = await import('./report.js');
const { State } = await import('../../state.js');

function resetState() {
  State.marchCtxDest = null;
  State.marchCtxUnits = [];
  State.previousDrawer = null;
  State.cityViewID = null;
  State.provinceData = [];
}

test('buildContext: nothing open → undefined, not an empty object', () => {
  resetState();
  assert.equal(buildContext(), undefined);
});

test('buildContext: march-order menu open → dest + unit ids, regardless of view', () => {
  resetState();
  State.marchCtxDest = { q: 5, r: 60, terrain: 'coastal_sea', isSea: true };
  State.marchCtxUnits = [{ id: 'u1' }, { id: 'u2' }];
  const ctx = buildContext();
  assert.deepEqual(ctx.march_ctx_dest, { q: 5, r: 60 });
  assert.deepEqual(ctx.unit_ids, ['u1', 'u2']);
});

test('buildContext: city drawer open → settlement_id from the active (cycled) settlement', () => {
  resetState();
  State.previousDrawer = 'city';
  State.provinceData = [
    { id: 'cap', own: true, is_capital: true, is_outpost: false },
    { id: 'colony', own: true, is_capital: false, is_outpost: false },
  ];
  State.cityViewID = 'colony'; // player had cycled off the capital
  const ctx = buildContext();
  assert.equal(ctx.settlement_id, 'colony', 'must respect cityViewID, not default to the capital');
});

test('buildContext: settlement_id is withheld when a different drawer was open — no misleading city context', () => {
  resetState();
  State.previousDrawer = 'war';
  State.provinceData = [{ id: 'cap', own: true, is_capital: true, is_outpost: false }];
  State.cityViewID = 'cap';
  const ctx = buildContext();
  assert.equal(ctx, undefined, 'a war-drawer report must not silently claim a city context');
});
