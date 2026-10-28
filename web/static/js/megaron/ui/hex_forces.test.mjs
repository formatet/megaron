import test from 'node:test';
import assert from 'node:assert/strict';
const noopEl=new Proxy({}, { get:(_t,k)=>k==='style'?{}:k==='value'?'':()=>noopEl, set:()=>true });
globalThis.ResizeObserver ??= class { observe() {} };
globalThis.document ??= {addEventListener(){},getElementById:()=>noopEl,createElement:()=>noopEl,querySelector:()=>noopEl,querySelectorAll:()=>[],body:noopEl};
globalThis.window ??= {addEventListener(){},matchMedia:()=>({matches:false,addEventListener(){}})};
globalThis.localStorage ??= {getItem:()=>null,setItem(){},removeItem(){}};
const { State }=await import('../state.js');
const {unitListHTML,ownUnitsAtHex}=await import('../render/map.js');
const unit=(extra={})=>({id:'u',type:'spearman',display_name:'First Spearmen',category:'land',status:'garrison',deployable:true,size:100,...extra});


test('hex own card exposes hull, order, cargo and direct messenger actions',()=>{
 State.provinceData=[];State.messengerData=[];
 const html=unitListHTML([unit({status:'marching',target_q:8,target_r:3,arrives_at:'2026-10-28T22:00:00Z',category:'naval',type:'merchantman',hull:5,cargo_units:[{display_name:'Passengers Lato',size:100,type:'spearman'}],provision_days:4})],[]);
 assert.match(html,/hull 5\/5/);assert.match(html,/Passengers Lato/);assert.match(html,/8,3/);assert.match(html,/No orders reach it at sea|no orders reach it at sea/);
 const land=unitListHTML([unit({status:'marching',target_q:8,target_r:3})],[]);
 assert.match(land,/>Redirect</);assert.match(land,/>Recall</);assert.match(land,/messenger/);assert.match(land,/data-unit-id/);
});
test('hex foreign card uses only the existing public projection',()=>{
 const html=unitListHTML([],[{id:'foreign',type:'galley',category:'naval',name:'Visible ship',owner:'Other',size:1,status:'marching',heading:'south',hull:3,provisions_by_good:{grain:999},target_q:999,target_r:999,cargo:{type:'spearman',size:100,units:1}}]);
 assert.match(html,/Visible ship/);assert.match(html,/Other/);assert.match(html,/spearman|Spearmen/);assert.match(html,/Hull: unknown/);assert.doesNotMatch(html,/999|hull:?\s*3|unitRecall|unitRedirect/i);
});
test('hex marching selection follows the physical route rather than its origin',()=>{
 const now=Date.now();State.founderPhase=null;State.unitsData=[unit({q:0,r:0,status:'marching',target_q:4,target_r:0,path:[[0,0],[1,0],[2,0],[3,0],[4,0]],departs_at:new Date(now-5000).toISOString(),arrives_at:new Date(now+5000).toISOString()})];
 assert.equal(ownUnitsAtHex({q:2,r:0}).length,1);assert.equal(ownUnitsAtHex({q:0,r:0}).length,0);
});

test('hex own card reads real cargo fields, pending runner and mobile collapse without duplicate order ids',()=>{
 State.provinceData=[];State.messengerData=[];State.tradeData=[{mine:true,ship_unit_id:'u',items:[{good_key:'tin',quantity:12}]}];
 const old=window.innerWidth;window.innerWidth=390;
 try {
  const html=unitListHTML([unit({orders_en_route:[{verb:'redirect',target_q:4,target_r:3,following_wait:true}],category:'naval',status:'freighting',provisions_by_good:{fish:4},provision_days:4})]);
  assert.match(html,/tin 12/);assert.match(html,/Messenger carrying redirect/);assert.match(html,/waiting for the force/);assert.match(html,/id="hex-uorder-u"/);assert.doesNotMatch(html,/id="uorder-u"/);assert.match(html,/<details class="hex-force"><summary>/);
 } finally {window.innerWidth=old;State.tradeData=[];}
});

test('freight hull follows its own caravan and never guesses the departure port',()=>{
 const now=Date.now();State.unitsData=[unit({status:'freighting',category:'naval',q:0,r:0})];State.tradeData=[];
 assert.equal(ownUnitsAtHex({q:0,r:0}).length,0);
 State.tradeData=[{mine:true,ship_unit_id:'u',origin_q:0,origin_r:0,dest_q:4,dest_r:0,departs_at:new Date(now-5000).toISOString(),arrives_at:new Date(now+5000).toISOString(),path:[[0,0],[1,0],[2,0],[3,0],[4,0]]}];
 assert.equal(ownUnitsAtHex({q:2,r:0}).length,1);assert.equal(ownUnitsAtHex({q:0,r:0}).length,0);State.tradeData=[];
});
