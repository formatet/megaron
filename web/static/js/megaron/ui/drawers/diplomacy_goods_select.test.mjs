import test from 'node:test';
import assert from 'node:assert/strict';

// megaron_plan_offertens_varulista.md: the trade-offer form's four good
// fields (want_good/offer_good, inline-thread compose + Compose tab) used to
// be free text — a stray letter produced a dead offer with escrowed silver
// locked for OfferExpiryTicks (168 ticks) before anyone noticed.
// goodsOptionsHTML/goodsSelectDisabledAttr (diplomacy.js) are the pure half
// of the fix: given the /api/v1/goods catalogue response, build the <select>
// markup and its disabled state, with no DOM or network touched — following
// the render/camera.test.mjs and cargo.test.mjs pattern of testing the pure
// function directly rather than the DOM-mutating caller.
//
// diplomacy.js imports cleanly under a plain `node --input-type=module`
// import with no `document` defined at all (verified by hand before writing
// this file) — unlike economy.js, its import graph no longer touches
// misc.js's module-top-level `document.addEventListener` (that autostart
// call moved into initMusicAutostart(), called explicitly from main.js), so
// none of the cargo.test.mjs/economy.test.mjs global-document-stub dance is
// needed here. A static import is used deliberately, as a standing check:
// if a future change reintroduces a module-top-level DOM touch, this file's
// own import line breaks loudly instead of silently degrading into the
// stub workaround.
const { goodsOptionsHTML, goodsSelectDisabledAttr } = await import('./diplomacy.js');

const GOODS = [
  { key: 'copper', name: 'Copper', tier: 'commodity', category: 'strategic' },
  { key: 'grain', name: 'Grain', tier: 'commodity', category: 'staple' },
];

test('AK1: goodsOptionsHTML renders one <option> per good, value = key', () => {
  const html = goodsOptionsHTML(GOODS);
  assert.match(html, /<option value="copper">Copper<\/option>/);
  assert.match(html, /<option value="grain">Grain<\/option>/);
});

test('AK1b: goodsOptionsHTML leads with an empty "choose" option — no good is silently pre-selected', () => {
  const html = goodsOptionsHTML(GOODS);
  assert.match(html, /^<option value="">— choose good —<\/option>/);
});

test('AK2 (falsifiable): an empty catalogue ([]) reads "no tradeable goods", not a blank list', () => {
  const html = goodsOptionsHTML([]);
  assert.match(html, /No tradeable goods/);
  assert.doesNotMatch(html, /<option value="[a-z]/, 'must not render a real-looking option for an empty catalogue');
});

test('AK3 (honest empty state): a failed fetch (null) reads "Could not load goods" — never silently falls back to free text', () => {
  const html = goodsOptionsHTML(null);
  assert.match(html, /Could not load goods/);
});

test('AK3b: goodsSelectDisabledAttr disables the control exactly when the catalogue failed to load (null), never otherwise', () => {
  assert.equal(goodsSelectDisabledAttr(null), ' disabled');
  assert.equal(goodsSelectDisabledAttr([]), '');
  assert.equal(goodsSelectDisabledAttr(GOODS), '');
});

test('AK4: goodsOptionsHTML escapes a hostile good name so it cannot inject markup', () => {
  const html = goodsOptionsHTML([{ key: 'grain', name: '<img src=x onerror=alert(1)>' }]);
  assert.doesNotMatch(html, /<img/);
});

test('AK5: goodsOptionsHTML falls back to the raw key as the label when name is missing', () => {
  const html = goodsOptionsHTML([{ key: 'stone' }]);
  assert.match(html, /<option value="stone">stone<\/option>/);
});
