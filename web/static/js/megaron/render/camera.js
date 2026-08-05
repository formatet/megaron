import { ZOOM_MIN, ZOOM_MAX } from '../config.js';

// Single source of truth for the zoom clamp — was previously duplicated
// between zoom() and the wheel handler in initMap(), which is why raising
// the floor in only one of them didn't fix Timothy's "zooms out too far"
// report (2026-07-25). Bounds themselves are named calibration in config.js.
export function clampZoom(z) {
  return Math.min(ZOOM_MAX, Math.max(ZOOM_MIN, z));
}

// Applies a requested zoom factor to a camera state, anchored at (ax, ay).
// The zoom itself is clamped to [ZOOM_MIN, ZOOM_MAX] first, then the camera
// is translated with the EFFECTIVE factor (clampedZoom / oldZoom) — not the
// requested factor. At the clamp the effective factor is exactly 1, so the
// camera doesn't move at all; on a partially-clamped step it moves by the
// partial amount. Without this, the two call sites translated by the
// requested factor and clamped the zoom afterwards, so at ZOOM_MIN/ZOOM_MAX
// the camera kept sliding toward the anchor while the scale stood still.
export function zoomStep(camera, factor, ax, ay) {
  const newZoom = clampZoom(camera.zoom * factor);
  const effective = newZoom / camera.zoom;
  return {
    x: ax + (camera.x - ax) * effective,
    y: ay + (camera.y - ay) * effective,
    zoom: newZoom,
  };
}

// Pan clamp (World-Rim step 1, 2026-08-05). Until now camera.js clamped ZOOM
// only — there was no pan bound at all, so a drag or a held key carried the
// view off the world entirely and left the player staring at empty background.
//
// Screen position of a world point: screen = world·k + camera, k = zoom·scale.
// The visible strip is [0, viewport]. The world rect, widened by `margin`, must
// keep covering it: the widened left edge may not slide right of 0, and the
// widened right edge may not slide left of viewport. When the widened rect is
// SHORTER than the viewport (fully zoomed out on a small world) no such camera
// exists — then centre it, which is what a player expects anyway.
//
// margin is in WORLD UNITS, never screen pixels. In pixels the visible border
// would be a thick frame at min zoom and a hairline at 1:1 — the same trap the
// ground lattice fell into (megaron_terrangrendering, LOD 2026-08-03).
function clampAxis(cam, k, viewport, min, max, margin) {
  const lo = (min - margin) * k; // screen offset of the widened near edge
  const hi = (max + margin) * k; // ... and of the far edge
  if (hi - lo <= viewport) return viewport / 2 - (lo + hi) / 2; // world fits: centre it
  return Math.min(-lo, Math.max(viewport - hi, cam));
}

// clampPan returns {x, y} for a camera, given the canvas size in px, the world's
// bounding box in world units, and a margin in world units. `world` may be null
// (tiles not loaded yet) — then the camera is returned untouched, because a
// clamp against an unknown world would snap the view to the origin.
export function clampPan(camera, scale, viewport, world, margin) {
  if (!world) return { x: camera.x, y: camera.y };
  const k = camera.zoom * scale;
  return {
    x: clampAxis(camera.x, k, viewport.w, world.minX, world.maxX, margin),
    y: clampAxis(camera.y, k, viewport.h, world.minY, world.maxY, margin),
  };
}
