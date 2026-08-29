import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

// webb/notisarkiv-o-panelfokus, beteende 1 (Timothy 2026-08-22): "notislistan
// ska vara ett arkiv, allt ska finnas där." "Clear all" — en knapp vars enda
// effekt var att DELETE:a varje notis för denna Wanax — togs bort ur
// notis-drawern och ersattes av en etikett ("archive"). Servern behåller sin
// DELETE-endpoint, men ingen klientkod får längre anropa den.
//
// notif.js själv är sidoeffektfri vid modul-scope, MEN den importerar
// updateNotifBadge från ui/chips.js — och chips.js gör
// `window.addEventListener('resize', recomputeChips)` på MODULNIVÅ (se
// ../chips.test.mjs). En vanlig statisk import av notif.js drar därför med
// sig den kraschen. Samma stub-sedan-dynamisk-import-konvention som
// cargo.test.mjs/gossip.test.mjs.
globalThis.window ??= { addEventListener() {} };
const notifModule = await import('./notif.js');

test('AK1: notif.js exporterar inte längre clearAllNotifs — funktionen är borttagen, inte bara oanvänd', () => {
  assert.equal(notifModule.clearAllNotifs, undefined);
  assert.equal(typeof notifModule.loadNotifDrawer, 'function');
  assert.equal(typeof notifModule.notifShowKind, 'function');
});

// De två återstående kontrollerna läser den faktiska markupen/källan i stället
// för att bara lita på JS-exporten ovan — regressionen kunde annars smyga sig
// in igen via map.html (knappen) eller main.js (window-bryggan) utan att
// notif.js självt ändras.
const repoFile = (relFromHere) => fileURLToPath(new URL(relFromHere, import.meta.url));
const mapHtml = readFileSync(repoFile('../../../../map.html'), 'utf8');
const mainJs = readFileSync(repoFile('../../main.js'), 'utf8');

test('AK2: notis-drawerns header i map.html saknar "Clear all"-knappen och visar arkiv-etiketten i stället', () => {
  const drawerBlock = mapHtml.slice(mapHtml.indexOf('id="drawer-notif"'), mapHtml.indexOf('id="drawer-notif"') + 600);
  assert.doesNotMatch(drawerBlock, /notif-clear-all/);
  assert.doesNotMatch(drawerBlock, /clearAllNotifs/);
  assert.doesNotMatch(drawerBlock, />\s*Clear all\s*</);
  assert.match(drawerBlock, /notif-archive-note/);
  assert.match(drawerBlock, />archive</);
});

test('AK3: clearAllNotifs finns inte kvar i main.js window-bryggan (den skulle annars vara nåbar även utan knapp i markupen)', () => {
  assert.doesNotMatch(mainJs, /clearAllNotifs/);
});
