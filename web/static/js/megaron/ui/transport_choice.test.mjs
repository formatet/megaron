import test from 'node:test';
import assert from 'node:assert/strict';
import {transportChoicesHTML} from './transport_choice.js';
test('transport choices show both routes, authoritative time, risk and unavailable reason',()=>{
 const html=transportChoicesHTML({selected_by:'land',options:[{by:'land',available:true,travel_ticks:2,travel_seconds:7200,arrives_at:'2026-10-28T12:00:00Z',risk:'Interception',requirements:[]},{by:'sea',available:false,reason:'No free <Galley>',risk:'Storm and interception',requirements:['Free ship with cargo space']}]},'cargo');
 assert.match(html,/value="land" checked/);assert.match(html,/value="sea" disabled/);assert.match(html,/2 game days/);assert.match(html,/2h/);assert.match(html,/Interception/);assert.match(html,/No free &lt;Galley&gt;/);assert.match(html,/Free ship with cargo space/);
});
test('transport choices preserve explicit available sea choice without selecting unavailable routes',()=>{
 const options=[{by:'land',available:true,travel_ticks:3,travel_seconds:180},{by:'sea',available:true,travel_ticks:1,travel_seconds:60}];
 assert.match(transportChoicesHTML({selected_by:'land',options},'x','sea'),/value="sea" checked/);
 assert.doesNotMatch(transportChoicesHTML({options:options.map(o=>({...o,available:false}))},'x'),/ checked/);
});

import {loadTransportChoices} from './transport_choice.js';
import {State} from '../state.js';
test('late transport options cannot replace a newer destination or another world',async()=>{
 const saved={fetch:globalThis.fetch,document:globalThis.document,localStorage:globalThis.localStorage};const pending=[];const el={textContent:'',innerHTML:'',querySelector:()=>({value:'land'})};
 globalThis.document={getElementById:id=>id==='picker'?el:null};globalThis.localStorage={getItem:()=>null};globalThis.fetch=()=>new Promise(resolve=>pending.push(resolve));State.WORLD_ID='world';
 try{
  const old=loadTransportChoices('picker','home','old');const next=loadTransportChoices('picker','home','next');
  const data=label=>new Response(JSON.stringify({options:[{by:'land',available:true,travel_ticks:1,travel_seconds:60,risk:label}]}));
  pending[1](data('new destination'));await next;const html=el.innerHTML;assert.match(html,/new destination/);
  pending[0](data('old destination'));await old;assert.equal(el.innerHTML,html);
  const world=loadTransportChoices('picker','home','next');State.WORLD_ID='other';pending[2](data('wrong world'));await world;assert.doesNotMatch(el.innerHTML,/wrong world/);
 }finally{Object.assign(globalThis,saved);State.WORLD_ID=null;}
});
