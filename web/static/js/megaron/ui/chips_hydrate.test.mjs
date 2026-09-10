// "Arkivet matar stapeln" — initNotifications bygger om Dispatches vid load.
//
// ⚠️ EGEN FIL, av samma skäl som grannen chips_dismiss.test.mjs dokumenterar:
// den här filen sätter `globalThis.document =` rakt av med en bredare
// element-stubb, och node:test kör varje FIL i egen process. Slås filerna ihop
// vinner den sista stubben.
//
// Vad som bevisas: en Wanax som varit borta möter sina obehandlade händelser
// som brickor vid inloggning i stället för ett tomt fält (megaron_arbetssatt
// §14 grind 2). Före den här slicen föddes ett chip BARA av en live-WS-push,
// alltså var stapeln alltid tom precis vid återkomsten — den enda stund den
// var till för.
import test from 'node:test';
import assert from 'node:assert/strict';

globalThis.window ??= { addEventListener() {}, openDrawer() {} };
globalThis.localStorage ??= { getItem: () => 'test-token' };

function makeChipEl() {
  const classes = new Set();
  const sub = { style: {}, addEventListener() {} };
  return {
    style: {}, dataset: {},
    classList: { add: c => classes.add(c), remove: c => classes.delete(c), contains: c => classes.has(c) },
    addEventListener() {},
    querySelector: () => sub,
    querySelectorAll: () => [],
    remove() {},
    // The chip's own innerHTML setter — captured so a test can read the text.
    set innerHTML(v) { this._html = v; },
    get innerHTML() { return this._html || ''; },
  };
}

const strip = {
  clientWidth: 800,
  children: [],
  appendChild(c) { this.children.push(c); },
  querySelectorAll: () => strip.children.filter(c => !c.classList.contains('dismissing')),
};
const badge = { style: {} };
globalThis.document = {
  getElementById(id) {
    if (id === 'gt-dispatch-strip') return strip;
    if (id === 'gt-notif-badge') return badge;
    if (id === 'dc-dismiss-all') return { style: {} };
    return null;
  },
  createElement: () => makeChipEl(),
};

const { State } = await import('../state.js');
State.WORLD_ID = 'w-1';
const { initNotifications } = await import('./chips.js');
const { notifDomain } = await import('./format.js');

// One archived notification, `mins` minutes old. created_at is what decides the
// order the server returns them in and the "ago" label on the chip.
function notif(kind, mins, id) {
  return { id, kind, level: 3, body: {}, created_at: new Date(Date.now() - mins * 60000).toISOString(),
           read_at: null };
}

let lastURL = null;
function serve(notifications) {
  globalThis.fetch = url => {
    lastURL = url;
    return Promise.resolve({
      ok: true, status: 200, headers: { get: () => null },
      json: () => Promise.resolve({ notifications, unread: notifications.length }),
    });
  };
}

test('HY1: stapeln byggs ur arkivet — n olästa ger n brickor', async () => {
  strip.children.length = 0;
  serve([notif('FoodShortfall', 5, 'a'), notif('SiegeStarted', 30, 'b')]);
  await initNotifications();
  assert.equal(strip.children.length, 2);
});

test('HY2: frågan ställs med dispatchable=true — en tystad sort ska inte återuppstå vid omladdning', () => {
  // Mutera bort ?dispatchable=true och den här raden faller: servern skulle då
  // svara med tystade sorter också, och muten skulle hålla live men brytas av
  // varje sidladdning — precis det tysta felläge som är värre än ingen mute.
  assert.match(lastURL, /\/notifications\?unread=true&dispatchable=true$/);
});

test('HY3: äldst till vänster — spelaren arbetar sig igenom i den ordning buden kom', async () => {
  strip.children.length = 0;
  // Servern svarar nyast först (ORDER BY created_at DESC).
  serve([notif('SiegeStarted', 1, 'ny'), notif('FoodShortfall', 60, 'gammal')]);
  await initNotifications();
  assert.deepEqual(strip.children.map(c => c.dataset.notifId), ['gammal', 'ny']);
});

test('HY4: vid backlog behålls de NYASTE tolv, inte de tolv äldsta', async () => {
  strip.children.length = 0;
  // 20 notiser, den nyaste först (som servern svarar). De äldsta ska falla
  // bort: en inkommande marsch från i natt har restid kvar att svara på,
  // gårdagens har det inte. Kvarvarande ligger ändå äldst-till-vänster.
  serve(Array.from({ length: 20 }, (_, i) => notif('UnitArrived', i, `n${i}`)));
  await initNotifications();
  assert.equal(strip.children.length, 12);
  assert.equal(strip.children[0].dataset.notifId, 'n11', 'vänstra brickan är den äldsta av de behållna');
  assert.equal(strip.children[11].dataset.notifId, 'n0', 'högra brickan är den allra nyaste');
});

test('HY5: varje bricka bär sitt notis-id, så avfärdande kan markera rätt arkivrad läst', async () => {
  strip.children.length = 0;
  serve([notif('DivinePunishment', 3, 'x-1')]);
  await initNotifications();
  assert.equal(strip.children[0].dataset.notifId, 'x-1');
});

test('HY6: en sort utan egen gren får ändå en bricka — i neutral systemfärg', () => {
  // Roten under hela slicen: ws.js namngav domän per hand i nitton grenar, och
  // 33 serversorter hade ingen gren alls och kastades tyst. Nu är felläget en
  // trist bricka, aldrig tystnad.
  assert.equal(notifDomain('FoodShortfall'), 'city');
  assert.equal(notifDomain('SiegeStarted'), 'war');
  assert.equal(notifDomain('CaravanRaided'), 'trade');
  assert.equal(notifDomain('MessengerArrival'), 'diplomacy');
  assert.equal(notifDomain('DivineBlessing'), 'kult');
  assert.equal(notifDomain('SortenSomInteFinnsÄn'), 'system');
});

test('HY7: ett tomt arkiv ger en tom stapel och ingen badge', async () => {
  strip.children.length = 0;
  serve([]);
  await initNotifications();
  assert.equal(strip.children.length, 0);
  assert.equal(badge.style.display, 'none');
});
