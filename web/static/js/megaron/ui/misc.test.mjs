import test from 'node:test';
import assert from 'node:assert/strict';
import { State } from '../state.js';
import { currentCalendarDate, monthLabel, shouldPlayCue } from './misc.js';

// The notifications drawer's date header only showed the month NAME ("Day 6
// of the Olive, Year 1") — with no ordinal there is no way to count days
// between two notifications, which is exactly what the asynchronicity gate
// needs (megaron_arbetssatt.md: a Wanax back after nine hours must be able to
// read what happened and when). monthLabel appends the month's 1..12 ordinal;
// the intercalary Shadow Days (month 0) sit outside that numbered cycle and
// must NOT gain a misleading "(0)".
//
// state.js/misc.js have no top-level DOM side effects (only inside functions
// called later), so this file imports them directly — no globalThis.document
// stub needed, unlike marchctx.test.mjs/cargo.test.mjs which pull in
// render/map.js's top-level listeners.

function setTick(tick) {
  // TICK_SECONDS large enough that wall-clock jitter between this call and
  // the currentCalendarDate() call below never crosses a tick boundary.
  State.CURRENT_TICK = tick;
  State.TICK_SECONDS = 600;
  State.TICK_ANCHOR_MS = Date.now();
}

test('AK1: monthLabel appends the 1..12 ordinal for an ordinary month', () => {
  setTick(65); // dayOfYear 65 -> month 3 ("the Olive"), day 6
  const cal = currentCalendarDate();
  assert.equal(cal.month, 3);
  assert.equal(cal.monthName, 'the Olive');
  assert.equal(monthLabel(cal), 'the Olive (3)');
});

test('AK2: monthLabel gives the intercalary Shadow Days no ordinal', () => {
  setTick(362); // dayOfYear 362 -> month 0 (Shadow Days), day 3
  const cal = currentCalendarDate();
  assert.equal(cal.month, 0);
  assert.equal(cal.monthName, 'the Shadow Days of the Goddess');
  assert.equal(monthLabel(cal), 'the Shadow Days of the Goddess');
  assert.doesNotMatch(monthLabel(cal), /\(0\)/);
});

test('AK3 (regression): the notification drawer date header carries the ordinal for a normal month', () => {
  setTick(65);
  const cal = currentCalendarDate();
  const header = `Day ${cal.day} of ${monthLabel(cal)}, Year ${cal.year}`;
  assert.equal(header, 'Day 6 of the Olive (3), Year 1');
});

// ── Music cue priority/throttle (Timothy 2026-09-25) ───────────────────────
// shouldPlayCue is MusicPlayer.cue()'s whole decision, pure so it can be
// tested without an Audio element (same reasoning as ui/sfx.js's shouldPlay).

test('MC1: nothing playing — a cue is allowed', () => {
  const ok = shouldPlayCue('war', { now: 1000, muted: false, started: true, activeCue: null, lastPlayedAt: {} });
  assert.equal(ok, true);
});

test('MC2: a higher-priority cue replaces a lower one (doom > war > victory)', () => {
  const base = { now: 1000, muted: false, started: true, lastPlayedAt: {} };
  assert.equal(shouldPlayCue('doom', { ...base, activeCue: 'war' }), true, 'doom outranks war');
  assert.equal(shouldPlayCue('war', { ...base, activeCue: 'victory' }), true, 'war outranks victory');
});

test('MC3: a lower-or-equal-priority cue is dropped while one is already playing', () => {
  const base = { now: 1000, muted: false, started: true, lastPlayedAt: {} };
  assert.equal(shouldPlayCue('victory', { ...base, activeCue: 'war' }), false, 'victory does not outrank war');
  assert.equal(shouldPlayCue('war', { ...base, activeCue: 'war' }), false, 'a cue cannot replace itself');
  assert.equal(shouldPlayCue('war', { ...base, activeCue: 'doom' }), false, 'nothing outranks doom');
});

test('MC4: the same cue name within 10 minutes is dropped, and allowed again after', () => {
  const base = { muted: false, started: true, activeCue: null, lastPlayedAt: { war: 100_000 } };
  assert.equal(shouldPlayCue('war', { ...base, now: 100_000 + 5 * 60_000 }), false, 'a returning player must not get three war cues for three sightings');
  assert.equal(shouldPlayCue('war', { ...base, now: 100_000 + 10 * 60_000 }), true, 'the throttle window has passed');
});

test('MC5: muted, or not yet started (no user gesture), drops the cue', () => {
  const base = { now: 1000, activeCue: null, lastPlayedAt: {} };
  assert.equal(shouldPlayCue('war', { ...base, muted: true, started: true }), false);
  assert.equal(shouldPlayCue('war', { ...base, muted: false, started: false }), false);
});
