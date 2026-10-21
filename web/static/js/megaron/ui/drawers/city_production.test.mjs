import { State as timeTestState } from './../../state.js';
import { fmtTickTime as testTickTime } from './../fmt_clock.js';
timeTestState.CURRENT_TICK=0; timeTestState.TICK_SECONDS=60; timeTestState.TICK_ANCHOR_MS=Date.now();
import test from 'node:test';
import assert from 'node:assert/strict';
import { productionSectionsHTML, populationHTML, foodSummaryHTML, foodDetailsHTML } from './city_production.js';
const pd=(extra={})=>({population:1000,grain_prod_rate:4.5,grain_consum_rate:3.2,food:{coverage_ticks:4.2,food_net_per_tick:-2},...extra});
test('J: only three primary sections, all secondary abilities remain under closed More',()=>{
 const html=productionSectionsHTML({is_capital:false}),[primary,more]=html.split('<details');
 assert.equal((primary.match(/class="dsec"/g)||[]).length,3,'three primary sections');
 for(const id of ['city-pop-sec','city-prod-sec','city-food-sec'])assert.ok(primary.includes(id));
 for(const id of ['city-devotion-sec','city-food-details-sec','city-lasttick-sec','city-ticklog-sec'])assert.ok(more.includes(id),'retained '+id);
 assert.match(more,/onclick="loadTicklog\(\)"/);assert.doesNotMatch(more,/\bopen\b/,'More starts closed');
 for (const is_capital of [true, false]) assert.doesNotMatch(productionSectionsHTML({is_capital}), /city-gift-sec|city-loyalty-sec|Loyalty log|Gift from capital/);
});
test('J: food days come from authoritative mixed-food coverage, from the single city stock',()=>{
 const html=foodSummaryHTML(pd());assert.match(html,/Food for 4.2 days/,'authoritative food days');assert.doesNotMatch(html,/100|ticks|Coverage/);
 assert.match(foodDetailsHTML(pd()),/4.2 days at current consumption · grain \+ fish/);
 assert.doesNotMatch(foodDetailsHTML(pd()),/Reserve|Stores above|releases below/);
});
test('pen: herd line shows count / cap and warns when full',()=>{
 const open=foodSummaryHTML(pd({food:{coverage_ticks:4.2,food_net_per_tick:1,herd:{count:40,cap:50}}}));
 assert.match(open,/Herd 40 \/ 50/);assert.doesNotMatch(open,/full/);
 const full=foodSummaryHTML(pd({food:{coverage_ticks:4.2,food_net_per_tick:1,herd:{count:50,cap:50}}}));
 assert.match(full,/Herd 50 \/ 50 — full: new animals are eaten, then lost/);assert.match(full,/stat-warn/);
 assert.doesNotMatch(foodSummaryHTML(pd()),/Herd/);
 const calves=foodSummaryHTML(pd({food:{coverage_ticks:4.2,food_net_per_tick:1,herd:{count:40,cap:50,births_next:4,births_in:3,births_grain:4}}}));
 assert.match(calves,/Herd 40 \/ 50 — 4 calves in 3 days \(4 grain\)/);
 const one=foodSummaryHTML(pd({food:{coverage_ticks:4.2,food_net_per_tick:1,herd:{count:10,cap:50,births_next:1,births_in:1,births_grain:0}}}));
 assert.match(one,/1 calf in 1 day \(0 grain\)/);
});
test('J: growing low stocks never say starving; food-worker warning survives',()=>{
 const data=pd();data.food.coverage_ticks=0;data.food.food_net_per_tick=2;
 assert.match(foodSummaryHTML(data),/0 days · stocks growing/);assert.doesNotMatch(foodSummaryHTML(data),/stat-warn|starv/);

 data.food_self_sufficient=false;assert.match(foodSummaryHTML(data),/fields cannot feed everyone/);
 data.food.food_net_per_tick=-2;assert.doesNotMatch(foodSummaryHTML(data),/stocks growing/);
});
test('J: missing/invalid coverage is honestly unavailable, never zero or NaN days',()=>{
 for(const data of [null,{},pd({food:null}),pd({food:{}}),pd({food:{coverage_ticks:NaN}})])assert.match(foodSummaryHTML(data),/not reported/);
});
test('J: population keeps input-free player words; no slaughter button',()=>{
 assert.match(populationHTML(pd(),700,10,'p'),/Citizens<\/span><span class="sr-val">10</);assert.match(populationHTML(pd(),700,10,'p'),/Free to work<\/span><span class="sr-val">7</);
 assert.match(populationHTML(pd(),700,10,'p'),/Livestock/);
 assert.doesNotMatch(populationHTML(pd(),700,10,'p'),/<button|[Ss]laughter/);
});
test('growth line: one sentence per state, from the server growth_state',()=>{
 const g=(state,net)=>populationHTML(pd({food:{growth_state:state,food_net_per_tick:net}}),0,0,'p');
 assert.match(g('holding',-1.67),/Not growing: food net -1\.7 per day \(needs more than 0\)/);
 assert.match(g('growing',30.8),/Growing: food net \+30\.8 per day/);
 assert.match(g('shrinking',-5),/Shrinking: hunger/);
 assert.match(g('growing',1),/class="growth-line"/);
 assert.doesNotMatch(populationHTML(pd(),0,0,'p'),/growth-line/,'no state reported, no line');
});

test('blessed harvest: food section shows authoritative bonus and goods, hides at end tick',()=>{
 const blessing={until_tick:112,remaining_ticks:12,yield_bonus:0.37,goods:['grain','wine']};
 assert.match(foodSummaryHTML(pd({harvest_blessing:blessing})),/Blessed harvest: \+37% grain, wine yield · 12 days left/,'active harvest missing from food section');
 assert.doesNotMatch(foodSummaryHTML(pd({harvest_blessing:blessing})),/Blessed harvest:[^<]*fish|25%|ticks/,'no invented bonus, fish or internal time');
 for(const b of [null,{...blessing,remaining_ticks:0},{...blessing,remaining_ticks:-1}])assert.doesNotMatch(foodSummaryHTML(pd({harvest_blessing:b})),/Blessed harvest/,'no row once blessing ends');
 assert.match(foodSummaryHTML(pd({harvest_blessing:{...blessing,remaining_ticks:1}})),/1 day left/);
});

test('owned city centre offers the authoritative place naming form inside Catchment, other tiles never do',async t=>{
 const {State}=await import('../../state.js');
 const saved=State.tileData;t.after(()=>State.tileData=saved);
 const city={id:'own-city',q:6,r:79,is_capital:true};
 for(const [kind,terrain] of [['river','river_ford'],['massif','mountain_red'],['desert','semi_desert'],['forest','forest_cedar']]){
  const tile={q:6,r:79,terrain,place_kind:kind,place_id:'6,79',can_name:true};
  State.tileData=[{...tile,q:7,r:79},tile];
  const html=productionSectionsHTML(city),form=html.indexOf('place-name-'+kind+'-6,79');
  assert.ok(form>html.indexOf('Catchment &amp; workplaces') && form<html.indexOf('id="city-prod-sec"'),'owned city naming stays beside catchment');
  assert.ok(html.includes(`namePlace('${kind}','6,79')`),'same usable action as hexpanel');
  assert.equal((html.split('<details')[0].match(/class="dsec"/g)||[]).length,3,'no new primary section');
  for(const patch of [{can_name:false},{terrain:'fog'},{tier:'fog'},{visible:false}]){
   State.tileData=[{...tile,...patch}];assert.doesNotMatch(productionSectionsHTML(city),/place-name-|namePlace\(/,'spent or unknown centre has no form');
  }
  State.tileData=[{...tile,q:7}];assert.doesNotMatch(productionSectionsHTML(city),/namePlace\(/,'neighbour rights cannot stand in for centre');
 }
 State.tileData=[];assert.doesNotMatch(productionSectionsHTML(city),/namePlace\(/,'missing terrain does not guess a naming right');
});

test('owned-city naming success refreshes authoritative map and rerenders City despite cleared selectedHex',async t=>{
 const {State}=await import('../../state.js');const {namePlace}=await import('../placename.js');
 const saved=Object.fromEntries(['document','window','localStorage','fetch'].map(k=>[k,globalThis[k]]));
 const state=Object.fromEntries(['tileData','WORLD_ID','activeDrawer','selectedHex','dirty'].map(k=>[k,State[k]]));
 t.after(()=>{for(const [k,v] of Object.entries(saved)) v===undefined ? delete globalThis[k] : globalThis[k]=v;Object.assign(State,state);});
 const city={id:'own-city',q:6,r:79,is_capital:true};
 const tile={q:6,r:79,terrain:'river_ford',place_kind:'river',place_id:'6,79',can_name:true};
 State.tileData=[tile];State.WORLD_ID='world';State.activeDrawer='city';State.selectedHex=null;
 let cityHTML=productionSectionsHTML(city),opened=0;assert.match(cityHTML,/place-name-river-6,79/);
 globalThis.document={getElementById:id=>id==='place-name-river-6,79'?{value:'Alpheios'}:null};
 globalThis.localStorage={getItem:()=>null};
 globalThis.window={openDrawer:name=>{assert.equal(name,'city');cityHTML=productionSectionsHTML(city);opened++;},reopenSelectedHex:()=>assert.fail('owned City has no selected hex to reopen')};
 globalThis.fetch=async url=>url.endsWith('/name')?new Response('{"name":"Alpheios"}'):new Response(JSON.stringify([{...tile,place_name:'Alpheios',can_name:false}]));
 await namePlace('river','6,79');
 assert.equal(opened,1,'successful name reopens the same City drawer');
 assert.equal(State.tileData[0].place_name,'Alpheios');
 assert.match(cityHTML,/<strong>Alpheios<\/strong>/,'named place remains visible beside own-city catchment');
 assert.doesNotMatch(cityHTML,/namePlace\(|place-name-/,'spent naming right disappears from actual City template');
});

test('growth food balance does not print a signed rounded zero',()=>{
 const html=populationHTML(pd({food:{growth_state:'holding',food_net_per_tick:-1e-12}}),0,0,'p');
 assert.match(html,/food net about even/);assert.doesNotMatch(html,/-0.0/);
});

test('AIRound3: population food and army food use different explicit balances',()=>{
 const data=pd({grain_prod_rate:0.5,grain_consum_rate:5,resources:{fish:{rate:5}},army_upkeep:{grain:2},food_net_per_tick_after_upkeep:-1.5,food:{coverage_ticks:10,food_net_per_tick:0.5}});
 const html=foodDetailsHTML(data);
 assert.match(html,/grain 0.5 \+ fish 5 − people eat 5 = 0.5 net before army/);
 assert.match(html,/eats 2 grain per day → food net -1.5 after army/);
 assert.doesNotMatch(foodSummaryHTML(data),/stocks growing/);
});

test('AIRound4: Scheria remainder fish is separate from nearby grain',()=>{
 const html=foodDetailsHTML(pd({food_gubbar_required:5,food_gubbar_placed:5,automatic_food:{nearjord_grain_per_tick:0.5,remainder_people:42,remainder_grain_per_tick:0,remainder_fish_per_tick:0.42}}));
 assert.match(html,/0.50 grain \+ 0.42 fish per day without placed citizens/);assert.match(html,/nearby ground 0.50 grain; 42 people outside whole citizens: 0.00 grain \+ 0.42 fish/);assert.doesNotMatch(html,/could move/);
});

test('equal food: four stocks and actual meal use server totals, never grain-only estimates',()=>{
 const data=pd({grain_prod_rate:999,grain_consum_rate:999,army_upkeep:{grain:999,food:2},food_net_per_tick_after_upkeep:5,
  food:{coverage_ticks:12.5,production_per_tick:10,consumption_per_tick:3,food_net_per_tick:7,
   goods:{grain:100,fish:900,oil:40,wine:60},consumed:{grain:0.3,fish:2.7},consumed_tick:42}});
 assert.match(foodSummaryHTML(data),/Food for 12.5 days/);
 assert.match(foodSummaryHTML(data),/100 grain, 900 fish, 40 oil, 60 wine/);
 const detail=foodDetailsHTML(data);
 assert.match(detail,/production 10 − people eat 3 = 7 net before army/);
 assert.match(detail,/eats 2 food per day → food net 5 after army/);
 assert.match(detail,new RegExp('Last meal.*0.3 grain, 2.7 fish.*'+testTickTime(42)));
 assert.doesNotMatch(detail,/999|army needs grain/);
 data.food.goods={grain:0,fish:0.008,oil:0.0000001,wine:0.1};
 assert.match(foodSummaryHTML(data),/0 grain, under 0.1 fish, under 0.1 oil, 0.1 wine/);
 data.food.consumed={grain:0,fish:0.008,oil:0.0000001};
 assert.match(foodDetailsHTML(data),/Last meal.*0 grain, under 0.1 fish, under 0.1 oil/);
 data.food.consumed_tick=null;
 assert.doesNotMatch(foodDetailsHTML(data),/Last meal/,'no meal invented before first resolution');
});

test('population only offers whole citizen groups, never remainder people',()=>{
 for(const [pop,idle,total,free] of [[1005,5,10,0],[1099,99,10,0],[1100,100,11,1],[1205,205,12,2]]){
  const html=populationHTML(pd({population:pop,food:{growth_state:'growing',food_net_per_tick:1}}),idle,0,'p');
  assert.match(html,new RegExp('Citizens</span><span class="sr-val">'+total+'<'));
  assert.match(html,new RegExp('Free to work</span><span class="sr-val">'+free+'<'));
  assert.doesNotMatch(html,/Population [0-9]|1,005|1,099|1,100|1,205/);
 }
});
