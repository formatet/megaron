import test from 'node:test';
import assert from 'node:assert/strict';
import { clampZoom, zoomStep } from './camera.js';
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
