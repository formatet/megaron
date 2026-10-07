import test from 'node:test';
import assert from 'node:assert/strict';
import { loadMarchPreview, marchPreviewHTML, createMarchPreview } from './march_preview.js';
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
  const value = await loadMarchPreview('w', { id: 'u', mode: 'redirect' }, {}, () => { throw Error('must not request march'); });
  for (const reason of [value.reason, 'courier_required']) {
    assert.match(marchPreviewHTML({ available: false, reason }), /Runner must deliver/);
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
