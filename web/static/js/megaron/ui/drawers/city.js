import { BUILDING_LABELS as _BLD_LBL } from '../building_names.js';
import { disbandUnitsHTML } from '../disband.js';
import { showInlineResult, confirmInline } from '../inline_result.js';
import { demolishLabel, demolishPrompt, demolishResultLine } from '../demolish.js';
import { State, activeCitySettlement } from '../../state.js';
import { fetchAuth } from '../../api.js';
import { arrivalHTML, fmtArrival } from '../time.js';
import { serverNow } from '../../clock.js';
import { fmtTickTime } from '../fmt_clock.js';
import { renderLockedActions } from '../misc.js';
import { esc, formatApiError } from '../format.js';
import { unitTypeLabel } from '../actornames.js';
import { startCityAnim } from '../../render/city.js';
import { renderGubbeGrid, terrainLabel } from '../citygrid.js';
import { productionSectionsHTML, populationHTML, foodSummaryHTML, foodDetailsHTML, storageWarningsHTML } from './city_production.js';
import { fmtNum, fmtDays, fmtDuration } from '../fmt_num.js';

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
    document.getElementById('city-food-details-sec').innerHTML = foodDetailsHTML(pd);
    if (pd && pd.last_tick) {
      const lt = pd.last_tick;
      const prodRows = Object.entries(lt.production || {}).map(([k,v]) => `<tr><td>${k}</td><td style="color:var(--safe-text)">+${fmtNum(Number(v.toFixed(2)))}</td></tr>`).join('');
      const consRows = Object.entries(lt.consumption || {}).map(([k,v]) => `<tr><td>${k}</td><td style="color:var(--accent)">−${fmtNum(Number(v.toFixed(2)))}</td></tr>`).join('');
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
        <div class="dsec-title">Whole units in ${esc(capital.name)}</div>
        ${unitsResp.ok ? disbandUnitsHTML(units, capital.settlement_id || pd.id, capital.id) : '<p class="empty-state">Could not load units. Reopen the city to try again.</p>'}
        <div id="war-disband-res" class="action-result" role="status"></div>`;
    }

    const locked = await renderLockedActions('province', capital.id);
    if (seq !== cityLoadSeq) return;
    document.getElementById('city-gar-sec').insertAdjacentHTML('beforeend', locked);

  } catch(e) { console.error('city drawer', e); }
}

// ── City build action ─────────────────────────────────────────────────────


// A temple is built for one god: the Construct list has one row per god, its
// value "temple:<god_id>". splitBuildValue gives back the type and the god.
const TEMPLE_GODS = { potnia:'Potnia', velchanos:'Velchanos', asterion:'Asterion', sea_daimons_stillness:'the Sea Daimons of Stillness' };
export function splitBuildValue(value) {
  const [type, god] = String(value).split(':');
  return { type, god: god || '' };
}
export function templeName(god) { return `Temple of ${TEMPLE_GODS[god] || god}`; }

export async function startBuild() {
  const world = State.WORLD_ID;
  const capital = activeCitySettlement();
  if (!capital) return;
  const sel = document.getElementById('city-build-select');
  const hexSel = document.getElementById('city-build-hex');
  const resultEl = document.getElementById('city-build-result');
  if (!sel || !resultEl) return;
  const { type: btype, god } = splitBuildValue(sel.value);
  resultEl.textContent = '';
  const body = { building_type: btype };
  if (btype === 'temple') body.god = god;
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
    const receipt = `${btype === 'temple' ? templeName(god) : (_BLD_LBL[btype]||btype)} queued${hexLabel}${d.duration_game_days != null ? ' · ' + fmtDuration(d.duration_game_days) : ''}${d.complete_at ? ' · ready ' + fmtArrival(d.complete_at, d.complete_tick, 'ready') : ''}.`;
    resultEl.textContent = receipt;
    // Refresh only the buildings section — avoids resetting the active tab
    await refreshCityBuildings(capital.id);
    // Refresh replaces this section, including its receipt node.
    if (State.WORLD_ID === world && activeCitySettlement()?.id === capital.id) {
      const currentResult = document.getElementById('city-build-result');
      if (currentResult) { currentResult.style.color = 'var(--safe-text)'; currentResult.textContent = receipt; }
    }
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
let cityBuildQuotes = [];
function showBuildQuote(quote) {
 const el=document.getElementById("city-build-quote");if(el)el.innerHTML=buildQuoteHTML(quote);
 const button=document.querySelector("#city-bld-sec button[onclick=\"startBuild()\"]");if(button)button.disabled=!quote || quote.at_max_level;
}
export function buildQuoteHTML(quote) {
 if(!quote)return '<p class="empty-state">Build price unavailable.</p>';
 const name=esc(quote.name||_BLD_LBL[quote.type]||quote.type);
 if(quote.at_max_level)return `<p class="empty-state">${name} L${quote.current_level} — maximum level.</p>`;
 return `<p class="empty-state">${name}: L${quote.current_level} → L${quote.next_level}<br>${esc(fmtBuildCost(quote))} · ${fmtDuration(quote.duration_game_days)}${State.TICK_SECONDS > 0 && Number.isFinite(quote.duration_game_days) ? ' · if started now, ready ' + fmtArrival(new Date(serverNow() + quote.duration_game_days * State.TICK_SECONDS * 1000).toISOString(), undefined, 'ready') : ''}</p>`;
}


export async function onCityBuildTypeChange(preferredHex) {
  const typeSel = document.getElementById('city-build-select');
  const hexSel = document.getElementById('city-build-hex');
  if (!typeSel || !hexSel) return;
  // Only the newest type change may write the hex picker; an older reply
  // (slow placement-options for a previous type) must not reopen or refill it.
  const seq = ++buildTypeSeq;
  const catalogue = await getBuildingCatalogue();
  if (seq !== buildTypeSeq) return;
  const { type: selType, god: selGod } = splitBuildValue(typeSel.value);
  const entry = (catalogue || []).find(e => e.type === selType);
  if (!entry || !entry.hex_bound) {
    showBuildQuote(cityBuildQuotes.find(q=>q.type===entry?.type && !q.new_site && q.hex_q==null && (selType!=='temple' || q.god_id===selGod)));
    hexSel.style.display = 'none';
    hexSel.innerHTML = '';
    return;
  }
  showBuildQuote(null);
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
  const html = hexBuildOptionsHTML(entry.type, placementOpts, placementOpts?.build_quotes || []);
  hexSel.innerHTML = html || '<option value="">(no valid catchment hex)</option>';
  if (chosen && [...hexSel.options].some(o => o.value === chosen)) hexSel.value = chosen;
  hexSel.style.display = 'block';
  hexSel.onchange=()=>{if(!hexSel.value){showBuildQuote(null);return;}const [q,r]=hexSel.value.split(',').map(Number);const quotes=placementOpts?.build_quotes||[];showBuildQuote(quotes.find(x=>x.type===entry.type && x.hex_q===q && x.hex_r===r)||quotes.find(x=>x.type===entry.type && x.new_site));};
  hexSel.onchange();
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
// The wall is the population ceiling, and wall N needs a Granary of level N: the
// next rung is offered with its cost and what it holds, or shown disabled with the
// reason ("needs a Granary of level 2"). `ctx` = { walls, siloLevel } from the
// province response; without it (or without the server's wall_ladder) the old
// static line stays.
export function wallOptionHTML(entry, ctx) {
  const next = ctx && (entry.wall_ladder || []).find(w => w.level === Number(ctx.walls) + 1);
  if (!next) return ctx && Number(ctx.walls) >= 3 ? '<option value="wall" disabled>Wall — at the highest level</option>' : WALL_OPTION_HTML;
  const holds = `holds ${fmtNum(next.population_cap)}`;
  if (Number(ctx.siloLevel) < next.requires_silo) {
    return `<option value="wall" disabled>${next.name} — needs a Granary of level ${next.requires_silo} (${holds})</option>`;
  }
  return `<option value="wall">${next.name} — ${fmtBuildCost({ costs: next.costs })} (${holds})</option>`;
}

// One row per god. Prices come from the server's per-god build_quotes.
function templeOptionsHTML(entry, ctx) {
  const hasQuotes = ctx && Object.hasOwn(ctx, 'buildQuotes');
  return Object.keys(TEMPLE_GODS).map(god => {
    const value = `temple:${god}`, label = templeName(god);
    const quote = (ctx?.buildQuotes || []).find(q => q.type === 'temple' && q.god_id === god);
    if (hasQuotes && !quote) return `<option value="${value}" disabled>${label} — price unavailable</option>`;
    if (quote?.at_max_level) return `<option value="${value}" disabled>${label} L${quote.current_level} — maximum level</option>`;
    const level = quote ? ` L${quote.current_level} → L${quote.next_level}` : '';
    return `<option value="${value}">${label}${level} — ${fmtBuildCost(quote || entry)}</option>`;
  }).join('');
}

// Rows are sorted by the name the player reads (Timothy 2026-10-24): the server
// sorts by type key, so silo → "Granary" and stonequarry → "Stone Quarry" sat
// out of place.
const optionText = html => html.replace(/<[^>]*>/g, '');
export function buildingOptionsHTML(catalogue, ctx) {
  return (catalogue || []).flatMap(entry => {
    const html = buildingOptionHTML(entry, ctx);
    return html.match(/<option[\s\S]*?<\/option>/g) || [];
  }).sort((a, b) => optionText(a).localeCompare(optionText(b), 'en')).join('');
}

function buildingOptionHTML(entry, ctx) {
  if (entry.type === 'wall') return wallOptionHTML(entry, ctx);
  if (entry.type === 'temple') return templeOptionsHTML(entry, ctx);
  const label = _BLD_LBL[entry.type] || entry.type;
  if (entry.hex_bound && ctx?.buildQuotes) return `<option value="${entry.type}">${label} — choose a hex for its level and price</option>`;
  const hasQuotes = ctx && Object.hasOwn(ctx, 'buildQuotes');
  const quote = (ctx?.buildQuotes || []).find(q => q.type === entry.type && !q.new_site && q.hex_q == null);
  if (hasQuotes && !quote) return `<option value="${entry.type}" disabled>${label} — price unavailable</option>`;
  if (quote?.at_max_level) return `<option value="${entry.type}" disabled>${label} L${quote.current_level} — maximum level</option>`;
  const costStr = fmtBuildCost(quote || entry);
  const level = quote ? ` L${quote.current_level} → L${quote.next_level}` : '';
  // Name and price only (Timothy 2026-09-30: the purpose/requires tail made
  // the list too long). Gates still hold: the hex picker only offers valid
  // hexes and the server's refusal names the missing condition.
  return `<option value="${entry.type}">${label}${level} — ${costStr}</option>`;
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
export function hexBuildOptionsHTML(buildingType, placementOpts, quotes) {
  const price = (q,r) => quotes?.find(x=>x.type===buildingType && x.hex_q===q && x.hex_r===r) || quotes?.find(x=>x.type===buildingType && x.new_site);
  const priced = (label,q,r) => {const quote=price(q,r);return quote ? (quote.at_max_level ? label + ` — L${quote.current_level}, maximum level` : label + ` — L${quote.current_level} → L${quote.next_level}: ${fmtBuildCost(quote)}`) : label + (quotes ? " — price unavailable" : "");};
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
    rows.push({ q, r, label: priced(`(${q},${r})${terrain ? ' ' + terrain : ''}${tail}`,q,r) });
  }
  for (const h of placementOpts.hexes || []) {
    const key = `${h.hex_q},${h.hex_r}`;
    if (seen.has(key)) continue;
    if (h.building && h.building.type === buildingType) {
      const terrain = terrainLabel(h.terrain);
      const fx = h.building.upgrade_effect ? `: ${esc(h.building.upgrade_effect)}` : '';
      const label = quotes ? priced(`(${h.hex_q},${h.hex_r})${terrain ? ' ' + terrain : ''}${fx}`,h.hex_q,h.hex_r) : `(${h.hex_q},${h.hex_r})${terrain ? ' ' + terrain : ''} — upgrade to L${h.building.level + 1}${fx}`;
      rows.push({ q: h.hex_q, r: h.hex_r, label });
    }
  }
  return rows.map(o => `<option value="${o.q},${o.r}" ${price(o.q,o.r)?.at_max_level ? "disabled" : ""}>${o.label}</option>`).join('');
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
          blds.map(b => `<tr><td>${esc(b.name||_BLD_LBL[b.type]||b.type)}${b.hex_q != null ? ` @ (${b.hex_q},${b.hex_r})` : ''}</td><td>L${b.level}</td>` +
            (b.id ? `<td style="text-align:right"><button class="btn-small" onclick="demolishBuilding('${provinceID}','${b.id}','${esc(demolishLabel(b.name||_BLD_LBL[b.type]||b.type, b.hex_q, b.hex_r))}')" style="padding:.05rem .3rem;font-size:.68rem;cursor:pointer">Demolish</button></td>` : '') +
            `</tr>`).join('')
        }</table><div id="city-demolish-res"></div>`
      : '<p class="empty-state">No buildings yet.</p>';
    if (pd.build_queue_max != null) h2 += `<p class="empty-state">Queue: ${bq.length}/${pd.build_queue_max} slots used — fixed limit; population and buildings do not add slots. Finish or cancel a build.</p>`;
    if (bq.length) h2 += `<div class="dsec-title" style="margin-top:.8rem">Build queue</div><table class="goods-mini">${
      // A building finishes, it doesn't arrive — say "ready" (matches the
      // Training column below and war.js's ship-build phrasing).
      bq.map(b => `<tr><td>${esc(b.name||_BLD_LBL[b.type]||b.type)} · ${esc(b.settlement_name||pd.name||'this city')}${b.hex_q != null ? ` at (${b.hex_q},${b.hex_r})` : ''}</td><td>${arrivalHTML(b.complete_at, undefined, 'ready')}</td>` +
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
          else if (u.category === 'naval') label = 'building';
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
          <p class="empty-state" style="font-size:.72rem">Smelted on its own every game day — put a worker in the foundry and keep the ingredients in store.</p>`;
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
    cityBuildQuotes = pd.build_quotes || [];
    const constructHTML = buildingCatalogue ? `
      <select id="city-build-select" class="build-select" onchange="onCityBuildTypeChange()">
        ${buildingOptionsHTML(buildingCatalogue, { walls: pd.walls, siloLevel: pd.silo_level, buildQuotes: pd.build_quotes })}
      </select>
      <select id="city-build-hex" class="build-select" style="display:none;margin-top:.3rem"></select>
      <div id="city-build-quote"></div>
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
