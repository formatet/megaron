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

test('Bönegrund: a lasting prayer shows its length per step and says the temple rests longer after a larger gift', () => {
  // Changed 2026-11-12 (bon-gavosteg): the "Lasts N ... a larger gift lasts longer" note is now the step list.
  const html = offerRowHTML({ ...prayer, god_id: 'potnia', base_duration_game_days: 6 }, stock);
  assert.match(html, /6 game days · 10 grain \+ 3 silver/);
  assert.match(html, /12 game days/);
  assert.match(html, /The temple rests longer after a larger gift\./);
  assert.doesNotMatch(offerRowHTML({ ...prayer, god_id: 'potnia' }, stock), /How much do you give|rests longer/);
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

// Speed prayers (steg 2b): Runners/caravans on the road are named; city and realm hold for departures too.
const swift = { id: 'minoan_asterion_swift', god_id: 'asterion', offering: { timber: 4, silver: 3 }, scope_cost_factors: { unit: 1, city: 2, realm: 4 },
  targets: { always: true, unit: [{ id: 'm1', name: 'Runner to Phaistos', where: 'arrives in 4 game days' }], city: { name: 'Knossos', count: 1 }, realm: { count: 1 } } };

test('speed prayer: the road journeys are listed by destination and arrival; city and realm say they hold for departures too', () => {
  const html = offerRowHTML(swift, { timber: 100, silver: 100 });
  assert.match(html, /Runner to Phaistos \(arrives in 4 game days\)/);
  assert.match(html, /every Runner and caravan of Knossos, on the road now \(1\) and while it lasts · gift ×2/);
  assert.match(html, /all of yours, on the road now \(1\) and while it lasts · gift ×4/);
});

test('speed prayer: with nothing on the road the city and realm steps stay open, the named step is greyed out', () => {
  const empty = { ...swift, targets: { ...swift.targets, unit: [], city: { name: 'Knossos', count: 0 }, realm: { count: 0 } } };
  const choices = scopeChoices(empty);
  assert.deepEqual(choices.map(c => c.enabled), [false, true, true]);
  assert.equal(defaultScope(empty), 'city');
  assert.doesNotMatch(offerRowHTML(empty, {}), /nothing of this kind to bless/);
});

// Blessings of troops/ships (beslut 2026-11-07): city and realm open with no unit of the kind; they cover those that join later.
const arm = { id: 'minoan_velchanos_strength', god_id: 'velchanos', offering: { livestock: 6 }, scope_cost_factors: { unit: 1, city: 2, realm: 4 },
  targets: { always: true, unit: [], city: { name: 'Knossos', count: 0 }, realm: { count: 0 } } };

test('troop blessing with no troops: city and realm are open, the unit step is greyed out, no "nothing to bless"', () => {
  const choices = scopeChoices(arm);
  assert.deepEqual(choices.map(c => c.enabled), [false, true, true]);
  assert.equal(defaultScope(arm), 'city');
  const html = offerRowHTML(arm, {});
  assert.match(html, /any supported by Knossos while it lasts · gift ×2/);
  assert.match(html, /any of yours while it lasts · gift ×4/);
  assert.doesNotMatch(html, /nothing of this kind to bless/);
  assert.doesNotMatch(html, /Runner|caravan|on the road/);
});

test('troop blessing with troops: the count is told, and those that join later are included', () => {
  const some = { ...arm, targets: { ...arm.targets, city: { name: 'Knossos', count: 2 }, realm: { count: 5 } } };
  const html = offerRowHTML(some, {});
  assert.match(html, /2 supported by Knossos now, and any that join while it lasts · gift ×2/);
  assert.match(html, /all 5 of yours now, and any that join while it lasts · gift ×4/);
});

// Storm prayers (steg 3b): a storm picked from the storm map, or a sea hex typed as q,r. No scope steps, no price steps.
const calm = { id: 'minoan_stillness_calm', god: 'The Sea Daimons', god_id: 'sea_daimons_stillness', offering: { fish: 10, oil: 6 },
  targets: { unit: [], city: { count: 0 }, realm: { count: 0 }, kind: 'storm', storms: [{ id: 's-1', name: 'Old Grey', where: 'last seen near (4,4)' }] } };
const wake = { id: 'minoan_stillness_wake', god: 'The Sea Daimons', god_id: 'sea_daimons_stillness', offering: { fish: 10, wine: 6 },
  targets: { unit: [], city: { count: 0 }, realm: { count: 0 }, kind: 'hex' } };

test('Still the Storm offers the named storms of your map and no scope steps', () => {
  const html = offerRowHTML(calm, { fish: 20, oil: 20 });
  assert.match(html, /class="offer-target-storm"/);
  assert.match(html, /Old Grey \(last seen near \(4,4\)\)/);
  assert.doesNotMatch(html, /type="radio"|gift ×|Who is blessed/);
  assert.match(html, /data-good="fish" value="10"/);
});

test('Still the Storm with no named storm says so and offers no select', () => {
  const html = offerRowHTML({ ...calm, targets: { ...calm.targets, storms: [] } }, {});
  assert.match(html, /No named storm on your storm map/);
  assert.doesNotMatch(html, /offer-target-storm/);
});

test('Wake the Storm asks for a sea hex as q,r', () => {
  const html = offerRowHTML(wake, { fish: 20, wine: 20 });
  assert.match(html, /class="offer-target-hex"/);
  assert.match(html, /placeholder="q,r"/);
  assert.doesNotMatch(html, /type="radio"/);
});

test('chosenTarget reads the storm select and the hex field', () => {
  const host = (sel) => ({ querySelector: (q) => sel[q] || null });
  assert.deepEqual(chosenTarget(host({ '.offer-target-storm': { value: 's-1' } })), { target: 's-1' });
  assert.deepEqual(chosenTarget(host({ '.offer-target-hex': { value: ' 7,3 ' } })), { target: '7,3' });
});

// The Walls Fall (steg 5c): a foreign city picked from the list, no scope steps.
const wallsFall = { id: 'minoan_potnia_walls_fall', god: 'Potnia', god_id: 'potnia', offering: { livestock: 12, silver: 8 },
  targets: { unit: [], city: { count: 0 }, realm: { count: 0 }, kind: 'city', cities: [{ id: 'c-1', name: 'Mycenae', where: "Agamemnon's, wall 2 when last seen" }] } };

test('The Walls Fall offers the remembered walled foreign cities, says it will not be told, and has no scope steps', () => {
  const html = offerRowHTML(wallsFall, { livestock: 20, silver: 20 });
  assert.match(html, /class="offer-target-city"/);
  assert.match(html, /Mycenae \(Agamemnon's, wall 2 when last seen\)/);
  assert.match(html, /you will not be told/);
  assert.match(html, /already under way at the walls is not changed/);
  assert.doesNotMatch(html, /type="radio"|gift ×|Who is blessed/);
  assert.match(html, /data-good="livestock" value="12"/);
  assert.deepEqual(chosenTarget({ querySelector: (q) => (q === '.offer-target-city' ? { value: 'c-1' } : null) }), { target: 'c-1' });
});

test('The Walls Fall with no remembered walled city says so and offers no select', () => {
  const html = offerRowHTML({ ...wallsFall, targets: { ...wallsFall.targets, cities: [] } }, {});
  assert.match(html, /No foreign city with a wall you remember/);
  assert.doesNotMatch(html, /offer-target-city/);
});

// Permanent (steg 4): a fourth radio, "For ever", for one named unit, x20; only prayers that have the step.
const forever = { ...blessing, scope_cost_factors: { unit: 1, city: 2, realm: 4, permanent: 20 },
  targets: { ...blessing.targets, unit: [{ id: 'u1', name: 'Horns of Crete', where: 'in Knossos' }, { id: 'u2', name: 'Argo', where: 'sailing at (3,1)', forever: true }] } };

test('permanent step: a fourth radio "For ever" at the x20 gift, and units that already have it say so', () => {
  const html = offerRowHTML(forever, { fish: 1000, oil: 1000 });
  assert.match(html, /For ever — the one you choose below, and never a city; 1 without it yet · gift ×20/);
  assert.match(html, /name="scope-minoan_stillness_hull" value="permanent"/);
  assert.match(html, /Argo \(sailing at \(3,1\)\), blessed for ever/);
  assert.deepEqual(scaledRecipe(forever.offering, 20), { fish: 160, oil: 80 });
});

test('permanent step: not offered by a prayer without it, and greyed out when every unit already has it', () => {
  assert.doesNotMatch(offerRowHTML(blessing, { fish: 100 }), /value="permanent"/);
  assert.doesNotMatch(offerRowHTML(swift, {}), /value="permanent"/);
  const all = { ...forever, targets: { ...forever.targets, unit: [{ id: 'u2', name: 'Argo', where: 'x', forever: true }] } };
  assert.equal(scopeChoices(all).find(c => c.scope === 'permanent').enabled, false);
  assert.equal(scopeChoices(forever).find(c => c.scope === 'permanent').enabled, true);
});

test('chosenTarget reads the unit for the permanent step as for the named one', () => {
  const host = (scope, unit) => ({ querySelector: sel => sel.includes('checked') ? { value: scope } : sel.includes('offer-target-unit') ? { value: unit } : null });
  assert.deepEqual(chosenTarget(host('permanent', 'u1')), { target_scope: 'permanent', target: 'u1' });
});

// World blessings (steg 5a): a city or the realm, no unit to name; the scapegoat only the realm.
const worldPrayer = { id: 'minoan_potnia_fertility', god: 'Potnia', offering: { livestock: 4, grain: 4 }, can_prepare_offering: true, affordable: true,
  scope_cost_factors: { city: 2, realm: 4 },
  targets: { kind: 'world', always: true, unit: [], city: { name: 'Knossos', count: 1 }, realm: { count: 3 } } };

test('a world blessing offers only city and realm, both open, with the step price', () => {
  const c = scopeChoices(worldPrayer);
  assert.deepEqual(c.map(x => [x.scope, x.enabled, x.factor]), [['city', true, 2], ['realm', true, 4]]);
  assert.equal(defaultScope(worldPrayer), 'city');
  const html = offerRowHTML(worldPrayer, { livestock: 50, grain: 50 });
  assert.match(html, /Knossos only · gift ×2/);
  assert.match(html, /all 3 of your cities · gift ×4/);
  assert.doesNotMatch(html, /One named|For ever|offer-target-unit/);
  assert.match(html, /How wide is it laid\?/);
});

test('the scapegoat offers the realm alone', () => {
  const goat = { ...worldPrayer, id: 'minoan_potnia_scapegoat', scope_cost_factors: { realm: 4 } };
  const c = scopeChoices(goat);
  assert.deepEqual(c.map(x => x.scope), ['realm']);
  assert.equal(defaultScope(goat), 'realm');
  const host = { querySelector: s => s === 'input[type=radio]:checked' ? { value: 'realm' } : null };
  assert.deepEqual(chosenTarget(host), { target_scope: 'realm' });
});

// The Thread (steg 5b): land caravans only — a Runner is never taken, so the city step must not talk of one.
const thread = { id: 'minoan_asterion_thread_v2', god_id: 'asterion', offering: { wine: 3, silver: 2 }, scope_cost_factors: { unit: 1, city: 2, realm: 4 },
  targets: { always: true, unit: [{ id: 'c1', name: 'Caravan to Phaistos', where: 'arrives in 4 game days' }], city: { name: 'Knossos', count: 1 }, realm: { count: 1 } } };

test('the Thread: caravans are named, the city step says land caravans and never a Runner, and there is no fourth step', () => {
  const html = offerRowHTML(thread, { wine: 100, silver: 100 });
  assert.match(html, /Caravan to Phaistos \(arrives in 4 game days\)/);
  assert.match(html, /every land caravan of Knossos, on the road now \(1\) and while it lasts · gift ×2/);
  assert.doesNotMatch(html, /Runner/);
  assert.doesNotMatch(html, /For ever/);
});

test('the Bull\'s Horns: troops by name, city and realm by count, no fourth step', () => {
  const horns = { id: 'minoan_asterion_horns', god_id: 'asterion', offering: { livestock: 5, silver: 2 }, scope_cost_factors: { unit: 1, city: 2, realm: 4 },
    targets: { unit: [{ id: 'u1', name: 'Royal Guard', where: 'in Knossos' }], city: { name: 'Knossos', count: 1 }, realm: { count: 3 } } };
  const html = offerRowHTML(horns, { livestock: 100, silver: 100 });
  assert.match(html, /Royal Guard \(in Knossos\)/);
  assert.match(html, /1 of Knossos · gift ×2/);
  assert.match(html, /all 3 · gift ×4/);
  assert.doesNotMatch(html, /For ever/);
});

// The Bird Oracle (steg 5d): a city or a named place from one list, no scope steps.
const birdOracle = { id: 'minoan_asterion_bird_oracle', god: 'Asterion', god_id: 'asterion', offering: { silver: 10, wine: 8 },
  targets: { unit: [], city: { count: 0 }, realm: { count: 0 }, kind: 'place', cities: [{ id: 'c-1', name: 'Mycenae', where: 'held by Agamemnon, near (20,20)' }],
    places: [{ id: 'Sunken Cape', name: 'Sunken Cape', where: 'cape, near (70,70)' }] } };

test('The Bird Oracle offers known cities and named places, says what is seen and that the watched are told nothing', () => {
  const html = offerRowHTML(birdOracle, { silver: 20, wine: 20 });
  assert.match(html, /class="offer-target-place"/);
  assert.match(html, /<optgroup label="Cities">[^]*Mycenae/);
  assert.match(html, /<optgroup label="Named places">[^]*Sunken Cape/);
  assert.match(html, /within 2 hexes/);
  assert.match(html, /told nothing/);
  assert.doesNotMatch(html, /type="radio"|gift ×/);
  assert.deepEqual(chosenTarget({ querySelector: (q) => (q === '.offer-target-place' ? { value: 'Sunken Cape' } : null) }), { target: 'Sunken Cape' });
});

test('The Bird Oracle with no known place says so and offers no select', () => {
  const html = offerRowHTML({ ...birdOracle, targets: { ...birdOracle.targets, cities: [], places: [] } }, {});
  assert.match(html, /No named place on your map/);
  assert.doesNotMatch(html, /offer-target-place/);
});

test('world prayer with exactly one city says "your one city", not "all 1"', () => {
  const one = { ...blessing, targets: { ...blessing.targets, kind: 'world', realm: { count: 1 }, city: { name: 'Knossos', count: 1 } } };
  const html = offerRowHTML(one, { fish: 100, oil: 100 });
  assert.match(html, /your one city/);
  assert.doesNotMatch(html, /all 1 of your cities/);
  const three = offerRowHTML({ ...one, targets: { ...one.targets, realm: { count: 3 } } }, { fish: 100, oil: 100 });
  assert.match(three, /all 3 of your cities/);
});

// ── Pick on the map (Wake the Storm) ────────────────────────────────────────
const { stormHexRefusal, stormHexText, targetChooserHTML } = await import('./kult_offer.js');

test('Wake the Storm: the field has a Pick on the map button and stays an editable text input', () => {
  const html = targetChooserHTML({ targets: { kind: 'hex' } });
  assert.match(html, /data-pick-storm-hex/);
  assert.match(html, />Pick on the map</);
  assert.match(html, /class="offer-target-hex"/);
});

test('Wake the Storm: only seen sea is accepted; the reasons are plain', () => {
  assert.equal(stormHexRefusal({ terrain: 'deep_sea' }), null);
  assert.equal(stormHexRefusal({ terrain: 'coastal_sea' }), null);
  assert.equal(stormHexRefusal({ terrain: 'plains' }), 'Not a sea hex');
  assert.equal(stormHexRefusal({ terrain: 'lake' }), 'Not a sea hex');
  assert.equal(stormHexRefusal({ terrain: 'fog' }), 'You have not seen this ground');
  assert.equal(stormHexRefusal(undefined), 'You have not seen this ground');
});

test('Wake the Storm: a picked hex fills the field as q,r and the rite target is unchanged', () => {
  assert.equal(stormHexText({ q: 7, r: -3 }), '7,-3');
  assert.deepEqual(chosenTarget({ querySelector: q => q === '.offer-target-hex' ? { value: stormHexText({ q: 7, r: -3 }) } : null }), { target: '7,-3' });
});

// ── Gift steps (bon-gavosteg 2026-11-12): the size of the gift as 1, 2, 3, 5 times ──
const { giftSteps, giftStepsHTML, GIFT_STEPS } = await import('./kult_offer.js');
const lasting = { id: 'minoan_stillness_hull', god_id: 'sea_daimons_stillness', base_duration_game_days: 6, offering: { fish: 8, oil: 4 },
  scope_cost_factors: { unit: 1, city: 2, realm: 4, permanent: 20 },
  targets: { unit: [{ id: 'u1', name: 'Argo', where: 'in Knossos' }], city: { name: 'Knossos', count: 2 }, realm: { count: 3 } } };
const rich = { fish: 500, oil: 500 };

test('gift steps: 1, 2, 3 and 5 times; length and price follow the scope', () => {
  assert.deepEqual(GIFT_STEPS, [1, 2, 3, 5]);
  const unit = giftSteps(lasting, 'unit', rich);
  assert.deepEqual(unit.map(s => [s.n, s.days]), [[1, 6], [2, 12], [3, 18], [5, 30]]);
  assert.deepEqual(unit[1].goods, { fish: 16, oil: 8 });
  const city = giftSteps(lasting, 'city', rich);
  assert.deepEqual(city.map(s => s.days), [6, 12, 18, 30]);
  assert.deepEqual(city[3].goods, { fish: 80, oil: 40 });
  assert.ok(unit.every(s => s.payable && !s.reason));
});

test('gift steps: one the city cannot pay is disabled and says why in text', () => {
  const steps = giftSteps(lasting, 'unit', { fish: 9, oil: 100 });
  assert.deepEqual(steps.map(s => s.payable), [true, false, false, false]);
  assert.equal(steps[1].reason, 'needs 16 fish, city has 9');
  const html = giftStepsHTML(lasting, 'unit', { fish: 9, oil: 100 });
  assert.match(html, /12 game days · 16 fish \+ 8 oil[^]*needs 16 fish, city has 9/);
  assert.match(html, /value="2" disabled/);
  assert.match(html, /value="1" checked/);
  assert.match(html, /How much do you give\?/);
});

test('gift steps: the first affordable step is checked, and a kept choice survives a scope change', () => {
  assert.match(giftStepsHTML(lasting, 'city', { fish: 40, oil: 40 }), /value="1" checked[^]*value="3" disabled/);
  assert.match(giftStepsHTML(lasting, 'unit', rich, 3), /value="3" checked/);
  assert.doesNotMatch(giftStepsHTML(lasting, 'unit', { fish: 9, oil: 9 }, 3), /value="3" checked/);
});

test('gift steps: an instant prayer and the permanent step have no steps, only one line with the price', () => {
  const instant = { ...lasting, base_duration_game_days: 0 };
  assert.deepEqual(giftSteps(instant, 'unit', rich), []);
  assert.match(giftStepsHTML(instant, 'unit', rich), /Gift: 8 fish \+ 4 oil/);
  assert.doesNotMatch(giftStepsHTML(instant, 'unit', rich), /type="radio"/);
  assert.deepEqual(giftSteps(lasting, 'permanent', rich), []);
  const html = giftStepsHTML(lasting, 'permanent', rich);
  assert.match(html, /Gift: 160 fish \+ 80 oil/);
  assert.doesNotMatch(html, /type="radio"/);
});

test('offer row: steps under the chooser, the fields tucked into a closed "Offer other goods"', () => {
  const html = offerRowHTML(lasting, rich);
  assert.match(html, /data-offer-gifts/);
  assert.match(html, /name="gift-minoan_stillness_hull"/);
  assert.match(html, /The temple rests longer after a larger gift\./);
  assert.match(html, /<details class="offer-other">\s*<summary>Offer other goods<\/summary>[^]*data-good="fish"/);
  assert.doesNotMatch(html, /<details class="offer-other" open/);
  assert.doesNotMatch(html, /Lasts 6 game days with the traditional gift/);
  assert.doesNotMatch(html, /tick/i);
});

test('offer row: when the city lacks the recipe the other goods are open from the start', () => {
  const html = offerRowHTML({ ...lasting, can_prepare_offering: true, affordable: false }, { fish: 1, oil: 1 });
  assert.match(html, /<details class="offer-other" open/);
});

test('step 1 sends no offering, step 3 sends the exact goods, a changed field wins (fields mirror the checked step)', () => {
  const recipe = scaledRecipe(lasting.offering, 1);
  const entries = goods => Object.entries(goods).map(([good, value]) => ({ good, value: String(value) }));
  assert.equal(offeringFromEntries(entries(giftSteps(lasting, 'unit', rich)[0].goods), recipe), null);
  assert.deepEqual(offeringFromEntries(entries(giftSteps(lasting, 'unit', rich)[2].goods), recipe), { fish: 24, oil: 12 });
  const cityRecipe = scaledRecipe(lasting.offering, 2);
  assert.deepEqual(offeringFromEntries(entries(giftSteps(lasting, 'city', rich)[1].goods), cityRecipe), { fish: 32, oil: 16 });
  assert.deepEqual(offeringFromEntries([{ good: 'fish', value: '8' }, { good: 'oil', value: '4' }, { good: 'wine', value: '5' }], recipe), { fish: 8, oil: 4, wine: 5 });
});
