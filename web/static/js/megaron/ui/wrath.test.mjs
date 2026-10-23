import test from 'node:test';
import assert from 'node:assert/strict';
import {notifText,notifDomain} from './format.js';

test('decline warning shows standing, negative net and duration to 15, and opens Kult',()=>{
 const body={kharis:29.9,net_per_tick:-.2,days_to_15:75,wrath_threshold:15};
 const text=notifText('KharisDeclining',body);
 assert.match(text,/29.9/);assert.match(text,/-0.2/);assert.match(text,/15 in 75 days/);
 assert.match(text,/with or without a temple/);
 assert.equal(notifDomain('KharisDeclining'),'kult');
});
test('new punishment shows protection date; historical payload gets no invented protection',()=>{
 const old={type:'harvest_failure',amount:50,name:'Pylos'};
 assert.doesNotMatch(notifText('DivinePunishment',old),/Protected/);
 assert.match(notifText('DivinePunishment',{...old,protection_until:'2026-10-23T18:30:00Z'}),/Protected from further divine punishment until/);
});

test('temple demolition notice reports committed standing and ceiling', () => {
  const body = {god_id:'asterion', kharis_loss:20, kharis_before:90, kharis_after:70, cap_before:100, cap_after:50};
  const text = notifText('TempleDemolitionWrath', body);
  assert.match(text, /asterion is angered/);
  assert.match(text, /−20 kharis now \(90 → 70/);
  assert.match(text, /100 → 50/);
  assert.equal(notifDomain('TempleDemolitionWrath'), 'kult');
});
