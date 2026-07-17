import { State, ownCapital } from '../state.js';
import { fetchAuth } from '../api.js';
import { serverNow } from '../clock.js';
import { LIVE_RADIUS_SEA, LIVE_RADIUS_BASE, LIVE_RADIUS_MOUNTAIN_BONUS } from '../config.js';

// ── Palette — Settlers 2 warmth, Mediterranean olive country ─────────────
const TERRAIN_BASE = {
  deep_sea:           {c0:'#1A5276', c1:'#154360'},
  coastal_sea:        {c0:'#3A9AD9', c1:'#2E86C1'},
  coast_beach:        {c0:'#E8C97A', c1:'#D4B060'},
  plains:             {c0:'#8AAF3A', c1:'#74952C'},
  river_valley:       {c0:'#4CAF50', c1:'#388E3C'},
  river_delta:        {c0:'#6BBF59', c1:'#4E9B3E'},
  forest_olive_grove: {c0:'#5A8C30', c1:'#447020'},
  forest_olive_cypress:{c0:'#4A7A20', c1:'#386010'},
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

// ── Terrain detail — Settlers 2 quality ──────────────────────────────────
function drawDetail(ctx, cx, cy, terrain, seed, frame) {
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
    case 'forest_olive_grove':
    case 'forest_olive_cypress': {
      const dotColor = terrain === 'forest_olive_grove' ? '#3A6818' : '#2A5010';
      for (let i = 0; i < 3; i++) {
        const ox = ((seed * (i*11+2)) & 0x1b) - 12;
        const oy = ((seed * (i*9+4)) & 0x1b) - 12;
        ctx.fillStyle = dotColor;
        ctx.beginPath();
        ctx.arc(cx+ox, cy+oy, 3, 0, Math.PI*2);
        ctx.fill();
        ctx.fillStyle = '#5A9028';
        ctx.beginPath();
        ctx.arc(cx+ox-1, cy+oy-1, 1.5, 0, Math.PI*2);
        ctx.fill();
      }
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
    case 'coast_beach': {
      // animated wave ripple
      const waveTick = (frame >> 4) & 0x3;
      ctx.strokeStyle = 'rgba(255,255,255,0.18)';
      ctx.lineWidth = 0.8;
      for (let i = 0; i < 2; i++) {
        const wox = r3 - 6, woy = r4 - 6 + i * 5 + waveTick;
        ctx.beginPath();
        ctx.arc(cx + wox, cy + woy, 5, 0.2, Math.PI - 0.2);
        ctx.stroke();
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
  types.forEach((t, i) => {
    const ox = cx + 9, oy = cy - 8 + i * 5;
    switch (t) {
      case 'cu':
        ctx.fillStyle = '#C47C20';
        ctx.beginPath(); ctx.arc(ox, oy, 2, 0, Math.PI*2); ctx.fill();
        break;
      case 'sn':
        ctx.fillStyle = '#909090';
        ctx.fillRect(ox - 2, oy - 1.5, 4, 3);
        break;
      case 'cd':
        ctx.fillStyle = '#2A7010';
        ctx.beginPath(); ctx.moveTo(ox, oy - 3); ctx.lineTo(ox + 2.5, oy + 1.5); ctx.lineTo(ox - 2.5, oy + 1.5); ctx.closePath(); ctx.fill();
        break;
      case 'ag':
        ctx.fillStyle = '#C0C8D8';
        ctx.beginPath(); ctx.moveTo(ox, oy - 2.5); ctx.lineTo(ox + 2, oy); ctx.lineTo(ox, oy + 2.5); ctx.lineTo(ox - 2, oy); ctx.closePath(); ctx.fill();
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
  if (p.own && p.army_total > 0 && State.camera.zoom >= 0.45) {
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

function render() {
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
    if (t.terrain !== 'fog') drawDetail(ctx, x, y, t.terrain, seed, State.animFrame);
    if (t.terrain !== 'fog' && State.camera.zoom >= 0.5) drawDepositIcons(ctx, x, y, t);
  }

  // 2. Roads between adjacent own/allied provinces
  if (State.camera.zoom >= 0.5) {
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
  if (State.camera.zoom >= 0.55) {
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
    if (State.camera.zoom >= 0.55) drawLabel(ctx, x, y, p.name, p.own);
  }

  // 4b. Activity overlay badges (own non-outpost cities, zoom >= 0.4)
  if (State.activityOverlay && State.camera.zoom >= 0.4) {
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
  const [tilesRes, provRes, marchRes, msgRes, tradeRes, unitsRes] = await Promise.all([
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/map`),
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces`),
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/marches`),
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/messengers`),
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/trades`),
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units`),
  ]);

  if (tilesRes.ok) {
    State.tileData = await tilesRes.json();
    centreCamera();
  }
  if (provRes.ok) {
    State.provinceData = await provRes.json();
  }
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
  window.MusicPlayer.update();
}

function centreCamera() {
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
export function zoom(factor) {
  const cx = canvas.width/2, cy = canvas.height/2;
  State.camera.x = cx + (State.camera.x - cx) * factor;
  State.camera.y = cy + (State.camera.y - cy) * factor;
  State.camera.zoom = Math.min(5, Math.max(0.2, State.camera.zoom * factor));
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
  fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/colonize-preview?q=${h.q}&r=${h.r}`
      + `&pop=${fp.population}&seed=${Math.max(0, Math.round(fp.grain?.amount || 0))}`)
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
      const deposits = [tile.copper_deposit ? '⚒ Copper' : null, tile.tin_deposit ? '⚒ Tin' : null].filter(Boolean).join(' · ');
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
    State.camera.zoom = Math.min(5, Math.max(0.2, State.camera.zoom * factor));
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
