import test from 'node:test';
import assert from 'node:assert/strict';
import { State } from '../../state.js';
State.TICK_SECONDS=60;
import { warehouseHTML } from './city_warehouse.js';
import { productionSectionsHTML } from './city_production.js';
import { readFileSync } from 'node:fs';
const pd=(extra={})=>({food:{coverage_ticks:104.8,food_net_per_tick:1,herd:{count:40,cap:50}},...extra});
const good=(key,name,amount,cap=0)=>({key,name,amount,cap});
const goods=[good('timber','Timber',120,0),good('grain','Grain',463.9,2000),good('fish','Fish',88.8,2000),good('oil','Oil',15.9,2000),
 good('bronze','Bronze',7,0),good('stone','Stone',300,300),good('wine','Wine',0,0),good('livestock','Livestock',40,0),good('copper','Copper',9,1000)];
const labels=html=>[...html.matchAll(/sr-label">([^<]*)</g)].map(m=>m[1]);

test('Warehouse: food with granary cap, herd with pen cap, goods alphabetical with caps',()=>{
 const html=warehouseHTML(pd(),goods);
 assert.match(html,/Food for<\/span><span class="sr-val">104.8 game days/);
 assert.match(html,/Grain<\/span><span class="sr-val">463 \/ 2,000/);
 assert.match(html,/Fish<\/span><span class="sr-val">88 \/ 2,000/);
 assert.match(html,/Herd 40 \/ 50/);
 const goodsPart=html.split('>Goods<')[1];
 assert.deepEqual(labels(goodsPart),['Bronze','Copper','Stone','Timber'],'alphabetical, no food, no livestock, no zero amounts');
 assert.match(goodsPart,/Copper<\/span><span class="sr-val">9 \/ 1,000/);
 assert.match(goodsPart,/Timber<\/span><span class="sr-val">120</);
 assert.doesNotMatch(html,/ticks/);
 assert.match(html,/openCodex\('granary'\)/);
});
test('Warehouse: a store at its cap is marked full with the shared class',()=>{
 const html=warehouseHTML(pd(),goods);
 assert.match(html,/stat-row wh-full[^>]*><span class="sr-label">Stone<\/span><span class="sr-val">300 \/ 300 · full/);
 assert.equal((html.match(/wh-full/g)||[]).length,1);
 assert.match(readFileSync(new URL('../../../../megaron.css',import.meta.url),'utf8'),/\.wh-full\b/,'class exists in megaron.css');
});
test('Warehouse: empty city says so instead of rendering nothing',()=>{
 const html=warehouseHTML({},[]);
 assert.match(html,/No food stored/);assert.match(html,/No goods stored/);assert.doesNotMatch(html,/Herd/);
});
test('Warehouse: tab exists in City, between Production and Buildings',()=>{
 const src=readFileSync(new URL('./city.js',import.meta.url),'utf8');
 assert.match(src,/data-tab="produktion">Production<\/button>\s*<button class="dtab" data-tab="forrad">Warehouse<\/button>\s*<button class="dtab" data-tab="byggnader"/);
 assert.match(src,/id="ctab-forrad"/);
 assert.doesNotMatch(productionSectionsHTML({is_capital:false}),/Food for/);
});
test('Garrison: Recruit link opens War on the Recruit tab for that city',async()=>{
 const city=readFileSync(new URL('./city.js',import.meta.url),'utf8');
 assert.match(city,/Whole forces in[^`]*onclick="warOpenRecruit\('\$\{esc\(capital\.id\)\}'\)">Recruit<\/button>/);
 const opened=[];
 const noopEl=new Proxy({}, {get:(_t,k)=>k==='style'?{}:()=>noopEl,set:()=>true});
 globalThis.window={openDrawer:n=>opened.push(n),closeDrawer:n=>opened.push('close-'+n),addEventListener(){},matchMedia:()=>({matches:false,addEventListener(){}})};
 globalThis.document={getElementById:()=>noopEl,querySelector:()=>null,querySelectorAll:()=>[],addEventListener(){},createElement:()=>noopEl};
 globalThis.localStorage={getItem:()=>null,setItem(){}};globalThis.ResizeObserver??=class{observe(){}};
 const {warOpenRecruit,loadWarDrawer}=await import('./war.js');
 State.WORLD_ID='w';State.provinceData=[{id:'a',own:true,is_capital:true,name:'A'},{id:'b',own:true,name:'B'}];
 warOpenRecruit('b');
 assert.deepEqual(opened,['close-city','war']);
 const active=[];
 const tabs=['army','recruit','movements'].map(t=>({dataset:{tab:t},classList:{toggle:(c,on)=>on&&active.push(t),remove(){},add(){}},addEventListener(){}}));
 const bodyEl={innerHTML:'',querySelector:()=>null,querySelectorAll:q=>q==='.dtab'?tabs:[]};
 const els={};
 document.getElementById=k=>k==='war-body'?bodyEl:(els[k]||={style:{},innerHTML:'',classList:{toggle(){}},dataset:{}});
 globalThis.fetch=async()=>new Response("{}",{status:500});
 await loadWarDrawer().catch(()=>{});
 assert.deepEqual(active,["recruit"],"War opens on the Recruit tab");
});
