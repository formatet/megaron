import { fmtNum, fmtDays } from './fmt_num.js';
// Rules come from Temenos; client controls never maintain their own tunables.
export function configureExpedition(input, rules) {
  if (!input) return;
  input.value = rules ? rules.default_ticks : '';
  input.min = rules ? rules.min_ticks : '';
  input.max = rules ? rules.max_ticks : '';
  input.disabled = !rules;
}
export function expeditionTicks(input) {
  if (!input || input.disabled) throw new Error('Expedition rules unavailable — reload before choosing a duration.');
  const n = Number(input.value);
  if (!Number.isInteger(n) || n < Number(input.min) || n > Number(input.max)) throw new Error(`Choose a whole expedition duration from ${input.min} to ${input.max} game days.`);
  return n;
}
export function expeditionOrderText(q, r, ticks, rules) {
  if (!rules) return 'Expedition rules unavailable — reload before choosing a duration.';
  return `Explore around (${q},${r}), within ${rules.area_radius} hexes, for ${ticks} game days. Turn home by half the duration; report on return.`;
}
export function expeditionMissionText(e) {
  if (!e) return '';
  const reasons = {half_time:'half the time reached',area_known:'area explored',no_path:'no reachable unexplored ground'};
  const state = e.homeward ? 'returning home' + (e.turn_reason ? ' — ' + (reasons[e.turn_reason] || e.turn_reason) : '') : `turns home by game day ${fmtNum(e.turn_tick)}`;
  return `Expedition around (${e.area_q},${e.area_r}), ${fmtDays(e.length_ticks)}; ${state}; home by game day ${fmtNum(e.home_by_tick)}.`;
}
