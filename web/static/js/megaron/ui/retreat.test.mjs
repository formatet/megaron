import test from 'node:test';
import assert from 'node:assert/strict';
import {
  RETREAT_PRESETS, retreatBody, retreatDefaultValue, retreatDefaultLabel,
  retreatDefaultSectionHTML, unitRetreatControlHTML,
} from './retreat.js';

test('realm and battle retreat offer only the two decided modes', () => {
  assert.deepEqual(RETREAT_PRESETS, [
    { value: 'when_losing', label: 'Retreat when it goes badly' },
    { value: 'hold', label: 'Hold to the last man' },
  ]);
  for (const html of [retreatDefaultSectionHTML({ when_losing: true }), unitRetreatControlHTML({ id: 'u1', in_battle: true })]) {
    assert.match(html, /Retreat when it goes badly/);
    assert.match(html, /Hold to the last man/);
    assert.doesNotMatch(html, /%|losses|Your whole realm/);
    assert.equal((html.match(/<option value="(?:hold|when_losing)"/g) || []).length, 2);
  }
});

test('retreatBody rejects every retired percentage and sends exactly one mode', () => {
  assert.deepEqual(retreatBody('hold'), { hold_to_last_man: true });
  assert.deepEqual(retreatBody('when_losing'), { when_losing: true });
  for (const value of ['', 'loyalty', '0.75', '0.5', '0.25', '0.4', 'unknown']) {
    assert.equal(retreatBody(value), null, 'retired retreat percentage became a request');
  }
});

test('GET mode reads back with the selected value and exact player wording', () => {
  assert.equal(retreatDefaultValue({ when_losing: true, retreat_at_loss: null }), 'when_losing');
  assert.equal(retreatDefaultValue({ hold_to_last_man: true }), 'hold');
  assert.equal(retreatDefaultLabel({ when_losing: true }), 'Retreat when it goes badly');
  assert.equal(retreatDefaultLabel({ hold_to_last_man: true }), 'Hold to the last man');
});

test('realm setting keeps its save action without explanatory prose', () => {
  const html = retreatDefaultSectionHTML({ hold_to_last_man: true });
  assert.match(html, /<option value="hold" selected>/);
  assert.match(html, /onclick="saveRetreatDefault\(\)"/);
  assert.doesNotMatch(html, /<p|style=/);
});

test('a stale percentage cannot reappear as a selectable third mode', () => {
  const html = retreatDefaultSectionHTML({ retreat_at_loss: 0.4 });
  assert.match(html, /Reload/);
  assert.doesNotMatch(html, /<select|60%|value="0.4"/);
});

test('failed read gives the server reason instead of claiming a default', () => {
  const html = retreatDefaultSectionHTML(null, 'you have not joined this world');
  assert.match(html, /you have not joined this world/);
  assert.doesNotMatch(html, /<select/);
});

test('battle override is available only to a fighting unit and is scoped to this battle', () => {
  assert.equal(unitRetreatControlHTML({ id: 'u1', in_battle: false }), '');
  const html = unitRetreatControlHTML({ id: 'u1', in_battle: true });
  assert.match(html, /This battle only/);
  assert.match(html, /unitRetreatOrder\('u1'\)/);
  assert.match(html, /<option value="when_losing">Retreat when it goes badly<\/option>/);
});
