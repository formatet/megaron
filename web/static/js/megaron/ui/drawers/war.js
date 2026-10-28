import {renderUnitCard,unitStatusLabel,readyWord} from '../force_card.js';
export {renderUnitCard,unitStatusLabel} from '../force_card.js';
import { disbandReceipt } from '../disband.js';
import { submitRecruitment, recruitmentPending, recruitmentReceipt, rememberRecruitmentReceipt } from '../recruitment.js';
import { foodGoodsText } from '../food.js';
import { showInlineResult, confirmInline } from '../inline_result.js';
import { State, ownCapital, activeCitySettlement } from '../../state.js';
import { serverNow } from '../../clock.js';
import { fetchAuth } from '../../api.js';
import { esc, formatApiError, passageNote, supersededOrderNote, purseLine } from '../format.js';
import { fmtEta, fmtArrival, arrivalHTML } from '../time.js';
import { renderLockedActions } from '../misc.js';
import { unitTypeLabel, actorName } from '../actornames.js';
import { playWarHorn } from '../sfx.js';
import { loadMap } from '../../render/map.js';
import { loadCityDrawer } from './city.js';
import { retreatBody, retreatDefaultSectionHTML, unitRetreatControlHTML } from '../retreat.js';
import { canTakeStance, stanceSentLine } from '../stance.js';
import { forceSections } from '../forces.js';
import { warMovements } from '../movements.js';
import { expeditionMissionText } from '../expedition.js';
import { chooseMarchTarget } from '../marchctx.js';
import { fmtNum, fmtDays } from '../fmt_num.js';
import { fmtTickTime } from '../fmt_clock.js';
import { orderRunnerHTML, orderPursuitText } from '../runner_status.js';
import { recallAll, recallAllControlsHTML, recallAllResultHTML } from '../recall_all.js';
import { roadPanelHTML, roadOrderBody, parseHex } from '../roadorder.js';
let recallAllResult = '';
let recallAllBusy = false;


// "ready <eta>" while still building/training, collapsing to a bare "ready"
// once complete (fmtArrival's doneWord already reads "ready" — this just
// avoids gluing the hardcoded "ready " prefix onto it a second time).
// Unit catalogue (GET /api/v1/units) — static for a world's lifetime, so fetch
// once and memoize rather than refetching on every drawer render. Same pattern
// as getRecipes() in city.js (ui/drawers/city.js): cache the in-flight/resolved
// promise, clear it on failure so the next render retries instead of
// permanently caching a miss. Replaces the old hardcoded cost strings and
// POP_COSTS map, which drifted from the real balance numbers at least twice
// (cedar appeared in the war_chariot string, bronze vanished from war_galley's)
// — the same staleness bug the recipe catalogue fixed for crafting (mig 099).
let _unitCataloguePromise = null;
async function getUnitCatalogue() {
  if (!_unitCataloguePromise) {
    _unitCataloguePromise = fetchAuth('/api/v1/units').then(r => {
      if (!r.ok) throw new Error('forces catalogue fetch failed: ' + r.status);
      return r.json();
    }).catch(e => {
      console.error('getUnitCatalogue', e);
      _unitCataloguePromise = null;
      return null; // null = fetch failed; [] would mean "server has no units"
    });
  }
  return _unitCataloguePromise;
}

// Local recruit-spec ids match the catalogue's `type` key for every unit
// except 'ship', which the /recruit endpoint still accepts as a legacy alias
// for 'galley' (unit.Canonical, server/internal/unit) but which the
// catalogue and can_recruit report only under the canonical 'galley'.
const CATALOGUE_TYPE = { ship: 'galley' };

// The catalogue owns the total price and the people drafted by one order.
function trimNum(v) {
  return (Math.round(v * 1000) / 1000).toString();
}
export function fmtUnitCost(entry, isNaval) {
  if (!entry || !entry.batch_men) return null;
  const goods = Object.entries(entry.costs || {})
    .filter(([, v]) => v > 0)
    .map(([k, v]) => trimNum(v) + ' ' + (k === 'timber' ? 'timber (or cedar)' : k))
    .join(' + ');
  return (goods || '0 cost') + ' · ' + entry.batch_men + (isNaval ? ' crew' : ' men') + ' from the city';
}

export function recruitmentPaidText(costs) {
  const parts = Object.entries(costs || {}).filter(([, amount]) => amount > 0)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([good, amount]) => trimNum(amount) + ' ' + good);
  return parts.length ? ' · paid ' + parts.join(' + ') : '';
}

// ── War drawer ────────────────────────────────────────────────────────────
// See cityLoadSeq: an older reply must not fill the nodes a newer load built.
let warLoadSeq = 0;
export async function loadWarDrawer() {
  const seq = ++warLoadSeq;
  const body = document.getElementById('war-body');
  const capital = ownCapital();
  // Preserve recruit-city selection across reloads. Founder phase (no capital
  // yet) has no settlement to recruit at, so this stays null there.
  const prevRecruitCity = capital ? (document.getElementById('war-recruit-city')?.value || capital.id) : null;
  // Preserve active tab across reloads (recruit-city switch, post-action refresh, etc).
  const prevTab = body.querySelector('.dtab.active')?.dataset.tab || 'army';

  body.innerHTML = `
    <div class="drawer-tabs">
      <button class="dtab active" data-tab="army">Forces</button>
      <button class="dtab" data-tab="recruit">Recruit</button>
      <button class="dtab" data-tab="movements">Marches</button>
    </div>
    <div id="wtab-army" class="city-tab"><div class="loading" style="font-size:.8rem">Loading…</div></div>
    <div id="wtab-recruit" class="city-tab" style="display:none"><div class="loading" style="font-size:.8rem">Loading…</div></div>
    <div id="wtab-movements" class="city-tab" style="display:none"></div>`;

  body.querySelectorAll('.dtab').forEach(tab => {
    tab.addEventListener('click', function() {
      body.querySelectorAll('.dtab').forEach(t => t.classList.remove('active'));
      this.classList.add('active');
      body.querySelectorAll('.city-tab').forEach(c => c.style.display = 'none');
      const el = document.getElementById('wtab-' + this.dataset.tab);
      if (el) el.style.display = '';
    });
  });

  if (prevTab !== 'army') {
    body.querySelectorAll('.dtab').forEach(t => t.classList.toggle('active', t.dataset.tab === prevTab));
    body.querySelectorAll('.city-tab').forEach(c => c.style.display = 'none');
    const el = document.getElementById('wtab-' + prevTab);
    if (el) el.style.display = '';
  }

  renderWarMovements();

  try {
    if (!capital) {
      // Founder phase: no settlement yet, but /units is settlement-independent
      // — the host + any field cohorts already exist server-side. Show them
      // in Army/Movements; Recruit needs a city, so it stays locked.
      const [unitsRes, retreatSec] = await Promise.all([
        fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units`),
        loadRetreatDefaultSection(),
      ]);
      const unitResponse = unitsRes && unitsRes.ok ? await unitsRes.json() : {};
      if (seq !== warLoadSeq) return;
      const allUnits = unitResponse.units || [];
      let armyHtml = retreatSec + '<div class="dsec">' + recallAllControlsHTML(allUnits, recallAllResult, recallAllBusy) + '</div>' + '<div class="dsec"><div class="dsec-title">Forces</div>';
      armyHtml += allUnits.length
        ? renderForceGroups(allUnits)
        : '<p class="empty-state">No forces.</p>';
      armyHtml += '</div>';
      document.getElementById('wtab-army').innerHTML = armyHtml;
      applyUnitFocus();
      document.getElementById('wtab-recruit').innerHTML =
        '<p class="empty-state" style="padding:1rem">No capital yet — found a settlement to train troops.</p>';
      return;
    }

    const needTwo = prevRecruitCity !== capital.id;
    const [res, recRes, unitsRes, catalogue, retreatSec] = await Promise.all([
      fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${capital.id}`),
      needTwo ? fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${prevRecruitCity}`) : Promise.resolve(null),
      fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units`),
      getUnitCatalogue(),
      loadRetreatDefaultSection(),
    ]);
    if (!res.ok) throw new Error();
    const pd = (await res.json()).settlement;
    const recPd = needTwo && recRes && recRes.ok ? (await recRes.json()).settlement : pd;
    const army = pd.army || {};
    const buildings = new Set((recPd.buildings || []).map(b => b.type));
    const canRec = {};
    (recPd.can_recruit || []).forEach(r => { canRec[r.unit] = r; });
    const unitResponse = unitsRes && unitsRes.ok ? await unitsRes.json() : {};
    if (seq !== warLoadSeq) return;
    const allUnits = unitResponse.units || [];
    // catalogue is null on fetch failure (getUnitCatalogue already logged it) —
    // catByType then stays empty and every row degrades to "cost data
    // unavailable" below instead of falling back to a guessed number.
    const catByType = {};
    (catalogue || []).forEach(u => { catByType[u.type] = u; });

    // Army tab — the realm-wide retreat setting, then the discrete units list
    let armyHtml = retreatSec + '<div class="dsec">' + recallAllControlsHTML(allUnits, recallAllResult, recallAllBusy) + '</div>' + '<div class="dsec"><div class="dsec-title">Forces</div>';
    if (allUnits.length) {
      armyHtml += renderForceGroups(allUnits);
    } else {
      armyHtml += '<p class="empty-state">No forces. Recruit in the Recruit tab.</p>';
    }
    armyHtml += '</div>';
    armyHtml += '<div id="war-unit-res" style="font-size:.75rem;margin:.3rem .6rem;min-height:1rem"></div>';
    document.getElementById('wtab-army').innerHTML = armyHtml;
    applyUnitFocus();

    // Recruit tab
    // id = intern nyckel; etiketten hämtas ur actornames.js, aldrig skriven här.
    const UNIT_SPECS = [
      { id:'spearman',       req: buildings.has('barracks') ? null : 'barracks' },
      { id:'war_chariot',    req: buildings.has('stable')   ? null : 'stable' },
      { id:'ship',           req: buildings.has('shipyard')  ? null : 'shipyard' },
      { id:'war_galley',     req: !buildings.has('shipyard') ? 'shipyard' : (!buildings.has('foundry') ? 'foundry' : null) },
      { id:'merchantman',    req: buildings.has('shipyard')  ? null : 'shipyard' },
      { id:'elite_infantry', req: buildings.has('foundry')  ? null : 'foundry' },
    ];
    const mySettlements = State.provinceData.filter(p => p.own && !p.is_outpost);
    let settlementOpts = mySettlements.map(s =>
      '<option value="' + s.id + '"' + (s.id === prevRecruitCity ? ' selected' : '') + '>' + esc(s.name) + (s.is_capital ? ' ★' : '') + '</option>'
    ).join('');
    let recHtml = '<div class="dsec">';
    if (mySettlements.length > 1) {
      recHtml += '<div style="display:flex;align-items:center;gap:.4rem;padding:.25rem 0;font-size:.78rem;color:var(--text-dim)">'
        + '<span>Recruit at:</span>'
        + '<select id="war-recruit-city" style="flex:1;font-size:.75rem;padding:.15rem .3rem;background:var(--warm-white);border:1px solid var(--border);" onchange="loadWarDrawer()">'
        + settlementOpts + '</select></div>';
    } else {
      recHtml += '<input type="hidden" id="war-recruit-city" value="' + capital.id + '">';
    }
    recHtml += '<div class="dsec-title">Recruit Forces</div>';
    recHtml += '<div style="font-size:.65rem;color:var(--text-dim);margin-bottom:.3rem">Land forces train as a full 100-man cohort. Once ready, a cohort can still march after losing men. Reinforce refills it over time only in its original home city — garrison it there and use Reinforce (Army tab). Ships are built one at a time.</div>';
    const NAVAL_SPEC_IDS = ['ship', 'war_galley', 'merchantman'];
    for (const u of UNIT_SPECS) {
      const isNaval = NAVAL_SPEC_IDS.includes(u.id);
      // canRec is keyed by the server's canonical unit type (province.UnitSpecs),
      // which is 'galley' not 'ship' — go through the same alias the catalogue
      // lookup uses so the affordability check actually matches for the Galley
      // row (previously always undefined, so "— insufficient" could never show).
      const catType = CATALOGUE_TYPE[u.id] || u.id;
      const catEntry = catByType[catType];
      const noBuilding = u.req !== null;
      const noResources = !noBuilding && canRec[catType]?.can_recruit === false;
      const disabled = noBuilding || noResources || recruitmentPending(State.WORLD_ID, prevRecruitCity, u.id);
      const costStr = catEntry ? fmtUnitCost(catEntry, isNaval) : 'cost data unavailable';
      const reasons = canRec[catType]?.reasons || (noBuilding ? ['needs a ' + u.req] : []);
      const costText = esc(costStr + (reasons.length ? ' · ' + reasons.join(' · ') : ''));
      const opStyle = disabled ? 'opacity:.5;' : '';
      if (isNaval) {
        // Ship-build overhaul: one vessel per build, optional name, no men select.
        recHtml += '<div style="display:flex;align-items:center;gap:.4rem;padding:.28rem 0;border-bottom:1px solid var(--border);' + opStyle + '">'
          + '<span style="flex:1;font-size:.8rem">' + unitTypeLabel(u.id) + '</span>'
          + '<span style="font-size:.65rem;color:var(--text-dim);text-align:right">' + costText + '</span>'
          + '<input id="wrc-name-' + u.id + '" type="text" placeholder="name (optional)" ' + (disabled ? 'disabled' : '') + ' style="width:100px;padding:.12rem .2rem;border:1px solid var(--border);background:var(--warm-white);font-family:var(--mono);font-size:.7rem">'
          + '<button data-recruit-type="' + u.id + '" onclick="warRecruitShip(\'' + u.id + '\')" ' + (disabled ? 'disabled' : '') + ' style="padding:.2rem .45rem;border:1px solid var(--border);background:var(--sandstone);font-size:.7rem;cursor:pointer;white-space:nowrap">Build 1 Ship</button>'
          + '</div>';
      } else {
        // Kohort-rekrytering: one Train call always drafts a whole 100-man
        // cohort — no men selector anymore (was 10..100 in steps of 10).
        recHtml += '<div style="display:flex;align-items:center;gap:.4rem;padding:.28rem 0;border-bottom:1px solid var(--border);' + opStyle + '">'
          + '<span style="flex:1;font-size:.8rem">' + unitTypeLabel(u.id) + '</span>'
          + '<span style="font-size:.65rem;color:var(--text-dim);text-align:right">' + costText + '</span>'
          + '<button data-recruit-type="' + u.id + '" onclick="warRecruitFromUI(\'' + u.id + '\')" ' + (disabled ? 'disabled' : '') + ' style="padding:.2rem .45rem;border:1px solid var(--border);background:var(--sandstone);font-size:.7rem;cursor:pointer;white-space:nowrap">Train 100</button>'
          + '</div>';
      }
    }
    recHtml += '</div><div id="war-recruit-res" style="font-size:.78rem;margin:.4rem .6rem;min-height:1rem">' + esc(recruitmentReceipt(State.WORLD_ID,prevRecruitCity)) + '</div>';

    const abandonable = mySettlements.filter(s => !s.is_capital);
    if (abandonable.length) {
      recHtml += '<div class="dsec"><div class="dsec-title" style="color:var(--accent)">Abandon settlement</div>'
        + abandonable.map(s =>
          '<div class="stat-row"><span class="sr-label">' + esc(s.name) + '</span>'
          + '<span class="sr-val"><button class="btn-small btn-danger" onclick="warAbandon(\'' + s.settlement_id + '\',\'' + esc(s.name) + '\')">Abandon</button></span></div>'
        ).join('')
        + '<div id="war-abandon-res" style="font-size:.72rem;margin-top:.2rem;min-height:.9rem"></div></div>';
    }
    document.getElementById('wtab-recruit').innerHTML = recHtml;
    // Scope to the selected recruit city, not the capital fallback misc.js
    // uses by default — this panel has its own city selector, and the panel
    // above (buildings/canRec) is already built from prevRecruitCity, so the
    // Locked hints must describe the same settlement or they contradict it.
    const locked = await renderLockedActions('military', prevRecruitCity);
    if (seq !== warLoadSeq) return;
    document.getElementById('wtab-recruit').insertAdjacentHTML('beforeend', locked);

  } catch(e) {
    console.error('war drawer', e);
    if (seq !== warLoadSeq) return;
    document.getElementById('wtab-army').innerHTML = '<p class="empty-state" style="padding:.5rem">Could not load.</p>';
    document.getElementById('wtab-recruit').innerHTML = '<p class="empty-state" style="padding:.5rem">Could not load.</p>';
  }
}

function renderWarMovements() {
  const el = document.getElementById('wtab-movements');
  if (!el) return;
  const { outgoing, incoming } = warMovements({
    units: State.unitsData, foreign: State.foreignUnitData,
    marches: State.marchData, provinces: State.provinceData,
  });
  const placeName = (q, r) => {
    const p = State.provinceData.find(p => p.q === q && p.r === r);
    return p && p.name ? esc(p.name) : '(' + q + ',' + r + ')';
  };
  let html = '';
  if (outgoing.length) {
    html += '<div class="dsec"><div class="dsec-title">Your Marches</div>';
    html += outgoing.map(m => '<div class="obj-card">'
      + '<div class="obj-icon">⚔</div>'
      + '<div class="obj-info"><div class="obj-name">' + esc(m.title) + ' → ' + placeName(m.target_q, m.target_r) + '</div><div class="obj-sub">Arrives ' + arrivalHTML(m.arrives_at) + ' · recall/redirect in the Forces tab</div></div>'
      + '</div>').join('');
    html += '</div>';
  } else {
    html += '<div class="dsec"><p class="empty-state">No ordered forces in the field.</p></div>';
  }
  if (incoming.length) {
    html += '<div class="dsec"><div class="dsec-title" style="color:var(--accent)">⚠ Incoming</div>';
    html += incoming.map(m => '<div class="obj-card">'
      + '<div class="obj-icon" style="color:var(--accent)">⚔</div>'
      + '<div class="obj-info"><div class="obj-name">' + esc(m.title) + ' → ' + placeName(m.target_q, m.target_r) + (m.ifBound ? '?' : '') + '</div><div class="obj-sub">'
      + (m.ifBound ? 'Heading for its lands — there ' + arrivalHTML(m.arrives_at) + ' if that is its goal' : 'Arrives ' + arrivalHTML(m.arrives_at))
      + '</div></div>'
      + '</div>').join('');
    html += '</div>';
  }
  el.innerHTML = html;
}

export function warRecruitFromUI(unitType) {
  const sel = document.getElementById('war-recruit-city');
  const pid = sel ? sel.value : null;
  if (!pid) return;
  return warRecruit(pid, unitType);
}

async function warRecruit(provinceID, unitType, name = '') {
  const world = State.WORLD_ID;
  const status = text => {
    if (State.WORLD_ID !== world) return;
    const el = document.getElementById('war-recruit-res');
    if (el) el.textContent = text;
  };
  const buttons = [...document.querySelectorAll('[data-recruit-type]')].filter(b => b.dataset.recruitType === unitType);
  buttons.forEach(b => { b.disabled = true; });
  await submitRecruitment({ world, city: provinceID, type: unitType, name, status,
    accepted: async data => {
      const label = unitTypeLabel(unitType);
      let receipt = data.names?.length
        ? 'Order received: building ' + data.names.join(', ') + ' — ' + (data.shipyard_waiting ? 'waiting for shipyard workers' : readyWord(data.complete_at))
        : 'Order received: ' + (data.pop_drawn || data.men || 100) + ' ' + label + ' — ' + (data.shipyard_waiting ? 'waiting for shipyard workers' : readyWord(data.complete_at));
      receipt += recruitmentPaidText(data.paid_costs);
      rememberRecruitmentReceipt(world, provinceID, receipt);
      status(receipt);
      await loadWarDrawer();
    },
  });
  buttons.forEach(b => { if (b.isConnected) b.disabled = false; });
}

export async function warRecruitShip(unitType) {
  const pid = document.getElementById('war-recruit-city')?.value;
  if (!pid) return;
  const name = document.getElementById('wrc-name-' + unitType)?.value.trim() || '';
  return warRecruit(pid, unitType, name);
}

const cancellingTraining = new Set();
export async function unitCancelTraining(unitID) {
  const world = State.WORLD_ID;
  const key = world + '/' + unitID;
  if (cancellingTraining.has(key)) return;
  cancellingTraining.add(key);
  const status = text => {
    if (State.WORLD_ID !== world) return;
    const el = document.getElementById('war-unit-res');
    if (el) el.textContent = text;
  };
  status('Cancelling training…');
  let received = false;
  try {
    const response = await fetchAuth(`/api/v1/worlds/${world}/units/${unitID}/cancel-training`, { method: 'POST' });
    const data = await response.json().catch(() => ({}));
    if (!response.ok) { status(formatApiError(data, 'Could not cancel training')); return; }
    received = true;
    if (State.WORLD_ID !== world) return;
    await loadWarDrawer();
    status(data.already_cancelled ? 'Training was already cancelled.' : `Training cancelled: ${data.pop_restored} people returned and reserved resources refunded.`);
  } catch (_) {
    status(received ? 'Training cancelled. Refresh Army to see the updated forces.' : 'Could not confirm cancellation. Refresh Army before trying again.');
  } finally { cancellingTraining.delete(key); }
}

const disbandingUnits = new Set();
export async function warDisband(provinceID, unitID, button) {
  const key = State.WORLD_ID + ':' + unitID;
  if (!unitID || disbandingUnits.has(key)) return;
  disbandingUnits.add(key);
  const worldID=State.WORLD_ID;
  if(button)button.disabled=true;
  let received=false;
  const status=(text,success=false)=>{if(State.WORLD_ID===worldID && activeCitySettlement()?.id===provinceID){const el=document.getElementById('war-disband-res');if(el){el.textContent=text;el.classList?.toggle('disband-success',success);}}};
  try {
    const res = await fetchAuth('/api/v1/worlds/' + worldID + '/provinces/' + provinceID + '/disband', {
      method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({unit_ids:[unitID]}),
    });
    const data=await res.json().catch(()=>({}));
    if(!res.ok){status(data.error || 'Disband failed.');return;}
    received=true;
    if(State.WORLD_ID!==worldID || activeCitySettlement()?.id!==provinceID)return;
    await loadCityDrawer();
    if(activeCitySettlement()?.id===provinceID)document.querySelector('#city-body .dtab[data-tab="garnison"]')?.click();
    status(disbandReceipt(data),true);
  } catch (_) { status(received?'Unit disbanded. Reopen the city to see the result.':'Could not confirm disbanding. Refresh forces before trying again.'); }
  finally {disbandingUnits.delete(key);if(button && button.isConnected)button.disabled=false;}
}

// ── Discrete unit helpers (C8-web) ───────────────────────────────────────
// Map → drawer bridge: clicking a hex with an own positioned/marching unit
// selects it — the War drawer opens with that unit's card highlighted and
// scrolled into view, so orders (march, stance, recall) are given from the
// card. Called from render/map.js via the window bridge.
let _focusUnitID = null;
export function warFocusUnit(unitID) {
  _focusUnitID = unitID;
  window.openDrawer('war');
}

function applyUnitFocus() {
  if (!_focusUnitID) return;
  const card = document.getElementById('ucard-' + _focusUnitID);
  _focusUnitID = null;
  if (!card) return;
  card.classList.add('ucard-focus');
  card.scrollIntoView({ block: 'center' });
}

export function renderForceGroups(forces) {
 return forceSections(forces).map(group=>'<div class="dsec"><div class="dsec-title">'+group.name+'</div>'+ (group.forces.length?group.forces.map(renderUnitCard).join(''):'<p class="empty-state">None.</p>')+'</div>').join('');
}

export async function warAbandon(settlementID, name) {
  const resEl = document.getElementById('war-abandon-res');
  confirmInline(resEl, 'Abandon ' + name + '? This cannot be undone.', async () => {
    const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/settlements/${settlementID}/abandon`, { method: 'POST' });
    if (res.ok) {
      await loadMap();
      await loadWarDrawer();
    } else {
      const d = await res.json().catch(() => ({}));
      showInlineResult(resEl, d.error || 'Abandon failed', true);
    }
  });
}

// The recall/redirect endpoint answers in one of two shapes. Every ordinary
// unit gets 202 {status:"order_dispatched", courier_arrives_at} — a Runner is
// on its way and the order lands when it arrives. The nomadic host gets
// 200 {status:"order_applied"} with NO courier: Wanax travels with the host,
// so the order needs no messenger (unit.CommandedInPerson, server-side). Read
// the shape rather than assuming the courier field is there — printing
// "reaches the unit undefined" was the bug this replaces.
export function orderSentLine(d, verb) {
  if (d && d.status === 'order_applied') {
    return verb + ' order given in person — you ride with the host, so it takes effect at once.';
  }
  if (d?.following_unit) return verb + ' order sent — ' + orderPursuitText(d, fmtArrival(d.courier_arrives_at, d.courier_due_tick)) + supersededOrderNote(d);
  if (d?.catch_up === 'at_destination') return verb + ' order sent by Runner — follows the unit to its destination, arriving ' + fmtArrival(d.courier_arrives_at, d.courier_due_tick) + '. Applies there if the unit is still present.' + supersededOrderNote(d);
  return verb + ' order sent by messenger — reaches the unit ' + fmtArrival(d.courier_arrives_at, d.courier_due_tick) + '.' + supersededOrderNote(d);
}

export async function unitRecall(unitID, resultElement) {
  const statusEl = resultElement || document.getElementById('uorder-' + unitID);
  const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units/${unitID}/recall`, {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: '{}',
  });
  const d = await res.json().catch(() => ({}));
  if (res.ok) {
    if (statusEl) { statusEl.style.color = 'var(--safe-text)'; statusEl.textContent = orderSentLine(d, 'Recall'); }
  } else if (statusEl) {
    statusEl.style.color = 'var(--accent)';
    statusEl.textContent = d.error || 'Recall failed';
  }
  return {ok: res.ok, error: d.error || 'Recall failed'};
}

export async function unitRecallAll() {
  const button = document.getElementById('recall-all-send');
  if (!button || button.disabled || recallAllBusy) return;
  recallAllBusy = true;
  let hasMarching = true;
  button.disabled = true;
  try {
    const response = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units`);
    const data = await response.json();
    if (!response.ok) throw new Error(data.error || 'Could not load forces.');
    if (!Array.isArray(data.units)) throw new Error('Could not read forces.');
    hasMarching = data.units.some(u => u.status === 'marching');
    recallAllResult = recallAllResultHTML(await recallAll(data.units, unitRecall));
  } catch (error) {
    recallAllResult = '<p>' + esc(error.message || 'Recall failed.') + '</p>';
  } finally {
    const result = document.getElementById('recall-all-result');
    if (result) result.innerHTML = recallAllResult;
    recallAllBusy = false;
    const currentButton = document.getElementById('recall-all-send');
    if (currentButton) currentButton.disabled = !hasMarching;
  }
}

export function unitRedirectToggle(unitID) {
  const row = document.getElementById('uredir-' + unitID);
  if (row) row.style.display = row.style.display === 'none' ? 'block' : 'none';
}

// The typed Q/R fallback (megaron_arbetssatt.md: keep it only if it costs
// nothing) — collapsed behind its own link so the primary instruction
// ("right-click the map") isn't buried under an input form most players
// won't need.
export function unitRedirectTypedToggle(unitID) {
  const row = document.getElementById('uredir-typed-' + unitID);
  if (row) row.style.display = row.style.display === 'none' ? 'flex' : 'none';
}

export async function unitRedirect(unitID) {
  const q = parseInt(document.getElementById('uredir-q-' + unitID)?.value || '0', 10);
  const r = parseInt(document.getElementById('uredir-r-' + unitID)?.value || '0', 10);
  const statusEl = document.getElementById('uorder-' + unitID);
  const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units/${unitID}/recall`, {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({ target_q: q, target_r: r }),
  });
  const d = await res.json().catch(() => ({}));
  if (res.ok) {
    if (statusEl) { statusEl.style.color = 'var(--safe-text)'; statusEl.textContent = orderSentLine(d, 'Redirect'); }
  } else if (statusEl) {
    statusEl.style.color = 'var(--accent)';
    statusEl.textContent = d.error || 'Redirect failed';
  }
}

export function unitMarch(unitID) {
  // Use the same destination menu as the map, pinned to the chosen card.
  if (State.activeDrawer) window.closeDrawer(State.activeDrawer);
  chooseMarchTarget(unitID);
}

export async function unitStance(unitID) {
  const sel = document.getElementById('ustance-' + unitID);
  if (!sel) return;
  const stance = sel.value;
  const resEl = document.getElementById('war-unit-res');
  if (resEl) resEl.textContent = '';
  const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units/${unitID}/stance`, {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({ stance }),
  });
  const data = await res.json().catch(() => ({}));
  if (res.ok) {
    // Field unit: 202 order_dispatched — the stance travels by runner
    // and applies on delivery (temenos_orderlopare_plan.md Fas 5). Refresh
    // messengers so the runner + the card's pending-order line show at once.
    if (data.status === 'order_dispatched') {
      if (resEl) {
        resEl.style.color = 'var(--text-dim)';
        resEl.textContent = stanceSentLine(data, fmtArrival(data.courier_arrives_at, data.courier_due_tick)) + passageNote(data);
      }
      fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/messengers`).then(r => r.ok && r.json().then(d => { State.messengerData = d; State.dirty = true; }));
    }
    loadWarDrawer();
  } else if (resEl) {
    resEl.style.color = 'var(--accent)';
    resEl.textContent = formatApiError(data, 'Stance change failed');
  }
}

// unitRetreatOrder chooses one of the two mid-battle retreat modes. Same
// latency rule as unitStance: a field unit's commander only hears it when a
// Runner physically arrives (order_dispatched, 202); a garrisoned unit
// already inside the battle (distance 0 — the Wanax is in that city) applies
// at once. The control only shows while the unit is fighting (in_battle), but
// the battle can end before a Runner arrives — any refusal still surfaces via
// formatApiError below. It overrides the realm-wide setting for this battle only.
export async function unitRetreatOrder(unitID) {
  const sel = document.getElementById('uretreat-' + unitID);
  if (!sel || !sel.value) return;
  const resEl = document.getElementById('war-unit-res');
  if (resEl) resEl.textContent = '';
  const body = retreatBody(sel.value);
  if (!body) return;
  const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units/${unitID}/standing-orders`, {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify(body),
  });
  const data = await res.json().catch(() => ({}));
  if (res.ok) {
    if (data.status === 'order_dispatched') {
      if (resEl) {
        resEl.style.color = 'var(--text-dim)';
        resEl.textContent = '🏃 Messenger carries the retreat order — applies on delivery' + passageNote(data);
      }
      fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/messengers`).then(r => r.ok && r.json().then(d => { State.messengerData = d; State.dirty = true; }));
    }
    loadWarDrawer();
  } else if (resEl) {
    resEl.style.color = 'var(--accent)';
    resEl.textContent = formatApiError(data, 'Retreat order failed');
  }
}

// loadRetreatDefaultSection fetches the realm-wide retreat setting and renders
// its section. A failed read shows the server's reason, never a guessed value.
async function loadRetreatDefaultSection() {
  try {
    const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/retreat-default`);
    const data = await res.json().catch(() => ({}));
    if (!res.ok) return retreatDefaultSectionHTML(null, formatApiError(data, 'Could not load your retreat setting'));
    return retreatDefaultSectionHTML(data);
  } catch (e) {
    console.error('loadRetreatDefaultSection', e);
    return retreatDefaultSectionHTML(null);
  }
}

// saveRetreatDefault stores the realm-wide retreat setting (PUT
// …/retreat-default). A standing doctrine, not an order to a unit: no Runner,
// it applies at once — but only to units entering a battle from now on.
export async function saveRetreatDefault() {
  const sel = document.getElementById('war-retreat-default');
  const resEl = document.getElementById('war-retreat-default-res');
  const body = sel ? retreatBody(sel.value) : null;
  if (!body) return;
  if (resEl) { resEl.className = 'retreat-res'; resEl.textContent = ''; }
  const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/retreat-default`, {
    method: 'PUT', headers: {'Content-Type':'application/json'}, body: JSON.stringify(body),
  });
  const data = await res.json().catch(() => ({}));
  if (!resEl) return;
  if (res.ok) {
    resEl.textContent = 'Saved — applies to battles your forces enter from now on.';
  } else {
    resEl.className = 'retreat-res retreat-res-err';
    resEl.textContent = formatApiError(data, 'Could not save your retreat setting');
  }
}

// unitReinforce marks a decimated cohort as awaiting refill (POST
// .../units/{id}/reinforce, megaron_plan_rekryteringsmodell.md). No
// immediate effect on size — the tick worker trickles men in over time out
// of the origin city's population growth; this just flips the flag. Applies
// at once (no Runner/courier order here — the button only shows for a
// cohort already sitting in its own home-city garrison, distance 0).
export async function unitReinforce(unitID) {
  const resEl = document.getElementById('war-unit-res');
  if (resEl) resEl.textContent = '';
  const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units/${unitID}/reinforce`, {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: '{}',
  });
  const data = await res.json().catch(() => ({}));
  if (res.ok) {
    loadWarDrawer();
  } else if (resEl) {
    resEl.style.color = 'var(--accent)';
    resEl.textContent = formatApiError(data, 'Reinforce failed');
  }
}

export function unitLoadPrompt(shipID, settlementID) {
  // Find co-located garrison land units from allUnits cache
  const shipEl = document.getElementById('war-unit-res');
  // Re-fetch units to build select
  fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units`).then(async r => {
    if (!r.ok) return;
    const units = (await r.json()).units || [];
    // No size gate here either — the server's Load handler (unit.go:670-677)
    // only requires status='garrison', not size=100 (its own doc-comment above
    // says size=100 but the code dropped that gate; same bug, same fix here).
    const candidates = units.filter(u =>
      u.category === 'land' && u.status === 'garrison' && u.deployable &&
      u.settlement_id === settlementID
    );
    if (!candidates.length) {
      if (shipEl) { shipEl.style.color='var(--accent)'; shipEl.textContent='No eligible land unit at this city to load.'; }
      return;
    }
    if (candidates.length === 1) {
      unitLoad(shipID, candidates[0].id);
      return;
    }
    // Multiple: show a quick alert-style select — rare case
    const choice = candidates.find(u => u.id) || candidates[0];
    unitLoad(shipID, choice.id);
  });
}

async function unitLoad(shipID, cargoUnitID) {
  const resEl = document.getElementById('war-unit-res');
  if (resEl) resEl.textContent = '';
  const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units/${shipID}/load`, {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({ unit_id: cargoUnitID }),
  });
  const data = await res.json().catch(() => ({}));
  if (res.ok) {
    loadWarDrawer();
  } else if (resEl) {
    resEl.style.color = 'var(--accent)';
    resEl.textContent = formatApiError(data, 'Load failed');
  }
}

export async function unitUnload(shipID) {
  const resEl = document.getElementById('war-unit-res');
  if (resEl) resEl.textContent = '';
  const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units/${shipID}/unload`, {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({}),
  });
  const data = await res.json().catch(() => ({}));
  if (res.ok) {
    loadWarDrawer();
  } else if (resEl) {
    resEl.style.color = 'var(--accent)';
    resEl.textContent = formatApiError(data, 'Unload failed');
  }
}

// unitPickupToggle shows/hides the ship-choice panel for "Fetch by ship"
// (R1, megaron_plan_hamta_hem.md, slice 2b) — same toggle shape as
// unitRedirectToggle above.
export function unitPickupToggle(unitID) {
  const row = document.getElementById('upick-' + unitID);
  if (row) row.style.display = row.style.display === 'none' ? 'block' : 'none';
}

// unitPickup sends the chosen ship to fetch unitID. wait_ticks is only sent
// when the Wanax typed one — an empty field means "server default"
// (combat.PickupWaitDefaultTicks), never a client-guessed number.
export async function unitPickup(unitID) {
  const shipSel = document.getElementById('upick-ship-' + unitID);
  const waitInput = document.getElementById('upick-wait-' + unitID);
  const resEl = document.getElementById('upick-res-' + unitID);
  const shipID = shipSel ? shipSel.value : '';
  const body = { ship_id: shipID };
  const waitVal = waitInput ? parseInt(waitInput.value, 10) : NaN;
  if (!isNaN(waitVal)) body.wait_ticks = waitVal;
  if (resEl) resEl.textContent = '';
  const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units/${unitID}/pickup`, {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify(body),
  });
  const data = await res.json().catch(() => ({}));
  if (res.ok) {
    if (resEl) {
      resEl.style.color = 'var(--safe-text)';
      resEl.textContent = 'Ship sails to (' + data.shore_q + ',' + data.shore_r + ') — arrives ' + fmtTickTime(data.arrival_tick, data.arrives_at) +
        (data.messenger_id ? ', a messenger rides along' : '') + '.';
    }
    loadWarDrawer();
  } else if (resEl) {
    resEl.style.color = 'var(--accent)';
    resEl.textContent = formatApiError(data, 'Fetch by ship failed');
  }
}

// unitRoadToggle shows/hides the Build road panel — same toggle shape as unitPickupToggle.
export function unitRoadToggle(unitID) {
  const row = document.getElementById('uroad-' + unitID);
  if (row) row.style.display = row.style.display === 'none' ? 'block' : 'none';
}

// unitBuildRoad sends the Build Road order. 202: the stone is on its way and a
// Runner carries the order (or, for an army in the ordering city, it is heard at
// once). The Umami event comes from fetchAuth's verb catalogue, not from here.
export async function unitBuildRoad(unitID) {
  const row = document.getElementById('uroad-' + unitID);
  const sel = document.getElementById('uroad-to-' + unitID);
  const resEl = document.getElementById('uroad-res-' + unitID);
  const body = roadOrderBody(row ? parseHex(row.dataset.from) : null, sel ? sel.value : '');
  if (!body) { if (resEl) resEl.textContent = 'Choose the hex the road reaches.'; return; }
  if (resEl) resEl.textContent = '';
  const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units/${unitID}/build-road`, {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify(body),
  });
  const data = await res.json().catch(() => ({}));
  if (res.ok) {
    if (resEl) {
      resEl.style.color = 'var(--safe-text)';
      resEl.textContent = data.status === 'order_dispatched'
        ? 'Road ordered — ' + fmtNum(data.stone) + ' stone is on its way; the Runner reaches the army ' + fmtArrival(data.courier_arrives_at, data.courier_due_tick) + '.'
        : 'Road ordered — ' + fmtNum(data.stone) + ' stone is on its way to the army.';
    }
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/road-sites`).then(r => r.ok && r.json().then(d => { State.roadSites = d.sites || []; State.dirty = true; }));
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/messengers`).then(r => r.ok && r.json().then(d => { State.messengerData = d; State.dirty = true; }));
  } else if (resEl) {
    resEl.style.color = 'var(--accent)';
    resEl.textContent = formatApiError(data, 'Build road failed');
  }
}

// unitRepair starts a hull repair job (megaron_plan_skeppsreparation.md
// Slice C) on a damaged ship in garrison — same fire-and-refresh shape as
// unitUnload above. The server is the sole judge of whether a shipyard
// exists and has an open berth; this button does not pre-check either, same
// posture as Load/Unload not pre-checking building requirements.
export async function unitRepair(shipID) {
  const resEl = document.getElementById('war-unit-res');
  if (resEl) resEl.textContent = '';
  const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units/${shipID}/repair`, {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({}),
  });
  const data = await res.json().catch(() => ({}));
  if (res.ok) {
    loadWarDrawer();
  } else if (resEl) {
    resEl.style.color = 'var(--accent)';
    resEl.textContent = formatApiError(data, 'Repair failed');
  }
}

async function shipPortAction(shipID, verb) {
	const resEl = document.getElementById('war-unit-res');
	const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units/${shipID}/${verb}`, {
		method: 'POST', headers: {'Content-Type':'application/json'}, body: '{}',
	});
	const data = await res.json().catch(() => ({}));
	if (res.ok) await loadWarDrawer();
	else if (resEl) { resEl.style.color = 'var(--accent)'; resEl.textContent = formatApiError(data, verb === 'lay-up' ? 'Lay up failed' : 'Fit out failed'); }
}
export async function unitLayUp(shipID) { return shipPortAction(shipID, 'lay-up'); }
export async function unitFitOut(shipID) { return shipPortAction(shipID, 'fit-out'); }
