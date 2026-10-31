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

test('AK3: kharis_devotion_idle true draws the short unfilled-workplaces warning', () => {
  const view = kharisNetView({ kharis_net_known: true, kharis_net_per_tick: 1, kharis_devotion_idle: true });
  assert.match(view.idleHtml, /kult-warn/);
  assert.match(view.idleHtml, /Temple workplaces unfilled<\/p>/);
  assert.doesNotMatch(view.idleHtml, /place citizens/);
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


test('realm upkeep is one short line from the authoritative total, including templeless realms; demand/given likewise', () => {
  const pd = {kharis_city_count: 5, kharis_base_decay: 0.1, kharis_city_decay: 1, kharis_daily_decay: 1.1, kharis_population:2500, kharis_population_decay:.25,
    kharis_net_known: true, kharis_net_per_tick: -1.1, temple_offers: []};
  assert.match(kharisDecayHTML(pd), /Upkeep<\/span><span class="sr-val">−1.1 \/ game day/);
  assert.doesNotMatch(kharisDecayHTML(pd), /residents|base \+|loses each game day/);
  assert.match(kharisDecayHTML({kharis_demand:5,kharis_given_weight:2.345}), /Demand: 5 · Given: 2.35/);
  assert.match(kharisNetView(pd).netHtml, /-1.1 kharis per game day/);
  // Deliberately unlike today's rule: display values, never recalculate the formula.
  assert.match(kharisDecayHTML({...pd, kharis_daily_decay: 9.3}), /−9.3 \/ game day/);
});

test('missing or partial realm decay never claims zero cost', () => {
  for (const pd of [null, {}, {kharis_city_count: 5}, {kharis_city_count:5,kharis_daily_decay:NaN}]) {
    assert.match(kharisDecayHTML(pd), /not reported/);
    assert.doesNotMatch(kharisDecayHTML(pd), /0.0|NaN/);
  }
});

test('production and extra worker yield are server-owned; yield is conditional, not a staffing recommendation', () => {
 const view=kharisNetView({kharis_net_known:true,kharis_net_per_tick:1.35,kharis_production_per_tick:2.1,kharis_per_temple_worker:1.05});
 assert.match(view.netHtml,/Temple production/);assert.match(view.netHtml,/2.10 kharis per game day/);
 assert.match(view.netHtml,/Per extra temple worker<\/span><span class="sr-val">\+1.05</);
 assert.doesNotMatch(view.netHtml,/Requires a free citizen|reference offering/);
});

test('R3/R5: blessings from kharis 60, wrath risk with a number, no prose; protection stays', async () => {
 const {kharisRulesHTML}=await import('./kult_kharis.js');
 const html=kharisRulesHTML({kharis_blessing_threshold:60,kharis_wrath:{threshold:15,daily_probability:.03,protection_remaining_days:2,protection_until:'2026-10-23T18:30:00Z'}});
 assert.match(html,/Blessings from kharis<\/span><span class="sr-val">60</);
 assert.match(html,/Wrath risk below 15<\/span><span class="sr-val">3% \/ game day</);assert.match(html,/Protected from divine punishment/);
 assert.doesNotMatch(html,/Everyday prayers|level-2 temple|lowered ceiling|with or without a temple/);
 assert.doesNotMatch(html,/Invalid|NaN|game day 123/);
});

test('R2 punishment curve: the Kult rules name both tiers', async () => {
 const {kharisRulesHTML}=await import('./kult_kharis.js');
 const html=kharisRulesHTML({kharis_blessing_threshold:60,kharis_wrath:{threshold:30,daily_probability:.02,wrathful_threshold:5,wrathful_probability:.04,protection_remaining_days:0}});
 assert.match(html,/Wrath risk below 30<\/span><span class="sr-val">2% \/ game day, 4% below 5</);
});

test('above ceiling is a label with the ceiling; no worker yield is promised', () => {
 const view = kharisNetView({kharis:80,kharis_cap:50,kharis_net_known:true,kharis_net_per_tick:-.2,kharis_per_temple_worker:1.05});
 assert.match(view.netHtml,/Above ceiling<\/span><span class="sr-val">50</);assert.doesNotMatch(view.netHtml,/Once below|declines through/);
 assert.match(view.netHtml,/-0.2 kharis per game day/);
 assert.doesNotMatch(view.netHtml,/Per extra temple worker/);
 const normal=kharisNetView({kharis:50,kharis_cap:50,kharis_per_temple_worker:1.05});
 assert.doesNotMatch(normal.netHtml,/standing drifts down/);
 assert.match(normal.netHtml,/Per extra temple worker/);
});

test('ORDER29: empty temple slots above the ceiling do not invite staffing to raise kharis', () => {
 const view = kharisNetView({kharis:80,kharis_cap:50,kharis_devotion_idle:true,kharis_per_temple_worker:1.05});
 assert.equal(view.idleHtml,'');
 assert.match(view.netHtml,/Above ceiling<\/span><span class="sr-val">50</);
 assert.doesNotMatch(view.netHtml,/Per extra temple worker|place citizens/);
});

 test('standing shows the server short line (band + number), escaped, never the prose summary',()=>{
 const html=kharisStandingHTML({kharis:90,kharis_standing:{short:'Kharis 4.0 · <Wrathful>',summary:'Kharis 4.0 (Wrathful) · about 3% chance of divine punishment'}});
 assert.match(html,/Kharis 4.0 · &lt;Wrathful&gt;/);assert.doesNotMatch(html,/chance/);
 assert.equal(kharisStandingHTML({kharis:90}),'');
 });
