import { fmtTickTime } from './fmt_clock.js';
// Registered game time takes priority over a notification's delivery timestamp.
function notificationTick(n) {
  const body = typeof n.body === 'string' ? JSON.parse(n.body) : (n.body || n.payload || {});
  const tick = n.world_tick ?? body.world_tick;
  return Number.isInteger(tick) && tick >= 0 ? tick : null;
}
export function notificationDay(n) {
  const tick = notificationTick(n);
  return fmtTickTime(tick);
}
function notificationBody(n) {
  return typeof n.body === 'string' ? JSON.parse(n.body) : (n.body || {});
}
function compareNotifications(a, b) {
  const at = notificationTick(a), bt = notificationTick(b);
  if (at != null && bt != null && at !== bt) return bt - at;
  return (Date.parse(b.created_at) || 0) - (Date.parse(a.created_at) || 0);
}
// Only the latest recorded transition for each city can still need action.
// A recovery keeps its historical start readable without pinning it forever.
export function urgentNotifications(notifs) {
  const urgent = new Set(), seen = new Set();
  [...notifs].sort(compareNotifications).forEach(n => {
    const b = notificationBody(n);
    if (n.kind === 'SubsistenceWarning' && b.tier === 'critical') urgent.add(n);
    if (!['GarrisonStarvationStarted', 'GarrisonStarvationEnded', 'CivilianStarvationStarted', 'CivilianStarvationEnded'].includes(n.kind)) return;
    const scope = b.settlement_id || b.province_id;
    if (!scope) return;
    const key = (n.kind.startsWith('Civilian') ? 'civilian:' : 'garrison:') + scope;
    if (seen.has(key)) return;
    seen.add(key);
    if (n.kind.endsWith('Started')) urgent.add(n);
  });
  return urgent;
}
