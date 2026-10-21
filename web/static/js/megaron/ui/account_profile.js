import { fmtTickTime } from './fmt_clock.js';
import { esc } from './format.js';

export function accountProfileHTML(profile) {
  const rows = [
    ['Wanax', profile.wanax_name],
    ['Founded', profile.founded_tick == null ? 'Not yet founded' : `Since ${fmtTickTime(profile.founded_tick)}`],
    ['Cities', profile.cities],
    ['Population', profile.population],
    ['Kharis', Number(profile.kharis).toFixed(1)],
  ];
  return rows.map(([label, value]) => `<div class="stat-row"><span class="sr-label">${label}</span><span class="sr-val">${esc(String(value))}</span></div>`).join('');
}
