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
  if (Number.isFinite(pd?.kharis_demand) && Number.isFinite(pd?.kharis_given_weight)) {
    return '<p>Demand: ' + pd.kharis_demand + ' · Given: ' + Number(pd.kharis_given_weight.toFixed(2)) + '</p>';
  }
  if (!Number.isFinite(pd?.kharis_daily_decay)) return '<p class="empty-state">Cult upkeep not reported.</p>';
  return '<div class="stat-row"><span class="sr-label">Upkeep</span><span class="sr-val">−' + pd.kharis_daily_decay.toFixed(1) + ' / game day</span></div>';
}

export function kharisNetView(pd, scope = 'city') {
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
    out.netHtml += '<div class="stat-row"><span class="sr-label">' + (scope === 'realm' ? 'Temple workers across your realm' : 'Temple workers here') + '</span><span class="sr-val">' + pd.temple_staffed + '/' + pd.temple_capacity + '</span></div>';
  }
  if (pd.kharis_devotion_idle === true && !(pd.kharis > pd.kharis_cap)) {
    out.idleHtml = '<p class="kult-warn">Temple workplaces unfilled</p>';
  }

  const aboveCap = Number.isFinite(pd.kharis) && Number.isFinite(pd.kharis_cap) && pd.kharis > pd.kharis_cap;
  if (aboveCap) out.netHtml += '<div class="stat-row"><span class="sr-label">Above ceiling</span><span class="sr-val">' + pd.kharis_cap + '</span></div>';
  if (!aboveCap && Number.isFinite(pd.kharis_per_temple_worker)) {
    out.netHtml += '<div class="stat-row"><span class="sr-label">Per extra temple worker</span><span class="sr-val">+' + pd.kharis_per_temple_worker.toFixed(2) + '</span></div>';
  }
  return out;
}

export function kharisStandingHTML(pd) {
 return pd?.kharis_standing?.short ? `<p>${esc(pd.kharis_standing.short)}</p>` : '';
}

export function kharisRulesHTML(pd) {
 const threshold=pd?.kharis_blessing_threshold;
 if (!Number.isFinite(threshold)) return '';
 let html='<div class="stat-row"><span class="sr-label">Blessings from kharis</span><span class="sr-val">' + threshold + '</span></div>';
 const wrath=pd?.kharis_wrath;
 if (wrath && Number.isFinite(wrath.threshold) && Number.isFinite(wrath.daily_probability)) {
  const wrathful=Number.isFinite(wrath.wrathful_threshold) && Number.isFinite(wrath.wrathful_probability) ? ', ' + Math.round(wrath.wrathful_probability*100) + '% below ' + wrath.wrathful_threshold : '';
  html+='<div class="stat-row"><span class="sr-label">Wrath risk below ' + wrath.threshold + '</span><span class="sr-val">' + Math.round(wrath.daily_probability*100) + '% / game day' + wrathful + '</span></div>';
  if (wrath.protection_remaining_days>0 && wrath.protection_until) {
   html+='<div class="stat-row"><span class="sr-label">Protected from divine punishment</span><span class="sr-val">' + fmtDuration(wrath.protection_remaining_days) + ' · until ' + fmtClock(wrath.protection_until) + '</span></div>';
  }
 }
 return html;
}

export function templeTitheHTML(offer) {
 if (!offer?.parts) return '';
 return '<br>'+offer.parts.map(p=>esc(p.good)+': '+(p.paid?'paid':'missing')).join(' · ');
}
