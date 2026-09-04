import { State, ownCapital } from '../../state.js';
import { serverNow } from '../../clock.js';
import { fetchAuth } from '../../api.js';
import { track } from '../../telemetry.js';
import { esc, formatApiError } from '../format.js';
import { fmtEta, fmtArrival, arrivalHTML } from '../time.js';
import { renderLockedActions } from '../misc.js';
import { unitTypeLabel, actorName } from '../actornames.js';
import { loadMap } from '../../render/map.js';
import { loadCityDrawer } from './city.js';

// "ready <eta>" while still building/training, collapsing to a bare "ready"
// once complete (fmtArrival's doneWord already reads "ready" — this just
// avoids gluing the hardcoded "ready " prefix onto it a second time).
function readyWord(iso) {
  const eta = fmtArrival(iso, undefined, 'ready');
  return eta === 'ready' ? eta : 'ready ' + eta;
}

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
      if (!r.ok) throw new Error('units catalogue fetch failed: ' + r.status);
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

// entry.costs from /api/v1/units is the TOTAL for one recruit call — a
// batch of entry.batch_men (10 for land, the crew size for naval), NOT a
// per-man rate. Dividing back by batch_men here recovers the same per-man
// number the server itself stores (province.UnitSpecs costs) — this is a
// unit conversion of fetched data, not a second hardcoded copy of it — so
// the label keeps matching the men-count selector next to it exactly like
// the old hardcoded "/man" strings did (this is the one place a stray ×10
// would sneak back in, so keep any future edit here dividing, never
// multiplying, the batch total).
function trimNum(v) {
  return (Math.round(v * 1000) / 1000).toString();
}
function fmtUnitCost(entry, isNaval) {
  // batch_men is the divisor for every good below, so a missing/zero value
  // would render "Infinity grain/man" rather than degrade. The server always
  // sets it (10 for land, unit.CrewFor for naval), so treat absence as a
  // failed read, not as something to paper over with a guessed batch size.
  if (!entry || !entry.batch_men) return null;
  const perMan = Object.entries(entry.costs || {})
    .filter(([, v]) => v > 0)
    .map(([k, v]) => trimNum(v / entry.batch_men) + ' ' + k)
    .join(' + ');
  const costStr = (perMan || '0 cost') + '/man';
  // pop_cost is an AFFORDABILITY GATE, not a per-man cost and not citizens
  // consumed: the server's only use of it is `laborPool >= spec.PopCost`
  // (api/handlers/province.go, the can_recruit loop), and found_metropolis.go
  // says so in as many words ("an affordability gate and a catalogue figure,
  // never a population deduction"). What recruiting actually costs in people
  // is one citizen per man (`population - totalMen` in Recruit). So label it
  // as the floor it is — "5 pop" beside "3 grain/man" reads as five citizens
  // per man, which is wrong by an order of magnitude in the scary direction.
  // (province/training.go's own doc-comment still says "citizens consumed per
  // unit trained" — that comment is stale; the call sites are the truth.)
  const popStr = 'needs ' + entry.pop_cost + '+ pop';
  return isNaval ? (costStr + ' · ' + popStr + ' (crew ' + entry.batch_men + ')') : (costStr + ' · ' + popStr);
}

// ── War drawer ────────────────────────────────────────────────────────────
export async function loadWarDrawer() {
  const body = document.getElementById('war-body');
  const capital = ownCapital();
  // Preserve recruit-city selection across reloads. Founder phase (no capital
  // yet) has no settlement to recruit at, so this stays null there.
  const prevRecruitCity = capital ? (document.getElementById('war-recruit-city')?.value || capital.id) : null;
  // Preserve active tab across reloads (recruit-city switch, post-action refresh, etc).
  const prevTab = body.querySelector('.dtab.active')?.dataset.tab || 'army';

  body.innerHTML = `
    <div class="drawer-tabs">
      <button class="dtab active" data-tab="army">Army</button>
      <button class="dtab" data-tab="recruit">Recruit</button>
      <button class="dtab" data-tab="movements">Movements</button>
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

  renderWarMovements(capital);

  try {
    if (!capital) {
      // Founder phase: no settlement yet, but /units is settlement-independent
      // — the host + any field cohorts already exist server-side. Show them
      // in Army/Movements; Recruit needs a city, so it stays locked.
      const unitsRes = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units`);
      const allUnits = unitsRes && unitsRes.ok ? ((await unitsRes.json()).units || []) : [];
      let armyHtml = '<div class="dsec"><div class="dsec-title">Units</div>';
      armyHtml += allUnits.length
        ? allUnits.map(u => renderUnitCard(u)).join('')
        : '<p class="empty-state">No units.</p>';
      armyHtml += '</div>';
      document.getElementById('wtab-army').innerHTML = armyHtml;
      applyUnitFocus();
      document.getElementById('wtab-recruit').innerHTML =
        '<p class="empty-state" style="padding:1rem">No capital yet — found a settlement to train troops.</p>';
      return;
    }

    const needTwo = prevRecruitCity !== capital.id;
    const [res, recRes, unitsRes, catalogue] = await Promise.all([
      fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${capital.id}`),
      needTwo ? fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${prevRecruitCity}`) : Promise.resolve(null),
      fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units`),
      getUnitCatalogue(),
    ]);
    if (!res.ok) throw new Error();
    const pd = (await res.json()).settlement;
    const recPd = needTwo && recRes && recRes.ok ? (await recRes.json()).settlement : pd;
    const army = pd.army || {};
    const buildings = new Set((recPd.buildings || []).map(b => b.type));
    const canRec = {};
    (recPd.can_recruit || []).forEach(r => { canRec[r.unit] = r.can_recruit; });
    const allUnits = unitsRes && unitsRes.ok ? ((await unitsRes.json()).units || []) : [];
    // catalogue is null on fetch failure (getUnitCatalogue already logged it) —
    // catByType then stays empty and every row degrades to "cost data
    // unavailable" below instead of falling back to a guessed number.
    const catByType = {};
    (catalogue || []).forEach(u => { catByType[u.type] = u; });

    // Army tab — discrete units list
    let armyHtml = '<div class="dsec"><div class="dsec-title">Units</div>';
    if (allUnits.length) {
      armyHtml += allUnits.map(u => renderUnitCard(u)).join('');
    } else {
      armyHtml += '<p class="empty-state">No units. Recruit in the Recruit tab.</p>';
    }
    armyHtml += '</div>';
    armyHtml += '<div id="war-unit-res" style="font-size:.75rem;margin:.3rem .6rem;min-height:1rem"></div>';
    armyHtml += `<div id="war-march-panel" style="display:none;background:var(--bg-raised);border:1px solid var(--border);border-radius:2px;margin:.4rem .6rem;padding:.5rem;font-size:.78rem">
      <div style="font-weight:bold;margin-bottom:.3rem">March unit</div>
      <div style="display:flex;gap:.3rem;align-items:center;flex-wrap:wrap">
        <label style="color:var(--text-dim)">Q</label>
        <input id="wmp-q" type="number" value="0" style="width:44px;padding:.1rem .25rem;border:1px solid var(--border);background:var(--warm-white);font-family:var(--mono);font-size:.75rem">
        <label style="color:var(--text-dim)">R</label>
        <input id="wmp-r" type="number" value="0" style="width:44px;padding:.1rem .25rem;border:1px solid var(--border);background:var(--warm-white);font-family:var(--mono);font-size:.75rem">
        <label style="color:var(--text-dim)">Stance</label>
        <select id="wmp-stance" style="font-size:.72rem;padding:.1rem .2rem;border:1px solid var(--border);background:var(--warm-white)">
          <option value="">none</option>
          <option value="storm">storm</option>
          <option value="sentry">sentry</option>
        </select>
        <button onclick="unitMarchSend()" style="padding:.2rem .45rem;border:1px solid var(--border);background:var(--accent-war);color:#fff;font-size:.7rem;cursor:pointer">March →</button>
        <button onclick="closeMarchPanel()" style="padding:.2rem .45rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.7rem;cursor:pointer">Cancel</button>
      </div>
      <div id="wmp-err" style="color:var(--accent);font-size:.7rem;margin-top:.2rem;min-height:.8rem"></div>
    </div>`;
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
    recHtml += '<div class="dsec-title">Train Units</div>';
    recHtml += '<div style="font-size:.65rem;color:var(--text-dim);margin-bottom:.3rem">Land units train as a full 100-man cohort. A cohort worn down in battle regenerates over time in its home city — garrison it there and use Reinforce (Army tab). Ships are built one at a time.</div>';
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
      const noResources = !noBuilding && canRec[catType] === false;
      const disabled = noBuilding || noResources;
      const costStr = catEntry ? fmtUnitCost(catEntry, isNaval) : 'cost data unavailable';
      const costText = noBuilding ? ('requires ' + u.req) : (noResources ? costStr + ' — insufficient' : costStr);
      const opStyle = disabled ? 'opacity:.5;' : '';
      if (isNaval) {
        // Ship-build overhaul: one vessel per build, optional name, no men select.
        recHtml += '<div style="display:flex;align-items:center;gap:.4rem;padding:.28rem 0;border-bottom:1px solid var(--border);' + opStyle + '">'
          + '<span style="flex:1;font-size:.8rem">' + unitTypeLabel(u.id) + '</span>'
          + '<span style="font-size:.65rem;color:var(--text-dim);text-align:right">' + costText + '</span>'
          + '<input id="wrc-name-' + u.id + '" type="text" placeholder="name (optional)" ' + (disabled ? 'disabled' : '') + ' style="width:100px;padding:.12rem .2rem;border:1px solid var(--border);background:var(--warm-white);font-family:var(--mono);font-size:.7rem">'
          + '<button onclick="warRecruitShip(\'' + u.id + '\')" ' + (disabled ? 'disabled' : '') + ' style="padding:.2rem .45rem;border:1px solid var(--border);background:var(--sandstone);font-size:.7rem;cursor:pointer;white-space:nowrap">Build 1 Ship</button>'
          + '</div>';
      } else {
        // Kohort-rekrytering: one Train call always drafts a whole 100-man
        // cohort — no men selector anymore (was 10..100 in steps of 10).
        recHtml += '<div style="display:flex;align-items:center;gap:.4rem;padding:.28rem 0;border-bottom:1px solid var(--border);' + opStyle + '">'
          + '<span style="flex:1;font-size:.8rem">' + unitTypeLabel(u.id) + '</span>'
          + '<span style="font-size:.65rem;color:var(--text-dim);text-align:right">' + costText + '</span>'
          + '<button onclick="warRecruitFromUI(\'' + u.id + '\')" ' + (disabled ? 'disabled' : '') + ' style="padding:.2rem .45rem;border:1px solid var(--border);background:var(--sandstone);font-size:.7rem;cursor:pointer;white-space:nowrap">Train 100</button>'
          + '</div>';
      }
    }
    recHtml += '</div><div id="war-recruit-res" style="font-size:.78rem;margin:.4rem .6rem;min-height:1rem"></div>';

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
    document.getElementById('wtab-recruit').innerHTML += await renderLockedActions('military', prevRecruitCity);

  } catch(e) {
    console.error('war drawer', e);
    document.getElementById('wtab-army').innerHTML = '<p class="empty-state" style="padding:.5rem">Could not load.</p>';
    document.getElementById('wtab-recruit').innerHTML = '<p class="empty-state" style="padding:.5rem">Could not load.</p>';
  }
}

function renderWarMovements(capital) {
  const el = document.getElementById('wtab-movements');
  if (!el) return;
  // capital may be null (founder phase) — State.provinceData has no owned
  // rows yet then, so ownPos/outgoing/incoming all come out empty and the
  // tab falls through to "No movements." below.
  const ownPos = new Set(State.provinceData.filter(p => p.own).map(p => p.q + ',' + p.r));
  const outgoing = State.marchData.filter(m => ownPos.has(m.origin_q + ',' + m.origin_r));
  const incoming = State.marchData.filter(m => {
    const t = State.provinceData.find(p => p.q === m.target_q && p.r === m.target_r && p.own);
    return t && !ownPos.has(m.origin_q + ',' + m.origin_r);
  });
  let html = '';
  if (outgoing.length) {
    html += '<div class="dsec"><div class="dsec-title">Outgoing</div>';
    html += outgoing.map(m => {
      const target = State.provinceData.find(p => p.q === m.target_q && p.r === m.target_r);
      const tname = target ? esc(target.name) : '(' + m.target_q + ',' + m.target_r + ')';
      return '<div class="obj-card">'
        + '<div class="obj-icon">⚔</div>'
        + '<div class="obj-info"><div class="obj-name">' + m.intent.charAt(0).toUpperCase() + m.intent.slice(1) + ' → ' + tname + '</div><div class="obj-sub">Arrives ' + arrivalHTML(m.arrives_at) + ' · recall/redirect in the Army tab</div></div>'
        + '</div>';
    }).join('');
    html += '</div>';
  } else {
    html += '<div class="dsec"><p class="empty-state">No armies in the field.</p></div>';
  }
  if (incoming.length) {
    html += '<div class="dsec"><div class="dsec-title" style="color:var(--accent)">⚠ Incoming</div>';
    html += incoming.map(m => {
      const origin = State.provinceData.find(p => p.q === m.origin_q && p.r === m.origin_r);
      const oname = origin ? esc(origin.name) : '(' + m.origin_q + ',' + m.origin_r + ')';
      return '<div class="obj-card">'
        + '<div class="obj-icon" style="color:var(--accent)">⚔</div>'
        + '<div class="obj-info"><div class="obj-name">' + m.intent.charAt(0).toUpperCase() + m.intent.slice(1) + ' from ' + oname + '</div><div class="obj-sub">Arrives ' + arrivalHTML(m.arrives_at) + '</div></div>'
        + '</div>';
    }).join('');
    html += '</div>';
  }
  el.innerHTML = html || '<p class="empty-state" style="padding:.5rem">No movements.</p>';
}

export function warRecruitFromUI(unitType) {
  const sel = document.getElementById('war-recruit-city');
  const pid = sel ? sel.value : null;
  if (!pid) return;
  warRecruit(pid, unitType);
}

async function warRecruit(provinceID, unitType) {
  // Kohort-rekrytering: the server always drafts a whole 100-man cohort for
  // land units now (men is ignored server-side) — nothing left to read here.
  const res = await fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/provinces/' + provinceID + '/recruit', {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({ unit_type: unitType }),
  });
  const data = await res.json().catch(() => ({}));
  const resEl = document.getElementById('war-recruit-res');
  if (res.ok) {
    track('recruit_started', { unit: unitType });
    // Reload war drawer to show updated training queue
    loadWarDrawer();
  } else if (resEl) {
    resEl.style.color = 'var(--accent)';
    resEl.textContent = formatApiError(data, 'failed');
  }
}

// Ship-build overhaul: builds exactly one vessel (no men field — crew is
// fixed per type); an empty name input means "let the game suggest one".
export async function warRecruitShip(unitType) {
  const sel = document.getElementById('war-recruit-city');
  const pid = sel ? sel.value : null;
  if (!pid) return;
  const nameEl = document.getElementById('wrc-name-' + unitType);
  const name = nameEl ? nameEl.value.trim() : '';
  const body = { unit_type: unitType };
  if (name) body.name = name;
  const res = await fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/provinces/' + pid + '/recruit', {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify(body),
  });
  const data = await res.json().catch(() => ({}));
  const resEl = document.getElementById('war-recruit-res');
  if (res.ok) {
    track('recruit_started', { unit: unitType });
    if (resEl) {
      const built = (data.names && data.names[0]) ? data.names[0] : unitType;
      resEl.style.color = 'var(--text-dim)';
      resEl.textContent = 'Building "' + built + '" — ' + readyWord(data.complete_at);
    }
    loadWarDrawer();
  } else if (resEl) {
    resEl.style.color = 'var(--accent)';
    resEl.textContent = formatApiError(data, 'failed');
  }
}

export async function warDisband(provinceID) {
  const inf = parseInt(document.getElementById('wdb-inf')?.value || '0');
  const cha = parseInt(document.getElementById('wdb-cha')?.value || '0');
  if (inf + cha === 0) return;
  const res = await fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/provinces/' + provinceID + '/disband', {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({ spearman: inf, war_chariot: cha }),
  });
  const data = await res.json().catch(() => ({}));
  if (res.ok) {
    // loadCityDrawer() re-renders #war-disband-res from scratch (city.js),
    // so the result line is set on the fresh element afterwards, not before.
    await loadCityDrawer();
    const resEl = document.getElementById('war-disband-res');
    if (resEl && typeof data.pop_restored === 'number') {
      resEl.style.color = 'var(--text-dim)';
      resEl.textContent = '+' + data.pop_restored + ' population (now ' + data.population + ')';
    }
  } else {
    const resEl = document.getElementById('war-disband-res');
    if (resEl) resEl.textContent = formatApiError(data, 'failed');
  }
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

// State for the march panel — module-local, only ever read/written from the
// functions in this file.
let _marchUnitID = null;

function renderUnitCard(u) {
  const lbl = esc(actorName(u));
  const isNaval = u.category === 'naval';
  const isForming = u.status === 'forming';
  const isTraining = u.status === 'training';
  const isGarrison = u.status === 'garrison';
  const isPositioned = u.status === 'positioned';
  const isMarching = u.status === 'marching';
  const isEmbarked = u.status === 'embarked';

  // Location string
  let loc = '';
  if (isGarrison || isForming || isTraining || isEmbarked) {
    const prov = State.provinceData.find(p => p.settlement_id === u.settlement_id || p.id === u.settlement_id);
    const place = prov ? esc(prov.name) : 'city';
    // A galley idle at its coastal settlement is docked — say it is "i hamn"
    // rather than just naming the city (a land garrison is self-evidently in the
    // city; a ship being IN PORT vs at sea is the meaningful distinction).
    loc = (isNaval && isGarrison) ? '⚓ i hamn — ' + place : place;
  } else if (isMarching && u.target_q != null) {
    // arrival_tick is the authoritative arrival (K4) — the stored arrives_at
    // stamp lies across server downtime; the tick self-corrects.
    loc = '→ (' + u.target_q + ',' + u.target_r + ') arrives ' + arrivalHTML(u.arrives_at, u.arrival_tick);
  } else if (u.q != null) {
    loc = '(' + u.q + ',' + u.r + ')';
  }

  // Progress: naval forming shows the build ETA (size is always 1); land forming
  // shows the men/100 gathering bar; land training shows a full bar + ready ETA
  // (it has all 100 men and is maturing to a deployable garrison).
  const bar = (pct) => '<div style="margin:.2rem 0;background:var(--border);height:4px;border-radius:2px"><div style="background:var(--accent-war);height:4px;width:' + pct + '%"></div></div>';
  const dim = (txt) => '<span style="font-size:.65rem;color:var(--text-dim)">' + txt + '</span>';
  let progress = '';
  if (isNaval && isForming) {
    progress = dim('building — ' + readyWord(u.build_complete_at));
  } else if (isForming) {
    // men_to_deploy is server-computed (100 − size); recruiting more of the
    // same type into this settlement is what fills it — say so, so a
    // half-formed unit doesn't read as a stuck pipeline.
    const needed = u.men_to_deploy != null ? u.men_to_deploy : (100 - u.size);
    progress = bar(u.size) + dim(u.size + '/100 · forming — ' + needed + ' more men needed before training starts');
  } else if (isTraining) {
    progress = bar(100) + dim('100/100 · training — ' + readyWord(u.build_complete_at));
  } else if (isGarrison && u.reinforcing) {
    // Manskaps-underhåll (megaron_plan_rekryteringsmodell.md): a decimated
    // cohort trickles back to 100 out of its home city's population growth,
    // a few men per game-day — not a stuck pipeline, just slow by design.
    // (No separate origin-city name field on the wire — the server exposes
    // origin_settlement_id, not a name, so this stays generic.)
    progress = bar(u.size) + dim(u.size + '/100 · reinforcing — refilling from home-city growth');
  }

  // Pending order (Fas 5): a Runner is en route to this unit — the order
  // executes only on delivery; surface the courier ETA on the card.
  const runner = (State.messengerData || []).find(m => m.own && m.kind === 'order' && m.order_unit_id === u.id);
  let pendingOrder = '';
  if (runner) {
    // Once the runner has arrived, the order is being applied server-side (a
    // worker poll away) — say "delivering", not the stale "en route" ETA.
    const arrived = serverNow() >= new Date(runner.arrives_at).getTime();
    pendingOrder = arrived
      ? '<div style="font-size:.65rem;color:var(--text-dim)">🏃 Runner levererar ordern…</div>'
      : '<div style="font-size:.65rem;color:var(--text-dim)">🏃 Runner en route — order arrives ' + arrivalHTML(runner.arrives_at) + '</div>';
  }

  // Stance badge
  const stanceBadge = u.stance
    ? '<span style="font-size:.6rem;padding:.1rem .25rem;border:1px solid var(--border);color:var(--text-dim);margin-left:.2rem">' + u.stance + '</span>'
    : '';

  // Crew badge for naval
  const crewBadge = isNaval && u.crew
    ? '<span style="font-size:.6rem;color:var(--text-dim);margin-left:.3rem">crew ' + u.crew + '</span>'
    : '';

  // Hull badge (megaron_plan_skeppsreparation.md §B2) — only shown while
  // damaged (hull < 5); a pristine ship (hull omitted or 5) shows nothing,
  // same "don't clutter the common case" posture as crewBadge above.
  const hullBadge = isNaval && u.hull != null && u.hull < 5
    ? '<span style="font-size:.6rem;color:var(--accent-war);margin-left:.3rem">hull ' + u.hull + '/5</span>'
    : '';

  // Matmätaren (megaron_plan_skeppsproviant.md §7, Timothy 2026-08-26). Dygn,
  // inte råa korn — resten av spelet mäts i speldygn.
  //
  // Döljs i hamn med flit: där äter skeppet ur stadens magasin, så ett lager
  // ombord säger ingenting. Till sjöss visas den ALLTID, inte bara när den är
  // låg — till skillnad från hull-bricka ovan. Skälet är att det här är talet
  // som avgör om du kan ge en order alls, och ett skepp som tyst närmar sig
  // noll är precis det mekaniken finns för att göra synligt.
  const atSea = isNaval && u.status !== 'garrison' && u.status !== 'repairing';
  const days = u.provision_days || 0;
  const foodBadge = atSea
    ? '<span style="font-size:.6rem;margin-left:.3rem;color:' +
      (days <= 0 ? 'var(--accent-war)' : days < 3 ? 'var(--accent-war)' : 'var(--text-dim)') + '">' +
      (days <= 0 ? 'out of food' : 'food ' + days + 'd') + '</span>'
    : '';

  // Cargo badge
  const cargoBadge = u.cargo_unit_id
    ? '<span style="font-size:.6rem;color:var(--accent-city);margin-left:.3rem">carrying unit</span>'
    : '';

  // Action buttons
  let actions = '';

  // March button: garrison or positioned, deployable per the
  // server's own march grind (march_start.go:120-136 — status must be garrison
  // or positioned, and fortify stance blocks it). The server has NO size gate:
  // a battle-worn cohort below 100 men can still march, so the client must not
  // invent one either — u.deployable is the field the server already computes
  // (`status != forming && status != training`, api/handlers/unit.go:1342).
  // Positioned units (out on the map, e.g. a ship that finished a plain march)
  // must be orderable too — otherwise they're stranded. The server already
  // allows marching a positioned unit; this just surfaces the button. (The
  // map right-click used to read the unit's own hex as the target — fixed by
  // routing that click to warFocusUnit, landing here, render/map.js contextmenu.)
  const canMarch = (isGarrison || isPositioned) && u.deployable && u.stance !== 'fortify';
  if (canMarch) {
    actions += '<button onclick="unitMarch(\'' + u.id + '\')" style="padding:.15rem .35rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.65rem;cursor:pointer">March</button> ';
  }

  // Stance buttons: garrison or positioned, land only — naval
  // units carry no stance (ship-build overhaul 2026-07-09).
  if ((isGarrison || isPositioned) && !isNaval) {
    actions += '<select id="ustance-' + u.id + '" style="font-size:.65rem;padding:.1rem;border:1px solid var(--border);background:var(--warm-white)">'
      + '<option value="none">stance…</option>'
      + '<option value="fortify">fortify</option>'
      + '<option value="storm">storm</option>'
      + '<option value="sentry">sentry</option>'
      + (u.stance ? '<option value="none">— clear</option>' : '')
      + '</select> '
      + '<button onclick="unitStance(\'' + u.id + '\')" style="padding:.15rem .35rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.65rem;cursor:pointer">Set</button> ';
  }

  // Reinforce button (megaron_plan_rekryteringsmodell.md): only when the
  // server says this cohort can actually receive it — garrisoned in its own
  // origin city and under 100 men (can_reinforce mirrors the endpoint's own
  // gate exactly, api/handlers/unit.go Reinforce). Hidden once reinforcing is
  // already true — the progress bar above says the same thing, a second
  // button to press would just re-flip a flag that's already set.
  if (isGarrison && u.can_reinforce && !u.reinforcing) {
    actions += '<button onclick="unitReinforce(\'' + u.id + '\')" style="padding:.15rem .35rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.65rem;cursor:pointer">Reinforce</button> ';
  }

  // Load button: naval garrison without cargo — pick from co-located garrison land units
  if (isNaval && isGarrison && !u.cargo_unit_id && u.settlement_id) {
    actions += '<button onclick="unitLoadPrompt(\'' + u.id + '\',\'' + (u.settlement_id||'') + '\')" style="padding:.15rem .35rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.65rem;cursor:pointer">Load</button> ';
  }

  // Unload button: naval garrison with cargo
  if (isNaval && isGarrison && u.cargo_unit_id) {
    actions += '<button onclick="unitUnload(\'' + u.id + '\')" style="padding:.15rem .35rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.65rem;cursor:pointer">Unload</button> ';
  }

  // Repair button (megaron_plan_skeppsreparation.md Slice C): naval garrison
  // with hull < 5. The server rejects it if the settlement has no shipyard
  // or the yard is full — this button only knows the ship is damaged and
  // docked, same "let the server be the judge" posture as Load/Unload.
  if (isNaval && isGarrison && u.hull != null && u.hull < 5) {
    actions += '<button onclick="unitRepair(\'' + u.id + '\')" style="padding:.15rem .35rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.65rem;cursor:pointer">Repair</button> ';
  }

  // Recall/redirect: marching units only. The order travels by messenger —
  // it does not apply instantly (temenos_settlement.md load-bearing pillar).
  let redirectRow = '';
  if (isMarching) {
    actions += '<button onclick="unitRecall(\'' + u.id + '\')" style="padding:.15rem .35rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.65rem;cursor:pointer">Recall</button> ';
    actions += '<button onclick="unitRedirectToggle(\'' + u.id + '\')" style="padding:.15rem .35rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.65rem;cursor:pointer">Redirect</button> ';
    redirectRow = '<div id="uredir-' + u.id + '" style="display:none;margin-top:.2rem;gap:.25rem;align-items:center;font-size:.65rem">'
      + '<label>Q <input id="uredir-q-' + u.id + '" type="number" value="0" style="width:40px;padding:.1rem .2rem;border:1px solid var(--border);background:var(--warm-white);font-family:var(--mono);font-size:.65rem"></label>'
      + '<label>R <input id="uredir-r-' + u.id + '" type="number" value="0" style="width:40px;padding:.1rem .2rem;border:1px solid var(--border);background:var(--warm-white);font-family:var(--mono);font-size:.65rem"></label>'
      + '<button onclick="unitRedirect(\'' + u.id + '\')" style="padding:.1rem .3rem;border:1px solid var(--border);background:var(--accent-war);color:#fff;font-size:.65rem;cursor:pointer">Send order →</button>'
      + '</div>';
  }
  const orderStatus = '<div id="uorder-' + u.id + '" style="font-size:.65rem;color:var(--text-dim);margin-top:.15rem"></div>';

  return '<div id="ucard-' + u.id + '" style="padding:.3rem .2rem;border-bottom:1px solid var(--border)">'
    + '<div style="display:flex;align-items:center;gap:.3rem;flex-wrap:wrap">'
    + '<span style="font-size:.8rem;font-weight:bold">' + lbl + '</span>'
    + stanceBadge + crewBadge + hullBadge + foodBadge + cargoBadge
    + '<span style="font-size:.68rem;color:var(--text-dim);margin-left:auto">' + u.status + '</span>'
    + '</div>'
    + progress
    + (loc ? '<div style="font-size:.65rem;color:var(--text-dim)">' + loc + '</div>' : '')
    + pendingOrder
    + (actions ? '<div style="margin-top:.2rem;display:flex;gap:.2rem;flex-wrap:wrap;align-items:center">' + actions + '</div>' : '')
    + redirectRow + (isMarching ? orderStatus : '')
    + '</div>';
}

export async function warAbandon(settlementID, name) {
  if (!confirm('Abandon ' + name + '? This cannot be undone.')) return;
  const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/settlements/${settlementID}/abandon`, { method: 'POST' });
  const resEl = document.getElementById('war-abandon-res');
  if (res.ok) {
    await loadMap();
    loadWarDrawer();
  } else {
    const d = await res.json().catch(() => ({}));
    if (resEl) resEl.textContent = d.error || 'Abandon failed';
  }
}

// The recall/redirect endpoint answers in one of two shapes. Every ordinary
// unit gets 202 {status:"order_dispatched", courier_arrives_at} — a Runner is
// on its way and the order lands when it arrives. The nomadic host gets
// 200 {status:"order_applied"} with NO courier: Wanax travels with the host,
// so the order needs no messenger (unit.CommandedInPerson, server-side). Read
// the shape rather than assuming the courier field is there — printing
// "reaches the unit undefined" was the bug this replaces.
function orderSentLine(d, verb) {
  if (d && d.status === 'order_applied') {
    return verb + ' order given in person — you ride with the host, so it takes effect at once.';
  }
  return verb + ' order sent by Runner — reaches the unit ' + fmtArrival(d.courier_arrives_at) + '.';
}

export async function unitRecall(unitID) {
  const statusEl = document.getElementById('uorder-' + unitID);
  const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units/${unitID}/recall`, {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: '{}',
  });
  const d = await res.json().catch(() => ({}));
  if (res.ok) {
    if (statusEl) { statusEl.style.color = 'var(--safe)'; statusEl.textContent = orderSentLine(d, 'Recall'); }
  } else if (statusEl) {
    statusEl.style.color = 'var(--accent)';
    statusEl.textContent = d.error || 'Recall failed';
  }
}

export function unitRedirectToggle(unitID) {
  const row = document.getElementById('uredir-' + unitID);
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
    if (statusEl) { statusEl.style.color = 'var(--safe)'; statusEl.textContent = orderSentLine(d, 'Redirect'); }
  } else if (statusEl) {
    statusEl.style.color = 'var(--accent)';
    statusEl.textContent = d.error || 'Redirect failed';
  }
}

export function unitMarch(unitID) {
  _marchUnitID = unitID;
  const panel = document.getElementById('war-march-panel');
  if (panel) {
    // The panel renders once at the bottom of the army tab — below the fold as
    // soon as the unit list fills the drawer, so opening it looked like a dead
    // button. Move it directly under the clicked unit's card instead.
    const card = document.getElementById('ucard-' + unitID);
    if (card) card.after(panel);
    panel.style.display = '';
    document.getElementById('wmp-err').textContent = '';
    panel.scrollIntoView({ block: 'nearest' });
    document.getElementById('wmp-q')?.focus();
  }
}

export function closeMarchPanel() {
  _marchUnitID = null;
  const panel = document.getElementById('war-march-panel');
  if (panel) panel.style.display = 'none';
}

export async function unitMarchSend() {
  if (!_marchUnitID) return;
  const q = parseInt(document.getElementById('wmp-q').value, 10);
  const r = parseInt(document.getElementById('wmp-r').value, 10);
  const stance = document.getElementById('wmp-stance').value || undefined;
  const errEl = document.getElementById('wmp-err');
  errEl.textContent = '';
  const body = { target_q: q, target_r: r };
  if (stance) body.stance = stance;
  const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units/${_marchUnitID}/march`, {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify(body),
  });
  const data = await res.json().catch(() => ({}));
  if (res.ok) {
    track('march_sent', { intent: stance || 'march' });
    closeMarchPanel();
    loadWarDrawer();
  } else {
    errEl.textContent = formatApiError(data, 'March failed');
  }
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
        resEl.textContent = '🏃 Runner carries the stance order — applies on delivery';
      }
      fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/messengers`).then(r => r.ok && r.json().then(d => { State.messengerData = d; State.dirty = true; }));
    }
    loadWarDrawer();
  } else if (resEl) {
    resEl.style.color = 'var(--accent)';
    resEl.textContent = formatApiError(data, 'Stance change failed');
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
