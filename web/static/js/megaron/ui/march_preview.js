import { esc, formatApiError } from './format.js';
import { fmtNum, fmtDays, fmtDay } from './fmt_num.js';
import { arrivalHTML } from './time.js';

// Forecasts are read-only. Keep each unit separate: crew and cargo can make
// two ships in the same selection travel at different speeds.
export async function loadMarchPreview(worldID, pick, order, read) {
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(order)) {
    if (value !== undefined && value !== '') query.set(key, String(value));
  }
  const response = await read(`/api/v1/worlds/${worldID}/units/${pick.id}/march-preview?${query}`);
  const data = await response.json();
  if (!response.ok) throw new Error(formatApiError(data, 'Forecast unavailable'));
  return data;
}

export function marchSilverLine(data) {
  if (!Number.isFinite(data.upkeep_silver_home) || !Number.isFinite(data.upkeep_silver_field)
      || !Object.hasOwn(data, 'silver_runway_ticks')) return '';
  const rate = value => value.toLocaleString('en-US', {maximumFractionDigits: 4});
  const runway = data.silver_runway_ticks === null ? 'no silver drain'
    : Number.isFinite(data.silver_runway_ticks) ? (data.silver_payer_count > 1 ? 'shortest treasury lasts ' : 'treasury lasts ') + fmtDays(data.silver_runway_ticks)
    : 'treasury duration unknown';
  return 'Pay: ' + rate(data.upkeep_silver_home) + ' → ' + rate(data.upkeep_silver_field)
    + ' silver per day in the field · ' + runway;
}

export function marchExpeditionPlanLine(data) {
 if (!Number.isInteger(data.planned_expedition_ticks)) return '';
 const home = Number.isInteger(data.planned_home_by_tick_if_departing_now)
  ? ` — with immediate departure, home by ${fmtDay(data.planned_home_by_tick_if_departing_now)}` : '';
 return `Planned expedition: ${fmtDays(data.planned_expedition_ticks)}${home}. First-leg arrival remains unknown.`;
}
function marchArrivalHTML(data) {
 const plan = marchExpeditionPlanLine(data);
 return marchFirstArrivalHTML(data) + (plan ? '<div>' + esc(plan) + '</div>' : '');
}
function marchFirstArrivalHTML(data) {
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

export function marchPreviewHTML(data) {
  const sold = marchSilverLine(data);
  return marchArrivalHTML(data) + (sold ? '<div class="march-sold">' + esc(sold) + '</div>' : '');
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
        const selectedIDs = picks.map(pick => pick.id).join(',');
        const forecasts = await Promise.all(picks.map(async pick => {
          try {
            const data = await loadMarchPreview(worldID, pick, {...order, ...(pick.order || {}), unit_ids: selectedIDs}, read);
            return {html: '<div>' + esc(pick.name) + ': ' + marchArrivalHTML(data) + '</div>', data};
          } catch (error) {
            return {html: '<div>' + esc(pick.name) + ': Forecast unavailable: ' + esc(error.message) + '</div>'};
          }
        }));
        const finance = forecasts.find(forecast => forecast.data && marchSilverLine(forecast.data));
        const sold = finance ? '<div class="march-sold">' + esc(marchSilverLine(finance.data)) + '</div>' : '';
        if (current === generation) render(forecasts.map(forecast => forecast.html).join('') + sold);
      }, delay);
    },
  };
}
