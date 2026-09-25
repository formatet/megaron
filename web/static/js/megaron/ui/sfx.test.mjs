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

test('SX-A1: arrivals get the arrival call, other dispatches a chime, own-sound kinds nothing extra', async () => {
  const { soundForKind } = await import('./sfx.js');
  assert.equal(soundForKind('UnitArrived'), 'arrival');
  assert.equal(soundForKind('UnitExploreReturned'), 'arrival');
  assert.equal(soundForKind('MessengerReturned'), 'arrival');
  assert.equal(soundForKind('FoodShortfall'), 'chime');
  assert.equal(soundForKind('TradeDelivery'), 'chime');
  assert.equal(soundForKind('BattleWon'), null, 'the clash already plays — no chime on top');
  assert.equal(soundForKind('UnitRedirected'), null, 'the horn already plays');
  assert.equal(soundForKind(undefined), null);
});

test('SX-A2: the chime and the arrival call are throttled independently and obey mute', () => {
  _resetSfx();
  assert.equal(shouldPlay('chime', 1000), true);
  assert.equal(shouldPlay('chime', 1500), false, 'a burst of dispatches is one chime, not a stack');
  assert.equal(shouldPlay('arrival', 1500), true, 'an arrival is not silenced by a chime');
  setSoundMuted(true);
  assert.equal(shouldPlay('chime', 99999), false);
  setSoundMuted(false);
});

// ── Music cues (Timothy 2026-09-25) ─────────────────────────────────────────
test('SX-M1: musicCueFor routes ForeignMarchSighted to war only when it threatens a settlement', async () => {
  const { musicCueFor } = await import('./sfx.js');
  assert.equal(musicCueFor('ForeignMarchSighted', { threatens_settlement_id: 42 }), 'war');
  assert.equal(musicCueFor('ForeignMarchSighted', {}), null, 'sighted elsewhere on the map is not this player\'s war');
  assert.equal(musicCueFor('ForeignMarchSighted', null), null, 'no payload at all is not a threat either');
});

test('SX-M2: musicCueFor routes capture/occupation by role', async () => {
  const { musicCueFor } = await import('./sfx.js');
  assert.equal(musicCueFor('SettlementCaptured', { role: 'attacker' }), 'victory');
  assert.equal(musicCueFor('SettlementCaptured', { role: 'defender' }), 'doom');
  assert.equal(musicCueFor('CityOccupied', { role: 'attacker' }), 'victory');
  assert.equal(musicCueFor('CityOccupied', { role: 'defender' }), 'doom');
  assert.equal(musicCueFor('SettlementCaptured', { role: 'bystander' }), null);
});

test('SX-M3: everything else — including BattleWon/BattleLost, which keep only their clash SFX — routes to no cue', async () => {
  const { musicCueFor } = await import('./sfx.js');
  assert.equal(musicCueFor('BattleWon', { role: 'attacker' }), null);
  assert.equal(musicCueFor('BattleLost', {}), null);
  assert.equal(musicCueFor('UnitArrived', {}), null);
  assert.equal(musicCueFor(undefined, {}), null);
});
