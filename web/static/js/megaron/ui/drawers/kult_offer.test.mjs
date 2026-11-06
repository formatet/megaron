import test from 'node:test';
import assert from 'node:assert/strict';
const { offerFields, offerRowHTML, offeringFromEntries, scopeChoices, chosenTarget, scaledRecipe, defaultScope } = await import('./kult_offer.js');

const prayer = { id: 'minoan_potnia_vision_v2', god: 'Potnia', offering: { grain: 10, silver: 3 },
  favours: { wine: 1.8, grain: 0.9, bronze: 1.4 }, offering_baseline: 45.4 };
const stock = { grain: 0, silver: 5, wine: 30, fish: 12, timber: 40, bronze: 2, oil: 0 };

test('offer row is pre-filled with the recipe, even for goods at 0 stock', () => {
  const f = offerFields(prayer, stock);
  assert.equal(f.find(x => x.good === 'grain').amount, 10);
  assert.equal(f.find(x => x.good === 'silver').amount, 3);
});

test('offer row adds edible and favoured held goods at 0, skips the rest', () => {
  const f = offerFields(prayer, stock);
  const goods = f.map(x => x.good);
  assert.ok(goods.includes('wine') && goods.includes('fish') && goods.includes('bronze'));
  assert.ok(!goods.includes('timber') && !goods.includes('oil'));
  assert.equal(f.find(x => x.good === 'fish').amount, 0);
});

test('offer row shows the keryx taste line and no odds', () => {
  const html = offerRowHTML(prayer, stock);
  assert.match(html, /Favours: wine, bronze/);assert.doesNotMatch(html, /loves|always enough/);
  assert.doesNotMatch(html, /×|worth|~\d/);
  assert.match(html, /Bring the offering/);
  assert.match(html, /Cancel/);
  assert.doesNotMatch(html, /%|chance|odds/i);
});

test('unchanged recipe sends no offering', () => {
  const e = [{ good: 'grain', value: '10' }, { good: 'silver', value: '3' }, { good: 'fish', value: '0' }];
  assert.equal(offeringFromEntries(e, prayer.offering), null);
});

test('changed fields send exactly the positive numbers; zero fields are left out', () => {
  const e = [{ good: 'grain', value: '0' }, { good: 'silver', value: '3' }, { good: 'fish', value: '12' }, { good: 'wine', value: '' }];
  assert.deepEqual(offeringFromEntries(e, prayer.offering), { silver: 3, fish: 12 });
});

test('a city holding incense gets an incense field and the god is said to love it', () => {
  const oracle = { id: 'x', god: 'Potnia', offering: { grain: 10 }, favours: { incense: 3.0, wine: 1.8 } };
  const held = { grain: 5, incense: 4, wine: 0 };
  const f = offerFields(oracle, held);
  const inc = f.find(x => x.good === 'incense');
  assert.ok(inc, 'incense is offered when held');
  assert.equal(inc.amount, 0);
  const html = offerRowHTML(oracle, held);
  assert.match(html, /data-good="incense"/);
  assert.match(html, /Favours: incense, wine/);
  assert.doesNotMatch(html, /worth|odds|%/i);
});

test('alternative preparation starts at zero instead of silently offering missing recipe goods',()=>{
 const p={...prayer,affordable:false,can_prepare_offering:true};
 const fields=offerFields(p,stock);assert.ok(fields.length>0);assert.ok(fields.every(f=>f.amount===0));
 const html=offerRowHTML(p,stock);assert.doesNotMatch(html,/traditional gift is unavailable/);
});

// Lato, tick 5053: 28 grain offered out of 29, net +0.5 — the city starved
// six days. The row says what the gift takes from the city's food (A.3).
test('offer row names the food the gift takes and warns when a game day is not left', async () => {
  const { offerFoodText } = await import('./kult_offer.js');
  const food = { consumption_per_tick: 55.8 };
  const text = offerFoodText({ grain: 28, silver: 3 }, { grain: 29.6, fish: 4, silver: 9 }, food, 'Lato');
  assert.match(text, /takes 28 of Lato's 29 grain/);
  assert.match(text, /leaves 5 food, less than Lato eats in a game day \(55\.8\)/);
  assert.equal(offerFoodText({ silver: 3 }, { grain: 29 }, food, 'Lato'), '');
  const plenty = offerFoodText({ grain: 10 }, { grain: 900 }, food, 'Lato');
  assert.match(plenty, /takes 10 of Lato's 900 grain/);
  assert.doesNotMatch(plenty, /less than/);
});

test('Bönegrund: a lasting prayer names its base length and says a larger gift lasts longer', () => {
  const html = offerRowHTML({ ...prayer, god_id: 'potnia', base_duration_game_days: 6 }, stock);
  assert.match(html, /Lasts 6 game days with the traditional gift; a larger gift lasts longer\./);
  assert.match(html, /larger gift makes the temple rest longer/);
  assert.doesNotMatch(offerRowHTML({ ...prayer, god_id: 'potnia' }, stock), /Lasts/);
});

// Blessing prayers (steg 2a): the offer row carries a target chooser with the price of each step.
const blessing = { id: 'minoan_stillness_hull', god_id: 'sea_daimons_stillness', offering: { fish: 8, oil: 4 }, scope_cost_factors: { unit: 1, city: 2, realm: 4 },
  targets: { unit: [{ id: 'u1', name: 'Horns of Crete', where: 'in Knossos' }, { id: 'u2', name: 'Argo', where: 'sailing at (3,1)' }], city: { name: 'Knossos', count: 2 }, realm: { count: 3 } } };

test('target chooser: unit/city/realm with the gift multiple, ships by name and place', () => {
  const html = offerRowHTML(blessing, { fish: 100, oil: 100 });
  assert.match(html, /One named — 2 to choose from · gift ×1/);
  assert.match(html, /This city's — 2 of Knossos · gift ×2/);
  assert.match(html, /The whole realm — all 3 · gift ×4/);
  assert.match(html, /Horns of Crete \(in Knossos\)/);
  assert.match(html, /name="scope-minoan_stillness_hull" value="unit" checked/);
  assert.doesNotMatch(html, /%|chance|odds/i);
});

test('target chooser: a step with nothing to bless is greyed out and the first usable step is the default', () => {
  const only = { ...blessing, targets: { unit: [], city: { name: 'Knossos', count: 0 }, realm: { count: 0 } } };
  assert.ok(scopeChoices(only).every(c => !c.enabled));
  assert.match(offerRowHTML(only, {}), /nothing of this kind to bless/);
  const cityOnly = { ...blessing, targets: { unit: [], city: { name: 'Knossos', count: 1 }, realm: { count: 1 } } };
  assert.equal(defaultScope(cityOnly), 'city');
  assert.match(offerRowHTML(cityOnly, { fish: 100 }), /value="unit" disabled/);
});

test('target chooser: the fields are pre-filled with the gift of the chosen step; a prayer without targets is unchanged', () => {
  const field = (html, good) => Number(new RegExp('data-good="' + good + '" value="(\\d+)"').exec(html)[1]);
  assert.equal(field(offerRowHTML(blessing, { fish: 100, oil: 100 }), 'fish'), 8);
  assert.deepEqual(scaledRecipe(blessing.offering, 4), { fish: 32, oil: 16 });
  const cityFirst = { ...blessing, targets: { ...blessing.targets, unit: [] } };
  assert.equal(field(offerRowHTML(cityFirst, { fish: 100, oil: 100 }), 'fish'), 16);
  assert.doesNotMatch(offerRowHTML(prayer, stock), /offer-scope/);
});

test('chosenTarget reads the checked step and, for one unit, the chosen unit', () => {
  const host = (scope, unit) => ({ querySelector: sel => sel.includes('checked') ? { value: scope } : sel.includes('offer-target-unit') ? { value: unit } : null });
  assert.deepEqual(chosenTarget(host('unit', 'u2')), { target_scope: 'unit', target: 'u2' });
  assert.deepEqual(chosenTarget(host('realm', 'u2')), { target_scope: 'realm' });
  assert.deepEqual(chosenTarget(null), {});
});
