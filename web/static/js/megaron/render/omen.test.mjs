import test from 'node:test';
import assert from 'node:assert/strict';
import { omenPhase, omenOverlay, OMEN_STEP_MS, OMEN_HOLD_MS, OMEN_FADE_MS, omenDuration } from './omen.js';

// Byggplan D.2 (Timothy's bug report): an answered prayer that shows a place
// lifts the fog to live colour one hex at a time, ~0.3 s apart, then fades it
// to memory grey in steps.
test('each revealed hex waits its turn, shows live, fades in steps, then is memory', () => {
  assert.equal(OMEN_STEP_MS, 300);
  assert.equal(omenPhase(2, 2 * OMEN_STEP_MS - 1).phase, 'hidden');
  assert.equal(omenPhase(2, 2 * OMEN_STEP_MS).phase, 'live');
  const f1 = omenPhase(0, OMEN_HOLD_MS);
  const f2 = omenPhase(0, OMEN_HOLD_MS + OMEN_FADE_MS * 0.7);
  assert.equal(f1.phase, 'fade');
  assert.ok(f1.alpha > 0 && f1.alpha < f2.alpha && f2.alpha < 1, `stepped fade ${f1.alpha} → ${f2.alpha}`);
  assert.equal(omenPhase(0, OMEN_HOLD_MS + OMEN_FADE_MS).phase, 'memory');
  assert.equal(omenDuration(3), 2 * OMEN_STEP_MS + OMEN_HOLD_MS + OMEN_FADE_MS);
});

test('overlay: fog over hexes not yet reached, green over live ones, nothing once done', () => {
  const omen = { tiles: [{ q: 1, r: 0 }, { q: 2, r: 0 }], start: 1000 };
  const early = omenOverlay(omen, 1010);
  assert.equal(early.length, 2);
  assert.notEqual(early[0].fill, early[1].fill, 'first hex live, second still fog');
  assert.equal(early[1].alpha, 1);
  const fading = omenOverlay(omen, 1000 + OMEN_HOLD_MS + 1);
  assert.ok(fading[0].alpha < early[0].alpha, 'the first hex is fading');
  assert.equal(omenOverlay(omen, 1000 + omenDuration(2)), null);
});
