import test from 'node:test';
import assert from 'node:assert/strict';

const noopEl = new Proxy({}, { get: (_t, k) => (k === 'style' ? {} : () => noopEl), set: () => true });
globalThis.document ??= { addEventListener() {}, getElementById: () => noopEl, createElement: () => noopEl, querySelector: () => noopEl, querySelectorAll: () => [], body: noopEl };
globalThis.window ??= { addEventListener() {}, matchMedia: () => ({ matches: false, addEventListener() {} }) };
globalThis.localStorage ??= { getItem: () => null, setItem() {}, removeItem() {} };
const { createPicker, barHTML } = await import('./hexpick.js');

function rig(names = {}) {
  const log = [];
  const env = {
    hide() { log.push('hide'); return { token: log.length }; },
    restore(t) { log.push('restore:' + (t && t.token)); },
    render(v) { log.push(v ? 'render' : 'clear'); rig.last = v; },
    describe(q, r) { return { name: names[q + ',' + r] || '' }; },
  };
  return { log, p: createPicker(env) };
}

test('start hides panels and remembers the token; bar has the prompt and no selection', () => {
  const { log, p } = rig();
  p.pick({ prompt: 'Redirect the Spearmen: tap the new destination' });
  assert.deepEqual(log, ['hide', 'render']);
  assert.equal(p.active(), true);
  assert.equal(p.view().prompt, 'Redirect the Spearmen: tap the new destination');
  assert.equal(p.view().sel, null);
  assert.equal(p.view().canConfirm, false);
});

test('first tap selects (button, not a choice); another tap moves the selection; confirm resolves and restores', async () => {
  const { log, p } = rig({ '12,21': 'Knossos' });
  const done = p.pick({ prompt: 'x' });
  p.tap(3, 4);
  assert.deepEqual(p.view().sel, { q: 3, r: 4, name: '' });
  assert.equal(p.view().canConfirm, true);
  assert.equal(p.active(), true, 'one tap must not choose');
  p.tap(12, 21);
  assert.deepEqual(p.view().sel, { q: 12, r: 21, name: 'Knossos' });
  p.confirm();
  assert.deepEqual(await done, { q: 12, r: 21 });
  assert.equal(p.active(), false);
  assert.deepEqual(log.filter(x => x.startsWith('restore')), ['restore:1'], 'restores with the token hide() gave, once');
  assert.equal(log.at(-1), 'clear');
});

test('cancel resolves null and restores', async () => {
  const { log, p } = rig();
  const done = p.pick({});
  p.tap(1, 1);
  p.cancel();
  assert.equal(await done, null);
  assert.equal(log.filter(x => x.startsWith('restore')).length, 1);
  p.cancel(); // no-op when idle
  assert.equal(log.filter(x => x.startsWith('restore')).length, 1);
});

test('a refused hex shows the reason and offers no Use button; confirm does nothing; a good tap clears it', async () => {
  const { p } = rig();
  const done = p.pick({ accept: h => (h.q === 9 ? 'Not a sea hex' : null) });
  p.tap(9, 0);
  assert.equal(p.view().reason, 'Not a sea hex');
  assert.equal(p.view().canConfirm, false);
  const html = barHTML(p.view());
  assert.match(html, /Not a sea hex/);
  assert.doesNotMatch(html, /data-pick-ok/);
  p.confirm();
  assert.equal(p.active(), true);
  p.tap(1, 0);
  assert.equal(p.view().reason, null);
  assert.match(barHTML(p.view()), /\(1,0\) — Use this hex/);
  p.confirm();
  assert.deepEqual(await done, { q: 1, r: 0 });
});

test('a second pick cancels the first (null) and starts fresh', async () => {
  const { p } = rig();
  const first = p.pick({ prompt: 'one' });
  p.tap(1, 1);
  const second = p.pick({ prompt: 'two' });
  assert.equal(await first, null);
  assert.equal(p.view().prompt, 'two');
  assert.equal(p.view().sel, null);
  p.tap(2, 2); p.confirm();
  assert.deepEqual(await second, { q: 2, r: 2 });
});

test('taps while idle do nothing', () => {
  const { log, p } = rig();
  p.tap(1, 1); p.confirm();
  assert.deepEqual(log, []);
});

test('bar escapes the prompt and shows coordinates with the name', () => {
  const html = barHTML({ prompt: '<b>x</b>', sel: { q: 12, r: 21, name: 'Phaistos' }, reason: null, canConfirm: true });
  assert.doesNotMatch(html, /<b>/);
  assert.match(html, /\(12,21\) Phaistos — Use this hex/);
  assert.match(html, /data-pick-cancel/);
});
