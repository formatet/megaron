import { esc } from '../format.js';

// The Offer row under a prayer: one number field per good, pre-filled with the
// prayer's recipe, or empty when composing an alternative. Mirrors
// `keryx rite --offer`. Odds are never shown.
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
  return [...goods].sort().map(g => ({ good: g, amount: prayer.can_prepare_offering && !prayer.affordable ? 0 : recipe[g] || 0, stock: held[g] || 0 }));
}

// Words, not weights (Timothy 2026-10-19: "lite väl mkt siffror"): the god's
// two dearest goods, then up to two more that also please. Never a worth or odds.
function andList(xs) {
  return xs.length < 2 ? xs.join('') : xs.slice(0, -1).join(', ') + ' and ' + xs[xs.length - 1];
}

export function favouredText(god, favours) {
  const liked = Object.entries(favours || {}).filter(([, w]) => w > 1)
    .sort(([, a], [, b]) => b - a).map(([g]) => g);
  if (!liked.length) return '';
  const name = god || 'The god';
  let text = name + ' loves ' + andList(liked.slice(0, 2)) + '.';
  if (liked.length > 2) {
    const more = andList(liked.slice(2, 4));
    text += ' ' + more[0].toUpperCase() + more.slice(1) + ' also please ' + name + '.';
  }
  return text + ' The traditional offering is always enough.';
}

export function offerRowHTML(prayer, stock) {
  const fields = offerFields(prayer, stock).map(f =>
    '<label class="offer-field">' + esc(f.good) +
    ' <input type="number" class="offer-input" min="0" step="1" data-good="' + esc(f.good) + '" value="' + esc(String(f.amount)) + '">' +
    ' <span class="offer-stock">/ ' + esc(String(Math.floor(f.stock))) + '</span></label>').join('');
  const note = esc(favouredText(prayer.god, prayer.favours));
  const id = esc(prayer.id);
  const preparationNote = prayer.can_prepare_offering && !prayer.affordable ? '<p class="offer-note">Choose goods for your offering; the traditional gift is unavailable.</p>' : '';
  return '<div class="offer-fields">' + fields + '</div>' +
    '<p class="offer-note" data-offer-food></p>' +
    preparationNote +
    (note ? '<p class="offer-note">' + note + '</p>' : '') +
    '<button class="obj-cta btn-small" onclick="bringOffering(\'' + id + '\')">Bring the offering</button> ' +
    '<button class="btn-small btn-secondary" onclick="cancelOffering(\'' + id + '\')">Cancel</button>';
}

// What the gift takes from the city's food, and whether a day's eating is
// left after it (byggplan A.3: Lato gave 28 of 29 grain and starved six days).
// food is the province's settlement.food (consumption_per_tick).
export function offerFoodText(offering, stock, food, city) {
  const name = city || 'the city';
  const held = stock || {};
  const parts = [];
  let taken = 0, total = 0;
  for (const g of EDIBLE) {
    total += held[g] || 0;
    const n = Number((offering || {})[g]) || 0;
    if (n > 0) {
      taken += n;
      parts.push(`takes ${Math.round(n)} of ${name}'s ${Math.floor(held[g] || 0)} ${g}`);
    }
  }
  if (!parts.length) return '';
  let text = 'The offering ' + parts.join(', ') + '.';
  const eat = (food && food.consumption_per_tick) || 0;
  const left = Math.max(0, total - taken);
  if (eat > 0 && left < eat) {
    text += ` That leaves ${Math.floor(left)} food, less than ${name} eats in a day (${Number(eat.toFixed(1))}).`;
  }
  return text;
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
