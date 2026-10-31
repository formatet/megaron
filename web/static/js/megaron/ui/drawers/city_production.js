import { fmtTickTime } from '../fmt_clock.js';
import { foodGoodsText } from '../food.js';
import { esc } from '../format.js';
import { fmtNum, fmtDays, fmtDuration } from '../fmt_num.js';
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
    <div class="dsec"><div class="dsec-title">Food</div><div id="city-food-sec"></div></div>
    <details id="city-more" class="dsec"><summary class="dsec-title">More</summary>
      <div class="dsec-title">People at work</div><div id="city-people-details"></div>
      <div class="dsec-title">Food details</div><div id="city-food-details-sec"></div>
      <div class="dsec-title">Last game day</div><div id="city-lasttick-sec"></div>
      <div class="dsec-title">Daily history <button class="btn-small" onclick="loadTicklog()">Show recent game days</button></div><div id="city-ticklog-sec"></div>
    </details>`;
}

// Same sentence as keryx `status` (growthLine in cmd_status.go). growth_state
// and food_net_per_tick come from the server's growth gate; '' if not reported.
function growthLineText(pd, includePopulation = true) {
  const s = pd?.food;
  if (!s || !s.growth_state) return '';
  const prefix = includePopulation ? `Population ${n(pd.population)} — ` : '';
  const net = Number(s.food_net_per_tick) || 0;
  const signed = net !== 0 && Math.abs(net)<0.05 ? 'about even' : (net >= 0 ? '+' : '-') + Math.abs(net).toFixed(1);
  if (s.growth_state === 'growing') return `${prefix}growing: food net ${signed} per game day`;
  if (s.growth_state === 'holding') return `${prefix}not growing: food net ${signed} per game day (needs more than 0)`;
  if (s.growth_state === 'capped') return `${prefix}not growing: the walls are full`;
  if (s.growth_state === 'shrinking') return `${prefix}shrinking: hunger`;
  return '';
}

// Progress and forecast are server-owned; the client only formats their scale.
export function nextCitizenText(pd) {
  const progress = pd?.next_citizen_progress;
  if (!Number.isFinite(progress)) return '';
  const state = pd?.food?.growth_state;
  let ending = ({holding:'not growing',capped:'the walls are full',shrinking:'shrinking'})[state];
  const ticks = pd.next_citizen_in_ticks;
  if (!ending && Number.isFinite(ticks) && ticks > 0) {
    const [days, wall] = fmtDuration(ticks).split(' · ');
    ending = (ticks <= 1 ? 'next game day' : `in about ${days}`) + (wall ? ` (${wall})` : '');
  }
  return `${fmtNum(progress)} / 100 · ${ending || (state === 'growing' ? 'the walls are full before then' : 'estimate unavailable')}`;
}
export function nextCitizenHTML(pd) {
  const text = nextCitizenText(pd);
  if (!text) return '';
  const progress = pd.next_citizen_progress;
  const filled = Math.floor(Math.max(0,Math.min(100,progress))/10);
  const steps = Array.from({length:10},(_,i)=>`<span class="citizen-step${i<filled?' filled':''}"></span>`).join('');
  return `<div class="next-citizen"><span>Next citizen</span><span class="citizen-progress" role="progressbar" aria-label="Progress toward next citizen" aria-valuemin="0" aria-valuemax="100" aria-valuenow="${progress}">${steps}</span><span class="citizen-progress-text">${esc(text)}</span></div>`;
}

export function populationHTML(pd, idle, livestock, provinceID) {
  const growth = growthLineText(pd, false);
  const g = growth ? growth.charAt(0).toUpperCase() + growth.slice(1) : '';
  // What the walls hold — the ceiling only, never "room for N more"; none if the
  // server sent no cap. Counted in citizens like the rows under it (Timothy 2026-10-24: "Walls hold
  // 2,000" beside "Citizens 111" read as two different scales).
  const cap = Number(pd?.population_cap);
  return (g ? `<p class="growth-line">${g}</p>` : '') + (cap > 0 ? row('Walls hold', n(Math.floor(cap / 100))) : '') + row('Citizens', n(Math.floor(pd.population / 100))) + nextCitizenHTML(pd) + row('Free to work', n(Math.floor(idle / 100))) +
    row('Livestock', n(Math.floor(livestock)));
}

function harvestBlessingHTML(pd) {
  const b = pd?.harvest_blessing;
  if (!b || !(b.remaining_ticks > 0) || !(b.yield_bonus > 0) || !b.goods?.length) return '';
  const percent = Number((b.yield_bonus * 100).toFixed(6));
  return `<p class="harvest-blessing">Blessed harvest: +${percent}% ${esc(b.goods.join(', '))} yield · ${fmtNum(b.remaining_ticks)} ${b.remaining_ticks === 1 ? 'game day' : 'game days'} left</p>`;
}

// A storm standing on the city's staffed fishing hexes stops their fishing
// (Timothy 2026-10-24); the server counts them (storm_no_fishing).
export function stormFishingHTML(pd) {
  const n = Number(pd?.storm_no_fishing) || 0;
  if (n <= 0) return '';
  return `<p class="stat-warn">Storm over ${fmtNum(n)} ${n === 1 ? 'fishing ground' : 'fishing grounds'} — its fishers stay but catch nothing while it stands.</p>`;
}

// Production side of food only: today's net per game day. Stocks, coverage and
// the herd live in City -> Warehouse (city_warehouse.js).
export function foodSummaryHTML(pd) {
  const blessing = harvestBlessingHTML(pd) + stormFishingHTML(pd);
  const net = Number(pd?.food?.food_net_per_tick);
  const warning = pd?.food_self_sufficient === false ? '<p class="stat-warn">The fields cannot feed everyone.</p>' : '';
  if (!Number.isFinite(net)) return blessing + warning + '<p class="empty-state">Food production not reported.</p>';
  const signed = net !== 0 && Math.abs(net) < 0.05 ? 'about even' : (net >= 0 ? '+' : '-') + Math.abs(net).toFixed(1);
  return blessing + row('Food net', `${signed} per game day`) + warning;
}

// The next calves, from the server's herd.births_* fields: "4 calves in 3 days (4 grain)".
function calvesText(h) {
  const born = Number(h.births_next), inDays = Number(h.births_in);
  if (!(born > 0) || !Number.isFinite(inDays)) return '';
  return ` — ${fmtNum(born)} ${born === 1 ? 'calf' : 'calves'} in ${fmtNum(inDays)} ${inDays === 1 ? 'game day' : 'game days'} (${fmtNum(Number(h.births_grain) || 0)} grain)`;
}

// The herd and its pen: count / cap. A full pen eats, then loses, new animals.
export function herdHTML(h) {
  if (!h || !Number.isFinite(Number(h.count)) || !Number.isFinite(Number(h.cap))) return '';
  const full = Number(h.count) >= Number(h.cap);
  return `<p${full ? ' class="stat-warn"' : ''} title="A Pen raises the herd cap. Animals above the cap are eaten as food; any still above it after the meal are lost.">Herd ${fmtNum(h.count)} / ${fmtNum(h.cap)}${full ? ' — full: new animals are eaten, then lost' : ''}${calvesText(h)}</p>`;
}

export function foodDetailsHTML(pd) {
  const s = pd?.food;
  const peopleNet = s?.food_net_per_tick;
  const shared = Number.isFinite(s?.production_per_tick) && Number.isFinite(s?.consumption_per_tick);
  const grain = shared ? row('Food for people', `production ${n(s.production_per_tick)}${Number(s.herd?.surplus_rate) > 0 ? ` + herd ${n(s.herd.surplus_rate)}` : ''} − people eat ${n(s.consumption_per_tick)} = ${n(peopleNet)} net before army`) : pd?.grain_prod_rate != null ? row('Food for people', `grain ${n(pd.grain_prod_rate)} + fish ${n(pd.resources?.fish?.rate || 0)} − people eat ${n(pd.grain_consum_rate || 0)}${peopleNet != null ? ` = ${n(peopleNet)} net before army` : ''}`) : '';
  const army = pd?.food_net_per_tick_after_upkeep == null ? '' : row('Army food', `eats ${n(pd.army_upkeep?.food ?? pd.army_upkeep?.grain ?? 0)} ${pd.army_upkeep?.food != null ? 'food' : 'grain'} per game day → food net ${n(pd.food_net_per_tick_after_upkeep)} after army`);
  if (!s) return grain + army + '<p class="empty-state">Food details not reported.</p>';
  const note = pd.food_gubbar_required == null ? '' : row('Food citizens', `${n(pd.food_gubbar_required)} needed · ${n(pd.food_gubbar_placed || 0)} placed`) + '<p class="empty-state">1 citizen = 100 people. Estimate uses the best food slots for people; the army shares the food stocks and is itemized separately.</p>';
  const a = pd.automatic_food;
  const automatic = a ? `<p class="empty-state">Automatic food: ${Number(a.nearjord_grain_per_tick + a.remainder_grain_per_tick).toFixed(2)} grain + ${Number(a.remainder_fish_per_tick || 0).toFixed(2)} fish per game day without placed citizens (nearby ground ${Number(a.nearjord_grain_per_tick).toFixed(2)} grain; ${a.remainder_people} people outside whole citizens: ${Number(a.remainder_grain_per_tick).toFixed(2)} grain + ${Number(a.remainder_fish_per_tick || 0).toFixed(2)} fish, a fraction of the best food worker).</p>` : '';
  const coverage = s.coverage_ticks != null && Number.isFinite(Number(s.coverage_ticks))
    ? row('City stocks', `${fmtDuration(s.coverage_ticks)} at current consumption · ${s.goods ? foodGoodsText(s.goods) : 'grain + fish'}`)
    : '<p class="empty-state">Food stores not reported.</p>';
  const eaten = s.consumed_tick != null && s.consumed ? row('Last meal', `${foodGoodsText(s.consumed) || 'none'} · ${fmtTickTime(s.consumed_tick)}`) : '';
  return grain + army + note + automatic + coverage + eaten;
}

// Warning and ETA come from the authenticated owner's forecast, not rates here.
export function storageWarningsHTML(pd) {
  if (!Array.isArray(pd?.storage)) return '';
  return pd.storage.filter(item => item.warning === true && Number.isFinite(item.full_in_ticks) && item.full_in_ticks >= 0)
    .map(item => row(`${esc(item.good_key)} storage`, `${fmtNum(item.amount)} / ${fmtNum(item.cap)} · full in ${fmtDays(item.full_in_ticks)}`)
      + '<p class="empty-state">Send goods away or trade.</p>').join('');
}
