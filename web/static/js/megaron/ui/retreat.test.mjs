import test from 'node:test';
import assert from 'node:assert/strict';
import {
  RETREAT_PRESETS, retreatBody, retreatDefaultValue, retreatDefaultLabel,
  retreatDefaultSectionHTML, unitRetreatControlHTML,
} from './retreat.js';

// The server breaks a side when current/start <= retreat_at_loss — the value
// is the strength LEFT. "25% losses" must send 0.75 (the old per-unit control
// sent 0.25 and so meant 75% losses).
test('presets: the label names the losses, the value is the strength left', () => {
  for (const p of RETREAT_PRESETS) {
    if (p.value === 'hold') continue;
    const losses = Math.round((1 - parseFloat(p.value)) * 100);
    assert.equal(p.label, 'retreat at ' + losses + '% losses');
  }
  assert.deepEqual(retreatBody('0.75'), { retreat_at_loss: 0.75 });
});

test('retreatBody: each mode maps to exactly one API field', () => {
  assert.deepEqual(retreatBody('hold'), { hold_to_last_man: true });
  assert.deepEqual(retreatBody('loyalty'), { by_loyalty: true });
  assert.equal(retreatBody(''), null);
});

test('retreatDefaultValue/Label: the GET body reads back as the chosen option', () => {
  assert.equal(retreatDefaultValue({ by_loyalty: true, retreat_at_loss: null }), 'loyalty');
  assert.equal(retreatDefaultValue({ by_loyalty: false, hold_to_last_man: true }), 'hold');
  assert.equal(retreatDefaultLabel({ by_loyalty: false, retreat_at_loss: 0.5 }), 'retreat at 50% losses');
  // A threshold set from keryx that is no preset still reads correctly.
  assert.equal(retreatDefaultLabel({ by_loyalty: false, retreat_at_loss: 0.4 }), 'retreat at 60% losses');
});

test('section: current value selected, says it only reaches battles entered from now on', () => {
  const html = retreatDefaultSectionHTML({ by_loyalty: false, hold_to_last_man: true, retreat_at_loss: null });
  assert.match(html, /When to retreat/);
  assert.match(html, /<option value="hold" selected>/);
  assert.match(html, /from now on/);
  assert.match(html, /onclick="saveRetreatDefault\(\)"/);
  assert.doesNotMatch(html, /style=/);   // classes only, no inline colours
});

test('section: a custom keryx threshold gets its own selected option', () => {
  const html = retreatDefaultSectionHTML({ by_loyalty: false, retreat_at_loss: 0.4 });
  assert.match(html, /<option value="0.4" selected>retreat at 60% losses<\/option>/);
});

test('section: a failed read says so rather than showing the default as if it were set', () => {
  const html = retreatDefaultSectionHTML(null, 'you have not joined this world');
  assert.match(html, /you have not joined this world/);
  assert.doesNotMatch(html, /<select/);
});

test('per-unit control: only while the unit is fighting, and it says "this battle"', () => {
  assert.equal(unitRetreatControlHTML({ id: 'u1', in_battle: false }), '');
  const html = unitRetreatControlHTML({ id: 'u1', in_battle: true });
  assert.match(html, /this battle/);
  assert.match(html, /unitRetreatOrder\('u1'\)/);
  assert.match(html, /<option value="0.75">retreat at 25% losses<\/option>/);
});
