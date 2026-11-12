import { BUILDING_LABELS as _BLD_LBL } from '../building_names.js';
import { disbandUnitsHTML } from '../disband.js';
import { showInlineResult, confirmInline } from '../inline_result.js';
import { demolishLabel, demolishPrompt, demolishResultLine } from '../demolish.js';
import { State, activeCitySettlement } from '../../state.js';
import { fetchAuth } from '../../api.js';
import { arrivalHTML, fmtArrival } from '../time.js';
import { fmtTickTime } from '../fmt_clock.js';
import { renderLockedActions } from '../misc.js';
import { esc, formatApiError, playerError, errorDetail } from '../format.js';
import { unitTypeLabel } from '../actornames.js';
import { startCityAnim } from '../../render/city.js';
import { renderGubbeGrid, shipyardShareNote, setBuildHandler } from '../citygrid.js';
import { buildBody, constructListHTML, getBuildingCatalogue, loadStock, showBuildResult, templeName } from '../build_rows.js';
import { productionSectionsHTML, populationHTML, blessingsSectionHTML, foodSummaryHTML, foodDetailsHTML, storageWarningsHTML } from './city_production.js';
import { warehouseHTML } from './city_warehouse.js';
import { fmtNum, fmtDays, fmtDuration } from '../fmt_num.js';
import { ownBoardSection } from '../market_board.js';

// The settlement the City drawer currently shows: cycle via the drawer's
// prev/next arrows. Defaults to the capital (activeCitySettlement, state.js).
export function cycleCityView(dir) {
  const mine = State.provinceData.filter(p => p.own && !p.is_outpost);
  if (mine.length < 2) return;
  const current = activeCitySettlement();
  let idx = mine.findIndex(p => p.id === current.id);
  idx = (idx + dir + mine.length) % mine.length;
  State.cityViewID = mine[idx].id;
  State.dirty = true; // the map's catchment tint follows the shown city
  loadCityDrawer();
}


let cityLoadSeq = 0;
export async function loadCityDrawer() {
  const seq = ++cityLoadSeq;
  const capital = activeCitySettlement();
  // Get terrain from State.tileData (not in State.provinceData)
  const capitalTile = capital ? State.tileData.find(t => t.q === capital.q && t.r === capital.r) : null;
  const terrainLabel = capitalTile
    ? capitalTile.terrain.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase())
    : '—';

  // Update drawer title dynamically for city
  const titleEl = document.getElementById('city-drawer-title');
  if (titleEl) titleEl.textContent = capital ? `City — ${capital.name}${capital.is_capital ? ' ★' : ''}` : 'City';
  const mineCount = State.provinceData.filter(p => p.own && !p.is_outpost).length;
  document.querySelectorAll('#drawer-city .drawer-nav').forEach(b => b.disabled = mineCount < 2);

  const body = document.getElementById('city-body');
  if (!capital) {
    body.innerHTML = '<p class="empty-state" style="padding:1rem">No city founded yet. Click a hex to colonize.</p>';
    return;
  }

  // Capitalized keys match province API's ArmyComposition struct field names.
  const POP_COSTS = { Spearman:5, EliteInfantry:10, WarChariot:8, Ship:10, WarGalley:12, Merchantman:8 };
  const UNIT_DP   = { Spearman:1, EliteInfantry:3,  WarChariot:4, Ship:1,  WarGalley:3,  Merchantman:0 };

  body.innerHTML = `
    <canvas id="city-scene" class="city-scene"></canvas>
    <div id="city-siege-banner"></div>
    <div class="drawer-tabs">
      <button class="dtab active" data-tab="produktion">Production</button>
      <button class="dtab" data-tab="forrad">Warehouse</button>
      <button class="dtab" data-tab="byggnader">Buildings</button>
      <button class="dtab" data-tab="garnison">Garrison</button>
    </div>
    <div id="ctab-produktion" class="city-tab">
      ${productionSectionsHTML(capital)}
    </div>
    <div id="ctab-forrad" class="city-tab" style="display:none"><div id="city-wh-sec"></div></div>
    <div id="ctab-byggnader" class="city-tab" style="display:none"><div id="city-bld-sec"><div class="loading" style="font-size:.8rem">Loading…</div></div></div>
    <div id="ctab-garnison"  class="city-tab" style="display:none"><div id="city-gar-sec"><div class="loading" style="font-size:.8rem">Loading…</div></div></div>`;

  body.querySelectorAll('.dtab').forEach(tab => {
    tab.addEventListener('click', function() {
      body.querySelectorAll('.dtab').forEach(t => t.classList.remove('active'));
      this.classList.add('active');
      body.querySelectorAll('.city-tab').forEach(c => c.style.display = 'none');
      const el = document.getElementById('ctab-' + this.dataset.tab);
      if (el) el.style.display = '';
    });
  });

  const capitalTile2 = State.tileData.find(t => t.q === capital.q && t.r === capital.r);

  try {
    const [settResp, goodsResp, unitsResp] = await Promise.all([
      fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${capital.id}`),
      fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${capital.id}/goods`),
      fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units`),
    ]);
    const pd    = settResp.ok  ? (await settResp.json()).settlement : null;
    const goods = goodsResp.ok ? await goodsResp.json() : [];
    const units = unitsResp.ok ? (await unitsResp.json()).units || [] : [];
    if (seq !== cityLoadSeq) return;

    // Canvas — start animated city scene.
    // `pd` skickas med: scenens citadell skalas av befolkning och murnivå ur
    // SAMMA sanning som kartans size_tier, så kartan och stadsvyn aldrig kan
    // säga olika saker om samma stad.
    startCityAnim(document.getElementById('city-scene'), capitalTile2,
                  pd ? pd.buildings : [], pd ? pd.build_queue : [], pd);

    // Belägring S1+S2 (megaron_plan_belagring.md): NOT FOW-gated — a besieged
    // Wanax learns this from the flag + falling production, not from sight,
    // so it always shows when the server says besieged, regardless of what
    // the client can otherwise see on the map.
    const siegeBanner = document.getElementById('city-siege-banner');
    if (siegeBanner) {
      if (pd && pd.besieged) {
        const who = (pd.besieged_by || [])
          .map(b => `${b.size} ${unitTypeLabel(b.unit_type)} (${b.owner_name || 'unknown'})`)
          .join(', ') || 'an unseen enemy';
        siegeBanner.innerHTML = `<div class="attack-warn">⚔ BESIEGED — a chokepoint is held by ${who}. Catchment production is cut off.</div>`;
      } else {
        siegeBanner.innerHTML = '';
      }
    }

    // ── Produktion ──────────────────────────────────────────────────────────
    if (pd) {
      const army    = pd.army || {};
      const armyPop = Object.entries(POP_COSTS).reduce((s,[k,c]) => s + (army[k]||0)*c, 0);
      const idle    = goods[0] ? (goods[0].idle_citizens || 0) : 0;
      // S1c (megaron_plan_foda_konsistens.md): livestock's strongest sink is a
      // deliberate Wanax choice, never automatic — trade one animal for +10
      // population right now. Button disabled at 0 rather than hidden, so the
      // herd's role as a growth lever stays visible even when empty.
      const livestockGood = goods.find(g => g.key === 'livestock');
      const livestock = livestockGood ? livestockGood.amount : 0;
      document.getElementById('city-pop-sec').innerHTML = populationHTML(pd, idle, livestock, capital.id);
      document.getElementById('city-people-details').innerHTML = `<div class="stat-row"><span class="sr-label">In service</span><span class="sr-val">${fmtNum(armyPop)}</span></div><div class="stat-row"><span class="sr-label">People</span><span class="sr-val">${fmtNum(pd.population)}</span></div>`;
    } else {
      document.getElementById('city-pop-sec').innerHTML = '<p class="empty-state">—</p>';
    }

    document.getElementById('city-food-sec').innerHTML = foodSummaryHTML(pd);
    document.getElementById('city-blessings-sec').innerHTML = blessingsSectionHTML(pd);
    document.getElementById('city-wh-sec').innerHTML = warehouseHTML(pd, goods);
    document.getElementById('city-food-details-sec').innerHTML = foodDetailsHTML(pd);
    if (pd && pd.last_tick) {
      const lt = pd.last_tick;
      const prodRows = Object.entries(lt.production || {}).sort(([a],[b]) => a.localeCompare(b)).map(([k,v]) => `<tr><td>${k}</td><td style="color:var(--safe-text)">+${fmtNum(Number(v.toFixed(2)))}</td></tr>`).join('');
      const consRows = Object.entries(lt.consumption || {}).sort(([a],[b]) => a.localeCompare(b)).map(([k,v]) => `<tr><td>${k}</td><td style="color:var(--accent)">−${fmtNum(Number(v.toFixed(2)))}</td></tr>`).join('');
      document.getElementById('city-lasttick-sec').innerHTML = `
        <div class="stat-row"><span class="sr-label">Counted</span><span class="sr-val">${esc(fmtTickTime(lt.tick))}</span></div>
        ${(prodRows||consRows) ? `<table class="goods-mini" style="margin-top:.3rem">${prodRows}${consRows}</table>` : ''}`;
    } else {
      document.getElementById('city-lasttick-sec').innerHTML = '<p class="empty-state">—</p>';
    }


    document.getElementById('city-prod-sec').innerHTML = storageWarningsHTML(pd) + '<div id="city-gubbe-grid"><div class="loading">Loading…</div></div>';


    // ── Gubbe placement (P5) ────────────────────────────────────────────────
    const gridEl = document.getElementById('city-gubbe-grid');
    if (gridEl) renderGubbeGrid(gridEl, capital.id, capital.q, capital.r);

    // ── Byggnader ───────────────────────────────────────────────────────────
    // Delegate to refreshCityBuildings so startBuild() can call it too
    // without resetting the active drawer tab.
    refreshCityBuildings(capital.id);

    // ── Garnison ────────────────────────────────────────────────────────────
    if (pd) {
      document.getElementById('city-gar-sec').innerHTML = `
        <div class="dsec-title">Whole forces in ${esc(capital.name)} <button class="btn-small" onclick="warOpenRecruit('${esc(capital.id)}')">Recruit</button></div>
        ${unitsResp.ok ? disbandUnitsHTML(units, capital.settlement_id || pd.id, capital.id) : '<p class="empty-state">Could not load forces. Reopen the city to try again.</p>'}
        <div id="war-disband-res" class="action-result" role="status"></div>`;
    }

    const locked = await renderLockedActions('province', capital.id);
    if (seq !== cityLoadSeq) return;
    document.getElementById('city-gar-sec').insertAdjacentHTML('beforeend', locked);

  } catch(e) { console.error('city drawer', e); }
}

// ── City build action ─────────────────────────────────────────────────────

// The one function that sends a build. spec = { type, god?, hex_q?, hex_r? };
// the request body is buildBody(spec) (unchanged wire format). Called from the
// Construct list's [Build] and from the placement grid's rows. Returns
// { ok, text } so the caller can show the outcome where the player clicked.
export async function startBuild(spec) {
  const world = State.WORLD_ID;
  const capital = activeCitySettlement();
  if (!capital || !spec || !spec.type) return { ok: false, text: 'No city to build in.' };
  const { type: btype, god } = spec;
  const resultEl = document.getElementById('city-build-result');
  if (resultEl) resultEl.textContent = '';
  const hexLabel = spec.hex_q != null ? ` @ (${spec.hex_q},${spec.hex_r})` : '';
  const r = await fetchAuth(
    `/api/v1/worlds/${State.WORLD_ID}/provinces/${capital.id}/build`,
    { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify(buildBody(spec)) }
  );
  const d = await r.json().catch(() => ({}));
  if (r.ok) {
    const receipt = `${btype === 'temple' ? templeName(god) : (_BLD_LBL[btype]||btype)} queued${hexLabel}${d.duration_game_days != null ? ' · ' + fmtDuration(d.duration_game_days) : ''}${d.complete_at ? ' · ready ' + fmtArrival(d.complete_at, d.complete_tick, 'ready') : d.active === false ? ' · starts when the current build is done' : ''}.`;
    if (resultEl) { resultEl.style.color = 'var(--safe-text)'; resultEl.textContent = receipt; }
    // Refresh only the buildings section — avoids resetting the active tab
    await refreshCityBuildings(capital.id);
    // Refresh replaces this section, including its receipt node.
    if (State.WORLD_ID === world && activeCitySettlement()?.id === capital.id) {
      const currentResult = document.getElementById('city-build-result');
      if (currentResult) { currentResult.style.color = 'var(--safe-text)'; currentResult.textContent = receipt; }
    }
    return { ok: true, text: receipt };
  }
  const text = playerError(r, d, 'Build failed.');
  if (resultEl) { resultEl.style.color = 'var(--accent)'; resultEl.textContent = text; }
  return { ok: false, text, detail: errorDetail(r, d) };
}
// The placement grid builds through the same function.
setBuildHandler(startBuild);

// [Choose a hex →]: show the Production tab with the grid in pick mode for `type`.
export function pickBuildHex(type) {
  const capital = activeCitySettlement();
  const gridEl = document.getElementById('city-gubbe-grid');
  if (!capital || !gridEl) return;
  document.querySelector('#city-body [data-tab="produktion"]')?.click();
  renderGubbeGrid(gridEl, capital.id, capital.q, capital.r, { pick: { type } });
  gridEl.scrollIntoView?.({ block: 'start' });
}


// Recipe catalogue (GET /api/v1/recipes) — static for a world's lifetime, so
// fetch once and memoize rather than refetching on every drawer render (same
// reasoning as the buildings/units catalogues, which nothing in this client
// consumed yet — this is the first consumer, so the memoization pattern is
// new here rather than copied). `_recipesPromise` is the memo; concurrent
// callers before the first fetch resolves share the same in-flight request.
// On failure the promise is cleared so the next render retries instead of
// permanently caching a failure.
let _recipesPromise = null;
async function getRecipes() {
  if (!_recipesPromise) {
    _recipesPromise = fetchAuth('/api/v1/recipes').then(r => {
      if (!r.ok) throw new Error('recipes fetch failed: ' + r.status);
      return r.json();
    }).catch(e => {
      console.error('getRecipes', e);
      _recipesPromise = null;
      return null; // null = fetch failed; [] would mean "server has no recipes"
    });
  }
  return _recipesPromise;
}


export async function loadTicklog() {
  const capital = activeCitySettlement();
  const el = document.getElementById('city-ticklog-sec');
  if (!capital || !el) return;
  el.innerHTML = '<div class="loading" style="font-size:.8rem">Loading…</div>';
  try {
    const r = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${capital.id}/ticklog?last=10`);
    if (!r.ok) { el.innerHTML = '<p class="empty-state">Could not load.</p>'; return; }
    const data = await r.json();
    const ticks = data.ticks || [];
    if (!ticks.length) { el.innerHTML = '<p class="empty-state">No daily history yet.</p>'; return; }
    // The booked last game day (server ledger): one line per post, "Other" last and
    // only when it is not ~0. Earlier days list only their events.
    const led = data.ledger || {};
    const ledgerHtml = !led.known
      ? '<p class="empty-state">The books for the last game day are not ready yet.</p>'
      : `<p>Last game day, ending ${fmtTickTime(led.tick)}</p>` + (led.goods || []).map(g =>
        `<table class="goods-mini ledger-table"><tr><th>${esc(g.good)}</th><th class="num">${g.delta >= 0 ? '+' : ''}${fmtNum(Number(g.delta.toFixed(1)))}</th></tr>` +
        (g.posts || []).map(p => `<tr><td>${esc(p.label)}</td><td class="num">${p.amount >= 0 ? '+' : ''}${fmtNum(Number(p.amount.toFixed(1)))}</td></tr>`).join('') +
        (g.other ? `<tr class="ledger-other"><td>Other</td><td class="num">${g.other >= 0 ? '+' : ''}${fmtNum(Number(g.other.toFixed(1)))}</td></tr>` : '') +
        '</table>').join('');
    el.innerHTML = `${ledgerHtml}
    <table class="goods-mini">
      ${ticks.map(t => {
        const evs  = (t.events||[]).map(e => e.type).join(', ');
        return `<tr><td>${fmtTickTime(t.tick)}</td><td>${evs || '—'}</td></tr>`;
      }).join('')}
    </table>`;
  } catch (_) {
    el.innerHTML = '<p class="empty-state">Could not load.</p>';
  }
}

export async function cancelBuild(provinceID, queueID) {
  const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${provinceID}/build-queue/${queueID}`, { method: 'DELETE' });
  if (res.ok) {
    await refreshCityBuildings(provinceID);
  } else {
    const d = await res.json().catch(() => ({}));
    showInlineResult('city-bld-sec', playerError(res, d, 'Could not cancel build'), true, errorDetail(res, d));
  }
}

// Demolish a finished building: a deliberate second click (inline confirm, no
// browser dialog), then DELETE .../buildings/:id. The server lifts the people
// working there to idle, so the gubbe grid is redrawn too.
export async function demolishBuilding(provinceID, buildingID, label) {
  const host = document.getElementById('city-demolish-res');
  // Fetch the owner-only quote at the moment of confirmation, including after a
  // buildings refresh; province summaries do not carry demolition consequences.
  const quoteRes = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${provinceID}/buildings`);
  if (!quoteRes.ok) { showInlineResult('city-bld-sec', 'Could not load demolition consequences. Try again.', true); return; }
  const buildings = await quoteRes.json();
  const building = buildings.find(b => b.id === buildingID);
  if (!building) { showInlineResult('city-bld-sec', 'This building no longer stands.', true); return; }
  if (building.type === 'temple' && !building.demolition_kharis) {
    showInlineResult('city-bld-sec', 'Temple consequences are unavailable. Try again.', true); return;
  }
  confirmInline(host, demolishPrompt(label, building.demolition_kharis), async () => {
    const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${provinceID}/buildings/${buildingID}`, { method: 'DELETE' });
    if (res.ok) {
      const d = await res.json().catch(() => ({}));
      await refreshCityBuildings(provinceID);
      const gridEl = document.getElementById('city-gubbe-grid');
      const c = activeCitySettlement();
      if (gridEl && c) renderGubbeGrid(gridEl, c.id, c.q, c.r);
      showInlineResult('city-bld-sec', demolishResultLine(label, d.citizens_lifted, d.demolition_kharis));
    } else {
      const d = await res.json().catch(() => ({}));
      showInlineResult('city-bld-sec', playerError(res, d, 'Could not demolish'), true, errorDetail(res, d));
    }
  }, { confirmLabel: 'Demolish', cancelLabel: 'Keep it' });
}

// Re-fetch province data and update only the buildings/queue section of the city drawer.
async function refreshCityBuildings(provinceID) {
  const bldSec = document.getElementById('city-bld-sec');
  if (!bldSec) return;
  try {
    const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${provinceID}`);
    if (!res.ok) return;
    const pd = (await res.json()).settlement;
    if (!pd) return;
    const blds = pd.buildings || [], bq = pd.build_queue || [], tu = pd.training_units || [];
    // hex_q/hex_r are set only for hex-bound buildings (farm/mine/lumbermill/
    // stonequarry) — null for a city building (megaron_plan_byggnad_pa_hex.md
    // §A). Two of the same type at different hexes level up independently,
    // so the hex is the only way to tell them apart in this list.
    let h2 = blds.length
      ? `<div class="dsec-title">Built</div><table class="goods-mini">${
          blds.map(b => `<tr><td>${esc(b.name||_BLD_LBL[b.type]||b.type)}${b.hex_q != null ? ` @ (${b.hex_q},${b.hex_r})` : ''}</td><td>L${b.level}</td>` +
            (b.id ? `<td style="text-align:right"><button class="btn-small" onclick="demolishBuilding('${provinceID}','${b.id}','${esc(demolishLabel(b.name||_BLD_LBL[b.type]||b.type, b.hex_q, b.hex_r))}')" style="padding:.05rem .3rem;font-size:.68rem;cursor:pointer">Demolish</button></td>` : '') +
            `</tr>`).join('')
        }</table><div id="city-demolish-res"></div>`
      : '<p class="empty-state">No buildings yet.</p>';
    if (pd.market_board) h2 += await ownBoardSection(pd.market_board, pd.id);
    if (pd.build_queue_max != null) h2 += `<p class="empty-state">Queue: ${bq.length}/${pd.build_queue_max} — one building at a time, the rest wait their turn. Builders in a Workshop speed the active build up (up to 3x). Finish or cancel a build to free a place.</p>`;
    if (bq.length) h2 += `<div class="dsec-title" style="margin-top:.8rem">Build queue</div><table class="goods-mini">${
      // A building finishes, it doesn't arrive — say "ready" (matches the
      // Training column below and war.js's ship-build phrasing). Only the
      // active build has a finish time; the others say "next" until they start.
      bq.map(b => `<tr><td>${esc(b.name||_BLD_LBL[b.type]||b.type)} · ${esc(b.settlement_name||pd.name||'this city')}${b.hex_q != null ? ` at (${b.hex_q},${b.hex_r})` : ''}</td><td>${b.active === false || !b.complete_at ? `next in line · ${fmtNum(b.work_left_game_days ?? 0)} game days of work` : `${b.rate > 1 ? `${b.rate}x pace · ` : ''}${arrivalHTML(b.complete_at, undefined, 'ready')}${b.blessed ? ' · Builder\'s Blessing' : ''}`}</td>` +
        `<td style="text-align:right"><button class="btn-small" onclick="cancelBuild('${provinceID}','${b.id}')" style="padding:.05rem .3rem;font-size:.68rem;cursor:pointer">✕</button></td></tr>`).join('')
    }</table>`;
    if (tu.length) {
      // One row per maturing unit: land gathers men (forming, X/100), then trains
      // (100/100, ready ETA), then deploys to garrison; naval builds a vessel.
      h2 += `<div class="dsec-title" style="margin-top:.8rem">Training</div><table class="goods-mini">${
        tu.map(u => {
          const name = unitTypeLabel(u.unit);
          // A trained unit is ready, not arrived.
          let label, eta = !u.shipyard_waiting && u.ready_at ? arrivalHTML(u.ready_at, undefined, 'ready') : '';
          if (u.shipyard_waiting) label = 'waiting for shipyard workers';
          else if (u.category === 'naval') label = 'building' + (shipyardShareNote(u.shipyard_workers, u.shipyard_sharing) ? ' · ' + shipyardShareNote(u.shipyard_workers, u.shipyard_sharing) : '');
          else if (u.status === 'training') label = `${u.size}/100 · training`;
          // Say what's missing, not just the raw count — recruiting more of
          // the same type into this settlement is what fills it (a
          // half-formed unit otherwise reads as a stuck pipeline).
          else label = `${u.size}/100 · forming — ${100 - u.size} more needed`;
          return `<tr><td>${name}</td><td>${label}</td><td>${eta}</td></tr>`;
        }).join('')
      }</table>`;
    }
    // Recipes are fetched once regardless of whether a foundry is built —
    // also needed below for the Construct dropdown's foundry purpose label,
    // so a recipe change never leaves a stale string there either.
    const [recipes, buildingCatalogue, stock] = await Promise.all([getRecipes(), getBuildingCatalogue(), loadStock(provinceID)]);
    const foundryRecipe = recipes ? recipes.find(rc => rc.building_type === 'foundry') : null;

    if (blds.some(b => b.type === 'foundry')) {
      const res = pd.resources || {};
      if (foundryRecipe) {
        const outputName = foundryRecipe.output_key.charAt(0).toUpperCase() + foundryRecipe.output_key.slice(1);
        const ingredientsStr = foundryRecipe.ingredients.map(i => `${i.quantity} ${i.good_key}`).join(' + ');
        const stockStr = foundryRecipe.ingredients
          .map(i => `${((res[i.good_key] || {}).amount || 0).toFixed(0)} ${i.good_key}`)
          .join(', ');
        h2 += `
          <div class="dsec-title" style="margin-top:.8rem">Foundry — ${outputName}</div>
          <div style="font-size:.72rem;color:var(--text-dim);margin-bottom:.3rem">${ingredientsStr} → ${foundryRecipe.output_qty} ${foundryRecipe.output_key} · stock: ${stockStr}</div>
          <p class="empty-state" style="font-size:.72rem">Smelted on its own every game day — put a worker in the foundry and keep the ingredients in store.</p>`;
      } else {
        // Degrade honestly: no fabricated ratio.
        h2 += `
          <div class="dsec-title" style="margin-top:.8rem">Foundry</div>
          <p class="empty-state" style="font-size:.72rem">Recipe data unavailable — try reloading.</p>`;
      }
    }
    const queueFull = pd.build_queue_max != null && bq.length >= pd.build_queue_max;
    const constructHTML = constructListHTML(buildingCatalogue, {
      walls: pd.walls, siloLevel: pd.silo_level, buildQuotes: pd.build_quotes, canAfford: pd.can_afford, queueFull, stock,
    });
    h2 += `
      <div class="dsec-title" style="margin-top:.8rem">Construct</div>
      ${constructHTML}`;
    bldSec.innerHTML = h2;
    bldSec.querySelectorAll('[data-build-act]').forEach(btn => btn.addEventListener('click', async () => {
      const { buildType: type, buildGod: god } = btn.dataset;
      if (btn.dataset.buildAct === 'pick') { pickBuildHex(type); return; }
      const spec = type === 'temple' ? { type, god } : { type };
      btn.disabled = true;
      const out = await startBuild(spec);
      btn.disabled = false;
      // The answer goes under the row that was clicked (the list was redrawn on success).
      const host = document.getElementById('city-bld-sec');
      if (!showBuildResult(host, spec, out.text, out.ok, out.detail)) showInlineResult('city-bld-sec', out.text, !out.ok, out.detail);
    }));
  } catch(e) { console.error('refreshCityBuildings', e); }
}

// Select the owning city before loading: a dispatch must never open the capital
// as a substitute for the city named in its payload.
export async function openCityFromDispatch(action) {
 const city=(State.provinceData||[]).find(c=>c.own&&c.id===action.id);
 if(!city)return;
 State.cityViewID=city.id;State.selectedHex={q:city.q,r:city.r};State.dirty=true;
 window.centreOn?.(city.q,city.r);window.openDrawer?.('city');
 await loadCityDrawer();
 document.querySelector('#city-body [data-tab="produktion"]')?.click();
 if(action.type==='staff'){
  const grid=document.getElementById('city-gubbe-grid');
  if(grid)await renderGubbeGrid(grid,city.id,city.q,city.r,{selected:action.hex?null:'city',targetHex:action.hex,buildingType:action.building,god:action.god});
  grid?.scrollIntoView({block:'start'});
 }
}
