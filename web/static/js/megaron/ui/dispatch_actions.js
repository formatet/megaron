import { State } from '../state.js';
import { buildingLabel } from './building_names.js';

const STAFFABLE=new Set(['farm','mine','lumbermill','stonequarry','olive_press','winery','foundry','temple','shipyard']);
const UNIT_KINDS=new Set(['TrainComplete','OrderFailed','MarchStalled','UnitArrived','UnitFetched','UnitRecalled','UnitRedirected','UnitExploreReturned','ExpeditionTurnedHome','ShipRepaired','ShipDamaged','ShipStormDamaged','ShipStormSpared','ShipStranded','ShipSweptFromSea','ShipCameHomeForFood','PickupWaiting','PickupTimedOut']);
const CULT_KINDS=new Set(['Omen','DivineBlessing','DivinePunishment','KharisBandChanged','KharisDeclining','RiteAnswered','TempleDemolitionWrath','GodsAngered']);
const CARGO_KINDS=new Set(['TradeDelivery','TradeReturn','TradeLost','GiftDispatchedV2','GiftDelivered','GiftDeliveredV2','GiftLost','GiftLostV2','TransferDelivered','TransferDeliveredV2','TransferLostV2','CaravanRaided','CaravanSeized']);
const LETTER_KINDS=new Set(['MessengerArrival','MessengerReturned','OfferAccepted','OfferDeclined','OfferExpired']);
const finitePoint=(q,r)=>Number.isFinite(q)&&Number.isFinite(r)?{q,r}:null;

// A primary action names its real destination. Missing/dead targets produce no
// button; immutable historical coordinates can still open the incident site.
export function dispatchAction(kind,p={},context={}) {
 const provinces=context.provinces||State.provinceData||[],units=context.units||State.unitsData||[];
 const person=LETTER_KINDS.has(kind)&&(p.sender_id||p.counterpart_wanax_id);
 if(person)return {type:'letters',person,label:kind==='MessengerArrival'?'Reply':'Read correspondence'};
 if(kind==='agora_ready')return {type:'account',label:'Open community chat'};
 if(kind.startsWith('StandingOrder')&&p.standing_order_id){
  const route=(context.standingOrders||[]).find(r=>r.id===p.standing_order_id);
  return route?{type:'route',id:route.id,label:'Manage route '+(route.from_name||'')+' → '+(route.to_name||'')}:null;
 }
 const transport=(context.transports||State.tradeData||[]).find(t=>t.id===p.transport_id);
 if(CARGO_KINDS.has(kind)&&transport)return {type:'cargo',id:transport.id,label:'Show cargo in transit'};
 const unitID=p.unit_id||p.ship_id||p.sentry_unit_id||(kind==='OpeningDispatch'?p.host_id:null);
 const unit=units.find(u=>u.id===unitID && u.status!=='disbanded');
 if(unit && (UNIT_KINDS.has(kind)||kind==='OpeningDispatch'))return {type:'unit',id:unit.id,label:unit.category==='naval'?'Show the ship':'Review the unit'};
 const settlementID=(['OfferAccepted','OfferDeclined','OfferExpired'].includes(kind)?p.counterparty_id:null)||p.settlement_id||p.dest_id||p.destination_id||p.threatens_settlement_id||(CARGO_KINDS.has(kind)?p.origin_id:null);
 const city=provinces.find(c=>c.id===settlementID||c.settlement_id===settlementID);
 if(city?.own){
  if(CULT_KINDS.has(kind))return {type:'cult',id:city.id,settlementID:city.settlement_id||settlementID,label:'Open cult in '+city.name};
  if(CARGO_KINDS.has(kind))return {type:'goods',id:city.id,label:'Review goods in '+city.name};
  if(kind==='BuildComplete'&&STAFFABLE.has(p.building_type))return {type:'staff',id:city.id,settlementID:city.settlement_id||settlementID,building:p.building_type,god:p.god_id||null,hex:finitePoint(p.hex_q,p.hex_r),label:'Staff '+(p.temple||p.building_name||buildingLabel(p.building_type))};
  return {type:'city',id:city.id,label:'Show '+city.name};
 }
 const point=finitePoint(p.q,p.r)||finitePoint(p.hex_q,p.hex_r)||(city?finitePoint(city.q,city.r):null);
 if(point)return {type:'hex',...point,label:'Show '+(city?.name||`location (${point.q},${point.r})`)};
 return null;
}

export async function followDispatchAction(action) {
 if(!action)return;
 if(State.activeDrawer)window.closeDrawer?.(State.activeDrawer);
 switch(action.type){
  case 'letters':return window.openLettersWith?.(action.person);
  case 'account':return (await import('./account_window.js')).openAccountWindow();
  case 'unit':return window.warFocusUnit?.(action.id);
  case 'city':
  case 'staff':return (await import('./drawers/city.js')).openCityFromDispatch(action);
  case 'cult':return (await import('./drawers/kult.js')).openKultFromDispatch(action.id);
  case 'goods':return (await import('./drawers/economy.js')).focusEconomyCity(action.id);
  case 'cargo':return (await import('./drawers/economy.js')).focusCargo(action.id);
  case 'route':return (await import('./drawers/economy.js')).focusStandingOrder(action.id);
  case 'hex':return (await import('../render/map.js')).focusDispatchHex(action.q,action.r);
 }
}
