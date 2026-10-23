import test from 'node:test';
import assert from 'node:assert/strict';
const noopEl=new Proxy({}, { get:(_t,k)=>k==='style'?{}:k==='value'?'':()=>noopEl, set:()=>true });
globalThis.ResizeObserver ??= class { observe() {} };
globalThis.document ??= {addEventListener(){},getElementById:()=>noopEl,createElement:()=>noopEl,querySelector:()=>noopEl,querySelectorAll:()=>[],body:noopEl};
globalThis.window ??= {addEventListener(){},matchMedia:()=>({matches:false,addEventListener(){}})};
globalThis.localStorage ??= {getItem:()=>null,setItem(){},removeItem(){}};
const { State }=await import('../state.js');
const {renderUnitCard,unitStatusLabel}=await import('./drawers/war.js');
const unit=(extra={})=>({id:'u',type:'spearman',display_name:'First Spearmen',category:'land',status:'garrison',deployable:true,size:100,...extra});


test('P: garrison shows March and eligible Reinforce; stance and battle retreat are directly available',()=>{
 const html=renderUnitCard(unit({can_reinforce:true,size:75,in_battle:true}));
 assert.match(html,/>March</);assert.match(html,/>Reinforce</);
 assert.doesNotMatch(html,/>Recall<|<details/,'garrison actions must not be hidden');
 assert.match(html,/ustance-u/,'stance must be on the card');
 assert.match(html,/uretreat-u/,'all battle retreat choices must survive');
 for(const value of ['when_losing','hold']) assert.match(html,new RegExp('value="'+value+'"'));
 assert.doesNotMatch(html,/value="0\.(?:75|5|25)"/,'retired percentage must not be a battle choice');
 assert.doesNotMatch(html,/<details/,'all applicable controls must be directly visible');
 assert.doesNotMatch(renderUnitCard(unit({can_reinforce:true,reinforcing:true})),/>Reinforce</);
 assert.doesNotMatch(renderUnitCard(unit({stance:'fortify'})),/>March</);
});

test('P: marching land has Recall, stance and typed redirect directly available',()=>{
 const html=renderUnitCard(unit({status:'marching',target_q:4,target_r:2}));
 assert.match(html,/>Recall</,'marching land must keep Recall visible');
 assert.doesNotMatch(html,/>March<|<details/);
 assert.match(html,/>Redirect</,'redirect must be on the card');
 assert.match(html,/uredir-q-u/);assert.match(html,/uredir-r-u/);
 assert.match(html,/ustance-u/);
 assert.match(html,/on the march/);assert.doesNotMatch(html,/Invalid Date|NaNm|arrives <\/div>/);
});

test('P: positioned units remain orderable and fetch uses server-provided ships',()=>{
 const html=renderUnitCard(unit({status:'positioned',can_fetch_by_ship:true,pickup_ships:[{id:'s',name:'Ship <one>',settlement_name:'Home',can_carry_runner:true}]}));
 assert.match(html,/>March</);assert.doesNotMatch(html,/<details/);
 assert.match(html,/Fetch by ship/);assert.match(html,/upick-ship-u/);
 assert.match(html,/Ship &lt;one&gt;/);
 assert.doesNotMatch(renderUnitCard(unit({status:'positioned',can_fetch_by_ship:false})),/Fetch by ship/);
});

test('P: naval loading unloading and repair are directly available; at sea has no recall or redirect',()=>{
 for(const [extra,control] of [[{settlement_id:'home'},'Load'],[{cargo_unit_id:'cargo'},'Unload'],[{hull:3},'Repair']]){
  const html=renderUnitCard(unit({category:'naval',type:'galley',...extra}));
  assert.match(html,new RegExp('>'+control+'<'));
  assert.doesNotMatch(html,/<details/);
 }
 const html=renderUnitCard(unit({category:'naval',type:'galley',status:'marching'}));
 assert.doesNotMatch(html,/>Recall<|>Redirect<|ustance-/,'ships must never promise recall or redirect');
 assert.match(html,/at sea/);assert.match(html,/returns to port automatically/);
});

test('P: laid-up hull has Fit out and zero upkeep; fitting-out hull has ETA and no orders',()=>{
 const active=renderUnitCard(unit({category:'naval',type:'galley',settlement_id:'home',cargo_unit_id:null,can_lay_up:true}));
 assert.match(active,/>Lay up</);
 const stored=renderUnitCard(unit({category:'naval',type:'galley',status:'laid_up',deployable:false,settlement_id:'home',crew:20,can_fit_out:true,upkeep_context:'inactive',upkeep_food_per_tick:0,upkeep_silver_per_tick:0}));
 assert.match(stored,/>Fit out</);
 assert.match(stored,/Laid up|crew ashore 20/);
 assert.match(stored,/Ship upkeep while laid up: 0 food, 0 silver/);
 assert.doesNotMatch(stored,/>March<|>Load<|>Repair<|>Lay up</);
 const blocked=renderUnitCard(unit({category:'naval',type:'galley',status:'laid_up',deployable:false,settlement_id:'home',can_fit_out:false}));
 assert.match(blocked,/<button disabled[^>]*>Fit out<\/button>/);
 const preparing=renderUnitCard(unit({category:'naval',type:'galley',status:'fitting_out',deployable:false,settlement_id:'home',build_complete_at:'2026-10-22T10:00:00Z'}));
 assert.match(preparing,/fitting out/);
 assert.doesNotMatch(preparing,/>March<|>Fit out<|>Lay up<|>Load</);
});

test('P: forming training embarked repairing and freight do not offer inapplicable orders',()=>{
 for(const status of ['forming','training','embarked','repairing','freighting']){
  const html=renderUnitCard(unit({status,deployable:false}));
  assert.doesNotMatch(html,/>March<|>Recall<|ustance-|>Reinforce<|<details/);
 }
 const html=renderUnitCard(unit({category:'naval',status:'forming',deployable:false}));
 assert.doesNotMatch(html,/out of food/,'a hull being built is not at sea');
});

test('P: every server lifecycle status has a player label; unknown status never leaks raw jargon',()=>{
 const labels={garrison:'in the city',positioned:'in the field',marching:'on the march',forming:'gathering men',training:'training',embarked:'aboard a ship',repairing:'being repaired',freighting:'carrying goods',disbanded:'dismissed'};
 for(const [status,label] of Object.entries(labels)) assert.equal(unitStatusLabel(unit({status})),label);
 assert.equal(unitStatusLabel(unit({category:'naval'})),'in harbour');
 assert.equal(unitStatusLabel(unit({status:'<new_kind>'})),'awaiting news');
});

test('P: ships off the coast take no March while a city remains, but keep the R6 stranded exception',()=>{
 const saved=State.provinceData;
 try {
  State.provinceData=[{own:true,is_capital:true}];
  assert.doesNotMatch(renderUnitCard(unit({category:'naval',status:'positioned'})),/>March</,'a ship at sea cannot receive a port order');
  State.provinceData=[];
  assert.match(renderUnitCard(unit({category:'naval',status:'positioned'})),/>March</,'R6 stranded ships must remain orderable');
 } finally {State.provinceData=saved;}
});

 test('P: numeric values and missing ETA guards survive the restoration',()=>{
 const forming=renderUnitCard(unit({status:'forming',deployable:false,size:75,men_to_deploy:25}));
 assert.match(forming,/75 of 100/);assert.match(forming,/25 more men/);
 const marching=renderUnitCard(unit({status:'marching',target_q:4,target_r:2,arrives_at:'broken'}));
 assert.doesNotMatch(marching,/NaN|Invalid Date| arrives /);
 assert.match(marching,/id="uorder-u"/,'inline order receipt must remain on the card');
 });

test('B: Army card shows server rations and explicit zero spearman silver without guessing',()=>{
 for(const [extra,want] of [
  [{upkeep_context:'active',upkeep_grain_per_tick:0.5,upkeep_silver_per_tick:0},'0.5 grain, 0 silver per day'],
  [{status:'marching',upkeep_grain_per_tick:1,upkeep_silver_per_tick:0},'1 grain, 0 silver per day'],
  [{type:'elite_infantry',upkeep_grain_per_tick:0.6,upkeep_silver_per_tick:2},'0.6 grain, 2 silver per day'],
  [{category:'naval',type:'galley',upkeep_grain_per_tick:0.02,upkeep_silver_per_tick:1.5},'0.02 grain, 1.5 silver per day'],
 ]) {
  const html=renderUnitCard(unit(extra));
  assert.ok(html.includes('Upkeep: '+want));
  assert.match(html,/class="unit-upkeep"/);
 }
 assert.doesNotMatch(renderUnitCard(unit()),/unit-upkeep/,'missing authoritative rates must not become guessed zero');
});


test('B: upkeep billing context comes from server, not guessed from unit status',()=>{
 assert.match(renderUnitCard(unit({status:'garrison',upkeep_context:'inactive',upkeep_grain_per_tick:0.2,upkeep_silver_per_tick:0})), /Upkeep when active: 0.2 grain, 0 silver per day/);
 assert.match(renderUnitCard(unit({status:'garrison',upkeep_context:'subsistence',upkeep_grain_per_tick:1,upkeep_silver_per_tick:0})), /Upkeep after settling: 1 grain, 0 silver per day/);
 assert.match(renderUnitCard(unit({status:'forming',upkeep_context:'active',upkeep_grain_per_tick:0.5,upkeep_silver_per_tick:0})), />Upkeep: 0.5 grain, 0 silver per day/);
});


test('B: civilian Nomadic Host has no upkeep row when the server says none',()=>{
 assert.doesNotMatch(renderUnitCard(unit({type:'nomadic_host',upkeep_context:'none',upkeep_grain_per_tick:0,upkeep_silver_per_tick:0})), /unit-upkeep|Upkeep:/);
});

test('Haiku 6: a ship at sea with part of a day aboard is not out of food',()=>{
 const ship=extra=>renderUnitCard(unit({category:'naval',type:'galley',status:'marching',provision_days:0,...extra}));
 assert.match(ship({provisions_aboard:true}),/food for less than a day/);
 assert.doesNotMatch(ship({provisions_aboard:true}),/out of food/);
 assert.match(ship({}),/out of food/);
});

test('equal food: roster food alias wins and ship manifest preserves each good',()=>{
 const html=renderUnitCard(unit({category:'naval',type:'galley',status:'marching',crew:40,
  upkeep_context:'active',upkeep_food_per_tick:0.5,upkeep_grain_per_tick:999,upkeep_silver_per_tick:0,
  provision_days:7,provisions_aboard:true,provisions_by_good:{grain:1,fish:9,oil:2,wine:3}}));
 assert.match(html,/Upkeep: 0.5 food, 0 silver per day/);
 assert.match(html,/food for 7 days.*1 grain, 9 fish, 2 oil, 3 wine/);
 assert.doesNotMatch(html,/999/);
 const small=renderUnitCard(unit({category:'naval',type:'galley',status:'marching',crew:40,
  provision_days:0,provisions_aboard:true,provisions_by_good:{grain:0,fish:0.008,oil:0.0000001,wine:0.1}}));
 assert.match(small,/food for less than a day.*0 grain, under 0.1 fish, under 0.1 oil, 0.1 wine/);
 assert.doesNotMatch(small,/&lt;|<0.1|0 fish/,'small positives remain safe plain text, distinct from zero');
});

test('B: Army pay preserves server home, field and surviving-crew amounts',()=>{
 for(const [state,pay] of [
  [{type:'elite_infantry',status:'garrison'},0.75],
  [{type:'elite_infantry',status:'marching'},1.5],
  [{type:'war_chariot',status:'positioned'},2],
  [{category:'naval',type:'galley',status:'garrison',crew:20},0.1],
  [{category:'naval',type:'galley',status:'freighting',crew:11},0.11],
  [{category:'naval',type:'war_galley',status:'repairing',crew:50},0.25],
  [{category:'naval',type:'emporos',status:'fitting_out',crew:10},0.05],
 ]) {
  const html=renderUnitCard(unit({...state,upkeep_context:'active',upkeep_food_per_tick:0.1,upkeep_silver_per_tick:pay}));
  assert.ok(html.includes(pay+' silver per day'), JSON.stringify(state));
 }
});

test('freighting ship shows real provisions, final ration and empty hold without new orders',()=>{
 const ship=extra=>renderUnitCard(unit({category:'naval',type:'merchantman',status:'freighting',deployable:false,crew:10,hull:5,...extra}));
 const fed=ship({provision_days:7,provisions_aboard:true,provisions_by_good:{fish:0.7}});
 assert.match(fed,/food for 7 days.*0.7 fish/);
 assert.match(ship({provision_days:0,provisions_aboard:true}),/food for less than a day/);
 assert.doesNotMatch(ship({provision_days:0,provisions_aboard:true}),/out of food/);
 assert.match(ship({provision_days:0,provisions_aboard:false}),/out of food/);
 assert.doesNotMatch(fed,/>Mission<|>Load<|>Repair<|>Recall<|>Redirect</,'freight binding still blocks orders');
 for(const status of ['garrison','laid_up','fitting_out']){
  assert.doesNotMatch(renderUnitCard(unit({category:'naval',type:'merchantman',status,provision_days:7,provisions_aboard:true})),/food for 7 days/);
 }
});

test('ORDER28: paused builds and repairs explain missing workers without a stale ETA',()=>{
 for (const status of ['forming','repairing']) {
  const html=renderUnitCard(unit({type:'ship',category:'naval',status,shipyard_waiting:true,build_complete_at:'2099-01-01T12:00:00Z'}));
  assert.match(html,/waiting for shipyard workers/);
  assert.doesNotMatch(html,/ready |2099/);
 }
});
