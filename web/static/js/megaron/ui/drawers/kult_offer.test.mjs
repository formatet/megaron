import test from 'node:test';
import assert from 'node:assert/strict';
const { offerFields, offerRowHTML, offeringFromEntries } = await import('./kult_offer.js');

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
  assert.match(html, /Potnia favours wine ×1\.8, bronze ×1\.4 · expects an offering worth ~45/);
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
