// The omen reveal (byggplan D.2): when an answered prayer marks a place, the
// revealed hexes lift out of the fog one at a time, OMEN_STEP_MS apart, shine
// live green for OMEN_HOLD_MS, then fade in steps to the memory grey the
// server already gave them. Authoritative tiles are released only on the 300 ms
// steps; the green overlay never paints fog. Pure, so it can be tested
// without a canvas; the canvas palette is the renderer's own.
import { pixelSpans } from './memory.js';

export const OMEN_STEP_MS = 300;
export const OMEN_HOLD_MS = 1500;
export const OMEN_FADE_MS = 900;
const FADE_STEPS = 3; // stepped, never a soft ramp (megaron_grafikregler)
const OMEN_LIVE = '#7FB54A';  // the gods' living green
const OMEN_LIVE_ALPHA = 0.6;

export function omenDuration(count) {
  return Math.max(0, count) * OMEN_STEP_MS + OMEN_HOLD_MS + OMEN_FADE_MS;
}

// Phase of the index-th revealed hex, elapsed ms after the reveal began.
// alpha on a fading hex is how far it has gone to memory grey.
export function omenPhase(index, elapsed) {
  const t = elapsed - (index + 1) * OMEN_STEP_MS;
  if (t < 0) return { phase: 'hidden', alpha: 0 };
  if (t < OMEN_HOLD_MS) return { phase: 'live', alpha: 0 };
  const f = t - OMEN_HOLD_MS;
  if (f < OMEN_FADE_MS) return { phase: 'fade', alpha: (Math.floor(f / (OMEN_FADE_MS / FADE_STEPS)) + 1) / (FADE_STEPS + 1) };
  return { phase: 'memory', alpha: 1 };
}

// omenOverlay: what to paint over each revealed hex this frame, or null once
// the reveal is over.
export function omenOverlay(omen, now) {
  const elapsed = now - omen.start;
  if (elapsed >= omenDuration(omen.tiles.length)) return null;
  const out = [];
  omen.tiles.forEach((p, i) => {
    const { phase, alpha } = omenPhase(i, elapsed);
    if (phase === 'live') out.push({ q: p.q, r: p.r, fill: OMEN_LIVE, alpha: OMEN_LIVE_ALPHA });
    else if (phase === 'fade') out.push({ q: p.q, r: p.r, fill: OMEN_LIVE, alpha: OMEN_LIVE_ALPHA * (1 - alpha) });
  });
  return out;
}

// drawOmen paints the overlay with whole device pixels, like the memory tint.
export function drawOmen(ctx, overlay, polygon) {
  const m = ctx.getTransform();
  ctx.save();
  try {
    ctx.resetTransform();
    for (const o of overlay) {
      ctx.globalAlpha = o.alpha;
      ctx.fillStyle = o.fill;
      ctx.beginPath();
      const points = polygon(o).map(([x, y]) => [m.a * x + m.c * y + m.e, m.b * x + m.d * y + m.f]);
      for (const [x, y, w] of pixelSpans(points, ctx.canvas.width, ctx.canvas.height)) ctx.rect(x, y, w, 1);
      ctx.fill();
    }
  } finally {
    ctx.restore();
  }
}


const key = t => `${t.q},${t.r}`;

// Snapshot eligibility before the map request: explored/live tiles never enter
// the animation, even if the prayer's radius includes them. Absent tiles are fog.
export function beginOmen(before, tiles, find, start) {
  const old = new Map(before.map(t => [key(t), t]));
  const unique = new Map(tiles.map(t => [key(t), { q: t.q, r: t.r }]));
  const dist = t => find ? (Math.abs(t.q-find.q)+Math.abs(t.q+t.r-find.q-find.r)+Math.abs(t.r-find.r))/2 : 0;
  const order = [...unique.values()].filter(t => !old.has(key(t)) || old.get(key(t)).terrain === 'fog')
    .sort((a,b) => dist(a)-dist(b) || a.q-b.q || a.r-b.r);
  return { tiles: order, start, before, old, latest: before, cached: before, count: 0, source: before };
}

// A poll can deliver the fully revealed map mid-animation. Keep pending hexes
// at their original fog (or absent), rather than covering terrain with new ink.
// Preserve array identity between steps: the normal raster cache stays baked.
export function omenMap(omen, latest, now) {
  if (!omen.tiles.length) return latest;
  const count = Math.min(omen.tiles.length, Math.max(0, Math.floor((now-omen.start)/OMEN_STEP_MS)));
  if (omen.source === latest && omen.count === count) return omen.cached;
  omen.source = latest;
  omen.count = count;
  if (count === 0) return (omen.cached = omen.before);
  const pending = new Set(omen.tiles.slice(count).map(key));
  omen.cached = latest.flatMap(t => pending.has(key(t)) ? (omen.old.has(key(t)) ? [omen.old.get(key(t))] : []) : [t]);
  return omen.cached;
}
