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

// Timothy 2026-09-25: "it doesn't seem possible to give orders to units that
// have already been given orders — that is wrong, they must be reachable by
// orders." The right-click march menu used to filter marching units out
// entirely; marchCtxOrderMode is the pure decision (eligible? march or
// redirect?) that now drives both that filter and the group's send target.
const { marchCtxOrderMode } = await import('./marchctx.js');

test('AK9: a garrisoned/positioned unit is eligible to march', () => {
  assert.equal(marchCtxOrderMode({ status: 'garrison', deployable: true }), 'march');
  assert.equal(marchCtxOrderMode({ status: 'positioned', deployable: true }), 'march');
});

test('AK10: a marching unit is eligible too — but for redirect, not a fresh march', () => {
  assert.equal(marchCtxOrderMode({ status: 'marching', deployable: true }), 'redirect');
});

test('AK11: a fortified unit stays ineligible (server blocks fresh march on it)', () => {
  assert.equal(marchCtxOrderMode({ status: 'garrison', deployable: true, stance: 'fortify' }), null);
});

test('AK12: a still-forming/training unit stays ineligible even while nominally marching-shaped', () => {
  assert.equal(marchCtxOrderMode({ status: 'forming', deployable: false }), null);
  assert.equal(marchCtxOrderMode({ status: 'marching', deployable: false }), null);
});

test('AK13: a unit embarked/disbanded/other status is ineligible', () => {
  assert.equal(marchCtxOrderMode({ status: 'embarked', deployable: true }), null);
});

test('AK14: marching and garrisoned units of the same type+hex stay in SEPARATE groups — a redirect send must never merge with a fresh-march send', () => {
  const units = [
    { id: 'u1', type: 'spearman', status: 'garrison', deployable: true, q: 5, r: 5, display_name: 'First Spearmen' },
    { id: 'u2', type: 'spearman', status: 'marching', deployable: true, q: 5, r: 5, display_name: 'Second Spearmen' },
  ];
  const groups = groupMarchUnits(units, []);
  assert.equal(groups.length, 2, 'march and redirect groups must not merge even at the same (q,r)');
  const byMode = Object.fromEntries(groups.map(g => [g.mode, g]));
  assert.deepEqual(byMode.march.ids, ['u1']);
  assert.deepEqual(byMode.redirect.ids, ['u2']);
});

test('AK15: a redirect group is marked in its label — the player must see this send goes to a Runner', () => {
  const u = { id: 'u1', type: 'spearman', status: 'marching', deployable: true, q: 5, r: 5, display_name: 'First Spearmen' };
  const groups = groupMarchUnits([u], []);
  assert.match(marchGroupLabelHTML(groups[0]), /redirect by Runner/);
});
