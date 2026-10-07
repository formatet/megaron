import test from 'node:test';
import assert from 'node:assert/strict';
import {recallAll, recallAllControlsHTML, recallAllResultHTML} from './recall_all.js';

test('Recall all attempts every marching unit despite a refusal', async () => {
  const units=[{id:'a',display_name:'Alpha',status:'marching'},{id:'b',display_name:'<Beta>',status:'marching'},{id:'home',status:'garrison'},{id:'c',display_name:'Gamma',status:'marching'}];
  const calls=[];
  const results=await recallAll(units,async id=>{calls.push(id);return id==='b'?{ok:false,error:'Already called home <again>'}:{ok:true};});
  assert.deepEqual(calls,['a','b','c'],'every marching unit must receive its own recall attempt');
  const html=recallAllResultHTML(results);
  assert.match(html,/Two units are being called home/);
  assert.match(html,/&lt;Beta&gt;: Already called home &lt;again&gt;/);
  assert.doesNotMatch(html,/Alpha:|Gamma:|Runner|Group|<Beta>/);
  assert.match(recallAllControlsHTML(units),/onclick="unitRecallAll\(\)"/);
  assert.doesNotMatch(recallAllControlsHTML(units),/ disabled|<input/);
});

test('no marching units means disabled button and no recall requests',async()=>{
  const units=[{id:'a',status:'garrison'}];
  assert.match(recallAllControlsHTML(units),/ disabled/);
  const results=await recallAll(units,()=>assert.fail('no recall when none marching'));
  assert.deepEqual(results,[]);
  assert.match(recallAllResultHTML(results),/No units are being called home/);
});

test('a transport failure names that unit and continues to later units',async()=>{
  const calls=[];
  const results=await recallAll([{id:'a',status:'marching'},{id:'b',status:'marching'}],async id=>{calls.push(id);if(id==='a')throw Error('Connection lost');return {ok:true};});
  assert.deepEqual(calls,['a','b']);assert.match(recallAllResultHTML(results),/a: Connection lost/);
  assert.match(recallAllResultHTML(results),/One unit is being called home/);
});
