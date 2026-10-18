import test from 'node:test';
import assert from 'node:assert/strict';
import { notificationDay, urgentNotifications } from './notification_state.js';
import { notifText, notifIcon, notifDomain } from './format.js';

test('recorded game dates include zero, prefer envelope, and never infer historical dates', () => {
  assert.equal(notificationDay({world_tick:0,body:{world_tick:9}}),'Day #0');
  assert.equal(notificationDay({world_tick:12,body:{world_tick:9}}),'Day #12');
  assert.equal(notificationDay({world_tick:null,body:'{"world_tick":0}'}),'Day #0');
  for (const body of [{}, {world_tick:null}, {world_tick:-1}, {world_tick:'7'}])
    assert.equal(notificationDay({created_at:'2026-10-18T09:00:00Z',body}),'Day unknown');
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
