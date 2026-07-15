import { State } from '../state.js';
import { fetchAuth } from '../api.js';
import { esc } from './format.js';
import { MusicPlayer } from './misc.js';
import { canvas } from '../render/map.js';

// ── March context menu (per-unit model) ───────────────────────────────────
// Right-clicking a hex orders individual units to it via
// POST /worlds/{id}/units/{unitID}/march. The old aggregate
// /provinces/{id}/march route was removed in the per-unit migration; the map
// now marches discrete units, the same model the War drawer uses. A sea hex
// lists only ships (galleys); a land hex lists only land units. Attack vs
// reinforce is decided server-side on arrival from the target's ownership —
// there is no client-chosen intent beyond optional colonize + stance.
const UNIT_MENU_LABELS = {
  spearman:'Spearmen', elite_infantry:'Elite Infantry', war_chariot:'War Chariot',
  ship:'Galley', galley:'Galley', war_galley:'War Galley', merchantman:'Emporos',
  nomadic_host:'Nomadic Host',
};


const marchCtx = document.getElementById('march-ctx');

export function closeMarchCtx() {
  marchCtx.style.display = 'none';
  State.marchCtxDest   = null;
  State.marchCtxUnits  = [];
  State.marchCtxGroups = [];
  document.getElementById('mctx-err').textContent = '';
  const nameEl = document.getElementById('mctx-colony-name');
  if (nameEl) { nameEl.value = ''; nameEl.style.display = 'none'; }
  const chk = document.getElementById('mctx-colonize-chk');
  if (chk) chk.checked = false;
  const prevEl = document.getElementById('mctx-colonize-preview');
  if (prevEl) { prevEl.style.display = 'none'; prevEl.innerHTML = ''; }
}

export async function onColonizeToggle() {
  const chk = document.getElementById('mctx-colonize-chk');
  const nameEl = document.getElementById('mctx-colony-name');
  if (nameEl) nameEl.style.display = chk && chk.checked ? 'block' : 'none';

  // Colonize catchment forecast (DEL A parity with keryx): show the founding
  // grain balance before the march is committed. Best-effort — a failed fetch
  // never blocks the March button.
  const prevEl = document.getElementById('mctx-colonize-preview');
  if (!prevEl) return;
  if (!(chk && chk.checked) || !State.marchCtxDest) {
    prevEl.style.display = 'none';
    prevEl.innerHTML = '';
    return;
  }
  const dest = State.marchCtxDest;
  prevEl.style.display = 'block';
  prevEl.innerHTML = '<span style="color:var(--text-dim)">Reading the land…</span>';
  try {
    const r = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/colonize-preview?q=${dest.q}&r=${dest.r}`);
    if (!r.ok) throw new Error();
    const p = await r.json();
    if (State.marchCtxDest !== dest) return; // the menu moved on while we fetched
    prevEl.innerHTML = renderColonizePreviewHTML(p);
  } catch (_) {
    if (State.marchCtxDest === dest) prevEl.innerHTML = '<span style="color:var(--text-dim)">No forecast available.</span>';
  }
}

// Mirrors keryx's renderColonizePreview (cmd_unit.go): grain prod − cons = net
// per game-day (rates are per-tick from the server, ×24), seed reach, farm
// note, plus known deposits/goods. FOW-safe — only known:true catchment hexes
// contribute to the deposit list; unknown hexes are counted, not guessed at.
// Exported for the founder-phase Host panel (render/map.js via the window
// bridge) — the founding forecast is the SAME surface with ?pop=&seed=.
export function renderColonizePreviewHTML(p) {
  const td = 24;
  const g = p.grain || {};
  const total = (p.catchment || []).length;
  const known = total - (p.unknown_hexes || 0);
  const prodDay = (g.base_per_tick || 0) * td;
  const netDay  = (g.est_net_per_tick || 0) * td;
  const consDay = prodDay - netDay;

  let html = `<div style="color:var(--text-dim)">Catchment forecast — ${known}/${total} hexes known</div>`;
  html += `<div>Grain: prod ~${prodDay.toFixed(0)} − cons ~${consDay.toFixed(0)} = ` +
    `<b style="color:${netDay < 0 ? 'var(--accent)' : 'var(--safe)'}">net ${netDay >= 0 ? '+' : ''}${netDay.toFixed(0)}/day</b></div>`;
  if (netDay < 0) {
    const reach = g.days_until_empty != null ? ` → lasts ~${g.days_until_empty.toFixed(0)} days` : '';
    const farmNetDay = (g.with_farm_per_tick || 0) * td - consDay;
    const farmNote = (g.with_farm_per_tick || 0) <= (g.base_per_tick || 0)
      ? ' (no farmland in known catchment — a farm will not help here)' : '';
    html += `<div>Seed ${(g.seed || 0).toFixed(0)} grain${reach}. With farm: ${farmNetDay >= 0 ? '+' : ''}${farmNetDay.toFixed(0)}/day${farmNote}</div>`;
    html += `<div style="color:var(--text-dim)">A colony does not feed itself — build a farm if the land bears it, or send grain by internal transfer.</div>`;
  } else {
    html += `<div>Seed ${(g.seed || 0).toFixed(0)} grain — the colony feeds itself.</div>`;
  }

  const dep = {};
  (p.catchment || []).forEach(ce => {
    if (!ce.known) return;
    if (ce.copper_deposit) dep.copper = true;
    if (ce.tin_deposit)    dep.tin = true;
    if (ce.silver_deposit) dep.silver = true;
    if (ce.cedar_deposit)  dep.cedar = true;
  });
  const extras = ['copper', 'tin', 'silver', 'cedar'].filter(d => dep[d]).map(d => d + '-deposit ✓');
  Object.keys(p.goods || {}).sort().forEach(gk => {
    if (gk === 'grain') return;
    const rate = (p.goods[gk] || 0) * td;
    if (rate > 0) extras.push(`${gk} ~${rate.toFixed(0)}/day`);
  });
  if (extras.length) html += `<div style="color:var(--text-dim)">Also: ${extras.join(', ')}</div>`;
  return html;
}

function positionMarchCtx(screenX, screenY) {
  const vw = window.innerWidth, vh = window.innerHeight;
  const w = marchCtx.offsetWidth, h = marchCtx.offsetHeight;
  marchCtx.style.left = Math.min(screenX + 8, vw - w - 8) + 'px';
  marchCtx.style.top  = Math.min(screenY + 8, vh - h - 8) + 'px';
}

// Open the march menu for a destination hex.
// dest = { q, r, terrain, isSea, name, isSettlement, allied }
export async function openMarchCtx(dest, screenX, screenY) {
  State.marchCtxDest  = dest;
  State.marchCtxUnits = [];
  document.getElementById('mctx-err').textContent = '';
  document.getElementById('mctx-name').textContent = dest.name;

  let hint;
  if (dest.isSea)             hint = 'Order galleys here — they reveal fog-of-war and sail home on their own.';
  else if (dest.isSettlement) hint = dest.allied ? 'March land units here to reinforce the garrison on arrival.' : 'March land units here to attack on arrival.';
  else                        hint = 'March land units to this hex, or found a new settlement here.';
  document.getElementById('mctx-hint').textContent = hint;

  // Colonize option only for empty land tiles — and never in founder phase:
  // a people without a city cannot colonize, they FOUND (the Host panel owns
  // that affordance).
  const colRow = document.getElementById('mctx-colonize-row');
  colRow.style.display = (!dest.isSea && !dest.isSettlement && !State.founderPhase) ? 'block' : 'none';
  const chk = document.getElementById('mctx-colonize-chk');
  if (chk) chk.checked = false;
  onColonizeToggle();

  marchCtx.style.display = 'block';
  positionMarchCtx(screenX, screenY);

  document.getElementById('mctx-units').innerHTML = '<span style="color:var(--text-dim);font-size:.75rem">Loading units…</span>';
  const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units`);
  if (!res.ok) { document.getElementById('mctx-units').innerHTML = '<span style="color:var(--accent);font-size:.75rem">Could not load units.</span>'; return; }
  const all = ((await res.json()).units) || [];

  // Eligible to march: garrisoned or positioned, non-priest, deployable.
  // Naval hex → ships; land hex → full-strength land units.
  const wantNaval = dest.isSea;
  State.marchCtxUnits = all.filter(u => {
    if (u.type === 'priest') return false;
    if (u.status !== 'garrison' && u.status !== 'positioned') return false;
    const naval = u.category === 'naval';
    if (wantNaval !== naval) return false;
    // size guards land units still forming (recruits trickle in 0→100). The
    // host is size=1 by construction — one movable marker, never "forming" —
    // and the server lets it march (Fas 2 rev size-grinden for it).
    if (!naval && u.size < 100 && u.type !== 'nomadic_host') return false;
    return true;
  });

  renderMarchUnitList();
  positionMarchCtx(screenX, screenY);
}

function renderMarchUnitList() {
  const el = document.getElementById('mctx-units');
  const stanceRow = document.getElementById('mctx-stance-row');
  State.marchCtxGroups = [];
  if (!State.marchCtxUnits.length) {
    el.innerHTML = '<p style="font-size:.73rem;color:var(--text-dim);margin:.3rem 0">'
      + (State.marchCtxDest && State.marchCtxDest.isSea
          ? 'No galleys available. Build ships in a coastal city first.'
          : 'No land units ready to march.') + '</p>';
    stanceRow.style.display = 'none';
    return;
  }
  // Each unit is one vessel (naval) or one 100-man stack (land). Group the
  // fungible ones by type + origin so the player picks a quantity ("send 3 of
  // 5") instead of hunting identical checkboxes — the count expands back into
  // that many discrete /units/{id}/march calls on send.
  const byKey = new Map();
  for (const u of State.marchCtxUnits) {
    const prov = State.provinceData.find(p => p.settlement_id === u.settlement_id || p.id === u.settlement_id);
    const loc  = prov ? prov.name : (u.q != null ? '(' + u.q + ',' + u.r + ')' : '');
    const key  = u.type + '|' + loc;
    if (!byKey.has(key)) byKey.set(key, { type: u.type, loc, ids: [] });
    byKey.get(key).ids.push(u.id);
  }
  State.marchCtxGroups = Array.from(byKey.values());
  el.innerHTML = State.marchCtxGroups.map((g, i) => {
    const lbl    = UNIT_MENU_LABELS[g.type] || g.type;
    const max    = g.ids.length;
    const locTag = g.loc ? ' <span style="color:var(--text-dim)">· ' + esc(g.loc) + '</span>' : '';
    return '<div class="mctx-row">'
      + '<span class="mctx-label">' + lbl + locTag + '</span>'
      + '<input class="mctx-input" type="number" id="mg-' + i + '" min="0" max="' + max + '" value="0">'
      + '<span class="mctx-max">/' + max + '</span>'
      + '</div>';
  }).join('');
  // Fleets have no stance — only land units take a stance.
  stanceRow.style.display = (State.marchCtxDest && State.marchCtxDest.isSea) ? 'none' : 'block';
}

export async function sendMarch() {
  if (!State.marchCtxDest) return;
  const picks = [];
  State.marchCtxGroups.forEach((g, i) => {
    const el = document.getElementById('mg-' + i);
    let n = el ? parseInt(el.value, 10) || 0 : 0;
    n = Math.max(0, Math.min(n, g.ids.length));
    for (let k = 0; k < n; k++) picks.push(g.ids[k]);
  });
  if (!picks.length) {
    document.getElementById('mctx-err').textContent = 'Choose how many units to send.';
    return;
  }
  // Naval fleets carry no stance.
  const stance = State.marchCtxDest.isSea ? '' : (document.getElementById('mctx-stance').value || '');
  const chk = document.getElementById('mctx-colonize-chk');
  const colonize = !!(chk && chk.checked
    && document.getElementById('mctx-colonize-row').style.display !== 'none');
  const nameEl = document.getElementById('mctx-colony-name');
  const colonyName = colonize && nameEl ? nameEl.value.trim() : '';

  document.getElementById('mctx-send').disabled = true;
  document.getElementById('mctx-err').textContent = '';

  const results = await Promise.all(picks.map(uid => {
    const body = { target_q: State.marchCtxDest.q, target_r: State.marchCtxDest.r };
    if (stance)   body.stance = stance;
    if (colonize) { body.intent = 'colonize'; if (colonyName) body.name = colonyName; }
    // Sea destinations are explore orders: the ship sweeps fog at the target
    // then sails home automatically — no separate recall needed.
    if (State.marchCtxDest.isSea) body.intent = 'explore';
    return fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units/${uid}/march`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
    }).then(async res => ({ ok: res.ok, err: res.ok ? '' : (((await res.json().catch(() => ({}))).error) || 'March failed') }));
  }));

  document.getElementById('mctx-send').disabled = false;
  const failed = results.filter(r => !r.ok);
  if (!failed.length) {
    closeMarchCtx();
  } else {
    const okCount = results.length - failed.length;
    document.getElementById('mctx-err').textContent = (okCount ? okCount + ' sent · ' : '') + failed[0].err;
  }
  // Refresh either way — some units may have dispatched. Units drive the map's
  // per-unit movement layer; marches keeps the legacy layer/music in sync.
  fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units`).then(r => r.ok && r.json().then(d => { State.unitsData = d.units || []; State.dirty = true; }));
  fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/marches`).then(r => r.ok && r.json().then(d => { State.marchData = d; State.dirty = true; MusicPlayer.update(); }));
}

document.addEventListener('keydown', e => { if (e.key === 'Escape') closeMarchCtx(); });
document.addEventListener('mousedown', e => {
  if (marchCtx.style.display !== 'none' && !marchCtx.contains(e.target) && e.target !== canvas) {
    closeMarchCtx();
  }
});
