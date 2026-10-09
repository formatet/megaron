// Wall-clock dates only: never world durations. Shared without UI dependencies.
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
