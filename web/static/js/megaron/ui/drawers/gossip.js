import { esc, fmtAgo } from '../format.js';

// ── Rumour rows ────────────────────────────────────────────────────────────
// Keryx parity: mirrors `keryx gossip` (cmd_gossip.go) — region, category,
// text, age, importance and hops-away. Since slice O rumours are marked rows
// of the notification list (notif.js); the separate Gossip drawer and its
// Known Wanaxes table are gone (Diplomacy → Known covers the rulers).

// "N hops away" mirrors keryx's hopLabel (cmd_gossip.go): only drawn once a
// rumour has actually travelled (hops > 0) — a locally-sourced rumour (0
// hops) carries no distance qualifier in either client.
export function hopsLabel(hops) {
  if (!hops || hops <= 0) return '';
  return hops === 1 ? '1 hop away' : hops + ' hops away';
}

// One rumour as a row of the notification list: marked "Rumour" so it can never
// be read as something that certainly happened.
export function renderRumourRowHTML(g) {
  const hops = hopsLabel(g.hops);
  return '<div class="notif-list-item nl-routine gossip-row' + (g.importance === 'major' ? ' gossip-major' : '') + '">'
    + '<span class="nli-kind">🗣</span>'
    + '<span class="nli-text"><i>Rumour</i> · ' + esc(g.source_region || 'Unknown region')
    + (g.category ? ' · ' + esc(g.category) : '')
    + (hops ? ' <span class="gossip-hops">(' + hops + ')</span>' : '')
    + ': ' + esc(g.text) + '</span>'
    + '<span class="nli-time">' + fmtAgo(g.generated_at) + '</span>'
    + '</div>';
}
