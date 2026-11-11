import { BUILDING_LABELS as _BLD_LBL } from './building_names.js';
import { State } from '../state.js';
import { fetchAuth } from '../api.js';
import { esc } from './format.js';
import { fmtNum, fmtDuration } from './fmt_num.js';
import { fmtArrival } from './time.js';
import { serverNow } from '../clock.js';

// Building by clicking: the rows of the Buildings tab's Construct list and the
// rows the placement grid shows for a hex. Pure string builders over the same
// data the old dropdowns used (building catalogue, province build_quotes,
// placement-options) -- no rules are derived here.

export const TEMPLE_GODS = { potnia:'Potnia', velchanos:'Velchanos', asterion:'Asterion', sea_daimons_stillness:'the Sea Daimons of Stillness' };
export function templeName(god) { return `Temple of ${TEMPLE_GODS[god] || god}`; }

// The one request body every build sends. Key order is the wire format.
export function buildBody(spec) {
  const body = { building_type: spec.type };
  if (spec.type === 'temple') body.god = spec.god;
  if (spec.hex_q != null && spec.hex_r != null) { body.hex_q = spec.hex_q; body.hex_r = spec.hex_r; }
  return body;
}

// Building catalogue (GET /api/v1/buildings): static for a world, fetched once;
// a failure clears the memo and returns null (never [] -- that would mean "no buildings").
let _cataloguePromise = null;
export async function getBuildingCatalogue() {
  if (!_cataloguePromise) {
    _cataloguePromise = fetchAuth('/api/v1/buildings').then(r => {
      if (!r.ok) throw new Error('buildings catalogue fetch failed: ' + r.status);
      return r.json();
    }).catch(e => {
      console.error('getBuildingCatalogue', e);
      _cataloguePromise = null;
      return null;
    });
  }
  return _cataloguePromise;
}

// "1 timber 9 stone", rounded like the CLI (%.0f per good), alphabetical by good
// (the server's map order is not stable).
export function fmtBuildCost(entry) {
  const costs = Object.entries(entry.costs || {}).filter(([, v]) => v > 0);
  if (entry.cost_silver > 0) costs.push(['silver', entry.cost_silver]);
  return costs.sort(([a], [b]) => a.localeCompare(b)).map(([k, v]) => `${Math.round(v)} ${k}`).join(' ');
}

const WALL_STATIC = 'upgrade (Palisade→Stone Wall→Bronze Wall)';
const step = q => `L${q.current_level} → L${q.next_level}`;

// The line "if started now, ready ≈ 15:28" (empty without a tick length).
export function readyIfStarted(days) {
  if (!(State.TICK_SECONDS > 0) || !Number.isFinite(days)) return '';
  return 'if started now, ready ' + fmtArrival(new Date(serverNow() + days * State.TICK_SECONDS * 1000).toISOString(), undefined, 'ready');
}

// One row per thing the catalogue offers. ctx = { walls, siloLevel, buildQuotes,
// canAfford, queueFull }. A row that cannot be built now is `locked` with ONE
// `reason`; reasons come from the catalogue/quotes/province the server sent.
export function buildRows(catalogue, ctx = {}) {
  const hasQuotes = Object.hasOwn(ctx, 'buildQuotes');
  const quotes = ctx.buildQuotes || [];
  const affordRow = type => (ctx.canAfford || []).find(a => a.type === type);
  const rows = [];
  const add = (row, entry, quote, afford) => {
    const src = quote || entry;
    const r = { hexBound: false, god: '', level: '', locked: false, reason: '', ...row,
      cost: fmtBuildCost(src), days: Number.isFinite(src.duration_game_days) ? src.duration_game_days : null };
    if (!r.locked && ctx.queueFull) { r.locked = true; r.reason = 'the build queue is full'; }
    if (!r.locked && afford && afford.can_afford === false && !afford.at_max_level) { r.locked = true; r.reason = 'not enough goods in store'; }
    rows.push(r);
  };
  for (const entry of catalogue || []) {
    const type = entry.type;
    if (type === 'wall') {
      const next = (entry.wall_ladder || []).find(w => w.level === Number(ctx.walls) + 1);
      if (!next) {
        const top = ctx.walls != null && Number(ctx.walls) >= 3;
        rows.push({ key: 'wall', type, hexBound: false, god: '', label: 'Wall', level: '', cost: '', note: top ? '' : WALL_STATIC, days: null, locked: top, reason: top ? 'at the highest level' : '' });
      } else if (Number(ctx.siloLevel) < next.requires_silo) {
        rows.push({ key: 'wall', type, hexBound: false, god: '', label: next.name, level: '', cost: fmtBuildCost({ costs: next.costs }), note: `holds ${fmtNum(next.population_cap)}`, days: null, locked: true, reason: `needs a Granary of level ${next.requires_silo}` });
      } else {
        add({ key: 'wall', type, label: next.name, note: `holds ${fmtNum(next.population_cap)}` }, { costs: next.costs }, null, null);
      }
      continue;
    }
    if (type === 'temple') {
      for (const god of Object.keys(TEMPLE_GODS)) {
        const quote = quotes.find(q => q.type === 'temple' && q.god_id === god);
        const base = { key: `temple:${god}`, type, god, label: templeName(god) };
        if (hasQuotes && !quote) rows.push({ ...base, level: '', cost: '', days: null, locked: true, reason: 'price unavailable' });
        else if (quote?.at_max_level) rows.push({ ...base, level: `L${quote.current_level}`, cost: '', days: null, locked: true, reason: 'maximum level' });
        else add({ ...base, level: quote ? step(quote) : '' }, entry, quote, null);
      }
      continue;
    }
    const label = _BLD_LBL[type] || type;
    if (entry.hex_bound && hasQuotes) {
      const site = quotes.find(q => q.type === type && q.new_site);
      add({ key: type, type, label, hexBound: true, level: site ? step(site) : '' }, entry, site, null);
      continue;
    }
    const quote = quotes.find(q => q.type === type && !q.new_site && q.hex_q == null);
    const base = { key: type, type, label };
    if (hasQuotes && !quote) rows.push({ ...base, level: '', cost: '', days: null, locked: true, reason: 'price unavailable' });
    else if (quote?.at_max_level) rows.push({ ...base, level: `L${quote.current_level}`, cost: '', days: null, locked: true, reason: 'maximum level' });
    else add({ ...base, level: quote ? step(quote) : '' }, entry, quote, affordRow(type));
  }
  // The name the player reads decides the order (Timothy 2026-10-24).
  return rows.sort((a, b) => a.label.localeCompare(b.label, 'en'));
}

function rowText(r) {
  const head = `<b>${esc(r.label)}</b>${r.level ? ' ' + esc(r.level) : ''}`;
  const bits = [r.cost && esc(r.cost), r.note && esc(r.note), r.days != null && fmtDuration(r.days)].filter(Boolean);
  return head + (bits.length ? ' · ' + bits.join(' · ') : '');
}

export function buildRowHTML(r) {
  const attrs = `data-build-type="${esc(r.type)}"${r.god ? ` data-build-god="${esc(r.god)}"` : ''}`;
  const eta = !r.locked && r.days != null ? readyIfStarted(r.days) : '';
  const button = r.locked ? ''
    : r.hexBound ? `<button class="btn-small btn-primary build-row-btn" data-build-act="pick" ${attrs}>Choose a hex →</button>`
    : `<button class="btn-small btn-primary build-row-btn" data-build-act="build" ${attrs}>Build</button>`;
  return `<div class="build-row${r.locked ? ' locked' : ''}" ${attrs}>
    <div class="build-row-text">${rowText(r)}${eta ? `<div class="build-row-sub">${esc(eta)}</div>` : ''}${r.locked ? `<div class="build-row-reason">${esc(r.reason)}</div>` : ''}</div>${button}</div>`;
}

// The Construct list. A catalogue that could not be loaded is one honest line.
export function constructListHTML(catalogue, ctx) {
  if (!catalogue) return '<p class="empty-state">Could not load the building list. Reopen the city to retry.</p>';
  return `<div class="build-list">${buildRows(catalogue, ctx).map(buildRowHTML).join('')}</div>`;
}

// Hexes where a hex-bound type can be built or levelled, from ONE
// placement-options response: valid_hexes_for_building[type] (new sites) plus
// hexes already carrying a building of that same type (the upgrade path, which
// the server leaves out of the valid list). Each carries its own quote.
export function hexBuildChoices(buildingType, placementOpts, quotes) {
  if (!placementOpts) return [];
  const price = (q, r) => quotes?.find(x => x.type === buildingType && x.hex_q === q && x.hex_r === r) || quotes?.find(x => x.type === buildingType && x.new_site);
  const byCoord = new Map((placementOpts.hexes || []).map(h => [`${h.hex_q},${h.hex_r}`, h]));
  const seen = new Set();
  const out = [];
  for (const { q, r, effect } of (placementOpts.valid_hexes_for_building || {})[buildingType] || []) {
    seen.add(`${q},${r}`);
    out.push({ q, r, effect: effect || '', upgrade: false, quote: price(q, r), terrain: byCoord.get(`${q},${r}`)?.terrain || '' });
  }
  for (const h of placementOpts.hexes || []) {
    if (seen.has(`${h.hex_q},${h.hex_r}`) || h.building?.type !== buildingType) continue;
    out.push({ q: h.hex_q, r: h.hex_r, effect: h.building.upgrade_effect || '', upgrade: true, level: h.building.level, quote: price(h.hex_q, h.hex_r), terrain: h.terrain || '' });
  }
  return out.map(c => ({ ...c, maxed: !!c.quote?.at_max_level }));
}

export function hexStepLabel(c) {
  if (c.quote && !c.quote.at_max_level) return step(c.quote);
  if (c.upgrade && c.level != null) return `L${c.level} → L${c.level + 1}`;
  return c.quote?.at_max_level ? `L${c.quote.current_level} (maximum)` : 'L0 → L1';
}

// Pick mode: Map "q,r" -> step label for every hex that can take `type` now.
export function pickHexes(buildingType, placementOpts) {
  const m = new Map();
  for (const c of hexBuildChoices(buildingType, placementOpts, placementOpts?.build_quotes || []))
    if (!c.maxed) m.set(`${c.q},${c.r}`, hexStepLabel(c));
  return m;
}

// One row for building `type` on hex (c.q,c.r).
export function hexBuildRowHTML(type, c) {
  const label = esc(_BLD_LBL[type] || type);
  const price = c.quote ? `${esc(fmtBuildCost(c.quote))} · ${fmtDuration(c.quote.duration_game_days)}` : 'price unavailable';
  const eta = c.quote ? readyIfStarted(c.quote.duration_game_days) : '';
  return `<div class="build-row" data-build-type="${esc(type)}" data-build-q="${c.q}" data-build-r="${c.r}">
    <div class="build-row-text"><b>${label}</b> ${esc(hexStepLabel(c))} · ${price}${c.effect ? `<div class="build-row-sub">${esc(c.effect)}</div>` : ''}${eta ? `<div class="build-row-sub">${esc(eta)}</div>` : ''}</div>
    <button class="btn-small btn-primary build-row-btn" data-build-act="build-hex" data-build-type="${esc(type)}" data-build-q="${c.q}" data-build-r="${c.r}">Build</button></div>`;
}

// The rows for one hex: every hex-bound type from the catalogue that hex accepts.
export function hexRowsHTML(catalogue, placementOpts, q, r, onlyType) {
  const quotes = placementOpts?.build_quotes || [];
  return (catalogue || []).filter(e => e.hex_bound && (!onlyType || e.type === onlyType))
    .sort((a, b) => (_BLD_LBL[a.type] || a.type).localeCompare(_BLD_LBL[b.type] || b.type, 'en'))
    .flatMap(e => hexBuildChoices(e.type, placementOpts, quotes).filter(c => c.q === q && c.r === r && !c.maxed).map(c => hexBuildRowHTML(e.type, c)))
    .join('');
}
