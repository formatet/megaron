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
    pd.kharis_base_decay.toFixed(2) + ' base + ' + count + (count === 1 ? ' city' : ' cities') +
    ' cost ' + pd.kharis_city_decay.toFixed(2) + ' + ' + pd.kharis_population + ' residents cost ' +
    pd.kharis_population_decay.toFixed(2) + ' = ' + pd.kharis_daily_decay.toFixed(2) + ' kharis per day</span></div>';

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
    out.netHtml = (Number.isFinite(pd.kharis_production_per_tick) ? '<div class="stat-row"><span class="sr-label">Temple production</span><span class="sr-val">' + pd.kharis_production_per_tick.toFixed(2) + ' kharis per day</span></div>' : '') + '<div class="stat-row"><span class="sr-label">Net</span><span class="sr-val">' +
      (v >= 0 ? '+' : '') + v.toFixed(1) + ' kharis per day</span></div>';
  }

  if (Number.isInteger(pd.temple_staffed) && Number.isInteger(pd.temple_capacity)) {
    out.netHtml += '<div class="stat-row"><span class="sr-label">Temple workers here</span><span class="sr-val">' + pd.temple_staffed + '/' + pd.temple_capacity + '</span></div>';
  }
  if (pd.kharis_devotion_idle === true) {
    out.idleHtml = '<p class="kult-warn">Temple workplaces remain unfilled — place citizens in your temples in City.</p>';
  }

  if (Number.isFinite(pd.kharis_per_temple_worker)) {
    out.netHtml += '<p class="empty-state">One more fed temple worker gives +' + pd.kharis_per_temple_worker.toFixed(2) +
      ' kharis per day before scarcity. Requires a free citizen, a temple workplace and its daily offering.</p>';
  }
  return out;
}
