import { State } from '../state.js';
import { fetchAuth } from '../api.js';
import { track } from '../telemetry.js';
import { esc } from './format.js';
import { unitTypeLabel } from './actornames.js';
import { arrivalHTML } from './time.js';
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

const marchCtx = document.getElementById('march-ctx');

export function closeMarchCtx() {
  marchCtx.style.display = 'none';
  State.marchCtxDest   = null;
  State.marchCtxUnits  = [];
  State.marchCtxGroups = [];
  document.getElementById('mctx-err').textContent = '';
  const etaEl = document.getElementById('mctx-eta');
  if (etaEl) { etaEl.style.display = 'none'; etaEl.innerHTML = ''; }
  document.getElementById('mctx-send').style.display = ''; // restore after a post-send confirmation collapse
  const nameEl = document.getElementById('mctx-colony-name');
  if (nameEl) { nameEl.value = ''; nameEl.style.display = 'none'; }
  const chk = document.getElementById('mctx-colonize-chk');
  if (chk) chk.checked = false;
  const prevEl = document.getElementById('mctx-colonize-preview');
  if (prevEl) { prevEl.style.display = 'none'; prevEl.innerHTML = ''; }
  // The menu is gone — so is any catchment preview it armed (Bugg 3).
  State.catchmentPreview = null;
  State.dirty = true;
}

export async function onColonizeToggle() {
  const chk = document.getElementById('mctx-colonize-chk');
  const nameEl = document.getElementById('mctx-colony-name');
  if (nameEl) nameEl.style.display = chk && chk.checked ? 'block' : 'none';

  // 7-hex catchment tint (render §3.6) while the colonize box is armed — NOT
  // the FOV band (that's the plain march button's affordance, bindMarchButton
  // in render/map.js). Mirrors the box's own on/off, independent of whether
  // the forecast text below (colonize-preview element) exists.
  State.catchmentPreview = (chk && chk.checked && State.marchCtxDest)
    ? { q: State.marchCtxDest.q, r: State.marchCtxDest.r } : null;
  State.dirty = true;

  // Colonize catchment forecast (DEL A parity with keryx): show the founding
  // grain balance before the march is committed. Best-effort — a failed fetch
  // never blocks the March button.
  const prevEl = document.getElementById('mctx-colonize-preview');
  if (!prevEl) return;
  if (!(chk && chk.checked) || !State.marchCtxDest) {
    prevEl.style.display = 'none';
    prevEl.innerHTML = '';
    repositionMarchCtx();
    return;
  }
  const dest = State.marchCtxDest;
  prevEl.style.display = 'block';
  prevEl.innerHTML = '<span style="color:var(--text-dim)">Reading the land…</span>';
  repositionMarchCtx();
  try {
    const r = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/colonize-preview?q=${dest.q}&r=${dest.r}`);
    if (!r.ok) throw new Error();
    const p = await r.json();
    if (State.marchCtxDest !== dest) return; // the menu moved on while we fetched
    prevEl.innerHTML = renderColonizePreviewHTML(p);
  } catch (_) {
    if (State.marchCtxDest === dest) prevEl.innerHTML = '<span style="color:var(--text-dim)">No forecast available.</span>';
  }
  repositionMarchCtx();
}

// Mirrors keryx's renderColonizePreview (cmd_unit.go): grain prod − cons = net
// per tick (tick == day, mig 109 — the server's rates are already per-tick,
// no ×24), seed reach, farm note, plus known deposits/goods. FOW-safe — only
// known:true catchment hexes
// contribute to the deposit list; unknown hexes are counted, not guessed at.
// Exported for the founder-phase Host panel (render/map.js via the window
// bridge) — the founding forecast is the SAME surface with ?pop=&seed=.
export function renderColonizePreviewHTML(p) {
  const g = p.grain || {};
  const total = (p.catchment || []).length;
  const known = total - (p.unknown_hexes || 0);
  // base_per_tick/est_net_per_tick/with_farm_per_tick are already per-tick —
  // no ×24 here now that tick == day (mig 109); that used to convert an
  // hourly tick rate to a daily one and is the same class of stale scaling
  // as cmd_goods.go's Rate/d bug.
  const prodTick = g.base_per_tick || 0;
  const netTick  = g.est_net_per_tick || 0;
  const consTick = prodTick - netTick;

  let html = `<div style="color:var(--text-dim)">Catchment forecast — ${known}/${total} hexes known</div>`;
  html += `<div>Grain: prod ~${prodTick.toFixed(0)} − cons ~${consTick.toFixed(0)} = ` +
    `<b style="color:${netTick < 0 ? 'var(--accent)' : 'var(--safe)'}">net ${netTick >= 0 ? '+' : ''}${netTick.toFixed(0)}/tick</b></div>`;
  if (netTick < 0) {
    const reach = g.ticks_until_empty != null ? ` → lasts ~${g.ticks_until_empty.toFixed(0)} ticks` : '';
    const farmNetTick = (g.with_farm_per_tick || 0) - consTick;
    const farmNote = (g.with_farm_per_tick || 0) <= (g.base_per_tick || 0)
      ? ' (no farmland in known catchment — a farm will not help here)' : '';
    html += `<div>Seed ${(g.seed || 0).toFixed(0)} grain${reach}. With farm: ${farmNetTick >= 0 ? '+' : ''}${farmNetTick.toFixed(0)}/tick${farmNote}</div>`;
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
    const rate = p.goods[gk] || 0;
    if (rate > 0) extras.push(`${gk} ~${rate.toFixed(0)}/tick`);
  });
  if (extras.length) html += `<div style="color:var(--text-dim)">Also: ${extras.join(', ')}</div>`;

  // Founding gifts (metropolis only — the server sends founding_gifts for
  // ?starter_farm=1). They fall out of the same geography as the numbers above,
  // so a founder comparing two sites sees them here instead of discovering them
  // after the irreversible settle.
  (p.founding_gifts || []).forEach(gift => {
    html += `<div>🎁 <b>${gift.label}</b> <span style="color:var(--text-dim)">— ${gift.detail}</span></div>`;
  });

  // Per-hex breakdown (DEL C — terräng-luckan): the aggregate above answers
  // "will it feed itself"; this answers "what IS the catchment" hex by hex, so
  // the Host panel lets a founder actually inspect the ground before settling
  // instead of only seeing the one hex the host stands on. FOW-safe: a
  // catchment entry only carries terrain/deposit fields when known:true — an
  // unknown hex has nothing here to leak. Server orders `catchment` as
  // [centre, ...6 neighbours] (world.go ColonizePreview) — entry 0 is tagged
  // "centrum" for readability, but nothing above depends on that ordering.
  const hexRows = (p.catchment || []).map((ce, i) => {
    const centerTag = i === 0 ? ' <span style="color:var(--text-dim)">(center)</span>' : '';
    if (!ce.known) return `<div>? <span style="color:var(--text-dim)">unexplored</span>${centerTag}</div>`;
    const hexDeps = [];
    if (ce.copper_deposit) hexDeps.push('copper');
    if (ce.tin_deposit)    hexDeps.push('tin');
    if (ce.silver_deposit) hexDeps.push('silver');
    if (ce.cedar_deposit)  hexDeps.push('cedar');
    const depStr = hexDeps.length ? ` <span style="color:var(--text-dim)">· ${hexDeps.join(', ')}</span>` : '';
    return `<div>${catchmentTerrainLabel(ce.terrain)}${depStr}${centerTag}</div>`;
  }).join('');
  if (hexRows) {
    html += `<div style="font-size:.7rem;line-height:1.4;margin-top:.4rem;border-top:1px solid var(--border);padding-top:.3rem">${hexRows}</div>`;
  }
  return html;
}

// Human terrain label for the per-hex catchment breakdown above. Not the full
// TERRAIN_LABELS map from render/map.js (not importable here without pulling
// in the whole canvas renderer) — same fallback shape as its terrainLabel().
function catchmentTerrainLabel(t) {
  return t.charAt(0).toUpperCase() + t.slice(1).replaceAll('_', ' ');
}

let lastCtxPos = null;

function positionMarchCtx(screenX, screenY) {
  lastCtxPos = { x: screenX, y: screenY };
  const vw = window.innerWidth, vh = window.innerHeight;
  const w = marchCtx.offsetWidth, h = marchCtx.offsetHeight;
  marchCtx.style.left = Math.min(screenX + 8, vw - w - 8) + 'px';
  // Clamp top ≥ 8 too: a menu taller than the viewport must anchor at the top
  // (max-height + overflow in .march-ctx takes it from there), never above it.
  marchCtx.style.top  = Math.max(8, Math.min(screenY + 8, vh - h - 8)) + 'px';
}

// Re-clamp against the viewport after content loads/expands async (unit list,
// colonize forecast) — the menu was positioned when it was still small, so a
// late expansion pushed the send button below the screen edge.
function repositionMarchCtx() {
  if (lastCtxPos && marchCtx.style.display !== 'none') positionMarchCtx(lastCtxPos.x, lastCtxPos.y);
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

  // Eligible to march: garrisoned or positioned, deployable.
  // Naval hex → ships; land hex → land units. u.deployable is the server's own
  // field (status != forming/training, api/handlers/unit.go:1342) — the server
  // has no size gate on march (march_start.go), so a battle-worn cohort below
  // 100 men is still orderable. Fortify stance blocks march server-side
  // (march_start.go:132-135) and must not show as eligible here either.
  const wantNaval = dest.isSea;
  State.marchCtxUnits = all.filter(u => {
    if (u.status !== 'garrison' && u.status !== 'positioned') return false;
    const naval = u.category === 'naval';
    if (wantNaval !== naval) return false;
    if (!u.deployable) return false;
    if (u.stance === 'fortify') return false;
    return true;
  });

  renderMarchUnitList();
  positionMarchCtx(screenX, screenY);
}

// Each unit is one vessel (naval) or one 100-man stack (land). Group the
// fungible ones by type + origin so the player picks a quantity ("send 3 of 5")
// instead of hunting identical checkboxes — the count expands back into that
// many discrete /units/{id}/march calls on send.
//
// The grouping is worth keeping; what it used to throw away was IDENTITY. The
// row label was unitTypeLabel(g.type), i.e. the type, and a field unit's only
// location tag is a raw (q,r) — so telling two spearmen apart meant counting
// hexes (Timothy 2026-08-04). Groups therefore carry `names` alongside `ids`,
// in the SAME order, because sendMarch takes the first n of ids.
export function groupMarchUnits(units, provinceData) {
  const byKey = new Map();
  for (const u of units) {
    const prov = (provinceData || []).find(p => p.settlement_id === u.settlement_id || p.id === u.settlement_id);
    const loc  = prov ? prov.name : (u.q != null ? '(' + u.q + ',' + u.r + ')' : '');
    const key  = u.type + '|' + loc;
    if (!byKey.has(key)) byKey.set(key, { type: u.type, loc, ids: [], names: [] });
    const g = byKey.get(key);
    g.ids.push(u.id);
    // display_name is server-formatted ("First Spearmen of Knossos") and always
    // present today; the type label is a fallback so a partial payload leaves a
    // readable row rather than a blank one.
    g.names.push(u.display_name || unitTypeLabel(u.type));
  }
  return Array.from(byKey.values());
}

// A group of one is fungible with nothing, so it wears its own name. A group of
// several keeps the type label — the count is the affordance there, and the
// members are listed by marchGroupNamesHTML below.
export function marchGroupLabelHTML(g) {
  const single = g.ids.length === 1;
  const head   = single ? g.names[0] : unitTypeLabel(g.type);
  // "First Spearmen of Knossos · Knossos" says Knossos twice: the name's "of X"
  // is the SUPPORTING town, loc is where the unit stands, and for a garrisoned
  // unit those coincide. Drop the tag only when it literally repeats the name's
  // own tail — a field unit's (q,r), or a unit garrisoned away from home, keeps it.
  const redundant = single && head.endsWith('of ' + g.loc);
  const locTag = (g.loc && !redundant)
    ? ' <span class="mctx-loc">· ' + esc(g.loc) + '</span>'
    : '';
  return esc(head) + locTag;
}

// Numbered, because the count sends the first n in this order — the list is the
// queue, not just a roster.
export function marchGroupNamesHTML(g) {
  if (g.ids.length < 2) return '';
  return '<div class="mctx-names">'
    + g.names.map((n, k) => (k + 1) + '. ' + esc(n)).join('<br>')
    + '</div>';
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
  State.marchCtxGroups = groupMarchUnits(State.marchCtxUnits, State.provinceData);
  el.innerHTML = State.marchCtxGroups.map((g, i) => {
    const max = g.ids.length;
    return '<div class="mctx-row">'
      + '<span class="mctx-label">' + marchGroupLabelHTML(g) + '</span>'
      + '<input class="mctx-input" type="number" id="mg-' + i + '" min="0" max="' + max + '" value="0">'
      + '<span class="mctx-max">/' + max + '</span>'
      + '</div>'
      + marchGroupNamesHTML(g);
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
    }).then(async res => {
      const data = await res.json().catch(() => ({}));
      return { ok: res.ok, data, err: res.ok ? '' : (data.error || 'March failed') };
    });
  }));

  document.getElementById('mctx-send').disabled = false;
  const failed = results.filter(r => !r.ok);
  if (failed.length < results.length) {
    track('march_sent', { intent: colonize ? 'colonize' : State.marchCtxDest.isSea ? 'explore' : (stance || 'march') });
    // Refetch the units the order just moved. Without this the unit stays
    // 'garrison' in State until the 30s poll — and worse, map.js's 3s fast
    // poll is GATED on State.unitsData containing a marching unit, so it can
    // never start itself: the data that would wake it is the data nobody
    // fetched. The unit is then invisible for up to 30s and reappears already
    // arrived, which reads as teleportation (Timothy 2026-08-04). Harmless in
    // the live world where a tick is 3600s — the poll always beats a multi-hour
    // march — and glaring in the acceptance world at tick 6s, where 30s is five
    // game hours and the whole march fits between two samples.
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units`)
      .then(r => r.ok && r.json().then(d => { State.unitsData = d.units || []; State.dirty = true; }));
    // A field unit's order rides a runner, so nothing is marching yet — the
    // messenger is what the fast poll must see (its own `courierOut` branch).
    if (results.some(r => r.ok && r.data.status === 'order_dispatched')) {
      fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/messengers`)
        .then(r => r.ok && r.json().then(d => { State.messengerData = d; State.dirty = true; }));
    }
  }
  // Arrival line (Fas B): the march response carries the authoritative
  // arrival_tick + derived arrives_at_utc (K4). Same destination for every
  // unit sent, so the first success speaks for all of them. The panel stays
  // open so the ETA is readable — Escape/click-away closes it as usual.
  const first = results.find(r => r.ok);
  const etaEl = document.getElementById('mctx-eta');
  // A field unit's order travels by runner (temenos_orderlopare_plan.md
  // Fas 5): the 202 is a dispatch receipt with the COURIER's ETA — the march
  // begins only on delivery. Garrisoned units keep the immediate arrival line.
  const dispatched = first && first.data.status === 'order_dispatched';
  const showEta = first && etaEl && (dispatched || first.data.arrives_at_utc || first.data.arrives_at);
  if (showEta) {
    etaEl.innerHTML = dispatched
      ? '🏃 Runner carries the order — reaches the unit ' +
        arrivalHTML(first.data.courier_arrives_at, first.data.courier_due_tick) +
        '; the march begins on delivery'
      : '✓ Marching — arrives ' +
        arrivalHTML(first.data.arrives_at_utc || first.data.arrives_at, first.data.arrival_tick);
    etaEl.style.display = 'block';
  }
  if (failed.length) {
    const okCount = results.length - failed.length;
    document.getElementById('mctx-err').textContent = (okCount ? okCount + ' sent · ' : '') + failed[0].err;
  } else if (showEta) {
    // All sent — collapse the order form into a confirmation so a second
    // "March →" can't re-send the same (now marching) units. Escape/click-away
    // closes; openMarchCtx rebuilds the form from scratch next time.
    document.getElementById('mctx-units').innerHTML = '';
    document.getElementById('mctx-stance-row').style.display = 'none';
    document.getElementById('mctx-colonize-row').style.display = 'none';
    document.getElementById('mctx-send').style.display = 'none';
  } else {
    closeMarchCtx();
  }
  // Refresh either way — some units may have dispatched. Units drive the map's
  // per-unit movement layer; marches keeps the legacy layer/music in sync.
  fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units`).then(r => r.ok && r.json().then(d => { State.unitsData = d.units || []; State.dirty = true; }));
  fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/marches`).then(r => r.ok && r.json().then(d => { State.marchData = d; State.dirty = true; MusicPlayer.update(); }));
  // …and messengers, so a dispatched runner appears on the map at once.
  fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/messengers`).then(r => r.ok && r.json().then(d => { State.messengerData = d; State.dirty = true; }));
}

document.addEventListener('keydown', e => { if (e.key === 'Escape') closeMarchCtx(); });
document.addEventListener('mousedown', e => {
  if (marchCtx.style.display !== 'none' && !marchCtx.contains(e.target) && e.target !== canvas) {
    closeMarchCtx();
  }
});
