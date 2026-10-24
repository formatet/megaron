import { State as timeTestState } from './../state.js';
import { fmtTickTime as testTickTime } from './fmt_clock.js';
timeTestState.CURRENT_TICK=0; timeTestState.TICK_SECONDS=60; timeTestState.TICK_ANCHOR_MS=Date.now();
import test from 'node:test';
import assert from 'node:assert/strict';
import { loadMarchPreview, marchPreviewHTML, marchSilverLine, createMarchPreview } from './march_preview.js';
const pause = () => new Promise(resolve => setTimeout(resolve, 15));

test('forecast reads the exact order with GET, including landing and stance', async () => {
  let url;
  const result = await loadMarchPreview('w', { id: 'u' }, {
    target_q: -3, target_r: 8, intent: 'land', cargo_intent: 'colonize', name: 'A & B', stance: '',
  }, async (...args) => {
    assert.equal(args.length, 1, 'no mutation method or body');
    url = new URL(args[0], 'https://test.invalid');
    return { ok: true, json: async () => ({ available: true, arrival_tick: 9 }) };
  });
  assert.equal(url.pathname, '/api/v1/worlds/w/units/u/march-preview');
  assert.equal(url.searchParams.get('target_q'), '-3');
  assert.equal(url.searchParams.get('cargo_intent'), 'colonize');
  assert.equal(url.searchParams.get('name'), 'A & B');
  assert.equal(url.searchParams.has('stance'), false);
  assert.equal(result.arrival_tick, 9);
});

test('redirect and courier never masquerade as immediate arrival', async () => {
  let requests = 0;
  const value = await loadMarchPreview('w', { id: 'u', mode: 'redirect' }, {}, async (...args) => {
    requests++; assert.equal(args.length, 1, 'redirect preview remains read-only GET');
    return {ok:true,json:async()=>({available:false,reason:'redirect',upkeep_silver_home:2,upkeep_silver_field:2,silver_runway_ticks:10})};
  });
  assert.equal(requests,1);
  assert.match(marchPreviewHTML(value), /Pay: 2 → 2 silver/);
  for (const reason of [value.reason, 'courier_required']) {
    assert.match(marchPreviewHTML({ available: false, reason }), /messenger must deliver/);
    assert.doesNotMatch(marchPreviewHTML({ available: false, reason }), /Estimated arrival/);
  }
  assert.match(marchPreviewHTML({ available: false, reason: 'unknown_terrain' }), /unexplored terrain/);
});

test('each selected unit keeps its own arrival; names and API errors are escaped', async () => {
  const output = [];
  const controller = createMarchPreview(html => output.push(html), async url => ({
    ok: !url.includes('/bad/'),
    json: async () => url.includes('/bad/') ? { error: '<img src=x>' } : {
      available: true, arrival_tick: 20, duration_ticks: url.includes('/slow/') ? 8 : 2,
      arrives_at_utc: '2099-01-01T00:00:00Z',
    },
  }), 0);
  controller.update('w', [{id:'fast',name:'<fast>'},{id:'slow',name:'Slow'},{id:'bad',name:'Bad'}], { target_q: 1 });
  await pause();
  const html = output.at(-1);
  assert.match(html, /2 game days/); assert.match(html, /8 game days/);
  assert.match(html, /&lt;fast&gt;/); assert.doesNotMatch(html, /<img|<fast>/);
  assert.match(html, /Forecast unavailable/);
});

test('changing destination discards old response even during debounce; close cancels', async () => {
  const output = [];
  const pending = [];
  const controller = createMarchPreview(html => output.push(html), url => new Promise(resolve => pending.push({url,resolve})), 0);
  const picks = [{id:'u',name:'Unit'}];
  controller.update('w', picks, { target_q: 1 });
  await pause();
  controller.update('w', picks, { target_q: 2 });
  pending[0].resolve({ ok: true, json: async () => ({ available: false, reason:'unknown_terrain' }) });
  await pause();
  assert.equal(output.at(-1), 'Estimating arrival…');
  controller.cancel();
  pending[1].resolve({ ok: true, json: async () => ({ available: false, reason:'courier_required' }) });
  await pause();
  assert.equal(output.at(-1), 'Estimating arrival…');
  controller.update('w', [], {});
  assert.equal(output.at(-1), '');
});


test('sold uses authoritative rates, includes explicit zero and avoids infinite runway',()=>{
 assert.equal(marchSilverLine({upkeep_silver_home:0,upkeep_silver_field:0,silver_runway_ticks:null}), 'Pay: 0 → 0 silver per game day in the field · no silver drain');
 assert.equal(marchSilverLine({upkeep_silver_home:0.6,upkeep_silver_field:2,silver_runway_ticks:25}), 'Pay: 0.6 → 2 silver per game day in the field · treasury lasts 25 game days');
 assert.equal(marchSilverLine({upkeep_silver_home:0.6,upkeep_silver_field:2,silver_runway_ticks:0}), 'Pay: 0.6 → 2 silver per game day in the field · treasury lasts 0 game days');
 assert.equal(marchSilverLine({upkeep_silver_home:0.45,upkeep_silver_field:1.5,silver_runway_ticks:1}), 'Pay: 0.45 → 1.5 silver per game day in the field · treasury lasts 1 game day');
 assert.equal(marchSilverLine({available:true}), '');
});

test('sold remains visible when courier or unexplored terrain prevents arrival estimate',()=>{
 for(const reason of ['courier_required','unknown_terrain']) {
  const html=marchPreviewHTML({available:false,reason,upkeep_silver_home:0.6,upkeep_silver_field:2,silver_runway_ticks:25});
  assert.match(html,/class="march-sold"/);
  assert.match(html,/Pay: 0.6 → 2 silver per game day in the field · treasury lasts 25 game days/);
  assert.doesNotMatch(html,/Estimated arrival|Infinity|NaN/);
 }
 assert.doesNotMatch(marchPreviewHTML({available:false,reason:'unknown_terrain'}), /march-sold/);
});

test('plural selection sends all IDs to every GET and shows one combined sold line after arrivals',async()=>{
 const urls=[]; const output=[];
 const controller=createMarchPreview(html=>output.push(html), async url=>{
  urls.push(new URL(url,'https://test.invalid'));
  return {ok:true,json:async()=>({available:false,reason:'courier_required',upkeep_silver_home:1.2,upkeep_silver_field:4,silver_runway_ticks:12.5,silver_payer_count:2})};
 },0);
 controller.update('w',[{id:'a',name:'Alpha'},{id:'b',name:'Beta'}],{target_q:3});
 await pause();
 assert.equal(urls.length,2);
 for(const url of urls) assert.equal(url.searchParams.get('unit_ids'),'a,b');
 const html=output.at(-1);
 assert.equal((html.match(/class="march-sold"/g)||[]).length,1);
 assert.match(html,/Pay: 1.2 → 4 silver per game day in the field · shortest treasury lasts 12.5 game days/);
 assert.ok(html.indexOf('Beta:') < html.indexOf('Pay:'));
 assert.equal((html.match(/messenger must deliver/g)||[]).length,2);
});

test('one zero-drain batch row is rendered when all treasuries have no silver drain',async()=>{
 const output=[];
 const controller=createMarchPreview(html=>output.push(html),async()=>({ok:true,json:async()=>({available:false,reason:'unknown_terrain',upkeep_silver_home:0,upkeep_silver_field:0,silver_runway_ticks:null,silver_payer_count:2})}),0);
 controller.update('w',[{id:'a',name:'Alpha'},{id:'b',name:'Beta'}],{target_q:3});
 await pause();
 const html=output.at(-1);
 assert.equal((html.match(/class="march-sold"/g)||[]).length,1);
 assert.match(html,/Pay: 0 → 0 silver per game day in the field · no silver drain/);
 assert.doesNotMatch(html,/NaN|Infinity|shortest treasury lasts/);
});


test('redirect-only selection keeps messenger arrival warning and one authoritative sold row',async()=>{
 const output=[]; const urls=[];
 const controller=createMarchPreview(html=>output.push(html),async url=>{
  urls.push(new URL(url,'https://test.invalid'));
  return {ok:true,json:async()=>({available:false,reason:'redirect',upkeep_silver_home:4,upkeep_silver_field:4,silver_runway_ticks:10,silver_payer_count:1})};
 },0);
 controller.update('w',[{id:'a',name:'Alpha',mode:'redirect'},{id:'b',name:'Beta',mode:'redirect'}],{target_q:3});
 await pause();
 assert.equal(urls.length,2);
 for(const url of urls) assert.equal(url.searchParams.get('unit_ids'),'a,b');
 const html=output.at(-1);
 assert.equal((html.match(/class="march-sold"/g)||[]).length,1);
 assert.equal((html.match(/messenger must deliver/g)||[]).length,2);
 assert.match(html,/Pay: 4 → 4 silver per game day in the field · treasury lasts 10 game days/);
 assert.doesNotMatch(html,/Estimated arrival/);
});

test('ford preview preserves separate land and naval mission options', async () => {
  const urls = [];
  const preview = createMarchPreview(() => {}, async path => {
    urls.push(new URL(path, 'https://test.invalid'));
    return {ok:true,json:async()=>({available:false})};
  }, 0);
  preview.update('w', [{id:'land',name:'Land',order:{stance:'sentry'}}, {id:'ship',name:'Ship',order:{intent:'explore'}}], {target_q:6,target_r:67,ticks:12});
  await pause();
  assert.equal(urls.length, 2);
  for (const url of urls) { assert.equal(url.searchParams.get('target_q'), '6'); assert.equal(url.searchParams.get('target_r'), '67'); }
  assert.equal(urls[0].searchParams.get('stance'), 'sentry');
  assert.equal(urls[0].searchParams.has('intent'), false);
  assert.equal(urls[1].searchParams.get('intent'), 'explore');
  assert.equal(urls[1].searchParams.get('ticks'), '12');
  assert.equal(urls[1].searchParams.has('stance'), false);
});

test('expedition plan is a known budget, conditional on departure, not a hidden first-leg ETA', () => {
  const html = marchPreviewHTML({available:false,reason:'unknown_terrain',planned_expedition_ticks:12,planned_home_by_tick_if_departing_now:17});
  assert.match(html,/Planned expedition: 12 game days/);
  assert.match(html,new RegExp('with immediate departure, home by '+testTickTime(17)));
  assert.match(html,/First-leg arrival remains unknown/);
  assert.doesNotMatch(html,/Estimated arrival:/);
});

test('expedition preview names exact first target without disclosing its ETA',()=>{
 const html=marchPreviewHTML({available:false,reason:'unknown_terrain',planned_expedition_ticks:100,planned_first_target_q:24,planned_first_target_r:20});
 assert.match(html,/first to \(24,20\), then explores around it/);
 assert.match(html,/First-leg arrival remains unknown/);
});

test('pay forecast preserves authoritative half-home and full-field figures',()=>{
 assert.match(marchSilverLine({upkeep_silver_home:0.75,upkeep_silver_field:1.5,silver_runway_ticks:12}), /0.75 → 1.5 silver per game day/);
 assert.match(marchSilverLine({upkeep_silver_home:0.1,upkeep_silver_field:0.2,silver_runway_ticks:12}), /0.1 → 0.2 silver per game day/);
});
