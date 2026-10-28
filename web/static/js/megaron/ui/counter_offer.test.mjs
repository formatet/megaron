import test from 'node:test';import assert from 'node:assert/strict';
import {counterOfferDefaults,canCounterOffer,counterScopeMatches} from './counter_offer.js';
test('counter terms reverse the trading role and retain only exact original terms',()=>{
 assert.deepEqual(counterOfferDefaults({kind:'buy',want_good:'tin',want_qty:20,offer_silver:12,by:'land',secret:'no'}),{kind:'sell',offer_good:'tin',offer_qty:20,want_silver:12,by:'land'});
 assert.deepEqual(counterOfferDefaults({kind:'sell',offer_good:'grain',offer_qty:50,want_silver:10,by:'sea'}),{kind:'buy',want_good:'grain',want_qty:50,offer_silver:10,by:'sea'});
 for(const offer of [null,{kind:'sell',offer_good:'tin',offer_qty:0,want_silver:1},{kind:'buy',want_good:'tin',want_qty:1,offer_silver:Infinity}])assert.throws(()=>counterOfferDefaults(offer));
});
test('only the authoritative incoming counter action survives its deadline',()=>{
 const letter={direction:'incoming',actions:['trade_accept','trade_counter'],trade_offer:{kind:'buy'},expires_at:'2026-10-28T20:00:00Z'};
 assert.equal(canCounterOffer(letter,Date.parse('2026-10-28T19:59:00Z')),true);assert.equal(canCounterOffer(letter,Date.parse(letter.expires_at)),false);assert.equal(canCounterOffer({...letter,actions:['trade_accept']}),false);assert.equal(canCounterOffer({...letter,direction:'outgoing'}),false);
});
test('stale counter buttons cannot submit into a replacement world, person or offer',()=>{
 const s={world:'w',player:'p',person:'other',id:'old'};assert.equal(counterScopeMatches(s,'w','p','other','old'),true);
 for(const a of [['new','p','other','old'],['w','new','other','old'],['w','p','new','old'],['w','p','other','new']])assert.equal(counterScopeMatches(s,...a),false);
});
