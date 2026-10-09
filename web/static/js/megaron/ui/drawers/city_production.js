import { fmtNum, fmtDays } from '../fmt_num.js';
import { sitosStateHtml } from './sitos_view.js';
const n = value => fmtNum(Math.round(Number(value) * 100) / 100);
const row = (label, value) => `<div class="stat-row"><span class="sr-label">${label}</span><span class="sr-val">${value}</span></div>`;

export function productionSectionsHTML(city) {
  return `<div class="dsec"><div class="dsec-title">Population</div><div id="city-pop-sec"></div></div>
    <div class="dsec"><div class="dsec-title">Catchment &amp; workplaces</div><div id="city-prod-sec"></div></div>
    <div class="dsec"><div class="dsec-title">Food</div><div id="city-sitos-sec"></div></div>
    <details id="city-more" class="dsec"><summary class="dsec-title">More</summary>
      <div class="dsec-title">People at work</div><div id="city-people-details"></div>
      <div class="dsec-title">Devotion</div><div id="city-devotion-sec"></div>
      <div class="dsec-title">Food reserve</div><div id="city-reserve-sec"></div>
      <div class="dsec-title">Last day</div><div id="city-lasttick-sec"></div>
      <div class="dsec-title">Loyalty log</div><div id="city-loyalty-sec"></div>
      ${!city.is_capital ? '<div class="dsec-title">Gift from capital</div><div id="city-gift-sec"></div>' : ''}
      <div class="dsec-title">Daily history <button class="btn-small" onclick="loadTicklog()">Show recent days</button></div><div id="city-ticklog-sec"></div>
    </details>`;
}

export function populationHTML(pd, idle, livestock, provinceID) {
  return row('People', n(pd.population)) + row('Free to work', n(idle)) +
    row('Livestock', n(Math.floor(livestock))) + `<button class="btn-small" onclick="slaughterLivestock('${provinceID}')" ${livestock < 1 ? 'disabled' : ''} title="Trade one animal for 10 people, right now">Slaughter → 10 people</button>` +
    '<div id="city-slaughter-result" class="action-result"></div>';
}

// Coverage is the city's grain+fish stock at current consumption, not a
// depletion forecast including future production, or days bought by reserve.
export function foodSummaryHTML(pd) {
  const s = pd?.sitos;
  if (!s || s.coverage_ticks == null || !Number.isFinite(Number(s.coverage_ticks))) return '<p class="empty-state">Food stores not reported.</p>';
  const growing = Number(s.food_net_per_tick) > 0;
  const warning = pd.food_self_sufficient === false ? ' · the fields cannot feed everyone' : '';
  return `<p${warning ? ' class="stat-warn"' : ''} title="Current city food at today’s consumption; reserve details are under More.">Food lasts ${fmtDays(s.coverage_ticks)}${growing ? ' · stocks growing' : ''}${warning}.</p>`;
}

export function foodDetailsHTML(pd) {
  const s = pd?.sitos;
  const grain = pd?.grain_prod_rate != null ? row('Grain per day', `${n(pd.grain_prod_rate)} made · ${n(pd.grain_consum_rate || 0)} eaten · ${n((pd.grain_prod_rate || 0) - (pd.grain_consum_rate || 0))} net`) : '';
  if (!s) return grain || '<p class="empty-state">Food reserve not reported.</p>';
  const perGood = Object.entries(s.granary_per_good || {}).filter(([,v])=>v>0).map(([k,v])=>`${n(v)} ${k}`).join(', ');
  const note = pd.food_gubbar_required == null ? '' : row('Food workers', `${n(pd.food_gubbar_required)} needed · ${n(pd.food_gubbar_placed || 0)} placed`);
  return grain + note + row('City stocks', `${fmtDays(s.coverage_ticks)}`) +
    row('Reserve', `${n(s.granary_total || 0)} of ${n(s.granary_cap || 0)} food${perGood ? ' · '+perGood : ''}`) +
    row('Reserve rules', `Stores above ${fmtDays(s.high_ticks)} · releases below ${fmtDays(s.low_ticks)}`) +
    sitosStateHtml(s).replace('coverage is rising','food lasts longer');
}
