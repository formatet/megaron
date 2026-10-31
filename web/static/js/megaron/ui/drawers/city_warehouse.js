import { fmtNum, fmtDays } from '../fmt_num.js';
import { esc } from '../format.js';
import { atStorageCeiling } from './economy.js';
import { herdHTML } from './city_production.js';

// City → Warehouse: what the city holds and how much room it has. Amounts and
// caps are the server's (goods endpoint `amount`/`cap`, province `food`); this
// only lists them. Every store is the same row shape, so a new one (e.g. a
// Storehouse) is one more row in the same list, not a new section.
const FOOD_KEYS = ['grain', 'fish', 'oil'];
const NOT_GOODS = new Set([...FOOD_KEYS, 'livestock']);

const stored = (amount, cap) => `${fmtNum(Math.floor(amount || 0))}${cap > 0 ? ' / ' + fmtNum(Math.floor(cap)) : ''}`;
function storeRow(label, amount, cap) {
  const full = atStorageCeiling(amount || 0, cap || 0);
  return `<div class="stat-row${full ? ' wh-full' : ''}"${full ? ' title="Full: anything produced past the cap is discarded."' : ''}><span class="sr-label">${esc(label)}</span><span class="sr-val">${stored(amount, cap)}${full ? ' · full' : ''}</span></div>`;
}
const labelOf = g => g.name || g.key;

export function warehouseHTML(pd, goods) {
  const list = Array.isArray(goods) ? goods : [];
  const cov = pd?.food?.coverage_ticks;
  const foodRows = FOOD_KEYS.map(k => list.find(g => g.key === k))
    .filter(g => g && (g.amount > 0 || g.cap > 0))
    .map(g => storeRow(labelOf(g), g.amount, g.cap)).join('');
  const food = `<div class="dsec"><div class="dsec-title">Food <button class="btn-small" onclick="openCodex('granary')" title="Read the Codex" aria-label="Read the Codex">?</button></div>`
    + (cov != null && Number.isFinite(Number(cov)) ? `<div class="stat-row"><span class="sr-label">Food for</span><span class="sr-val">${fmtDays(cov)}</span></div>` : '')
    + (foodRows || '<p class="empty-state">No food stored.</p>') + '</div>';
  const herdLine = herdHTML(pd?.food?.herd);
  const herd = herdLine ? `<div class="dsec"><div class="dsec-title">Herd</div>${herdLine}</div>` : '';
  const others = list.filter(g => !NOT_GOODS.has(g.key) && g.amount > 0)
    .sort((a, b) => labelOf(a).localeCompare(labelOf(b)));
  const goodsRows = others.length ? others.map(g => storeRow(labelOf(g), g.amount, g.cap)).join('') : '<p class="empty-state">No goods stored.</p>';
  return food + herd + `<div class="dsec"><div class="dsec-title">Goods</div>${goodsRows}</div>`;
}
