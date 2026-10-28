import test from 'node:test';
import assert from 'node:assert/strict';
import { State } from '../../state.js';
import * as dip from './diplomacy.js';

const person={counterpart_id:'myrine',counterpart_name:'Myrine',preview:'The last line',state:'waiting_you'};
const letter={id:'incoming',sender_name:'Myrine',direction:'incoming',message:'A <letter>',sent_at:'2026-10-24T08:00:00Z',actions:['reply'],reply_before:'2099-10-24T23:18:00Z'};
async function rig(run){
 const saved={document:globalThis.document,fetch:globalThis.fetch,localStorage:globalThis.localStorage,setTimeout:globalThis.setTimeout};const nodes=new Map(),calls=[];
 function node(id){if(!nodes.has(id))nodes.set(id,{id,_html:'',get innerHTML(){return this._html},set innerHTML(html){this._html=html;for(const [,field] of html.matchAll(/<textarea id="([^"]+)"/g))node(field).value=''},style:{},dataset:{},value:'',querySelector:()=>null,querySelectorAll:()=>[],addEventListener(){},classList:{toggle(){}}});return nodes.get(id)}
 const body=node('diplomacy-body');globalThis.document={getElementById:id=>node(id),querySelector:()=>({value:'buy'})};globalThis.localStorage={getItem:()=>null};globalThis.setTimeout=()=>0;
 State.WORLD_ID='world';State.MY_SETTLEMENT_ID='home';State.provinceData=[{settlement_id:'contact',name:'Kyme',owner:'Myrine',owner_id:'myrine'},{own:true,settlement_id:'home',name:'Nostos'},{is_outpost:true,settlement_id:'outpost',name:'Outpost'}];
 globalThis.fetch=async(url,opts={})=>{calls.push({url,opts});let data={items:[person]};if(url.endsWith('/letters/myrine'))data={counterpart_name:'Myrine',items:[letter]};else if(url.endsWith('/goods'))data=[{key:'grain',name:'Grain'}];else if(opts.method==='POST')data={arrives_at:'2099-01-01T00:00:00Z'};return new Response(JSON.stringify(data),{status:200})};
 try{await run({body,node,calls})}finally{Object.assign(globalThis,saved);State.provinceData=[];State.WORLD_ID=null;State.MY_SETTLEMENT_ID=null}
}
test('drawer has exactly Letters and Runners and one state per Wanax',()=>rig(async({body,node,calls})=>{await dip.loadDiplomacyDrawer();assert.deepEqual([...body.innerHTML.matchAll(/data-tab="([^"]+)"/g)].map(m=>m[1]),['letters','runners']);assert.match(node('dtab-letters').innerHTML,/Myrine/);assert.match(node('dtab-letters').innerHTML,/Waiting on you/);assert.match(node('dtab-letters').innerHTML,/The last line/);assert.ok(calls.some(c=>c.url.endsWith('/letters')));assert.doesNotMatch(node('dtab-letters').innerHTML,/Refresh|Search|case|archive/)}));
test('person exchange opens newest letter and places same-runner reply below it',()=>rig(async({node,calls})=>{await dip.loadDiplomacyDrawer();await dip.dipToggleThread('dip-thread-myrine');const html=node('dtab-letters').innerHTML;assert.match(html,/A &lt;letter&gt;/);assert.match(html,/data-letter="incoming" open/);assert.match(html,/Send with their runner \(leaves/);assert.ok(html.indexOf('id="letter-compose"')>html.indexOf('A &lt;letter&gt;'));assert.ok(calls.some(c=>c.url.endsWith('/letters/myrine')))}));
test('first letter and both trade directions use same dispatch endpoint without subjects',()=>rig(async({node,calls})=>{await dip.loadDiplomacyDrawer();await dip.dipWrite('contact');assert.match(node('dtab-letters').innerHTML,/Send a runner/);node('letter-compose-text').value='First';await dip.dipSendInThread('letter-compose','contact');let p=calls.filter(c=>c.opts.method==='POST').at(-1);assert.deepEqual(JSON.parse(p.opts.body),{destination_id:'contact',message:'First'});node('letter-compose-text').value='Buy';node('letter-compose-good').value='grain';node('letter-compose-qty').value='1';node('letter-compose-silver').value='2';await dip.dipSendInThread('letter-compose','contact');p=calls.filter(c=>c.opts.method==='POST').at(-1);assert.deepEqual(JSON.parse(p.opts.body).trade_offer,{kind:'buy',want_good:'grain',want_qty:1,offer_silver:2});globalThis.document.querySelector=()=>({value:'sell'});node('letter-compose-text').value='Sell';node('letter-compose-offer-good').value='grain';node('letter-compose-offer-qty').value='3';node('letter-compose-want-silver').value='4';await dip.dipSendInThread('letter-compose','contact');p=calls.filter(c=>c.opts.method==='POST').at(-1);assert.deepEqual(JSON.parse(p.opts.body).trade_offer,{kind:'sell',offer_good:'grain',offer_qty:3,want_silver:4})}));
test('unreachable destination does not open composer or dispatch',()=>rig(async({node,calls})=>{await dip.loadDiplomacyDrawer();const before=node('dtab-letters').innerHTML;for(const id of ['rumour','home','outpost','missing'])await dip.dipWrite(id);assert.equal(node('dtab-letters').innerHTML,before);assert.equal(calls.filter(c=>c.opts.method==='POST').length,0)}));
test('late response cannot overwrite a replacement world',()=>rig(async({node})=>{let resolve;globalThis.fetch=()=>new Promise(r=>{resolve=r});const pending=dip.loadDiplomacyDrawer();State.WORLD_ID='new-world';node('dtab-letters').innerHTML='new world';resolve(new Response(JSON.stringify({items:[person]})));await pending;assert.equal(node('dtab-letters').innerHTML,'new world')}));
test('state words distinguish whose turn it is',()=>{assert.equal(dip.lettersState('waiting_you'),'Waiting on you');assert.equal(dip.lettersState('waiting_them'),'Waiting on them');assert.equal(dip.lettersState('done'),'Done')});
test('accepted terms persist, only supplied actions render, gifts carry no trade controls',()=>{const html=dip.lettersThreadHTML({counterpart_name:'Myrine',items:[{...letter,id:'old',actions:[],trade_offer:{kind:'sell',offer_qty:2,offer_good:'copper',want_silver:5,status:'accepted'},offer_state:'accepted'},{...letter,id:'gift',kind:'gift',message:'Gift received',goods:{silver:5.125},actions:[]}]});assert.match(html,/Agreed: 2 copper for 5 silver/);assert.match(html,/Gift received: 5.125 silver/);assert.doesNotMatch(html,/dipAccept|dipDecline/);assert.match(html,/data-letter="gift" open/)});
test('expired windows suppress reply and trade actions without a refresh button',()=>{assert.deepEqual(dip.letterActions({...letter,reply_window_closed:true}),[]);assert.deepEqual(dip.letterActions({actions:['trade_accept','trade_decline'],expires_at:'2000-01-01T00:00:00Z'}),[])});

test('local draft survives server failure, a retry and successful dispatch clears saved copy',()=>rig(async({node})=>{
 await dip.loadDiplomacyDrawer();await dip.dipWrite('contact');
 const compose=node('letter-compose');compose.querySelectorAll=()=>[node('letter-compose-text')];
 node('letter-compose-text').value='Keep this unsent letter';
 await dip.refreshDiplomacyLetters();assert.equal(node('letter-compose-text').value,'Keep this unsent letter');
 const real=globalThis.fetch;globalThis.fetch=async(url,opts)=>opts?.method==='POST'?new Response(JSON.stringify({error:'Route is closed'}),{status:409}):real(url,opts);
 await dip.dipSendInThread('letter-compose','contact');assert.equal(node('letter-compose-text').value,'Keep this unsent letter');assert.match(node('letter-compose-status').textContent,/Route is closed/);
 globalThis.fetch=real;await dip.dipSendInThread('letter-compose','contact');await dip.refreshDiplomacyLetters();assert.equal(node('letter-compose-text').value,'');
}));
test('an unavailable thread retains a local reply when the server recovers',()=>rig(async({node})=>{
 await dip.loadDiplomacyDrawer();await dip.dipToggleThread('dip-thread-myrine');node('letter-compose').querySelectorAll=()=>[node('dip-reply-incoming')];node('dip-reply-incoming').value='Do not lose this reply';
 const real=globalThis.fetch;globalThis.fetch=async()=>new Response('{}',{status:403});await dip.refreshDiplomacyLetters();assert.match(node('dtab-letters').innerHTML,/Could not load letters/);
 globalThis.fetch=real;await dip.refreshDiplomacyLetters();assert.equal(node('dip-reply-incoming').value,'Do not lose this reply');
}));

test('trade offer sends selected goods mode and cannot auto-fallback from unavailable choice',()=>rig(async({node,calls})=>{
 await dip.loadDiplomacyDrawer();await dip.dipWrite('contact');node('letter-compose-text').value='Sea trade';node('letter-compose-good').value='grain';node('letter-compose-qty').value='1';node('letter-compose-silver').value='2';
 const picker=node('letter-compose-route');picker.hasAttribute=()=>true;picker.querySelector=()=>({value:'sea'});
 await dip.dipSendInThread('letter-compose','contact');assert.equal(JSON.parse(calls.filter(c=>c.opts.method==='POST').at(-1).opts.body).trade_offer.by,'sea');
 calls.length=0;node('letter-compose-text').value='Keep';picker.querySelector=()=>null;await dip.dipSendInThread('letter-compose','contact');assert.equal(calls.filter(c=>c.opts.method==='POST').length,0);assert.match(node('letter-compose-status').textContent,/Choose an available/);
}));
