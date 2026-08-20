import { State, activeCitySettlement } from '../../state.js';
import { fetchAuth } from '../../api.js';
import { track } from '../../telemetry.js';
import { arrivalHTML } from '../time.js';
import { renderLockedActions } from '../misc.js';
import { unitTypeLabel } from '../actornames.js';
import { startCityAnim } from '../../render/city.js';
import { renderGubbeGrid } from '../citygrid.js';

// The settlement the City drawer currently shows: cycle via the drawer's
// prev/next arrows. Defaults to the capital (activeCitySettlement, state.js).
export function cycleCityView(dir) {
  const mine = State.provinceData.filter(p => p.own && !p.is_outpost);
  if (mine.length < 2) return;
  const current = activeCitySettlement();
  let idx = mine.findIndex(p => p.id === current.id);
  idx = (idx + dir + mine.length) % mine.length;
  State.cityViewID = mine[idx].id;
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
    // Cult (devotion) is always named explicitly when its input exists (even
    // at 0 — the server floors it), never left out like the other goods'
    // v>0 filter would. PUT .../labor REPLACES the whole allocation and any
    // good not named drops to 0% — for cult that means dropping to the
    // server's floor. Omitting it here on a save that only touches e.g.
    // timber would silently flatten a higher devotion set earlier (by keryx
    // or an earlier web session) down to the floor. Always echoing back
    // the value shown (which loadCityDrawer seeds from the live devotion)
    // keeps an untouched devotion exactly where it was.
    if (v > 0 || inp.dataset.good === 'cult') percent[inp.dataset.good] = v;
  });
  const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${provinceID}/labor`, {
    method: 'PUT',
    headers: {'Content-Type':'application/json'},
    body: JSON.stringify({percent}),
  });
  if (res.ok) {
    if (msg) msg.textContent = 'Saved!';
    // Cult (devotion) is additive and deliberately excluded from the response's
    // percent/citizens/idle_percent (server never counts it in the ≤100% sum —
    // province.go LaborAlloc, `filtered` never gets a "cult" key). Re-read the
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
          if (cit) cit.textContent = Math.round((pd2.devotion || 0) * (pd2.labor_pool || 0));
          const rateCell = document.getElementById('labor-rate-cult');
          if (rateCell) {
            const atCap = devPct2 >= devCapPct2;
            rateCell.textContent = `${devPct2}% of ${devCapPct2}% cap${atCap ? ' · at cap — build a higher-level temple to devote more' : ''}`;
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
export async function loadCityDrawer() {
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
      <div class="dsec"><div class="dsec-title">Befolkning</div><div id="city-pop-sec"><div class="loading" style="font-size:.8rem">Loading…</div></div></div>
      <div class="dsec"><div class="dsec-title">Produktion</div><div id="city-prod-sec"><div class="loading" style="font-size:.8rem">Loading…</div></div></div>
      <div class="dsec"><div class="dsec-title">Sitos</div><div id="city-sitos-sec"><div class="loading" style="font-size:.8rem">Loading…</div></div></div>
      <div class="dsec"><div class="dsec-title">Senaste tick</div><div id="city-lasttick-sec"><div class="loading" style="font-size:.8rem">Loading…</div></div></div>
      <div class="dsec">
        <div class="dsec-title">Ticklog <button class="btn-small" onclick="loadTicklog()" style="margin-left:.4rem;padding:.05rem .3rem;font-size:.65rem;cursor:pointer">Show recent ticks</button></div>
        <div id="city-ticklog-sec"></div>
      </div>
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
      document.getElementById('city-pop-sec').innerHTML = `
        <div class="stat-row"><span class="sr-label">Population</span><span class="sr-val">${pd.population}</span></div>
        <div class="stat-row"><span class="sr-label">In service</span><span class="sr-val">${armyPop}</span></div>
        <div class="stat-row stat-row-strong"><span class="sr-label">Labor pool</span><span class="sr-val">${lp}</span></div>
        <div class="stat-row"><span class="sr-label">Idle</span><span class="sr-val">${idle}</span></div>
        <div class="stat-row">
          <span class="sr-label">Livestock</span>
          <span class="sr-val">${Math.floor(livestock)}
            <button class="btn-small" onclick="slaughterLivestock('${capital.id}')" ${livestock < 1 ? 'disabled' : ''}
              style="margin-left:.4rem;padding:.05rem .3rem;font-size:.65rem;cursor:pointer"
              title="Trade one animal for +10 population, right now">Slaughter → +10 pop</button>
          </span>
        </div>
        <div id="city-slaughter-result" class="action-result"></div>`;
    } else {
      document.getElementById('city-pop-sec').innerHTML = '<p class="empty-state">—</p>';
    }

    // ── Sitos + senaste tick ──────────────────────────────────────────────
    // Grain itemized as prod − cons = net per tick (keryx `status` parity,
    // DEL C): the stored rate is already net, so a lone negative number
    // reads as an alarm when it is often just normal balance.
    let grainRow = '';
    if (pd && pd.grain_prod_rate != null) {
      // grain_prod_rate/grain_consum_rate are already per-tick (economy.
      // GrainConsumptionPerCitizenPerTick) — no ×24 here now that tick == day
      // (mig 109); that used to convert an hourly tick rate to a daily one and
      // is the same class of stale scaling as cmd_goods.go's Rate/d bug.
      const prodTick = pd.grain_prod_rate || 0;
      const consTick = pd.grain_consum_rate || 0;
      const netTick  = prodTick - consTick;
      const be = pd.breakeven_grain_weight != null
        ? ` <span style="color:var(--text-dim);font-size:.7rem">(break-even ≥${Math.round(pd.breakeven_grain_weight * 100)}% grain share)</span>` : '';
      grainRow = `<div class="stat-row"><span class="sr-label">Grain</span><span class="sr-val">prod ${prodTick.toFixed(1)} − cons ${consTick.toFixed(1)} = <b style="color:${netTick >= 0 ? 'var(--safe)' : 'var(--accent)'}">${netTick >= 0 ? '+' : ''}${netTick.toFixed(1)}/tick</b>${be}</span></div>`;
    }
    if (pd && pd.sitos) {
      // Coverage is the trigger (mig 106), so it leads. Colour it against the
      // low threshold — that is the line where the granary starts feeding the
      // city, and where an empty granary means nobody will.
      const s = pd.sitos;
      const cov = s.coverage_ticks || 0;
      // Low coverage is only an alarm when the stock is also shrinking — a new
      // city sits near zero coverage while filling up fast, and at 60 min/tick
      // that lasts real days.
      const falling = (s.food_net_per_tick || 0) <= 0;
      const covColour = (cov < (s.low_ticks || 0) && falling) ? 'var(--accent)' : 'var(--safe)';
      const perGood = Object.entries(s.granary_per_good || {})
        .filter(([, v]) => v > 0)
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([k, v]) => `${v.toFixed(0)} ${k}`).join(', ');
      document.getElementById('city-sitos-sec').innerHTML = grainRow + `
        <div class="stat-row"><span class="sr-label">Coverage</span><span class="sr-val" style="color:${covColour}"><b>${cov.toFixed(1)} ticks</b> <span style="color:var(--text-dim);font-size:.7rem">(stores above ${s.high_ticks} · releases below ${s.low_ticks})</span></span></div>
        <div class="stat-row"><span class="sr-label">Granary</span><span class="sr-val">${(s.granary_total||0).toFixed(0)} / ${(s.granary_cap||0).toFixed(0)} food${perGood ? ` <span style="color:var(--text-dim);font-size:.7rem">(${perGood})</span>` : ''}</span></div>`;
    } else {
      document.getElementById('city-sitos-sec').innerHTML = grainRow || '<p class="empty-state">—</p>';
    }
    if (pd && pd.last_tick) {
      const lt = pd.last_tick;
      const prodRows = Object.entries(lt.production || {}).map(([k,v]) => `<tr><td>${k}</td><td style="color:var(--safe)">+${v.toFixed(2)}</td></tr>`).join('');
      const consRows = Object.entries(lt.consumption || {}).map(([k,v]) => `<tr><td>${k}</td><td style="color:var(--accent)">−${v.toFixed(2)}</td></tr>`).join('');
      document.getElementById('city-lasttick-sec').innerHTML = `
        <div class="stat-row"><span class="sr-label">Tick</span><span class="sr-val">#${lt.tick}</span></div>
        ${(lt.sitos_food_in > 0 || lt.sitos_food_out > 0) ? `<div class="stat-row"><span class="sr-label">Sitos</span><span class="sr-val">${lt.sitos_food_in > 0 ? `<span style="color:var(--safe)">+${lt.sitos_food_in.toFixed(0)} food from granary</span>` : ''}${(lt.sitos_food_in > 0 && lt.sitos_food_out > 0) ? ' · ' : ''}${lt.sitos_food_out > 0 ? `<span style="color:var(--text-dim)">${lt.sitos_food_out.toFixed(0)} food stored</span>` : ''}</span></div>` : ''}
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
        <div style="font-size:.72rem;color:var(--text-dim)"><span class="labor-cit" data-good="cult">${Math.round(devWeight*lp)}</span> cit · <span id="labor-rate-cult">${devPct}% of ${devCapPct}% cap${atCap ? ' · at cap — build a higher-level temple to devote more' : ''}</span></div>
        <div style="margin-top:.4rem;display:flex;gap:.4rem;align-items:center">
          <button id="labor-save-btn" onclick="saveLaborAlloc('${capital.id}')"
            style="padding:.3rem .7rem;background:var(--accent);border:none;color:#000;font-size:.8rem;cursor:pointer">
            Assign →
          </button>
          <span id="labor-save-msg" style="font-size:.75rem;color:var(--safe)"></span>
          <span id="labor-save-err" style="font-size:.75rem;color:var(--danger)"></span>
        </div>`;
    } else {
      // Mirrors the server's own 422 wording (province.go LaborAlloc, key=="cult"
      // branch) rather than inventing separate copy — no temple, no control.
      devotionHTML = `<p class="empty-state">Devotion (cult): needs a temple here — build one first.</p>`;
    }

    document.getElementById('city-prod-sec').innerHTML = `
      <div class="dsec-title">Devotion</div>
      ${devotionHTML}
      <div class="dsec-title" style="margin-top:.6rem">Catchment &amp; workplaces</div>
      <div id="city-gubbe-grid"><div class="loading" style="font-size:.8rem">Loading…</div></div>`;
    const cultInp = document.getElementById('labor-input-cult');
    if (cultInp) cultInp.addEventListener('input', () => {
      const cit = document.querySelector('.labor-cit[data-good="cult"]');
      if (cit) cit.textContent = Math.round((parseFloat(cultInp.value||0)||0)/100*lp);
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
              return `<tr><td>${unitTypeLabel(k)}</td><td style="text-align:right">${n}</td><td style="text-align:right;color:var(--text-dim)">${n*POP_COSTS[k]}</td><td style="text-align:right;color:var(--safe)">${n*UNIT_DP[k]}</td></tr>`;
            }).join('')}
            <tr style="border-top:1px solid var(--border);font-weight:bold"><td>Total</td><td></td><td style="text-align:right;color:var(--accent)">${totalPop}</td><td style="text-align:right;color:var(--safe)">${totalDP} DP</td></tr>
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

    document.getElementById('city-gar-sec').innerHTML += await renderLockedActions('province', capital.id);

  } catch(e) { console.error('city drawer', e); }
}

// S1c (megaron_plan_foda_konsistens.md): player-initiated livestock slaughter
// for +10 population right now — never automatic. On success the whole
// drawer is reloaded (population, the herd's stock, and — if the +10 crossed
// a new full hundred — the newly auto-placed gubbe in the catchment grid all
// change together, so a narrower refresh would risk showing stale numbers).
export async function slaughterLivestock(provinceID) {
  const resultEl = document.getElementById('city-slaughter-result');
  if (resultEl) resultEl.textContent = '';
  const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${provinceID}/slaughter-livestock`, { method: 'POST' });
  const d = await res.json().catch(() => ({}));
  if (res.ok) {
    track('livestock_slaughtered', { gubbar_placed: d.gubbar_placed || 0 });
    await loadCityDrawer();
  } else if (resultEl) {
    resultEl.style.color = 'var(--accent)';
    resultEl.textContent = d.error || 'Slaughter failed.';
  }
}

// ── City build action ─────────────────────────────────────────────────────
const _BLD_LBL = {
  farm:'Farm', barracks:'Barracks', mine:'Mine', lumbermill:'Lumbermill',
  stonequarry:'Stone Quarry', market:'Agora', wall:'Wall', tower:'Tower',
  harbour:'Harbour', shipyard:'Shipyard', foundry:'Foundry', stable:'Stable',
  bronze_wall:'Bronze Wall', olive_press:'Olive Press', winery:'Winery',
  temple:'Temple',
};

export async function startBuild() {
  const capital = activeCitySettlement();
  if (!capital) return;
  const sel = document.getElementById('city-build-select');
  const resultEl = document.getElementById('city-build-result');
  if (!sel || !resultEl) return;
  const btype = sel.value;
  resultEl.textContent = '';
  const r = await fetchAuth(
    `/api/v1/worlds/${State.WORLD_ID}/provinces/${capital.id}/build`,
    { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({building_type: btype}) }
  );
  const d = await r.json().catch(() => ({}));
  if (r.ok) {
    track('build_started', { building: btype });
    resultEl.style.color = 'var(--safe)';
    resultEl.textContent = `${_BLD_LBL[btype]||btype} queued.`;
    // Refresh only the buildings section — avoids resetting the active tab
    await refreshCityBuildings(capital.id);
  } else {
    resultEl.style.color = 'var(--accent)';
    resultEl.textContent = d.error || 'Build failed.';
  }
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

export async function startCraft(provinceID, recipeID) {
  const qtyEl = document.getElementById('city-craft-qty');
  const resultEl = document.getElementById('city-craft-result');
  const qty = qtyEl ? parseInt(qtyEl.value, 10) || 0 : 0;
  if (!resultEl || qty <= 0 || !recipeID) return;
  resultEl.textContent = '';
  const r = await fetchAuth(
    `/api/v1/worlds/${State.WORLD_ID}/provinces/${provinceID}/craft`,
    { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({recipe_id: Number(recipeID), quantity: qty}) }
  );
  const d = await r.json().catch(() => ({}));
  if (r.ok) {
    resultEl.style.color = 'var(--safe)';
    // Result reads the server's own output_key/produced (Craft's response),
    // not a client-guessed output — no recipe assumption survives here.
    resultEl.textContent = `${d.produced ?? qty} ${d.output_key || 'goods'} crafted.`;
    await refreshCityBuildings(provinceID);
  } else {
    resultEl.style.color = 'var(--accent)';
    resultEl.textContent = d.error || 'Craft failed.';
  }
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
    if (!ticks.length) { el.innerHTML = '<p class="empty-state">No tick history yet.</p>'; return; }
    el.innerHTML = `<table class="goods-mini">
      <tr style="color:var(--text-dim);font-size:.7rem"><td>Tick</td><td>Production</td><td>Consumption</td><td>Events</td></tr>
      ${ticks.map(t => {
        const prod = Object.entries(t.production||{}).map(([k,v]) => `${k} +${v.toFixed(1)}`).join(', ');
        const cons = Object.entries(t.consumption||{}).map(([k,v]) => `${k} -${v.toFixed(1)}`).join(', ');
        const evs  = (t.events||[]).map(e => e.type).join(', ');
        return `<tr><td>#${t.tick}</td><td style="color:var(--safe)">${prod}</td><td style="color:var(--accent)">${cons}</td><td style="color:var(--text-dim)">${evs}</td></tr>`;
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
    alert(d.error || 'Could not cancel build');
  }
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
    let h2 = blds.length
      ? `<div class="dsec-title">Built</div><table class="goods-mini">${
          blds.map(b => `<tr><td>${_BLD_LBL[b.type]||b.type}</td><td>L${b.level}</td></tr>`).join('')
        }</table>`
      : '<p class="empty-state">No buildings yet.</p>';
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
    const recipes = await getRecipes();
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
          <div class="dsec-title" style="margin-top:.8rem">Craft — ${outputName}</div>
          <div style="font-size:.72rem;color:var(--text-dim);margin-bottom:.3rem">${ingredientsStr} → ${foundryRecipe.output_qty} ${foundryRecipe.output_key} · stock: ${stockStr}</div>
          <div style="display:flex;gap:.4rem;align-items:center">
            <input type="number" id="city-craft-qty" min="1" value="1" style="width:4rem;background:var(--warm-white);border:1px solid var(--border);padding:.15rem .3rem;font-family:var(--mono);font-size:.75rem">
            <button class="btn-primary btn-small" onclick="startCraft('${provinceID}',${foundryRecipe.id})">Craft →</button>
          </div>
          <div id="city-craft-result" class="action-result"></div>`;
      } else {
        // Degrade honestly: no fabricated ratio, no dead Craft button.
        h2 += `
          <div class="dsec-title" style="margin-top:.8rem">Craft</div>
          <p class="empty-state" style="font-size:.72rem">Recipe data unavailable — try reloading.</p>`;
      }
    }
    const prevSel = document.getElementById('city-build-select')?.value || '';
    h2 += `
      <div class="dsec-title" style="margin-top:.8rem">Construct</div>
      <select id="city-build-select" class="build-select">
        <option value="market">Agora — 100 timber 60 stone · +0.5 silver/tick</option>
        <option value="barracks">Barracks — 80 timber 80 stone · recruits</option>
        <option value="farm">Farm — 50 timber 20 stone · +grain/tick</option>
        <option value="foundry">Foundry — 80 timber 100 stone · ${foundryRecipe ? 'craft ' + foundryRecipe.output_key : 'craft goods'}</option>
        <option value="harbour">Harbour — 140 timber 60 stone · fish, sea trade</option>
        <option value="lumbermill">Lumbermill — 40 timber 40 stone · +timber/tick</option>
        <option value="mine">Mine — 60 timber 40 stone · +ore/tick</option>
        <option value="olive_press">Olive Press — 30 timber 40 stone · +oil/tick</option>
        <option value="shipyard">Shipyard — 140 timber 60 stone · builds/repairs ships</option>
        <option value="stable">Stable — 60 timber 40 stone · horses</option>
        <option value="stonequarry">Stone Quarry — 50 timber 20 stone · +stone/tick</option>
        <option value="temple">Temple — 60 timber 60 stone</option>
        <option value="wall">Wall — upgrade (Palisade→Stone Wall→Bronze Wall)</option>
        <option value="winery">Winery — 40 timber 30 stone · +wine/tick</option>
      </select>
      <button class="btn-primary btn-small" onclick="startBuild()" style="margin-top:.5rem;width:100%">+ Build</button>
      <div id="city-build-result" class="action-result"></div>`;
    bldSec.innerHTML = h2;
    // Restore previous dropdown selection and result message
    const newSel = document.getElementById('city-build-select');
    if (newSel && prevSel) newSel.value = prevSel;
  } catch(e) { console.error('refreshCityBuildings', e); }
}
