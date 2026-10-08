import test from 'node:test';
import assert from 'node:assert/strict';
import {State} from '../../state.js';
import {openCitySettlement} from './economy.js';
test('Economy City link closes Economy before opening the requested province',()=>{
 const calls=[];globalThis.window={closeDrawer:n=>calls.push(['close',n]),openDrawer:n=>calls.push(['open',n,State.cityViewID])};
 openCitySettlement('province-target');assert.deepEqual(calls,[['close','economy'],['open','city','province-target']],'Economy must close before City opens');
});
