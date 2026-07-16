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
  return new Date(iso).getTime() - serverNow();
}

// Short relative duration: "3h 18m" / "42m" / "arrived". The compact form for
// tight cells and expiry countdowns (moved here from ui/format.js fmtEta).
export function fmtEta(iso, arrivalTick) {
  const ms = msUntil(iso, arrivalTick);
  if (ms <= 0) return 'arrived';
  const h = Math.floor(ms / 3600000), m = Math.floor((ms % 3600000) / 60000);
  return h > 0 ? `${h}h ${m}m` : `${m}m`;
}

// Local clock time with date context — never a bare "19:00" that lies across
// midnight (Fas B rule 2). "today 21:14" / "tomorrow 08:12" / "Fri 21:14" /
// "18 Jul 21:14". Formatting is the player's locale; the input is an epoch ms
// in the player's frame.
export function fmtClock(epochMs) {
  const at = new Date(epochMs);
  const hm = at.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  const now = new Date();
  const dayDiff = Math.round(
    (new Date(at.getFullYear(), at.getMonth(), at.getDate())
     - new Date(now.getFullYear(), now.getMonth(), now.getDate())) / 86400000);
  if (dayDiff === 0) return `today ${hm}`;
  if (dayDiff === 1) return `tomorrow ${hm}`;
  if (dayDiff > 1 && dayDiff < 7) {
    return `${at.toLocaleDateString([], { weekday: 'short' })} ${hm}`;
  }
  return `${at.toLocaleDateString([], { day: 'numeric', month: 'short' })} ${hm}`;
}

// Arrival line: primary local clock, secondary relative — "today 21:14 · in
// 3h 18m". The remaining duration is measured against server time, then
// projected into the player's clock frame for display (Date.now() + ms), so a
// skewed player clock still reads its own local time correctly.
export function fmtArrival(iso, arrivalTick) {
  const ms = msUntil(iso, arrivalTick);
  if (ms <= 0) return 'arrived';
  return `${fmtClock(Date.now() + ms)} · in ${fmtEta(iso, arrivalTick)}`;
}

// fmtArrival wrapped in a span whose hover title spells out the full instant
// with an explicit timezone (Fas B rule 1: hover = explicit tidszon). For
// innerHTML call sites.
export function arrivalHTML(iso, arrivalTick) {
  const ms = msUntil(iso, arrivalTick);
  if (ms <= 0) return 'arrived';
  const full = new Date(Date.now() + ms).toLocaleString([], {
    weekday: 'short', year: 'numeric', month: 'short', day: 'numeric',
    hour: '2-digit', minute: '2-digit', timeZoneName: 'short',
  });
  return `<span title="${esc(full)}">${esc(fmtArrival(iso, arrivalTick))}</span>`;
}
