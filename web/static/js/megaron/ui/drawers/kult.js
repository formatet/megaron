import { templeTitheHTML } from './kult_kharis.js';
import { confirmInline, showInlineResult } from '../inline_result.js';
import { State, ownCapital } from '../../state.js';
import { fetchAuth } from '../../api.js';
import { esc } from '../format.js';
import { fmtEta } from '../time.js';
import { fmtDays } from '../fmt_num.js';
import { serverNow } from '../../clock.js';
import { renderLockedActions } from '../misc.js';
import { demolishPrompt, demolishResultLine } from '../demolish.js';
import { realmKultHTML, prayerActionID } from './kult_realm.js';
import { divineValuesHTML } from './kult_values.js';
import { offerRowHTML, offeringFromEntries, offerFoodText } from './kult_offer.js';
import { kharisNetView, kharisDecayHTML, kharisRulesHTML, kharisStandingHTML } from './kult_kharis.js';

// ── Kult drawer ───────────────────────────────────────────────────────────
// See cityLoadSeq: an older reply must not replace what a newer load rendered.
let dispatchCityID=null,dispatchWorldID=null;
const dispatchCity=()=>dispatchWorldID===State.WORLD_ID?dispatchCityID:null;
export function kultCityForID(provinces,id,fallback){
 return id?provinces.find(p=>p.own&&p.id===id)||null:fallback;
}
const activeKultCity=()=>kultCityForID(State.provinceData||[],dispatchCity(),ownCapital());
const activeKultSettlement=()=>{const city=activeKultCity();return city?.settlement_id||(!dispatchCity()?State.MY_SETTLEMENT_ID:null);};
export async function openKultFromDispatch(id){
 if(!kultCityForID(State.provinceData||[],id,null))return;
 dispatchCityID=id;dispatchWorldID=State.WORLD_ID;kultTab='prayers';window.openDrawer?.('kult');await loadKultDrawer();
}
let kultLoadSeq = 0;
let kultTab = 'kharis';
export async function loadKultDrawer() {
 const seq=++kultLoadSeq, world=State.WORLD_ID;
 const el=document.getElementById('kult-body');
 el.innerHTML='<div class="loading">Loading…</div>';
 try {
  const response=await fetchAuth('/api/v1/worlds/'+world+'/cult');
  if(!response.ok)throw new Error('cult unavailable');
  const view=await response.json();
  const locked=await renderLockedActions('cult');
  if(seq!==kultLoadSeq||world!==State.WORLD_ID)return;
  prayerFood.clear();
  prayerCatalog=(view.prayers||[]).map(p=>({...p,action_id:prayerActionID(p)}));
  el.innerHTML=realmKultHTML(view,kultTab,prayerStatusHTML)+locked;
  el.querySelectorAll?.('[data-cult-tab]').forEach(button=>button.addEventListener('click',()=>{
   kultTab=button.dataset.cultTab;
   el.querySelectorAll('[data-cult-tab]').forEach(b=>{b.classList.toggle('active',b===button);b.setAttribute('aria-selected',b===button?'true':'false');});
   el.querySelectorAll('[data-cult-panel]').forEach(panel=>panel.hidden=panel.dataset.cultPanel!==kultTab);
  }));
  el.querySelectorAll?.('[data-cult-demolish]').forEach(button=>button.addEventListener('click',()=>demolishCultTemple(view.temples.find(t=>t.id===button.dataset.cultDemolish))));
  el.querySelectorAll?.('[data-cult-city]').forEach(button=>button.addEventListener('click',()=>window.openCitySettlement(button.dataset.cultCity)));
 }catch(error){if(seq===kultLoadSeq&&world===State.WORLD_ID)el.innerHTML='<p class="empty-state">Could not load your realm’s cult. Try again.</p>';}
}

async function demolishCultTemple(temple){
 if(!temple)return;
 const host=document.getElementById('cult-demolish-'+temple.id);
 const url='/api/v1/worlds/'+State.WORLD_ID+'/provinces/'+temple.province_id+'/buildings';
 try{
  const response=await fetchAuth(url);if(!response.ok)throw new Error();
  const building=(await response.json()).find(b=>b.id===temple.id);
  if(!building?.demolition_kharis){showInlineResult(host,'Temple consequences are unavailable. Try again.',true);return;}
  confirmInline(host,demolishPrompt(temple.name+' in '+temple.city_name,building.demolition_kharis),async()=>{
   const res=await fetchAuth(url+'/'+temple.id,{method:'DELETE'});const result=await res.json();
   if(!res.ok){showInlineResult(host,result.error||'Could not demolish',true);return;}
   await loadKultDrawer();showInlineResult('kult-body',demolishResultLine(temple.name,result.citizens_lifted,result.demolition_kharis));
  },{confirmLabel:'Demolish',cancelLabel:'Keep it'});
 }catch(_){showInlineResult(host,'Could not load demolition consequences. Try again.',true);}
}

const TEMPLE_GOOD_ORDER = ['grain', 'fish', 'timber', 'silver'];
function templeGoodsText(goods, separator) {
  const entries = Object.entries(goods || {}).sort(([a], [b]) => {
    const ai = TEMPLE_GOOD_ORDER.indexOf(a), bi = TEMPLE_GOOD_ORDER.indexOf(b);
    return (ai < 0 ? 4 : ai) - (bi < 0 ? 4 : bi) || a.localeCompare(b);
  });
  return entries.map(([good, amount]) => esc(String(Number(amount.toPrecision(4)))) + ' ' + esc(good)).join(separator) || '—';
}

export function templeOfferingsHTML(offers) {
  let html = '<div class="dsec"><div class="dsec-title">Temple offerings</div>';
  if (!offers.length) {
    return html + '<p class="empty-state">No temples — kharis will not climb without a temple and offerings.</p></div>';
  }
  for (const t of offers) {
    const mark = t.fed ? '<span style="color:var(--safe)">✓</span>' : '<span style="color:var(--accent)">✗</span>';
    const next = (t.paid_fraction > 0 || t.fed) ? templeGoodsText(t.offering, ' + ') : 'unfunded (nothing taken)';
    html += '<div class="stat-row" style="align-items:flex-start;flex-wrap:wrap"><span class="sr-label">' + esc(t.name || '') + (t.temple ? ' — ' + esc(t.temple) : '') + ': </span> ' +
      '<span class="sr-val">reference ' + templeGoodsText(t.required_goods, ' + ') + ' per game day<br>' +
      'Alternatives: ' + templeGoodsText(t.alternatives, ' or ') + '<br>' +
      'Stock: ' + templeGoodsText(t.stock, ', ') + '<br>' +
      'Next gift: ' + next + ' ' + mark + templeTitheHTML(t) + '</span></div>';
  }
  return html + '</div>';
}

// sd.cult_level is NOT a five-tier cult-level progression — the server never
// runs one. internal/kharis/tick.go's deriveMood "replaces player-set
// cult_level" and writes ONE of the same four kharis-mood tiers as
// sd.divine_mood into this column instead, just spelled in Swedish:
// tveksam/vardig/overdadig/vredgad (religion.MoodSuspicious/Indifferent/
// Favorable thresholds). 'enkel' is settlement.go's fallback for a player
// whose first daily tick hasn't run yet (no mood derived at all) — also not
// a tier. 'forsummad'/'praktfull' were the old five-tier model's labels and
// the server can never send them (2026-08-26 audit, megaron_arbetssatt.md
// semantic gate — this table was leaking 'tveksam'/'vredgad' straight to the
// screen as raw Swedish).
const CULT_LABELS = { enkel:'Not yet assessed', tveksam:'Suspicious', vardig:'Indifferent', overdadig:'Favorable', vredgad:'Wrathful' };

// cultLevelLabel translates sd.cult_level for display. Pure — no DOM/State —
// so it can be unit-tested directly (economy.js/cargo.js pattern).
export function cultLevelLabel(cultLevel) {
  return CULT_LABELS[cultLevel] || cultLevel;
}

let prayerCatalog = [];
const prayerFood = new Map();

// Offer opens the offering row under the prayer; nothing is sent yet.
export async function okRite(prayerID) {
  const prayer = prayerCatalog.find(p => p.action_id === prayerID || p.id === prayerID);
  const host = document.getElementById('offer-row-' + prayerID);
  if (!prayer || !host) return;
  const stock = {};
  try {
    const base='/api/v1/worlds/'+State.WORLD_ID+'/provinces/'+prayer.province_id;
    const [r,status]=await Promise.all([fetchAuth(base+'/goods'),fetchAuth(base)]);
    if(!r.ok||!status.ok)throw new Error('city unavailable');
    (await r.json()).forEach(g=>{stock[g.key]=g.amount;});
    prayerFood.set(prayerID,(await status.json()).settlement?.food);
  } catch (_) {showInlineResult(host,'Could not load this temple’s city. Try again.',true);return;}
  host.innerHTML = offerRowHTML({...prayer,id:prayerID}, stock);
  // The food line follows the fields as the gift is changed.
  const foodLine = host.querySelector('[data-offer-food]');
  const inputs = Array.from(host.querySelectorAll('.offer-input'));
  const update = () => {
    const offering = Object.fromEntries(inputs.map(i => [i.dataset.good, Number(i.value) || 0]));
    if (foodLine) foodLine.textContent = offerFoodText(offering, stock, prayerFood.get(prayerID), prayer.city_name);
  };
  inputs.forEach(i => i.addEventListener('input', update));
  update();
}

export function cancelOffering(prayerID) {
  const host = document.getElementById('offer-row-' + prayerID);
  if (host) host.innerHTML = '';
}

export async function bringOffering(prayerID) {
  const prayer = prayerCatalog.find(p => p.action_id === prayerID || p.id === prayerID);
  const host = document.getElementById('offer-row-' + prayerID);
  const inputs = host ? Array.from(host.querySelectorAll('.offer-input')) : [];
  if (prayer?.can_prepare_offering && !prayer.affordable && !inputs.some(i => Number.isFinite(Number(i.value)) && Number(i.value) > 0)) {
    showInlineResult(host, 'Choose goods for your offering.', true);
    return;
  }
  const offering = offeringFromEntries(inputs.map(i => ({ good: i.dataset.good, value: i.value })), prayer && prayer.offering);
  return castRite(prayerID, offering, host);
}

// No offering = the prayer's traditional recipe on the server.
async function castRite(prayerID, offering, host) {
  const prayer=prayerCatalog.find(p=>p.action_id===prayerID||p.id===prayerID);
  if(!prayer)return;
  const body = { prayer: prayer.id };
  if (offering) body.offering = offering;
  const r = await fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/settlements/' + prayer.settlement_id + '/rite', {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify(body),
  });
  const d = await r.json().catch(function(){return {};});
  if (r.ok) {
    await loadKultDrawer();
    showInlineResult('kult-body', prayerResultText(d, prayerCatalog.find(p => p.action_id === prayerID || p.id === prayerID)));
  } else {
    showInlineResult(host || 'kult-body', d.error || 'Rite failed', true);
  }
}

// Server-owned waiting/active state gates the same Offer button the drawer uses.
export function prayerStatusHTML(p, frenzyActive = false, frenzyUntil = null) {
  const waitDays = p.cooldown_remaining_game_days || 0;
  if (p.can_prepare_offering && !p.affordable) {
    return `<button class="obj-cta btn-small" onclick="okRite('${p.id}')">Compose offering</button> <span class="empty-state">The traditional gift is unavailable; choose another offering.</span>`;
  }
  if (p.unavailable_reason) return `<span class="empty-state">${esc(p.unavailable_reason)}</span>`;
  if (p.departure_pending) {
    const waiting = waitDays > 0 ? `; god waits ${esc(fmtDays(waitDays))}` : '';
    return `<span class="empty-state">awaiting next departure${waiting}</span>`;
  }
  if (p.active_remaining_game_days > 0) {
    return `<span class="empty-state">active — ${esc(fmtDays(p.active_remaining_game_days))} left</span>`;
  }
  if (p.effect_type === 'battle_frenzy' && frenzyActive) {
    return `<span style="color:var(--safe-text);font-size:.72rem">active — expires ${fmtEta(frenzyUntil, undefined, 'expired')}</span>`;
  }
  if (waitDays > 0) {
    return `<span style="color:var(--text-dim);font-size:.72rem">on cooldown — ${esc(fmtDays(waitDays))}</span>`;
  }
  if (!p.affordable) {
    return '<span style="color:var(--text-dim);font-size:.72rem">not affordable</span>';
  }
  return `<button class="obj-cta btn-small" onclick="okRite('${p.id}')">Offer</button>`;
}

// Current everyday prayers cannot receive a silent answer. Historical and legacy
// refusals still keep their recorded meaning.
export function prayerResultText(result, prayer) {
  if (result.message) return result.message;
  if (result.success) return 'The gods answered!';
  return prayer?.god_id ? 'The prayer could not be completed.' : 'The gods are silent.';
}
