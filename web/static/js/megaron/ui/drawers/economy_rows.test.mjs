import test from 'node:test';
import assert from 'node:assert/strict';
import { State } from '../../state.js';
import * as economy from './economy.js';

// Drive the real Send goods form: stub only the browser DOM and HTTP.
async function rig(run) {
  const saved={document:globalThis.document,fetch:globalThis.fetch,localStorage:globalThis.localStorage};
  const handlers={},calls=[];
  const host={innerHTML:'',addEventListener(type,fn){handlers[type]=fn;}};
  host.querySelector=()=>null;
  globalThis.document={getElementById:id=>id==='ectab-send'?host:null,createElement:()=>({set innerHTML(v){this.h=v;},querySelector:()=>null})};
  globalThis.localStorage={getItem:()=>null};
  State.WORLD_ID='world';State.MY_SETTLEMENT_ID='city-a';
  State.provinceData=[{id:'province-a',settlement_id:'city-a',own:true,name:'Phaistos',is_capital:true},{id:'province-b',settlement_id:'city-b',own:true,name:'Kyme'}];
  globalThis.fetch=async(url,opts={})=>{calls.push({url,opts});
    const json=d=>new Response(JSON.stringify(d),{status:opts.method==='POST'?201:200});
    if(url.endsWith('/provinces/province-a/goods'))return json([{key:'grain',name:'Grain',amount:240,shippable:true},{key:'wine',name:'Wine',amount:30,shippable:true}]);
    if(url.endsWith('/provinces/province-b/goods'))return json([{key:'wine',name:'Wine',amount:5,shippable:true}]);
    if(url.endsWith('/trade/destinations'))return json([{settlement_id:'city-b',name:'Kyme',owner_name:'Me',own:true},{settlement_id:'city-g',name:'Gournia',owner_name:'Myrine',own:false}]);
    if(url.includes('/trade/preview'))return json({selected_by:'land',options:[{by:'land',available:true,travel_ticks:3,arrives_at:'2026-01-01T12:00:00Z'},{by:'sea',available:false,reason:'No free ship'}]});
    if(url.endsWith('/standing-orders')&&opts.method==='POST')return json({id:'so1'});
    if(url.endsWith('/trade'))return json({transport_id:'t1',kind:'transfer'});
    return json([]);};
  const click=(act,extra={})=>handlers.click({target:{closest:()=>({dataset:{act,...extra}})}});
  const type=(key,value)=>handlers.input({target:{dataset:{amt:key},value}});
  try{await run({host,calls,click,type,handlers,tick:()=>new Promise(r=>setTimeout(r,20))});}finally{Object.assign(globalThis,saved);State.WORLD_ID=null;State.provinceData=[];State.MY_SETTLEMENT_ID=null;}
}

test('Send goods: preselected foreign destination posts today\'s transfer body (no crew field)',()=>rig(async({host,calls,click,type,tick})=>{
  await economy.openSendGoods('city-g').catch(()=>{});
  const {loadSendGoods}=await import('./send_goods.js');
  await loadSendGoods(State.provinceData,'city-g');
  assert.match(host.innerHTML,/Send as a gift →/);assert.doesNotMatch(host.innerHTML,/<select|Crewed by/);
  type('grain','50');await click('send');await tick();
  const post=calls.find(c=>c.opts.method==='POST');
  assert.equal(post.url,'/api/v1/worlds/world/provinces/province-a/trade');
  assert.equal(post.opts.body,JSON.stringify({destination_id:'city-g',items:[{good_key:'grain',quantity:50}],by:'land'}));
}));

test('Send goods: Keep it stocked posts a standing order crewed by the source city',()=>rig(async({host,calls,click,type,handlers,tick})=>{
  const {loadSendGoods}=await import('./send_goods.js');
  await loadSendGoods(State.provinceData,'city-b');
  await handlers.change({target:{dataset:{act:'keep'},checked:true}});
  assert.match(host.innerHTML,/Keep it stocked →/);
  type('grain','200');await click('send');await tick();
  const post=calls.find(c=>c.opts.method==='POST');
  assert.equal(post.url,'/api/v1/worlds/world/standing-orders');
  assert.equal(post.opts.body,JSON.stringify({from_settlement_id:'city-a',to_settlement_id:'city-b',crewed_by_settlement_id:'city-a',outbound:[{good_key:'grain',threshold:200}],return:[],by:'land'}));
}));

test('cargo rows carry the prayers over the owner\'s own caravan (the Thread)', () => {
  const rows = economy.formatCargoRows([{ id: 't1', good_key: 'grain', quantity: 10, role: 'sender', arrival_tick: 5, arrives_at: '2026-01-01T00:00:00Z', blessings: ['the Thread, 12 game days'] },
    { id: 't2', good_key: 'wine', quantity: 3, role: 'sender', arrival_tick: 6, arrives_at: '2026-01-01T00:00:00Z' }], 0);
  assert.deepEqual(rows.map(r => r.blessings), [['the Thread, 12 game days'], []]);
  const html = economy.renderCargoHTML([{ id: 't1', good_key: 'grain', quantity: 10, role: 'sender', arrival_tick: 5, arrives_at: '2026-01-01T00:00:00Z', blessings: ['the Thread, 12 game days'] }], 0);
  assert.match(html, /the Thread, 12 game days/);
});
