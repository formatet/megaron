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

// One decimal when it is exact, otherwise two: the 1/3 and 7/12 kharis pace (2026-11-04) would read 0.3/0.6.
const kx = (x) => (Math.abs(x * 10 - Math.round(x * 10)) < 1e-9 ? x.toFixed(1) : x.toFixed(2));

export function kharisDecayHTML(pd) {
  if (!Number.isFinite(pd?.kharis_daily_decay)) return '<p class="empty-state">Cult upkeep not reported.</p>';
  if (pd.kharis_net_known === true) return ''; // shown inside the game-day block of kharisNetView
  return '<div class="stat-row"><span class="sr-label">Upkeep</span><span class="sr-val neg">−' + kx(pd.kharis_daily_decay) + ' / game day</span></div>';
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
    // One heading carries the unit; the rows are plain signed numbers.
    out.netHtml = '<p>Kharis each game day</p>' +
      (Number.isFinite(pd.kharis_production_per_tick) ? '<div class="stat-row"><span class="sr-label">Temple production</span><span class="sr-val">+' + kx(pd.kharis_production_per_tick) + '</span></div>' : '') +
      (Number.isFinite(pd.kharis_daily_decay) ? '<div class="stat-row"><span class="sr-label">Upkeep</span><span class="sr-val neg">−' + kx(pd.kharis_daily_decay) + '</span></div>' : '') +
      '<div class="stat-row"><span class="sr-label">Net</span><span class="sr-val' + (v < 0 ? ' neg' : '') + '">' + (v >= 0 ? '+' : '') + kx(v) + '</span></div>';
  }

  if (Number.isInteger(pd.temple_staffed) && Number.isInteger(pd.temple_capacity)) {
    out.netHtml += '<div class="stat-row"><span class="sr-label">' + (scope === 'realm' ? 'Temple workers across your realm' : 'Temple workers here') + '</span><span class="sr-val">' + pd.temple_staffed + '/' + pd.temple_capacity + '</span></div>';
  }
  if (pd.kharis_devotion_idle === true && !(pd.kharis > pd.kharis_cap)) {
    out.idleHtml = '<p class="kult-warn">Temple workplaces unfilled</p>';
  }

  const aboveCap = Number.isFinite(pd.kharis) && Number.isFinite(pd.kharis_cap) && pd.kharis > pd.kharis_cap;
  if (aboveCap) out.netHtml += '<div class="stat-row"><span class="sr-label">Above ceiling</span><span class="sr-val warn">' + pd.kharis_cap + '</span></div>';
  if (!aboveCap && Number.isFinite(pd.kharis_per_temple_worker)) {
    out.netHtml += '<div class="stat-row"><span class="sr-label">Per extra temple worker</span><span class="sr-val">+' + pd.kharis_per_temple_worker.toFixed(2) + '</span></div>';
  }
  return out;
}

// What today's standing does, one short row per effect, from the server's
// numbers. The prose summary is for notices; panels show rows.
export function kharisEffectRows(st) {
 if (!st) return [];
 const pct = p => Math.round(p * 100) + '%';
 const rows = [];
 if (st.punishment_probability > 0) rows.push(['Divine punishment', pct(st.punishment_probability) + ' each game day']);
 if (st.blessing_probability > 0) rows.push(['Blessing', pct(st.blessing_probability) + ' each game day']);
 if (st.storm_spare_probability > 0) rows.push(['Storms at sea', pct(st.storm_spare_probability) + ' spared']);
 if (st.storm_wrath_probability > 0) rows.push(['Storms at sea', pct(st.storm_wrath_probability) + ' harder']);
 const b = st.battle_strength_factor;
 if (Number.isFinite(b) && b !== 1) rows.push(['In battle', (b > 1 ? '+' : '−') + pct(Math.abs(b - 1))]);
 return rows;
}

export function kharisStandingHTML(pd) {
 const st = pd?.kharis_standing;
 if (!st?.short) return '';
 return `<p>${esc(st.short)}</p>` + kharisEffectRows(st).map(([l, v]) => '<div class="stat-row"><span class="sr-label">' + l + '</span><span class="sr-val">' + v + '</span></div>').join('');
}

export function kharisRulesHTML(pd) {
 const threshold=pd?.kharis_blessing_threshold;
 if (!Number.isFinite(threshold)) return '';
 let html='<div class="stat-row"><span class="sr-label">Blessings from kharis</span><span class="sr-val">' + threshold + '</span></div>';
 const wrath=pd?.kharis_wrath;
 if (wrath && Number.isFinite(wrath.threshold) && Number.isFinite(wrath.daily_probability)) {
  const wrathful=Number.isFinite(wrath.wrathful_threshold) && Number.isFinite(wrath.wrathful_probability) ? ', ' + Math.round(wrath.wrathful_probability*100) + '% below ' + wrath.wrathful_threshold : '';
  html+='<div class="stat-row"><span class="sr-label">Wrath risk below ' + wrath.threshold + '</span><span class="sr-val">' + Math.round(wrath.daily_probability*100) + '% / game day' + wrathful + '</span></div>';
  if (wrath.scapegoat_remaining_game_days>0) {
   html+='<div class="stat-row"><span class="sr-label">Scapegoat</span><span class="sr-val">stands for your realm for ' + fmtDuration(wrath.scapegoat_remaining_game_days) + ' · bears the next divine punishment</span></div>';
  }
  if (wrath.protection_remaining_days>0 && wrath.protection_until) {
   html+='<div class="stat-row"><span class="sr-label">Protected from divine punishment</span><span class="sr-val">' + fmtDuration(wrath.protection_remaining_days) + ' · until ' + fmtClock(wrath.protection_until) + '</span></div>';
  }
 }
 return html;
}

export function templeTitheHTML(offer) {
 if (!offer?.parts) return '';
 return '<br>'+offer.parts.map(p=>esc(p.good)+': '+(p.paid?'paid':'missing')).join(' · ')+richOfferingLine(offer);
}

// Rich offering status: paid in full (+50%) or not paid, never a partial bonus.
export function richOfferingLine(offer) {
 if (!offer?.rich_offering_good) return '';
 const amount = esc(String(Number(Number(offer.rich_offering_amount || 0).toPrecision(2))));
 return '<br>Rich offering: ' + amount + ' ' + esc(offer.rich_offering_good) + (offer.rich_offering_paid ? ' — paid, +50%' : ' — not paid');
}
