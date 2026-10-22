import test from 'node:test';
import assert from 'node:assert/strict';
import {disbandUnitsHTML,disbandPeople} from './disband.js';
import {State} from '../state.js';
const noopEl=new Proxy({}, {get:(_t,k)=>k==='style'?{}:k==='value'?'':()=>noopEl,set:()=>true});
globalThis.ResizeObserver=class {observe(){}};
globalThis.document={addEventListener(){},getElementById:()=>noopEl,createElement:()=>noopEl,querySelector:()=>noopEl,querySelectorAll:()=>[],body:noopEl};
globalThis.window={addEventListener(){},matchMedia:()=>({matches:false,addEventListener(){}})};
globalThis.localStorage={getItem:()=>null,setItem(){},removeItem(){}};
const {warDisband}=await import('./drawers/war.js');

test('whole-unit city roster uses names and actual survivors or crew, not quantities',()=>{
 const units=[
  {id:'u1',type:'spearman',category:'land',status:'garrison',size:100,settlement_id:'city',display_name:'1st Spearmen of Phaistos'},
  {id:'u2',type:'spearman',category:'land',status:'garrison',size:41,settlement_id:'city',display_name:'<Bronze Guard>'},
  {id:'u3',type:'galley',category:'naval',status:'garrison',size:1,crew:20,settlement_id:'city',display_name:'Asterion'},
  {id:'u4',type:'galley',category:'naval',status:'laid_up',size:1,crew:20,settlement_id:'city',display_name:'Dolphin'},
  {id:'away',type:'spearman',status:'marching',settlement_id:'city',size:100},
  {id:'other',type:'spearman',status:'garrison',settlement_id:'other',size:100},
 ];
 const html=disbandUnitsHTML(units,'city','province');
 assert.equal((html.match(/<button/g)||[]).length,4);
 for(const text of ['1st Spearmen of Phaistos','&lt;Bronze Guard&gt;','Asterion','Dolphin','Returns 100 people','Returns 41 people','Returns 20 people','Returns 0 people'])assert.ok(html.includes(text),text);
 assert.doesNotMatch(html,/<input|wdb-inf|away|other/);
 assert.equal(disbandPeople(units[3]),0);
 assert.equal(disbandPeople({category:'naval',status:'garrison'}),0,'ListUnits omits a zero crew field');
});

test('whole-unit click sends selected id once while pending and preserves refusal',async()=>{
 State.WORLD_ID='world'; State.cityViewID='province'; State.provinceData=[{id:'province',own:true,is_capital:true}];
 const result={textContent:''};
 globalThis.document={getElementById:id=>id==='war-disband-res'?result:null};
 globalThis.localStorage={getItem:()=>null};
 let release;let calls=0;let payload;
 globalThis.fetch=async(url,opts)=>{calls++;payload=JSON.parse(opts.body);await new Promise(r=>{release=r});return new Response('{"error":"unit is marching"}',{status:422});};
 const button={disabled:false,isConnected:true};
 const first=warDisband('province','u2',button);
 await new Promise(r=>setImmediate(r));
 assert.equal(button.disabled,true);
 await warDisband('province','u2',button);
 assert.equal(calls,1);assert.deepEqual(payload,{unit_ids:['u2']});
 release();await first;
 assert.equal(result.textContent,'unit is marching');assert.equal(button.disabled,false);
});
