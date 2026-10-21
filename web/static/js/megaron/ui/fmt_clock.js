import { msUntilWorldTick } from '../world_time.js';
import { serverNow } from '../clock.js';
// Wall-clock dates only: never world durations. Shared without UI dependencies.
export function fmtClock(epochMs) {
  const at = new Date(epochMs);
  const hm = at.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hourCycle: 'h23' });
  const now = new Date();
  const dayDiff = Math.round(
    (new Date(at.getFullYear(), at.getMonth(), at.getDate())
     - new Date(now.getFullYear(), now.getMonth(), now.getDate())) / 86400000);
  if (dayDiff === 0) return `today ${hm}`;
  if (dayDiff === -1) return `yesterday ${hm}`;
  if (dayDiff === 1) return `tomorrow ${hm}`;
  if (Math.abs(dayDiff) < 7) {
    return `${at.toLocaleDateString([], { weekday: 'short' })} ${hm}`;
  }
  return `${at.toLocaleDateString([], { day: 'numeric', month: 'short', ...(at.getFullYear() !== now.getFullYear() ? { year: 'numeric' } : {}) })} ${hm}`;
}

// Absolute world instants use the current world anchor, including past ticks.
// Unlike arrival countdowns, a historical instant must never become "arrived".
export function fmtTickTime(value, iso) {
  const delta = Number.isInteger(value) && value >= 0 ? msUntilWorldTick(value) : NaN;
  if (Number.isFinite(delta)) return fmtClock(Date.now() + delta);
  const at = iso ? Date.parse(iso) : NaN;
  return Number.isFinite(at) ? fmtClock(Date.now() + at - serverNow()) : 'time unavailable';
}
