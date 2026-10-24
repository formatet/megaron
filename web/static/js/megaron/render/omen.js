// The omen reveal (byggplan D.2): when an answered prayer marks a place, the
// revealed hexes lift out of the fog one at a time, OMEN_STEP_MS apart, shine
// live green for OMEN_HOLD_MS, then fade in steps to the memory grey the
// server already gave them. Drawn as an overlay over the finished map, so the
// baked terrain is never rebaked per frame. Pure, so it can be tested
// without a canvas; the canvas palette is the renderer's own.
import { pixelSpans } from './memory.js';

export const OMEN_STEP_MS = 300;
export const OMEN_HOLD_MS = 1500;
export const OMEN_FADE_MS = 900;
const FADE_STEPS = 3; // stepped, never a soft ramp (megaron_grafikregler)
const OMEN_FOG = '#1C1C1C';   // markraster fog c0
const OMEN_LIVE = '#7FB54A';  // the gods' living green
const OMEN_LIVE_ALPHA = 0.6;

export function omenDuration(count) {
  return Math.max(0, count - 1) * OMEN_STEP_MS + OMEN_HOLD_MS + OMEN_FADE_MS;
}

// Phase of the index-th revealed hex, elapsed ms after the reveal began.
// alpha on a fading hex is how far it has gone to memory grey.
export function omenPhase(index, elapsed) {
  const t = elapsed - index * OMEN_STEP_MS;
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
    if (phase === 'hidden') out.push({ q: p.q, r: p.r, fill: OMEN_FOG, alpha: 1 });
    else if (phase === 'live') out.push({ q: p.q, r: p.r, fill: OMEN_LIVE, alpha: OMEN_LIVE_ALPHA });
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
