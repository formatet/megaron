import test from 'node:test';
import assert from 'node:assert/strict';
import {State} from '../state.js';
import {readFileSync} from 'node:fs';
import {dispatchAction,followDispatchAction} from './dispatch_actions.js';
const context={provinces:[{id:'province',settlement_id:'city',name:'Tylissos',own:true,q:2,r:3}],units:[{id:'ship',category:'naval',status:'garrison'},{id:'army',category:'land',status:'garrison'}],standingOrders:[{id:'route',from_name:'Tylissos',to_name:'Petras'}]};
test('completed hex building targets its owning city grid and exact site, including zero coordinate',()=>{
 assert.deepEqual(dispatchAction('BuildComplete',{settlement_id:'city',building_type:'lumbermill',hex_q:0,hex_r:7},context),{type:'staff',id:'province',settlementID:'city',building:'lumbermill',god:null,hex:{q:0,r:7},label:'Staff Lumbermill'});
 assert.equal(dispatchAction('BuildComplete',{settlement_id:'city',building_type:'market'},context).label,'Show Tylissos');
 assert.equal(dispatchAction('BuildComplete',{settlement_id:'gone',building_type:'lumbermill'},context),null);
});
test('workplace completion selects city centre and god; unit completions select the actual live card',()=>{
 const temple=dispatchAction('BuildComplete',{settlement_id:'city',building_type:'temple',temple:'Temple of Potnia',god_id:'potnia'},context);
 assert.equal(temple.hex,null);assert.equal(temple.god,'potnia');assert.equal(temple.label,'Staff Temple of Potnia');
 assert.deepEqual(dispatchAction('TrainComplete',{unit_id:'ship',settlement_id:'city'},context),{type:'unit',id:'ship',label:'Show the ship'});
 for(const kind of ['OrderFailed','UnitArrived','MarchStalled','UnitRedirected','UnitRecalled','UnitFetched'])assert.equal(dispatchAction(kind,{unit_id:'army'},context).id,'army',kind);
 assert.equal(dispatchAction('ShipFoundered',{unit_id:'gone'},context),null);
 assert.deepEqual(dispatchAction('ShipFoundered',{unit_id:'gone',q:0,r:8},context),{type:'hex',q:0,r:8,label:'Show location (0,8)'});
});
test('correspondence and standing route targets are specific, absent routes are omitted',()=>{
 assert.deepEqual(dispatchAction('MessengerArrival',{sender_id:'other'},context),{type:'letters',person:'other',label:'Reply'});
 assert.equal(dispatchAction('OfferAccepted',{counterpart_wanax_id:'other'},context).person,'other');
 assert.equal(dispatchAction('StandingOrderPaused',{standing_order_id:'route'},context).id,'route');
 assert.equal(dispatchAction('StandingOrderPaused',{standing_order_id:'deleted'},context),null);
});
test('every current literal and archived reader kind omits a targetless button, and maps an explicit incident site honestly',()=>{
 const source=readFileSync(new URL('./format.js',import.meta.url),'utf8');
 const calls=JSON.parse(readFileSync(new URL('../../../../../docs/reviews/dispatchtext-omen/notify-call-sites.json',import.meta.url),'utf8'));
 const kinds=new Set([...calls.map(c=>c.kind).filter(k=>k!=='<dynamic>'),...source.matchAll(/^    case '([^']+)':/gm)].flatMap(x=>typeof x==='string'?[x]:[x[1]]));
 for(const kind of kinds){
  const action=dispatchAction(kind,{},context);
  if(kind==='agora_ready'){assert.equal(action.type,'account');continue;}
  assert.equal(action,null,kind+' must not invent a target');
  assert.deepEqual(dispatchAction(kind,{q:0,r:9},context),{type:'hex',q:0,r:9,label:'Show location (0,9)'},kind);
 }
 for(const kind of ['BuildComplete','TrainComplete','OpeningDispatch'])assert.ok(kinds.has(kind),kind+' must be inventoried');
});
test('the live unit action reaches the existing focused War surface',async()=>{
 let chosen;globalThis.window={warFocusUnit:id=>{chosen=id;}};
 await followDispatchAction(dispatchAction('TrainComplete',{unit_id:'ship'},context));assert.equal(chosen,'ship');
 await followDispatchAction(null);assert.equal(chosen,'ship');
});

test('cult, delivered goods and live cargo reach their actionable selected surface',()=>{
 for(const kind of ['KharisBandChanged','KharisDeclining','RiteAnswered','DivineBlessing','DivinePunishment']){const action=dispatchAction(kind,{settlement_id:'city'},context);assert.equal(action.type,'cult',kind);assert.equal(action.id,'province');assert.equal(action.settlementID,'city');}
 for(const kind of ['TradeDelivery','TradeReturn','GiftDeliveredV2','TransferDelivered','TransferDeliveredV2']){const action=dispatchAction(kind,{dest_id:'city'},context);assert.equal(action.type,'goods',kind);assert.equal(action.id,'province');}
 const cargo=dispatchAction('GiftDispatchedV2',{transport_id:'cargo',dest_id:'city'},{...context,transports:[{id:'cargo'}]});assert.equal(cargo.type,'cargo');assert.equal(cargo.id,'cargo');
 assert.equal(dispatchAction('TrainComplete',{unit_id:'deleted',settlement_id:'city'},context).type,'city','a dead unit never gets a bogus unit card');
});

test('following a dispatch closes the covering drawer before showing its target',async()=>{
 const calls=[];State.activeDrawer='notif';
 globalThis.window={closeDrawer:name=>{calls.push(['close',name]);State.activeDrawer=null;},warFocusUnit:id=>calls.push(['unit',id])};
 await followDispatchAction({type:'unit',id:'ship'});
 assert.deepEqual(calls,[['close','notif'],['unit','ship']]);
 assert.equal(State.activeDrawer,null);
});
