import { State } from '../../state.js';
import { fetchAuth } from '../../api.js';
import { esc, fmtAgo } from '../format.js';

// ── Gossip + Wanaxes drawer (web/gossip-wanax-drawer) ──────────────────────
// Keryx parity: mirrors `keryx gossip` (cmd_gossip.go) for the rumour feed —
// region, category, text, age, importance and hops-away — plus a directory
// built from GET /wanaxes, the FOW-gated trade-discovery endpoint keryx's
// `messenger`/`resolveMessengerDest` already reads to resolve a destination
// by ruler name. Neither had a web surface before this slice
// (megaron_plan_webbytor_keryx_paritet.md, Slice GOSSIP+WANAXES).
//
// Distinct from diplomacy.js's Cities/Rulers tabs (which read /cities and
// /diplomacy) — this drawer reads /gossip and /wanaxes specifically, per the
// plan's endpoint list. Two sections in one drawer because keryx's own
// `gossip`/messenger-resolution feature set treats them as one picture: what
// you've heard, and who you now know as a result.

// "N hops away" mirrors keryx's hopLabel (cmd_gossip.go): only drawn once a
// rumour has actually travelled (hops > 0) — a locally-sourced rumour (0
// hops) carries no distance qualifier in either client.
export function hopsLabel(hops) {
  if (!hops || hops <= 0) return '';
  return hops === 1 ? '1 hop away' : hops + ' hops away';
}

export function renderGossipRowsHTML(items) {
  if (!items || !items.length) {
    return '<p class="empty-state" style="padding:1rem">No rumours have reached you yet.</p>';
  }
  return items.map(g => {
    const majorClass = g.importance === 'major' ? ' gossip-major' : '';
    const hops = hopsLabel(g.hops);
    return '<div class="inbox-item gossip-row' + majorClass + '">'
      + '<div class="ii-from">'
      +   esc(g.source_region || 'Unknown region')
      +   (g.category ? ' · ' + esc(g.category) : '')
      +   (hops ? ' <span class="gossip-hops">(' + hops + ')</span>' : '')
      +   '<span style="float:right">' + fmtAgo(g.generated_at) + '</span>'
      + '</div>'
      + '<div class="ii-body">' + esc(g.text) + '</div>'
      + '</div>';
  }).join('');
}

// The /wanaxes response is one row per SETTLEMENT (see world.go Wanaxes) —
// "known wanaxes" means the rulers behind those cities, so group by owner
// before rendering. A wanax with no owner name (shouldn't happen — the
// server filters owner_id IS NOT NULL — but defensively grouped rather than
// dropped) falls under "Unknown".
export function groupWanaxes(rows) {
  const byOwner = new Map();
  (rows || []).forEach(r => {
    const key = r.owner || 'Unknown';
    if (!byOwner.has(key)) byOwner.set(key, { owner: key, own: false, kingdom: '', settlements: [] });
    const entry = byOwner.get(key);
    if (r.own) entry.own = true;
    if (r.kingdom) entry.kingdom = r.kingdom;
    if (r.name) entry.settlements.push(r.name);
  });
  return [...byOwner.values()].sort((a, b) => a.owner.localeCompare(b.owner));
}

export function renderWanaxesHTML(rows) {
  const wanaxes = groupWanaxes(rows);
  if (!wanaxes.length) {
    return '<p class="empty-state" style="padding:1rem">No wanaxes known yet — explore the map.</p>';
  }
  return '<table class="goods-mini">'
    + '<tr style="color:var(--text-dim);font-size:.7rem"><td>Wanax</td><td>Kingdom</td><td>Cities</td></tr>'
    + wanaxes.map(w =>
        '<tr><td>' + esc(w.owner) + (w.own ? ' ★' : '') + '</td>'
        + '<td style="color:var(--text-dim)">' + esc(w.kingdom || '—') + '</td>'
        + '<td style="color:var(--text-dim)">' + esc(w.settlements.join(', ')) + '</td></tr>'
      ).join('')
    + '</table>';
}

export async function loadGossipDrawer() {
  const body = document.getElementById('gossip-body');
  if (!body) return;
  body.innerHTML = '<div class="loading" style="padding:.5rem">Loading…</div>';
  try {
    const [gossipRes, wanaxRes] = await Promise.all([
      fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/gossip`),
      fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/wanaxes`),
    ]);
    const gossip  = gossipRes && gossipRes.ok ? await gossipRes.json().catch(() => null) : null;
    const wanaxes = wanaxRes  && wanaxRes.ok  ? await wanaxRes.json().catch(() => null)  : null;
    body.innerHTML =
      '<div class="dsec"><div class="dsec-title">Rumours</div>'
      + (gossip === null
          ? '<p class="empty-state" style="padding:1rem">Could not load rumours.</p>'
          : renderGossipRowsHTML(gossip))
      + '</div>'
      + '<div class="dsec"><div class="dsec-title">Known Wanaxes</div>'
      + (wanaxes === null
          ? '<p class="empty-state" style="padding:1rem">Could not load wanaxes.</p>'
          : renderWanaxesHTML(wanaxes))
      + '</div>';
  } catch (e) {
    console.error('loadGossipDrawer', e);
    body.innerHTML = '<p class="empty-state" style="padding:.5rem">Could not load gossip.</p>';
  }
}
