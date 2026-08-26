import test from 'node:test';
import assert from 'node:assert/strict';

// kult.js pulls in ui/misc.js (renderLockedActions), which runs
// `document.addEventListener(...)` at module top level (music-player
// autostart) — same problem documented in cargo.test.mjs/economy.test.mjs.
// Stub the one global it touches, then dynamically import.
globalThis.document ??= { addEventListener() {} };
const { cultLevelLabel } = await import('./kult.js');

// The server never runs a five-tier cult-level model (forsummad/enkel/vardig/
// praktfull/overdadig) — internal/kharis/tick.go's deriveMood "replaces
// player-set cult_level" and writes ONE of four mood strings into the same
// column, in Swedish: tveksam/vardig/overdadig/vredgad (plus 'enkel' as the
// settlement.go fallback before a player's first daily tick has ever run).
// 'forsummad' and 'praktfull' are dead values the server can never send.

test('AK1: vredgad (Wrathful mood) renders an English label, not the raw Swedish string', () => {
  assert.equal(cultLevelLabel('vredgad'), 'Wrathful');
});

test('AK1b: tveksam (Suspicious mood) renders an English label, not the raw Swedish string', () => {
  assert.equal(cultLevelLabel('tveksam'), 'Suspicious');
});

test('AK2: vardig and overdadig — already-correct-looking values — still translate correctly', () => {
  assert.equal(cultLevelLabel('vardig'), 'Indifferent');
  assert.equal(cultLevelLabel('overdadig'), 'Favorable');
});

test('AK3: enkel (pre-first-tick fallback) gets an honest label, not a fake tier name', () => {
  assert.equal(cultLevelLabel('enkel'), 'Not yet assessed');
});

test('AK4: a value the server never sends still falls back to itself, not a fabricated label', () => {
  assert.equal(cultLevelLabel('some_unknown_value'), 'some_unknown_value');
});
