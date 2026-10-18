import test from 'node:test';
import assert from 'node:assert/strict';
import { productionSectionsHTML, populationHTML, foodSummaryHTML, foodDetailsHTML } from './city_production.js';
const pd=(extra={})=>({population:1000,grain_prod_rate:4.5,grain_consum_rate:3.2,sitos:{coverage_ticks:4.2,low_ticks:10,high_ticks:30,granary_total:100,granary_cap:600,granary_per_good:{grain:60,fish:40},food_net_per_tick:-2},...extra});
test('J: only three primary sections, all secondary abilities remain under closed More',()=>{
 const html=productionSectionsHTML({is_capital:false}),[primary,more]=html.split('<details');
 assert.equal((primary.match(/class="dsec"/g)||[]).length,3,'three primary sections');
 for(const id of ['city-pop-sec','city-prod-sec','city-sitos-sec'])assert.ok(primary.includes(id));
 for(const id of ['city-devotion-sec','city-reserve-sec','city-lasttick-sec','city-loyalty-sec','city-gift-sec','city-ticklog-sec'])assert.ok(more.includes(id),'retained '+id);
 assert.match(more,/onclick="loadTicklog\(\)"/);assert.doesNotMatch(more,/\bopen\b/,'More starts closed');
 assert.doesNotMatch(productionSectionsHTML({is_capital:true}),/city-gift-sec/,'no gift to own capital');
});
test('J: food days come from authoritative mixed-food coverage, not grain net or reserve',()=>{
 const html=foodSummaryHTML(pd());assert.match(html,/Food lasts 4.2 days/,'authoritative food days');assert.doesNotMatch(html,/100|ticks|Coverage|Sitos/);
 assert.match(foodDetailsHTML(pd()),/60 grain, 40 fish/);assert.match(foodDetailsHTML(pd()),/Stores above 30 days · releases below 10 days/,'same thresholds retained');
 assert.match(foodDetailsHTML(pd()),/sitos-state-release/);
});
test('J: growing low stocks never say starving; food-worker warning survives',()=>{
 const data=pd();data.sitos.coverage_ticks=0;data.sitos.granary_total=0;data.sitos.food_net_per_tick=2;
 assert.match(foodSummaryHTML(data),/0 days · stocks growing/);assert.doesNotMatch(foodSummaryHTML(data),/stat-warn|starv/);
 assert.match(foodDetailsHTML(data),/sitos-state-empty-growing/);
 data.food_self_sufficient=false;assert.match(foodSummaryHTML(data),/fields cannot feed everyone/);
 data.sitos.food_net_per_tick=-2;assert.match(foodDetailsHTML(data),/sitos-state-empty-shrinking/);
});
test('J: missing/invalid coverage is honestly unavailable, never zero or NaN days',()=>{
 for(const data of [null,{},pd({sitos:null}),pd({sitos:{}}),pd({sitos:{coverage_ticks:NaN}})])assert.match(foodSummaryHTML(data),/not reported/);
});
test('J: population and slaughter capability keep input-free player words and eligibility',()=>{
 assert.match(populationHTML(pd(),700,10,'p'),/1,000/);assert.match(populationHTML(pd(),700,10,'p'),/700/);
 assert.match(populationHTML(pd(),700,10,'p'),/onclick="slaughterLivestock\('p'\)"/);
 assert.match(populationHTML(pd(),0,0,'p'),/disabled/);
});
test('growth line: one sentence per state, from the server growth_state',()=>{
 const g=(state,net)=>populationHTML(pd({sitos:{growth_state:state,food_net_per_tick:net}}),0,0,'p');
 assert.match(g('holding',-1.67),/Population 1,000 — not growing: food net -1\.7 per day \(needs more than 0\)/);
 assert.match(g('growing',30.8),/growing: food net \+30\.8 per day/);
 assert.match(g('shrinking',-5),/shrinking: hunger/);
 assert.match(g('growing',1),/class="growth-line"/);
 assert.doesNotMatch(populationHTML(pd(),0,0,'p'),/growth-line/,'no state reported, no line');
});

test('blessed harvest: food section shows authoritative bonus and goods, hides at end tick',()=>{
 const blessing={until_tick:112,remaining_ticks:12,yield_bonus:0.37,goods:['grain','wine']};
 assert.match(foodSummaryHTML(pd({harvest_blessing:blessing})),/Blessed harvest: \+37% grain, wine yield · 12 days left/,'active harvest missing from food section');
 assert.doesNotMatch(foodSummaryHTML(pd({harvest_blessing:blessing})),/fish|25%|ticks/,'no invented bonus, fish or internal time');
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
 const html=populationHTML(pd({sitos:{growth_state:'holding',food_net_per_tick:-1e-12}}),0,0,'p');
 assert.match(html,/food net about even/);assert.doesNotMatch(html,/-0.0/);
});

test('AIRound3: population food and army food use different explicit balances',()=>{
 const data=pd({grain_prod_rate:0.5,grain_consum_rate:5,resources:{fish:{rate:5}},army_upkeep:{grain:2},food_net_per_tick_after_upkeep:-1.5,sitos:{coverage_ticks:10,food_net_per_tick:0.5}});
 const html=foodDetailsHTML(data);
 assert.match(html,/grain 0.5 \+ fish 5 − people eat 5 = 0.5 net before army/);
 assert.match(html,/eats 2 grain per day → food net -1.5 after army/);
 assert.doesNotMatch(foodSummaryHTML(data),/stocks growing/);
});

test('AIRound4: Scheria remainder fish is separate from nearby grain',()=>{
 const html=foodDetailsHTML(pd({food_gubbar_required:5,food_gubbar_placed:5,automatic_food:{nearjord_grain_per_tick:0.5,remainder_people:42,remainder_grain_per_tick:0,remainder_fish_per_tick:0.42}}));
 assert.match(html,/0.50 grain \+ 0.42 fish per day without placed citizens/);assert.match(html,/nearby ground 0.50 grain; 42 people outside whole citizens: 0.00 grain \+ 0.42 fish/);assert.doesNotMatch(html,/could move/);
});
