import test from 'node:test';
import assert from 'node:assert/strict';
import { State } from '../../state.js';
import { loadEconomyDrawer } from './economy.js';
const body={innerHTML:'',querySelectorAll:()=>[]},goods={innerHTML:''};
globalThis.document={getElementById:id=>id==='economy-body'?body:id==='ectab-goods'?goods:null};
globalThis.localStorage={getItem:()=>null};
const provinces=[{id:'province-a',settlement_id:'settlement-a',name:'Same name',own:true,is_capital:true},{id:'province-b',settlement_id:'settlement-b',name:'Same name',own:true}];
const overview=[{id:'settlement-b',population:2000,grain_prod_rate:5,grain_consum_rate:10,sitos:{coverage_ticks:5,low_ticks:10,high_ticks:30,granary_total:10,food_net_per_tick:-5}},{id:'settlement-a',population:1000,grain_prod_rate:20,grain_consum_rate:5,sitos:{coverage_ticks:40,low_ticks:10,high_ticks:30,granary_total:100,food_net_per_tick:15}}];
async function render(rows,status=200){
 State.WORLD_ID='world';State.provinceData=provinces;
 const calls=[];globalThis.fetch=async(url)=>{
  calls.push(url);
  return new Response(JSON.stringify(url.endsWith('/overview')?rows:url.endsWith('/actions')?[]:[{key:'grain',amount:100,rate_per_tick:1}]),{status:url.endsWith('/overview')?status:200});
 };
 await loadEconomyDrawer();await new Promise(resolve=>setImmediate(resolve));return {html:goods.innerHTML,calls};
}
test('Economy id: actual drawer joins settlement ids while links and goods keep province ids',async()=>{
 const {html,calls}=await render(overview);
 assert.match(html,/one thousand/,'settlement overview must join settlement id');assert.match(html,/two thousand/);
 assert.doesNotMatch(html,/no data/);assert.match(html,/forty game days/);assert.match(html,/five game days/);
 assert.match(html,/openCitySettlement\('province-a'\)/,'city link keeps province id');assert.match(html,/openCitySettlement\('province-b'\)/);
 assert.ok(calls.some(p=>p.endsWith('/provinces/province-a/goods')));assert.ok(calls.some(p=>p.endsWith('/provinces/province-b/goods')));
 assert.equal(calls.filter(p=>p.endsWith('/settlements/overview')).length,1,'one overview request');
});
test('Economy id: missing or refused overview remains no data; never match province id or name',async()=>{
 const wrong=[{...overview[1],id:'province-a'}];
 assert.match((await render(wrong)).html,/no data/);assert.doesNotMatch((await render(wrong)).html,/one thousand/);
 const partial=(await render([overview[1]])).html;assert.match(partial,/one thousand/,'matched row stays correct');assert.match(partial,/no data/,'missing sibling stays unavailable');
 assert.match((await render([],403)).html,/no data/,'refused overview stays unavailable');
});
