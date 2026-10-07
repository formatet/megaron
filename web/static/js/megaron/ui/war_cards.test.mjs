import test from 'node:test';
import assert from 'node:assert/strict';
const noopEl=new Proxy({}, { get:(_t,k)=>k==='style'?{}:k==='value'?'':()=>noopEl, set:()=>true });
globalThis.document ??= {addEventListener(){},getElementById:()=>noopEl,createElement:()=>noopEl,querySelector:()=>noopEl,querySelectorAll:()=>[],body:noopEl};
globalThis.window ??= {addEventListener(){},matchMedia:()=>({matches:false,addEventListener(){}})};
globalThis.localStorage ??= {getItem:()=>null,setItem(){},removeItem(){}};
const { State }=await import('../state.js');
const {renderUnitCard,unitStatusLabel}=await import('./drawers/war.js');
const unit=(extra={})=>({id:'u',type:'spearman',display_name:'First Spearmen',category:'land',status:'garrison',deployable:true,size:100,...extra});
const primary=html=>html.split('<details')[0];
const more=html=>html.match(/<details[^>]*>([\s\S]*?)<\/details>/)?.[1] || '';

test('I: garrison shows March and eligible Reinforce; stance and battle retreat stay in closed More',()=>{
 const html=renderUnitCard(unit({can_reinforce:true,size:75,in_battle:true}));
 assert.match(primary(html),/>March</);assert.match(primary(html),/>Reinforce</);
 assert.doesNotMatch(primary(html),/ustance-|uretreat-|>Recall</,'stance must stay under More');
 assert.match(more(html),/ustance-u/,'stance must stay under More');
 assert.match(more(html),/uretreat-u/,'all battle retreat choices must survive');
 for(const value of ['0.75','0.5','0.25','hold']) assert.match(more(html),new RegExp('value="'+value+'"'));
 assert.doesNotMatch(html,/<details[^>]*\bopen\b/,'secondary controls must begin closed');
 assert.doesNotMatch(renderUnitCard(unit({can_reinforce:true,reinforcing:true})),/>Reinforce</);
 assert.doesNotMatch(renderUnitCard(unit({stance:'fortify'})),/>March</);
});

test('I: marching land has Recall as primary, retaining stance and typed redirect under More',()=>{
 const html=renderUnitCard(unit({status:'marching',target_q:4,target_r:2}));
 assert.match(primary(html),/>Recall</,'marching land must keep Recall visible');
 assert.doesNotMatch(primary(html),/>March<|>Redirect<|ustance-/);
 assert.match(more(html),/>Redirect</,'redirect must survive under More');
 assert.match(more(html),/uredir-q-u/);assert.match(more(html),/uredir-r-u/);
 assert.match(more(html),/ustance-u/);
 assert.match(html,/on the march/);assert.doesNotMatch(html,/Invalid Date|NaNm|arrives <\/div>/);
});

test('I: positioned units remain orderable and fetch uses server-provided ships',()=>{
 const html=renderUnitCard(unit({status:'positioned',can_fetch_by_ship:true,pickup_ships:[{id:'s',name:'Ship <one>',settlement_name:'Home',can_carry_runner:true}]}));
 assert.match(primary(html),/>March</);assert.doesNotMatch(primary(html),/Fetch by ship/);
 assert.match(more(html),/Fetch by ship/);assert.match(more(html),/upick-ship-u/);
 assert.match(more(html),/Ship &lt;one&gt;/);
 assert.doesNotMatch(renderUnitCard(unit({status:'positioned',can_fetch_by_ship:false})),/Fetch by ship/);
});

test('I: naval loading unloading and repair remain in More; at sea has no recall or redirect',()=>{
 for(const [extra,control] of [[{settlement_id:'home'},'Load'],[{cargo_unit_id:'cargo'},'Unload'],[{hull:3},'Repair']]){
  const html=renderUnitCard(unit({category:'naval',type:'galley',...extra}));
  assert.match(more(html),new RegExp('>'+control+'<'));
  assert.doesNotMatch(primary(html),new RegExp('>'+control+'<'));
 }
 const html=renderUnitCard(unit({category:'naval',type:'galley',status:'marching'}));
 assert.doesNotMatch(html,/>Recall<|>Redirect<|ustance-/,'ships must never promise recall or redirect');
 assert.match(html,/at sea/);assert.match(html,/returns to port automatically/);
});

test('I: forming training embarked repairing and freight do not offer inapplicable orders',()=>{
 for(const status of ['forming','training','embarked','repairing','freighting']){
  const html=renderUnitCard(unit({status,deployable:false}));
  assert.doesNotMatch(html,/>March<|>Recall<|ustance-|>Reinforce<|<details/);
 }
 const html=renderUnitCard(unit({category:'naval',status:'forming',deployable:false}));
 assert.doesNotMatch(html,/out of food/,'a hull being built is not at sea');
});

test('I: every server lifecycle status has a player label; unknown status never leaks raw jargon',()=>{
 const labels={garrison:'in the city',positioned:'in the field',marching:'on the march',forming:'gathering men',training:'training',embarked:'aboard a ship',repairing:'being repaired',freighting:'carrying goods',disbanded:'dismissed'};
 for(const [status,label] of Object.entries(labels)) assert.equal(unitStatusLabel(unit({status})),label);
 assert.equal(unitStatusLabel(unit({category:'naval'})),'in harbour');
 assert.equal(unitStatusLabel(unit({status:'<new_kind>'})),'awaiting news');
});

test('I: ships off the coast take no March while a city remains, but keep the R6 stranded exception',()=>{
 const saved=State.provinceData;
 try {
  State.provinceData=[{own:true,is_capital:true}];
  assert.doesNotMatch(renderUnitCard(unit({category:'naval',status:'positioned'})),/>March</,'a ship at sea cannot receive a port order');
  State.provinceData=[];
  assert.match(renderUnitCard(unit({category:'naval',status:'positioned'})),/>March</,'R6 stranded ships must remain orderable');
 } finally {State.provinceData=saved;}
});
