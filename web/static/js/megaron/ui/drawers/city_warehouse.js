import { fmtNum } from '../fmt_num.js';
import { esc } from '../format.js';
import { atStorageCeiling } from './economy.js';
import { herdHTML } from './city_production.js';
import { foodAnswer, foodAnswerValue } from '../food_line.js';

// City → Warehouse: what the city holds and how much room it has. Amounts and
// caps are the server's (goods endpoint `amount`/`cap`, province `food`); this
// only lists them. Every store is the same row shape, so a new one (e.g. a
// Storehouse) is one more row in the same list, not a new section.
const FOOD_KEYS = ['fish', 'grain', 'oil'];
// Building takes these; shown at 0 so the player sees what is missing where they build.
const BUILD_GOODS = ['stone', 'timber'];
const NOT_GOODS = new Set([...FOOD_KEYS, 'livestock']);

const signed = v => `${v >= 0 ? '+' : '\u2212'}${fmtNum(Math.abs(Number(Number(v).toFixed(1))))}`;
const stored = (amount, cap) => `${fmtNum(Math.floor(amount || 0))}${cap > 0 ? ' / ' + fmtNum(Math.floor(cap)) : ''}`;
// neutral: a single food good going down is not a loss while Food as a whole is up; its sign is shown, never red.
function storeRow(label, amount, cap, net, shared, neutral) {
  // A good in the Storehouse has no cap of its own: only the store row shows X / Y.
  if (shared) cap = 0;
  const full = atStorageCeiling(amount || 0, cap || 0);
  const netText = Number.isFinite(net) ? ` · ${signed(net)} per game day` : '';
  return `<div class="stat-row${full ? ' wh-full' : ''}"${full ? ' title="Full: anything produced past the cap is discarded."' : ''}><span class="sr-label">${esc(label)}</span><span class="sr-val${net < 0 && !neutral ? ' neg' : ''}">${stored(amount, cap)}${full ? ' · full' : ''}${netText}</span></div>`;
}
const labelOf = g => g.name || g.key;

export function warehouseHTML(pd, goods) {
  const list = Array.isArray(goods) ? goods : [];
  // One forecast owner (server): expected net per game day for every good, and how
  // long the food lasts at the net, with all known posts (citizens, army, temples).
  const fc = pd?.forecast;
  const net = {};
  for (const g of fc?.goods || []) net[g.good_key] = g.net;
  const foodRows = FOOD_KEYS.map(k => list.find(g => g.key === k))
    .filter(g => g && (g.amount > 0 || g.cap > 0))
    .map(g => storeRow(labelOf(g), g.amount, g.cap, net[g.key], false, true)).join('');
  const answer = foodAnswer(pd);
  const foodFor = answer ? `<div class="stat-row"><span class="sr-label">Food</span><span class="sr-val${answer.net < 0 ? ' neg' : ''}">${foodAnswerValue(answer)}</span></div>` : '';
  const foodFlows = (fc?.food?.flows || []).map(f => `<div class="stat-row wh-flow"><span class="sr-label">${esc(f.label)}</span><span class="sr-val${f.amount < 0 ? ' neg' : ''}">${signed(f.amount)}</span></div>`).join('');
  const food = `<div class="dsec"><div class="dsec-title">Food <button class="btn-small" onclick="openCodex('granary')" title="Read the Codex" aria-label="Read the Codex">?</button></div>`
    + foodFor + foodFlows
    + (foodRows || '<p class="empty-state">No food stored.</p>') + '</div>';
  const herdLine = herdHTML(pd?.food?.herd);
  const herd = herdLine ? `<div class="dsec"><div class="dsec-title">Herd</div>${herdLine}</div>` : '';
  const withBuild = [...list, ...BUILD_GOODS.filter(k => !list.some(g => g.key === k)).map(k => ({ key: k, name: k[0].toUpperCase() + k.slice(1), amount: 0, cap: 0 }))];
  const others = withBuild.filter(g => !NOT_GOODS.has(g.key) && (g.amount > 0 || BUILD_GOODS.includes(g.key)))
    .sort((a, b) => labelOf(a).localeCompare(labelOf(b)));
  const goodsRows = (others.some(g => g.amount > 0) ? '' : '<p class="empty-state">No goods stored.</p>') + others.map(g => storeRow(labelOf(g), g.amount, g.cap, net[g.key], g.stored_in === 'storehouse')).join('');
  const sh = pd?.storehouse;
  const storesRow = sh ? storeRow('Stores', sh.amount, sh.capacity, undefined)
    + (sh.level < 1 ? '<p class="empty-state">No Storehouse built: everything but food and the herd shares room for 500. Build a Storehouse in Construct to raise it.</p>' : '') : '';
  return food + herd + `<div class="dsec"><div class="dsec-title">Goods <button class="btn-small" onclick="openCodex('storehouse')" title="Read the Codex" aria-label="Read the Codex">?</button></div>${storesRow}${goodsRows}</div>`;
}
