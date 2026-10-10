import { test } from 'node:test';
import assert from 'node:assert/strict';
import { stormPanelHTML } from './stormname.js';
import { unitHoverLines } from './hover.js';

test('stormPanelHTML: nothing without a storm; a named storm shows its name and no form', () => {
  assert.equal(stormPanelHTML([]), '');
  const html = stormPanelHTML([{ id: 'abc', name: 'Skyla', tier: 'live', can_name: false }]);
  assert.match(html, /Storm Skyla/);
  assert.doesNotMatch(html, /<input/);
});

test('stormPanelHTML: only a storm you may name carries the form; names are escaped', () => {
  const html = stormPanelHTML([{ id: 'abc', name: '', tier: 'remembered', seen_tick: 4, can_name: true }]);
  assert.match(html, /last seen on day 4/);
  assert.match(html, /id="storm-name-abc"/);
  assert.match(html, /onclick="nameStorm\('abc'\)"/);
  assert.match(stormPanelHTML([{ id: 'x', name: '<b>', tier: 'live' }]), /Storm &lt;b&gt;/);
});

test('hover: a named storm is called by its name', () => {
  const lines = unitHoverLines({ placeName: () => '', storms: [{ tier: 'live', heading: 'NE', name: 'Skyla' }, { tier: 'remembered', seen_tick: 3 }] });
  assert.deepEqual(lines, ['Storm Skyla — drifting NE', 'Storm — last seen on day 3']);
});
