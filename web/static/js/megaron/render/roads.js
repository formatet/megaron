// Road edges on the map (megaron_plan_vagar.md V2). A road is an EDGE between
// two neighbouring hexes, drawn from hex centre to hex centre as a whole-pixel
// line (no anti-aliasing, grafikregler 1). It is STRUCTURE, not signal: a dust
// stroke with a one-pixel dark shadow on the underside, quieter than a unit
// route. Edges come from GET /roads, which already holds back every edge whose
// two hexes the player has not seen — the client draws what it is given.
// The palette is the canvas renderer's own (exempt from the CSS variables).

export const ROAD_DUST = '#CDB67E';
export const ROAD_SHADOW = '#4A3A24';
const ROAD_PLANNED = '#E3D3A4';

// Bresenham over whole pixels, endpoints included.
export function linePixels(ax, ay, bx, by) {
  let x = Math.round(ax), y = Math.round(ay);
  const tx = Math.round(bx), ty = Math.round(by);
  const dx = Math.abs(tx - x), sx = x < tx ? 1 : -1;
  const dy = -Math.abs(ty - y), sy = y < ty ? 1 : -1;
  let err = dx + dy;
  const out = [];
  for (;;) {
    out.push([x, y]);
    if (x === tx && y === ty) break;
    const e = 2 * err;
    if (e >= dy) { err += dy; x += sx; }
    if (e <= dx) { err += dx; y += sy; }
  }
  return out;
}

// windingPixels is linePixels bent into a crooked track: three waypoints at 1/4,
// 1/2 and 3/4 of the edge are pushed sideways by a fixed amount drawn from the
// edge itself, so a road looks trodden rather than ruled. The ends stay on the
// hex centres, so neighbouring edges still join, and the same edge always bends
// the same way from either side (the hash reads the endpoints in sorted order).
export function windingPixels(ax, ay, bx, by) {
  let [x0, y0, x1, y1] = [ax, ay, bx, by];
  if (x0 > x1 || (x0 === x1 && y0 > y1)) [x0, y0, x1, y1] = [x1, y1, x0, y0];
  const len = Math.hypot(x1 - x0, y1 - y0);
  if (len < 4) return linePixels(ax, ay, bx, by);
  const nx = -(y1 - y0) / len, ny = (x1 - x0) / len;
  let h = (Math.round(x0) * 73856093) ^ (Math.round(y0) * 19349663) ^ (Math.round(x1) * 83492791) ^ (Math.round(y1) * 2654435761);
  const rnd = () => { h = Math.imul(h ^ (h >>> 15), 2246822507); h = Math.imul(h ^ (h >>> 13), 3266489909); h ^= h >>> 16; return ((h >>> 0) / 4294967296) * 2 - 1; };
  const pts = [[x0, y0]];
  for (const t of [0.25, 0.5, 0.75]) {
    const off = rnd() * len * 0.16;
    pts.push([x0 + (x1 - x0) * t + nx * off, y0 + (y1 - y0) * t + ny * off]);
  }
  pts.push([x1, y1]);
  const out = [];
  for (let i = 1; i < pts.length; i++) {
    const seg = linePixels(pts[i - 1][0], pts[i - 1][1], pts[i][0], pts[i][1]);
    for (const p of i === 1 ? seg : seg.slice(1)) out.push(p);
  }
  return ax === x0 && ay === y0 ? out : out.reverse();
}

// roadSegments turns the server's edges into pixel segments. hexPx(q, r) is the
// renderer's hex-centre function. An edge needs both ends; anything malformed is dropped.
export function roadSegments(edges, hexPx) {
  const out = [];
  for (const e of edges || []) {
    if (![e.q1, e.r1, e.q2, e.r2].every(Number.isFinite)) continue;
    out.push([hexPx(e.q1, e.r1), hexPx(e.q2, e.r2)]);
  }
  return out;
}

// plannedSegments: the owner's own unfinished roads (GET /road-sites), drawn
// dotted so a road that is waiting reads differently from a finished one.
export function plannedSegments(sites, hexPx) {
  const out = [];
  for (const s of sites || []) {
    if (![s.from_q, s.from_r, s.to_q, s.to_r].every(Number.isFinite)) continue;
    out.push([hexPx(s.from_q, s.from_r), hexPx(s.to_q, s.to_r)]);
  }
  return out;
}

export function drawRoads(ctx, segments, planned = []) {
  ctx.save();
  const dust = [];
  for (const [a, b] of segments) for (const p of windingPixels(a.x, a.y, b.x, b.y)) dust.push(p);
  ctx.fillStyle = ROAD_SHADOW;
  for (const [x, y] of dust) ctx.fillRect(x, y + 1, 1, 1);
  ctx.fillStyle = ROAD_DUST;
  for (const [x, y] of dust) ctx.fillRect(x, y, 1, 1);
  // Unfinished: every other pixel pair, no shadow.
  ctx.fillStyle = ROAD_PLANNED;
  for (const [a, b] of planned) {
    windingPixels(a.x, a.y, b.x, b.y).forEach(([x, y], i) => { if (i % 4 < 2) ctx.fillRect(x, y, 1, 1); });
  }
  ctx.restore();
}
