import test from 'node:test';
import assert from 'node:assert/strict';
import { startBuild } from './city.js';

// The build list and hex rows live in ../build_rows.test.mjs. Here: the one
// function that sends a build, and the receipt line.

test('startBuild sends the unchanged POST bodies: city building, hex building, temple with god',async t=>{
 const {State}=await import('../../state.js');
 const saved={WORLD_ID:State.WORLD_ID,provinceData:State.provinceData,MY_SETTLEMENT_ID:State.MY_SETTLEMENT_ID};
 const g={document:globalThis.document,fetch:globalThis.fetch,localStorage:globalThis.localStorage};globalThis.localStorage={getItem:()=>null,setItem(){},removeItem(){}};
 t.after(()=>{Object.assign(State,saved);for(const [k,v] of Object.entries(g))v===undefined?delete globalThis[k]:globalThis[k]=v;});
 Object.assign(State,{WORLD_ID:'w',MY_SETTLEMENT_ID:'c',provinceData:[{id:'p',own:true,is_capital:true,settlement_id:'c'}]});
 const result={textContent:'',style:{}};
 globalThis.document={getElementById:id=>id==='city-build-result'?result:null,addEventListener(){}};
 const bodies=[];
 globalThis.fetch=async(url,o)=>{if(o&&o.method==='POST'){bodies.push(o.body);return new Response('{"duration_game_days":4}',{status:200});}return new Response('{}',{status:404});};
 const a=await startBuild({type:'silo'});
 await startBuild({type:'farm',hex_q:3,hex_r:-1});
 await startBuild({type:'temple',god:'velchanos'});
 assert.deepEqual(bodies,['{"building_type":"silo"}','{"building_type":"farm","hex_q":3,"hex_r":-1}','{"building_type":"temple","god":"velchanos"}']);
 assert.equal(a.ok,true);assert.match(a.text,/Granary queued/);
 assert.match(result.textContent,/Temple of Velchanos queued/);
});

test('startBuild shows the server refusal and reports not ok',async t=>{
 const {State}=await import('../../state.js');
 const saved={WORLD_ID:State.WORLD_ID,provinceData:State.provinceData};
 const g={document:globalThis.document,fetch:globalThis.fetch,localStorage:globalThis.localStorage};globalThis.localStorage={getItem:()=>null,setItem(){},removeItem(){}};
 t.after(()=>{Object.assign(State,saved);for(const [k,v] of Object.entries(g))v===undefined?delete globalThis[k]:globalThis[k]=v;});
 Object.assign(State,{WORLD_ID:'w',provinceData:[{id:'p',own:true,is_capital:true,settlement_id:'c'}]});
 const result={textContent:'',style:{}};
 globalThis.document={getElementById:id=>id==='city-build-result'?result:null,addEventListener(){}};
 globalThis.fetch=async()=>new Response('{"error":"needs a harbour"}',{status:422});
 const out=await startBuild({type:'shipyard'});
 assert.equal(out.ok,false);assert.match(out.text,/harbour/);assert.match(result.textContent,/harbour/);
});
