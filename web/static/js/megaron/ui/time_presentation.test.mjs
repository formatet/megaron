import test from 'node:test';
import assert from 'node:assert/strict';
import {State} from '../state.js';
import {fmtTickTime,fmtClock} from './fmt_clock.js';
import {noteServerDate} from '../clock.js';
import {notificationDay} from './notification_state.js';
import {expeditionMissionText} from './expedition.js';

test('absolute world clock: zero, history, midnight, cadence, missing anchor and delivery',t=>{
 const now=new Date(2026,9,21,23,50).getTime();
 t.mock.timers.enable({apis:['Date'],now});
 Object.assign(State,{CURRENT_TICK:100,TICK_SECONDS:60,TICK_ANCHOR_MS:now});
 assert.equal(fmtTickTime(100),'today 23:50');
 assert.equal(fmtTickTime(120),'tomorrow 00:10');
 assert.equal(fmtTickTime(0),'today 22:10');
 assert.equal(notificationDay({world_tick:0,created_at:'2020-01-01T00:00:00Z'}),'today 22:10');
 assert.equal(notificationDay({created_at:'2020-01-01T00:00:00Z'}),'time unavailable');
 assert.equal(fmtClock(now-86400000),'yesterday 23:50');
 assert.match(fmtClock(now-3*86400000),/Sun 23:50/);
 assert.match(fmtClock(new Date(2027,0,1,9,10).getTime()),/2027.*09:10/);
 State.TICK_SECONDS=3600;
 assert.equal(fmtTickTime(102),'tomorrow 01:50');
 const text=expeditionMissionText({area_q:16,area_r:29,length_ticks:30,turn_tick:102,home_by_tick:103});
 assert.equal(text,'Expedition around (16,29), 30 days; turns home by tomorrow 01:50; home by tomorrow 02:50.');
 State.TICK_ANCHOR_MS=null;
 assert.equal(fmtTickTime(0),'time unavailable');
 assert.equal(fmtTickTime(null),'time unavailable');
 assert.equal(fmtTickTime(NaN),'time unavailable');
 assert.equal(fmtTickTime(-1),'time unavailable');
 assert.equal(fmtTickTime(1.5),'time unavailable');
});

test('world clock projects into player clock despite server skew',t=>{
 const now=new Date(2026,9,21,12).getTime();
 t.mock.timers.enable({apis:['Date'],now});
 noteServerDate(new Date(now+3600000-500).toUTCString());
 Object.assign(State,{CURRENT_TICK:100,TICK_SECONDS:60,TICK_ANCHOR_MS:now+3600000});
 assert.equal(fmtTickTime(110),'today 12:10');
 noteServerDate(new Date(now-500).toUTCString());
});
