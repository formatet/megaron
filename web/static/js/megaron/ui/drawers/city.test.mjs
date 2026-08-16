import test from 'node:test';
import assert from 'node:assert/strict';
import { loyaltyLogRowsHTML } from './city.js';

// megaron_plan_webbytor_keryx_paritet.md, Slice LOYALTY-LOG: the city drawer
// shows the loyalty VALUE but never WHY it changed. loyaltyLogRowsHTML is the
// pure string builder for the new "Lojalitetslogg" section (same pattern as
// economy.js's goodsRateCell) — a fake loyalty-log response in, HTML out, no
// DOM or fetch needed to test it.

test('AK1: city.js imports under node --test without a DOM', () => {
  assert.equal(typeof loyaltyLogRowsHTML, 'function');
});

test('empty loyalty-log renders a friendly empty-state, not a bare table', () => {
  const html = loyaltyLogRowsHTML([]);
  assert.match(html, /empty-state/);
  assert.doesNotMatch(html, /<table/);
});

test('a positive delta is signed with a leading + and the --safe tone', () => {
  const html = loyaltyLogRowsHTML([
    { id: 1, event_type: 'gift', loyalty_delta: 1, reason: 'Received a significant gift', created_at: '2026-08-16T10:00:00Z' },
  ]);
  assert.match(html, /\+1/);
  assert.match(html, /var\(--safe\)/);
  assert.match(html, /Received a significant gift/);
});

test('a negative delta keeps its own minus sign (no double sign) and the --accent tone', () => {
  const html = loyaltyLogRowsHTML([
    { id: 2, event_type: 'revolt_risk', loyalty_delta: -2, reason: 'Garrison dominated by foreign troops', created_at: '2026-08-15T09:00:00Z' },
  ]);
  assert.match(html, />-2</);
  assert.doesNotMatch(html, /\+-2/);
  assert.match(html, /var\(--accent\)/);
});

test('rows are rendered in the order given (server already sorts newest-first) — not re-sorted client-side', () => {
  const html = loyaltyLogRowsHTML([
    { id: 3, event_type: 'a', loyalty_delta: 1, reason: 'newest', created_at: '2026-08-16T10:00:00Z' },
    { id: 2, event_type: 'b', loyalty_delta: -1, reason: 'older', created_at: '2026-08-15T10:00:00Z' },
  ]);
  assert.ok(html.indexOf('newest') < html.indexOf('older'));
});

test('a hostile reason string is escaped, not injected as markup', () => {
  const html = loyaltyLogRowsHTML([
    { id: 4, event_type: 'x', loyalty_delta: 1, reason: '<img src=x onerror=alert(1)>', created_at: '2026-08-16T10:00:00Z' },
  ]);
  assert.doesNotMatch(html, /<img/);
  assert.match(html, /&lt;img/);
});

test('a zero delta (if it ever occurs) gets no sign and the neutral --text-dim tone', () => {
  const html = loyaltyLogRowsHTML([
    { id: 5, event_type: 'noop', loyalty_delta: 0, reason: 'no change', created_at: '2026-08-16T10:00:00Z' },
  ]);
  assert.match(html, />0</);
  assert.match(html, /var\(--text-dim\)/);
});
