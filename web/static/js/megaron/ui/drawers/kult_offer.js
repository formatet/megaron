import { esc } from '../format.js';
import { fmtDays } from '../fmt_num.js';

// The Offer row under a prayer: one number field per good, pre-filled with the
// prayer's recipe, or empty when composing an alternative. Mirrors
// `keryx rite --offer`. Odds are never shown.
const EDIBLE = ['fish', 'grain', 'oil', 'wine'];

// Goods worth a field: the recipe's, plus held goods that are edible or that
// the god favours. Recipe goods show even at 0 in stock; others only when held.
export function offerFields(prayer, stock, factor = 1) {
  const recipe = scaledRecipe(prayer.offering, factor);
  const favours = prayer.favours || {};
  const held = stock || {};
  const goods = new Set(Object.keys(recipe));
  for (const [g, n] of Object.entries(held)) {
    if (n >= 1 && (EDIBLE.includes(g) || favours[g] > 1)) goods.add(g);
  }
  return [...goods].sort().map(g => ({ good: g, amount: prayer.can_prepare_offering && !prayer.affordable ? 0 : recipe[g] || 0, stock: held[g] || 0 }));
}

// Blessing prayers (steg 2a) name a target: one unit, the city's units, or all.
// The server lists them with the price of each step (x the traditional gift).
const SCOPES = [['unit', 'One named'], ['city', 'This city\'s'], ['realm', 'The whole realm']];

export function scaledRecipe(recipe, factor = 1) {
  return Object.fromEntries(Object.entries(recipe || {}).map(([g, n]) => [g, n * factor]));
}

export function scopeFactor(prayer, scope) {
  return (prayer.scope_cost_factors && prayer.scope_cost_factors[scope]) || 1;
}

// Which steps can be chosen now: a step with nothing to bless is greyed out.
export function scopeChoices(prayer) {
  const t = prayer.targets;
  if (!t) return [];
  const count = { unit: (t.unit || []).length, city: t.city?.count || 0, realm: t.realm?.count || 0 };
  // Runner/caravan prayers (steg 2b) hold for departures too, so city and realm are always open.
  return SCOPES.map(([scope, label]) => ({ scope, label, count: count[scope], enabled: count[scope] > 0 || (!!t.always && scope !== 'unit'), factor: scopeFactor(prayer, scope) }));
}

export function defaultScope(prayer) {
  return scopeChoices(prayer).find(c => c.enabled)?.scope || 'unit';
}

export function targetChooserHTML(prayer) {
  const choices = scopeChoices(prayer);
  if (!choices.length) return '';
  const first = defaultScope(prayer);
  const id = esc(prayer.id);
  const rows = choices.map(c => {
    const what = c.scope === 'unit' ? c.count + ' to choose from'
      : prayer.targets.always ? (c.scope === 'city' ? 'every Runner and caravan of ' + (prayer.targets.city.name || 'this city') : 'all of yours') + ', on the road now (' + c.count + ') and while it lasts'
      : c.scope === 'city' ? c.count + ' of ' + (prayer.targets.city.name || 'this city')
      : 'all ' + c.count;
    return '<label class="offer-scope"><input type="radio" name="scope-' + id + '" value="' + c.scope + '"' +
      (c.scope === first ? ' checked' : '') + (c.enabled ? '' : ' disabled') + '> ' +
      esc(c.label) + ' — ' + esc(what) + ' · gift ×' + c.factor + '</label>';
  }).join('<br>');
  const options = (prayer.targets.unit || []).map(u =>
    '<option value="' + esc(u.id) + '">' + esc(u.name) + (u.where ? ' (' + esc(u.where) + ')' : '') + '</option>').join('');
  const select = options ? '<select class="offer-target-unit" aria-label="Who to bless">' + options + '</select>' : '';
  const none = choices.some(c => c.enabled) ? '' : '<p class="offer-note">You have nothing of this kind to bless.</p>';
  return '<div class="offer-target"><p class="offer-note">Who is blessed? The wider the blessing, the dearer the gift.</p>' + rows + '<br>' + select + none + '</div>';
}

// What the player chose in an open offer row: { target_scope, target } to send, or {} for a prayer without targets.
export function chosenTarget(host) {
  if (!host || !host.querySelector) return {};
  const radio = host.querySelector('input[type=radio]:checked');
  if (!radio) return {};
  const scope = radio.value;
  const out = { target_scope: scope };
  if (scope === 'unit') {
    const sel = host.querySelector('.offer-target-unit');
    if (sel && sel.value) out.target = sel.value;
  }
  return out;
}

// Words, not weights (Timothy 2026-10-19: "lite väl mkt siffror"): the god's
// dearest goods, as a bare list (nittiotalsenkelhet 2026-10-31). Never a worth or odds.
function favouredText(favours) {
  const liked = Object.entries(favours || {}).filter(([, w]) => w > 1)
    .sort(([, a], [, b]) => b - a).map(([g]) => g);
  return liked.length ? 'Favours: ' + liked.slice(0, 4).join(', ') : '';
}

export function offerRowHTML(prayer, stock) {
  const fields = offerFields(prayer, stock, prayer.targets ? scopeFactor(prayer, defaultScope(prayer)) : 1).map(f =>
    '<label class="offer-field">' + esc(f.good) +
    ' <input type="number" class="offer-input" min="0" step="1" data-good="' + esc(f.good) + '" value="' + esc(String(f.amount)) + '">' +
    ' <span class="offer-stock">/ ' + esc(String(Math.floor(f.stock))) + '</span></label>').join('');
  const note = esc(favouredText(prayer.favours));
  const id = esc(prayer.id);
  // The client does not value a gift (the gods' reckoning is server-side), so it
  // names the base length and says that a larger gift lasts longer.
  const length = prayer.base_duration_game_days > 0
    ? '<p class="offer-note">Lasts ' + esc(fmtDays(prayer.base_duration_game_days)) + ' with the traditional gift; a larger gift lasts longer.</p>'
    : '';
  const restNote = prayer.god_id ? '<p class="offer-note">A larger gift makes the temple rest longer; a higher temple level shortens the rest.</p>' : '';
  return targetChooserHTML(prayer) + '<div class="offer-fields">' + fields + '</div>' +
    '<p class="offer-note" data-offer-food></p>' +
    length + restNote +
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
    text += ` That leaves ${Math.floor(left)} food, less than ${name} eats in a game day (${Number(eat.toFixed(1))}).`;
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
