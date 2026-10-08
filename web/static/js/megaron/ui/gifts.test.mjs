import {readFileSync} from 'node:fs';
import test from 'node:test';
import assert from 'node:assert/strict';
import {notifText, notifDomain} from './format.js';
import {State} from '../state.js';
import {loadDiplomacyDrawer} from './drawers/diplomacy.js';
import {loadTransferDestinations} from './drawers/economy.js';
const b={sender_id:'a',recipient_id:'b',origin_name:'Kyme',destination_name:'<Nostos>',origin_id:'origin',destination_id:'dest',transport_id:'cargo',good_key:'silver',quantity:50,credited_quantity:5.125,lost_quantity:44.875,reason:'storage_full',owner_changed:true,actual_recipient_name:'New Wanax',recipient_name:'Old Wanax'};
test('gift notices preserve exact credited and lost quantities and changed owner',()=>{
 const text=notifText('GiftDelivered',b);
 for(const word of ['5.125 silver received','44.875 lost','New Wanax','Old Wanax','storage full']) assert.ok(text.includes(word),word);
 assert.match(notifText('GiftLost',{...b,credited_quantity:0,lost_quantity:50}),/Gift lost/);
 assert.match(notifText('GiftLost',{...b,credited_quantity:0,lost_quantity:25,returned_quantity:25}),/25 sent home aboard the damaged ship/);
 assert.equal(notifDomain('GiftDelivered'),'trade');
});
test('actual Diplomacy drawer places gift among letters with escaped text and no offer controls',async()=>{
 const body={innerHTML:'',querySelectorAll:()=>[]},threads={innerHTML:'',querySelectorAll:()=>[],insertAdjacentHTML(_,html){this.innerHTML+=html;}};
 globalThis.document={getElementById:id=>id==='diplomacy-body'?body:id==='dtab-threads'?threads:null};globalThis.localStorage={getItem:()=>null};
 State.WORLD_ID='world';State.MY_PLAYER_ID='b';State.MY_SETTLEMENT_ID='dest';State.provinceData=[];
 globalThis.fetch=async url=>new Response(JSON.stringify(url.endsWith('/gifts')?[{kind:'GiftDelivered',body:b,created_at:'2026-10-08T10:00:00Z'}]:url.endsWith('/inbox')?[{id:'letter',from_name:'Kyme',from_id:'origin',message:'An ordinary letter',arrived_at:'2026-10-08T09:00:00Z'}]:[]));
 await loadDiplomacyDrawer();assert.match(threads.innerHTML,/An ordinary letter/);assert.match(threads.innerHTML,/5.125 silver received/);assert.match(threads.innerHTML,/&lt;Nostos&gt;/);assert.equal((threads.innerHTML.match(/class="dip-thread"/g)||[]).length,1);
 assert.doesNotMatch(threads.innerHTML,/dip-reply-cargo|dipAccept|dipDecline|dipCancel\('cargo'/);
});
test('actual Transfer destination loader shows foreign owner and protects replacement form from late response',async()=>{
 let sel={innerHTML:''};globalThis.document={getElementById:id=>id==='ec-tr-to'?sel:null};globalThis.localStorage={getItem:()=>null};State.WORLD_ID='world';
 globalThis.fetch=async()=>new Response(JSON.stringify([{settlement_id:'dest',name:'<Nostos>',owner_name:'Wanax <B>',own:false},{settlement_id:'home',name:'Home',own:true}]));
 await loadTransferDestinations('province');assert.match(sel.innerHTML,/Wanax &lt;B&gt; \(gift\)/);assert.match(sel.innerHTML,/your city/);assert.match(sel.innerHTML,/value="dest"/);
 let resolve;globalThis.fetch=()=>new Promise(r=>{resolve=r});const pending=loadTransferDestinations('province');sel={innerHTML:'new form'};resolve(new Response(JSON.stringify([{settlement_id:'old',name:'Old',own:true}])));await pending;assert.equal(sel.innerHTML,'new form');
});

test('gift formatter reads returned_quantity from a real PostgreSQL/scan/arrival payload',()=>{
 const payload=JSON.parse(readFileSync(new URL('../../../../../docs/reviews/gava/payload-limped.json',import.meta.url),'utf8'));
 assert.equal(payload.returned_quantity,25);assert.equal(payload.lost_quantity,25);assert.equal(payload.credited_quantity,0);
 const text=notifText('GiftLost',payload);assert.match(text,/25 sent home/);assert.match(text,/25 lost/);assert.match(text,/0 silver received/);
});
