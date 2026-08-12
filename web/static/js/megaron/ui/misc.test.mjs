import test from 'node:test';
import assert from 'node:assert/strict';
import { State } from '../state.js';
import { currentCalendarDate, monthLabel } from './misc.js';

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
