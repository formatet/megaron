// ── Storms at sea ────────────────────────────────────────────────────────
// A storm is three connected sea hexes that drift slowly (GET /storms,
// megaron_plan_stormar.md). Live inside the Wanax's sight; outside it, drawn
// where it was LAST SEEN as a dithered ghost — never where it is now.
//
// Whole pixels, a stepped palette, no blending (megaron_grafikregler): chunky
// 2×2 cloud blocks in horizontal bands, sliding rain streaks, a 1px charcoal
// contour only round the storm's OUTER edge (weight marks the storm, not each
// hex), a lightning flash on two phases in four. The cloud churns on a slow phase clocked off State.animFrame, never
// the wall clock, so the frozen-frame rigs get the same picture every time.

export const STORM_FRAMES = 40; // render frames per cloud phase; the loop wakes on a phase change
export const STORM_PHASES = 4;

const CHARCOAL = '#1E232B';
// Violet-slate, deliberately NOT the grey-blue of mountains: a storm must not read as rock.
const TONES = ['#7A7FA6', '#4F5478', '#2B2F4A']; // light · mid · dark
const RAIN = '#C4D2E6';
const FLASH = '#F4E27A';

// Neighbour across edge i of a flat-top hex whose corner i sits at angle i·60°
// (hexPts in map.js): edge i faces the axial neighbour listed here.
export const EDGE_NEIGHBOUR = [[1, 0], [0, 1], [-1, 1], [-1, 0], [0, -1], [1, -1]];

// Which edges of each hex are on the storm's outer boundary.
export function stormOuterEdges(hexes) {
  const inside = new Set(hexes.map(h => `${h.q},${h.r}`));
  return hexes.map(h => EDGE_NEIGHBOUR.map(([dq, dr]) => !inside.has(`${h.q + dq},${h.r + dr}`)));
}

function hash(x, y, phase) {
  let h = Math.imul(x * 374761393 + y * 668265263 + phase * 2246822519, 3266489917);
  h = Math.imul(h ^ (h >>> 15), 2246822519);
  return (h ^ (h >>> 13)) >>> 0;
}

// Tone index (0 light … 2 dark) of the cloud pixel at (x, y): 2×2 blocks, two-row bands.
export function stormTone(x, y, phase) {
  const n = hash(x >> 1, y >> 1, phase) & 7;
  const band = (y >> 2) & 1;
  const v = n + band * 2;
  return v < 2 ? 0 : v < 6 ? 1 : 2;
}

// Rain: short diagonal streaks that slide down-left one step per phase.
export function stormRain(x, y, phase) {
  return (x * 2 + y + phase * 3) % 9 === 0 && (y % 5) < 3;
}

function inHex(dx, dy, S) {
  const ax = Math.abs(dx), ay = Math.abs(dy);
  return ay <= S * Math.sqrt(3) / 2 && Math.sqrt(3) * ax + ay <= Math.sqrt(3) * S;
}

const sprites = new Map();
function hexSprite(S, phase, ghost) {
  const key = `${S}:${phase}:${ghost ? 1 : 0}`;
  let c = sprites.get(key);
  if (c) return c;
  const w = 2 * S + 1, h = Math.ceil(S * Math.sqrt(3)) + 1;
  c = document.createElement('canvas');
  c.width = w; c.height = h;
  const g = c.getContext('2d');
  const cx = S, cy = h >> 1;
  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      if (!inHex(x - cx, y - cy, S)) continue;
      if (ghost && ((x + y) & 1)) continue; // dither: a remembered storm is half there
      g.fillStyle = ghost ? TONES[1] : (stormRain(x, y, phase) ? RAIN : TONES[stormTone(x, y, phase)]);
      g.fillRect(x, y, 1, 1);
    }
  }
  sprites.set(key, c);
  return c;
}

// Draw one storm. hexPx(q, r) → world px centre; hexPts(cx, cy) → six corner points.
export function drawStorm(ctx, storm, { S, hexPx, hexPts, frame }) {
  const ghost = storm.tier !== 'live';
  const phase = ghost ? 0 : ((frame / STORM_FRAMES) | 0) % STORM_PHASES;
  const spr = hexSprite(S, phase, ghost);
  for (const h of storm.hexes) {
    const { x, y } = hexPx(h.q, h.r);
    ctx.drawImage(spr, x - S, y - (spr.height >> 1));
  }
  const outer = stormOuterEdges(storm.hexes);
  ctx.save();
  ctx.strokeStyle = CHARCOAL;
  ctx.lineWidth = 1;
  if (ghost) ctx.setLineDash([2, 2]);
  storm.hexes.forEach((h, i) => {
    const { x, y } = hexPx(h.q, h.r);
    const pts = hexPts(x, y);
    outer[i].forEach((isOuter, e) => {
      if (!isOuter) return;
      const a = pts[e], b = pts[(e + 1) % 6];
      ctx.beginPath();
      ctx.moveTo(a[0], a[1]);
      ctx.lineTo(b[0], b[1]);
      ctx.stroke();
    });
  });
  ctx.restore();
  // A lightning flash in one hex now and then (one phase in four).
  if (!ghost && (phase === 1 || phase === 3)) {
    const h = storm.hexes[(hash(storm.hexes[0].q, storm.hexes[0].r, 7) >>> 3) % 3];
    const { x, y } = hexPx(h.q, h.r);
    ctx.fillStyle = FLASH;
    for (const [dx, dy] of [[1, -6], [0, -5], [-1, -4], [0, -3], [1, -2], [0, -1], [-1, 0], [0, 1]]) ctx.fillRect(x + dx, y + dy, 1, 1);
  }
}
