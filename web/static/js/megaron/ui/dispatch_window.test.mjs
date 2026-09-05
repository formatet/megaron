import test from 'node:test';
import assert from 'node:assert/strict';

// dispatch_window.js has NO top-level DOM/window touches (its whole point —
// see the file's own header comment on why centreOn is reached via
// window.centreOn instead of a direct ui/search.js import), so this file
// needs no globalThis stubbing before import, unlike its DOM-heavy siblings.
import { resolveDestination } from './dispatch_window.js';
import { State } from '../state.js';

test('AK1: a direct q/r on the payload wins outright, no lookup needed', () => {
  State.provinceData = [];
  assert.deepEqual(resolveDestination('UnitArrived', { q: 5, r: 9 }), { q: 5, r: 9 });
});

test('AK2: settlement_id resolves via State.provinceData', () => {
  State.provinceData = [{ id: 'sid-1', q: 3, r: 4 }];
  assert.deepEqual(resolveDestination('BuildComplete', { settlement_id: 'sid-1' }), { q: 3, r: 4 });
});

test('AK3: dest_id/destination_id (transport/trade kinds) resolve the same way settlement_id does', () => {
  State.provinceData = [{ id: 'sid-2', q: 7, r: 1 }];
  assert.deepEqual(resolveDestination('TradeDelivery', { dest_id: 'sid-2' }), { q: 7, r: 1 });
  assert.deepEqual(resolveDestination('TradeReturn', { destination_id: 'sid-2' }), { q: 7, r: 1 });
});

test('AK4: OfferAccepted/Declined/Expired go to the COUNTERPARTY city, not the recipient\'s own (Timothy 2026-09-04, megaron_plan_dispatches.md §3)', () => {
  State.provinceData = [
    { id: 'own-city', q: 1, r: 1 },
    { id: 'their-city', q: 20, r: 20 },
  ];
  const payload = { settlement_id: 'own-city', counterparty_id: 'their-city' };
  assert.deepEqual(resolveDestination('OfferAccepted', payload), { q: 20, r: 20 });
  assert.deepEqual(resolveDestination('OfferDeclined', payload), { q: 20, r: 20 });
  assert.deepEqual(resolveDestination('OfferExpired', payload), { q: 20, r: 20 });
});

test('AK5: OfferAccepted falls back to settlement_id if counterparty_id is somehow absent', () => {
  State.provinceData = [{ id: 'own-city', q: 1, r: 1 }];
  assert.deepEqual(resolveDestination('OfferAccepted', { settlement_id: 'own-city' }), { q: 1, r: 1 });
});

test('AK6: a non-offer kind never picks up counterparty_id even if present', () => {
  State.provinceData = [
    { id: 'own', q: 2, r: 2 },
    { id: 'other', q: 9, r: 9 },
  ];
  assert.deepEqual(
    resolveDestination('BuildComplete', { settlement_id: 'own', counterparty_id: 'other' }),
    { q: 2, r: 2 },
  );
});

test('AK7: an unresolvable settlement_id falls through to a unit_id lookup', () => {
  State.provinceData = [];
  State.unitsData = [{ id: 'u1', q: 11, r: 12 }];
  assert.deepEqual(resolveDestination('OrderFailed', { settlement_id: 'ghost', unit_id: 'u1' }), { q: 11, r: 12 });
});

test('AK8: a foreign unit (not own) is still found via State.foreignUnitData', () => {
  State.provinceData = [];
  State.unitsData = [];
  State.foreignUnitData = [{ id: 'f1', q: 30, r: 31 }];
  assert.deepEqual(resolveDestination('SentryAlerted', { ship_id: 'f1' }), { q: 30, r: 31 });
});

test('AK9: no destination anywhere returns null (the disabled-button case)', () => {
  State.provinceData = [];
  State.unitsData = [];
  State.foreignUnitData = [];
  assert.equal(resolveDestination('WorldAnnouncement', {}), null);
  assert.equal(resolveDestination('WorldAnnouncement', null), null);
});
