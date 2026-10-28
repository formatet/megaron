import { State as timeTestState } from './../state.js';
import { fmtTickTime as testTickTime } from './fmt_clock.js';
timeTestState.CURRENT_TICK=0; timeTestState.TICK_SECONDS=60; timeTestState.TICK_ANCHOR_MS=Date.now();
import test from 'node:test';
import assert from 'node:assert/strict';
import { notificationDay, urgentNotifications } from './notification_state.js';
import { notifText, notifIcon, notifDomain } from './format.js';

test('recorded game dates include zero, prefer envelope, and never infer historical dates', () => {
  assert.equal(notificationDay({world_tick:0,body:{world_tick:9}}),testTickTime(0));
  assert.equal(notificationDay({world_tick:12,body:{world_tick:9}}),testTickTime(12));
  assert.equal(notificationDay({world_tick:null,body:'{"world_tick":0}'}),testTickTime(0));
  for (const body of [{}, {world_tick:null}, {world_tick:-1}, {world_tick:'7'}])
    assert.equal(notificationDay({created_at:'2026-10-18T09:00:00Z',body}),'time unavailable');
});

test('garrison hunger notices tell the player grain is needed, recovery never claims all are fed', () => {
  const hungry=notifText('GarrisonStarvationStarted',{name:'<Coast>',food_net_per_tick:24});
  assert.match(hungry,/&lt;Coast&gt;.*losing men.*Soldiers need grain.*food surplus.*Place citizens on grain or transfer grain/);
  const recovered=notifText('GarrisonStarvationEnded',{name:'Coast'});
  assert.match(recovered,/no longer losing men to hunger/);
  assert.doesNotMatch(recovered,/everyone|all.*fed/);
  for (const kind of ['GarrisonStarvationStarted','GarrisonStarvationEnded']) {
    assert.equal(notifDomain(kind),'war');assert.equal(notifIcon(kind),'🌾');
  }
  assert.equal(notifDomain('KharisBandChanged'),'kult');
  assert.match(notifText('KharisBandChanged',{from_band:'Indifferent',to_band:'Wrathful',kharis:3}),/Indifferent → Wrathful.*3.*Check Kult/);
});

test('latest recorded transition clears hunger urgency, restart restores it for that city only', () => {
  const event=(kind,tick,city)=>({kind,world_tick:tick,body:{settlement_id:city}});
  const old=event('GarrisonStarvationStarted',1,'a'), recovered=event('GarrisonStarvationEnded',2,'a'), other=event('GarrisonStarvationStarted',1,'b');
  const urgency=urgentNotifications([old,recovered,other]);
  assert.ok(!urgency.has(old));assert.ok(urgency.has(other));assert.ok(!urgency.has(recovered));
  const restarted=event('GarrisonStarvationStarted',3,'a');
  assert.ok(urgentNotifications([old,recovered,restarted]).has(restarted));
});

test('civilian hunger and garrison hunger clear independently in the same city', () => {
  const event=(kind,tick,city='a')=>({kind,world_tick:tick,body:{settlement_id:city}});
  const civil=event('CivilianStarvationStarted',1), army=event('GarrisonStarvationStarted',2);
  const civilEnd=event('CivilianStarvationEnded',3), armyEnd=event('GarrisonStarvationEnded',4);
  let urgent=urgentNotifications([civil,army,civilEnd]);
  assert.ok(urgent.has(army));assert.ok(!urgent.has(civil));
  urgent=urgentNotifications([civil,army,armyEnd]);
  assert.ok(urgent.has(civil));assert.ok(!urgent.has(army));
  const restart=event('CivilianStarvationStarted',5);
  assert.ok(urgentNotifications([civil,civilEnd,armyEnd,restart]).has(restart));
});

test('civilian hunger prose reports the actual meal, and ownership closure never promises recovery', () => {
  const text=notifText('CivilianStarvationStarted',{name:'<Coast>',unmet:0.03});
  assert.match(text,/&lt;Coast&gt;.*going hungry.*&lt;0.1 grain.*grain or fish.*transfer food/);
  assert.match(notifText('CivilianStarvationEnded',{name:'Coast'}),/people were fed again/);
  const closed=notifText('CivilianStarvationEnded',{name:'Coast',reason:'no_longer_owned_or_active'});
  assert.match(closed,/warning closed/);assert.doesNotMatch(closed,/fed again/);
  for(const kind of ['CivilianStarvationStarted','CivilianStarvationEnded']) {
    assert.equal(notifDomain(kind),'city');assert.equal(notifIcon(kind),'🍽');
  }
  assert.match(notifText('FoodShortfall',{name:'Oldtown',unmet:4}),/Oldtown went hungry today — 4 grain unmet/);
});

test('equal food: new notices say food while historical grain outcomes retain meaning',()=>{
 const start=notifText('GarrisonStarvationStarted',{name:'Coast',food_category:true});
 assert.match(start,/Soldiers share the city's grain, fish and oil\./);
 assert.doesNotMatch(start,/need grain even when/);
 assert.match(notifText('GarrisonStarvationStarted',{name:'Coast'}),/Soldiers need grain even when/);
 assert.match(notifText('CivilianStarvationStarted',{unmet:0.03,food_category:true}),/&lt;0.1 food.*grain, fish or oil,/);
 assert.match(notifText('CivilianStarvationStarted',{unmet:0.03}),/&lt;0.1 grain/);
 assert.match(notifText('UnitAttrition',{reason:'food_shortage',disbanded:true}),/no food/);
 assert.match(notifText('UnitAttrition',{reason:'grain_shortage',disbanded:true}),/no grain/);
 for(const reason of ['food_shortage','grain_shortage'])assert.match(notifText('UnitLostAtSea',{reason,lost:5}),/ship starved.*5 men gone/);
});
