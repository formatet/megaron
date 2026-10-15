// Road edges on the map (megaron_plan_vagar.md V2). A road is an EDGE between
// two neighbouring hexes, drawn from hex centre to hex centre as a whole-pixel
// line (no anti-aliasing, grafikregler 1). It is STRUCTURE, not signal: a dust
// stroke with a one-pixel dark shadow on the underside, quieter than a unit
// route. Edges come from GET /roads, which already holds back every edge whose
// two hexes the player has not seen — the client draws what it is given.
// The palette is the canvas renderer's own (exempt from the CSS variables).

export const ROAD_DUST = '#CDB67E';
export const ROAD_SHADOW = '#4A3A24';
export const ROAD_PLANNED = '#E3D3A4';

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
  for (const [a, b] of segments) for (const p of linePixels(a.x, a.y, b.x, b.y)) dust.push(p);
  ctx.fillStyle = ROAD_SHADOW;
  for (const [x, y] of dust) ctx.fillRect(x, y + 1, 1, 1);
  ctx.fillStyle = ROAD_DUST;
  for (const [x, y] of dust) ctx.fillRect(x, y, 1, 1);
  // Unfinished: every other pixel pair, no shadow.
  ctx.fillStyle = ROAD_PLANNED;
  for (const [a, b] of planned) {
    linePixels(a.x, a.y, b.x, b.y).forEach(([x, y], i) => { if (i % 4 < 2) ctx.fillRect(x, y, 1, 1); });
  }
  ctx.restore();
}
