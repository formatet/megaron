import assert from 'node:assert/strict';
import {routeProgress,routePosition} from '../../../web/static/js/megaron/render/routes.js';
// Reconstructed shortest legal sea path, NOT the overwritten historical route.
const path=[[60,27],[59,27],[59,26],[58,26]];
const actor={route_version:2,dispatch_offset_milli:731,departure_tick:103,arrival_tick:104,departs_at:'2026-10-24T18:10:14.118Z',arrives_at:'2026-10-24T18:11:14Z'};
// Nominal tick 103 begins created_at + 103*60 seconds = 18:09:30.229916Z.
const now=Date.parse(actor.departs_at),anchor=Date.parse('2026-10-24T18:09:30.229916Z');
const tickNow=103+(now-anchor)/60000;
const progress=routeProgress(actor,now,tickNow);
const pos=routePosition(path,progress,(q,r)=>({x:q,y:r}),[200,200,200],1,2);
console.log(JSON.stringify({tickNow,progress,position:pos,wallProgress:routeProgress(actor,now),elapsedMs:0}));
if(process.argv.includes('--assert-departure'))assert.equal(pos.index,0,'new voyage at dispatch must occupy first entered hex, not final hex');
else {assert.equal(pos.index,0);assert.equal(routeProgress(actor,now),0);}
