import { esc } from '../format.js';
import { fmtNum, fmtDays } from '../fmt_num.js';
import { sitosStateHtml } from './sitos_view.js';
import { State } from '../../state.js';
import { placePanelHTML } from '../placename.js';
const n = value => fmtNum(Math.round(Number(value) * 100) / 100);
const row = (label, value) => `<div class="stat-row"><span class="sr-label">${label}</span><span class="sr-val">${value}</span></div>`;

export function productionSectionsHTML(city) {
  // An owned city opens its drawer instead of the terrain card. Keep the
  // same naming action reachable for the place under its centre hex.
  const centre = (State.tileData || []).find(t => t.q === city.q && t.r === city.r);
  const placeNameForm = centre ? placePanelHTML(centre, {showName:true}) : '';
  return `<div class="dsec"><div class="dsec-title">Population</div><div id="city-pop-sec"></div></div>
    <div class="dsec"><div class="dsec-title">Catchment &amp; workplaces</div>${placeNameForm}<div id="city-prod-sec"></div></div>
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

// Same sentence as keryx `status` (growthLine in cmd_status.go). growth_state
// and food_net_per_tick come from the server's growth gate; '' if not reported.
export function growthLineText(pd) {
  const s = pd?.sitos;
  if (!s || !s.growth_state) return '';
  const pop = n(pd.population);
  const net = Number(s.food_net_per_tick) || 0;
  const signed = net !== 0 && Math.abs(net)<0.05 ? 'about even' : (net >= 0 ? '+' : '-') + Math.abs(net).toFixed(1);
  if (s.growth_state === 'growing') return `Population ${pop} — growing: food net ${signed} per day`;
  if (s.growth_state === 'holding') return `Population ${pop} — not growing: food net ${signed} per day (needs more than 0)`;
  if (s.growth_state === 'shrinking') return `Population ${pop} — shrinking: hunger`;
  return '';
}

export function populationHTML(pd, idle, livestock, provinceID) {
  const g = growthLineText(pd);
  return (g ? `<p class="growth-line">${g}</p>` : '') + row('People', n(pd.population)) + row('Free to work', n(idle)) +
    row('Livestock', n(Math.floor(livestock))) + `<button class="btn-small" onclick="slaughterLivestock('${provinceID}')" ${livestock < 1 ? 'disabled' : ''} title="Trade one animal for 10 people, right now">Slaughter → 10 people</button>` +
    '<div id="city-slaughter-result" class="action-result"></div>';
}

export function harvestBlessingHTML(pd) {
  const b = pd?.harvest_blessing;
  if (!b || !(b.remaining_ticks > 0) || !(b.yield_bonus > 0) || !b.goods?.length) return '';
  const percent = Number((b.yield_bonus * 100).toFixed(6));
  return `<p class="harvest-blessing">Blessed harvest: +${percent}% ${esc(b.goods.join(', '))} yield · ${fmtNum(b.remaining_ticks)} ${b.remaining_ticks === 1 ? 'day' : 'days'} left</p>`;
}

// Coverage is the city's grain+fish stock at current consumption, not a
// depletion forecast including future production, or days bought by reserve.
export function foodSummaryHTML(pd) {
  const blessing = harvestBlessingHTML(pd);
  const s = pd?.sitos;
  if (!s || s.coverage_ticks == null || !Number.isFinite(Number(s.coverage_ticks))) return blessing + '<p class="empty-state">Food stores not reported.</p>';
  const growing = Number(pd.food_net_per_tick_after_upkeep ?? s.food_net_per_tick) > 0;
  const warning = pd.food_self_sufficient === false ? ' · the fields cannot feed everyone' : '';
  return blessing + `<p${warning ? ' class="stat-warn"' : ''} title="Current city food at today’s consumption; reserve details are under More.">Food lasts ${fmtDays(s.coverage_ticks)}${growing ? ' · stocks growing' : ''}${warning}.</p>`;
}

export function foodDetailsHTML(pd) {
  const s = pd?.sitos;
  const peopleNet = s?.food_net_per_tick;
  const grain = pd?.grain_prod_rate != null ? row('Food for people', `grain ${n(pd.grain_prod_rate)} + fish ${n(pd.resources?.fish?.rate || 0)} − people eat ${n(pd.grain_consum_rate || 0)}${peopleNet != null ? ` = ${n(peopleNet)} net before army` : ''}`) : '';
  const armyReserve = (pd.army_upkeep?.grain || 0) > 0 && (s?.granary_per_good?.grain || 0) > 0 ? ` · ${n(s.granary_per_good.grain)} grain in this city's Sitos can feed the army` : '';
  const army = pd?.food_net_per_tick_after_upkeep == null ? '' : row('Army food', `eats ${n(pd.army_upkeep?.grain || 0)} grain per day → food net ${n(pd.food_net_per_tick_after_upkeep)} after army${armyReserve}`);
  if (!s) return grain || '<p class="empty-state">Food reserve not reported.</p>';
  const perGood = Object.entries(s.granary_per_good || {}).filter(([,v])=>v>0).map(([k,v])=>`${n(v)} ${k}`).join(', ');
  const note = pd.food_gubbar_required == null ? '' : row('Food citizens', `${n(pd.food_gubbar_required)} needed · ${n(pd.food_gubbar_placed || 0)} placed`) + '<p class="empty-state">1 citizen = 100 people. Estimate uses the best food slots for people; the army needs grain separately.</p>';
  const a = pd.automatic_food;
  const automatic = a ? `<p class="empty-state">Automatic food: ${Number(a.nearjord_grain_per_tick + a.remainder_grain_per_tick).toFixed(2)} grain + ${Number(a.remainder_fish_per_tick || 0).toFixed(2)} fish per day without placed citizens (nearby ground ${Number(a.nearjord_grain_per_tick).toFixed(2)} grain; ${a.remainder_people} people outside whole citizens: ${Number(a.remainder_grain_per_tick).toFixed(2)} grain + ${Number(a.remainder_fish_per_tick || 0).toFixed(2)} fish, a fraction of the best food worker).</p>` : '';
  return grain + army + note + automatic + row('City stocks', `${fmtDays(s.coverage_ticks)}`) +
    row('Reserve', `${n(s.granary_total || 0)} of ${n(s.granary_cap || 0)} food${perGood ? ' · '+perGood : ''}`) +
    row('Reserve rules', `Stores above ${fmtDays(s.high_ticks)} · releases below ${fmtDays(s.low_ticks)}`) +
    sitosStateHtml(s).replace('coverage is rising','food lasts longer');
}
