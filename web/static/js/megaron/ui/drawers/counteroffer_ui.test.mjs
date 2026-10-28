import test from 'node:test';import assert from 'node:assert/strict';import {lettersThreadHTML,letterActions} from './diplomacy.js';
const offer={id:'original',direction:'incoming',actions:['trade_accept','trade_decline','trade_counter'],trade_offer:{kind:'buy',want_good:'tin',want_qty:20,offer_silver:10,status:'pending'},sent_at:'2026-10-28T09:00:00Z',expires_at:'2099-10-28T20:00:00Z'};
test('a delivered authoritative offer has a counter action bound to that exact letter',()=>{const html=lettersThreadHTML({counterpart_name:'Myrine',items:[offer]});assert.match(html,/dipCounter\('original'\)/);assert.match(html,/>Counter-offer</);assert.match(html,/id="dip-counter-original"/)});
test('expired and unauthorized offers have no counter affordance',()=>{assert.deepEqual(letterActions({...offer,expires_at:'2000-01-01T00:00:00Z'}),[]);assert.doesNotMatch(lettersThreadHTML({items:[{...offer,actions:[]}]}),/dipCounter/)});
test('a counter-offer is shown as an offer line, not as acceptance',()=>{const html=lettersThreadHTML({items:[{...offer,direction:'outgoing',actions:[],trade_offer:{kind:'sell',offer_good:'tin',offer_qty:20,want_silver:12,status:'pending',counter_offer_id:'original'}}]});assert.match(html,/Counter-offer · Offer: 20 tin for 12 silver/)});

import {State} from '../../state.js';import * as dip from './diplomacy.js';
async function rig(run) {
 const saved={document:globalThis.document,fetch:globalThis.fetch,localStorage:globalThis.localStorage,setTimeout:globalThis.setTimeout};const nodes=new Map(),calls=[];let post;
 const node=id=>{if(!nodes.has(id))nodes.set(id,{id,value:'',textContent:'',innerHTML:'',style:{},hidden:false,disabled:false,classList:{toggle(){}},dataset:{},addEventListener(){},querySelector(){return null},querySelectorAll(){return []}});return nodes.get(id)};
 globalThis.document={getElementById:node,querySelector:()=>null};globalThis.localStorage={getItem:()=>null};globalThis.setTimeout=()=>0;
 globalThis.fetch=async(url,opts={})=>{calls.push({url,opts});if(url.endsWith('/trade-counter'))return post?post():new Response(JSON.stringify({id:'new',arrives_at:'2099-10-28T20:00:00Z'}),{status:201});return new Response(JSON.stringify(url.endsWith('/goods')?[{key:'tin',name:'Tin'}]:url.endsWith('/letters/person')?{counterpart_name:'Myrine',items:[offer]}:{items:[{counterpart_id:'person',counterpart_name:'Myrine'}]}));};
 State.WORLD_ID='world';State.MY_PLAYER_ID='player';State.MY_SETTLEMENT_ID='home';State.provinceData=[];
 try {await dip.loadDiplomacyDrawer();await dip.dipToggleThread('person');dip.dipCounter('original');await run({node,calls,setPost:fn=>post=fn});}finally{Object.assign(globalThis,saved);State.WORLD_ID=null;State.MY_PLAYER_ID=null;State.MY_SETTLEMENT_ID=null;}
}
const posts=calls=>calls.filter(c=>c.opts.method==='POST');
test('one line, roles reversed and prefilled; one POST to the exact original',()=>rig(async({node,calls})=>{
 assert.match(node('dip-counter-original').innerHTML,/Sell <input/);assert.match(node('dip-counter-original').innerHTML,/ tin for /);
 assert.equal(node('dip-counter-qty-original').value,'20');assert.equal(node('dip-counter-silver-original').value,'10');
 node('dip-counter-silver-original').value='12';
 await dip.dipSendCounter('different');assert.equal(posts(calls).length,0);
 await dip.dipSendCounter('original',{});const p=posts(calls);assert.equal(p.length,1);assert.equal(p[0].url,'/api/v1/worlds/world/messengers/original/trade-counter');
 assert.deepEqual(JSON.parse(p[0].opts.body),{message:'Counter-offer.',trade_offer:{kind:'sell',offer_good:'tin',offer_qty:20,want_silver:12}});
 assert.match(node('dip-counter-status-original').textContent,/They must agree to the new terms/);
}));
test('a failed counter says the original still stands and never falls back to another send',()=>rig(async({node,calls,setPost})=>{
 node('dip-counter-silver-original').value='';await dip.dipSendCounter('original');assert.equal(posts(calls).length,0);assert.match(node('dip-counter-status-original').textContent,/Enter a quantity/);
 node('dip-counter-silver-original').value='12';setPost(()=>new Response(JSON.stringify({error:'Insufficient goods'}),{status:422}));await dip.dipSendCounter('original',{});
 assert.equal(posts(calls).length,1);assert.match(node('dip-counter-status-original').textContent,/original offer still stands/);
}));
test('double click dispatches once and a replacement world ignores the late result',()=>rig(async({node,calls,setPost})=>{
 let resolve;setPost(()=>new Promise(r=>resolve=r));const first=dip.dipSendCounter('original',{});await dip.dipSendCounter('original',{});assert.equal(posts(calls).length,1);
 State.WORLD_ID='replacement';node('dip-counter-status-original').textContent='New world';resolve(new Response(JSON.stringify({id:'new',arrives_at:'2099-10-28T20:00:00Z'}),{status:201}));await first;assert.equal(node('dip-counter-status-original').textContent,'New world');
}));
test('Cancel removes the line and its send',()=>rig(async({node,calls})=>{
 dip.dipCancelCounter('original');assert.equal(node('dip-counter-original').innerHTML,'');await dip.dipSendCounter('original',{});assert.equal(posts(calls).length,0);
}));
