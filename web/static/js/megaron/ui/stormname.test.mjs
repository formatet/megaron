import { State as timeTestState } from './../state.js';
import { fmtTickTime as testTickTime } from './fmt_clock.js';
timeTestState.CURRENT_TICK=0; timeTestState.TICK_SECONDS=60; timeTestState.TICK_ANCHOR_MS=Date.now();
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { stormPanelHTML } from './stormname.js';
import { unitHoverLines } from './hover.js';

test('stormPanelHTML: nothing without a storm; a named storm shows its name and no form', () => {
  assert.equal(stormPanelHTML([]), '');
  const html = stormPanelHTML([{ id: 'abc', name: 'Skyla', tier: 'live', can_name: false }]);
  assert.match(html, /Storm Skyla/);
  assert.doesNotMatch(html, /<input/);
});

test('stormPanelHTML: only a storm you may name carries the form; names are escaped', () => {
  const html = stormPanelHTML([{ id: 'abc', name: '', tier: 'remembered', seen_tick: 4, can_name: true }]);
  assert.match(html, new RegExp('last seen '+testTickTime(4)));
  assert.match(html, /id="storm-name-abc"/);
  assert.match(html, /onclick="nameStorm\('abc'\)"/);
  assert.match(stormPanelHTML([{ id: 'x', name: '<b>', tier: 'live' }]), /Storm &lt;b&gt;/);
});

test('hover: a named storm is called by its name', () => {
  const lines = unitHoverLines({ placeName: () => '', storms: [{ tier: 'live', heading: 'NE', name: 'Skyla' }, { tier: 'remembered', seen_tick: 3 }] });
  assert.deepEqual(lines, ['Storm Skyla — drifting NE', "Storm — last seen " + testTickTime(3)]);
});

test('worked water storm slot follows incoming and departing weather without rebuilding other controls', async()=>{
 const {stormHexPanelHTML,updateStormData}=await import('./stormname.js');
 const slot={dataset:{stormQ:'4',stormR:'2',stormSignature:'[]'},innerHTML:'',querySelectorAll:()=>[]};
 const workerControl={value:'unsent order'};
 const root={querySelectorAll:selector=>{assert.equal(selector,'[data-storm-q][data-storm-r]');return [slot];}};
 const storm={id:'live',tier:'live',name:'Skyla',hexes:[{q:4,r:2}]};
 assert.match(stormHexPanelHTML(4,2,[]),/data-storm-q="4"/);
 updateStormData([storm],root);assert.match(slot.innerHTML,/Storm Skyla/);assert.equal(workerControl.value,'unsent order');
 const previous=slot.innerHTML;updateStormData([{...storm,seen_tick:999}],root);assert.equal(slot.innerHTML,previous,'a new live tick does not rebuild the name form');
 updateStormData([{...storm,hexes:[{q:5,r:2}]}],root);assert.equal(slot.innerHTML,'','departed storm clears the open hex');
 updateStormData([{...storm,tier:'remembered',seen_tick:4}],root);assert.match(slot.innerHTML,/last seen/);
 assert.equal(stormHexPanelHTML(9,9,[storm]).includes('storm-note'),false,'unrelated hex gets no storm');
});
