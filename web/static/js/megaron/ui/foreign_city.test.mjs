import test from 'node:test';
import assert from 'node:assert/strict';
globalThis.window ??= { addEventListener() {}, matchMedia: () => ({ matches: false, addEventListener() {} }) };
globalThis.localStorage ??= { getItem: () => null, setItem() {} };
const { foreignIntelHTML } = await import('./foreign_city.js');

test('a city seen but never entered says so and invites contact', () => {
  const html = foreignIntelHTML({ size: 'town', buildings: [{ name: 'Granary' }], intel: { level: 'seen', seen_at: '2026-10-30T14:20:00Z', seen_live: true } });
  assert.match(html, /Seen now: town, Granary\./);
  assert.match(html, /Send a messenger/);
});

test('a contact shows its words, its source and building levels, never a number of its own making', () => {
  const html = foreignIntelHTML({ size: 'town', intel: { level: 'contact', contact_source: 'messenger', contact_at: '2026-10-27T09:10:00Z',
    contact: { size: 'town', food: 'hungry', goods: [{ good: 'oil', word: 'plenty of' }], buildings: [{ name: 'Granary', level: 2 }] } } });
  assert.match(html, /Your messenger was there .*: town, hungry, plenty of oil\./);
  assert.match(html, /Buildings then: Granary L2\./);
});

test('an own city has no intel and renders nothing', () => {
  assert.equal(foreignIntelHTML({ name: 'Mine' }), '');
});
