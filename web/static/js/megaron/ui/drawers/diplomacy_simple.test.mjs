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
  function node(id){if(!nodes.has(id))nodes.set(id,{id,innerHTML:'',style:{},dataset:{},classList:cls,value:'',querySelectorAll:()=>[],addEventListener(){},click(){this.listener?.call(this);}});return nodes.get(id);}
  const body=node('diplomacy-body');
  body.querySelectorAll=selector=>selector==='.dtab'?[...body.innerHTML.matchAll(/data-tab="([^"]+)"/g)].map(([,name])=>{const t=node('tab-'+name);t.dataset.tab=name;t.addEventListener=(_,fn)=>t.listener=fn;return t;}):selector==='.city-tab'?[...body.innerHTML.matchAll(/id="(dtab-[^"]+)"/g)].map(([,id])=>node(id)):[];
  globalThis.document={getElementById:id=>id==='net-status'?null:node(id),querySelector:()=>({value:'buy'})};
  globalThis.localStorage={getItem:()=>null};globalThis.setTimeout=()=>0;
  State.WORLD_ID='world';State.MY_SETTLEMENT_ID='origin';State.provinceData=[{id:'province',name:'Kyme',settlement_id:'contact',own:false,owner:'Alector'}];
  const cities=[{name:'Kyme',owner:'Alector',owner_id:'ruler',settlement_id:'contact',knowledge:'known',q:3,r:4,copper_deposit:true},{name:'Rumour city',owner:'Alector',owner_id:'ruler',settlement_id:'rumour',knowledge:'rumour',bearing:'east',industry_hint:'tin'}, {name:'Nostos',owner:'Me',own:true,settlement_id:'origin',knowledge:'known'}];
  const rulers=[{owner:'Alector',owner_id:'ruler',known_cities:1,rumour_cities:1}];
  globalThis.fetch=async(url,opts={})=>{calls.push({url,opts});let data=[];if(url.endsWith('/cities'))data=cities;else if(url.endsWith('/diplomacy'))data=rulers;else if(url.endsWith('/goods'))data=[{key:'grain',name:'Grain'}];else if(opts.method==='POST')data={id:'letter',arrives_at:new Date(Date.now()+60000).toISOString()};return new Response(JSON.stringify(data),{status:opts.method==='POST'?201:200});};
  try{await run({body,node,calls,cities,rulers});}finally{Object.assign(globalThis,saved);State.provinceData=[];State.MY_SETTLEMENT_ID=null;State.WORLD_ID=null;}
}

test('M: actual drawer offers exactly Correspondence and Known',()=>rig(async({body})=>{
  await dip.loadDiplomacyDrawer();
  assert.deepEqual([...body.innerHTML.matchAll(/data-tab="([^"]+)"/g)].map(m=>m[1]),['threads','known']);
  assert.doesNotMatch(body.innerHTML,/Compose|Cities|Rulers/);
}));

test('M: Known keeps ruler and city knowledge, writes numbers in words, gates rumours',()=>rig(async({node})=>{
  await dip.loadDiplomacyDrawer();
  assert.ok(node('tab-known').listener,'real Known click registered');
  await node('tab-known').listener.call(node('tab-known'));
  const html=node('dtab-known').innerHTML;
  assert.match(html,/Alector/);assert.match(html,/Kyme/);assert.match(html,/Rumour city/);
  assert.match(html,/one known city/);assert.match(html,/one rumoured city/);
  assert.match(html,/data-write="contact"/);assert.doesNotMatch(html,/data-write="rumour"/);
  assert.match(html,/Copper/);assert.match(html,/east/);assert.match(html,/tin/);
  assert.doesNotMatch(html.replace(/<[^>]*>/g,''),/\d/);
}));

test('M: first letter and both trade directions remain reachable without any prior letter',()=>rig(async({node,calls})=>{
  await dip.loadDiplomacyDrawer();assert.equal(typeof dip.dipWrite,'function');
  await dip.dipWrite('contact');
  const html=node('dtab-threads').innerHTML;
  assert.match(html,/Kyme/);assert.match(html,/data-open/);assert.match(html,/Attach trade offer/);
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
  for(const id of ['rumour','origin','missing'])await dip.dipWrite(id);
  assert.equal(node('dtab-threads').innerHTML,before);assert.equal(calls.filter(c=>c.opts.method==='POST').length,0);
}));
