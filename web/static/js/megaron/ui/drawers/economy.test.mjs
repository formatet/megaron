import test from 'node:test';
import assert from 'node:assert/strict';
import { State } from '../../state.js';
import { loadEconomyDrawer, loadTransferGoods, startTransfer, atStorageCeiling, goodsRateCell } from './economy.js';

test('atStorageCeiling: mirrors keryx — >=99% of a positive cap, nothing without a cap', () => {
  assert.equal(atStorageCeiling(1000000, 1000000), true);
  assert.equal(atStorageCeiling(990000, 1000000), true);   // exactly 99%
  assert.equal(atStorageCeiling(989999, 1000000), false);  // just under
  assert.equal(atStorageCeiling(1000000, 0), false);       // no cap = never full
  assert.equal(atStorageCeiling(50, -1), false);
});

test('goodsRateCell: a good at cap reads "full" (dimmed) not green growth, and names the wasted rate', () => {
  const html = goodsRateCell({ amount: 1000000, cap: 1000000, rate_per_tick: 12.3 });
  assert.match(html, /class="goods-atcap"/);   // dimmed, not --safe green
  assert.match(html, /full/);
  assert.match(html, /\+12\.3 lost/);           // the wasted labour is visible
  assert.doesNotMatch(html, /--safe/);
});

test('goodsRateCell: at cap but idle (rate 0) reads plain "full", no wasted-rate tail', () => {
  const html = goodsRateCell({ amount: 1000000, cap: 1000000, rate_per_tick: 0 });
  assert.match(html, /class="goods-atcap"/);
  assert.match(html, />full</);
  assert.doesNotMatch(html, /lost/);
});

test('goodsRateCell: below cap and producing keeps the green growth cell', () => {
  const html = goodsRateCell({ amount: 500, cap: 1000000, rate_per_tick: 4.5 });
  assert.match(html, /var\(--safe\)/);
  assert.match(html, /\+4\.5\/tick/);
  assert.doesNotMatch(html, /goods-atcap/);
});

test('goodsRateCell: below cap and idle renders an empty cell', () => {
  assert.equal(goodsRateCell({ amount: 500, cap: 1000000, rate_per_tick: 0 }), '<td></td>');
});

test('AK1: economy.js imports under node --test without a DOM (proves the misc.js pointerdown listener no longer runs at module scope)', () => {
  assert.equal(typeof loadEconomyDrawer, 'function');
  assert.equal(typeof loadTransferGoods, 'function');
  assert.equal(typeof startTransfer, 'function');
});

test('loadEconomyDrawer: with no owned settlements, renders the empty-state message and returns before touching anything else', async () => {
  const bodyStub = { innerHTML: '' };
  const calls = [];
  globalThis.document = {
    getElementById(id) {
      calls.push(id);
      return id === 'economy-body' ? bodyStub : null;
    },
  };
  State.provinceData = []; // no own settlements at all
  await loadEconomyDrawer();
  assert.match(bodyStub.innerHTML, /No settlements/);
  // Only the one lookup for #economy-body — the early return means it never
  // goes on to build tabs or fetch goods.
  assert.deepEqual(calls, ['economy-body']);
  delete globalThis.document;
});
