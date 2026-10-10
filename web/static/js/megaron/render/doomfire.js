// ── Doom fire over a burning city ────────────────────────────────────────
// A city sacked and burned this tick (/provinces `burning`) burns on the map
// for that one game day, then stands as a ruin. The effect is the PSX Doom fire
// (Fabien Sanglard's write-up): the bottom row is held hot, every pixel copies
// the one beneath it with a random sideways drift and a random one-step
// cooling. Reimplemented here from the description — no third-party code.
//
// Whole pixels, a stepped palette, no blending (megaron_grafikregler).
// Clocked on State.animFrame, never the wall clock, and seeded per city, so
// the frozen-frame BILD rigs get the same picture every time.

export const FIRE_W = 16;
export const FIRE_H = 18;
// Render frames per fire step. The render loop wakes on this phase change.
export const FIRE_FRAMES = 3;
// Catch-up cap after the tab was hidden: the fire forgets its past within
// FIRE_H steps anyway, so replaying more than that buys nothing.
const MAX_CATCHUP = FIRE_H * 2;

// Index 0 = no fire (transparent). Charcoal ember up to white-hot core.
export const FIRE_PALETTE = [
  null, '#2A1A12', '#5A1E0E', '#8A2A0A', '#B8400A',
  '#D86A10', '#E89A20', '#F0C840', '#F8F0B0',
];
const HOT = FIRE_PALETTE.length - 1;

// mulberry32 — small deterministic PRNG.
function rng(seed) {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6D2B79F5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return (t ^ (t >>> 14)) >>> 0;
  };
}

function seedOf(key) {
  let h = 2166136261;
  for (let i = 0; i < key.length; i++) h = Math.imul(h ^ key.charCodeAt(i), 16777619);
  return h >>> 0;
}

export function newFire(key) {
  const buf = new Uint8Array(FIRE_W * FIRE_H);
  // Hot source row, cooler at the edges so the flames rise as a pyre, not a wall.
  for (let x = 3; x < FIRE_W - 3; x++) buf[(FIRE_H - 1) * FIRE_W + x] = HOT;
  return { buf, step: 0, next: rng(seedOf(key)) };
}

// One Doom-fire step, in place as in Doom: a pixel no flame reached this step
// keeps last step's value, which is what gives the fire its body. The drift
// is symmetric (Doom's leans left, as wind) and a flame drifting off the
// field is gone — clamping it piled a solid column against the edge.
export function stepFire(f) {
  const { buf, next } = f;
  for (let x = 0; x < FIRE_W; x++) {
    for (let y = 1; y < FIRE_H; y++) {
      const src = buf[y * FIRE_W + x];
      if (src === 0) { buf[(y - 1) * FIRE_W + x] = 0; continue; }
      const r = next();
      const dx = x + (r % 3) - 1;
      if (dx < 0 || dx >= FIRE_W) continue;
      // Cools one step 5 times in 8 (Doom: 1 in 2) so the flames end in
      // tongues below the top of the field instead of filling it.
      buf[(y - 1) * FIRE_W + dx] = Math.max(0, src - (((r >>> 8) & 7) < 5 ? 1 : 0));
    }
  }
  f.step++;
}

const fires = new Map();

// The fire for `key` advanced to `target` steps. A target behind the stored
// step (rig reset) restarts it from the seed so frame N is always the same.
export function fireAt(key, target) {
  let f = fires.get(key);
  if (!f || f.step > target) { f = newFire(key); fires.set(key, f); }
  if (target - f.step > MAX_CATCHUP) f.step = target - MAX_CATCHUP;
  while (f.step < target) stepFire(f);
  return f;
}

// Draw the fire with its base at (cx, baseY), over the ruin.
export function drawFire(ctx, cx, baseY, key, animFrame) {
  const f = fireAt(key, Math.floor(animFrame / FIRE_FRAMES));
  const x0 = Math.round(cx - FIRE_W / 2);
  const y0 = Math.round(baseY - FIRE_H);
  ctx.save();
  for (let y = 0; y < FIRE_H; y++) {
    for (let x = 0; x < FIRE_W; x++) {
      const v = f.buf[y * FIRE_W + x];
      if (!v) continue;
      ctx.fillStyle = FIRE_PALETTE[v];
      ctx.fillRect(x0 + x, y0 + y, 1, 1);
    }
  }
  ctx.restore();
}
