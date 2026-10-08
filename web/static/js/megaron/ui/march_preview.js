import { esc, formatApiError } from './format.js';
import { fmtNum, fmtDays } from './fmt_num.js';
import { arrivalHTML } from './time.js';

// Forecasts are read-only. Keep each unit separate: crew and cargo can make
// two ships in the same selection travel at different speeds.
export async function loadMarchPreview(worldID, pick, order, read) {
  if (pick.mode === 'redirect') return { available: false, reason: 'redirect' };
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(order)) {
    if (value !== undefined && value !== '') query.set(key, String(value));
  }
  const response = await read(`/api/v1/worlds/${worldID}/units/${pick.id}/march-preview?${query}`);
  const data = await response.json();
  if (!response.ok) throw new Error(formatApiError(data, 'Forecast unavailable'));
  return data;
}

export function marchPreviewHTML(data) {
  if (data.available && Number.isInteger(data.arrival_tick)) {
    return 'Estimated arrival: ' + arrivalHTML(data.arrives_at_utc, data.arrival_tick)
      + ' · ' + esc(fmtDays(data.duration_ticks)) + ' travelling';
  }
  if (data.reason === 'courier_required' || data.reason === 'redirect') {
    return 'Arrival not yet known — a messenger must deliver the order first.';
  }
  if (data.reason === 'unknown_terrain') return 'Arrival not yet known — unexplored terrain.';
  return 'Arrival forecast unavailable.';
}

// Invalidate immediately on every edit, close or send, including while the
// next request is debounced. A late response must never overwrite a new order.
export function createMarchPreview(render, read, delay = 180) {
  let generation = 0;
  let timer;
  return {
    cancel() { generation++; clearTimeout(timer); },
    update(worldID, picks, order) {
      const current = ++generation;
      clearTimeout(timer);
      if (!picks.length) { render(''); return; }
      render('Estimating arrival…');
      timer = setTimeout(async () => {
        const rows = await Promise.all(picks.map(async pick => {
          let line;
          try { line = marchPreviewHTML(await loadMarchPreview(worldID, pick, order, read)); }
          catch (error) { line = 'Forecast unavailable: ' + esc(error.message); }
          return '<div>' + esc(pick.name) + ': ' + line + '</div>';
        }));
        if (current === generation) render(rows.join(''));
      }, delay);
    },
  };
}
