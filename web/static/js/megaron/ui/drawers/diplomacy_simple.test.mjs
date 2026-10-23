import test from 'node:test';
import assert from 'node:assert/strict';
import { State } from '../../state.js';
import * as dip from './diplomacy.js';

// Actual drawer/navigation/dispatch consumers. Only HTTP and minimal DOM are
// faked; this never duplicates a directory builder or a trade request builder.
async function rig(run) {
  const saved={document:globalThis.document,fetch:globalThis.fetch,localStorage:globalThis.localStorage,setTimeout:globalThis.setTimeout};
  const nodes=new Map(),calls=[];
  const cls={add(){},remove(){},toggle(){}};
  function node(id){if(!nodes.has(id))nodes.set(id,{id,_html:'',get innerHTML(){return this._html;},set innerHTML(html){this._html=html;for(const [,field] of html.matchAll(/<textarea id="([^"]+)"/g))node(field).value='';},insertAdjacentHTML(_position,html){this._html+=html;},style:{},dataset:{},classList:cls,value:'',querySelector:()=>null,querySelectorAll:()=>[],addEventListener(){},click(){this.listener?.call(this);}});return nodes.get(id);}
  const body=node('diplomacy-body');
  body.querySelectorAll=selector=>selector==='.dtab'?[...body.innerHTML.matchAll(/data-tab="([^"]+)"/g)].map(([,name])=>{const t=node('tab-'+name);t.dataset.tab=name;t.addEventListener=(_,fn)=>t.listener=fn;return t;}):selector==='.city-tab'?[...body.innerHTML.matchAll(/id="(dtab-[^"]+)"/g)].map(([,id])=>node(id)):[];
  globalThis.document={getElementById:id=>id==='net-status'?null:node(id),querySelector:()=>({value:'buy'})};
  globalThis.localStorage={getItem:()=>null};globalThis.setTimeout=()=>0;
  State.WORLD_ID='world';State.MY_SETTLEMENT_ID='origin';State.provinceData=[{id:'province',name:'Kyme',settlement_id:'contact',own:false,owner:'Alector'},{id:'own-province',settlement_id:'origin',name:'Nostos',own:true,is_capital:true},{settlement_id:'outpost',name:'Outpost',is_outpost:true},{settlement_id:'unnamed'}];
  const cities=[{name:'Kyme',owner:'Alector',owner_id:'ruler',settlement_id:'contact',knowledge:'known',q:3,r:4,copper_deposit:true},{name:'Rumour city',owner:'Alector',owner_id:'ruler',settlement_id:'rumour',knowledge:'rumour',bearing:'~15 hexes east of Nostos',industry_hint:'tin'}, {name:'Nostos',owner:'Me',own:true,settlement_id:'origin',knowledge:'known'}];
  const rulers=[{owner:'Alector',owner_id:'ruler',known_cities:1,rumour_cities:1}];
  globalThis.fetch=async(url,opts={})=>{calls.push({url,opts});let data=url.includes('/messengers/queue?')?{items:[]}:[];if(url.endsWith('/cities'))data=cities;else if(url.endsWith('/diplomacy'))data=rulers;else if(url.endsWith('/goods'))data=[{key:'grain',name:'Grain'}];else if(opts.method==='POST')data={id:'letter',arrives_at:new Date(Date.now()+60000).toISOString()};return new Response(JSON.stringify(data),{status:opts.method==='POST'?201:200});};
  try{await run({body,node,calls,cities,rulers});}finally{Object.assign(globalThis,saved);State.provinceData=[];State.MY_SETTLEMENT_ID=null;State.WORLD_ID=null;}
}

test('M: actual drawer opens Queue and retains Correspondence, Known and Runners',()=>rig(async({body})=>{
  await dip.loadDiplomacyDrawer();
  assert.deepEqual([...body.innerHTML.matchAll(/data-tab="([^"]+)"/g)].map(m=>m[1]),['queue','threads','known','runners']);
  assert.doesNotMatch(body.innerHTML,/Compose|Cities|Rulers/);
}));

test('M: Known keeps ruler and city knowledge, gates rumours',()=>rig(async({node})=>{
  await dip.loadDiplomacyDrawer();
  assert.ok(node('tab-known').listener,'real Known click registered');
  await node('tab-known').listener.call(node('tab-known'));
  const html=node('dtab-known').innerHTML;
  assert.match(html,/Alector/);assert.match(html,/Kyme/);assert.match(html,/Rumour city/);
  assert.match(html,/1 known city/);assert.match(html,/1 rumoured city/);
  assert.match(html,/data-write="contact"/);assert.doesNotMatch(html,/data-write="rumour"/);
  assert.match(html,/Copper/);assert.match(html,/east/);assert.match(html,/tin/);
}));

test('M: first letter and both trade directions remain reachable without any prior letter',()=>rig(async({node,calls})=>{
  await dip.loadDiplomacyDrawer();assert.equal(typeof dip.dipWrite,'function');
  await dip.dipWrite('contact');
  const html=node('dtab-threads').innerHTML;
  assert.match(html,/Kyme/);assert.match(html,/data-open/);assert.match(html,/Attach trade offer/);assert.doesNotMatch(html,/NaN|Invalid Date/);
  const cid='dip-thread-Kyme-compose';
  assert.match(html,new RegExp('id="'+cid+'-text"'));
  node(cid+'-text').value='First letter';await dip.dipSendInThread(cid,'contact');
  let posts=calls.filter(c=>c.opts.method==='POST');assert.deepEqual(JSON.parse(posts.at(-1).opts.body),{destination_id:'contact',message:'First letter'});
  node(cid+'-text').value='Buy grain';node(cid+'-good').value='grain';node(cid+'-qty').value='1';node(cid+'-silver').value='2';await dip.dipSendInThread(cid,'contact');
  posts=calls.filter(c=>c.opts.method==='POST');assert.deepEqual(JSON.parse(posts.at(-1).opts.body).trade_offer,{kind:'buy',want_good:'grain',want_qty:1,offer_silver:2});
  globalThis.document.querySelector=()=>({value:'sell'});node(cid+'-text').value='Sell grain';node(cid+'-offer-good').value='grain';node(cid+'-offer-qty').value='3';node(cid+'-want-silver').value='4';await dip.dipSendInThread(cid,'contact');
  posts=calls.filter(c=>c.opts.method==='POST');assert.deepEqual(JSON.parse(posts.at(-1).opts.body).trade_offer,{kind:'sell',offer_good:'grain',offer_qty:3,want_silver:4});
  assert.ok(posts.every(c=>c.url.endsWith('/settlements/origin/messengers')));
}));

test('M: selecting rumour, own city or a removed contact creates no draft or dispatch',()=>rig(async({node,calls})=>{
  await dip.loadDiplomacyDrawer();assert.equal(typeof dip.dipWrite,'function');
  const before=node('dtab-threads').innerHTML;
  for(const id of ['rumour','origin','outpost','unnamed','missing'])await dip.dipWrite(id);
  assert.equal(node('dtab-threads').innerHTML,before);assert.equal(calls.filter(c=>c.opts.method==='POST').length,0);
}));


test('M: host still reads both tabs but does not gain a settlement-only trade composer',()=>rig(async({node})=>{
  State.MY_SETTLEMENT_ID=null;
  await dip.loadDiplomacyDrawer();await node('tab-known').listener.call(node('tab-known'));
  assert.match(node('dtab-known').innerHTML,/Kyme/);assert.doesNotMatch(node('dtab-known').innerHTML,/data-write=/);
  await dip.dipWrite('contact');assert.doesNotMatch(node('dtab-threads').innerHTML,/dip-inline-compose/);
}));

test('M: failed Known fetch offers retry without inventing an empty directory',()=>rig(async({node})=>{
  await dip.loadDiplomacyDrawer();
  const real=globalThis.fetch;
  globalThis.fetch=async url=>url.endsWith('/cities')?new Response('{}',{status:403}):real(url);
  await node('tab-known').listener.call(node('tab-known'));
  assert.match(node('dtab-known').innerHTML,/Could not load/);assert.equal(node('dtab-known').dataset.loaded,undefined);
  globalThis.fetch=real;await node('tab-known').listener.call(node('tab-known'));
  assert.match(node('dtab-known').innerHTML,/Kyme/);
}));

test('M: Write clears a stale composer while the chosen conversation loads',()=>rig(async({node})=>{
  await dip.loadDiplomacyDrawer();await dip.dipWrite('contact');
  assert.match(node('dtab-threads').innerHTML,/dip-inline-compose/);
  const real=globalThis.fetch;let release;
  globalThis.fetch=async(url,opts)=>url.endsWith('/inbox?history=true')?await new Promise(resolve=>{release=()=>resolve(new Response('[]',{status:200}));}):real(url,opts);
  const pending=dip.dipWrite('contact');
  try {assert.doesNotMatch(node('dtab-threads').innerHTML,/dip-inline-compose/);assert.match(node('dtab-threads').innerHTML,/Loading/);}
  finally {await new Promise(resolve=>setImmediate(resolve));release();await pending;}
  assert.match(node('dtab-threads').innerHTML,/dip-inline-compose/);
}));


test('M: late unavailable-actions response preserves a typed first letter',()=>rig(async({node})=>{
  await dip.loadDiplomacyDrawer();
  const real=globalThis.fetch;let release;
  globalThis.fetch=async(url,opts)=>url.endsWith('/actions')?await new Promise(resolve=>{release=()=>resolve(new Response(JSON.stringify([{category:'diplomacy',available:false,name:'reply',requirements:[]}]),{status:200}));}):real(url,opts);
  const pending=dip.dipWrite('contact');
  for(let i=0;i<20&&!release;i++)await new Promise(resolve=>setImmediate(resolve));
  assert.ok(release,'actual composer reaches the delayed capability request');
  const text=node('dip-thread-Kyme-compose-text');text.value='Keep these words while capabilities load.';
  release();await pending;
  assert.equal(text.value,'Keep these words while capabilities load.');
  assert.match(node('dtab-threads').innerHTML,/Unavailable:.*reply/);
}));


test('received history stays in the actual drawer after departure without dead reply or offer controls',()=>rig(async({node,calls})=>{
 const real=globalThis.fetch;
 globalThis.fetch=async(url,opts)=>url.endsWith('/inbox?history=true')?new Response(JSON.stringify([
  {id:'old',from_name:'Kyme',from_id:'contact',status:'arrived',message:'An old received letter',reply_text:'My old answer',sent_at:'2026-10-01T09:00:00Z',arrived_at:'2026-10-01T09:01:00Z',read_at:'already'},
  {id:'offer',from_name:'Kyme',from_id:'contact',status:'arrived',message:'The previous bargain',trade_offer:{status:'declined'},sent_at:'2026-10-01T09:00:00Z',arrived_at:'2026-10-01T09:01:00Z',read_at:'already'}
 ])):real(url,opts);
 await dip.loadDiplomacyDrawer();await dip.switchDiplomacyTab('threads');
 const html=node('dtab-threads').innerHTML;
 assert.match(html,/An old received letter/);assert.match(html,/My old answer/);assert.match(html,/declined/);
 assert.doesNotMatch(html,/dip-reply-row-old|dipAccept\('offer'|dipDecline\('offer'/);
}));

test('Queue mount does not fetch legacy history; actual Correspondence click loads once and tab changes preserve a written draft',()=>rig(async({node,calls})=>{
 await dip.loadDiplomacyDrawer();
 assert.ok(calls.some(c=>c.url.includes('/messengers/queue?')));
 assert.ok(!calls.some(c=>c.url.includes('/inbox?history=true')),'hidden old history must stay lazy');
 await node('tab-threads').listener.call(node('tab-threads'));
 assert.equal(calls.filter(c=>c.url.includes('/inbox?history=true')).length,1);
 await node('tab-queue').listener.call(node('tab-queue'));
 await node('tab-threads').listener.call(node('tab-threads'));
 assert.equal(calls.filter(c=>c.url.includes('/inbox?history=true')).length,1,'same mounted tab should not reload');
 await dip.dipWrite('contact');const text=node('dip-thread-Kyme-compose-text');text.value='Keep my unsent letter';
 const reads=calls.filter(c=>c.url.includes('/inbox?history=true')).length;
 await node('tab-queue').listener.call(node('tab-queue'));await node('tab-threads').listener.call(node('tab-threads'));
 assert.equal(text.value,'Keep my unsent letter');assert.equal(calls.filter(c=>c.url.includes('/inbox?history=true')).length,reads);
}));

test('a late lazy history request cannot replace a newer Write composer or typed draft',()=>rig(async({node})=>{
 await dip.loadDiplomacyDrawer();const real=globalThis.fetch;let release,history=0;
 globalThis.fetch=async(url,opts)=>{
  if(url.endsWith('/inbox?history=true')&&history++===0)return new Promise(resolve=>{release=()=>resolve(new Response(JSON.stringify([{id:'stale',from_name:'Old counterpart',from_id:'stale-city',message:'Stale response'}])));});
  return real(url,opts);
 };
 const older=node('tab-threads').listener.call(node('tab-threads'));
 for(let i=0;i<20&&!release;i++)await new Promise(resolve=>setImmediate(resolve));assert.ok(release);
 await dip.dipWrite('contact');const text=node('dip-thread-Kyme-compose-text');text.value='My current draft';
 release();await older;assert.equal(text.value,'My current draft');assert.match(node('dtab-threads').innerHTML,/dip-thread-Kyme/);assert.doesNotMatch(node('dtab-threads').innerHTML,/Stale response/);
}));
