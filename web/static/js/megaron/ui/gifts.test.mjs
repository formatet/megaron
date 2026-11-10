import {readFileSync} from 'node:fs';
import test from 'node:test';
import assert from 'node:assert/strict';
import {notifText, notifDomain} from './format.js';
import {State} from '../state.js';
const b={sender_id:'a',recipient_id:'b',origin_name:'Kyme',destination_name:'<Nostos>',origin_id:'origin',destination_id:'dest',transport_id:'cargo',good_key:'silver',quantity:50,credited_quantity:5.125,lost_quantity:44.875,reason:'storage_full',owner_changed:true,actual_recipient_name:'New Wanax',recipient_name:'Old Wanax'};
test('gift notices preserve exact credited and lost quantities and changed owner',()=>{
 const text=notifText('GiftDelivered',b);
 for(const word of ['5.125 silver received','44.875 lost','New Wanax','Old Wanax','storage full']) assert.ok(text.includes(word),word);
 assert.match(notifText('GiftLost',{...b,credited_quantity:0,lost_quantity:50}),/Gift lost/);
 assert.match(notifText('GiftLost',{...b,credited_quantity:0,lost_quantity:25,returned_quantity:25}),/25 sent home aboard the damaged ship/);
 assert.equal(notifDomain('GiftDelivered'),'trade');
});

test('gift formatter reads returned_quantity from a real PostgreSQL/scan/arrival payload',()=>{
 const payload=JSON.parse(readFileSync(new URL('../../../../../docs/reviews/gava/payload-limped.json',import.meta.url),'utf8'));
 assert.equal(payload.returned_quantity,25);assert.equal(payload.lost_quantity,25);assert.equal(payload.credited_quantity,0);
 const text=notifText('GiftLost',payload);assert.match(text,/25 sent home/);assert.match(text,/25 lost/);assert.match(text,/0 silver received/);
});

test('gift text names Wanax and never exposes a sender city',()=>{const text=notifText('GiftDelivered',{...b,sender_name:'Myrine',recipient_name:'Leandros'});assert.match(text,/Myrine → Leandros/);assert.doesNotMatch(text,/Kyme|Nostos/)});
