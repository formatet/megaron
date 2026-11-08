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
// The fourth step (steg 4): one named unit, for ever. Offered only by prayers that have it (scope_cost_factors.permanent).
const FOREVER = ['permanent', 'For ever'];

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
  const count = { unit: (t.unit || []).length, city: t.city?.count || 0, realm: t.realm?.count || 0,
    permanent: (t.unit || []).filter(u => !u.forever).length };
  let scopes = prayer.scope_cost_factors && prayer.scope_cost_factors.permanent ? [...SCOPES, FOREVER] : SCOPES;
  // A prayer lists only the steps it offers (the world blessings, steg 5a, have no unit step; the scapegoat only the realm).
  if (prayer.scope_cost_factors) scopes = scopes.filter(([scope]) => prayer.scope_cost_factors[scope]);
  // Runner/caravan prayers (steg 2b) hold for departures too, so city and realm are always open.
  return scopes.map(([scope, label]) => ({ scope, label, count: count[scope], enabled: count[scope] > 0 || (!!t.always && scope !== 'unit'), factor: scopeFactor(prayer, scope) }));
}

export function defaultScope(prayer) {
  return scopeChoices(prayer).find(c => c.enabled)?.scope || 'unit';
}

// Storm prayers (steg 3b) name no unit: Still the Storm picks a named storm from your storm map,
// Wake the Storm takes the coordinates of a sea hex of your own map.
function stormChooserHTML(prayer) {
  const t = prayer.targets;
  if (t.kind === 'hex') {
    return '<div class="offer-target"><p class="offer-note">Where is the storm woken? Give a sea hex of your own map as q,r.</p>' +
      '<label class="offer-field">Sea hex <input type="text" class="offer-target-hex" placeholder="q,r" inputmode="text" autocomplete="off" aria-label="Sea hex q,r"></label></div>';
  }
  const options = (t.storms || []).map(s =>
    '<option value="' + esc(s.id) + '">' + esc(s.name) + (s.where ? ' (' + esc(s.where) + ')' : '') + '</option>').join('');
  if (!options) return '<div class="offer-target"><p class="offer-note">No named storm on your storm map. A storm must be named before it can be stilled.</p></div>';
  return '<div class="offer-target"><p class="offer-note">Which storm is stilled? It shrinks by one hex, wherever it is now.</p>' +
    '<select class="offer-target-storm" aria-label="Storm to still">' + options + '</select></div>';
}

// The Walls Fall (steg 5c) names a foreign city of your map whose wall you remember. No scope steps.
function cityChooserHTML(prayer) {
  const options = (prayer.targets.cities || []).map(c =>
    '<option value="' + esc(c.id) + '">' + esc(c.name) + (c.where ? ' (' + esc(c.where) + ')' : '') + '</option>').join('');
  if (!options) return '<div class="offer-target"><p class="offer-note">No foreign city with a wall you remember. See a walled city first.</p></div>';
  return '<div class="offer-target"><p class="offer-note">Whose wall is Potnia asked to bring down? It falls one level 3 game days after the prayer, and you will not be told: see the city again to learn it. A battle already under way at the walls is not changed.</p>' +
    '<select class="offer-target-city" aria-label="City whose wall falls">' + options + '</select></div>';
}

// The Bird Oracle (steg 5d) names a city (yours or remembered) or a named place whose anchor you know.
function placeChooserHTML(prayer) {
  const cities = (prayer.targets.cities || []).map(c =>
    '<option value="' + esc(c.id) + '">' + esc(c.name) + (c.where ? ' (' + esc(c.where) + ')' : '') + '</option>').join('');
  const places = (prayer.targets.places || []).map(p =>
    '<option value="' + esc(p.id) + '">' + esc(p.name) + (p.where ? ' (' + esc(p.where) + ')' : '') + '</option>').join('');
  if (!cities && !places) return '<div class="offer-target"><p class="offer-note">No named place on your map. Asterion\'s birds need a city or a named land you know.</p></div>';
  return '<div class="offer-target"><p class="offer-note">Which place do Asterion\'s birds watch? For 1 game day you see what stands within 2 hexes of it, live, armies and fleets with their strength. Afterwards it is grey memory. The one watched is told nothing.</p>' +
    '<select class="offer-target-place" aria-label="Place to watch">' +
    (cities ? '<optgroup label="Cities">' + cities + '</optgroup>' : '') + (places ? '<optgroup label="Named places">' + places + '</optgroup>' : '') + '</select></div>';
}

// World blessings (steg 5a): a city or the realm, no unit to name.
function isWorldPrayer(prayer) { return prayer.targets && prayer.targets.kind === 'world'; }

export function targetChooserHTML(prayer) {
  if (prayer.targets && prayer.targets.kind === 'place') return placeChooserHTML(prayer);
  if (prayer.targets && prayer.targets.kind === 'city') return cityChooserHTML(prayer);
  if (prayer.targets && (prayer.targets.kind === 'storm' || prayer.targets.kind === 'hex')) return stormChooserHTML(prayer);
  const choices = scopeChoices(prayer);
  if (!choices.length) return '';
  const first = defaultScope(prayer);
  const id = esc(prayer.id);
  const rows = choices.map(c => {
    const what = isWorldPrayer(prayer) ? (c.scope === 'city' ? (prayer.targets.city.name || 'this city') + ' only' : (c.count === 1 ? 'your one city' : 'all ' + c.count + ' of your cities'))
      : c.scope === 'unit' ? c.count + ' to choose from'
      : c.scope === 'permanent' ? 'the one you choose below, and never a city; ' + c.count + ' without it yet'
      : prayer.targets.always ? (c.scope === 'city' ? 'every ' + (prayer.id === 'minoan_asterion_thread_v2' ? 'land caravan' : 'Runner and caravan') + ' of ' + (prayer.targets.city.name || 'this city') : 'all of yours') + ', on the road now (' + c.count + ') and while it lasts'
      : c.scope === 'city' ? c.count + ' of ' + (prayer.targets.city.name || 'this city')
      : 'all ' + c.count;
    return '<label class="offer-scope"><input type="radio" name="scope-' + id + '" value="' + c.scope + '"' +
      (c.scope === first ? ' checked' : '') + (c.enabled ? '' : ' disabled') + '> ' +
      esc(c.label) + ' — ' + esc(what) + ' · gift ×' + c.factor + '</label>';
  }).join('<br>');
  const options = (prayer.targets.unit || []).map(u =>
    '<option value="' + esc(u.id) + '">' + esc(u.name) + (u.where ? ' (' + esc(u.where) + ')' : '') + (u.forever ? ', blessed for ever' : '') + '</option>').join('');
  const select = options ? '<select class="offer-target-unit" aria-label="Who to bless">' + options + '</select>' : '';
  const none = choices.some(c => c.enabled) ? '' : '<p class="offer-note">You have nothing of this kind to bless.</p>';
  return '<div class="offer-target"><p class="offer-note">' + (isWorldPrayer(prayer) ? 'How wide is it laid?' : 'Who is blessed?') + ' The wider the blessing, the dearer the gift.</p>' + rows + '<br>' + select + none + '</div>';
}

// What the player chose in an open offer row: { target_scope, target } to send, or {} for a prayer without targets.
export function chosenTarget(host) {
  if (!host || !host.querySelector) return {};
  const place = host.querySelector('.offer-target-place');
  if (place && place.value) return { target: place.value };
  const city = host.querySelector('.offer-target-city');
  if (city && city.value) return { target: city.value };
  const storm = host.querySelector('.offer-target-storm');
  if (storm) return { target: storm.value };
  const hex = host.querySelector('.offer-target-hex');
  if (hex) return { target: (hex.value || '').trim() };
  const radio = host.querySelector('input[type=radio]:checked');
  if (!radio) return {};
  const scope = radio.value;
  const out = { target_scope: scope };
  if (scope === 'unit' || scope === 'permanent') {
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
