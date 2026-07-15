import { State, activeCitySettlement } from '../../state.js';
import { fetchAuth } from '../../api.js';
import { fmtSilver, fmtEta } from '../format.js';
import { renderLockedActions } from '../misc.js';
import { startCityAnim } from '../../render/city.js';

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
    if (v > 0) percent[inp.dataset.good] = v;
  });
  const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${provinceID}/labor`, {
    method: 'PUT',
    headers: {'Content-Type':'application/json'},
    body: JSON.stringify({percent}),
  });
  if (res.ok) {
    const data = await res.json();
    if (msg) msg.textContent = 'Saved!';
    const idleEl = document.getElementById('labor-idle-disp');
    if (idleEl && data.idle_percent != null) idleEl.textContent = Math.round(data.idle_percent);
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
  const UNIT_LBL  = {
    Spearman:'Spearmen', EliteInfantry:'Elite Infantry', WarChariot:'War Chariot',
    Ship:'Galley', WarGalley:'War Galley', Merchantman:'Emporos',
    spearman:'Spearmen', elite_infantry:'Elite Infantry', war_chariot:'War Chariot',
    ship:'Galley', galley:'Galley', war_galley:'War Galley', merchantman:'Emporos',
  };

  body.innerHTML = `
    <canvas id="city-scene" class="city-scene" width="320" height="110"></canvas>
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
    startCityAnim(document.getElementById('city-scene'), capitalTile2,
                  pd ? pd.buildings : [], pd ? pd.build_queue : []);

    // ── Produktion ──────────────────────────────────────────────────────────
    if (pd) {
      const army    = pd.army || {};
      const armyPop = Object.entries(POP_COSTS).reduce((s,[k,c]) => s + (army[k]||0)*c, 0);
      const lp      = pd.labor_pool || 0;
      const idle    = goods[0] ? (goods[0].idle_citizens || 0) : 0;
      document.getElementById('city-pop-sec').innerHTML = `
        <div class="stat-row"><span class="sr-label">Population</span><span class="sr-val">${pd.population}</span></div>
        <div class="stat-row"><span class="sr-label">In service</span><span class="sr-val">${armyPop}</span></div>
        <div class="stat-row stat-row-strong"><span class="sr-label">Labor pool</span><span class="sr-val">${lp}</span></div>
        <div class="stat-row"><span class="sr-label">Idle</span><span class="sr-val">${idle}</span></div>`;
    } else {
      document.getElementById('city-pop-sec').innerHTML = '<p class="empty-state">—</p>';
    }

    // ── Sitos + senaste tick ──────────────────────────────────────────────
    // Grain itemized as prod − cons = net per day (keryx `status` parity,
    // DEL C): the stored rate is already net, so a lone negative number
    // reads as an alarm when it is often just normal balance.
    let grainRow = '';
    if (pd && pd.grain_prod_rate != null) {
      const prodDay = (pd.grain_prod_rate || 0) * 24;
      const consDay = (pd.grain_consum_rate || 0) * 24;
      const netDay  = prodDay - consDay;
      const be = pd.breakeven_grain_weight != null
        ? ` <span style="color:var(--text-dim);font-size:.7rem">(break-even ≥${Math.round(pd.breakeven_grain_weight * 100)}% grain share)</span>` : '';
      grainRow = `<div class="stat-row"><span class="sr-label">Grain</span><span class="sr-val">prod ${prodDay.toFixed(1)} − cons ${consDay.toFixed(1)} = <b style="color:${netDay >= 0 ? 'var(--safe)' : 'var(--accent)'}">${netDay >= 0 ? '+' : ''}${netDay.toFixed(1)}/day</b>${be}</span></div>`;
    }
    if (pd && pd.sitos) {
      const s = pd.sitos;
      document.getElementById('city-sitos-sec').innerHTML = grainRow + `
        <div class="stat-row"><span class="sr-label">Fund</span><span class="sr-val">${fmtSilver(s.fund_silver)} / ${fmtSilver(s.fund_cap)}</span></div>
        <div class="stat-row"><span class="sr-label">Tax rate</span><span class="sr-val">+${s.fund_rate_per_tick.toFixed(2)}/tick</span></div>
        <div class="stat-row"><span class="sr-label">Grain ref. price</span><span class="sr-val">${s.ref_price_grain.toFixed(2)} (floor ${s.ref_price_floor} · ceiling ${s.ref_price_ceiling})</span></div>`;
    } else {
      document.getElementById('city-sitos-sec').innerHTML = grainRow || '<p class="empty-state">—</p>';
    }
    if (pd && pd.last_tick) {
      const lt = pd.last_tick;
      const prodRows = Object.entries(lt.production || {}).map(([k,v]) => `<tr><td>${k}</td><td style="color:var(--safe)">+${v.toFixed(2)}</td></tr>`).join('');
      const consRows = Object.entries(lt.consumption || {}).map(([k,v]) => `<tr><td>${k}</td><td style="color:var(--accent)">−${v.toFixed(2)}</td></tr>`).join('');
      document.getElementById('city-lasttick-sec').innerHTML = `
        <div class="stat-row"><span class="sr-label">Tick</span><span class="sr-val">#${lt.tick}</span></div>
        <div class="stat-row"><span class="sr-label">Sitos delta</span><span class="sr-val" style="color:${lt.sitos_delta>=0?'var(--safe)':'var(--accent)'}">${lt.sitos_delta>=0?'+':''}${lt.sitos_delta.toFixed(2)}</span></div>
        ${(prodRows||consRows) ? `<table class="goods-mini" style="margin-top:.3rem">${prodRows}${consRows}</table>` : ''}`;
    } else {
      document.getElementById('city-lasttick-sec').innerHTML = '<p class="empty-state">—</p>';
    }

    const prodGoods = goods.filter(g => g.producible || g.rate_per_tick > 0);
    if (prodGoods.length) {
      const lp = pd ? (pd.labor_pool || 0) : 0;
      // Each good gets a percent input (share of population). The percent auto-scales
      // with population; the resulting citizen count is shown so the player sees that
      // more citizens produce more even at a lower percent.
      const rows = prodGoods.map(g => {
        const ypw = g.yield_per_worker ? g.yield_per_worker.toFixed(3) : '—';
        const pct = g.percent != null ? Math.round(g.percent) : 0;
        return `<tr>
          <td style="padding:.2rem .3rem">${g.name||g.key}</td>
          <td style="padding:.2rem .3rem;text-align:right;white-space:nowrap">
            <input type="number" class="labor-input" data-good="${g.key}"
              value="${pct}" min="0" max="100" step="1"
              style="width:3.5rem;background:var(--bg-raised);border:1px solid var(--border);color:var(--text);padding:.15rem .3rem;font-size:.8rem;text-align:right">%
          </td>
          <td style="padding:.2rem .3rem;text-align:right;color:var(--text-dim);font-size:.75rem"><span class="labor-cit" data-good="${g.key}">${g.citizens||0}</span> cit</td>
          <td style="padding:.2rem .3rem;color:var(--safe)" id="labor-rate-${g.key}">+${((g.rate_per_tick||0)*24).toFixed(1)}/day</td>
        </tr>`;
      }).join('');
      const idlePct = Math.round((goods[0]||{}).idle_citizens!=null ? (100 - prodGoods.reduce((s,g)=>s+(g.percent||0),0)) : 0);
      document.getElementById('city-prod-sec').innerHTML =
        `<div style="font-size:.72rem;color:var(--text-dim);margin-bottom:.3rem">Share of population to assign (pop: <span id="labor-pool-disp">${lp}</span>, idle: <span id="labor-idle-disp">${Math.max(0,idlePct)}</span>%)</div>
         <table class="goods-mini" style="width:100%">
           <thead><tr style="color:var(--text-dim);font-size:.7rem"><td>Good</td><td style="text-align:right">Share</td><td style="text-align:right">Workers</td><td>Prod/day</td></tr></thead>
           <tbody>${rows}</tbody>
         </table>
         <div style="margin-top:.4rem;display:flex;gap:.4rem;align-items:center">
           <button id="labor-save-btn" onclick="saveLaborAlloc('${capital.id}')"
             style="padding:.3rem .7rem;background:var(--accent);border:none;color:#000;font-size:.8rem;cursor:pointer">
             Assign →
           </button>
           <span id="labor-save-msg" style="font-size:.75rem;color:var(--safe)"></span>
           <span id="labor-save-err" style="font-size:.75rem;color:var(--danger)"></span>
         </div>`;
      // Live preview: recompute resulting citizen counts + idle percent as the user edits.
      document.getElementById('city-prod-sec').querySelectorAll('.labor-input').forEach(inp => {
        inp.addEventListener('input', () => {
          let totalPct = 0;
          document.getElementById('city-prod-sec').querySelectorAll('.labor-input').forEach(i => {
            const p = parseFloat(i.value||0)||0;
            totalPct += p;
            const cit = document.querySelector(`.labor-cit[data-good="${i.dataset.good}"]`);
            if (cit) cit.textContent = Math.round(p/100*lp);
          });
          const idleEl = document.getElementById('labor-idle-disp');
          if (idleEl) idleEl.textContent = Math.max(0, Math.round(100 - totalPct));
        });
      });
    } else {
      document.getElementById('city-prod-sec').innerHTML = '<p class="empty-state">No production yet.</p>';
    }

    // ── Byggnader ───────────────────────────────────────────────────────────
    // Delegate to refreshCityBuildings so startBuild() can call it too
    // without resetting the active drawer tab.
    refreshCityBuildings(capital.id);

    // ── Garnison ────────────────────────────────────────────────────────────
    if (pd) {
      const army    = pd.army || {};
      const present = Object.entries(UNIT_LBL).filter(([k]) => k in POP_COSTS && (army[k]||0) > 0);
      const totalDP  = Object.entries(UNIT_DP).reduce((s,[k,d])  => s + (army[k]||0)*d, 0);
      const totalPop = Object.entries(POP_COSTS).reduce((s,[k,c]) => s + (army[k]||0)*c, 0);
      document.getElementById('city-gar-sec').innerHTML = present.length
        ? `<table class="goods-mini">
            <tr style="color:var(--text-dim);font-size:.7rem"><td>Unit</td><td style="text-align:right">Count</td><td style="text-align:right">Pop</td><td style="text-align:right">DP</td></tr>
            ${present.map(([k,lbl]) => {
              const n = army[k]||0;
              return `<tr><td>${lbl}</td><td style="text-align:right">${n}</td><td style="text-align:right;color:var(--text-dim)">${n*POP_COSTS[k]}</td><td style="text-align:right;color:var(--safe)">${n*UNIT_DP[k]}</td></tr>`;
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

// ── City build action ─────────────────────────────────────────────────────
const _BLD_LBL = {
  farm:'Farm', barracks:'Barracks', mine:'Mine', lumbermill:'Lumbermill',
  stonequarry:'Stone Quarry', market:'Agora', wall:'Wall', tower:'Tower',
  harbour:'Harbour', foundry:'Foundry', stable:'Stable',
  bronze_wall:'Bronze Wall', olive_press:'Olive Press', winery:'Winery',
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
    resultEl.style.color = 'var(--safe)';
    resultEl.textContent = `${_BLD_LBL[btype]||btype} queued.`;
    // Refresh only the buildings section — avoids resetting the active tab
    await refreshCityBuildings(capital.id);
  } else {
    resultEl.style.color = 'var(--accent)';
    resultEl.textContent = d.error || 'Build failed.';
  }
}

export async function startCraft(provinceID) {
  const qtyEl = document.getElementById('city-craft-qty');
  const resultEl = document.getElementById('city-craft-result');
  const qty = qtyEl ? parseInt(qtyEl.value, 10) || 0 : 0;
  if (!resultEl || qty <= 0) return;
  resultEl.textContent = '';
  const r = await fetchAuth(
    `/api/v1/worlds/${State.WORLD_ID}/provinces/${provinceID}/craft`,
    { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({recipe_id: 1, quantity: qty}) }
  );
  const d = await r.json().catch(() => ({}));
  if (r.ok) {
    resultEl.style.color = 'var(--safe)';
    resultEl.textContent = `${qty} bronze crafted.`;
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
  const UNIT_LBL = {
    Spearman:'Spearmen', EliteInfantry:'Elite Infantry', WarChariot:'War Chariot',
    Ship:'Galley', WarGalley:'War Galley', Merchantman:'Emporos',
    spearman:'Spearmen', elite_infantry:'Elite Infantry', war_chariot:'War Chariot',
    ship:'Galley', galley:'Galley', war_galley:'War Galley', merchantman:'Emporos',
  };
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
      bq.map(b => `<tr><td>${_BLD_LBL[b.type]||b.type}</td><td>${fmtEta(b.complete_at)}</td>` +
        `<td style="text-align:right"><button class="btn-small" onclick="cancelBuild('${provinceID}','${b.id}')" style="padding:.05rem .3rem;font-size:.68rem;cursor:pointer">✕</button></td></tr>`).join('')
    }</table>`;
    if (tu.length) {
      // One row per maturing unit: land gathers men (forming, X/100), then trains
      // (100/100, ready ETA), then deploys to garrison; naval builds a vessel.
      h2 += `<div class="dsec-title" style="margin-top:.8rem">Training</div><table class="goods-mini">${
        tu.map(u => {
          const name = UNIT_LBL[u.unit] || u.unit;
          let label, eta = u.ready_at ? fmtEta(u.ready_at) : '';
          if (u.category === 'naval') label = 'building';
          else if (u.status === 'training') label = `${u.size}/100 · training`;
          else label = `${u.size}/100 · forming`;
          return `<tr><td>${name}</td><td>${label}</td><td>${eta}</td></tr>`;
        }).join('')
      }</table>`;
    }
    if (blds.some(b => b.type === 'foundry')) {
      const res = pd.resources || {};
      const copper = (res.copper || {}).amount || 0;
      const tin    = (res.tin    || {}).amount || 0;
      h2 += `
        <div class="dsec-title" style="margin-top:.8rem">Craft — Bronze</div>
        <div style="font-size:.72rem;color:var(--text-dim);margin-bottom:.3rem">2 copper + 1 tin → 1 bronze · stock: ${copper.toFixed(0)} copper, ${tin.toFixed(0)} tin</div>
        <div style="display:flex;gap:.4rem;align-items:center">
          <input type="number" id="city-craft-qty" min="1" value="1" style="width:4rem;background:var(--warm-white);border:1px solid var(--border);padding:.15rem .3rem;font-family:var(--mono);font-size:.75rem">
          <button class="btn-primary btn-small" onclick="startCraft('${provinceID}')">Craft →</button>
        </div>
        <div id="city-craft-result" class="action-result"></div>`;
    }
    const prevSel = document.getElementById('city-build-select')?.value || '';
    h2 += `
      <div class="dsec-title" style="margin-top:.8rem">Construct</div>
      <select id="city-build-select" class="build-select">
        <option value="farm">Farm — 50 timber 20 stone · +grain/m</option>
        <option value="lumbermill">Lumbermill — 40 timber 40 stone · +timber/m</option>
        <option value="stonequarry">Stone Quarry — 50 timber 20 stone · +stone/m</option>
        <option value="mine">Mine — 60 timber 40 stone · +ore/m</option>
        <option value="barracks">Barracks — 80 timber 80 stone · recruits</option>
        <option value="market">Market — 100 timber 60 stone · +0.5 silver/m</option>
        <option value="wall">Wall — upgrade (Palisade→Stone Wall→Bronze Wall)</option>
        <option value="harbour">Harbour — 140 timber 60 stone · ships</option>
        <option value="foundry">Foundry — 80 timber 100 stone · craft bronze</option>
        <option value="stable">Stable — 60 timber 40 stone · horses</option>
        <option value="temple">Temple — 60 timber 60 stone</option>
        <option value="olive_press">Olive Press — 30 timber 40 stone · +oil/m</option>
        <option value="winery">Winery — 40 timber 30 stone · +wine/m</option>
      </select>
      <button class="btn-primary btn-small" onclick="startBuild()" style="margin-top:.5rem;width:100%">+ Build</button>
      <div id="city-build-result" class="action-result"></div>`;
    bldSec.innerHTML = h2;
    // Restore previous dropdown selection and result message
    const newSel = document.getElementById('city-build-select');
    if (newSel && prevSel) newSel.value = prevSel;
  } catch(e) { console.error('refreshCityBuildings', e); }
}
