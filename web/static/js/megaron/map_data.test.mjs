import { test } from 'node:test';
import assert from 'node:assert/strict';
import { State } from './state.js';
import { acceptMapTiles, pollMapTiles } from './map_data.js';

function harness(t, responses) {
  const globals = Object.fromEntries(['document','localStorage','fetch'].map(k => [k,globalThis[k]]));
  const state = { tiles: State.tileData, world: State.WORLD_ID, omen: State.omen };
  t.after(() => {
    for (const [k,v] of Object.entries(globals)) v === undefined ? delete globalThis[k] : globalThis[k] = v;
    State.tileData = state.tiles; State.WORLD_ID = state.world; State.omen = state.omen;
  });
  const calls = [];
  globalThis.document = { getElementById: () => null };
  globalThis.localStorage = { getItem: () => 'token' };
  globalThis.fetch = async (url, opts) => { calls.push({url,opts}); return responses.shift(); };
  State.WORLD_ID = 'world'; State.omen = null;
  return calls;
}

test('an unchanged map poll keeps the tile array; a changed one replaces it', async t => {
  const fog = [{q:1,r:2,terrain:'fog'}], seen = [{q:1,r:2,terrain:'plains'}];
  const calls = harness(t, [
    new Response(JSON.stringify(fog), {headers:{ETag:'W/"a"'}}),
    new Response(null, {status:304}),
    new Response(JSON.stringify(seen), {headers:{ETag:'W/"b"'}}),
  ]);
  await pollMapTiles('world');
  assert.equal(calls[0].opts.headers['If-None-Match'], undefined, 'first poll is unconditional');
  const held = State.tileData;
  await pollMapTiles('world');
  assert.equal(calls[1].opts.headers['If-None-Match'], 'W/"a"');
  assert.equal(State.tileData, held, '304 keeps array identity');
  await pollMapTiles('world');
  assert.notEqual(State.tileData, held);
  assert.deepEqual(State.tileData, seen);
});

test('a map accepted outside the poll replaces the held ETag', async t => {
  const calls = harness(t, [new Response('[]', {headers:{ETag:'W/"c"'}})]);
  acceptMapTiles([{q:0,r:0,terrain:'fog'}], 'W/"named"');
  await pollMapTiles('world');
  assert.equal(calls[0].opts.headers['If-None-Match'], 'W/"named"');
  acceptMapTiles([], null);
  State.WORLD_ID = 'other';
  const calls2 = harness(t, [new Response('[]')]);
  await pollMapTiles('other');
  assert.equal(calls2[0].opts.headers['If-None-Match'], undefined, 'no ETag, or another world, asks unconditionally');
});
