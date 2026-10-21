import test from 'node:test';import assert from 'node:assert/strict';
import {anchorWorldTime,worldTickNow,movementTickNow,msUntilWorldTick} from './world_time.js';
test('route clock starts at the actual server tick boundary, not response arrival',()=>{
 const state={},at=Date.parse('2026-10-12T07:00:00Z');
 assert.equal(anchorWorldTime({current_tick:10,tick_seconds:6,tick_anchor_at:'2026-10-12T07:00:00Z'},at+3000,state),true);
 assert.equal(worldTickNow(at+3000,state),10.5);assert.equal(movementTickNow(at+3000,state),10.5);
 assert.equal(movementTickNow(at-3000,state),10);assert.equal(movementTickNow(at+12000,state),10.999,'paused worker cannot advance a physical route');
 assert.equal(worldTickNow(at+12000,state),12,'ETA estimation remains continuous');
});
test('unknown tick clock stays explicit; legacy server fallback preserves old anchor behavior',()=>{
 assert.equal(worldTickNow(0,{}),null);const state={};assert.equal(anchorWorldTime({},100,state),false);
 anchorWorldTime({current_tick:1,tick_seconds:6},100,state);assert.equal(state.TICK_ANCHOR_MS,100);
});

test('arrival clock uses the current cadence and anchor rather than an old ISO',()=>{
 const state={CURRENT_TICK:10,TICK_SECONDS:60,TICK_ANCHOR_MS:1000};
 assert.equal(msUntilWorldTick(12,31000,state),90000);
 state.TICK_SECONDS=6;
 assert.equal(msUntilWorldTick(12,4000,state),9000);
 assert.ok(Number.isNaN(msUntilWorldTick(12,0,{})));
});
