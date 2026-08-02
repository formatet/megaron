import test from 'node:test';
import assert from 'node:assert/strict';

// "Last i rörelse" (web/last-i-rorelse, 2026-08): before this slice a player's
// own internal transfer vanished from view after the "sent" toast — no ETA, no
// list of what's still moving. formatCargoRows/renderCargoHTML are the pure
// half of the fix, following the render/camera.test.mjs pattern: explicit
// inputs, checked output, `nowMs` passed in rather than read from
// Date.now()/serverNow().
//
// economy.js itself has no side-effect-free import path, though: it pulls in
// ui/misc.js (renderLockedActions), which runs
// `document.addEventListener('pointerdown', ...)` at MODULE TOP LEVEL (the
// music-player autostart) — a plain `import` at the top of this file makes
// Node's test runner evaluate that line before any test body runs, crashing
// the whole file with "document is not defined" before a single assertion
// executes. A static import can't be rescued by anything done later in the
// same file. The fix: stub the one global misc.js's top level touches, THEN
// dynamically import economy.js — deferring evaluation of its import graph
// to run AFTER the stub exists.
globalThis.document ??= { addEventListener() {} };
const { formatCargoRows, renderCargoHTML } = await import('./economy.js');

const BASE_TRADE = {
  good_key: 'grain',
  quantity: 250,
  origin_q: 0, origin_r: 0,
  dest_q: 3, dest_r: 0,
  arrives_at: '2026-08-02T12:00:00.000Z',
  mine: true,
};
const NOW = new Date('2026-08-02T10:00:00.000Z').getTime(); // 2h before arrival

test('AK1: formatCargoRows carries good/quantity/from/to straight through', () => {
  const rows = formatCargoRows([BASE_TRADE], NOW);
  assert.equal(rows.length, 1);
  assert.equal(rows[0].good, 'grain');
  assert.equal(rows[0].qty, 250);
  assert.equal(rows[0].from, '(0,0)');
  assert.equal(rows[0].to, '(3,0)');
});

test('AK2: ETA is computed from arrives_at - nowMs, not from the wall clock', () => {
  const rows = formatCargoRows([BASE_TRADE], NOW);
  assert.equal(rows[0].eta, '2h 0m');
});

test('AK3: an already-arrived trade (arrives_at in the past) reads "arrived"', () => {
  const arrivedNow = new Date('2026-08-02T13:00:00.000Z').getTime(); // 1h AFTER arrives_at
  const rows = formatCargoRows([BASE_TRADE], arrivedNow);
  assert.equal(rows[0].eta, 'arrived');
});

test('AK4: quantity floors fractional amounts (matches the rest of the drawer)', () => {
  const rows = formatCargoRows([{ ...BASE_TRADE, quantity: 249.9 }], NOW);
  assert.equal(rows[0].qty, 249);
});

test('AK5 (regression): renderCargoHTML on an empty list says nothing is in transit, not a blank table', () => {
  const html = renderCargoHTML([], NOW);
  assert.match(html, /Nothing of yours in transit/);
  assert.doesNotMatch(html, /<table/);
});

test('AK6: renderCargoHTML never claims a fixed loss percentage — only that cargo is physical and can be seized', () => {
  const html = renderCargoHTML([BASE_TRADE], NOW);
  assert.match(html, /intercepted/);
  assert.match(html, /seized/);
  assert.doesNotMatch(html, /%/, 'must never quote a loss percentage for physical cargo');
});

test('AK7: renderCargoHTML escapes good_key so a hostile good name cannot inject markup', () => {
  const html = renderCargoHTML([{ ...BASE_TRADE, good_key: '<img src=x onerror=alert(1)>' }], NOW);
  assert.doesNotMatch(html, /<img/);
});
