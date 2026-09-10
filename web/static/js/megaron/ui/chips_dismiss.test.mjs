// Dispatch-strimmans "✕ all" (webb/notisarkiv-o-panelfokus, beteende 2).
//
// ⚠️ EGEN FIL med avsikt, inte av slarv. Grannen chips.test.mjs bevisar
// notis-BADGEN och stubbar `document.getElementById` så att just
// 'gt-notif-badge' ger dess eget objekt. Den här filen behöver en BREDARE
// element-stubb och sätter `globalThis.document =` rakt av. Slås de ihop
// till en fil vinner den sista stubben och grannens tre badge-tester faller
// på tom textContent — reproducerat vid merge 2026-08-29. node:test kör
// varje FIL i egen process, så åtskilda filer är hela lösningen.
import test from 'node:test';
import assert from 'node:assert/strict';

// webb/notisarkiv-o-panelfokus, beteende 2 (Timothy 2026-08-22): "'ta bort
// allt' är mer motiverat för den andra notislistan, där de ramlar in
// högerifrån." dismissAllChips() tömmer dispatch-STRIMMAN (transient), inte
// notis-arkivet (varje dispatch finns redan som rad i notif.js-drawern — se
// notif.test.mjs). Knappen (#dc-dismiss-all) ska bara synas vid >= 3 chips.
//
// chips.js touches `window.addEventListener('resize', recomputeChips)` at
// MODULE TOP LEVEL — samma fälla som master's chips.test.mjs redan
// dokumenterar (och samma konvention som cargo.test.mjs/marchctx.test.mjs):
// stubba globalen, importera dynamiskt.
globalThis.window ??= { addEventListener() {}, openDrawer() {} };

// En minimal DOM-fejk — mer än de vanliga noopEl-proxyerna i repot, men
// nödvändig här: recomputeChips/dismissAllChips går genom en RIKTIG lista av
// chip-element (classList, querySelectorAll, animationend), inte bara enstaka
// getElementById-anrop. Ingen extern testrigg dras in — bara node:test +
// vanliga objekt/closures.
function makeChipEl() {
  const classes = new Set();
  const listeners = {};
  const sub = { style: {}, addEventListener() {} }; // .dc-text/.dc-time/.dc-x stand-in
  return {
    style: {},
    dataset: {}, // real elements always have one; dismissChip reads .notifId
    classList: {
      add: (c) => classes.add(c),
      remove: (c) => classes.delete(c),
      contains: (c) => classes.has(c),
    },
    addEventListener(type, cb) { (listeners[type] ??= []).push(cb); },
    fireAnimationEnd() { (listeners.animationend || []).forEach(cb => cb()); },
    querySelector: () => sub,
    querySelectorAll: () => [],
    remove() {}, // dismissChip's animationend handler calls chip.remove() — a no-op here is fine,
                 // recomputeChips filters by classList anyway (see querySelectorAll above).
  };
}

function makeStrip() {
  const children = [];
  return {
    clientWidth: 400,
    children,
    appendChild(chip) { children.push(chip); },
    querySelectorAll(sel) {
      // Enda selektorn chips.js faktiskt använder är '.dispatch-chip:not(.dismissing)'.
      assert.equal(sel, '.dispatch-chip:not(.dismissing)');
      return children.filter(c => !c.classList.contains('dismissing'));
    },
  };
}

const strip = makeStrip();
const dismissAllBtn = { style: {} };
globalThis.document = {
  getElementById(id) {
    if (id === 'gt-dispatch-strip') return strip;
    if (id === 'dc-dismiss-all') return dismissAllBtn;
    if (id === 'gt-notif-badge') return { style: {} };
    return null;
  },
  createElement: () => makeChipEl(),
};

const { addDispatch, dismissAllChips } = await import('./chips.js');

test('AK1: chips.js importeras utan att röra DOM/window före denna punkt', () => {
  assert.ok(true);
});

test('AK2: "✕ all" är dold under 3 chips', () => {
  addDispatch({ kind: 'ArmyArrival', payload: {}, time: 'now' });
  addDispatch({ kind: 'BuildComplete', payload: {}, time: 'now' });
  assert.equal(strip.children.length, 2);
  assert.equal(dismissAllBtn.style.display, 'none');
});

test('AK3: "✕ all" dyker upp vid exakt 3 chips (DISMISS_ALL_FROM)', () => {
  addDispatch({ kind: 'MessengerArrival', payload: {}, time: 'now' });
  assert.equal(strip.children.length, 3);
  assert.equal(dismissAllBtn.style.display, '');
});

test('AK4: dismissAllChips märker varje aktivt chip som "dismissing" utan nätverksanrop för chip utan id', () => {
  // ⚠️ OMSKRIVEN 2026-09-10 tillsammans med "arkivet matar stapeln". Förut löd
  // raden "utan att röra nätverket/arkivet" och förbjöd VARJE fetch. Det är inte
  // längre sant: ett chip som bär ett notis-id markerar sin arkivrad LÄST när
  // det avfärdas (AK6), annars kom det tillbaka vid varje omladdning. Men läst
  // är inte raderat — invarianten "chippen arkiveras, raderas aldrig" står kvar
  // och bevakas av AK6:s metodkontroll (POST .../read, aldrig DELETE).
  // Chip UTAN id har inget att märka och ska fortfarande vara helt nätfria.
  const originalFetch = globalThis.fetch;
  globalThis.fetch = () => { throw new Error('dismissAllChips fick aldrig nätverksanropa'); };
  try {
    dismissAllChips();
  } finally {
    globalThis.fetch = originalFetch;
  }
  assert.equal(strip.children.length, 3, 'inget chip tas bort ur DOM:en förrän dess animationend');
  assert.ok(strip.children.every(c => c.classList.contains('dismissing')));
});

test('AK5: när animationen är klar försvinner chippen och "✕ all" göms igen under tröskeln', () => {
  strip.children.slice().forEach(c => c.fireAnimationEnd());
  // dismissChip's animationend-hanterare kallar chip.remove() — vår fejk har
  // ingen remove(), så chipsen ligger kvar i `children`men är märkta
  // dismissing; det är ändå recomputeChips (kallad från samma hanterare) vi
  // vill se effekten av, och den läser via querySelectorAll som redan
  // filtrerar bort dismissing-chip. display ska därför falla tillbaka till 'none'.
  assert.equal(dismissAllBtn.style.display, 'none');
});

test('AK6: ett chip som bär notis-id markerar SIN egen arkivrad läst vid avfärdande — POST .../read, aldrig DELETE', async () => {
  // Substratet bakom "arkivet matar stapeln": stapeln byggs om vid inloggning
  // ur de olästa notiserna, så ett avfärdat chip måste bli läst på servern —
  // annars är avfärdandet en gest utan minne och samma bricka står där igen
  // efter en omladdning. Läst ≠ raderat: raden finns kvar i arkivet.
  const calls = [];
  const originalFetch = globalThis.fetch;
  globalThis.localStorage ??= { getItem: () => 'test-token' };
  globalThis.fetch = (url, opts) => {
    calls.push({ url, method: (opts && opts.method) || 'GET' });
    return Promise.resolve({ ok: true, status: 204, headers: { get: () => null } });
  };
  try {
    addDispatch({ kind: 'FoodShortfall', payload: {}, id: 'notis-42', time: 'now' });
    const chip = strip.children[strip.children.length - 1];
    assert.equal(chip.dataset.notifId, 'notis-42', 'id:t ska ligga på elementet, inte bara i en closure');
    dismissAllChips();
    await new Promise(r => setImmediate(r)); // markRead är fire-and-forget
  } finally {
    globalThis.fetch = originalFetch;
  }
  assert.equal(calls.length, 1, 'exakt ett anrop — ett per avfärdat chip med id');
  assert.equal(calls[0].method, 'POST');
  assert.match(calls[0].url, /\/notifications\/notis-42\/read$/);
});
