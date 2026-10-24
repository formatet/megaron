import test from 'node:test';
import assert from 'node:assert/strict';
import { omenPhase, omenOverlay, OMEN_STEP_MS, OMEN_HOLD_MS, OMEN_FADE_MS, omenDuration } from './omen.js';

// Byggplan D.2 (Timothy's bug report): an answered prayer that shows a place
// lifts the fog to live colour one hex at a time, ~0.3 s apart, then fades it
// to memory grey in steps.
test('each revealed hex waits its turn, shows live, fades in steps, then is memory', () => {
  assert.equal(OMEN_STEP_MS, 300);
  assert.equal(omenPhase(2, 3 * OMEN_STEP_MS - 1).phase, 'hidden');
  assert.equal(omenPhase(2, 3 * OMEN_STEP_MS).phase, 'live');
  const f1 = omenPhase(0, OMEN_STEP_MS + OMEN_HOLD_MS);
  const f2 = omenPhase(0, OMEN_STEP_MS + OMEN_HOLD_MS + OMEN_FADE_MS * 0.7);
  assert.equal(f1.phase, 'fade');
  assert.ok(f1.alpha > 0 && f1.alpha < f2.alpha && f2.alpha < 1, `stepped fade ${f1.alpha} → ${f2.alpha}`);
  assert.equal(omenPhase(0, OMEN_STEP_MS + OMEN_HOLD_MS + OMEN_FADE_MS).phase, 'memory');
  assert.equal(omenDuration(3), 3 * OMEN_STEP_MS + OMEN_HOLD_MS + OMEN_FADE_MS);
});

test('overlay never paints hidden hexes or a replacement fog', () => {
  const omen = { tiles: [{ q: 1, r: 0 }, { q: 2, r: 0 }], start: 1000 };
  assert.deepEqual(omenOverlay(omen, 1010), []);
  const live = omenOverlay(omen, 1000 + OMEN_STEP_MS);
  assert.equal(live.length, 1);
  assert.equal(live[0].q, 1);
  assert.ok(live[0].alpha < 1);
  assert.equal(omenOverlay(omen, 1000 + omenDuration(2)), null);
});

test('ordinary map fog survives until its turn; known tiles never animate; polls cannot reveal pending tiles', async () => {
  const { beginOmen, omenMap } = await import('./omen.js');
  const before = [{ q: 0, r: 0, terrain: 'plains', tier: 'live' }, { q: 1, r: 0, terrain: 'fog' }, { q: 0, r: 1, terrain: 'hills', tier: 'memory' }];
  const tiles = [{ q: 0, r: 0 }, { q: 1, r: 0 }, { q: 2, r: 0 }, { q: 0, r: 1 }, { q: 1, r: 0 }];
  const after = [...before.filter(t => t.terrain !== 'fog'), { q: 1, r: 0, terrain: 'plains', tier: 'memory' }, { q: 2, r: 0, terrain: 'hills', tier: 'memory' }];
  const omen = beginOmen(before, tiles, { q: 1, r: 0 }, 1000);
  assert.deepEqual(omen.tiles, [{ q: 1, r: 0 }, { q: 2, r: 0 }]);
  assert.equal(omenMap(omen, after, 1000), before, 'zero is the exact original tile array');
  assert.equal(omenMap(omen, structuredClone(after), 1299), before, 'poll cannot expose pending discovery');
  const first = omenMap(omen, after, 1300);
  assert.equal(first.find(t => t.q === 1 && t.r === 0).terrain, 'plains');
  assert.ok(!first.some(t => t.q === 2), 'absent original hex remains absent');
  assert.equal(first.find(t => t.q === 0 && t.r === 1), before[2], 'known memory tile preserved');
  assert.equal(omenMap(omen, after, 1399), first, 'no per-frame tile-array invalidation');
  const all = omenMap(omen, after, 1600);
  assert.deepEqual(all, after);
  assert.equal(omenMap(omen, after, 1700), all);
});


test('an already known prayer patch keeps normal map refreshes and has no reveal', async () => {
  const { beginOmen, omenMap } = await import('./omen.js');
  const before = [{ q: 1, r: 0, terrain: 'hills', tier: 'remembered' }];
  const refreshed = [{ q: 1, r: 0, terrain: 'hills', tier: 'live' }];
  const omen = beginOmen(before, [{ q: 1, r: 0 }], null, 1000);
  assert.deepEqual(omen.tiles, []);
  assert.equal(omenMap(omen, refreshed, 1000), refreshed);
});
