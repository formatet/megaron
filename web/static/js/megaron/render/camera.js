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
