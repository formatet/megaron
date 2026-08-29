import test from 'node:test';
import assert from 'node:assert/strict';

// webb/notisarkiv-o-panelfokus, beteende 3 (Timothy 2026-08-22: "testa att
// stänga den"): .drawer (z-index 200, höger-ankrad) täckte visuellt
// .inspect-panel (z-index 15) UTAN att stänga den — panelen låg kvar i DOM:en
// med State.selectedHex levande och kom tillbaka så fort drawern stängdes
// igen. Fixen: openDrawer() (main.js) anropar nu closeInspect() (render/map.js)
// innan den öppnar drawern.
//
// main.js har inget test i repot sedan tidigare — det körs som ett
// self-executing async IIFE (start()) vid modul-import, som i sin tur
// bootstrappar mot /api/v1/auth/me, location.pathname, localStorage m.m.
// Samma noopEl/stub-then-dynamic-import-konvention som map.test.mjs och
// search_import.test.mjs använder räcker inte ensam här, eftersom den
// konventionens delade Proxy ger ett NYTT {} för varje `.style`-åtkomst —
// closeInspect()s effekt (document.getElementById('inspect-panel').style.display
// = 'none') skulle då aldrig gå att observera i efterhand. I stället: en
// getElementById som cachar EN persistent fejk-nod per id (så samma
// 'inspect-panel'-objekt kommer tillbaka både när testet läser det FÖRE och
// när closeInspect() skriver till det), plus en fetch-stub som får
// bootstrap()s auth-koll att misslyckas rent (401 → catch → return false)
// så att start() aldrig når initMap()/initWS() och aldrig behöver en riktig
// canvas/WebSocket-miljö.
function makeEl() {
  const classes = new Set();
  return {
    style: {},
    dataset: {},
    value: '',
    textContent: '',
    innerHTML: '',
    classList: {
      add: (c) => classes.add(c),
      remove: (c) => classes.delete(c),
      contains: (c) => classes.has(c),
    },
    addEventListener() {},
    removeEventListener() {},
    appendChild() {},
    insertBefore() {},
    contains: () => false,
    querySelector: () => makeEl(),
    querySelectorAll: () => [],
    getContext: () => ({}),
    remove() {},
  };
}

const elCache = new Map();
function getElementById(id) {
  if (!elCache.has(id)) elCache.set(id, makeEl());
  return elCache.get(id);
}

globalThis.document = {
  getElementById,
  addEventListener() {},
  querySelector: () => makeEl(),
  querySelectorAll: () => [],
  createElement: () => makeEl(),
  body: makeEl(),
  activeElement: null,
};
globalThis.window = { addEventListener() {} };
globalThis.localStorage = { getItem: () => null, setItem() {}, removeItem() {} };
globalThis.sessionStorage = { getItem: () => null, setItem() {}, removeItem() {} };
globalThis.location = { pathname: '/world/w1/map', href: '' };
// bootstrap() calls the raw global fetch (not fetchAuth) for /api/v1/auth/me —
// making it fail with ok:false sends bootstrap down its existing catch path
// (location.href = '/'; return false;), which stops start() before it ever
// calls initMap()/initWS()/initCelestial(), none of which this test needs.
globalThis.fetch = async () => ({ ok: false, status: 401, headers: { get: () => null }, json: async () => ({}) });

const { openDrawer, closeDrawer } = await import('./main.js');
const { State } = await import('./state.js');

test('AK1: main.js importeras (bakgrunds-bootstrap misslyckas tyst mot en stubbad 401) utan att krascha', () => {
  assert.equal(typeof openDrawer, 'function');
});

test('AK2: att öppna EN GODTYCKLIG drawer stänger inspect-panelen och nollställer State.selectedHex', () => {
  const inspectPanel = getElementById('inspect-panel');
  inspectPanel.style.display = 'flex'; // simulerar: en hex var vald, panelen syns
  State.selectedHex = { q: 3, r: 4 };

  // Ett påhittat drawernamn — poängen som testas är att closeInspect() körs
  // INNAN någon drawer öppnas, oavsett VILKEN. Ett riktigt namn (t.ex. 'notif')
  // skulle dra in fetchAuth/nätverk i testet, vilket är notif.test.mjs sak.
  openDrawer('__test_drawer__');

  assert.equal(inspectPanel.style.display, 'none',
    'drawern täckte panelen förut utan att stänga den — den ska nu vara riktigt stängd');
  assert.equal(State.selectedHex, null);
});

test('AK3: drawer-chrome öppnas som vanligt — closeInspect stör inte den normala vägen', () => {
  const drawerEl = getElementById('drawer-__test_drawer2__');
  openDrawer('__test_drawer2__');
  assert.ok(drawerEl.classList.contains('open'));
  assert.equal(State.activeDrawer, '__test_drawer2__');
});

test('AK4: stänger man drawern igen kommer panelen INTE tillbaka (den var riktigt stängd, inte bara dold bakom z-index)', () => {
  // Reproducerar exakt Timothys ursprungliga fynd: FÖRE fixen stängdes panelen
  // aldrig — den låg kvar i DOM:en med display:flex, bara visuellt övertäckt
  // av drawern (z-index 200 > 15). closeDrawer() rör aldrig #inspect-panel,
  // så när drawern stängdes och slutade täcka, kom den gamla panelen tillbaka.
  const inspectPanel = getElementById('inspect-panel');
  inspectPanel.style.display = 'flex';
  State.selectedHex = { q: 9, r: 1 };

  openDrawer('__test_drawer4__');   // ska stänga panelen på riktigt (closeInspect)
  closeDrawer('__test_drawer4__');  // rör inte #inspect-panel alls

  assert.equal(inspectPanel.style.display, 'none',
    'panelen kom tillbaka efter closeDrawer — den stängdes aldrig på riktigt av openDrawer');
  assert.equal(State.selectedHex, null);
});
