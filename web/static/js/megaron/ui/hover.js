// ── Hover — what the map tooltip says about the units on a hex ──
//
// Pure: map.js works out WHERE every actor is drawn right now (the same
// interpolation its sprites use) and hands this module the ones standing on
// the hovered hex. This module only decides the words.
//
// Timothy 2026-09-26: every unit you can see — your own or someone else's —
// says whose it is and what it is called; your own units in motion say where
// they left from and where they are going; a caravan's cargo is shown only to
// the two parties of the shipment (the server blanks it for everyone else —
// megaron_plan_karavanbeslag.md).
import { actorName, unitTypeLabel } from './actornames.js';

function route(placeName, oq, or, dq, dr) {
  return `from ${placeName(oq, or)} to ${placeName(dq, dr)}`;
}

function cargo(t) {
  return t.good_key ? `${Math.floor(t.quantity || 0)} ${t.good_key}` : '';
}

// unitHoverLines returns one line per actor on the hex, in the order
// own units · foreign units · caravans · runners.
//   own       — own units (GET /units) standing here
//   foreign   — foreign units (GET /foreign-units) standing here; a marching
//               one carries the server's .heading (province.ReadMarch)
//   caravans  — trade markers (GET /trades) here
//   runners   — messenger markers (GET /messengers) here
//   placeName — (q, r) → the settlement's name, or "(q,r)"
export function unitHoverLines({ own = [], foreign = [], caravans = [], runners = [], placeName }) {
  const lines = [];
  for (const u of own) {
    let line = actorName(u);
    if (u.status === 'marching' && u.target_q != null && u.q != null) {
      line += ' — ' + route(placeName, u.q, u.r, u.target_q, u.target_r);
    }
    lines.push(line);
  }
  // A foreign march shows its HEADING, never its destination (Timothy
  // 2026-09-26): the watcher sees which way it goes now — which may be wrong
  // about where it ends up, if the road bends round a mountain.
  for (const u of foreign) {
    const name = u.name || unitTypeLabel(u.type);
    let line = u.owner ? `${name} (${u.owner})` : name;
    if (u.status === 'marching' && u.heading) line += ` — heading ${u.heading}`;
    lines.push(line);
  }
  for (const t of caravans) {
    const load = cargo(t);
    if (t.role === 'sender') {
      lines.push(`Your caravan${load ? ': ' + load : ''} — ` + route(placeName, t.origin_q, t.origin_r, t.dest_q, t.dest_r));
    } else if (t.role === 'recipient') {
      lines.push(`Caravan to you${t.owner ? ' from ' + t.owner : ''}${load ? ': ' + load : ''}`);
    } else {
      lines.push(t.owner ? `Caravan of ${t.owner}` : 'Caravan');
    }
  }
  for (const m of runners) {
    if (m.own) {
      // megaron_plan_budets_tre_ben.md R5: which physical leg the runner is
      // actually on — 'to_port' falls through to the ordinary route line
      // below (its endpoints already point at the port, R4's own "precis som
      // i dag" for that leg).
      if (m.leg === 'waiting') {
        lines.push(`Your Runner — waiting for passage at ${placeName(m.leg_from_q, m.leg_from_r)}`);
      } else if (m.leg === 'aboard') {
        lines.push(`Your Runner — aboard ${m.carrier_name || 'a ship'}`);
      } else if (m.leg === 'ashore') {
        lines.push(`Your Runner — ashore, running to ${placeName(m.dest_q, m.dest_r)}`);
      } else if (m.leg === 'sealed') {
        lines.push(`Your Runner — sealed, returning to ${placeName(m.leg_from_q, m.leg_from_r)}`);
      } else {
        lines.push('Your Runner — ' + route(placeName, m.origin_q, m.origin_r, m.dest_q, m.dest_r));
      }
    } else {
      lines.push(m.sender ? `Runner of ${m.sender}` : 'Runner');
    }
  }
  return lines;
}
