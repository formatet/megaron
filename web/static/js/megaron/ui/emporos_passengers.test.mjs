import test from 'node:test';
import assert from 'node:assert/strict';
const noopEl=new Proxy({}, { get:(_t,k)=>k==='style'?{}:k==='value'?'':()=>noopEl, set:()=>true });
globalThis.ResizeObserver ??= class { observe() {} };
globalThis.document ??= {addEventListener(){},getElementById:()=>noopEl,createElement:()=>noopEl,querySelector:()=>noopEl,querySelectorAll:()=>[],body:noopEl};
globalThis.window ??= {addEventListener(){},matchMedia:()=>({matches:false,addEventListener(){}})};
globalThis.localStorage ??= {getItem:()=>null,setItem(){},removeItem(){}};
const {renderUnitCard}=await import('./drawers/war.js');
const passengers=[1,2,3].map(n=>({id:'cohort'+n,display_name:`${n}st Spearmen of Lato`,type:'spearman',size:100}));
const ship={id:'ship',type:'merchantman',category:'naval',size:1,crew:10,hull:5,status:'garrison',settlement_id:'city',deployable:true,troop_capacity:3,cargo_unit_id:'cohort1',cargo_unit_ids:passengers.map(p=>p.id),cargo_units:passengers};
test('Emporos card lists all passengers and stops loading at three',()=>{
 const html=renderUnitCard(ship);
 for(const p of passengers)assert.ok(html.includes(p.display_name));
 assert.match(html,/Passengers 3 \/ 3/);
 assert.doesNotMatch(html,/>Load<\/button>/);
 assert.match(html,/>Unload all<\/button>/);
 const partial=renderUnitCard({...ship,cargo_unit_ids:['cohort1','cohort2'],cargo_units:passengers.slice(0,2)});
 assert.match(partial,/>Load<\/button>/);
 const galley=renderUnitCard({...ship,type:'galley',troop_capacity:1,cargo_unit_ids:['cohort1'],cargo_units:passengers.slice(0,1)});
 assert.doesNotMatch(galley,/>Load<\/button>/);
});
test('passenger names are escaped in the carrier card',()=>{
 assert.doesNotMatch(renderUnitCard({...ship,cargo_units:[{...passengers[0],display_name:'<script>unsafe</script>'}]}),/<script>/);
});
