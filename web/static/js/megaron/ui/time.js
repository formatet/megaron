// ── Time presentation (Tid & kalender Fas B) ───────────────────────────────
// One module for every "when does it happen" string in the client. Consumes
// the K4 tick-contract: `arrival_tick` is the AUTHORITATIVE arrival time
// (temenos_tid_kalender_plan §K4); wall-clock ISO stamps are derived
// conveniences that go stale across server downtime (the world pauses, the
// stored stamp does not). Callers pass both when they have both — the tick
// wins, the ISO is the fallback for payloads that carry no tick yet.
//
// Layer note: imports clock.js (low) and state.js (bottom) only, so any
// module from api/ws upward may import it.
import { serverNow } from '../clock.js';
import { State } from '../state.js';
import { esc } from './format.js';
import { fmtClock } from './fmt_clock.js';
export { fmtClock } from './fmt_clock.js';

// Milliseconds until an instant. Tick path: estimate the world's current tick
// from the bootstrap anchor (State.CURRENT_TICK at State.TICK_ANCHOR_MS,
// advancing at TICK_SECONDS per tick) and convert the remaining ticks —
// self-correcting across tempo shifts and downtime every time the anchor is
// refreshed (bootstrap + WS reconnect). ISO path: plain diff against server
// time (clock.js skew-anchored).
export function msUntil(iso, arrivalTick) {
  if (arrivalTick != null && State.TICK_SECONDS > 0 && State.TICK_ANCHOR_MS != null) {
    const nowTick = State.CURRENT_TICK
      + (serverNow() - State.TICK_ANCHOR_MS) / (State.TICK_SECONDS * 1000);
    return (arrivalTick - nowTick) * State.TICK_SECONDS * 1000;
  }
  if (iso == null || iso === '') return NaN;
  return new Date(iso).getTime() - serverNow();
}

// Wall-clock estimate or doneWord, sharing the same date context as arrivals.
// `doneWord` names what "the instant has passed" means to THIS caller — a
// march or messenger has "arrived" (the default), but a finished build has
// not; it is "ready". Pass it, don't reinterpret the string downstream.
export function fmtEta(iso, arrivalTick, doneWord = 'arrived') {
  const ms = msUntil(iso, arrivalTick);
  if (!Number.isFinite(ms)) return '';
  if (ms <= 0) return doneWord;
  return '≈ ' + fmtClock(Date.now() + ms);
}

// Local clock time with date context — never a bare "19:00" that lies across
// midnight (Fas B rule 2). "today 21:14" / "tomorrow 08:12" / "Fri 21:14" /
// "18 Jul 21:14". Formatting is the player's locale; the input is an epoch ms
// in the player's frame.
// Arrival clock/date. The remaining time still uses the authoritative tick
// anchor, projected into the player's clock frame (Date.now() + ms).
export function fmtArrival(iso, arrivalTick, doneWord = 'arrived') {
  const ms = msUntil(iso, arrivalTick);
  if (!Number.isFinite(ms)) return '';
  if (ms <= 0) return doneWord;
  return '≈ ' + fmtClock(Date.now() + ms);
}

// fmtArrival wrapped in a span whose hover title spells out the full instant
// with an explicit timezone (Fas B rule 1: hover = explicit tidszon). For
// innerHTML call sites.
export function arrivalHTML(iso, arrivalTick, doneWord = 'arrived') {
  const ms = msUntil(iso, arrivalTick);
  if (!Number.isFinite(ms)) return '';
  if (ms <= 0) return doneWord;
  const full = new Date(Date.now() + ms).toLocaleString([], {
    weekday: 'short', year: 'numeric', month: 'short', day: 'numeric',
    hour: '2-digit', minute: '2-digit', timeZoneName: 'short',
  });
  return `<span title="${esc(full)}">${esc(fmtArrival(iso, arrivalTick, doneWord))}</span>`;
}
