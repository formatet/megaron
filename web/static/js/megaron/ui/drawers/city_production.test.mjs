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
