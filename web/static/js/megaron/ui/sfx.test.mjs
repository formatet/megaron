import test from 'node:test';
import assert from 'node:assert/strict';

const {
  shouldPlay, setSoundMuted, isSoundMuted, playWarHorn, playBattleClash, _resetSfx,
} = await import('./sfx.js');

test('SX1: muting the music mutes the war sounds — one control for all audio', () => {
  _resetSfx();
  assert.equal(shouldPlay('horn', 1000), true);
  setSoundMuted(true);
  assert.equal(isSoundMuted(), true);
  assert.equal(shouldPlay('horn', 99999), false, 'a player who silenced the music does not want a horn');
  setSoundMuted(false);
  assert.equal(shouldPlay('horn', 199999), true);
});

test('SX2: the same sound will not stack on itself within the throttle window', () => {
  _resetSfx();
  assert.equal(shouldPlay('battle', 10_000), true);
  assert.equal(shouldPlay('battle', 10_500), false, 'two battles concluding in one tick must not double-fire');
  assert.equal(shouldPlay('battle', 12_000), true, 'and it recovers once the window passes');
});

test('SX3: the horn and the clash throttle independently — an order during a battle is still heard', () => {
  _resetSfx();
  assert.equal(shouldPlay('battle', 5_000), true);
  assert.equal(shouldPlay('horn', 5_100), true, 'different kinds share no window');
});

test('SX4: a refused play does NOT arm the window — it stays refused, not re-armed by asking', () => {
  _resetSfx();
  assert.equal(shouldPlay('horn', 0), true);
  assert.equal(shouldPlay('horn', 500), false);
  assert.equal(shouldPlay('horn', 1000), false, 'the window still runs from the sound that actually played');
  assert.equal(shouldPlay('horn', 1600), true);
});

test('SX5: with no WebAudio (the Node case, and an old browser) the sounds are silent no-ops, never throws', () => {
  _resetSfx();
  assert.equal(globalThis.AudioContext, undefined, 'precondition: this environment has no WebAudio');
  assert.doesNotThrow(() => playWarHorn(), 'sound is decoration — it must never throw into the game loop');
  assert.doesNotThrow(() => playBattleClash());
});

test('SX6: mute survives across sound kinds and is readable', () => {
  _resetSfx();
  setSoundMuted(true);
  assert.equal(shouldPlay('horn', 1), false);
  assert.equal(shouldPlay('battle', 2), false);
  _resetSfx();
  assert.equal(isSoundMuted(), false, '_resetSfx restores the default for the next test');
});
