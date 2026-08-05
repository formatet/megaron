import test from 'node:test';
import assert from 'node:assert/strict';

// The march menu grouped units by type + origin and labelled each row with the
// TYPE, so a player ordering one of several identical spearmen had nothing to
// go on but the hex coordinates (Timothy 2026-08-04: "därför räknar Timothy
// hexar för att veta vem han beordrar"). groupMarchUnits/marchGroupLabelHTML/
// marchGroupNamesHTML are the pure half of the fix.
//
// marchctx.js touches document.getElementById at module top level and pulls in
// render/map.js, so — same trick as cargo.test.mjs — stub the globals its
// import graph reaches, THEN import dynamically. A static import would evaluate
// that graph before any test body runs.
const noopEl = new Proxy({}, {
  get: (_t, k) => (k === 'style' ? {} : (k === 'value' ? '' : () => noopEl)),
  set: () => true,
});
globalThis.document ??= {
  addEventListener() {},
  getElementById: () => noopEl,
  createElement: () => noopEl,
  querySelector: () => noopEl,
  querySelectorAll: () => [],
  body: noopEl,
};
globalThis.window ??= { addEventListener() {}, matchMedia: () => ({ matches: false, addEventListener() {} }) };
globalThis.localStorage ??= { getItem: () => null, setItem() {}, removeItem() {} };

const { groupMarchUnits, marchGroupLabelHTML, marchGroupNamesHTML } =
  await import('./marchctx.js');

const spearman = (id, ordinal, q, r) => ({
  id,
  type: 'spearman',
  display_name: ordinal + ' Spearmen of Knossos',
  settlement_id: null,
  q, r,
});

test('AK1: a group of one wears its own name, not the bare type', () => {
  const groups = groupMarchUnits([spearman('u1', 'First', 12, 7)], []);
  assert.equal(groups.length, 1);
  const html = marchGroupLabelHTML(groups[0]);
  assert.match(html, /First Spearmen of Knossos/,
    'a lone unit must be named — this is the row that used to read just "Spearmen"');
});

test('AK2: the field unit keeps its (q,r) tag — the name says who supports it, the tag says where it stands', () => {
  const groups = groupMarchUnits([spearman('u1', 'First', 12, 7)], []);
  const html = marchGroupLabelHTML(groups[0]);
  assert.match(html, /\(12,7\)/);
});

test('AK3: a garrisoned unit does not say its town twice', () => {
  const u = { id: 'u1', type: 'spearman', display_name: 'First Spearmen of Knossos', settlement_id: 's1' };
  const groups = groupMarchUnits([u], [{ settlement_id: 's1', name: 'Knossos' }]);
  const html = marchGroupLabelHTML(groups[0]);
  assert.match(html, /First Spearmen of Knossos/);
  assert.equal(html.match(/Knossos/g).length, 1, 'the "· Knossos" tag repeats the name and must be dropped');
});

test('AK4: a group of several keeps the type label and lists its members in send order', () => {
  const units = [
    spearman('u1', 'First', 12, 7),
    spearman('u2', 'Second', 12, 7),
    spearman('u3', 'Third', 12, 7),
  ];
  const groups = groupMarchUnits(units, []);
  assert.equal(groups.length, 1, 'identical units at one place stay ONE group — the counter is the affordance');
  assert.equal(groups[0].ids.length, 3);

  const names = marchGroupNamesHTML(groups[0]);
  assert.match(names, /1\. First Spearmen of Knossos/);
  assert.match(names, /2\. Second Spearmen of Knossos/);
  assert.match(names, /3\. Third Spearmen of Knossos/);

  // names[] must track ids[] index for index, because sendMarch sends ids[0..n-1].
  assert.deepEqual(groups[0].ids, ['u1', 'u2', 'u3']);
  assert.deepEqual(groups[0].names, [
    'First Spearmen of Knossos', 'Second Spearmen of Knossos', 'Third Spearmen of Knossos',
  ]);
});

test('AK5: a lone unit gets no member list — the label already names it', () => {
  const groups = groupMarchUnits([spearman('u1', 'First', 12, 7)], []);
  assert.equal(marchGroupNamesHTML(groups[0]), '');
});

test('AK6: units at different places stay different groups', () => {
  const groups = groupMarchUnits(
    [spearman('u1', 'First', 12, 7), spearman('u2', 'Second', 3, 3)],
    [],
  );
  assert.equal(groups.length, 2);
});

test('AK7: a missing display_name falls back to the type label, never a blank row', () => {
  const u = { id: 'u1', type: 'spearman', q: 1, r: 1 };
  const groups = groupMarchUnits([u], []);
  assert.ok(groups[0].names[0], 'name must not be empty');
  assert.match(marchGroupLabelHTML(groups[0]), /\w/);
});

test('AK8: a hostile unit name is escaped, not injected', () => {
  const u = { id: 'u1', type: 'spearman', display_name: '<img src=x onerror=alert(1)>', q: 1, r: 1 };
  const groups = groupMarchUnits([u], []);
  assert.doesNotMatch(marchGroupLabelHTML(groups[0]), /<img/);
});
