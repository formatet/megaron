import test from 'node:test';
import assert from 'node:assert/strict';
import { destinationButtons, transferBody, stockedBody, sendFormHTML, routeChoices, allAmount } from './send_goods.js';

const dests = [
  { settlement_id: 'g1', name: 'Gournia', owner_name: 'Myrine', own: false },
  { settlement_id: 'k1', name: 'Kyme', owner_name: 'Me', own: true },
  { settlement_id: 'a1', name: 'Amnisos', owner_name: 'Aias', own: false },
  { settlement_id: 'n1', name: 'Nostos', owner_name: 'Me', own: true },
];
const stock = [{ key: 'grain', name: 'Grain', amount: 240 }, { key: 'wine', name: 'Wine', amount: 30 }];
const landOnly = { options: [{ by: 'land', available: true, travel_ticks: 3, arrives_at: '2026-01-01T12:00:00Z' }, { by: 'sea', available: false, reason: 'No free ship' }] };
const both = { options: [{ by: 'land', available: true, travel_ticks: 3 }, { by: 'sea', available: true, travel_ticks: 1, cargo_capacity: 120, ship_name: 'Naia' }] };
const base = { froms: [{ id: 'pA', settlementId: 'sA', name: 'Phaistos' }], from: { id: 'pA', settlementId: 'sA', name: 'Phaistos' },
  dests: destinationButtons(dests), to: null, stock, amounts: {}, preview: null, by: '', keep: false, homeOpen: false, homeStock: [], home: {} };

test('(a) destinations: own cities first, foreign labelled "city — ruler", no gift suffix', () => {
  const b = destinationButtons(dests);
  assert.deepEqual(b.map(x => x.id), ['k1', 'n1', 'a1', 'g1']);
  assert.equal(b[2].label, 'Amnisos — Aias');
  assert.equal(b[0].label, 'Kyme');
  assert.ok(b.every(x => !/gift/i.test(x.label)));
});

test('(b) stocked order body: crewed_by = from city, outbound = rows, return empty', () => {
  const body = stockedBody({ fromSettlementId: 'sA', toSettlementId: 'k1', items: [{ good_key: 'grain', quantity: 200 }], home: [], by: 'land' });
  assert.equal(JSON.stringify(body), JSON.stringify({ from_settlement_id: 'sA', to_settlement_id: 'k1', crewed_by_settlement_id: 'sA', outbound: [{ good_key: 'grain', threshold: 200 }], return: [], by: 'land' }));
  const home = stockedBody({ fromSettlementId: 'sA', toSettlementId: 'k1', items: [{ good_key: 'grain', quantity: 1 }], home: [{ good_key: 'wine', floor: 0 }], by: '' });
  assert.deepEqual(home.return, [{ good_key: 'wine', floor: 0 }]);
  assert.ok(!('by' in home));
});

test('(c) transfer body is unchanged for the same input', () => {
  const items = [{ good_key: 'grain', quantity: 2.5 }];
  assert.equal(JSON.stringify(transferBody({ to: 'g1', items, by: 'sea' })), JSON.stringify({ destination_id: 'g1', items, by: 'sea' }));
  assert.equal(JSON.stringify(transferBody({ to: 'g1', items, by: '' })), JSON.stringify({ destination_id: 'g1', items }));
});

test('(d) form markup has no select and no "Crewed by"', () => {
  for (const s of [base, { ...base, to: 'g1', keep: true, preview: both, by: 'land' }, { ...base, froms: [base.from, { id: 'pB', settlementId: 'sB', name: 'Kyme' }] }]) {
    const html = sendFormHTML(s);
    assert.doesNotMatch(html, /<select/i);
    assert.doesNotMatch(html, /Crewed by/i);
    assert.doesNotMatch(html, /Add good|Add a good/i);
  }
});

test('single city: no From control, just a line', () => {
  const html = sendFormHTML(base);
  assert.match(html, /From Phaistos/);
  assert.doesNotMatch(html, /data-act="from"/);
});

test('one row per good held, with All', () => {
  const html = sendFormHTML({ ...base, to: 'k1', preview: landOnly, by: 'land' });
  assert.match(html, /Grain/); assert.match(html, /240 in store/); assert.match(html, /data-act="all" data-good="wine"/);
});

test('foreign destination: gift button and one line, no keep checkbox', () => {
  const html = sendFormHTML({ ...base, to: 'g1', preview: landOnly, by: 'land' });
  assert.match(html, /Send as a gift →/); assert.match(html, /A gift: nothing comes back\./);
  assert.doesNotMatch(html, /Keep it stocked/);
});

test('own destination: keep checkbox, checked turns the button', () => {
  assert.match(sendFormHTML({ ...base, to: 'k1', preview: landOnly, by: 'land' }), /data-act="keep"/);
  const html = sendFormHTML({ ...base, to: 'k1', preview: landOnly, by: 'land', keep: true });
  assert.match(html, /Keep it stocked →/); assert.match(html, /More/);
});

test('(e) one available route renders one line, no chooser; two render buttons', () => {
  const one = sendFormHTML({ ...base, to: 'k1', preview: landOnly, by: 'land' });
  assert.match(one, /By land · arrives .* · 3 game days/);
  assert.doesNotMatch(one, /data-act="by"/); assert.doesNotMatch(one, /type="radio"/);
  const two = sendFormHTML({ ...base, to: 'k1', preview: both, by: 'land' });
  assert.equal((two.match(/data-act="by"/g) || []).length, 2);
});

test('capacity: line only when cargo does not fit; All is bounded by the ship', () => {
  const s = { ...base, to: 'k1', preview: both, by: 'sea', amounts: { grain: '100', wine: '30' } };
  assert.match(sendFormHTML(s), /Too much: the Naia carries 120/);
  assert.doesNotMatch(sendFormHTML({ ...s, amounts: { grain: '10' } }), /Too much/);
  assert.equal(allAmount({ ...s, amounts: { grain: '100' } }, 'wine'), 20);
  assert.equal(allAmount({ ...s, amounts: {} }, 'grain'), 120);
  assert.equal(allAmount({ ...s, by: 'land', preview: both }, 'grain'), 240);
});

test('routeChoices: available only', () => {
  assert.deepEqual(routeChoices(both).map(o => o.by), ['land', 'sea']);
  assert.deepEqual(routeChoices(landOnly).map(o => o.by), ['land']);
});
