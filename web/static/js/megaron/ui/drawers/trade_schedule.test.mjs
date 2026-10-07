import test from 'node:test';
import assert from 'node:assert/strict';
import { tradeScheduleText } from './diplomacy.js';

test('accepted trade uses separately priced server ticks without claiming delivery from wall time', () => {
  assert.equal(tradeScheduleText({status:'accepted', goods_arrival_tick:42, silver_arrival_tick:49, goods_arrives_at:'2000-01-01T00:00:00Z'}), ' · goods scheduled tick 42 · silver scheduled tick 49');
  assert.equal(tradeScheduleText({status:'pending', goods_arrival_tick:42}), '');
  assert.equal(tradeScheduleText({status:'accepted'}), '');
});

const { dipAccept } = await import('./diplomacy.js');
const { State } = await import('../../state.js');

test('actual accept action sends one authorized request and renders both authoritative legs with recipient direction', async () => {
  const oldFetch = globalThis.fetch, oldDoc = globalThis.document, oldStorage = globalThis.localStorage;
  const oldWorld = State.WORLD_ID;
  try {
    State.WORLD_ID = 'world';
    globalThis.localStorage = { getItem: () => 'token' };
    for (const kind of ['buy','sell']) {
      const block = { innerHTML: '' }, btn = { disabled: false };
      globalThis.document = { getElementById: id => id === 'dip-trade-offer' ? block : null };
      let calls = 0;
      globalThis.fetch = async (url, opts) => {
        calls++;
        assert.equal(url, '/api/v1/worlds/world/messengers/offer/trade-accept');
        assert.equal(opts.method, 'POST');
        assert.equal(opts.headers.Authorization, 'Bearer token');
        return new Response(JSON.stringify({kind, quantity:3,good_key:'<grain>',silver_paid:2,goods_arrival_tick:42,silver_arrival_tick:49}), {status:200,headers:{'Content-Type':'application/json'}});
      };
      await dipAccept('offer', btn);
      assert.equal(calls, 1);
      assert.match(block.innerHTML, /goods arrive tick 42/);
      assert.match(block.innerHTML, /arrives tick 49/);
      assert.ok(block.innerHTML.includes(kind === 'sell' ? 'incoming' : 'outgoing'));
      assert.ok(block.innerHTML.includes('&lt;grain&gt;'));
    }
  } finally {
    globalThis.fetch=oldFetch;globalThis.document=oldDoc;globalThis.localStorage=oldStorage;State.WORLD_ID=oldWorld;
  }
});
