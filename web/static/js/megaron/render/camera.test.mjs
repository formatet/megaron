import test from 'node:test';
import assert from 'node:assert/strict';
import { clampZoom, zoomStep, clampPan } from './camera.js';
import { ZOOM_MIN, ZOOM_MAX } from '../config.js';

test('AK1: at ZOOM_MIN, a further zoom-out step leaves camera bit-identical', () => {
  const camera = { x: 100, y: 100, zoom: ZOOM_MIN };
  const next = zoomStep(camera, 0.91, 50, 50);
  assert.equal(next.zoom, camera.zoom, 'zoom should stay at floor');
  assert.equal(next.x, camera.x, 'camera.x should not move');
  assert.equal(next.y, camera.y, 'camera.y should not move');
});

test('AK2: at ZOOM_MAX, a further zoom-in step leaves camera bit-identical', () => {
  const camera = { x: 100, y: 100, zoom: ZOOM_MAX };
  const next = zoomStep(camera, 1.1, 50, 50);
  assert.equal(next.zoom, camera.zoom, 'zoom should stay at ceiling');
  assert.equal(next.x, camera.x, 'camera.x should not move');
  assert.equal(next.y, camera.y, 'camera.y should not move');
});

test('AK3 (regression): an unclamped step matches the pre-fix arithmetic exactly', () => {
  const camera = { x: 100, y: 100, zoom: 1 };
  const factor = 1.25;
  const next = zoomStep(camera, factor, 50, 50);
  // Pre-fix arithmetic: translate by the requested factor, clamp zoom after.
  // Away from the bounds, effective === requested, so this must match exactly.
  const expectedZoom = clampZoom(camera.zoom * factor);
  const expectedX = 50 + (camera.x - 50) * factor;
  const expectedY = 50 + (camera.y - 50) * factor;
  assert.equal(next.zoom, expectedZoom);
  assert.equal(next.x, expectedX);
  assert.equal(next.y, expectedY);
});

test('AK4: a partially-clamped step translates by the EFFECTIVE factor, not the requested one', () => {
  const camera = { x: 100, y: 100, zoom: 0.32 };
  const factor = 0.5; // requested: 0.32*0.5 = 0.16, clamped to ZOOM_MIN=0.3
  const effective = 0.3 / 0.32; // 0.9375
  const next = zoomStep(camera, factor, 50, 50);
  assert.equal(next.zoom, 0.3);
  assert.equal(next.x, 50 + (camera.x - 50) * effective);
  assert.equal(next.y, 50 + (camera.y - 50) * effective);
  // The leak-in-the-other-direction check: requested factor (0.5) and the
  // "freeze at clamp" answer (1) must both be wrong here.
  assert.notEqual(next.x, 50 + (camera.x - 50) * factor);
  assert.notEqual(next.x, camera.x);
});

test('clampZoom bounds', () => {
  assert.equal(clampZoom(0.01), ZOOM_MIN);
  assert.equal(clampZoom(100), ZOOM_MAX);
  assert.equal(clampZoom(1), 1);
});

// ── clampPan (World-Rim step 1) ─────────────────────────────────────────────
// camera.js clamped zoom only, so nothing stopped a drag from carrying the view
// off the world into empty background. Screen = world·k + camera, k = zoom·scale.

const SCALE = 2;
const VIEW = { w: 800, h: 600 };
// A world far bigger than the viewport: 0..2000 world units × k=2 → 4000 px.
const BIG = { minX: 0, minY: 0, maxX: 2000, maxY: 2000 };
const M = 66; // margin in world units (map.js uses S*3)

test('PAN1: a camera already inside the world is left exactly where it was', () => {
  const cam = { x: -1000, y: -900, zoom: 1 };
  const out = clampPan(cam, SCALE, VIEW, BIG, M);
  assert.equal(out.x, cam.x);
  assert.equal(out.y, cam.y);
});

test('PAN2: panning past the near edge stops at the margin, not at infinity', () => {
  // camera.x = +5000 would put the world far off to the right of the viewport.
  const out = clampPan({ x: 5000, y: 5000, zoom: 1 }, SCALE, VIEW, BIG, M);
  const nearEdge = (BIG.minX - M) * 2; // screen offset of the widened edge
  assert.equal(out.x, -nearEdge, 'the widened near edge must land exactly on screen 0');
  assert.ok(out.x < 5000, 'the camera must actually have been pulled back');
});

test('PAN3: panning past the far edge stops there too', () => {
  const out = clampPan({ x: -99999, y: -99999, zoom: 1 }, SCALE, VIEW, BIG, M);
  const farEdge = (BIG.maxX + M) * 2;
  assert.equal(out.x, VIEW.w - farEdge, 'the widened far edge must land on the viewport edge');
});

test('PAN4: the margin is in WORLD units — it scales with zoom, it is not a pixel frame', () => {
  const at1 = clampPan({ x: 5000, y: 5000, zoom: 1 }, SCALE, VIEW, BIG, M);
  const at2 = clampPan({ x: 5000, y: 5000, zoom: 2 }, SCALE, VIEW, BIG, M);
  // Twice the zoom → twice as many pixels of margin. A pixel-specified margin
  // would give the same number at both zooms; that is the trap being pinned.
  assert.equal(at2.x, at1.x * 2);
});

test('PAN5: a world smaller than the viewport is centred, not shoved into a corner', () => {
  const small = { minX: 0, minY: 0, maxX: 100, maxY: 100 }; // 200px + margins ≪ 800
  const out = clampPan({ x: 12345, y: -777, zoom: 1 }, SCALE, VIEW, small, M);
  const lo = (small.minX - M) * 2, hi = (small.maxX + M) * 2;
  assert.equal(lo + out.x, VIEW.w - (hi + out.x), 'equal empty space on both sides');
});

test('PAN6: an unloaded world (null bounds) leaves the camera untouched', () => {
  const cam = { x: 42, y: -17, zoom: 1 };
  const out = clampPan(cam, SCALE, VIEW, null, M);
  assert.equal(out.x, 42);
  assert.equal(out.y, -17);
});

test('PAN7: both axes are clamped — a bound on x only is not a bound', () => {
  const out = clampPan({ x: 5000, y: 5000, zoom: 1 }, SCALE, VIEW, BIG, M);
  assert.notEqual(out.y, 5000, 'the vertical axis must be clamped as well');
});
