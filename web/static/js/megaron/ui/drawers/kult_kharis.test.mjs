import test from 'node:test';
import assert from 'node:assert/strict';
import { kharisNetView, kharisDecayHTML, kharisStandingHTML } from './kult_kharis.js';

test('AK1: kharis_net_known true draws a Net row, distinct from Passive, signed with one decimal', () => {
  const view = kharisNetView({ kharis_net_per_tick: 2.34, kharis_net_known: true, kharis_devotion_idle: false });
  assert.match(view.netHtml, /sr-label">Net</, 'label must not be "Passive" — a distinct row');
  assert.match(view.netHtml, /\+2\.3 kharis per game day/, 'signed, one decimal, same unit as the passive row');
  assert.equal(view.idleHtml, '', 'idle not flagged — no warning');
});

test('AK1b: a negative net is signed with a minus, not a double sign', () => {
  const view = kharisNetView({ kharis_net_per_tick: -1.5, kharis_net_known: true });
  assert.match(view.netHtml, /-1\.5 kharis per game day/);
  assert.doesNotMatch(view.netHtml, /\+-/);
});

test('AK2: kharis_net_known false draws no net row at all — not even "0.0"', () => {
  const view = kharisNetView({ kharis_net_per_tick: 0, kharis_net_known: false });
  assert.equal(view.netHtml, '', 'kharis_net_known=false means "projection unavailable" — draw nothing');
});

test('AK2b: kharis_net_known missing entirely (not strictly true) also draws nothing', () => {
  const view = kharisNetView({ kharis_net_per_tick: 5 });
  assert.equal(view.netHtml, '');
});

test('AK3: kharis_devotion_idle true draws the idle-capacity warning', () => {
  const view = kharisNetView({ kharis_net_known: true, kharis_net_per_tick: 1, kharis_devotion_idle: true });
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


test('realm decay breakdown uses authoritative base, city cost and total, including templeless realms', () => {
  const pd = {kharis_city_count: 5, kharis_base_decay: 0.1, kharis_city_decay: 1, kharis_daily_decay: 1.1, kharis_population:2500, kharis_population_decay:.25,
    kharis_net_known: true, kharis_net_per_tick: -1.1, temple_offers: []};
  assert.match(kharisDecayHTML(pd), /0.1 base \+ 5 cities cost 1.0 \+ 2500 residents cost 0.3 = 1.1 kharis per game day/);
  assert.match(kharisNetView(pd).netHtml, /-1.1 kharis per game day/);
  // Deliberately unlike today's rule: display values, never recalculate the formula.
  assert.match(kharisDecayHTML({...pd, kharis_base_decay: 0.7, kharis_city_decay: 8.6, kharis_daily_decay: 9.3}), /0.7 base \+ 5 cities cost 8.6 \+ 2500 residents cost 0.3 = 9.3 kharis per game day/);
  assert.match(kharisDecayHTML({...pd, kharis_city_count: 1, kharis_city_decay: 0, kharis_daily_decay: 0.1}), /1 city cost 0.0/);
});

test('missing or partial realm decay never claims zero cost', () => {
  for (const pd of [null, {}, {kharis_city_count: 5}, {kharis_city_count:5,kharis_base_decay:0.1,kharis_city_decay:1,kharis_daily_decay:NaN}]) {
    assert.match(kharisDecayHTML(pd), /not reported/);
    assert.doesNotMatch(kharisDecayHTML(pd), /0.0|NaN/);
  }
});

test('production and extra worker yield are server-owned; yield is conditional, not a staffing recommendation', () => {
 const view=kharisNetView({kharis_net_known:true,kharis_net_per_tick:1.35,kharis_production_per_tick:2.1,kharis_per_temple_worker:1.05});
 assert.match(view.netHtml,/Temple production/);assert.match(view.netHtml,/2.10 kharis per game day/);
 assert.match(view.netHtml,/worker gives at least \+1.05 kharis per game day/);
 assert.match(view.netHtml,/Requires a free citizen, a temple workplace and its daily offering/);
});

test('R3/R5: everyday prayers work at L1, blessings need standing 60, temple does not grant protection', async () => {
 const {kharisRulesHTML}=await import('./kult_kharis.js');
 const html=kharisRulesHTML({kharis_blessing_threshold:60,kharis_wrath:{threshold:15,daily_probability:.03,protection_remaining_days:2,protection_until:'2026-10-23T18:30:00Z'}});
 assert.match(html,/Everyday prayers work at a level-1 temple/);assert.match(html,/need 60 kharis/);
 assert.match(html,/with or without a temple/);assert.match(html,/Protected from divine punishment/);
 assert.match(html,/Only a level-2 temple or higher/);
 assert.match(html,/Kharis retained above a lowered ceiling still counts/);
 assert.doesNotMatch(html,/Invalid|NaN|game day 123/);
});

test('above ceiling explains ignored gains and does not promise a worker can increase standing', () => {
 const view = kharisNetView({kharis:80,kharis_cap:50,kharis_net_known:true,kharis_net_per_tick:-.2,kharis_per_temple_worker:1.05});
 assert.match(view.netHtml,/Once below, temples can raise it again/);
 assert.match(view.netHtml,/-0.2 kharis per game day/);
 assert.doesNotMatch(view.netHtml,/One more fed temple worker/);
 const normal=kharisNetView({kharis:50,kharis_cap:50,kharis_per_temple_worker:1.05});
 assert.doesNotMatch(normal.netHtml,/standing drifts down/);
 assert.match(normal.netHtml,/One more fed temple worker/);
});

test('ORDER29: empty temple slots above the ceiling do not invite staffing to raise kharis', () => {
 const view = kharisNetView({kharis:80,kharis_cap:50,kharis_devotion_idle:true,kharis_per_temple_worker:1.05});
 assert.equal(view.idleHtml,'');
 assert.match(view.netHtml,/above the ceiling 50/);
 assert.doesNotMatch(view.netHtml,/One more|place citizens/);
});

 test('standing uses escaped server meaning without recomputing probability',()=>{
 assert.match(kharisStandingHTML({kharis:90,kharis_standing:{summary:'Kharis 4.0 (Wrathful) · 3% <risk>'}}),/3% &lt;risk&gt;/);
 assert.equal(kharisStandingHTML({kharis:90}),'');
 });
