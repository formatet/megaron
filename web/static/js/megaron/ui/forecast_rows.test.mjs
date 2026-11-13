import test from 'node:test';
import assert from 'node:assert/strict';
import { forecastRows } from './forecast_rows.js';

const cat = (terrain, extra = {}) => ({ known: true, terrain, ...extra });
const siteA = {
  goods: { copper: 2, grain: 7, stone: 25, timber: 6.96 },
  grain: { base_per_tick: 7.5, fish_per_tick: 0, consumption_per_tick: 20, food_net_per_tick: -12.5 },
  harbour_note: 'Inland: this hex touches no water',
  unknown_hexes: 1,
  catchment: [cat('hills', { copper_deposit: true }), cat('hills', { copper_deposit: true }), cat('plains'), cat('scrub_maquis'), { known: false }],
};
const siteB = {
  goods: { timber: 1, grain: 20 },
  grain: { base_per_tick: 20, fish_per_tick: 4, consumption_per_tick: 20, food_net_per_tick: 4 },
  harbour_note: 'Coastal: a harbour is possible here',
  unknown_hexes: 0,
  catchment: [cat('plains'), cat('plains'), cat('coastal_sea', { silver_deposit: true })],
};
const text = rows => rows.map(r => `${r.label}: ${r.value}`).join(' | ');

test('forecast rows give whole numbers, not yes/no', () => {
  const t = text(forecastRows(siteA, s => s));
  assert.match(t, /Timber: up to 7 a game day, fully worked/);
  assert.match(t, /Stone: up to 25 a game day, fully worked/);
  assert.match(t, /Copper: 2 hexes/);
  assert.match(t, /Tin: none seen/);
  assert.match(t, /Silver: none seen/);
  assert.match(t, /Food: 8 grain \+ 0 fish a game day, your people eat 20/);
  assert.match(t, /Ships: no ships ever — inland/);
  assert.doesNotMatch(t, /\byes\b/);
});

test('two different sites read differently', () => {
  assert.notEqual(text(forecastRows(siteA, s => s)), text(forecastRows(siteB, s => s)));
  const b = text(forecastRows(siteB, s => s));
  assert.match(b, /Silver: 1 hex\b/);
  assert.match(b, /Stone: none/);
  assert.match(b, /Ships: yes, coast/);
});

test('ground row counts known hexes by terrain, most common first', () => {
  const g = forecastRows(siteA, s => s).find(r => r.label === 'Ground');
  assert.equal(g.value, '2 hills, 1 plains, 1 scrub_maquis');
});

test('no forecast data: every row says unknown, never a guess', () => {
  assert.ok(forecastRows(null, s => s).every(r => r.value === 'unknown'));
});
