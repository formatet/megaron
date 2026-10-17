import test from 'node:test';
import assert from 'node:assert/strict';
import { divineValuesHTML } from './kult_values.js';

test('altar shows current divine values, not Goods labour units or guessed reference values', () => {
  const html = divineValuesHTML({bronze:37.5,grain:2.5,'<oil>':0.00012});
  assert.match(html,/How the gods weigh gifts/);
  assert.match(html,/37.5/);assert.match(html,/2.5/);assert.match(html,/0.00012/);
  assert.match(html,/&lt;oil&gt;/);assert.doesNotMatch(html,/<oil>/);
  assert.doesNotMatch(html,/workdays?/i);
  assert.match(divineValuesHTML(null),/unavailable/);
});
