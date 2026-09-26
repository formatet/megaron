// ── Movements — what is marching, out of your realm and into it ──
//
// Pure: the War drawer's Movements tab and the search overlay's "Your Armies"
// both list armies in the field. They used to read State.marchData alone, i.e.
// the legacy `marching_armies` table — which only recall and outpost paths
// still write, so an ordinary march never showed ("No armies in the field").
// The per-unit layer is where marches live now: own units with
// status 'marching' (GET /units) and foreign units in live vision that are
// marching onto one of your provinces (GET /foreign-units). Legacy rows are
// kept alongside so a recall column does not vanish from the list.
import { unitTypeLabel, actorName } from './actornames.js';

function cap(s) { return s ? s.charAt(0).toUpperCase() + s.slice(1) : ''; }

// warMovements returns { outgoing, incoming }, each a list of
// { title, target_q, target_r, origin_q, origin_r, arrives_at }.
// title is the unit/column line without the place name — the caller names
// the place, since only it knows how (search shows coords, War shows names).
export function warMovements({ units = [], foreign = [], marches = [], provinces = [] }) {
  const own = provinces.filter(p => p.own);
  const ownPos = new Set(own.map(p => p.q + ',' + p.r));
  const isOwn = (q, r) => ownPos.has(q + ',' + r);

  const outgoing = units
    .filter(u => u.status === 'marching' && u.target_q != null && u.target_r != null)
    .map(u => ({
      title: actorName(u) + (u.march_intent ? ' · ' + u.march_intent : ''),
      target_q: u.target_q, target_r: u.target_r,
      origin_q: u.q, origin_r: u.r,
      arrives_at: u.arrives_at,
    }));

  const incoming = foreign
    .filter(u => u.status === 'marching' && u.target_q != null && isOwn(u.target_q, u.target_r))
    .map(u => ({
      title: unitTypeLabel(u.type) + ' (' + u.size + ') of ' + u.owner,
      target_q: u.target_q, target_r: u.target_r,
      origin_q: u.q, origin_r: u.r,
      arrives_at: u.arrives_at,
    }));

  for (const m of marches) {
    const row = {
      title: cap(m.intent),
      target_q: m.target_q, target_r: m.target_r,
      origin_q: m.origin_q, origin_r: m.origin_r,
      arrives_at: m.arrives_at,
    };
    if (isOwn(m.origin_q, m.origin_r)) outgoing.push(row);
    else if (isOwn(m.target_q, m.target_r)) incoming.push(row);
  }

  const byArrival = (a, b) => String(a.arrives_at).localeCompare(String(b.arrives_at));
  return { outgoing: outgoing.sort(byArrival), incoming: incoming.sort(byArrival) };
}

// incomingTargetKeys: "q,r" of every own province something hostile is
// marching onto — the map's pulsing red glow. Same incoming list the War tab
// shows, so the map and the drawer can never disagree about a threat. Legacy
// columns count only with intent 'attack' (a recall column is no threat).
export function incomingTargetKeys({ foreign = [], marches = [], provinces = [] }) {
  const { incoming } = warMovements({
    foreign, provinces, marches: marches.filter(m => m.intent === 'attack'),
  });
  return new Set(incoming.map(m => m.target_q + ',' + m.target_r));
}
