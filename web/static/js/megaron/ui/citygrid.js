import { State } from '../state.js';
import { fetchAuth } from '../api.js';
import { formatApiError } from './format.js';

// Gubbe placement grid (P5: megaron_plan_fysisk_gubbemodell.md). Replaces the
// old percent-per-good allocation table (DE2=B, 2026-08-07) with the P0-UI-
// locked hex raster: 18 catchment hexes + a centre hex that drills into the
// settlement's built workplaces. This module owns rendering AND the
// place/unplace actions — the whole placement surface lives here, not spread
// across city.js.
//
// Layout math is a small self-contained flat-top axial→pixel projection
// (redblobgames' standard formula), deliberately NOT importing render/map.js
// — that module is a large stateful world-camera renderer, this is a static
// 19-cell widget; duplicating ~6 lines of hex-corner math beats coupling to
// map.js's internals (see P5 research: hexPts/hexPath aren't exported and
// are tightly bound to State.camera/SCALE).
//
// Colour discipline: only megaron.css :root tokens are used (no new hex
// codes) — water hexes read via --sea (already the sea's own colour
// elsewhere in the UI), land via --bg-raised, selection via --accent, full
// via --safe. Terrain TYPE is read from the printed name, not invented hue —
// consistent with the "silhouette/text over colour-coding" convention
// elsewhere (megaron_kartaktorer.md's unit-type-in-silhouette principle,
// applied to a DOM widget instead of a sprite).

const HEX_SIZE = 30; // px, centre-to-corner
const WATER_TERRAINS = new Set(['coastal_sea', 'deep_sea', 'river', 'river_ford']);

function hexCorners(cx, cy, size) {
  const pts = [];
  for (let i = 0; i < 6; i++) {
    const angle = Math.PI / 180 * (60 * i);
    pts.push([cx + size * Math.cos(angle), cy + size * Math.sin(angle)]);
  }
  return pts.map(p => p.join(',')).join(' ');
}

function hexCenterPx(dq, dr) {
  return {
    x: HEX_SIZE * 1.5 * dq,
    y: HEX_SIZE * Math.sqrt(3) * (dr + dq / 2),
  };
}

function terrainLabel(t) {
  return (t || '').replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
}

// target is {target_kind:'hex', hex_ordinal} or {target_kind:'building', building_type}
// — resolved once by the caller (which already knows which hex/building this
// row belongs to) and carried on the row as JSON, so the click handler never
// has to re-derive it from the DOM.
function goodRowHTML(target, good) {
  const capped = good.cap != null;
  const full = capped && good.placed >= good.cap;
  const pipsHTML = capped
    ? Array.from({ length: good.cap }, (_, i) =>
        `<span class="gubbe-pip${i < good.placed ? '' : ' empty'}"></span>`).join('')
    : `<span style="color:var(--text-dim)">${good.placed} placed, uncapped</span>`;
  const nextLabel = full ? 'full' : `+${good.marginal_yield.toFixed(1)}/tick next`;
  const targetAttr = JSON.stringify(target).replace(/"/g, '&quot;');
  return `
    <div class="gubbe-good-row" data-good="${good.good_key}" data-cap="${good.cap ?? ''}" data-target="${targetAttr}">
      <div>
        <b>${good.good_key}</b>
        <span style="color:var(--text-dim)"> ${good.rate_per_tick.toFixed(1)}/tick total · ${nextLabel}</span>
        <div>${pipsHTML}</div>
      </div>
      <div style="display:flex;gap:.3rem">
        <button class="btn-small gubbe-act" data-verb="place1" ${full ? 'disabled' : ''}>+1</button>
        ${full ? '' : `<button class="btn-small gubbe-act" data-verb="fill">Fill</button>`}
        <button class="btn-small gubbe-act" data-verb="unplace1" ${good.placed > 0 ? '' : 'disabled'}>−1</button>
      </div>
    </div>`;
}

// refusalText turns a failed placement response into the line the player reads.
//
// The server has written eight distinct, human-readable refusals for this
// endpoint since it was built (api/handlers/settlement_placement.go): an empty
// citizen pool, an unscouted hex, a hex already fully staffed for that good, a
// good the terrain cannot produce, a hex outside the catchment, and so on.
// doAction used to `await fetchAuth(...)` without ever reading res.ok, so every
// one of those was thrown away and a refusal was indistinguishable from a
// working game — the player clicked +1, nothing moved, nothing was said. That
// is the most-repeated interaction in a new player's first hour.
//
// No new wording is invented here: the server's own sentence is what surfaces.
// formatApiError is the same helper every other action surface in the client
// already routes failures through (build, recruit, march, stance, load, repair,
// trade — 17 call sites); the placement grid was the only one that never
// adopted it. The status-code fallback covers a body that is empty or not JSON,
// which res.json() throws on.
export async function refusalText(res) {
  let data = null;
  try {
    data = await res.json();
  } catch {
    // empty or non-JSON body — fall through to the status-code fallback
  }
  return formatApiError(data, `Could not place that citizen (error ${res.status}).`);
}

// placementOutcome is the network half of a grid action, with no DOM in sight —
// the same "export the pure half" seam report.js uses for buildContext. It
// answers the only two questions the caller has: did anything change (→ worth
// re-rendering), and is there something to tell the player (→ refusal).
//
// Keeping this separate is what makes the silent-failure fix testable at all:
// the widget's own render path is SVG + innerHTML + listeners, and a test that
// had to fake all of that would prove nothing about the decision.
//
// `fetchImpl` is injected so a test can drive the whole verb table off scripted
// responses. Returns { changed, refusal, refresh }.
export async function placementOutcome(verb, ctx) {
  const { placementsURL, targetBody, good, cap, fetchImpl } = ctx;
  const post = () => fetchImpl(placementsURL, {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ ...targetBody, good_key: good }),
  });

  if (verb === 'place1') {
    const res = await post();
    if (!res.ok) return { changed: false, refusal: await refusalText(res) };
    return { changed: true, refusal: null };
  }

  if (verb === 'unplace1') {
    // Gubbar are anonymous/interchangeable (P0-UI) — removing "one" from
    // this hex/good just means the first ordinal found there.
    const listRes = await fetchImpl(placementsURL);
    if (!listRes.ok) return { changed: false, refusal: await refusalText(listRes) };
    const list = (await listRes.json()).placements || [];
    const match = list.find(p =>
      p.good_key === good &&
      (targetBody.target_kind === 'hex' ? p.hex_ordinal === targetBody.hex_ordinal : p.building_type === targetBody.building_type));
    if (!match) {
      // The −1 button is only enabled when placed > 0, so an empty match means
      // the grid is stale — say so AND refresh, rather than the old silent
      // return that left a button which visibly did nothing.
      return { changed: false, refresh: true, refusal: 'No citizen of yours is working that here any more — refreshing.' };
    }
    const del = await fetchImpl(`${placementsURL}/${match.gubbe_ordinal}`, { method: 'DELETE' });
    if (!del.ok) return { changed: false, refusal: await refusalText(del) };
    return { changed: true, refusal: null };
  }

  if (verb === 'fill') {
    // Client-side bulk loop (no bulk endpoint) — same "+1 repeated" the
    // player could do by hand, just automated for THIS place (P0-UI:
    // "Fyll den här platsen" is fine, "Fyll bästa X" over hexes is not —
    // this never chooses WHERE, only repeats the already-chosen target).
    const guard = cap != null ? cap : 500; // uncapped (grain): a sane stop, not a real limit
    let placedCount = 0;
    for (let i = 0; i < guard; i++) {
      const r = await post();
      if (!r.ok) {
        // Placed nothing at all: no pip will move, so this is the silent case
        // and the reason has to be said. Placed some and then stopped: the
        // pips moved, the player got their feedback, and the stop reason is
        // visible in the re-rendered row.
        if (placedCount === 0) return { changed: false, refusal: await refusalText(r) };
        break;
      }
      placedCount++;
    }
    return { changed: placedCount > 0, refusal: null };
  }

  return { changed: false, refusal: null };
}

// renderGubbeGrid mounts the whole widget (SVG raster + selection panel)
// into containerEl. centerQ/centerR are the settlement's own tile — the
// catchment ring is relative to it (server returns absolute hex_q/hex_r).
export async function renderGubbeGrid(containerEl, provinceID, centerQ, centerR) {
  containerEl.innerHTML = '<div class="loading" style="font-size:.8rem">Loading…</div>';

  let opts;
  try {
    const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${provinceID}/placement-options`);
    if (!res.ok) throw new Error('placement-options ' + res.status);
    opts = await res.json();
  } catch (e) {
    console.error('renderGubbeGrid', e);
    containerEl.innerHTML = '<p class="empty-state">Could not load catchment.</p>';
    return;
  }

  const hexes = opts.hexes || [];
  const buildings = opts.buildings || [];

  // SVG viewBox: compute from actual hex centres so the widget scales to
  // whatever the catchment ring's true pixel spread is, rather than a
  // hardcoded guess (a fixed guess would clip a differently-sized ring
  // silently — the ring is always 18, but paranoia here is cheap).
  const points = [{ dq: 0, dr: 0, isCentre: true, ordinal: 0 }]
    .concat(hexes.map(h => ({ dq: h.hex_q - centerQ, dr: h.hex_r - centerR, hex: h })));
  const pixels = points.map(p => ({ ...p, ...hexCenterPx(p.dq, p.dr) }));
  const pad = HEX_SIZE * 1.4;
  const minX = Math.min(...pixels.map(p => p.x)) - pad;
  const maxX = Math.max(...pixels.map(p => p.x)) + pad;
  const minY = Math.min(...pixels.map(p => p.y)) - pad;
  const maxY = Math.max(...pixels.map(p => p.y)) + pad;

  const cells = pixels.map(p => {
    if (p.isCentre) {
      return `<g class="gubbe-hex-g" data-target="city">
        <polygon class="gubbe-hex gubbe-city-hex" points="${hexCorners(p.x, p.y, HEX_SIZE)}"></polygon>
        <text x="${p.x}" y="${p.y}" class="gubbe-hex-label">City</text>
      </g>`;
    }
    const h = p.hex;
    const water = WATER_TERRAINS.has(h.terrain);
    const anyGood = (h.goods || [])[0];
    const full = anyGood && anyGood.cap != null && (h.goods || []).every(g => g.cap == null || g.placed >= g.cap);
    const cls = ['gubbe-hex', water ? 'water' : '', full ? 'full' : ''].filter(Boolean).join(' ');
    return `<g class="gubbe-hex-g" data-target="hex" data-ordinal="${h.hex_ordinal}">
      <polygon class="${cls}" points="${hexCorners(p.x, p.y, HEX_SIZE)}"></polygon>
      <text x="${p.x}" y="${p.y - 4}" class="gubbe-hex-label">#${h.hex_ordinal}</text>
      <text x="${p.x}" y="${p.y + 9}" class="gubbe-hex-sub">${(h.goods || []).length} good${(h.goods || []).length === 1 ? '' : 's'}</text>
    </g>`;
  }).join('');

  // 18 revealed ring positions is the norm (own catchment ⊂ own sight
  // radius, P4 §P4 doc comment) — but an ordinal missing here IS a real fog
  // hex (P0-UI: "FOW-hex = svart = icke-placerbar"), not a bug, so a gap in
  // the ring is drawn as a dim placeholder rather than silently omitted.
  const seenOrdinals = new Set(hexes.map(h => h.hex_ordinal));
  let fowCells = '';
  for (let ord = 1; ord <= 18; ord++) {
    if (seenOrdinals.has(ord)) continue;
    // No coordinate is known for an unseen hex ordinal — there is nowhere
    // principled to place it on the grid, so it is listed in text instead
    // of guessed onto the canvas.
    fowCells += `<div class="gubbe-fow-note">#${ord} not yet scouted</div>`;
  }

  containerEl.innerHTML = `
    <div style="display:flex;justify-content:space-between;align-items:baseline;margin-bottom:.3rem">
      <span>Citizens: <b>${opts.total_gubbar - opts.pool_size}/${opts.total_gubbar}</b> placed</span>
      <span style="color:var(--text-dim)">${opts.pool_size} idle</span>
    </div>
    <svg class="gubbe-svg" viewBox="${minX} ${minY} ${maxX - minX} ${maxY - minY}" preserveAspectRatio="xMidYMid meet">${cells}</svg>
    ${fowCells ? `<div class="gubbe-fow-list">${fowCells}</div>` : ''}
    <div id="gubbe-detail" class="gubbe-detail-panel">
      <p class="empty-state">Click a catchment hex, or the city hex for buildings.</p>
    </div>`;

  const rerender = () => renderGubbeGrid(containerEl, provinceID, centerQ, centerR);

  // A refusal is shown where the player just clicked and does NOT re-render:
  // nothing changed on the server, so rebuilding the widget would only throw
  // away the selected hex and leave the player with no way back to the button
  // they pressed. On success we re-render as before.
  const showRefusal = (msg) => {
    const detail = containerEl.querySelector('#gubbe-detail');
    if (!detail) return;
    let el = detail.querySelector('.gubbe-refusal');
    if (!el) {
      el = document.createElement('div');
      el.className = 'gubbe-refusal';
      detail.appendChild(el);
    }
    el.textContent = msg;
  };

  const doAction = async (verb, targetBody, good, cap) => {
    // Clear any refusal from a previous click so a stale line never reads as
    // the verdict on this one.
    const stale = containerEl.querySelector('.gubbe-refusal');
    if (stale) stale.remove();

    const out = await placementOutcome(verb, {
      placementsURL: `/api/v1/worlds/${State.WORLD_ID}/provinces/${provinceID}/placements`,
      targetBody, good, cap, fetchImpl: fetchAuth,
    });
    if (out.refusal) showRefusal(out.refusal);
    if (out.changed || out.refresh) await rerender();
  };

  containerEl.querySelectorAll('.gubbe-hex-g').forEach(g => {
    g.addEventListener('click', () => {
      containerEl.querySelectorAll('.gubbe-hex').forEach(el => el.classList.remove('selected'));
      g.querySelector('.gubbe-hex').classList.add('selected');
      const detail = containerEl.querySelector('#gubbe-detail');

      if (g.dataset.target === 'city') {
        if (!buildings.length) {
          detail.innerHTML = '<p class="empty-state">No workplace buildings staffed here yet.</p>';
          return;
        }
        detail.innerHTML = buildings.map(b => `
          <div class="dsec-title">${b.building_type} L${b.level}</div>
          ${(b.goods || []).map(good => goodRowHTML({ target_kind: 'building', building_type: b.building_type }, good)).join('')}
        `).join('');
      } else {
        const ordinal = Number(g.dataset.ordinal);
        const hex = hexes.find(h => h.hex_ordinal === ordinal);
        if (!hex) return;
        detail.innerHTML = `
          <div class="dsec-title">#${hex.hex_ordinal} — ${terrainLabel(hex.terrain)}</div>
          ${(hex.goods || []).length
            ? hex.goods.map(good => goodRowHTML({ target_kind: 'hex', hex_ordinal: hex.hex_ordinal }, good)).join('')
            : '<p class="empty-state">No producible good on this hex.</p>'}`;
      }

      detail.querySelectorAll('.gubbe-act').forEach(btn => {
        btn.addEventListener('click', () => {
          const row = btn.closest('.gubbe-good-row');
          const good = row.dataset.good;
          const cap = row.dataset.cap === '' ? null : Number(row.dataset.cap);
          // dataset already HTML-entity-decodes the attribute (the &quot;
          // escaping in goodRowHTML is only needed to keep the attribute
          // itself well-formed), so this is plain JSON by the time it's read.
          const target = JSON.parse(row.dataset.target);
          doAction(btn.dataset.verb, target, good, cap);
        });
      });
    });
  });
}
