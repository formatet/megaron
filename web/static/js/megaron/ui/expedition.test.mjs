import test from 'node:test';
import assert from 'node:assert/strict';
import {configureExpedition,expeditionTicks,expeditionOrderText,expeditionMissionText} from './expedition.js';

test('duration follows changed server rules and refuses malformed or unavailable choices',()=>{
 const input={};const rules={min_ticks:6,max_ticks:22,default_ticks:14,area_radius:3};
 configureExpedition(input,rules);
 assert.equal(expeditionTicks(input),14);
 for(const value of ['',5,23,6.5,'bad']) {input.value=value;assert.throws(()=>expeditionTicks(input));}
 for(const value of [6,22]) {input.value=value;assert.equal(expeditionTicks(input),value);}
 assert.match(expeditionOrderText(9,-2,14,rules),/within 3 hexes, for 14 game days/);
 configureExpedition(input,undefined);assert.throws(()=>expeditionTicks(input),/rules unavailable/);
});
test('mission shows authoritative ticks until turn, then its actual return reason',()=>{
 const e={area_q:9,area_r:2,length_ticks:12,turn_tick:108,home_by_tick:114};
 assert.match(expeditionMissionText(e),/turns home by game day 108; home by game day 114/);
 e.homeward=true;e.turn_reason='no_path';
 assert.match(expeditionMissionText(e),/returning home — no reachable unexplored ground; home by game day 114/);
 assert.doesNotMatch(expeditionMissionText(e),/turns home/);
 assert.equal(expeditionMissionText(null),'');
});
