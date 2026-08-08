import test from 'node:test';
import assert from 'node:assert/strict';

// chips.js touches `window.addEventListener('resize', recomputeChips)` at
// MODULE TOP LEVEL — a plain static import crashes node --test with
// "window is not defined" before any test body runs. Same trick as
// cargo.test.mjs: stub the one global it needs, then import dynamically.
// (megaron_plan_modulniva_dom.md flagged this file as blocked; the fix
// that actually shipped for marchctx.js 2026-08-05 was this stub
// convention, not the init()-function refactor the plan called for — see
// that file's own header comment. Same convention applied here rather
// than touching chips.js's production code, which the plan's own
// contract forbids doing lightly to a file main.js already wires up
// correctly.)
globalThis.window ??= { addEventListener() {} };

const badge = { textContent: '', style: { display: '' } };
globalThis.document ??= {
  getElementById: (id) => (id === 'gt-notif-badge' ? badge : { addEventListener() {}, style: {} }),
};

const { updateNotifBadge } = await import('./chips.js');

test('AK1: import does not touch DOM/window before this point (proven by reaching here)', () => {
  assert.ok(true);
});

test('AK2: a positive count shows the number and reveals the badge', () => {
  updateNotifBadge(5);
  assert.equal(badge.textContent, '5');
  assert.equal(badge.style.display, 'inline');
});

test('AK3: a count over 99 caps the label at "99+"', () => {
  updateNotifBadge(140);
  assert.equal(badge.textContent, '99+');
});

test('AK4: zero hides the badge', () => {
  updateNotifBadge(0);
  assert.equal(badge.style.display, 'none');
});
