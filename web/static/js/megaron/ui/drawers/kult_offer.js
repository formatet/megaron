import { esc } from '../format.js';

// The Offer row under a prayer: one number field per good, pre-filled with the
// prayer's recipe. Mirrors `keryx rite --offer`. Odds are never shown.
const EDIBLE = ['grain', 'fish', 'oil', 'wine'];

// Goods worth a field: the recipe's, plus held goods that are edible or that
// the god favours. Recipe goods show even at 0 in stock; others only when held.
export function offerFields(prayer, stock) {
  const recipe = prayer.offering || {};
  const favours = prayer.favours || {};
  const held = stock || {};
  const goods = new Set(Object.keys(recipe));
  for (const [g, n] of Object.entries(held)) {
    if (n >= 1 && (EDIBLE.includes(g) || favours[g] > 1)) goods.add(g);
  }
  return [...goods].sort().map(g => ({ good: g, amount: recipe[g] || 0, stock: held[g] || 0 }));
}

function favouredText(favours) {
  return Object.entries(favours || {}).filter(([, w]) => w > 1)
    .sort(([, a], [, b]) => b - a).slice(0, 4)
    .map(([g, w]) => g + ' ×' + w.toFixed(1)).join(', ');
}

export function offerRowHTML(prayer, stock) {
  const fields = offerFields(prayer, stock).map(f =>
    '<label class="offer-field">' + esc(f.good) +
    ' <input type="number" class="offer-input" min="0" step="1" data-good="' + esc(f.good) + '" value="' + esc(String(f.amount)) + '">' +
    ' <span class="offer-stock">/ ' + esc(String(Math.floor(f.stock))) + '</span></label>').join('');
  const taste = favouredText(prayer.favours);
  let note = '';
  if (taste) {
    note = esc(prayer.god || '') + ' favours ' + esc(taste);
    if (prayer.offering_baseline > 0) note += ' · expects an offering worth ~' + Math.round(prayer.offering_baseline);
  }
  const id = esc(prayer.id);
  return '<div class="offer-fields">' + fields + '</div>' +
    (note ? '<p class="offer-note">' + note + '</p>' : '') +
    '<button class="obj-cta btn-small" onclick="bringOffering(\'' + id + '\')">Bring the offering</button> ' +
    '<button class="btn-small btn-secondary" onclick="cancelOffering(\'' + id + '\')">Cancel</button>';
}

// entries: [{good, value}] from the fields. Returns the offering to send, or
// null when it equals the recipe (server then uses the recipe, as before).
export function offeringFromEntries(entries, recipe) {
  const out = {};
  for (const { good, value } of entries) {
    const n = Number(value);
    if (Number.isFinite(n) && n > 0) out[good] = n;
  }
  const r = recipe || {};
  const same = Object.keys(out).length === Object.keys(r).length &&
    Object.entries(out).every(([g, n]) => r[g] === n);
  return same || !Object.keys(out).length ? null : out;
}
