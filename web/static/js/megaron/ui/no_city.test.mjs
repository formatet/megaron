import test from 'node:test';
import assert from 'node:assert/strict';
import { noCityText } from './no_city.js';

test('an empty window points at the Host while a Host exists', () => {
  for (const what of ['No city founded yet.', 'No settlements yet.', 'No capital yet.']) {
    assert.match(noCityText(what, { active: true }), /Click your Host on the map, then press Found the metropolis here\./);
    assert.equal(noCityText(what, null), what, 'no Host: nothing to point at');
  }
});
