import { State } from '../state.js';
// Player prose shows plain digits: "5.6", "1,000". Spelled-out numbers read
// slower and hide the size of a quantity (Timothy 2026-10-08). At most one
// decimal; a non-number is "unknown", never "NaN".
export function fmtNum(value) {
  const n = Number(value);
  if (!Number.isFinite(n)) return 'unknown';
  const rounded = Math.round(n * 10) / 10;
  return (Object.is(rounded, -0) ? 0 : rounded).toLocaleString('en-US', { maximumFractionDigits: 1 });
}

// Durations are days of the world; absolute instants use the shared clock formatter.
export function fmtDays(value, qualifier = '') {
  const number = fmtNum(value);
  return number + ' ' + (qualifier ? qualifier + ' ' : '') + (Number(number.replaceAll(',', '')) === 1 ? 'game day' : 'game days');
}

// Real duration is guidance only. Read the world's actual cadence; missing
// cadence leaves the game duration intact rather than guessing alpha speed.
export function fmtDuration(value, tickSeconds = State.TICK_SECONDS, qualifier = '') {
  const days = fmtDays(value, qualifier);
  const count = Number(value), cadence = Number(tickSeconds);
  if (!Number.isFinite(count) || count < 0 || !(cadence > 0) || !Number.isFinite(cadence)) return days;
  const hours = count * cadence / 3600;
  const quantity = hours >= 1 ? hours : hours * 60;
  const unit = hours >= 1 ? 'hour' : 'minute';
  const rounded = Number(quantity.toFixed(2));
  const wall = rounded > 0 || quantity === 0 ? rounded.toLocaleString('en-US', {maximumFractionDigits: 2}) : 'less than 0.01';
  return `${days} · about ${wall} ${unit}${rounded === 1 ? '' : 's'}`;
}
