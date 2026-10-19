import { showInlineResult, confirmInline } from '../inline_result.js';
import { demolishLabel, demolishPrompt, demolishResultLine } from '../demolish.js';
import { State, activeCitySettlement } from '../../state.js';
import { fetchAuth } from '../../api.js';
import { arrivalHTML } from '../time.js';
import { renderLockedActions } from '../misc.js';
import { esc, formatApiError } from '../format.js';
import { unitTypeLabel } from '../actornames.js';
import { startCityAnim } from '../../render/city.js';
import { renderGubbeGrid, terrainLabel } from '../citygrid.js';
import { productionSectionsHTML, populationHTML, foodSummaryHTML, foodDetailsHTML, storageWarningsHTML } from './city_production.js';
import { fmtNum } from '../fmt_num.js';

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

export async function saveLaborAlloc(provinceID) {
  const btn = document.getElementById('labor-save-btn');
  const msg = document.getElementById('labor-save-msg');
  const err = document.getElementById('labor-save-err');
  if (btn) btn.disabled = true;
  if (msg) msg.textContent = '';
  if (err) err.textContent = '';
  const percent = {};
  document.querySelectorAll('.labor-input').forEach(inp => {
    const v = parseFloat(inp.value||0)||0;
    // PUT .../labor is cult-only now (megaron_plan_riv_procentallokeringen.md
    // — production is set via placements, see citygrid.js). This is the only
    // .labor-input in the DOM (data-good="cult"); always name it explicitly,
    // even at 0, so an untouched devotion is echoed back rather than omitted.
    if (v > 0 || inp.dataset.good === 'cult') percent[inp.dataset.good] = v;
  });
  const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${provinceID}/labor`, {
    method: 'PUT',
    headers: {'Content-Type':'application/json'},
    body: JSON.stringify({percent}),
  });
  if (res.ok) {
    if (msg) msg.textContent = 'Saved!';
    // The PUT response only carries cult_percent/message now (server-side
    // narrowing, megaron_plan_riv_procentallokeringen.md). Re-read the
    // settlement so the shown value is the server's authoritative post-clamp
    // devotion (what was typed may have been above the temple's cap, or below
    // the 15%-per-level floor).
    const cultInp = document.getElementById('labor-input-cult');
    if (cultInp) {
      const r2 = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${provinceID}`);
      if (r2.ok) {
        const pd2 = (await r2.json()).settlement;
        if (pd2) {
          const devPct2 = Math.round((pd2.devotion || 0) * 100);
          const devCapPct2 = Math.round((pd2.devotion_capacity || 0) * 100);
          cultInp.value = devPct2;
          const cit = document.querySelector('.labor-cit[data-good="cult"]');
          if (cit) cit.textContent = fmtNum(Math.round((pd2.devotion || 0) * (pd2.labor_pool || 0)));
          const rateCell = document.getElementById('labor-rate-cult');
          if (rateCell) {
            const atCap = devPct2 >= devCapPct2;
            rateCell.textContent = `${fmtNum(devPct2)} percent of ${fmtNum(devCapPct2)} percent capacity${atCap ? ' · at cap — build a higher-level temple to devote more' : ''}`;
          }
        }
      }
    }
    setTimeout(() => { if (msg) msg.textContent = ''; }, 2000);
  } else {
    const body = await res.json().catch(() => ({}));
    if (err) err.textContent = body.error || 'Save failed';
  }
  if (btn) btn.disabled = false;
}

// Extracted from the old generic loadDrawerContent(name) dispatcher's
// `if (name === 'city') {...}` branch (plan: "en drawer = en modul") — the
// prelude that used to run unconditionally for every drawer name (capital/
// capitalTile/terrainLabel) was only ever consumed by this branch, so it
// moves here whole; the outer `if (name === 'city')` test is gone because
// this function is only ever called for the city drawer. `terrainLabel` is
// computed but does not appear to be read anywhere below — a pre-existing
// wart, not touched here.
// A newer load rebuilds the drawer DOM; a slower older reply must not fill
// (and so overwrite typed input in) the nodes that replaced its own.
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
      <button class="dtab" data-tab="byggnader">Buildings</button>
      <button class="dtab" data-tab="garnison">Garrison</button>
    </div>
    <div id="ctab-produktion" class="city-tab">
      ${productionSectionsHTML(capital)}
    </div>
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
    const [settResp, goodsResp] = await Promise.all([
      fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${capital.id}`),
      fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${capital.id}/goods`),
    ]);
    const pd    = settResp.ok  ? (await settResp.json()).settlement : null;
    const goods = goodsResp.ok ? await goodsResp.json() : [];
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
          .map(b => `${b.size} ${b.unit_type} (${b.owner_name || 'unknown'})`)
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
      const lp      = pd.labor_pool || 0;
      const idle    = goods[0] ? (goods[0].idle_citizens || 0) : 0;
      // S1c (megaron_plan_foda_konsistens.md): livestock's strongest sink is a
      // deliberate Wanax choice, never automatic — trade one animal for +10
      // population right now. Button disabled at 0 rather than hidden, so the
      // herd's role as a growth lever stays visible even when empty.
      const livestockGood = goods.find(g => g.key === 'livestock');
      const livestock = livestockGood ? livestockGood.amount : 0;
      document.getElementById('city-pop-sec').innerHTML = populationHTML(pd, idle, livestock, capital.id);
      document.getElementById('city-people-details').innerHTML = `<div class="stat-row"><span class="sr-label">In service</span><span class="sr-val">${fmtNum(armyPop)}</span></div><div class="stat-row"><span class="sr-label">Citizens (1 = 100 people)</span><span class="sr-val">${fmtNum(Math.floor(lp/100))}</span></div>`;
    } else {
      document.getElementById('city-pop-sec').innerHTML = '<p class="empty-state">—</p>';
    }

    document.getElementById('city-food-sec').innerHTML = foodSummaryHTML(pd);
    document.getElementById('city-food-details-sec').innerHTML = foodDetailsHTML(pd);
    if (pd && pd.last_tick) {
      const lt = pd.last_tick;
      const prodRows = Object.entries(lt.production || {}).map(([k,v]) => `<tr><td>${k}</td><td style="color:var(--safe-text)">+${fmtNum(Number(v.toFixed(2)))}</td></tr>`).join('');
      const consRows = Object.entries(lt.consumption || {}).map(([k,v]) => `<tr><td>${k}</td><td style="color:var(--accent)">−${fmtNum(Number(v.toFixed(2)))}</td></tr>`).join('');
      document.getElementById('city-lasttick-sec').innerHTML = `
        <div class="stat-row"><span class="sr-label">Day</span><span class="sr-val">#${fmtNum(lt.tick)}</span></div>
        ${(prodRows||consRows) ? `<table class="goods-mini" style="margin-top:.3rem">${prodRows}${consRows}</table>` : ''}`;
    } else {
      document.getElementById('city-lasttick-sec').innerHTML = '<p class="empty-state">—</p>';
    }

    const lp = pd ? (pd.labor_pool || 0) : 0;

    // ── Devotion (cult) ─────────────────────────────────────────────────────
    // Untouched by P5 (megaron_plan_fysisk_gubbemodell.md, explicit non-scope):
    // cult is temple labor, its own path (megaron_cult_ar_ingen_vara_plan.md),
    // never part of the gubbe-placement model. `pd.devotion`/
    // `pd.devotion_capacity` are weights (0..1) already on the settlement
    // payload fetched above. This used to share a table with the per-good
    // percent allocation rows P5 removes (DE2=B, 2026-08-07) — now its own
    // small block, logic unchanged.
    const devWeight  = pd ? (pd.devotion || 0) : 0;
    const devCapWt   = pd ? (pd.devotion_capacity || 0) : 0;
    const devPct     = Math.round(devWeight * 100);
    const devCapPct  = Math.round(devCapWt * 100);
    const hasTemple  = (pd && pd.buildings || []).some(b => b.type === 'temple');
    let devotionHTML;
    if (hasTemple && devCapWt > 0) {
      const atCap = devPct >= devCapPct;
      devotionHTML = `
        <div class="stat-row">
          <span class="sr-label">Devotion <span style="color:var(--text-dim);font-size:.65rem">(cult, additive)</span></span>
          <span class="sr-val">
            <input type="number" class="labor-input" id="labor-input-cult" data-good="cult"
              value="${devPct}" min="0" max="100" step="1"
              style="width:3.5rem;background:var(--bg-raised);border:1px solid var(--border);color:var(--text);padding:.15rem .3rem;font-size:.8rem;text-align:right">%
          </span>
        </div>
        <div style="font-size:.72rem;color:var(--text-dim)"><span class="labor-cit" data-good="cult">${fmtNum(Math.round(devWeight*lp))}</span> workers · <span id="labor-rate-cult">${fmtNum(devPct)} percent of ${fmtNum(devCapPct)} percent capacity${atCap ? ' · at cap — build a higher-level temple to devote more' : ''}</span></div>
        <div style="margin-top:.4rem;display:flex;gap:.4rem;align-items:center">
          <button id="labor-save-btn" onclick="saveLaborAlloc('${capital.id}')"
            style="padding:.3rem .7rem;background:var(--accent);border:none;color:#000;font-size:.8rem;cursor:pointer">
            Assign →
          </button>
          <span id="labor-save-msg" style="font-size:.75rem;color:var(--safe-text)"></span>
          <span id="labor-save-err" style="font-size:.75rem;color:var(--danger)"></span>
        </div>`;
    } else {
      // Mirrors the server's own 422 wording (province.go LaborAlloc, key=="cult"
      // branch) rather than inventing separate copy — no temple, no control.
      devotionHTML = `<p class="empty-state">Devotion (cult): needs a temple here — build one first.</p>`;
    }

    document.getElementById('city-devotion-sec').innerHTML = devotionHTML;
    document.getElementById('city-prod-sec').innerHTML = storageWarningsHTML(pd) + '<div id="city-gubbe-grid"><div class="loading">Loading…</div></div>';
    const cultInp = document.getElementById('labor-input-cult');
    if (cultInp) cultInp.addEventListener('input', () => {
      const cit = document.querySelector('.labor-cit[data-good="cult"]');
      if (cit) cit.textContent = fmtNum(Math.round((parseFloat(cultInp.value||0)||0)/100*lp));
    });

    // ── Gubbe placement (P5) ────────────────────────────────────────────────
    const gridEl = document.getElementById('city-gubbe-grid');
    if (gridEl) renderGubbeGrid(gridEl, capital.id, capital.q, capital.r);

    // ── Byggnader ───────────────────────────────────────────────────────────
    // Delegate to refreshCityBuildings so startBuild() can call it too
    // without resetting the active drawer tab.
    refreshCityBuildings(capital.id);

    // ── Garnison ────────────────────────────────────────────────────────────
    if (pd) {
      const army    = pd.army || {};
      const present = Object.keys(POP_COSTS).filter(k => (army[k]||0) > 0);
      const totalDP  = Object.entries(UNIT_DP).reduce((s,[k,d])  => s + (army[k]||0)*d, 0);
      const totalPop = Object.entries(POP_COSTS).reduce((s,[k,c]) => s + (army[k]||0)*c, 0);
      document.getElementById('city-gar-sec').innerHTML = present.length
        ? `<table class="goods-mini">
            <tr style="color:var(--text-dim);font-size:.7rem"><td>Unit</td><td style="text-align:right">Count</td><td style="text-align:right">Pop</td><td style="text-align:right">DP</td></tr>
            ${present.map(k => {
              const n = army[k]||0;
              return `<tr><td>${unitTypeLabel(k)}</td><td style="text-align:right">${n}</td><td style="text-align:right;color:var(--text-dim)">${n*POP_COSTS[k]}</td><td style="text-align:right;color:var(--safe-text)">${n*UNIT_DP[k]}</td></tr>`;
            }).join('')}
            <tr style="border-top:1px solid var(--border);font-weight:bold"><td>Total</td><td></td><td style="text-align:right;color:var(--accent)">${totalPop}</td><td style="text-align:right;color:var(--safe-text)">${totalDP} DP</td></tr>
          </table>`
        : '<p class="empty-state">No garrison. Recruit units in the War tab.</p>';
      document.getElementById('city-gar-sec').innerHTML += `
        <div class="dsec-title" style="margin-top:.8rem">Disband</div>
        <div style="display:flex;gap:.4rem;align-items:center;flex-wrap:wrap;font-size:.75rem">
          <label>Spearmen <input type="number" id="wdb-inf" min="0" value="0" style="width:3.2rem;background:var(--warm-white);border:1px solid var(--border);padding:.15rem .25rem;font-family:var(--mono)"></label>
          <label>Chariot <input type="number" id="wdb-cha" min="0" value="0" style="width:3.2rem;background:var(--warm-white);border:1px solid var(--border);padding:.15rem .25rem;font-family:var(--mono)"></label>
          <button class="btn-small" onclick="warDisband('${capital.id}')">Disband →</button>
        </div>
        <div id="war-disband-res" style="font-size:.72rem;margin-top:.2rem;min-height:.9rem"></div>`;
    }

    const locked = await renderLockedActions('province', capital.id);
    if (seq !== cityLoadSeq) return;
    document.getElementById('city-gar-sec').insertAdjacentHTML('beforeend', locked);

  } catch(e) { console.error('city drawer', e); }
}

// ── City build action ─────────────────────────────────────────────────────
const _BLD_LBL = {
  farm:'Farm', barracks:'Barracks', mine:'Mine', lumbermill:'Lumbermill',
  stonequarry:'Stone Quarry', market:'Market', wall:'Wall', tower:'Tower',
  harbour:'Harbour', shipyard:'Shipyard', foundry:'Foundry', stable:'Stable',
  bronze_wall:'Bronze Wall', olive_press:'Olive Press', winery:'Winery',
  temple:'Temple', pen:'Pen',
};

export async function startBuild() {
  const capital = activeCitySettlement();
  if (!capital) return;
  const sel = document.getElementById('city-build-select');
  const hexSel = document.getElementById('city-build-hex');
  const resultEl = document.getElementById('city-build-result');
  if (!sel || !resultEl) return;
  const btype = sel.value;
  resultEl.textContent = '';
  const body = { building_type: btype };
  // A hex-bound type (farm/mine/lumbermill/stonequarry) needs hex_q/hex_r —
  // the picker is only visible (see onCityBuildTypeChange) when the selected
  // type is hex-bound, so a visible, non-empty value is exactly when to send
  // one (megaron_plan_byggnad_pa_hex.md §A2).
  let hexLabel = '';
  if (hexSel && hexSel.style.display !== 'none' && hexSel.value) {
    const [q, r] = hexSel.value.split(',').map(Number);
    body.hex_q = q;
    body.hex_r = r;
    hexLabel = ` @ (${q},${r})`;
  }
  const r = await fetchAuth(
    `/api/v1/worlds/${State.WORLD_ID}/provinces/${capital.id}/build`,
    { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify(body) }
  );
  const d = await r.json().catch(() => ({}));
  if (r.ok) {
    resultEl.style.color = 'var(--safe-text)';
    resultEl.textContent = `${_BLD_LBL[btype]||btype} queued${hexLabel}.`;
    // Refresh only the buildings section — avoids resetting the active tab
    await refreshCityBuildings(capital.id);
  } else {
    resultEl.style.color = 'var(--accent)';
    resultEl.textContent = formatApiError(d, 'Build failed.');
  }
}

// onCityBuildTypeChange: the Construct type <select>'s onchange. Shows/hides
// and (re)populates the hex picker (#city-build-hex) — visible only for a
// hex-bound type (province.HexBoundBuildings via the catalogue's hex_bound
// field), fetched fresh each time since occupancy changes with every build
// (unlike the catalogue, this is NOT memoized).
let buildTypeSeq = 0;
export async function onCityBuildTypeChange(preferredHex) {
  const typeSel = document.getElementById('city-build-select');
  const hexSel = document.getElementById('city-build-hex');
  if (!typeSel || !hexSel) return;
  // Only the newest type change may write the hex picker; an older reply
  // (slow placement-options for a previous type) must not reopen or refill it.
  const seq = ++buildTypeSeq;
  const catalogue = await getBuildingCatalogue();
  if (seq !== buildTypeSeq) return;
  const entry = (catalogue || []).find(e => e.type === typeSel.value);
  if (!entry || !entry.hex_bound) {
    hexSel.style.display = 'none';
    hexSel.innerHTML = '';
    return;
  }
  const capital = activeCitySettlement();
  if (!capital) return;
  let placementOpts = null;
  try {
    const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${capital.id}/placement-options`);
    if (res.ok) placementOpts = await res.json();
  } catch (e) { console.error('onCityBuildTypeChange', e); }
  if (seq !== buildTypeSeq) return;
  // A hex the player picked while this refresh was pending survives if it is
  // still a valid choice.
  const chosen = preferredHex || hexSel.value;
  const html = hexBuildOptionsHTML(entry.type, placementOpts);
  hexSel.innerHTML = html || '<option value="">(no valid catchment hex)</option>';
  if (chosen && [...hexSel.options].some(o => o.value === chosen)) hexSel.value = chosen;
  hexSel.style.display = 'block';
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

// Building catalogue (GET /api/v1/buildings) — static for a world's lifetime,
// so fetch once and memoize rather than refetching on every drawer render.
// Same pattern as getUnitCatalogue() (war.js) and getRecipes() above: cache
// the in-flight/resolved promise, clear it on failure so the next render
// retries, return null on failure (never [] — [] would mean "the server has
// no buildings"). Replaces the 14-row hardcoded <option> list in the
// Construct dropdown, which was missing silver_mine entirely and quoted
// every other cost from before mig 136's dagsverkesskalan (~40-80x off on
// timber) — megaron_plan_byggkatalogen_i_webben.md.
let _buildingCataloguePromise = null;
async function getBuildingCatalogue() {
  if (!_buildingCataloguePromise) {
    _buildingCataloguePromise = fetchAuth('/api/v1/buildings').then(r => {
      if (!r.ok) throw new Error('buildings catalogue fetch failed: ' + r.status);
      return r.json();
    }).catch(e => {
      console.error('getBuildingCatalogue', e);
      _buildingCataloguePromise = null;
      return null; // null = fetch failed; [] would mean "server has no buildings"
    });
  }
  return _buildingCataloguePromise;
}

// wall keeps its old hardcoded option: the catalogue's "wall" entry carries
// only the L1 (Palisade) build cost — the Stone Wall/Bronze Wall upgrade
// ladder lives in province.WallLevelSpecs, a separate spec that
// GET /api/v1/buildings does not surface (province.LevelledBuildings
// excludes wall, so entry.upgrade_costs is empty for it). Rendering wall
// generically from entry.costs would silently drop the "upgrade" framing
// and the ladder note. Stop condition, megaron_plan_byggkatalogen_i_webben.md
// §7 — keep today's behaviour for wall specifically rather than inventing a
// new representation here.
const WALL_OPTION_HTML = '<option value="wall">Wall — upgrade (Palisade→Stone Wall→Bronze Wall)</option>';

// entry.costs comes from a Go map, so its JSON key order is not stable across
// requests — some explicit order is needed regardless of style. timber/stone
// first (matches the old hardcoded option strings and every current
// building's cost), anything else alphabetically after.
const GOOD_ORDER = ['timber', 'stone'];

// Cost string for one catalogue entry: "1 timber 9 stone", rounded the same
// way the CLI does (%.0f per good, cmd_build.go) so the two clients can never
// quote different prices for the same building.
function fmtBuildCost(entry) {
  const parts = Object.entries(entry.costs || {})
    .filter(([, v]) => v > 0)
    .sort(([a], [b]) => {
      const ia = GOOD_ORDER.indexOf(a), ib = GOOD_ORDER.indexOf(b);
      if (ia !== -1 || ib !== -1) return (ia === -1 ? 99 : ia) - (ib === -1 ? 99 : ib);
      return a.localeCompare(b);
    })
    .map(([k, v]) => `${Math.round(v)} ${k}`);
  if (entry.cost_silver > 0) parts.push(`${Math.round(entry.cost_silver)} silver`);
  return parts.join(' ');
}

// Construct dropdown <option> list, built from the building catalogue instead
// of a hardcoded list. Pure string builder — no DOM/fetch — testable the same
// as other pure HTML builders. `catalogue` is the array from
// getBuildingCatalogue(), already sorted alphabetically by type (server does
// the sort). Display name comes from the existing _BLD_LBL map (unchanged).
export function buildingOptionsHTML(catalogue) {
  return (catalogue || []).map(entry => {
    if (entry.type === 'wall') return WALL_OPTION_HTML;
    const label = _BLD_LBL[entry.type] || entry.type;
    const costStr = fmtBuildCost(entry);
    // Name and price only (Timothy 2026-09-30: the purpose/requires tail made
    // the list too long). Gates still hold: the hex picker only offers valid
    // hexes and the server's refusal names the missing condition.
    return `<option value="${entry.type}">${label} — ${costStr}</option>`;
  }).join('');
}

// hexBuildOptionsHTML: the Construct flow's hex picker for a hex-bound type
// (farm/mine/lumbermill/stonequarry, megaron_plan_byggnad_pa_hex.md §A2).
// Pure string builder over one placement-options response — no DOM/fetch,
// same testable shape as buildingOptionsHTML above.
//
// Two sources merged, deliberately: `valid_hexes_for_building[type]` is the
// server's list of hexes this type could be built on RIGHT NOW (catchment
// reach + terrain/deposit match) — but it EXCLUDES a hex already carrying a
// building of this same type (economy.ValidHexesForBuilding), because that
// hex isn't a NEW build, it's the upgrade path (build the same type on the
// same hex again). So a hex is offered as "upgrade" when `hexes[]` shows a
// building of this exact type already standing there, and as "build"
// otherwise. No client-side catchment/terrain validation is invented here —
// both lists come straight from the server.
export function hexBuildOptionsHTML(buildingType, placementOpts) {
  if (!placementOpts) return '';
  const hexesByCoord = new Map((placementOpts.hexes || []).map(h => [`${h.hex_q},${h.hex_r}`, h]));
  const valid = (placementOpts.valid_hexes_for_building || {})[buildingType] || [];
  const seen = new Set();
  const rows = [];
  for (const { q, r, effect } of valid) {
    const key = `${q},${r}`;
    seen.add(key);
    const h = hexesByCoord.get(key);
    const terrain = h ? terrainLabel(h.terrain) : '';
    // `effect` is the server's formatted string, printed verbatim (escaped).
    const tail = effect ? ` — ${esc(effect)}` : '';
    rows.push({ q, r, label: `(${q},${r})${terrain ? ' ' + terrain : ''}${tail}` });
  }
  for (const h of placementOpts.hexes || []) {
    const key = `${h.hex_q},${h.hex_r}`;
    if (seen.has(key)) continue;
    if (h.building && h.building.type === buildingType) {
      const terrain = terrainLabel(h.terrain);
      const fx = h.building.upgrade_effect ? `: ${esc(h.building.upgrade_effect)}` : '';
      const label = `(${h.hex_q},${h.hex_r})${terrain ? ' ' + terrain : ''} — upgrade to L${h.building.level + 1}${fx}`;
      rows.push({ q: h.hex_q, r: h.hex_r, label });
    }
  }
  return rows.map(o => `<option value="${o.q},${o.r}">${o.label}</option>`).join('');
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
    el.innerHTML = `<table class="goods-mini">
      <tr style="color:var(--text-dim);font-size:.7rem"><td>Day</td><td>Production</td><td>Consumption</td><td>Events</td></tr>
      ${ticks.map(t => {
        const prod = Object.entries(t.production||{}).map(([k,v]) => `${k} +${fmtNum(Number(v.toFixed(1)))}`).join(', ');
        const cons = Object.entries(t.consumption||{}).map(([k,v]) => `${k} -${fmtNum(Number(v.toFixed(1)))}`).join(', ');
        const evs  = (t.events||[]).map(e => e.type).join(', ');
        return `<tr><td>#${fmtNum(t.tick)}</td><td style="color:var(--safe-text)">${prod}</td><td style="color:var(--accent)">${cons}</td><td style="color:var(--text-dim)">${evs}</td></tr>`;
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
    showInlineResult('city-bld-sec', d.error || 'Could not cancel build', true);
  }
}

// Demolish a finished building: a deliberate second click (inline confirm, no
// browser dialog), then DELETE .../buildings/:id. The server lifts the people
// working there to idle, so the gubbe grid is redrawn too.
export function demolishBuilding(provinceID, buildingID, label) {
  const host = document.getElementById('city-demolish-res');
  confirmInline(host, demolishPrompt(label), async () => {
    const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${provinceID}/buildings/${buildingID}`, { method: 'DELETE' });
    if (res.ok) {
      const d = await res.json().catch(() => ({}));
      await refreshCityBuildings(provinceID);
      const gridEl = document.getElementById('city-gubbe-grid');
      const c = activeCitySettlement();
      if (gridEl && c) renderGubbeGrid(gridEl, c.id, c.q, c.r);
      showInlineResult('city-bld-sec', demolishResultLine(label, d.citizens_lifted));
    } else {
      const d = await res.json().catch(() => ({}));
      showInlineResult('city-bld-sec', d.error || 'Could not demolish', true);
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
          blds.map(b => `<tr><td>${_BLD_LBL[b.type]||b.type}${b.hex_q != null ? ` @ (${b.hex_q},${b.hex_r})` : ''}</td><td>L${b.level}</td>` +
            (b.id ? `<td style="text-align:right"><button class="btn-small" onclick="demolishBuilding('${provinceID}','${b.id}','${esc(demolishLabel(_BLD_LBL[b.type]||b.type, b.hex_q, b.hex_r))}')" style="padding:.05rem .3rem;font-size:.68rem;cursor:pointer">Demolish</button></td>` : '') +
            `</tr>`).join('')
        }</table><div id="city-demolish-res"></div>`
      : '<p class="empty-state">No buildings yet.</p>';
    if (pd.build_queue_max != null) h2 += `<p class="empty-state">Queue: ${bq.length}/${pd.build_queue_max} slots used — fixed limit; population and buildings do not add slots. Finish or cancel a build.</p>`;
    if (bq.length) h2 += `<div class="dsec-title" style="margin-top:.8rem">Build queue</div><table class="goods-mini">${
      // A building finishes, it doesn't arrive — say "ready" (matches the
      // Training column below and war.js's ship-build phrasing).
      bq.map(b => `<tr><td>${_BLD_LBL[b.type]||b.type}</td><td>${arrivalHTML(b.complete_at, undefined, 'ready')}</td>` +
        `<td style="text-align:right"><button class="btn-small" onclick="cancelBuild('${provinceID}','${b.id}')" style="padding:.05rem .3rem;font-size:.68rem;cursor:pointer">✕</button></td></tr>`).join('')
    }</table>`;
    if (tu.length) {
      // One row per maturing unit: land gathers men (forming, X/100), then trains
      // (100/100, ready ETA), then deploys to garrison; naval builds a vessel.
      h2 += `<div class="dsec-title" style="margin-top:.8rem">Training</div><table class="goods-mini">${
        tu.map(u => {
          const name = unitTypeLabel(u.unit);
          // A trained unit is ready, not arrived.
          let label, eta = u.ready_at ? arrivalHTML(u.ready_at, undefined, 'ready') : '';
          if (u.category === 'naval') label = 'building';
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
    const [recipes, buildingCatalogue] = await Promise.all([getRecipes(), getBuildingCatalogue()]);
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
          <p class="empty-state" style="font-size:.72rem">Smelted on its own every day — put a worker in the foundry and keep the ingredients in store.</p>`;
      } else {
        // Degrade honestly: no fabricated ratio.
        h2 += `
          <div class="dsec-title" style="margin-top:.8rem">Foundry</div>
          <p class="empty-state" style="font-size:.72rem">Recipe data unavailable — try reloading.</p>`;
      }
    }
    const prevSel = document.getElementById('city-build-select')?.value || '';
    const prevHex = document.getElementById('city-build-hex')?.value || '';
    // #city-build-hex: the hex picker for a hex-bound type (farm/mine/
    // lumbermill/stonequarry) — hidden by default, shown and populated by
    // onCityBuildTypeChange() when the selected catalogue entry's hex_bound
    // is true (megaron_plan_byggnad_pa_hex.md §A2).
    const constructHTML = buildingCatalogue ? `
      <select id="city-build-select" class="build-select" onchange="onCityBuildTypeChange()">
        ${buildingOptionsHTML(buildingCatalogue)}
      </select>
      <select id="city-build-hex" class="build-select" style="display:none;margin-top:.3rem"></select>
      <button class="btn-primary btn-small" onclick="startBuild()" style="margin-top:.5rem;width:100%">+ Build</button>`
      : `<p class="empty-state">Could not load the building catalogue — try again.</p>
      <button class="btn-primary btn-small" disabled style="margin-top:.5rem;width:100%">+ Build</button>`;
    h2 += `
      <div class="dsec-title" style="margin-top:.8rem">Construct</div>
      ${constructHTML}
      <div id="city-build-result" class="action-result"></div>`;
    bldSec.innerHTML = h2;
    // Restore previous dropdown selection and result message
    const newSel = document.getElementById('city-build-select');
    if (newSel) {
      if (prevSel) newSel.value = prevSel;
      // Also on the first render: a hex-bound default type needs its hex picker.
      onCityBuildTypeChange(prevHex);
    }
  } catch(e) { console.error('refreshCityBuildings', e); }
}
