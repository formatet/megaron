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
 for(const value of ['0.75','0.5','0.25','hold']) assert.match(html,new RegExp('value="'+value+'"'));
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
