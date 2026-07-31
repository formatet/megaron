import test from 'node:test';
import assert from 'node:assert/strict';
import { kharisNetView } from './kult_kharis.js';

test('AK1: kharis_net_known true draws a Net row, distinct from Passive, signed with one decimal', () => {
  const view = kharisNetView({ kharis_net_per_day: 2.34, kharis_net_known: true, kharis_devotion_idle: false });
  assert.match(view.netHtml, /sr-label">Net</, 'label must not be "Passive" — a distinct row');
  assert.match(view.netHtml, /\+2\.3 kharis\/day/, 'signed, one decimal, same unit as the passive row');
  assert.equal(view.idleHtml, '', 'idle not flagged — no warning');
});

test('AK1b: a negative net is signed with a minus, not a double sign', () => {
  const view = kharisNetView({ kharis_net_per_day: -1.5, kharis_net_known: true });
  assert.match(view.netHtml, /-1\.5 kharis\/day/);
  assert.doesNotMatch(view.netHtml, /\+-/);
});

test('AK2: kharis_net_known false draws no net row at all — not even "0.0"', () => {
  const view = kharisNetView({ kharis_net_per_day: 0, kharis_net_known: false });
  assert.equal(view.netHtml, '', 'kharis_net_known=false means "no temples" — draw nothing');
});

test('AK2b: kharis_net_known missing entirely (not strictly true) also draws nothing', () => {
  const view = kharisNetView({ kharis_net_per_day: 5 });
  assert.equal(view.netHtml, '');
});

test('AK3: kharis_devotion_idle true draws the idle-capacity warning', () => {
  const view = kharisNetView({ kharis_net_known: true, kharis_net_per_day: 1, kharis_devotion_idle: true });
  assert.match(view.idleHtml, /kult-warn/);
  assert.notEqual(view.idleHtml, '');
});

test('AK3b: kharis_devotion_idle false (or absent) draws no warning', () => {
  assert.equal(kharisNetView({ kharis_devotion_idle: false }).idleHtml, '');
  assert.equal(kharisNetView({}).idleHtml, '');
});

test('a null/undefined payload draws nothing, crashes nothing', () => {
  assert.deepEqual(kharisNetView(null), { netHtml: '', idleHtml: '' });
  assert.deepEqual(kharisNetView(undefined), { netHtml: '', idleHtml: '' });
});
