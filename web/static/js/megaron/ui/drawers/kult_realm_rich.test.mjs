import test from 'node:test';
import assert from 'node:assert/strict';
import { richOfferingControlHTML } from './kult_realm.js';

test('rich offering choice names the god\'s taste beside a good it prizes', () => {
  const html = richOfferingControlHTML({ id: 't1', offer: { stock: { fish: 3, wine: 2, tin: 0 }, rich_offering_tastes: { fish: 2 } } });
  assert.match(html, /<option value="fish">fish \(×2\)<\/option>/);
  assert.match(html, /<option value="wine">wine<\/option>/, 'an unlisted good has no taste mark');
  assert.doesNotMatch(html, /value="tin"/, 'goods not in store are not offered');
});

test('a chosen rich offering shows only the stop button', () => {
  assert.match(richOfferingControlHTML({ id: 't1', offer: { rich_offering_good: 'fish', stock: { fish: 3 } } }), /Stop rich offering/);
});
