import test from 'node:test';
import assert from 'node:assert/strict';
import { State } from '../../state.js';
import * as economy from './economy.js';

// Exercise real drawer clicks and POST handler. Stub only browser DOM and HTTP.
async function rig(run) {
  const saved={document:globalThis.document,fetch:globalThis.fetch,localStorage:globalThis.localStorage};
  const nodes=new Map(),calls=[];
  function node(id){if(!nodes.has(id))nodes.set(id,{id,value:'',innerHTML:'',textContent:'',insertAdjacentHTML(_position,html){this.innerHTML+=html;},style:{},classList:{add(){},remove(){}},dataset:{},rows:[],querySelectorAll(){return this.rows;},addEventListener(_,fn){this.listener=fn;}});return nodes.get(id);}
  const body=node('economy-body');
  body.querySelectorAll=sel=>sel==='.dtab'?['goods','transfer','automation','wants'].map(name=>{const t=node('tab-'+name);t.dataset.tab=name;return t;}):[];
  globalThis.document={getElementById:id=>id==='net-status'?null:node(id)};
  globalThis.localStorage={getItem:()=>null};
  State.WORLD_ID='world';State.provinceData=[{id:'province-a',settlement_id:'city-a',own:true,name:'Nostos'},{id:'province-b',settlement_id:'city-b',own:true,name:'Kyme'}];
  globalThis.fetch=async(url,opts={})=>{calls.push({url,opts});return new Response(JSON.stringify(url.endsWith('/goods')?[{key:'grain',name:'Grain',amount:0},{key:'silver',name:'Silver',amount:0},{key:'horses',name:'Horses',amount:0}]:[]),{status:opts.method==='POST'?201:200});};
  node('ec-so-from').value='city-a';node('ec-so-to').value='city-b';node('ec-so-crew').value='to';
  function rows(group,pairs){node('ec-so-'+group).rows=pairs.map(([good,amount])=>({querySelector:s=>({value:s==='select'?good:amount})}));}
  try{await run({node,calls,rows,body});}finally{Object.assign(globalThis,saved);State.WORLD_ID=null;State.provinceData=[];}
}

test('N: actual POST preserves multiple outbound goods, return floor zero, decimals and destination crew',()=>rig(async({rows,calls})=>{
  rows('out',[['grain','200'],['fish','50.5']]);rows('home',[['silver','0'],['stone','20']]);
  await economy.createStandingOrder();
  const post=calls.find(c=>c.opts.method==='POST');assert.ok(post,'row inputs must dispatch');
  assert.equal(post.url,'/api/v1/worlds/world/standing-orders');
  assert.deepEqual(JSON.parse(post.opts.body),{from_settlement_id:'city-a',to_settlement_id:'city-b',crewed_by_settlement_id:'city-b',outbound:[{good_key:'grain',threshold:200},{good_key:'fish',threshold:50.5}],return:[{good_key:'silver',floor:0},{good_key:'stone',floor:20}]});
}));

test('N: blank optional rows leave an empty return array and source crew intact',()=>rig(async({node,rows,calls})=>{
  node('ec-so-crew').value='from';rows('out',[['grain','0'],['',''],['fish','']]);rows('home',[['','']]);
  await economy.createStandingOrder();const post=calls.find(c=>c.opts.method==='POST');assert.ok(post,'zero is an entered amount');
  const body=JSON.parse(post.opts.body);assert.equal(body.crewed_by_settlement_id,'city-a');assert.deepEqual(body.outbound,[{good_key:'grain',threshold:0}]);assert.deepEqual(body.return,[]);
}));

test('N: drawer keeps four tabs and crew choices, replaces both CSV inputs with addable goods rows',()=>rig(async({node,body,calls})=>{
  await economy.loadEconomyDrawer();await node('tab-automation').listener.call(node('tab-automation'));
  // Wait for catalogue request and its DOM continuation.
  for(let i=0;i<10;i++)await new Promise(resolve=>setImmediate(resolve));
  assert.deepEqual([...body.innerHTML.matchAll(/data-tab="([^"]+)"/g)].map(m=>m[1]),['goods','transfer','automation','wants']);
  const html=node('ectab-automation').innerHTML;
  assert.match(html,/Crewed by \(which end supplies the gubbe\)/);assert.match(html,/value="from"/);assert.match(html,/value="to"/);
  assert.doesNotMatch(html,/comma-separated|grain:200|silver:0/);
  assert.match(node('ec-so-out').innerHTML,/value="silver"/);assert.match(node('ec-so-out').innerHTML,/value="horses"/);
  assert.match(html,/ec-so-out-add/);assert.match(html,/ec-so-home-add/);
  assert.ok(calls.some(c=>c.url.endsWith('/provinces/province-a/goods')),'own inventory catalogue');
  assert.ok(!calls.some(c=>c.url==='/api/v1/goods'),'offer catalogue would lose silver and parked goods');
}));

test('N: same-city and missing-outbound rows still cannot dispatch',()=>rig(async({node,rows,calls})=>{
  rows('out',[]);rows('home',[['silver','0']]);await economy.createStandingOrder();
  node('ec-so-to').value='city-a';rows('out',[['grain','200']]);await economy.createStandingOrder();
  assert.equal(calls.filter(c=>c.opts.method==='POST').length,0);
}));


test('N: failed goods fetch sends no form, reopening Automation retries full inventory',()=>rig(async({node})=>{
  await economy.loadEconomyDrawer();
  const fetch=globalThis.fetch;globalThis.fetch=async()=>new Response('{}',{status:400});
  await node('tab-automation').listener.call(node('tab-automation'));
  for(let i=0;i<10;i++)await new Promise(resolve=>setImmediate(resolve));
  assert.match(node('ectab-automation').innerHTML,/Could not load goods/);assert.doesNotMatch(node('ectab-automation').innerHTML,/Create route/);
  globalThis.fetch=fetch;await node('tab-automation').listener.call(node('tab-automation'));
  for(let i=0;i<10;i++)await new Promise(resolve=>setImmediate(resolve));
  assert.match(node('ectab-automation').innerHTML,/Create route/);assert.match(node('ec-so-home').innerHTML,/value="silver"/);
}));
