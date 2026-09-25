// Client mirror of the server's live-sight rule (server/internal/province/hex.go:
// LiveRadius, Eye.Sees, SeaSightline, hexLine) for the FOV preview band in
// render/map.js. The server is the truth; this only previews it. Pure — no DOM.
import { LIVE_RADIUS_SEA, LIVE_RADIUS_BASE, LIVE_RADIUS_MOUNTAIN_BONUS } from '../config.js';

export function isSea(terrain) {
  return terrain === 'coastal_sea' || terrain === 'deep_sea';
}

export function hexDist(q1, r1, q2, r2) {
  return (Math.abs(q1 - q2) + Math.abs(q1 + r1 - q2 - r2) + Math.abs(r1 - r2)) / 2;
}

// Ordinary vantage: eye kind's base radius, +bonus for mountains. A sea hex is
// read at this same vantage unless the open-water sightline reaches it.
export function liveRadius(kind, terrain) {
  let base = LIVE_RADIUS_BASE[kind] ?? LIVE_RADIUS_BASE.land;
  if (terrain === 'mountain_limestone' || terrain === 'mountain_red') base += LIVE_RADIUS_MOUNTAIN_BONUS;
  return base;
}

function cubeRound(q, s, r) {
  let rq = Math.round(q), rs = Math.round(s), rr = Math.round(r);
  const dq = Math.abs(rq - q), ds = Math.abs(rs - s), dr = Math.abs(rr - r);
  if (dq > ds && dq > dr) rq = -rs - rr;
  else if (ds > dr) { /* s is implied by q and r */ }
  else rr = -rq - rs;
  return [rq, rr];
}

// Hexes on the straight line a→b (both included), cube lerp at N = distance
// steps, both endpoints nudged by side·(1,2,-3)·ε so edge ties round consistently.
export function hexLine(aq, ar, bq, br, side) {
  const eps = 1e-6;
  const dq = side * eps, ds = side * 2 * eps, dr = -side * 3 * eps;
  const a = [aq + dq, -aq - ar + ds, ar + dr];
  const b = [bq + dq, -bq - br + ds, br + dr];
  const n = hexDist(aq, ar, bq, br);
  const out = [];
  for (let i = 0; i <= n; i++) {
    const t = n > 0 ? i / n : 0;
    out.push(cubeRound(a[0] + (b[0] - a[0]) * t, a[1] + (b[1] - a[1]) * t, a[2] + (b[2] - a[2]) * t));
  }
  return out;
}

// True when every hex strictly between (fq,fr) and the sea hex (tq,tr) is sea,
// on either tie-break side. terrainAt(q, r) returns a terrain or undefined; an
// unknown hex (fog, off the map) blocks — the preview may under-draw, never over.
export function seaSightline(fq, fr, tq, tr, terrainAt) {
  if (!isSea(terrainAt(tq, tr))) return false;
  for (const side of [1, -1]) {
    let clear = true;
    for (const [q, r] of hexLine(fq, fr, tq, tr, side)) {
      if ((q === fq && r === fr) || (q === tq && r === tr)) continue;
      if (!isSea(terrainAt(q, r))) { clear = false; break; }
    }
    if (clear) return true;
  }
  return false;
}

// Would an eye of kind at (fq,fr) see (tq,tr)? Mirrors server Eye.Sees.
export function eyeSees(kind, fq, fr, tq, tr, terrainAt) {
  const d = hexDist(fq, fr, tq, tr);
  if (d <= liveRadius(kind, terrainAt(tq, tr))) return true;
  return d <= LIVE_RADIUS_SEA && seaSightline(fq, fr, tq, tr, terrainAt);
}
