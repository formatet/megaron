import test from 'node:test';
import assert from 'node:assert/strict';
import { sitosGranaryState, sitosStateHtml } from './sitos_view.js';

// Rött-före (webb/magasinet-i-stadsvyn): city.js already renders coverage and
// granary totals (76d21666, 7ea8d40), but never the STATE the mechanic is in —
// `keryx status` says "släpper mat till staden" / "vilar" / "lägger undan ett
// tionde av överskottet" (cmd_status.go:362-372) and the web drawer says
// nothing of the kind. This test targets the module this slice adds; before
// it exists the import above fails, which is the red state — see report.

test('under LowDays: empty + shrinking reads as a hard warning', () => {
  const s = sitosGranaryState(3, 10, 30, 0, -5);
  assert.equal(s.text, 'TOMT och lagret krymper — staden är utan skydd');
  assert.equal(s.severity, 'empty-shrinking');
});

test('under LowDays but empty + growing reads as reassuring, not alarming', () => {
  const s = sitosGranaryState(3, 10, 30, 0, 12);
  assert.equal(s.text, 'tomt — men lagret växer, täckningen stiger');
  assert.equal(s.severity, 'empty-growing');
});

test('under LowDays with stock still in the granary: releasing', () => {
  const s = sitosGranaryState(4, 10, 30, 1200, -50);
  assert.equal(s.text, 'släpper mat till staden');
  assert.equal(s.severity, 'release');
});

test('between LowDays and HighDays: resting', () => {
  const s = sitosGranaryState(15, 10, 30, 5000, 3);
  assert.equal(s.text, 'vilar — varken undan eller ut');
  assert.equal(s.severity, 'rest');
});

test('over HighDays: storing a tenth of the surplus', () => {
  const s = sitosGranaryState(40, 10, 30, 20000, 10);
  assert.equal(s.text, 'lägger undan ett tionde av överskottet');
  assert.equal(s.severity, 'store');
});

test('branch order matches cmd_status.go: cov==low is NOT "under low" (Go uses cov < low)', () => {
  const s = sitosGranaryState(10, 10, 30, 5000, 0);
  assert.equal(s.severity, 'rest');
});

test('branch order matches cmd_status.go: cov==high is still "vilar", not "store" (Go uses cov <= high)', () => {
  const s = sitosGranaryState(30, 10, 30, 5000, 0);
  assert.equal(s.severity, 'rest');
});

test('sitosStateHtml renders a stat-row with the derived state text and severity class', () => {
  const html = sitosStateHtml({ coverage_ticks: 4, low_ticks: 10, high_ticks: 30, granary_total: 1200, food_net_per_tick: -50 });
  assert.match(html, /sr-label/);
  assert.match(html, /sitos-state-release/);
  assert.match(html, /släpper mat till staden/);
});

test('sitosStateHtml on a settlement with no sitos object at all renders nothing and does not throw', () => {
  assert.equal(sitosStateHtml(null), '');
  assert.equal(sitosStateHtml(undefined), '');
});
