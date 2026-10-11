import { test } from 'node:test';
import assert from 'node:assert/strict';
import { placePanelHTML, namePlace, PLACE_LABELS } from './placename.js';
import { notifText } from './format.js';
import { State } from '../state.js';

test('place naming form is rights-gated for every kind and excludes fog', () => {
  for (const kind of Object.keys(PLACE_LABELS)) {
    const tile = { terrain: 'river', place_kind: kind, place_id: '-6,79', can_name: true };
    assert.match(placePanelHTML(tile), new RegExp(`this ${PLACE_LABELS[kind]}`));
    assert.ok(placePanelHTML(tile).includes(`namePlace('${kind}','-6,79')`));
    for (const patch of [{ can_name: false }, { terrain: 'fog' }, { tier: 'fog' }, { visible: false }, { place_id: "x');alert(1)//" }, { place_kind: 'city' }]) {
      assert.equal(placePanelHTML({ ...tile, ...patch }), '', 'unknown place or spent right has no form');
    }
  }
});

test('place first reached notification puts named unit before place and offers naming', () => {
  for (const [kind, label] of Object.entries(PLACE_LABELS)) {
    const text = notifText('PlaceFirstReached', { unit_name: 'Bronze Guard', kind, q: 6, r: 79 });
    assert.ok(text.startsWith('Your Bronze Guard is the first to reach'));
    assert.ok(text.includes(`this ${label} — you may name it`));
    assert.doesNotMatch(text, /ticks?|\{"/);
  }
});

function harness(t, responses) {
  const globals = Object.fromEntries(['document','window','localStorage','fetch'].map(k => [k,globalThis[k]]));
  const state = { tiles: State.tileData, world: State.WORLD_ID, dirty: State.dirty };
  t.after(() => {
    for (const [k,v] of Object.entries(globals)) v === undefined ? delete globalThis[k] : globalThis[k] = v;
    State.tileData = state.tiles; State.WORLD_ID = state.world; State.dirty = state.dirty;
  });
  const error = { style: {}, textContent: '' }, calls = []; let reopened = 0;
  globalThis.document = { getElementById: id => id.startsWith('place-name-err-') ? error : id.startsWith('place-name-') ? { value: ' Alpheios ' } : null };
  globalThis.window = { reopenSelectedHex: () => reopened++ };
  globalThis.localStorage = { getItem: () => 'token' };
  globalThis.fetch = async (url, opts) => { calls.push({url,opts}); return responses.shift(); };
  State.WORLD_ID = 'world'; State.tileData = [{terrain:'fog'}]; State.dirty = false;
  return { error, calls, reopened: () => reopened };
}

test('naming POST uses Bearer then authoritative FOW map before reopening', async t => {
  const tiles = [{terrain:'river',place_kind:'river',place_id:'6,79',place_name:'Alpheios',can_name:false},{terrain:'fog'}];
  const h = harness(t,[new Response('{"name":"Alpheios"}'),new Response(JSON.stringify(tiles))]);
  await namePlace('river','6,79');
  assert.equal(h.calls[0].url, '/api/v1/worlds/world/places/river/6,79/name');
  assert.equal(h.calls[0].opts.headers.Authorization, 'Bearer token');
  assert.equal(h.calls[0].opts.method,'POST');
  assert.deepEqual(JSON.parse(h.calls[0].opts.body),{name:' Alpheios '});
  assert.equal(h.calls[1].url,'/api/v1/worlds/world/map');
  assert.deepEqual(State.tileData,tiles); assert.equal(State.dirty,true); assert.equal(h.reopened(),1);
});

for (const status of [400,403,409]) test(`naming ${status} shows server reason without stale success`, async t => {
  const h = harness(t,[new Response('{"error":"Right already spent"}',{status})]);
  await namePlace('river','6,79');
  assert.equal(h.error.textContent,'Right already spent'); assert.equal(h.calls.length,1); assert.equal(h.reopened(),0);
});

test('place naming notification escapes a unit name before inserting HTML', () => {
  assert.match(notifText('PlaceFirstReached',{unit_name:'<script>',kind:'river',q:1,r:2}),/Your &lt;script&gt;/);
});

test('first-reached WS notice refreshes naming rights mid-route without waiting for arrival', async t => {
  const { initWS } = await import('../ws.js');
  const saved = Object.fromEntries(['WebSocket','location','window','setInterval','setTimeout'].map(k=>[k,globalThis[k]]));
  const lastMsg = State.lastWsMsgAt;
  t.after(()=>{ for(const [k,v] of Object.entries(saved)) v===undefined ? delete globalThis[k] : globalThis[k]=v; State.lastWsMsgAt=lastMsg; });
  let socket,refreshes=0,dispatch;
  globalThis.WebSocket=class { constructor(){socket=this;} };
  globalThis.location={protocol:'http:',host:'localhost'};
  globalThis.setInterval=()=>0; globalThis.setTimeout=()=>0;
  globalThis.window={refreshTiles:()=>refreshes++,addDispatch:x=>dispatch=x,MusicPlayer:{cue(){}}};
  initWS();
  socket.onmessage({data:JSON.stringify({id:'notice',kind:'PlaceFirstReached',payload:{unit_name:'Bronze Guard',kind:'river',place_id:'6,79',q:6,r:80}})});
  assert.equal(refreshes,1,'a mid-route claim immediately updates can_name');
  assert.equal(dispatch.kind,'PlaceFirstReached');
  assert.equal(dispatch.payload.unit_name,'Bronze Guard');
});
