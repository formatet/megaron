import { esc } from '../format.js';
import { fmtDuration } from '../fmt_num.js';
import { fmtClock } from '../fmt_clock.js';
// kult_kharis.js — pure formatting for the kharis NET figure and the
// devotion-idle warning shown in the Kult drawer. No DOM, no fetch: kult.js
// already fetches `pd` (the province GET's settlement object) and hands it
// here; this module only decides what HTML (if any) to draw from fields
// that are already present in that payload.
//
// Net and decay are authoritative server projections, including templeless realms.
// Missing data stays unavailable; this formatter never derives the city formula.

export function kharisDecayHTML(pd) {
  const values = ['kharis_city_count', 'kharis_population', 'kharis_base_decay', 'kharis_city_decay', 'kharis_population_decay', 'kharis_daily_decay'];
  if (!pd || !values.every(key => Number.isFinite(pd[key]))) return '<p class="empty-state">Cult upkeep not reported.</p>';
  const count = pd.kharis_city_count;
  return '<div class="stat-row"><span class="sr-label">Upkeep of the cult</span><span class="sr-val">' +
    pd.kharis_base_decay.toFixed(1) + ' base + ' + count + (count === 1 ? ' city' : ' cities') +
    ' cost ' + pd.kharis_city_decay.toFixed(1) + ' + ' + pd.kharis_population + ' residents cost ' +
    pd.kharis_population_decay.toFixed(1) + ' = ' + pd.kharis_daily_decay.toFixed(1) + ' kharis per game day</span></div><p class="empty-state">This is what your whole realm loses each game day before temple production.</p>';

}

export function kharisNetView(pd) {
  const out = { netHtml: '', idleHtml: '' };
  if (!pd) return out;

  // Distinct from the existing "Passive" row (pd.kharis_per_tick, rendered by
  // kult.js) — that is the geographic rate; this is the daily maintenance
  // net (temple gain − decay), the figure that actually answers "did raising
  // devotion help". Same format as the passive row: sign + one decimal.
  if (pd.kharis_net_known === true) {
    const v = pd.kharis_net_per_tick || 0;
    out.netHtml = (Number.isFinite(pd.kharis_production_per_tick) ? '<div class="stat-row"><span class="sr-label">Temple production</span><span class="sr-val">' + pd.kharis_production_per_tick.toFixed(2) + ' kharis per game day</span></div>' : '') + '<div class="stat-row"><span class="sr-label">Net</span><span class="sr-val">' +
      (v >= 0 ? '+' : '') + v.toFixed(1) + ' kharis per game day</span></div>';
  }

  if (Number.isInteger(pd.temple_staffed) && Number.isInteger(pd.temple_capacity)) {
    out.netHtml += '<div class="stat-row"><span class="sr-label">Temple workers here</span><span class="sr-val">' + pd.temple_staffed + '/' + pd.temple_capacity + '</span></div>';
  }
  if (pd.kharis_devotion_idle === true && !(pd.kharis > pd.kharis_cap)) {
    out.idleHtml = '<p class="kult-warn">Temple workplaces remain unfilled — place citizens in your temples in City.</p>';
  }

  const aboveCap = Number.isFinite(pd.kharis) && Number.isFinite(pd.kharis_cap) && pd.kharis > pd.kharis_cap;
  if (aboveCap) out.netHtml += '<p class="empty-state">Above your temple’s ceiling: kharis declines through daily cult upkeep; temple workers and offerings do not raise it while it is above the ceiling ' + pd.kharis_cap + '. Once below, temples can raise it again, up to the ceiling.</p>';
  if (!aboveCap && Number.isFinite(pd.kharis_per_temple_worker)) {
    out.netHtml += '<p class="empty-state">One more fed temple worker gives at least +' + pd.kharis_per_temple_worker.toFixed(2) +
      ' kharis per game day before cult upkeep, and more when the cult’s reference offering of grain is scarce in the world. Requires a free citizen, a temple workplace and its daily offering.</p>';
  }
  return out;
}

export function kharisStandingHTML(pd) {
 return pd?.kharis_standing?.summary ? `<p class="empty-state">${esc(pd.kharis_standing.summary)}</p>` : '';
}

export function kharisRulesHTML(pd) {
 const threshold=pd?.kharis_blessing_threshold;
 if (!Number.isFinite(threshold)) return '';
 let html='<p class="empty-state">Everyday prayers work at a level-1 temple. Spontaneous blessings need ' + threshold + ' kharis and a maintained temple. Only a level-2 temple or higher lets kharis grow that far. Kharis retained above a lowered ceiling still counts.</p>';
 const wrath=pd?.kharis_wrath;
 if (wrath && Number.isFinite(wrath.threshold) && Number.isFinite(wrath.daily_probability)) {
  html+='<p class="empty-state">Below ' + wrath.threshold + ' kharis, there is a ' + Math.round(wrath.daily_probability*100) + '% chance of divine punishment each game day, with or without a temple.</p>';
  if (wrath.protection_remaining_days>0 && wrath.protection_until) {
   html+='<div class="stat-row"><span class="sr-label">Protected from divine punishment</span><span class="sr-val">' + fmtDuration(wrath.protection_remaining_days) + ' · until ' + fmtClock(wrath.protection_until) + '</span></div>';
  }
 }
 return html;
}
