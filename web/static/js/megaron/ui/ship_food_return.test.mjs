import test from 'node:test';
import assert from 'node:assert/strict';
import { notifText, notifDomain } from './format.js';
import { resolveDestination } from './dispatch_window.js';
import { State } from '../state.js';

test('ShipCameHomeForFood names the homecoming and preserves historical starving notice',()=>{
 const text=notifText('ShipCameHomeForFood',{unit_id:'ship',name:'Nike',crew_after:20,reason:'food_for_way_home'});
 assert.equal(text,'Nike came home early — food for the way back only (20 crew home)');
 assert.equal(notifDomain('ShipCameHomeForFood'),'war');
 assert.doesNotMatch(text,/turning|sailing slower|half strength|arrives/);
 assert.match(notifText('UnitReturnedStarving',{name:'Nike',crew_after:4}),/starved to half strength.*turning home on its own/);
});
test('food homecoming uses the home hex or the returned ship for navigation',()=>{
 const saved=State.unitsData;
 try {State.unitsData=[{id:'ship',q:3,r:4}];assert.deepEqual(resolveDestination('ShipCameHomeForFood',{unit_id:'ship'}),{q:3,r:4});assert.deepEqual(resolveDestination('ShipCameHomeForFood',{unit_id:'ship',q:5,r:6}),{q:5,r:6})}finally{State.unitsData=saved}
});
