import test from 'node:test';
import assert from 'node:assert/strict';
import { drawStamp, activityStamp, ACTIVITY_ICONS } from './cityicons.js';

function recorder() {
  const rects = [];
  return { rects, set fillStyle(v) {}, fillRect(x, y, w, h) { rects.push([x, y, w, h]); } };
}

test('drawStamp scale 2 draws 2x2 squares on whole pixels', () => {
  const ctx = recorder();
  drawStamp(ctx, ['K.', '.K'], 10, 20, { K: '#000' }, 2);
  assert.deepEqual(ctx.rects, [[10, 20, 2, 2], [12, 22, 2, 2]]);
});

test('drawStamp default scale stays 1x1', () => {
  const ctx = recorder();
  drawStamp(ctx, ['K.', '.K'], 10, 20, { K: '#000' });
  assert.deepEqual(ctx.rects, [[10, 20, 1, 1], [11, 21, 1, 1]]);
});

test('activityStamp: build and train give a stamp, an idle city gives none', () => {
  assert.equal(activityStamp({ build_active: true }), ACTIVITY_ICONS.build);
  assert.equal(activityStamp({ train_active: true }), ACTIVITY_ICONS.train);
  assert.equal(activityStamp({}), null);
  assert.equal(ACTIVITY_ICONS.idle, undefined);
});
