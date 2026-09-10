import test from 'node:test';
import assert from 'node:assert/strict';

// citygrid.js imports api.js/state.js, whose import graph touches browser
// globals at module load — same stub-then-dynamic-import trick as
// report.test.mjs and marchctx.test.mjs.
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

const { placementOutcome, refusalText, selectionKey } = await import('./citygrid.js');

// The refusal sentences below are copied verbatim from the server
// (api/handlers/settlement_placement.go) — they are the actual bodies this
// endpoint returns, not invented fixtures. If the server rewords one, these
// tests keep passing (the point is that whatever it says reaches the player),
// but the strings document what a player really sees.
const POOL_EMPTY = 'no citizens left in the pool — every citizen is already placed';
const FOG        = "that hex hasn't been scouted yet — fog-of-war hexes can't be staffed";
const NO_OPTION  = 'this hex has no production option for that good';
const FULLY      = 'this hex is fully staffed for that good';

const refuse = (status, error) => ({ ok: false, status, json: async () => ({ error }) });
const accept = (body = {}) => ({ ok: true, status: 200, json: async () => body });

const HEX = { target_kind: 'hex', hex_ordinal: 5 };
const base = (fetchImpl) => ({
  placementsURL: '/api/v1/worlds/W/provinces/P/placements',
  targetBody: HEX, good: 'grain', cap: 4, fetchImpl,
});

// ── The bug this slice closes ────────────────────────────────────────────────
// doAction awaited every fetch without reading res.ok, so a refusal and a
// success were indistinguishable: click +1, nothing moves, nothing is said.

test('CG1: a refused +1 carries the server\'s own reason, and does NOT count as a change', async () => {
  const out = await placementOutcome('place1', base(async () => refuse(409, POOL_EMPTY)));
  assert.equal(out.refusal, POOL_EMPTY, 'the pool refusal must reach the player verbatim');
  assert.equal(out.changed, false, 'nothing changed server-side, so nothing to re-render');
});

test('CG2: each distinct server refusal surfaces as itself — not one generic line', async () => {
  const seen = [];
  for (const [status, msg] of [[403, FOG], [422, NO_OPTION], [409, FULLY]]) {
    const out = await placementOutcome('place1', base(async () => refuse(status, msg)));
    seen.push(out.refusal);
  }
  assert.deepEqual(seen, [FOG, NO_OPTION, FULLY]);
  assert.equal(new Set(seen).size, 3, 'three different situations must read as three different lines');
});

test('CG3: a successful +1 reports a change and says nothing', async () => {
  const out = await placementOutcome('place1', base(async () => accept()));
  assert.equal(out.changed, true);
  assert.equal(out.refusal, null);
});

// ── Fill ────────────────────────────────────────────────────────────────────

test('CG4: Fill that places nothing at all explains why (the old code broke out silently)', async () => {
  const out = await placementOutcome('fill', base(async () => refuse(409, POOL_EMPTY)));
  assert.equal(out.refusal, POOL_EMPTY);
  assert.equal(out.changed, false);
});

test('CG5: Fill that places some then hits the cap stays quiet — the pips moved, that IS the feedback', async () => {
  let n = 0;
  const out = await placementOutcome('fill', base(async () => (++n <= 2 ? accept() : refuse(409, FULLY))));
  assert.equal(out.changed, true, 'two citizens were placed, so the grid must re-render');
  assert.equal(out.refusal, null, 'a partial fill is not a silent failure — do not nag');
});

test('CG6: Fill respects cap as its loop guard', async () => {
  let calls = 0;
  await placementOutcome('fill', { ...base(async () => { calls++; return accept(); }), cap: 3 });
  assert.equal(calls, 3);
});

// ── Unplace ─────────────────────────────────────────────────────────────────

test('CG7: −1 deletes the matching ordinal and reports a change', async () => {
  const urls = [];
  const out = await placementOutcome('unplace1', base(async (url, opts) => {
    urls.push((opts && opts.method) || 'GET');
    if (!opts) return accept({ placements: [{ good_key: 'grain', hex_ordinal: 5, gubbe_ordinal: 7 }] });
    return accept();
  }));
  assert.equal(out.changed, true);
  assert.equal(out.refusal, null);
  assert.deepEqual(urls, ['GET', 'DELETE']);
});

test('CG8: −1 with no matching citizen says so and refreshes instead of doing nothing', async () => {
  const out = await placementOutcome('unplace1', base(async (url, opts) =>
    opts ? accept() : accept({ placements: [] })));
  assert.match(out.refusal, /No citizen of yours/);
  assert.equal(out.refresh, true, 'a stale grid must be refreshed, not left as-is');
  assert.equal(out.changed, false);
});

test('CG9: a failed DELETE is reported, not swallowed', async () => {
  const out = await placementOutcome('unplace1', base(async (url, opts) =>
    opts ? refuse(403, 'not your settlement')
         : accept({ placements: [{ good_key: 'grain', hex_ordinal: 5, gubbe_ordinal: 7 }] })));
  assert.equal(out.refusal, 'not your settlement');
  assert.equal(out.changed, false);
});

// ── refusalText robustness ──────────────────────────────────────────────────

test('CG10: an empty or non-JSON error body still produces a line, never blank silence', async () => {
  const text = await refusalText({ ok: false, status: 502, json: async () => { throw new SyntaxError('no body'); } });
  assert.match(text, /502/);
  assert.notEqual(text.trim(), '');
});

// ── Keeping the panel open across a re-render ───────────────────────────────
// +1/−1 used to close the panel the player was clicking in: the re-render
// rebuilt the whole widget and the detail pane fell back to its empty state.
// selectionKey is what lets the restore path find the same hex again.

test('CG12: the city centre and a hex are never confused for one another', () => {
  assert.equal(selectionKey({ dataset: { target: 'city' } }), 'city');
  assert.equal(selectionKey({ dataset: { target: 'hex', ordinal: '5' } }), 'hex:5');
  assert.notEqual(selectionKey({ dataset: { target: 'city' } }),
                  selectionKey({ dataset: { target: 'hex', ordinal: '5' } }));
});

test('CG13: two different hexes get different keys — a restore must not land on the wrong one', () => {
  const keys = ['1', '5', '18'].map(o => selectionKey({ dataset: { target: 'hex', ordinal: o } }));
  assert.equal(new Set(keys).size, 3);
});

test('CG14: the key survives a round trip through a re-rendered element with the same identity', () => {
  // The restore matches on key, not on object identity — the <g> after a
  // re-render is a different element carrying the same dataset.
  const before = { dataset: { target: 'hex', ordinal: '12' } };
  const afterRerender = { dataset: { target: 'hex', ordinal: '12' } };
  assert.equal(selectionKey(before), selectionKey(afterRerender));
});

test('CG15: a missing or dataset-less element yields null, so nothing is restored rather than throwing', () => {
  assert.equal(selectionKey(null), null);
  assert.equal(selectionKey({}), null);
});

test('CG11: insufficient_goods is delegated to formatApiError, the same helper every other surface uses', async () => {
  const text = await refusalText({
    ok: false, status: 422,
    json: async () => ({ error: 'insufficient_goods', missing: [{ good: 'timber', need: 12.4, have: 3.02 }] }),
  });
  assert.match(text, /timber 12 needed, 3 in store/);
});
