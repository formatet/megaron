import { esc } from '../format.js';

export function divineValuesHTML(values) {
  const rows = Object.entries(values || {}).filter(([,v]) => Number.isFinite(v)).sort(([a],[b]) => a.localeCompare(b));
  const body = rows.length ? '<table class="goods-mini"><tr><th>Good</th><th>Offering value</th></tr>'
    + rows.map(([key,value]) => `<tr><td>${esc(key)}</td><td>${value.toLocaleString('en-US', {maximumSignificantDigits:4})}</td></tr>`).join('') + '</table>'
    : '<p class="empty-state">Current offering values unavailable.</p>';
  return '<div class="dsec"><div class="dsec-title">How the gods weigh gifts</div><p class="dim">Per unit, before each god’s preferences. These are not trade prices.</p>' + body + '</div>';
}
