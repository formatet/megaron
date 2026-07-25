import { State, ownCapital } from '../state.js';
import { fetchAuth } from '../api.js';
import { track } from '../telemetry.js';
import { serverNow } from '../clock.js';
import {
  LIVE_RADIUS_SEA, LIVE_RADIUS_BASE, LIVE_RADIUS_MOUNTAIN_BONUS, LOCAL_ZOOM,
  GARRISON_DOT_ZOOM, ACTIVITY_BADGE_ZOOM, ROAD_DEPOSIT_ZOOM, ZOOM_MIN, ZOOM_MAX,
} from '../config.js';

// ── Palette — Settlers 2 warmth, Mediterranean olive country ─────────────
const TERRAIN_BASE = {
  deep_sea:           {c0:'#1A5276', c1:'#154360'},
  coastal_sea:        {c0:'#3A9AD9', c1:'#2E86C1'},
  plains:             {c0:'#8AAF3A', c1:'#74952C'},
  river_valley:       {c0:'#4CAF50', c1:'#388E3C'},
  river_delta:        {c0:'#6BBF59', c1:'#4E9B3E'},
  forest_olive_grove: {c0:'#5A8C30', c1:'#447020'},
  hills:              {c0:'#C8A464', c1:'#B08C50'},
  mountain_limestone: {c0:'#E0D4B8', c1:'#C8BCA0'},
  mountain_red:       {c0:'#C8906A', c1:'#B07050'},
  scrub_maquis:       {c0:'#A8B860', c1:'#909A48'},
  semi_desert:        {c0:'#D4B878', c1:'#C0A060'},
  fog:                {c0:'#1C1C1C', c1:'#252018'},
};

// Culture accent colours (banner / flag)
const CULTURE_ACCENT = {
  akhaier:  '#CA8A04',
  khemetiu: '#0E7490',
  knaani:   '#86198F',
  thrakes:  '#1D4ED8',
  minoan:   '#0891B2',
  hatti:    '#374151',
};

// ── Hex geometry ─────────────────────────────────────────────────────────
const S = 22;    // hex size in logical pixels — Settlers 2 scale
export const SCALE = 2; // canvas scale factor

export function hexPx(q, r) {
  return {
    x: Math.round(S * 1.5 * q),
    y: Math.round(S * Math.sqrt(3) * (r + q / 2)),
  };
}

function hexPts(cx, cy) {
  const pts = [];
  for (let i = 0; i < 6; i++) {
    const a = Math.PI / 3 * i;
    pts.push([Math.round(cx + S * Math.cos(a)), Math.round(cy + S * Math.sin(a))]);
  }
  return pts;
}

// Hit-test: nearest hex to canvas coords
function hexAtScreen(sx, sy) {
  const wx = (sx - State.camera.x) / (State.camera.zoom * SCALE);
  const wy = (sy - State.camera.y) / (State.camera.zoom * SCALE);
  const q = (2/3 * wx) / S;
  const r = (-1/3 * wx + Math.sqrt(3)/3 * wy) / S;
  const s = -q - r;
  let rq = Math.round(q), rr = Math.round(r), rs = Math.round(s);
  const dq = Math.abs(rq-q), dr = Math.abs(rr-r), ds = Math.abs(rs-s);
  if (dq > dr && dq > ds) rq = -rr - rs;
  else if (dr > ds) rr = -rq - rs;
  return {q: rq, r: rr};
}

// ── Hex path helpers ─────────────────────────────────────────────────────
// Six axial directions in cube space.
const HEX_DIRS = [[1,0],[-1,0],[0,1],[0,-1],[1,-1],[-1,1]];

// Hex distance in axial coords (cube formula).
function hexDist(q1, r1, q2, r2) {
  return (Math.abs(q1-q2) + Math.abs(q1+r1-q2-r2) + Math.abs(r1-r2)) / 2;
}

// Mirrors server/internal/province/hex.go LiveRadius(eyeKind, targetTerrain) —
// see config.js LIVE_RADIUS_* for the mirrored constants.
function liveRadius(kind, terrain) {
  if (terrain === 'coastal_sea' || terrain === 'deep_sea') return LIVE_RADIUS_SEA;
  let base = LIVE_RADIUS_BASE[kind] ?? LIVE_RADIUS_BASE.land;
  if (terrain === 'mountain_limestone' || terrain === 'mountain_red') base += LIVE_RADIUS_MOUNTAIN_BONUS;
  return base;
}

// Adjacent neighbor of (cq,cr) that is one step closer to (tq,tr).
// Ties broken by array order — stable for a given origin/destination.
function hexNeighborToward(cq, cr, tq, tr) {
  let best = null, bestD = Infinity;
  for (const [dq, dr] of HEX_DIRS) {
    const d = hexDist(cq+dq, cr+dr, tq, tr);
    if (d < bestD) { bestD = d; best = {q: cq+dq, r: cr+dr}; }
  }
  return best;
}

// Build an adjacency-guaranteed hex path from (q1,r1) to (q2,r2) via greedy steps.
// Cube-lerp has a known rounding defect where consecutive sampled hexes can be
// non-adjacent; this greedy approach is safe for small maps.
function buildHexPath(q1, r1, q2, r2) {
  const path = [{q: q1, r: r1}];
  let cq = q1, cr = r1;
  while (cq !== q2 || cr !== r2) {
    const n = hexNeighborToward(cq, cr, q2, r2);
    if (!n) break;
    path.push(n);
    cq = n.q; cr = n.r;
  }
  return path;
}

const hexPathCache = {};
function getHexPath(q1, r1, q2, r2) {
  const key = `${q1},${r1}-${q2},${r2}`;
  if (!hexPathCache[key]) hexPathCache[key] = buildHexPath(q1, r1, q2, r2);
  return hexPathCache[key];
}

// Interpolated pixel position along the hex path at fractional progress [0,1].
function hexPathPx(q1, r1, q2, r2, progress) {
  const path = getHexPath(q1, r1, q2, r2);
  if (path.length === 0) return hexPx(q1, r1);
  if (path.length === 1) return Object.assign({}, hexPx(q1, r1), {q: q1, r: r1});
  const idx   = Math.min(path.length - 2, Math.floor(progress * (path.length - 1)));
  const local = (progress * (path.length - 1)) - idx;
  const a = hexPx(path[idx].q, path[idx].r);
  const b = hexPx(path[idx+1].q, path[idx+1].r);
  return {
    x: a.x + (b.x - a.x) * local,
    y: a.y + (b.y - a.y) * local,
    q: path[idx].q,
    r: path[idx].r,
  };
}

// Interpolated pixel position along an explicit server-provided A* waypoint list
// [[q,r],...] at fractional progress [0,1]. Same interpolation as hexPathPx, but
// walks the true route (via sea / around mountains) instead of a straight hex
// line, so the walker is drawn where the unit actually is. Returns null for an
// empty list so callers can fall back to the straight-line hexPathPx.
function pathPx(waypoints, progress) {
  if (!waypoints || waypoints.length === 0) return null;
  if (waypoints.length === 1) {
    const p = hexPx(waypoints[0][0], waypoints[0][1]);
    return {x: p.x, y: p.y, q: waypoints[0][0], r: waypoints[0][1]};
  }
  const idx   = Math.min(waypoints.length - 2, Math.floor(progress * (waypoints.length - 1)));
  const local = (progress * (waypoints.length - 1)) - idx;
  const a = hexPx(waypoints[idx][0],   waypoints[idx][1]);
  const b = hexPx(waypoints[idx+1][0], waypoints[idx+1][1]);
  return {
    x: a.x + (b.x - a.x) * local,
    y: a.y + (b.y - a.y) * local,
    q: waypoints[idx][0],
    r: waypoints[idx][1],
  };
}

function isTileVisible(q, r) {
  return State.tileData.some(t => t.q === q && t.r === r && t.terrain !== 'fog');
}
// ── Hex fill — solid path fill + outline ─────────────────────────────────
function hexPath(ctx, pts) {
  ctx.beginPath();
  ctx.moveTo(pts[0][0], pts[0][1]);
  for (let i = 1; i < 6; i++) ctx.lineTo(pts[i][0], pts[i][1]);
  ctx.closePath();
}

function fillHex(ctx, pts, c0, c1, seed) {
  hexPath(ctx, pts);
  ctx.fillStyle = c0;
  ctx.fill();
  ctx.strokeStyle = c0;
  ctx.lineWidth = 1;
  ctx.stroke();
}

// ── Deterministic per-hex variation ──────────────────────────────────────
// A hex must look identical on every reload and for every player, but must
// not read as a pattern. The old scheme — seed = (q*137 + r*31) & 0xff, then
// masked per detail with & 0x1b — could only reach eight offsets with gaps
// between them, which is why forest read as wallpaper rather than woodland.
//
// This is a 32-bit avalanche mix (Murmur3 finalizer) over (worldSalt, q, r,
// stream): neighbouring hexes share no visible structure, and each `stream`
// is an independent draw for the same hex.
//
// The world component is hashed from State.WORLD_ID, NOT worlds.map_seed —
// map_seed never leaves the server, and WORLD_ID gives the same guarantee
// (stable per world, different between worlds) with no server change. This is
// presentation only: it must never feed mapgen or any rule.
let worldSalt = 0;
let worldSaltFor;
function salt() {
  if (worldSaltFor !== State.WORLD_ID) {
    worldSaltFor = State.WORLD_ID;
    let h = 0x9e3779b9;
    const s = String(State.WORLD_ID ?? '');
    for (let i = 0; i < s.length; i++) h = Math.imul(h ^ s.charCodeAt(i), 0x85ebca6b) >>> 0;
    worldSalt = h >>> 0;
  }
  return worldSalt;
}

function hash32(q, r, stream) {
  let h = (salt() ^ Math.imul(q | 0, 0x27d4eb2d) ^ Math.imul(r | 0, 0x165667b1)
           ^ Math.imul(stream | 0, 0x9e3779b9)) >>> 0;
  h = Math.imul(h ^ (h >>> 16), 0x85ebca6b) >>> 0;
  h = Math.imul(h ^ (h >>> 13), 0xc2b2ae35) >>> 0;
  return (h ^ (h >>> 16)) >>> 0;
}
// Uniform float in [0,1) / integer in [0,n) from one stream of a hex's variation.
const rnd = (q, r, stream) => hash32(q, r, stream) / 4294967296;
const rndInt = (q, r, stream, n) => hash32(q, r, stream) % n;

// ── Tile lookup index ────────────────────────────────────────────────────
// Terrain that reacts to its neighbours needs six lookups per hex per frame,
// and State.tileData.find() is O(n) over the whole map (2240 tiles on the
// current world) — that would be ~400k comparisons a frame for one forest
// block. State.tileData is only ever REASSIGNED (loadMap/refreshTiles), never
// mutated in place, so caching on array identity is safe and rebuilds itself
// exactly when the fog map changes.
let tileIndex = new Map();
let tileIndexFor;
function terrainAt(q, r) {
  if (tileIndexFor !== State.tileData) {
    tileIndexFor = State.tileData;
    tileIndex = new Map(State.tileData.map(t => [`${t.q},${t.r}`, t.terrain]));
  }
  return tileIndex.get(`${q},${r}`);
}

// Directions from this hex where the woodland ends. Fog and off-map count as
// "unknown", NOT as open ground: the player has not seen those tiles, and
// letting the treeline thin toward them would leak the neighbour's terrain
// through the shape of the edge. Unknown neighbours keep the forest dense.
function openEdges(q, r) {
  const open = [];
  for (const [dq, dr] of HEX_DIRS) {
    const t = terrainAt(q + dq, r + dr);
    if (t && t !== 'fog' && t !== 'forest_olive_grove') open.push([dq, dr]);
  }
  return open;
}

// ── Forest floor ─────────────────────────────────────────────────────────
// Woodland is ground first, canopy second. The flat mid-green fill gave the
// canopy nothing to sit against and made a block of forest hexes read as one
// undifferentiated green field. This lays a shaded understory, patches of bare
// warm earth and low vegetation — all clipped to the hex so nothing bleeds
// into a neighbour's terrain.
const FOREST_FLOOR = '#3F6522'; // shaded understory
const FOREST_EARTH = '#5C4830'; // bare ground glimpsed between the trees
const FOREST_SCRUB = '#7BA23C'; // low vegetation catching light
const FOREST_EDGE  = '#86A03A'; // sunlit treeline where the woodland opens up
function drawForestFloor(ctx, cx, cy, q, r) {
  ctx.save();
  hexPath(ctx, hexPts(cx, cy));
  ctx.clip();

  ctx.globalAlpha = 0.55;
  ctx.fillStyle = FOREST_FLOOR;
  ctx.fillRect(cx - S, cy - S, S * 2, S * 2);

  // Everything below is drawn as integer-aligned blocks, never as ellipses.
  // The trees above are pixel sprites, and a soft anti-aliased smudge under a
  // hard-edged sprite reads as two different pictures stacked in one hex.

  // Bare ground: many small specks, not few large blobs. A big brown shape
  // reads as a mud pool — worse, it competes with the copper deposit marker,
  // which is a small brown dot. Ground is texture here, never a shape.
  ctx.globalAlpha = 0.32;
  ctx.fillStyle = FOREST_EARTH;
  for (let i = 0; i < 9; i++) {
    const a = rnd(q, r, 10 + i) * Math.PI * 2;
    const d = 2 + rnd(q, r, 20 + i) * 13;
    const w = 2 + rndInt(q, r, 30 + i, 3);
    ctx.fillRect(Math.round(cx + Math.cos(a) * d), Math.round(cy + Math.sin(a) * d),
                 w, 1 + rndInt(q, r, 40 + i, 2));
  }

  // Deeper shade in the hollows, laid down as a two-row stagger so it reads as
  // dithered ground rather than a blurred patch.
  ctx.globalAlpha = 0.26;
  ctx.fillStyle = '#25451A';
  for (let i = 0; i < 5; i++) {
    const a = rnd(q, r, 70 + i) * Math.PI * 2;
    const d = rnd(q, r, 80 + i) * 12;
    const bx = Math.round(cx + Math.cos(a) * d), by = Math.round(cy + Math.sin(a) * d);
    const w = 3 + rndInt(q, r, 90 + i, 4);
    ctx.fillRect(bx, by, w, 1);
    ctx.fillRect(bx + 1, by + 1, Math.max(1, w - 2), 1);
  }

  ctx.globalAlpha = 0.55;
  ctx.fillStyle = FOREST_SCRUB;
  for (let i = 0; i < 6; i++) {
    const a = rnd(q, r, 50 + i) * Math.PI * 2;
    const d = rnd(q, r, 60 + i) * 15;
    ctx.fillRect(Math.round(cx + Math.cos(a) * d), Math.round(cy + Math.sin(a) * d), 2, 1);
  }

  // Bryn — where the woodland meets open country the floor lightens toward the
  // neighbour instead of stopping dead at the hex edge.
  //
  // Scattered, not washed: a smooth disc along the edge reads as a halo drawn
  // AROUND the forest, which is decoration, not terrain. These are seeded
  // specks whose density falls off inward (the inward offset is raised to a
  // power, so most sit close to the edge and a few reach deeper), which reads
  // as a treeline breaking up. Specks that cross the edge are cut by the clip,
  // so the opening continues into the neighbouring hex.
  ctx.fillStyle = FOREST_EDGE;
  const mid = S * Math.sqrt(3) / 2; // centre → edge midpoint
  openEdges(q, r).forEach(([dq, dr], e) => {
    const ex = S * 1.5 * dq;
    const ey = S * Math.sqrt(3) * (dr + dq / 2);
    const len = Math.hypot(ex, ey) || 1;
    const ux = ex / len, uy = ey / len;
    for (let i = 0; i < 7; i++) {
      const along  = (rnd(q, r, 200 + e * 20 + i) - 0.5) * S * 1.15;
      const inward = Math.pow(rnd(q, r, 300 + e * 20 + i), 1.7) * S * 0.6;
      const px = cx + ux * (mid - inward) - uy * along;
      const py = cy + uy * (mid - inward) + ux * along;
      // Per-speck coverage: a uniform alpha over many small shapes reads as
      // film grain. Varying it makes the same specks read as depth instead.
      ctx.globalAlpha = 0.22 + rnd(q, r, 600 + e * 20 + i) * 0.3;
      ctx.fillRect(Math.round(px), Math.round(py),
                   2 + rndInt(q, r, 400 + e * 20 + i, 4),
                   1 + rndInt(q, r, 500 + e * 20 + i, 2));
    }
  });
  ctx.restore();
}

// ── Canopy ───────────────────────────────────────────────────────────────
// Trees grow in stands, not in an even scatter. Three lone dots per hex read
// as a repeating pattern; composed clumps with air between them read as
// woodland. Two rules carry the look:
//
//   Dark core, warm edge — crowns deepen toward the middle of the hex and
//   warm toward its rim, so a block of forest gets an inside and an outside.
//   Thinning at the bryn — a clump close to open country is dropped or
//   shrunk, so the treeline the floor already opens up is not contradicted
//   by trees standing to attention right at the edge.
//
// Everything is seeded per hex and clipped to it, like the floor.
// Trees are drawn as PIXEL SPRITES, not as arcs. An arc anti-aliases, and the
// canvas is scaled (zoom × SCALE), so a smooth curve gets resampled into mush
// at every zoom level — which is what made the old crowns read as flat
// boardgame counters. fillRect on integer coordinates scales into hard-edged
// blocks instead, which is the whole point of the pixel-art rule in
// temenos_designprinciper (1px charcoal outline, no anti-aliasing, no gradients).
//
// K charcoal outline · D shadowed side · M body · L sunlit side · T trunk
// Light falls from the upper left, so every sprite runs L → M → D across its
// width. A crown shaded on one side is what separates a tree from a green
// counter; the old arcs had a highlight dot but no shaded side at all.
const TREE_TALL = [
  '..KKK..',
  '.KLLMK.',
  'KLLMMDK',
  'KLMMMDK',
  'KLMMMDK',
  '.KMMDK.',
  '..KTK..',
  '..KTK..',
];
const TREE_SQUAT = [
  '.KKK.',
  'KLMDK',
  'KLMDK',
  '.KTK.',
];
const TREE_PALETTE = { K: '#1B2810', D: '#2B4E14', M: '#3E6E1F', L: '#5FA02C', T: '#463218' };

// Same sprites, shifted toward the sunlit end of the ramp — used for stands at
// the warm rim of the wood so a forest block keeps an inside and an outside.
const TREE_PALETTE_WARM = { K: '#22300F', D: '#3A6418', M: '#548C24', L: '#7CBB38', T: '#4E3A1C' };

// Precompute each sprite as horizontal runs of equal colour: one fillRect per
// run instead of one per pixel, so a hex of trees costs tens of draw calls
// rather than hundreds.
function spriteRuns(rows) {
  const runs = [];
  const w = Math.max(...rows.map(r => r.length));
  rows.forEach((row, y) => {
    let x = 0;
    while (x < row.length) {
      const ch = row[x];
      let n = 1;
      while (x + n < row.length && row[x + n] === ch) n++;
      if (ch !== '.') runs.push({ ch, x, y, n });
      x += n;
    }
  });
  return { runs, w, h: rows.length };
}
const SPRITE_TALL = spriteRuns(TREE_TALL);
const SPRITE_SQUAT = spriteRuns(TREE_SQUAT);

// Origin is the trunk's foot, so trees "stand" on their position and a lower
// tree overlaps a higher one correctly when the stand is drawn back to front.
function drawTree(ctx, sprite, palette, fx, fy) {
  const ox = Math.round(fx) - (sprite.w >> 1);
  const oy = Math.round(fy) - sprite.h;
  for (const r of sprite.runs) {
    ctx.fillStyle = palette[r.ch];
    ctx.fillRect(ox + r.x, oy + r.y, r.n, 1);
  }
}

function drawCanopy(ctx, cx, cy, q, r) {
  ctx.save();
  hexPath(ctx, hexPts(cx, cy));
  ctx.clip();

  const mid = S * Math.sqrt(3) / 2;
  const edges = openEdges(q, r).map(([dq, dr]) => {
    const ex = S * 1.5 * dq;
    const ey = S * Math.sqrt(3) * (dr + dq / 2);
    const len = Math.hypot(ex, ey) || 1;
    return { x: (ex / len) * mid, y: (ey / len) * mid };
  });

  // Collect the stand first, then draw it back to front. The map is seen from
  // above but the trees are drawn in side elevation (¾, as in Settlers 2 /
  // Colonization): a tree stands on its position, so one further down the hex
  // must overlap one further up, and that only works if they are sorted.
  const stand = [];
  const clumps = 3 + rndInt(q, r, 2, 3);
  for (let c = 0; c < clumps; c++) {
    const a = rnd(q, r, 700 + c) * Math.PI * 2;
    const d = Math.sqrt(rnd(q, r, 710 + c)) * 12;
    const gx = Math.cos(a) * d, gy = Math.sin(a) * d;

    // 0 deep inside the wood → 1 standing on an open edge.
    let openness = 0;
    for (const e of edges) {
      openness = Math.max(openness, 1 - Math.hypot(gx - e.x, gy - e.y) / (S * 0.9));
    }
    openness = Math.min(1, Math.max(0, openness));
    if (rnd(q, r, 720 + c) < openness * 0.8) continue;

    const trees = 2 + rndInt(q, r, 730 + c, 3);
    for (let i = 0; i < trees; i++) {
      const ta = rnd(q, r, 740 + c * 8 + i) * Math.PI * 2;
      const td = rnd(q, r, 800 + c * 8 + i) * 5;
      // Squat trees fill in around the tall ones — a stand of identical
      // silhouettes reads as a stamp, which is the thing we are getting away from.
      const tall = rnd(q, r, 860 + c * 8 + i) > 0.38 && openness < 0.6;
      stand.push({
        x: cx + gx + Math.cos(ta) * td,
        y: cy + gy + Math.sin(ta) * td,
        sprite: tall ? SPRITE_TALL : SPRITE_SQUAT,
        warm: openness > 0.35 || d > 8.5,
      });
    }
  }
  stand.sort((a, b) => a.y - b.y);

  for (const t of stand) {
    // Ground shadow cast down-right: the light in this world comes from the
    // upper left (same side the sprites are lit on). It is what makes a tree
    // read as standing up rather than lying flat on the hex.
    ctx.globalAlpha = 0.38;
    ctx.fillStyle = '#1A2A10';
    ctx.fillRect(Math.round(t.x) - 1, Math.round(t.y) - 1, t.sprite.w - 1, 2);
    ctx.fillRect(Math.round(t.x) + 1, Math.round(t.y), t.sprite.w - 2, 1);
    ctx.globalAlpha = 1;
    drawTree(ctx, t.sprite, t.warm ? TREE_PALETTE_WARM : TREE_PALETTE, t.x, t.y);
  }
  ctx.restore();
}

// ── Terrain detail — Settlers 2 quality ──────────────────────────────────
function drawDetail(ctx, cx, cy, terrain, seed, frame, q, r) {
  ctx.save();
  const r3 = (seed * 7 + 3) & 0xf;
  const r4 = (seed * 13 + 5) & 0xf;
  switch (terrain) {
    case 'plains':
    case 'river_valley':
    case 'river_delta': {
      // tiny wheat stalks
      for (let i = 0; i < 4; i++) {
        const ox = ((seed * (i*7+1)) & 0x1f) - 14;
        const oy = ((seed * (i*5+3)) & 0x1f) - 14;
        ctx.strokeStyle = i % 2 === 0 ? '#D4C060' : '#A09030';
        ctx.lineWidth = 0.7;
        ctx.beginPath();
        ctx.moveTo(cx+ox, cy+oy+3);
        ctx.lineTo(cx+ox, cy+oy-3);
        ctx.stroke();
        ctx.fillStyle = '#E8D070';
        ctx.fillRect(cx+ox-0.5, cy+oy-4, 1, 2);
      }
      break;
    }
    case 'forest_olive_grove': {
      drawForestFloor(ctx, cx, cy, q, r);
      drawCanopy(ctx, cx, cy, q, r);
      break;
    }
    case 'hills': {
      // rounded pebbles
      ctx.fillStyle = '#A08050';
      for (let i = 0; i < 3; i++) {
        const ox = ((seed * (i*5+1)) & 0x17) - 10;
        const oy = ((seed * (i*7+2)) & 0x13) - 8;
        ctx.beginPath(); ctx.ellipse(cx+ox, cy+oy, 3.5, 2, 0.3, 0, Math.PI*2); ctx.fill();
      }
      break;
    }
    case 'mountain_limestone':
    case 'mountain_red': {
      // angular shards
      const mc = terrain === 'mountain_limestone' ? '#B0A888' : '#A06048';
      ctx.fillStyle = mc;
      for (let i = 0; i < 2; i++) {
        const ox = ((seed * (i*9+3)) & 0x13) - 8;
        const oy = ((seed * (i*6+1)) & 0x0f) - 6;
        ctx.beginPath();
        ctx.moveTo(cx+ox, cy+oy-5);
        ctx.lineTo(cx+ox+4, cy+oy+3);
        ctx.lineTo(cx+ox-4, cy+oy+3);
        ctx.closePath();
        ctx.fill();
      }
      break;
    }
    case 'scrub_maquis': {
      ctx.fillStyle = '#7A9040';
      for (let i = 0; i < 5; i++) {
        const ox = ((seed * (i*3+7)) & 0x1f) - 14;
        const oy = ((seed * (i*4+2)) & 0x1f) - 14;
        ctx.beginPath(); ctx.arc(cx+ox, cy+oy, 1.5, 0, Math.PI*2); ctx.fill();
      }
      break;
    }
    case 'coastal_sea':
    case 'deep_sea': {
      // animated sea shimmer
      const seaTick = (frame >> 5) & 0x7;
      const alpha = 0.06 + 0.04 * Math.sin(seaTick * 0.8 + seed * 0.1);
      ctx.fillStyle = `rgba(255,255,255,${alpha.toFixed(3)})`;
      ctx.beginPath();
      ctx.ellipse(cx + (r3-7)*0.8, cy + (r4-7)*0.5, 6, 2, 0.4, 0, Math.PI*2);
      ctx.fill();
      break;
    }
    case 'semi_desert': {
      ctx.fillStyle = '#C09050';
      for (let i = 0; i < 3; i++) {
        const ox = ((seed*(i*7+2))&0x17)-10, oy = ((seed*(i*5+3))&0x13)-8;
        ctx.fillRect(cx+ox, cy+oy, 1, 1);
      }
      break;
    }
  }
  ctx.restore();
}

// ── Deposit resource icons (Sprint 4.5) — tiny pixel markers ────────────
function drawDepositIcons(ctx, cx, cy, tile) {
  const types = [];
  if (tile.copper_deposit) types.push('cu');
  if (tile.tin_deposit)    types.push('sn');
  if (tile.cedar_deposit)  types.push('cd');
  if (tile.silver_deposit) types.push('ag');
  if (!types.length) return;
  ctx.save();
  // Charcoal outline on every marker. These are game information sitting on
  // top of terrain that now has its own texture and tone, and an unoutlined
  // solid loses its silhouette the moment the ground behind it stops being
  // flat — cedar's dark green triangle disappeared into the canopy outright.
  // The outline is also what the project's own pixel-art rule asks for
  // (1px charcoal on solids, temenos_designprinciper.md).
  ctx.strokeStyle = '#221E18';
  ctx.lineWidth = 0.8;
  ctx.lineJoin = 'round';
  types.forEach((t, i) => {
    const ox = cx + 9, oy = cy - 8 + i * 5;
    switch (t) {
      case 'cu':
        ctx.fillStyle = '#C47C20';
        ctx.beginPath(); ctx.arc(ox, oy, 2, 0, Math.PI*2); ctx.fill(); ctx.stroke();
        break;
      case 'sn':
        ctx.fillStyle = '#909090';
        ctx.fillRect(ox - 2, oy - 1.5, 4, 3);
        ctx.strokeRect(ox - 2, oy - 1.5, 4, 3);
        break;
      case 'cd':
        ctx.fillStyle = '#2A7010';
        ctx.beginPath(); ctx.moveTo(ox, oy - 3); ctx.lineTo(ox + 2.5, oy + 1.5); ctx.lineTo(ox - 2.5, oy + 1.5); ctx.closePath(); ctx.fill(); ctx.stroke();
        break;
      case 'ag':
        ctx.fillStyle = '#C0C8D8';
        ctx.beginPath(); ctx.moveTo(ox, oy - 2.5); ctx.lineTo(ox + 2, oy); ctx.lineTo(ox, oy + 2.5); ctx.lineTo(ox - 2, oy); ctx.closePath(); ctx.fill(); ctx.stroke();
        break;
    }
  });
  ctx.restore();
}

// ── Province building sprite + flag ──────────────────────────────────────
function drawProvince(ctx, cx, cy, p) {
  // Razed (Del 2b sack) or collapsed: an abandoned ruin, not a standing city —
  // no owner, no flag, no garrison dot. Dim broken rubble instead of the building.
  if (p.state === 'razed' || p.state === 'collapsed') {
    ctx.save();
    ctx.fillStyle = '#5A5048';
    ctx.strokeStyle = '#2A2420';
    ctx.lineWidth = 0.8;
    ctx.fillRect(cx - 4, cy - 2, 3, 4);
    ctx.strokeRect(cx - 4, cy - 2, 3, 4);
    ctx.fillRect(cx, cy - 4, 4, 3);
    ctx.strokeRect(cx, cy - 4, 4, 3);
    ctx.fillStyle = '#3A342E';
    ctx.fillRect(cx - 1, cy + 1, 2, 1);
    ctx.restore();
    return;
  }
  const walls = Math.min(3, p.walls || 0);
  const accent = p.own ? '#D4AC0D' : (p.allied ? '#4CAF50' : '#C0392B');
  const culture = p.culture ? (CULTURE_ACCENT[p.culture] || '#888') : '#888';
  ctx.save();
  if (walls >= 1) {
    ctx.strokeStyle = '#9A7A50';
    ctx.lineWidth = walls >= 2 ? 2 : 1;
    ctx.beginPath();
    const r = 7 + walls;
    for (let i = 0; i < 6; i++) {
      const a = Math.PI / 3 * i;
      const x = cx + r * Math.cos(a), y = cy + r * Math.sin(a);
      i === 0 ? ctx.moveTo(x, y) : ctx.lineTo(x, y);
    }
    ctx.closePath();
    ctx.stroke();
  }
  const bw = 6 + walls * 1, bh = 5 + walls;
  ctx.fillStyle = '#D4B890';
  ctx.strokeStyle = '#7A5030';
  ctx.lineWidth = 0.8;
  ctx.fillRect(cx - bw/2, cy - bh/2, bw, bh);
  ctx.strokeRect(cx - bw/2, cy - bh/2, bw, bh);
  ctx.fillStyle = '#8A6040';
  ctx.fillRect(cx - 1.5, cy, 3, bh/2);
  ctx.fillStyle = culture;
  ctx.fillRect(cx - bw/2 + 1, cy - bh/2 + 1, bw - 2, 1.5);
  ctx.strokeStyle = accent;
  ctx.lineWidth = 0.6;
  ctx.beginPath();
  ctx.moveTo(cx, cy - bh/2 - 5);
  ctx.lineTo(cx, cy - bh/2 - 1);
  ctx.stroke();
  ctx.fillStyle = accent;
  ctx.beginPath();
  ctx.moveTo(cx, cy - bh/2 - 5);
  ctx.lineTo(cx + 4, cy - bh/2 - 3);
  ctx.lineTo(cx, cy - bh/2 - 1);
  ctx.closePath();
  ctx.fill();
  // Garrison dot (own cities only) — visible at zoom >= 0.45
  if (p.own && p.army_total > 0 && State.camera.zoom >= GARRISON_DOT_ZOOM) {
    ctx.fillStyle = '#8B1A1A';
    ctx.strokeStyle = '#3A0A0A';
    ctx.lineWidth = 0.5;
    const gx = cx + bw/2 + 1, gy = cy - 1;
    ctx.fillRect(gx, gy - 2, 3, 3);
    ctx.strokeRect(gx, gy - 2, 3, 3);
    ctx.fillStyle = '#E8B0B0';
    ctx.fillRect(gx + 1, gy - 1, 1, 1);
  }
  ctx.restore();
}

// ── Province name label (two-pass for legibility) ────────────────────────
function drawLabel(ctx, cx, cy, text, own) {
  ctx.save();
  ctx.font = own ? 'bold 7px monospace' : '6px monospace';
  ctx.textAlign = 'center';
  ctx.textBaseline = 'top';
  ctx.strokeStyle = '#000000aa';
  ctx.lineWidth = 2;
  ctx.strokeText(text, cx, cy + 10);
  ctx.fillStyle = own ? '#F9E79F' : '#E8D0A8';
  ctx.fillText(text, cx, cy + 10);
  ctx.restore();
}

// ── Activity overlay badge — build/train/idle indicator ──────────────────
function drawActivityBadge(ctx, cx, cy, p) {
  ctx.save();
  const bx = cx - 7, by = cy - 13;
  if (p.build_active) {
    // Hammer head (orange)
    ctx.fillStyle = '#D4780A';
    ctx.fillRect(bx, by, 5, 3);
    ctx.fillStyle = '#7A5030';
    ctx.fillRect(bx + 2, by + 2, 1, 3);
  } else if (p.train_active) {
    // Sword point (grey-blue)
    ctx.fillStyle = '#A8B8C8';
    ctx.beginPath();
    ctx.moveTo(bx + 2, by);
    ctx.lineTo(bx + 4, by + 5);
    ctx.lineTo(bx + 2, by + 4);
    ctx.lineTo(bx, by + 5);
    ctx.closePath();
    ctx.fill();
    ctx.fillStyle = '#7A5030';
    ctx.fillRect(bx + 1, by + 4, 2, 2);
  } else {
    // Idle — dim grey dot
    ctx.fillStyle = '#606060';
    ctx.beginPath();
    ctx.arc(bx + 2, by + 2, 2, 0, Math.PI * 2);
    ctx.fill();
  }
  ctx.restore();
}

// ── Dirt road between two province centres ────────────────────────────────
function drawRoad(ctx, ax, ay, bx, by) {
  ctx.save();
  ctx.strokeStyle = '#9A7850';
  ctx.lineWidth = 1.2;
  ctx.setLineDash([3, 4]);
  ctx.beginPath();
  ctx.moveTo(ax, ay);
  ctx.lineTo(bx, by);
  ctx.stroke();
  ctx.setLineDash([]);
  ctx.restore();
}

// ── Trade caravan figure — brown cloak, sack on back ─────────────────────
function drawCaravan(ctx, x, y, walkPhase) {
  ctx.save();
  const bob = walkPhase < 2 ? 0 : 1;
  ctx.fillStyle = '#7B4F28';
  ctx.fillRect(x-1, y-5+bob, 3, 5);
  ctx.fillStyle = '#5A3A18';
  ctx.beginPath();
  ctx.arc(x+0.5, y-7+bob, 2, 0, Math.PI*2);
  ctx.fill();
  ctx.fillStyle = '#9A7050';
  ctx.fillRect(x+1, y-4+bob, 2, 2);
  ctx.restore();
}

// ── Messenger figure — olive cloak, scroll in hand. A hemerodromos (order
// runner, kind='order') wears a crimson cloak so it reads as command, not
// diplomacy; own couriers additionally carry a small gold pennant so it is
// visible WHOSE runner it is (temenos_orderlopare_plan.md Fas 5).
function drawMessenger(ctx, x, y, walkPhase, isOrder, isOwn, delivering) {
  ctx.save();
  // Delivering: the runner has reached the unit and stopped to hand over the
  // scroll — freeze the gait so it doesn't jog in place (Timothy 2026-07-17)
  // during the worker-poll window before the order applies server-side.
  const bob = (delivering || walkPhase < 2) ? 0 : 1;
  ctx.fillStyle = isOrder ? '#A03A2A' : '#6B8B4A';
  ctx.fillRect(x-1, y-5+bob, 3, 5);
  ctx.fillStyle = isOrder ? '#6E2418' : '#3A5A28';
  ctx.beginPath();
  ctx.arc(x+0.5, y-7+bob, 2, 0, Math.PI*2);
  ctx.fill();
  ctx.fillStyle = '#F2E8C0';
  // Scroll: held forward as a handover gesture while delivering, else at the side.
  if (delivering) ctx.fillRect(x+2, y-5, 3, 2);
  else            ctx.fillRect(x+1, y-6+bob, 1, 3);
  if (isOwn) {
    ctx.fillStyle = '#D8B84A';
    ctx.fillRect(x-2, y-10+bob, 1, 3);
    ctx.fillRect(x-1, y-10+bob, 2, 1);
  }
  if (delivering) {
    // Faint gold pulse — a deliberate "handing over the order" beat.
    ctx.globalAlpha = 0.3 + 0.25 * Math.sin(State.animFrame * 0.12);
    ctx.strokeStyle = '#D8B84A';
    ctx.lineWidth = 0.6;
    ctx.beginPath();
    ctx.arc(x+0.5, y-4, 5, 0, Math.PI*2);
    ctx.stroke();
  }
  ctx.restore();
}

// ── Walking settler figure (8×10 px, Settlers-style) ─────────────────────
function drawWalker(ctx, x, y, intent, walkPhase) {
  ctx.save();
  const bob = walkPhase < 2 ? 0 : 1;
  const bodyColor = {attack:'#922B21', reinforce:'#1A5276', support:'#145A32', scout:'#7D6608'}[intent] || '#555';
  ctx.fillStyle = bodyColor;
  ctx.fillRect(x-2, y-5+bob, 4, 5);
  ctx.fillStyle = '#F4D0A0';
  ctx.beginPath();
  ctx.arc(x, y-7+bob, 2.5, 0, Math.PI*2);
  ctx.fill();
  ctx.strokeStyle = '#3A2010';
  ctx.lineWidth = 0.8;
  ctx.beginPath();
  if (walkPhase < 2) {
    ctx.moveTo(x-2, y-3+bob); ctx.lineTo(x-4, y-1+bob);
    ctx.moveTo(x+2, y-3+bob); ctx.lineTo(x+4, y+0+bob);
    ctx.moveTo(x-1, y+0+bob); ctx.lineTo(x-2, y+3+bob);
    ctx.moveTo(x+1, y+0+bob); ctx.lineTo(x+2, y+2+bob);
  } else {
    ctx.moveTo(x-2, y-3+bob); ctx.lineTo(x-4, y+0+bob);
    ctx.moveTo(x+2, y-3+bob); ctx.lineTo(x+4, y-1+bob);
    ctx.moveTo(x-1, y+0+bob); ctx.lineTo(x-2, y+2+bob);
    ctx.moveTo(x+1, y+0+bob); ctx.lineTo(x+2, y+3+bob);
  }
  ctx.stroke();
  ctx.restore();
}

// ── Naval vessel sprite (12×8 px, pixel boat) ────────────────────────────
function drawShip(ctx, x, y, intent, walkPhase) {
  ctx.save();
  const bob = walkPhase < 2 ? 0 : 1;
  // Sail color by intent
  const sailColor = {attack:'#C0392B', reinforce:'#1A5276', support:'#145A32', explore:'#0E7490'}[intent] || '#8B7355';
  // Hull — dark wood
  ctx.fillStyle = '#5C3A1E';
  ctx.fillRect(x - 5, y + 1 + bob, 10, 3);
  // Prow point (right)
  ctx.beginPath();
  ctx.moveTo(x + 5, y + 1 + bob);
  ctx.lineTo(x + 7, y + 2 + bob);
  ctx.lineTo(x + 5, y + 4 + bob);
  ctx.closePath();
  ctx.fill();
  // Mast
  ctx.fillStyle = '#7A5C30';
  ctx.fillRect(x - 0.5, y - 5 + bob, 1, 6);
  // Sail
  ctx.fillStyle = sailColor;
  ctx.beginPath();
  ctx.moveTo(x, y - 5 + bob);
  ctx.lineTo(x + 4, y - 2 + bob);
  ctx.lineTo(x, y + 1 + bob);
  ctx.closePath();
  ctx.fill();
  // Oar ripple (animated)
  ctx.strokeStyle = '#7AB8C8';
  ctx.lineWidth = 0.6;
  ctx.beginPath();
  const ripple = walkPhase < 2 ? 1 : 0;
  ctx.moveTo(x - 3, y + 4 + bob + ripple);
  ctx.lineTo(x + 3, y + 4 + bob);
  ctx.stroke();
  ctx.restore();
}
// ── Rural projections (Fas A2, megaron_lokal_varld.md) ───────────────────
// A rural sprite is NOT a new building — it is a cartographic projection of an
// existing city building onto a compatible catchment hex, placed server-side
// (rural-projections endpoint). Static (no gait): these are places, not movers.
// 1px charcoal outline, saturated foreground, no AA — temenos_designprinciper.
const RURAL_OUTLINE = '#33291E';

function drawFarm(ctx, x, y) {
  ctx.fillStyle = '#C9A227';         // ripe wheat
  ctx.strokeStyle = RURAL_OUTLINE; ctx.lineWidth = 0.7;
  ctx.fillRect(x - 4, y - 1, 8, 4);  // tilled plot
  ctx.strokeRect(x - 4, y - 1, 8, 4);
  ctx.strokeStyle = '#8A6A12';       // furrows
  ctx.beginPath();
  ctx.moveTo(x - 2, y - 1); ctx.lineTo(x - 2, y + 3);
  ctx.moveTo(x, y - 1);     ctx.lineTo(x, y + 3);
  ctx.moveTo(x + 2, y - 1); ctx.lineTo(x + 2, y + 3);
  ctx.stroke();
}

function drawMine(ctx, x, y) {
  ctx.fillStyle = '#8A8078';         // grey spoil heap
  ctx.strokeStyle = RURAL_OUTLINE; ctx.lineWidth = 0.7;
  ctx.beginPath();
  ctx.moveTo(x - 4, y + 3);
  ctx.lineTo(x, y - 3);
  ctx.lineTo(x + 4, y + 3);
  ctx.closePath();
  ctx.fill(); ctx.stroke();
  ctx.fillStyle = '#241C16';         // adit mouth
  ctx.fillRect(x - 1, y + 1, 2, 2);
}

function drawLumbermill(ctx, x, y) {
  ctx.strokeStyle = RURAL_OUTLINE; ctx.lineWidth = 0.7;
  ctx.fillStyle = '#7B4F28';         // stacked logs (end-on)
  for (const [dx, dy] of [[-3, 1], [0, 1], [3, 1], [-1.5, -2], [1.5, -2]]) {
    ctx.beginPath();
    ctx.arc(x + dx, y + dy, 1.6, 0, Math.PI * 2);
    ctx.fill(); ctx.stroke();
  }
  ctx.fillStyle = '#C8A464';         // core rings
  for (const [dx, dy] of [[-3, 1], [0, 1], [3, 1], [-1.5, -2], [1.5, -2]]) {
    ctx.fillRect(x + dx - 0.5, y + dy - 0.5, 1, 1);
  }
}

const RURAL_SPRITES = { farm: drawFarm, mine: drawMine, lumbermill: drawLumbermill };

// ── Main renderer ─────────────────────────────────────────────────────────

export function toggleActivityOverlay() {
  State.activityOverlay = !State.activityOverlay;
  document.getElementById('activity-btn').classList.toggle('active', State.activityOverlay);
  State.dirty = true;
}

export const canvas = document.getElementById('hex-canvas');
const tooltip = document.getElementById('tile-tooltip');
const container = document.getElementById('map-root');

function resizeCanvas() {
  canvas.width  = container.clientWidth;
  canvas.height = container.clientHeight;
}
resizeCanvas();
window.addEventListener('resize', resizeCanvas);

const ctx = canvas.getContext('2d');
ctx.imageSmoothingEnabled = false;

// Exported for the offline visual harness (web/static/showcase-forest.html),
// which stubs requestAnimationFrame and drives one frozen frame at a time so
// terrain screenshots are pixel-deterministic. Game code still enters the loop
// through initMap() only.
export function render() {
  State.animFrame++;
  const seaTick = State.animFrame >> 5;
  const seaChanged = seaTick !== State.lastSeaTick;
  if (seaChanged) State.lastSeaTick = seaTick;

  if (!State.dirty && !seaChanged && State.marchData.length === 0 && State.messengerData.length === 0 && State.tradeData.length === 0
      && !State.unitsData.some(u => u.status === 'marching')) {
    requestAnimationFrame(render);
    return;
  }
  State.dirty = false;

  ctx.clearRect(0, 0, canvas.width, canvas.height);
  ctx.save();
  ctx.translate(State.camera.x, State.camera.y);
  ctx.scale(State.camera.zoom * SCALE, State.camera.zoom * SCALE);

  // 1. Terrain tiles with animated detail + deposit icons
  for (const t of State.tileData) {
    const {x,y} = hexPx(t.q, t.r);
    const pts = hexPts(x, y);
    const base = TERRAIN_BASE[t.terrain] || TERRAIN_BASE.fog;
    const seed = (t.q*137 + t.r*31) & 0xff;
    fillHex(ctx, pts, base.c0, base.c1, seed);
    if (t.terrain !== 'fog') drawDetail(ctx, x, y, t.terrain, seed, State.animFrame, t.q, t.r);
    if (t.terrain !== 'fog' && State.camera.zoom >= ROAD_DEPOSIT_ZOOM) drawDepositIcons(ctx, x, y, t);
  }

  // 2. Roads between adjacent own/allied provinces
  if (State.camera.zoom >= ROAD_DEPOSIT_ZOOM) {
    const owned = State.provinceData.filter(p => p.own || p.allied);
    for (let i = 0; i < owned.length; i++) {
      for (let j = i+1; j < owned.length; j++) {
        const a = owned[i], b = owned[j];
        const dq = a.q-b.q, dr = a.r-b.r;
        if ((Math.abs(dq) + Math.abs(dq+dr) + Math.abs(dr)) / 2 <= 1) {
          const fa = hexPx(a.q, a.r), fb = hexPx(b.q, b.r);
          drawRoad(ctx, fa.x, fa.y, fb.x, fb.y);
        }
      }
    }
  }

  // 2.5 Catchment zone — subtle gold tint on the 7 catchment tiles of own cities
  // (the city's own hex + the 6 adjacent). [0,0] = the settlement's own hex.
  if (State.camera.zoom >= LOCAL_ZOOM) {
    for (const p of State.provinceData) {
      if (!p.own || p.is_outpost) continue;
      for (const [dq, dr] of [[0, 0], ...HEX_DIRS]) {
        const nq = p.q + dq, nr = p.r + dr;
        if (dq !== 0 || dr !== 0) {
          if (State.provinceData.find(x => x.q === nq && x.r === nr)) continue;
        }
        const t = State.tileData.find(t => t.q === nq && t.r === nr);
        if (!t || t.terrain === 'fog') continue;
        const {x, y} = hexPx(nq, nr);
        const pts = hexPts(x, y);
        ctx.save();
        hexPath(ctx, pts);
        ctx.globalAlpha = 0.11;
        ctx.fillStyle = '#D4AC0D';
        ctx.fill();
        ctx.globalAlpha = 0.25;
        ctx.strokeStyle = '#D4AC0D';
        ctx.lineWidth = 0.7;
        ctx.stroke();
        ctx.restore();
      }
    }
  }

  // 2.6 Rural projections — own city buildings drawn on their catchment hexes.
  // LOCAL register only (LOCAL_ZOOM): "why does THIS place work?" is a local
  // question; the strategic overview tones them down. Server places them within
  // the owner's FOW; skip any hex that has gone to fog since the last fetch.
  if (State.camera.zoom >= LOCAL_ZOOM) {
    for (const rp of State.ruralData) {
      const sprite = RURAL_SPRITES[rp.building_type];
      if (!sprite) continue;
      const t = State.tileData.find(t => t.q === rp.q && t.r === rp.r);
      if (!t || t.terrain === 'fog') continue;
      const { x, y } = hexPx(rp.q, rp.r);
      ctx.save();
      sprite(ctx, x, y);
      ctx.restore();
    }
  }

  // 3. Highlight selected hex
  if (State.selectedHex) {
    const {x,y} = hexPx(State.selectedHex.q, State.selectedHex.r);
    const pts = hexPts(x, y);
    ctx.save();
    ctx.strokeStyle = '#F9E79F'; ctx.lineWidth = 2.5;
    ctx.beginPath(); ctx.moveTo(pts[0][0], pts[0][1]);
    for (let i=1;i<6;i++) ctx.lineTo(pts[i][0], pts[i][1]);
    ctx.closePath(); ctx.stroke(); ctx.restore();
  }

  // 3.5 FOV preview band — hexes that would become live-visible from the hovered
  // march affordance's target, per server/internal/province/hex.go LiveRadius.
  // Fog tiles use the conservative base radius (no mountain bonus) since we
  // can't know their real terrain without leaking it through the band shape.
  if (State.fovPreview) {
    const { q: fq, r: fr, kind } = State.fovPreview;
    for (const t of State.tileData) {
      const known = t.terrain !== 'fog';
      const radius = known ? liveRadius(kind, t.terrain) : LIVE_RADIUS_BASE[kind];
      if (hexDist(fq, fr, t.q, t.r) > radius) continue;
      const {x, y} = hexPx(t.q, t.r);
      const pts = hexPts(x, y);
      ctx.save();
      hexPath(ctx, pts);
      ctx.globalAlpha = 0.16;
      ctx.fillStyle = '#C87F2A';
      ctx.fill();
      ctx.globalAlpha = 0.45;
      ctx.strokeStyle = '#C87F2A';
      ctx.lineWidth = 1;
      ctx.stroke();
      ctx.restore();
    }
  }

  // 3.6 Catchment preview — the 7 catchment hexes (target + 6 neighbours) of a
  // hovered/armed colonize affordance. This is deliberately NOT the FOV band
  // above: colonize's true footprint is the fixed 7-hex catchment (same shape
  // as 2.5's own-city tint), not the per-tile live-visibility radius, which
  // reads as a much bigger and irregular area than what the colony will
  // actually work (Bugg 3).
  if (State.catchmentPreview) {
    const { q: cq, r: cr } = State.catchmentPreview;
    for (const [dq, dr] of [[0, 0], ...HEX_DIRS]) {
      const nq = cq + dq, nr = cr + dr;
      const t = State.tileData.find(t => t.q === nq && t.r === nr);
      if (!t || t.terrain === 'fog') continue;
      const {x, y} = hexPx(nq, nr);
      const pts = hexPts(x, y);
      ctx.save();
      hexPath(ctx, pts);
      ctx.globalAlpha = 0.22;
      ctx.fillStyle = '#F5B041';
      ctx.fill();
      ctx.globalAlpha = 0.5;
      ctx.strokeStyle = '#F5B041';
      ctx.lineWidth = 1;
      ctx.stroke();
      ctx.restore();
    }
  }

  // 3b. Incoming attack glow — pulsing red on target hex of any visible attack march
  const attackTargets = new Set(
    State.marchData.filter(m => m.intent === 'attack').map(m => `${m.target_q},${m.target_r}`)
  );
  if (attackTargets.size > 0) {
    const pulse = 0.25 + 0.15 * Math.sin(State.animFrame * 0.08);
    for (const p of State.provinceData) {
      if (!attackTargets.has(`${p.q},${p.r}`)) continue;
      const {x, y} = hexPx(p.q, p.r);
      const pts = hexPts(x, y);
      ctx.save();
      hexPath(ctx, pts);
      ctx.globalAlpha = pulse;
      ctx.fillStyle = '#C0392B';
      ctx.fill();
      ctx.globalAlpha = pulse + 0.2;
      ctx.strokeStyle = '#E74C3C';
      ctx.lineWidth = 2;
      ctx.stroke();
      ctx.restore();
    }
  }

  // 4. Province buildings + flags
  for (const p of State.provinceData) {
    const {x,y} = hexPx(p.q, p.r);
    drawProvince(ctx, x, y, p);
    if (State.camera.zoom >= LOCAL_ZOOM) drawLabel(ctx, x, y, p.name, p.own);
  }

  // 4b. Activity overlay badges (own non-outpost cities, zoom >= ACTIVITY_BADGE_ZOOM)
  if (State.activityOverlay && State.camera.zoom >= ACTIVITY_BADGE_ZOOM) {
    for (const p of State.provinceData) {
      if (p.is_outpost || !p.own) continue;
      const {x, y} = hexPx(p.q, p.r);
      drawActivityBadge(ctx, x, y, p);
    }
  }

  // 5. Animated walkers for marching armies
  const walkPhase = Math.floor(State.animFrame / 8) % 4;
  for (const m of State.marchData) {
    const now = serverNow();
    const departs = new Date(m.departs_at).getTime();
    const arrives  = new Date(m.arrives_at).getTime();
    const progress = Math.min(1, Math.max(0, (now - departs) / (arrives - departs)));
    const pos = (m.path && m.path.length > 1)
      ? pathPx(m.path, progress)
      : hexPathPx(m.origin_q, m.origin_r, m.target_q, m.target_r, progress);
    if (isTileVisible(pos.q, pos.r)) {
      if (m.is_naval) {
        drawShip(ctx, Math.round(pos.x), Math.round(pos.y), m.intent, walkPhase);
      } else {
        drawWalker(ctx, Math.round(pos.x), Math.round(pos.y), m.intent, walkPhase);
      }
    }
  }

  // 5b. Per-unit armies & fleets (per-unit march model). Marching units animate
  // along their route (interpolated like marches); positioned units (on the map
  // without a settlement) sit where they stand. Garrison/forming/embarked units
  // live at a settlement and are already represented by its province, so they
  // are not drawn here.
  for (const u of State.unitsData) {
    const naval = u.category === 'naval';
    if (u.status === 'marching' && u.departs_at && u.arrives_at && u.q != null && u.target_q != null) {
      const now = serverNow();
      const departs = new Date(u.departs_at).getTime();
      const arrives = new Date(u.arrives_at).getTime();
      const progress = Math.min(1, Math.max(0, (now - departs) / (arrives - departs)));
      const pos = (u.path && u.path.length > 1)
        ? pathPx(u.path, progress)
        : hexPathPx(u.q, u.r, u.target_q, u.target_r, progress);
      if (isTileVisible(pos.q, pos.r)) {
        // explore/explore_return share the cyan "explore" sail; other legs use
        // the neutral default colour (intent is resolved server-side on arrival).
        const intent = (u.march_intent === 'explore' || u.march_intent === 'explore_return') ? 'explore' : (u.march_intent || '');
        if (naval) drawShip(ctx, Math.round(pos.x), Math.round(pos.y), intent, walkPhase);
        else       drawWalker(ctx, Math.round(pos.x), Math.round(pos.y), intent, walkPhase);
      }
    } else if (u.status === 'positioned' && u.q != null && isTileVisible(u.q, u.r)) {
      const {x, y} = hexPx(u.q, u.r);
      if (naval) drawShip(ctx, Math.round(x), Math.round(y), '', walkPhase);
      else       drawWalker(ctx, Math.round(x), Math.round(y), '', walkPhase);
    }
  }

  // 6. Animated messengers — OWN couriers are drawn along their whole route,
  // dimmed over fog (the player's own hemerodromos is information they already
  // possess — temenos_orderlopare_plan.md Fas 5); foreign messengers only
  // inside live-visible tiles, as before.
  for (const m of State.messengerData) {
    const now = serverNow();
    const sent   = new Date(m.sent_at).getTime();
    const arrives = new Date(m.arrives_at).getTime();
    const progress = Math.min(1, Math.max(0, (now - sent) / (arrives - sent)));
    const pos = hexPathPx(m.origin_q, m.origin_r, m.dest_q, m.dest_r, progress);
    const visible = isTileVisible(pos.q, pos.r);
    // An order runner at journey's end hasn't "failed to move" — it has arrived
    // and is delivering; the unit starts marching once the worker applies the
    // order (a poll away). Draw a settled handover instead of a jog-in-place.
    const delivering = m.kind === 'order' && progress >= 1;
    if (m.own) {
      ctx.save();
      if (!visible) ctx.globalAlpha = 0.45;
      drawMessenger(ctx, Math.round(pos.x), Math.round(pos.y), walkPhase, m.kind === 'order', true, delivering);
      ctx.restore();
    } else if (visible) {
      drawMessenger(ctx, Math.round(pos.x), Math.round(pos.y), walkPhase, m.kind === 'order', false, delivering);
    }
  }

  // 7. Animated trade caravans
  for (const t of State.tradeData) {
    const now = serverNow();
    const departs = new Date(t.departs_at).getTime();
    const arrives = new Date(t.arrives_at).getTime();
    const progress = Math.min(1, Math.max(0, (now - departs) / (arrives - departs)));
    const pos = hexPathPx(t.origin_q, t.origin_r, t.dest_q, t.dest_r, progress);
    if (isTileVisible(pos.q, pos.r)) {
      drawCaravan(ctx, Math.round(pos.x), Math.round(pos.y), walkPhase);
    }
  }

  ctx.restore();
  requestAnimationFrame(render);
}

// ── Data loading ──────────────────────────────────────────────────────────
export async function loadMap() {
  const [tilesRes, provRes, marchRes, msgRes, tradeRes, unitsRes, ruralRes] = await Promise.all([
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/map`),
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces`),
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/marches`),
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/messengers`),
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/trades`),
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units`),
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/rural-projections`),
  ]);

  if (tilesRes.ok) {
    State.tileData = await tilesRes.json();
  }
  if (provRes.ok) {
    // Must land before centreCamera() below — homePosition() reads the
    // capital out of State.provinceData.
    State.provinceData = await provRes.json();
  }
  centreCamera();
  if (marchRes.ok) {
    State.marchData = await marchRes.json();
  }
  if (unitsRes.ok) {
    State.unitsData = (await unitsRes.json()).units || [];
  }
  if (msgRes.ok) {
    State.messengerData = await msgRes.json();
  }
  if (tradeRes.ok) {
    State.tradeData = await tradeRes.json();
  }
  if (ruralRes.ok) {
    State.ruralData = await ruralRes.json();
  }
  window.MusicPlayer.update();
}

// The player's home hex: capital settlement, or — during the founder phase,
// before any settlement exists — the Nomadic Host's position. Camera should
// always open on this, never on the mean of every visible tile (that reads
// as a meaningless empty mid-ocean centre on a large/sparse map).
function homePosition() {
  const capital = ownCapital();
  if (capital) return { q: capital.q, r: capital.r };
  if (State.founderPhase && State.founderPhase.q != null) {
    return { q: State.founderPhase.q, r: State.founderPhase.r };
  }
  return null;
}

function centreCamera() {
  const home = homePosition();
  if (home) {
    const { x, y } = hexPx(home.q, home.r);
    State.camera.x = canvas.width/2  - x*SCALE;
    State.camera.y = canvas.height/2 - y*SCALE;
    return;
  }
  // Fallback for a transitional state with neither a capital nor an active
  // founder phase yet (e.g. mid-succession) — centre on visible tiles rather
  // than leaving the camera wherever it last was.
  const visible = State.tileData.filter(t => t.terrain !== 'fog');
  if (!visible.length) return;
  const sumX = visible.reduce((s,t) => s + hexPx(t.q,t.r).x, 0);
  const sumY = visible.reduce((s,t) => s + hexPx(t.q,t.r).y, 0);
  State.camera.x = canvas.width/2  - (sumX/visible.length)*SCALE;
  State.camera.y = canvas.height/2 - (sumY/visible.length)*SCALE;
}

// Reload provinces, marches, messengers and trades every 30s
// Refetch the fog-of-war map (State.tileData) WITHOUT recentring the State.camera. Fog
// changes — a scout/explore revealing new tiles, a unit's live vision moving —
// only land here, so this must run on the poll (and after unit arrivals);
// otherwise the canvas keeps the fog it had at page load and exploration looks
// like it did nothing.
export function refreshTiles() {
  fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/map`).then(r => r.ok && r.json().then(d => { State.tileData = d; State.dirty = true; }));
}

// ── Zoom helpers ──────────────────────────────────────────────────────────
// Single source of truth for the zoom clamp — was previously duplicated
// between zoom() and the wheel handler in initMap(), which is why raising
// the floor in only one of them didn't fix Timothy's "zooms out too far"
// report (2026-07-25). Bounds themselves are named calibration in config.js.
function clampZoom(z) {
  return Math.min(ZOOM_MAX, Math.max(ZOOM_MIN, z));
}

export function zoom(factor) {
  const cx = canvas.width/2, cy = canvas.height/2;
  State.camera.x = cx + (State.camera.x - cx) * factor;
  State.camera.y = cy + (State.camera.y - cy) * factor;
  State.camera.zoom = clampZoom(State.camera.zoom * factor);
  State.dirty = true;
}
export function resetView() {
  State.camera.zoom = 1;
  centreCamera();
  State.dirty = true;
}

// ── Inspect panel ─────────────────────────────────────────────────────────
// Canonical keys as of migration 028 (terrain enum) — mountain/forest/coast/sea
// were stale pre-rename leftovers that never matched a real tile.terrain value.
// "coast" has no terrain-enum replacement: migration 050 replaced the coast_beach
// terrain with a `coastal` boolean flag on land tiles, so fish is folded into
// producesText() below via that flag instead of a dict key.
const TERRAIN_GOODS = {
  plains:             'grain, horses',
  river_valley:       'grain ×3 (very fertile)',
  river_delta:        'grain ×4 (richest — exposed coast)',
  hills:              'copper (if deposit), wine, oil',
  mountain_limestone: 'stone, tin (if deposit)',
  mountain_red:       'stone, tin (if deposit)',
  forest_olive_grove: 'cedar (if deposit)',
  coastal_sea:        '—',
  deep_sea:           '—',
};

function producesText(tile) {
  const base = TERRAIN_GOODS[tile.terrain] || '—';
  if (!tile.coastal) return base;
  return base === '—' ? 'fish' : base + ', fish';
}

const UNIT_LABELS_SHORT = {
  spearman:'Spearmen', elite_infantry:'Elite Infantry', war_chariot:'War Chariot',
  ship:'Galley', galley:'Galley', war_galley:'War Galley', merchantman:'Emporos',
  nomadic_host:'Nomadic Host',
};

function unitListHTML(units) {
  if (!units.length) return '';
  const rows = units.map(u => {
    const lbl = UNIT_LABELS_SHORT[u.type] || u.type;
    return '<div style="display:flex;justify-content:space-between;align-items:center;gap:.4rem;padding:.2rem 0">'
      + '<span>' + lbl + ' <span style="color:var(--text-dim)">(' + u.status + ')</span></span>'
      + '<button data-unit-id="' + u.id + '" style="padding:.15rem .35rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.65rem;cursor:pointer">Visa →</button>'
      + '</div>';
  }).join('');
  return '<div style="margin-bottom:.5rem"><div class="ir-label" style="margin-bottom:.2rem">Units here</div>' + rows + '</div>';
}

function bindUnitButtons(foot) {
  foot.querySelectorAll('[data-unit-id]').forEach(b => {
    b.addEventListener('click', () => window.warFocusUnit(b.dataset.unitId));
  });
}

const MARCH_BTN_STYLE = 'display:block;width:100%;text-align:center;padding:.3rem;background:var(--bg-raised);border:1px solid var(--border);color:var(--text);font-size:.8rem;cursor:pointer;margin-top:.3rem;';

// Wire a march-affordance button: click opens march-ctx pre-filled with dest,
// hover sets the FOV preview band (map.js render §3.5) for `kind` ('land'|'ship').
function bindMarchButton(btn, dest, kind) {
  if (!btn) return;
  btn.addEventListener('click', e => window.openMarchCtx(dest, e.clientX, e.clientY));
  btn.addEventListener('mouseenter', () => { State.fovPreview = { q: dest.q, r: dest.r, kind }; State.dirty = true; });
  btn.addEventListener('mouseleave', () => { State.fovPreview = null; State.dirty = true; });
}

// Wire the colonize affordance button: same click behaviour as a march button
// (opens march-ctx pre-filled with dest), but hover previews the 7-hex
// catchment (render §3.6) instead of the FOV band — colonize's footprint is
// the fixed catchment, not live-visibility (Bugg 3).
function bindCatchmentPreviewButton(btn, dest) {
  if (!btn) return;
  btn.addEventListener('click', e => window.openMarchCtx(dest, e.clientX, e.clientY));
  btn.addEventListener('mouseenter', () => { State.catchmentPreview = { q: dest.q, r: dest.r }; State.dirty = true; });
  btn.addEventListener('mouseleave', () => { State.catchmentPreview = null; State.dirty = true; });
}

// Human names for the terrain enum — the raw keys leaked into panels and menu
// headers as "River_valley"/"Empty hex", machine-speak in the most prominent slot.
const TERRAIN_LABELS = {
  plains: 'Plains', hills: 'Hills', forest_olive_grove: 'Olive Grove',
  scrub_maquis: 'Maquis Scrub', semi_desert: 'Semi-Desert',
  river_valley: 'River Valley', river_delta: 'River Delta',
  coastal_sea: 'Coastal Sea', deep_sea: 'Deep Sea',
  mountain_limestone: 'Limestone Mountains', mountain_red: 'Red Mountains',
};

function terrainLabel(t) {
  return TERRAIN_LABELS[t] || (t.charAt(0).toUpperCase() + t.slice(1).replaceAll('_', ' '));
}

function fillTerrainFields(tile) {
  document.getElementById('ip-terrain').textContent = terrainLabel(tile.terrain);
  const deps = [tile.copper_deposit ? 'Copper' : null, tile.tin_deposit ? 'Tin' : null,
                tile.silver_deposit ? 'Silver' : null, tile.cedar_deposit ? 'Cedar' : null].filter(Boolean);
  const depRow = document.getElementById('ip-deposits-row');
  if (deps.length > 0) {
    document.getElementById('ip-deposits').textContent = deps.join(' · ');
    depRow.style.display = '';
  } else {
    depRow.style.display = 'none';
  }
  document.getElementById('ip-produces').textContent = producesText(tile);
}

function setCityFieldsVisible(visible) {
  ['ip-culture-row', 'ip-owner-row', 'ip-walls-row', 'ip-army-row'].forEach(id => {
    document.getElementById(id).style.display = visible ? '' : 'none';
  });
}

// Build the same dest object the march-ctx menu consumes, whether the caller is
// the left-click affordance panel or the right-click context menu. target is the
// province marker at (h.q,h.r), or null for an empty/sea/mountain hex.
function destFromHex(h, tile, target) {
  const isSea = tile.terrain === 'coastal_sea' || tile.terrain === 'deep_sea';
  if (target) {
    return { q: h.q, r: h.r, terrain: tile.terrain, isSea,
             name: target.name, isSettlement: true, allied: target.own ? true : !!target.allied };
  }
  return { q: h.q, r: h.r, terrain: tile.terrain, isSea,
           name: `${terrainLabel(tile.terrain)} (${h.q},${h.r})`,
           isSettlement: false, allied: false };
}

// Fog: nothing known yet — no terrain/deposits/produces, no affordances.
function openFogPanel(h) {
  document.getElementById('ip-name').textContent = 'Outforskat land';
  setCityFieldsVisible(false);
  document.getElementById('ip-terrain').textContent = `(${h.q},${h.r})`;
  document.getElementById('ip-deposits-row').style.display = 'none';
  document.getElementById('ip-produces').textContent = '—';
  document.getElementById('ip-foot').innerHTML = '<p class="empty-state">Segla eller marschera i närheten för att avslöja.</p>';
  document.getElementById('inspect-panel').style.display = 'flex';
}

// Foreign/allied settlement — today's openInspect content (Wanax/culture/walls/DP),
// plus the units-here list and a Marschera button. Own settlements never reach
// this function — they bypass the panel for the city drawer (see openHexPanel).
function openCityPanel(h, tile, marker, units) {
  document.getElementById('ip-name').textContent    = marker.name;
  setCityFieldsVisible(true);
  document.getElementById('ip-culture').textContent = marker.culture;
  fillTerrainFields(tile);

  let ownerText = marker.owner || '(unoccupied)';
  if (marker.allied) ownerText += ' (allied)';
  document.getElementById('ip-owner').textContent = ownerText;
  document.getElementById('ip-walls').textContent = '▓'.repeat(marker.walls) + '░'.repeat(Math.max(0,3-marker.walls));
  document.getElementById('ip-army').textContent = '…';
  fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${marker.id}/army`).then(r => {
    if (!r.ok) { document.getElementById('ip-army').textContent = '—'; return; }
    r.json().then(a => {
      const dp = (a.Spearman||0)*1 + (a.EliteInfantry||0)*3 + (a.WarChariot||0)*4;
      document.getElementById('ip-army').textContent = dp > 0 ? `${dp} DP` : '—';
    });
  }).catch(() => { document.getElementById('ip-army').textContent = '—'; });

  const foot = document.getElementById('ip-foot');
  let footHtml = unitListHTML(units);
  // A settlement OR a wandering host can send — the host's first contact with a
  // met city is one of its designed uses (mig 087; sendMessengerFromInspect
  // picks the endpoint).
  if ((State.MY_SETTLEMENT_ID || State.founderPhase) && marker.settlement_id) {
    footHtml += `
      <textarea id="ip-msg-text" class="msg-textarea" placeholder="Write message…" maxlength="1000" rows="3"></textarea>
      <div class="msg-foot">
        <button class="msg-send" onclick="sendMessengerFromInspect('${marker.settlement_id}')">Send Messenger</button>
        <span class="msg-err" id="ip-msg-err"></span>
      </div>`;
  }
  footHtml += '<button id="ip-march-btn" style="' + MARCH_BTN_STYLE + '">Marschera hit →</button>';
  foot.innerHTML = footHtml;
  bindUnitButtons(foot);
  bindMarchButton(document.getElementById('ip-march-btn'), destFromHex(h, tile, marker), 'land');

  document.getElementById('inspect-panel').style.display = 'flex';
}

// Mountain / sea / empty land — no province here. Mountains explain their own
// absence of affordances; sea gets galleys; empty land gets march + colonize.
function openTerrainPanel(h, tile, isMountain, isSea, units) {
  document.getElementById('ip-name').textContent =
    isSea ? `Sea (${h.q},${h.r})` : (isMountain ? `Mountains (${h.q},${h.r})` : `Empty hex (${h.q},${h.r})`);
  setCityFieldsVisible(false);
  fillTerrainFields(tile);

  const foot = document.getElementById('ip-foot');
  let footHtml = unitListHTML(units);

  if (isMountain) {
    footHtml += '<p class="empty-state">Ogenomträngligt — arméer kan inte gå här.</p>';
    foot.innerHTML = footHtml;
    bindUnitButtons(foot);
    document.getElementById('inspect-panel').style.display = 'flex';
    return;
  }

  const dest = destFromHex(h, tile, null);
  if (isSea) {
    footHtml += '<button id="ip-march-btn" style="' + MARCH_BTN_STYLE + '">Skicka galärer →</button>';
  } else if (State.founderPhase) {
    // Founder phase: empty land is a POSSIBLE HOME, never a colony ("aldrig
    // Kolonisera"). March the host here; the founding forecast shows what the
    // ground would feed. Settle itself lives on the Host panel — the server
    // founds where the host stands, nowhere else.
    footHtml += '<button id="ip-march-btn" style="' + MARCH_BTN_STYLE + '">Marschera hit →</button>'
             +  '<div id="ip-found-preview" style="font-size:.73rem;margin-top:.4rem">Hämtar grundningsprognos…</div>';
  } else {
    footHtml += '<button id="ip-march-btn" style="' + MARCH_BTN_STYLE + '">Marschera hit →</button>'
             +  '<button id="ip-colonize-btn" style="' + MARCH_BTN_STYLE + '">Kolonisera →</button>';
  }
  foot.innerHTML = footHtml;
  bindUnitButtons(foot);
  // Founder phase: marching the host to an empty hex FOUNDS there — so hovering
  // the march button previews the 7-hex catchment (bug 3 shape), not the FOV
  // band. Every other case keeps the plain march affordance.
  if (!isSea && State.founderPhase) {
    bindCatchmentPreviewButton(document.getElementById('ip-march-btn'), dest);
  } else {
    bindMarchButton(document.getElementById('ip-march-btn'), dest, isSea ? 'ship' : 'land');
  }

  if (!isSea && State.founderPhase) {
    const fp = State.founderPhase;
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/colonize-preview?q=${h.q}&r=${h.r}`
        + `&pop=${fp.population}&seed=${Math.max(0, Math.round(fp.grain?.amount || 0))}`)
      .then(r => r.ok ? r.json() : null)
      .then(p => {
        const el = document.getElementById('ip-found-preview');
        if (el && p) el.innerHTML = window.renderColonizePreviewHTML(p);
      })
      .catch(() => {});
  }

  const colBtn = document.getElementById('ip-colonize-btn');
  if (colBtn) {
    bindCatchmentPreviewButton(colBtn, dest);
    // Same march-ctx as Marschera, just pre-check the colonize box on open —
    // no second order code path (plan §"Målbild").
    colBtn.addEventListener('click', () => {
      const chk = document.getElementById('mctx-colonize-chk');
      if (chk) { chk.checked = true; window.onColonizeToggle(); }
    });
  }

  document.getElementById('inspect-panel').style.display = 'flex';
}

// Rural projection object card (Fas A2). Identity (what + whose omland) + the
// terrain it sits on, and a single primary CTA back to the owning city — the
// projection is a representation, so its card leads to the real building
// (megaron_lokal_varld.md). Any unit standing on the hex stays reachable via
// the units list; marching here is a right-click order, not a panel button.
const RURAL_LABELS = { farm: 'Farm', mine: 'Mine', lumbermill: 'Lumbermill' };

function openRuralPanel(h, tile, rural, units) {
  document.getElementById('ip-name').textContent = RURAL_LABELS[rural.building_type] || rural.building_type;
  setCityFieldsVisible(false);
  fillTerrainFields(tile);

  const foot = document.getElementById('ip-foot');
  let footHtml = `<p class="empty-state">Del av ${rural.name}s omland — brukas härifrån.</p>`;
  footHtml += unitListHTML(units);
  footHtml += `<button id="ip-rural-city-btn" style="${MARCH_BTN_STYLE}">Öppna ${rural.name} →</button>`;
  foot.innerHTML = footHtml;
  bindUnitButtons(foot);
  document.getElementById('ip-rural-city-btn').addEventListener('click', () => {
    document.getElementById('inspect-panel').style.display = 'none';
    State.cityViewID = rural.province_id;
    window.openDrawer('city');
  });

  document.getElementById('inspect-panel').style.display = 'flex';
}

// ── Founder phase: the Host panel (temenos_nomadic_host_fas4_plan.md 4.3) ──
// The people-on-the-move's own surface: status from /founding/status, the
// founding forecast from /colonize-preview with ?pop=&seed= (the metropolis's
// 4 000 and the carried grain — same endpoint and renderer as colonization,
// never its own), and the irreversible Settle. Disappears entirely the moment
// founder_phase.active goes false.

// One store line: "X speldygn kvar (≈ Y verklig tid)" — both derived from
// ticks_left at render time (B2: never a stored wall clock).
function hostStoreLine(label, s, tickSeconds) {
  if (!s || s.ticks_left == null) return `${label}: räcker tills vidare`;
  const gameDays = (s.ticks_left / 24).toFixed(0);
  const realH = Math.round(s.ticks_left * tickSeconds / 3600);
  const real = realH >= 48 ? `≈ ${Math.round(realH / 24)} dygn` : `≈ ${realH} h`;
  return `${label}: ${gameDays} speldygn kvar (${real} verklig tid)`;
}

async function openHostPanel(h, tile) {
  // Refresh the store numbers on every open — they drain per tick.
  try {
    const r = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/founding/status`);
    if (r.ok) {
      const fp = await r.json();
      State.founderPhase = fp.active ? fp : null;
    }
  } catch (_) {}
  const fp = State.founderPhase;
  if (!fp) { openHexPanel(h); return; } // settled meanwhile — normal routing

  document.getElementById('ip-name').textContent = 'Nomadic Host';
  setCityFieldsVisible(false);
  fillTerrainFields(tile);

  const foot = document.getElementById('ip-foot');
  foot.innerHTML =
    `<div style="margin-bottom:.5rem;line-height:1.5">
       <div>${(fp.population || 0).toLocaleString('sv-SE')} folk · Kan inte strida · Syn: 1 hex</div>
       <div>${hostStoreLine('Grain', fp.grain, fp.tick_seconds)}</div>
       <div>${hostStoreLine('Silver för Spearmen', fp.silver, fp.tick_seconds)}</div>
       <div>${fp.spearmen_in_field || 0} Spearmen-kohort${fp.spearmen_in_field === 1 ? '' : 'er'} i fält</div>
       <div>Budbärare: fria att sända</div>
     </div>
     <div id="ip-found-preview" style="font-size:.73rem;border-top:1px solid var(--border);padding-top:.4rem;margin-bottom:.3rem">Hämtar grundningsprognos…</div>
     <button id="ip-settle-btn" style="${MARCH_BTN_STYLE}">⚒ Grunda huvudstaden här</button>
     <span class="msg-err" id="ip-settle-err"></span>`;
  document.getElementById('inspect-panel').style.display = 'flex';

  // Glow the 7 catchment hexes the host would found on, for as long as the Host
  // panel is open — the map then shows exactly the ground the forecast below
  // describes, so "granska catchmenten före grundning" (design 2026-07-15) is
  // visible, not just tabular. Cleared by openHexPanel on the next hex click.
  State.catchmentPreview = { q: h.q, r: h.r };
  State.dirty = true;

  // The forecast for the hex the host STANDS on — settle founds here, nowhere else.
  // starter_farm=1: a metropolis founding seeds a starter farm (createMetropolis),
  // unlike a plain colony — see founding-forecast-fix-plan.
  fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/colonize-preview?q=${h.q}&r=${h.r}`
      + `&pop=${fp.population}&seed=${Math.max(0, Math.round(fp.grain?.amount || 0))}&starter_farm=1`)
    .then(r => r.ok ? r.json() : null)
    .then(p => {
      const el = document.getElementById('ip-found-preview');
      if (el && p) el.innerHTML = window.renderColonizePreviewHTML(p);
    })
    .catch(() => {});

  document.getElementById('ip-settle-btn').addEventListener('click', async () => {
    if (!confirm('Grunda din huvudstad här? Hostet upplöses — för alltid.')) return;
    const errEl = document.getElementById('ip-settle-err');
    const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/founding/settle`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({}),
    });
    if (res.ok) {
      track('settle');
      // The world changed shape: new province, new eyes, host gone. Reload
      // outright — founding happens once per world, a full refresh is honest.
      location.reload();
    } else {
      const err = await res.json().catch(() => ({ error: 'Unknown error' }));
      errEl.style.color = 'var(--accent)';
      errEl.textContent = err.error || 'Error';
    }
  });
}

// Dispatcher for a left-click (non-drag) on any hex — the affordance matrix.
// Always selects the hex (highlight), then routes to the right view.
function openHexPanel(h) {
  State.selectedHex = { q: h.q, r: h.r };
  State.fovPreview = null;
  State.catchmentPreview = null;
  State.dirty = true;

  const tile = State.tileData.find(t => t.q === h.q && t.r === h.r);
  const prov = State.provinceData.find(p => p.q === h.q && p.r === h.r);

  if (!tile || tile.terrain === 'fog') { openFogPanel(h); return; }

  // Founder phase: the host's own hex opens the Host panel, never a unit view.
  if (State.founderPhase && (State.unitsData || []).some(u =>
      u.type === 'nomadic_host' && u.q === h.q && u.r === h.r)) {
    openHostPanel(h, tile);
    return;
  }

  if (prov && prov.own) {
    // Own settlement — no mid-panel, the city drawer IS the info (framgångskriterium 2).
    document.getElementById('inspect-panel').style.display = 'none';
    State.cityViewID = prov.id;
    window.openDrawer('city');
    return;
  }

  const units = (State.unitsData || []).filter(u =>
    u.q === h.q && u.r === h.r && (u.status === 'positioned' || u.status === 'marching'));

  if (prov) { openCityPanel(h, tile, prov, units); return; }

  // Rural projection hex — a catchment hex carrying one of the player's own city
  // buildings (Fas A2). The card names it, says whose omland it is, and its
  // primary CTA leads back to the owning city's building context (the doc's
  // rule: the projection is a representation, its card leads to the real
  // building). Marching still works via right-click, so no march button here.
  const rural = (State.ruralData || []).find(rp => rp.q === h.q && rp.r === h.r);
  if (rural) { openRuralPanel(h, tile, rural, units); return; }

  const isMountain = tile.terrain === 'mountain_limestone' || tile.terrain === 'mountain_red';
  const isSea = tile.terrain === 'coastal_sea' || tile.terrain === 'deep_sea';
  openTerrainPanel(h, tile, isMountain, isSea, units);
}

export function closeInspect() {
  State.selectedHex = null;
  State.fovPreview = null;
  document.getElementById('inspect-panel').style.display = 'none';
  State.dirty = true;
}

export async function sendMessengerFromInspect(destSettlementID) {
  const textEl = document.getElementById('ip-msg-text');
  const errEl  = document.getElementById('ip-msg-err');
  const text = textEl ? textEl.value.trim() : '';
  if (!text) { if (errEl) { errEl.style.color='var(--accent)'; errEl.textContent='Write a message first.'; } return; }
  const token = localStorage.getItem('poleia_token');
  // Founder phase: no settlement to send from — the host itself is the origin.
  const sendPath = State.MY_SETTLEMENT_ID
    ? `/api/v1/worlds/${State.WORLD_ID}/settlements/${State.MY_SETTLEMENT_ID}/messengers`
    : `/api/v1/worlds/${State.WORLD_ID}/founding/messengers`;
  const res = await fetch(sendPath, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + token },
    body: JSON.stringify({ destination_id: destSettlementID, message: text }),
  });
  if (res.ok) {
    textEl.value = '';
    errEl.style.color = 'var(--safe)';
    errEl.textContent = 'Messenger sent.';
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/messengers`).then(r => r.ok && r.json().then(d => { State.messengerData = d; State.dirty = true; }));
  } else {
    const err = await res.json().catch(() => ({error: 'Unknown error'}));
    errEl.style.color = 'var(--accent)';
    errEl.textContent = err.error || 'Error';
  }
}

// ── initMap — everything that used to run immediately at the bottom of the
// old single <script> and needs State.WORLD_ID (or otherwise must not fire
// before bootstrap() has populated State). Called once by main.js right
// after bootstrap resolves. Input-handler *registration* below has no
// bootstrap dependency of its own (the handlers only read State when they
// fire, well after bootstrap is done) but is grouped here anyway so this
// module's entire "starts doing things" surface lives in one place — see
// the FAS 2 execution report for the full init()-function inventory. ──────
export function initMap() {
  // ── Input: drag + zoom + click ──────────────────────────────────────────
  canvas.addEventListener('mousedown', e => {
    State.dragging = true;
    State.lastMouse = {x: e.clientX, y: e.clientY};
  });
  canvas.addEventListener('mouseup', e => {
    if (!State.dragging) return;
    const dx = e.clientX - State.lastMouse.x, dy = e.clientY - State.lastMouse.y;
    State.dragging = false;
    if (Math.abs(dx) < 4 && Math.abs(dy) < 4) {
      const rect = canvas.getBoundingClientRect();
      const h = hexAtScreen(e.clientX - rect.left, e.clientY - rect.top);
      openHexPanel(h);
    }
  });
  canvas.addEventListener('mouseleave', () => { State.dragging = false; tooltip.style.display = 'none'; });
  canvas.addEventListener('mousemove', e => {
    if (State.dragging && State.lastMouse) {
      State.camera.x += e.clientX - State.lastMouse.x;
      State.camera.y += e.clientY - State.lastMouse.y;
      State.lastMouse = {x: e.clientX, y: e.clientY};
      State.dirty = true;
    }
    const rect = canvas.getBoundingClientRect();
    const h = hexAtScreen(e.clientX - rect.left, e.clientY - rect.top);
    const tile = State.tileData.find(t => t.q === h.q && t.r === h.r);
    const prov = State.provinceData.find(p => p.q === h.q && p.r === h.r);
    if (tile && tile.terrain !== 'fog') {
      tooltip.style.display = 'block';
      tooltip.style.left = (e.clientX + 14) + 'px';
      tooltip.style.top  = (e.clientY - 22) + 'px';
      const deposits = [tile.copper_deposit ? '⚒ Copper' : null, tile.tin_deposit ? '⚒ Tin' : null,
                        tile.silver_deposit ? '⚒ Silver' : null, tile.cedar_deposit ? '⚒ Cedar' : null].filter(Boolean).join(' · ');
      const tl = tile.terrain.charAt(0).toUpperCase() + tile.terrain.slice(1);
      if (prov) {
        const parts = [prov.name, tl];
        if (prov.owner) parts.push(`Wanax: ${prov.owner}`);
        if (prov.walls > 0) parts.push(`Walls L${prov.walls}`);
        if (prov.culture) parts.push(prov.culture);
        if (prov.own) parts.push('(you)');
        else if (prov.allied) parts.push('(ally)');
        if (deposits) parts.push(deposits);
        tooltip.textContent = parts.join(' · ');
      } else {
        const base = `(${h.q},${h.r}) ${tl}`;
        tooltip.textContent = deposits ? `${base} · ${deposits}` : base;
      }
    } else {
      tooltip.style.display = 'none';
    }
  });
  canvas.addEventListener('wheel', e => {
    e.preventDefault();
    const rect = canvas.getBoundingClientRect();
    const mx = e.clientX - rect.left, my = e.clientY - rect.top;
    const factor = e.deltaY < 0 ? 1.1 : 0.91;
    State.camera.x = mx + (State.camera.x - mx) * factor;
    State.camera.y = my + (State.camera.y - my) * factor;
    State.camera.zoom = clampZoom(State.camera.zoom * factor);
    State.dirty = true;
  }, {passive:false});

  window.addEventListener('resize', () => { State.dirty = true; });

  // Hide map tooltip when pointer enters any drawer
  document.querySelectorAll('.drawer').forEach(d => {
    d.addEventListener('mouseenter', () => { tooltip.style.display = 'none'; });
  });

  loadMap().then(() => { State.dirty = true; render(); });

  // Right-click to open march menu
  canvas.addEventListener('contextmenu', e => {
    e.preventDefault();
    const capital = ownCapital();
    if (!capital) return;
    const rect = canvas.getBoundingClientRect();
    const h = hexAtScreen(e.clientX - rect.left, e.clientY - rect.top);
    const target = State.provinceData.find(p => p.q === h.q && p.r === h.r);
    const tile = State.tileData.find(t => t.q === h.q && t.r === h.r);
    if (!tile || tile.terrain === 'fog') { window.closeMarchCtx(); return; }
    const isMountain = tile.terrain === 'mountain_limestone' || tile.terrain === 'mountain_red';
    if (target) {
      // Own settlement (capital included): march units home to reinforce the
      // garrison. Another Wanax's settlement: march to attack (or reinforce if
      // allied). Inspect lives on left-click — right-click is always orders.
      window.openMarchCtx(destFromHex(h, tile, target), e.clientX, e.clientY);
      return;
    }
    // Empty hex. Mountains are impassable; sea hexes take ships.
    if (isMountain) { window.closeMarchCtx(); return; }
    // Moment-22: an own field unit standing right here IS the order target,
    // not the hex — marching-to-your-own-hex is rejected server-side ("cannot
    // march to own hex"). Route to the same map→drawer bridge the left-click
    // "Visa →" button uses (war.js warFocusUnit): the War drawer opens with
    // that unit's card focused, its March button ready. Same orderable
    // condition as war.js canMarch (garrison never applies on an empty hex).
    const ownUnit = (State.unitsData || []).find(u =>
      u.q === h.q && u.r === h.r && (u.status === 'garrison' || u.status === 'positioned') &&
      u.type !== 'priest' && (u.category === 'naval' || u.size === 100));
    if (ownUnit) { window.closeMarchCtx(); window.warFocusUnit(ownUnit.id); return; }
    window.openMarchCtx(destFromHex(h, tile, null), e.clientX, e.clientY);
  });

  // Reload provinces, marches, messengers and trades every 30s
  setInterval(() => {
    refreshTiles();
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces`).then(r => r.ok && r.json().then(d => { State.provinceData = d; State.dirty = true; window.MusicPlayer.update(); }));
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/marches`).then(r => r.ok && r.json().then(d => { State.marchData = d; State.dirty = true; window.MusicPlayer.update(); }));
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/messengers`).then(r => r.ok && r.json().then(d => { State.messengerData = d; State.dirty = true; }));
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/trades`).then(r => r.ok && r.json().then(d => { State.tradeData = d; State.dirty = true; }));
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units`).then(r => r.ok && r.json().then(d => { State.unitsData = d.units || []; State.dirty = true; }));
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/rural-projections`).then(r => r.ok && r.json().then(d => { State.ruralData = d; State.dirty = true; }));
  }, 30000);

  // While any own unit is marching, refresh units + fog fast so the fog visibly
  // sweeps around the moving unit during the trip (the ship's on-screen position
  // already interpolates every frame; this is only needed so server-computed fog
  // keeps up). Idle — and cheap — whenever nothing is moving.
  setInterval(() => {
    const courierOut = State.messengerData.some(m => m.own);
    if (!State.unitsData.some(u => u.status === 'marching') && !courierOut) return;
    refreshTiles();
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units`).then(r => r.ok && r.json().then(d => { State.unitsData = d.units || []; State.dirty = true; }));
    // A running hemerodromos needs the same fast cadence: its delivery flips
    // the unit to marching (or applies a stance) server-side — poll messengers
    // so the runner vanishes and the unit moves without waiting for the 30s tick.
    if (courierOut) {
      fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/messengers`).then(r => r.ok && r.json().then(d => { State.messengerData = d; State.dirty = true; }));
    }
  }, 3000);
}
