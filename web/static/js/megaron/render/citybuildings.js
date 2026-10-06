// Native city-view architecture: flat earthen roofs, plaster and open work yards.
// Every footprint includes its roof depth and equipment; level expands the house.
import { set, row, col, rect, cube } from './pixelgrid.js';

function socle(g, x, y, w) {
  row(g, x, y, w, 'T');
  row(g, x + 1, y + 1, w - 1, 't');
}
function house(g, x, base, w, h = 15, depth = 4) {
  cube(g, x, base - h, w, h, { depth, roof: 2, parapet: true });
  rect(g, x + 3, base - 7, 4, 7, 'W');
  col(g, x + 3, base - 6, 6, 'w');
  if (w > 12) { rect(g, x + w - 6, base - 10, 3, 3, 'V'); row(g, x + w - 7, base - 7, 5, 'P'); }
  socle(g, x, base, w + depth * 2);
}
function jar(g, x, base, colour = 'R') {
  row(g, x + 1, base - 7, 3, 'r');
  rect(g, x, base - 5, 5, 4, colour);
  col(g, x, base - 4, 2, 'M');
  row(g, x + 1, base - 1, 3, 'r');
}
function awning(g, x, base, w, colour = 'n') {
  col(g, x, base - 12, 12, 'W'); col(g, x + w - 1, base - 12, 12, 'W');
  for (let yy = 0; yy < 4; yy++) row(g, x + 3 - yy, base - 15 + yy, w, yy % 2 ? colour : 'P');
  row(g, x, base - 11, w, colour);
}
function table(g, x, base, w) {
  rect(g, x, base - 5, w, 2, 'w'); col(g, x + 1, base - 3, 3, 'W'); col(g, x + w - 2, base - 3, 3, 'W');
}
function rock(g, x, base, level, silver) {
  const w = 28 + level * 2;
  for (let y = 0; y < 19; y++) {
    const inset = Math.max(0, 7 - Math.floor(y / 2));
    row(g, x + inset, base - 20 + y, w - inset * 2, y % 5 === 0 ? 'T' : 'q');
  }
  rect(g, x + 8, base - 12, 10, 12, 'V');
  col(g, x + 7, base - 13, 13, 'W'); col(g, x + 18, base - 13, 13, 'W');
  row(g, x + 6, base - 14, 14, 'w');
  for (let i = 0; i < level + 2; i++) rect(g, x + 22 + (i % 2) * 4, base - 3 - Math.floor(i / 2) * 3, 3, 2, silver ? 'S' : 'O');
  return w;
}
const BUILDINGS = {
  farm(g, x, b, l) {
    const w = 15 + l * 2; house(g, x, b - 4, w, 13);
    for (let i = 0; i < 3; i++) {
      rect(g, x + 4 + i * 6, b - 5, 4, 4, 'B'); row(g, x + 4 + i * 6, b - 3, 4, 'W');
    }
    for (let i = 0; i < 3; i++) { row(g, x + w + 9, b - 7 + i * 3, 9, 'r'); row(g, x + w + 9, b - 8 + i * 3, 7, 'B'); }
    return w + 18;
  },
  market(g, x, b, l) {
    const w = 16 + l * 2;
    house(g, x + 4, b - 6, w, 12, 3);
    awning(g, x, b, w + 5, 'O'); table(g, x + 2, b, w);
    for (let i = 0; i < 3; i++) rect(g, x + 3 + i * 5, b - 7, 3, 2, i === 1 ? 'g' : 'B');
    jar(g, x + w + 7, b); jar(g, x + w + 12, b - 3);
    return w + 17;
  },
  foundry(g, x, b, l) {
    house(g, x, b - 3, 13 + l, 14, 4);
    awning(g, x, b, 13, 'N'); table(g, x + 2, b, 10);
    row(g, x + 4, b - 7, 5, 'S');
    const ox = x + 23;
    rect(g, ox + 3, b - 20, 4, 8, 'r'); row(g, ox + 2, b - 21, 6, 'T');
    rect(g, ox + 1, b - 13, 9, 12, 'R'); rect(g, ox + 3, b - 16, 5, 3, 'M');
    rect(g, ox + 3, b - 7, 5, 6, 'V'); rect(g, ox + 4, b - 5, 3, 3, 'F');
    row(g, ox, b, 12, 'T');
    return 35;
  },
  stable(g, x, b, l) {
    house(g, x, b - 4, 18 + l * 2, 15, 4);
    rect(g, x + 3, b - 14, 11, 10, 'V');
    // Horse in the yard: body, raised neck, muzzle, mane and four legs.
    rect(g, x + 21, b - 7, 9, 4, 'R'); rect(g, x + 28, b - 11, 3, 7, 'R');
    row(g, x + 28, b - 12, 5, 'R'); col(g, x + 27, b - 11, 5, 'W');
    col(g, x + 22, b - 3, 3, 'W'); col(g, x + 28, b - 3, 3, 'W'); col(g, x + 20, b - 6, 4, 'W');
    for (let i = 0; i < 4; i++) col(g, x + 16 + i * 6, b - 3, 5, 'W');
    row(g, x + 16, b - 1, 19, 'w'); return 36;
  },
  barracks(g, x, b, l) {
    house(g, x, b - 3, 20 + l * 2, 17, 4);
    row(g, x + 1, b - 15, 19 + l * 2, 'O');
    for (let i = 0; i < 3; i++) {
      col(g, x + 5 + i * 6, b - 10, 10, 'W'); set(g, x + 5 + i * 6, b - 11, 'S');
      rect(g, x + 3 + i * 6, b - 6, 4, 4, 'O'); col(g, x + 4 + i * 6, b - 5, 2, 'B');
    }
    return 28 + l * 2;
  },
  temple(g, x, b, l) {
    const w = 20 + l * 2; house(g, x + 2, b - 3, w, 20, 4);
    rect(g, x + 5, b - 16, w - 6, 13, 'd'); row(g, x + 4, b - 17, w - 4, 'W');
    for (const dx of [5, w - 4]) { rect(g, x + dx, b - 16, 3, 13, 'O'); row(g, x + dx - 1, b - 16, 5, 'o'); }
    rect(g, x + 12, b - 11, 4, 8, 'V');
    row(g, x, b - 2, w + 5, 'P'); row(g, x - 0, b, w + 7, 'T');
    col(g, x + 11, b - 27, 3, 'P'); col(g, x + 17, b - 27, 3, 'P'); row(g, x + 11, b - 24, 7, 'P');
    return w + 10;
  },
  olive_press(g, x, b, l) {
    house(g, x, b - 3, 12 + l * 2, 14, 4);
    rect(g, x + 19, b - 4, 13, 4, 'T'); col(g, x + 21, b - 12, 10, 'W');
    row(g, x + 16, b - 11, 17, 'w'); rect(g, x + 29, b - 10, 4, 5, 'q');
    rect(g, x + 23, b - 5, 5, 2, 'n'); jar(g, x + 9, b, 'N'); return 34;
  },
  winery(g, x, b, l) {
    house(g, x, b - 4, 16 + l * 2, 15, 4);
    for (let i = 0; i < 4; i++) jar(g, x + 3 + i * 7, b - (i % 2 ? 2 : 0));
    rect(g, x + 24, b - 13, 8, 4, 'w'); row(g, x + 25, b - 13, 6, 'o'); return 34;
  },
  lumbermill(g, x, b, l) {
    house(g, x, b - 3, 12 + l * 2, 13, 4);
    for (let i = 0; i < 3; i++) { row(g, x + 18 + i, b - 2 - i * 3, 15 - i * 2, 'W'); row(g, x + 18 + i, b - 3 - i * 3, 14 - i * 2, 'w'); }
    col(g, x + 23, b - 18, 8, 'W'); col(g, x + 31, b - 18, 8, 'W'); row(g, x + 23, b - 18, 9, 'w');
    for (let i = 0; i < 6; i++) set(g, x + 24 + i, b - 12 - i, 'S'); return 34;
  },
  stonequarry(g, x, b, l) {
    for (let i = 0; i < 3; i++) {
      rect(g, x + i * 9, b - 10 - i * 4, 12, 10 + i * 4, 'q');
      row(g, x + i * 9, b - 10 - i * 4, 12, 'T');
      for (let j = 3; j < 10 + i * 4; j += 5) row(g, x + i * 9, b - 10 - i * 4 + j, 11, 't');
    }
    for (let i = 0; i < l + 1; i++) { rect(g, x + i * 7, b - 4, 6, 4, 'T'); row(g, x + i * 7, b - 4, 6, 'P'); }
    return 34;
  },
  mine(g, x, b, l) { return rock(g, x, b, l, false); },
  silver_mine(g, x, b, l) { return rock(g, x, b, l, true); },
  harbour(g, x, b, l) {
    house(g, x, b - 8, 13 + l, 13, 3);
    rect(g, x + 8, b - 5, 28, 7, 'z'); row(g, x + 11, b - 3, 23, 'Z');
    rect(g, x + 2, b - 5, 7, 7, 'w'); row(g, x + 4, b - 3, 12, 'W');
    row(g, x + 17, b - 1, 15, 'W'); row(g, x + 19, b, 11, 'R');
    col(g, x + 25, b - 18, 17, 'W');
    for (let i = 0; i < 9; i++) row(g, x + 26, b - 17 + i, Math.max(2, i), 'P');
    jar(g, x + 2, b - 5); return 37;
  },
};

export function buildingWidth(type, level) {
  const probe = { w: 0, h: 0, px: [] };                // rutnät som sväljer allt
  const fn = BUILDINGS[type];
  return fn ? fn(probe, 0, 0, Math.max(1, Math.min(3, level || 1))) : 0;
}

/** Ritar en färdig byggnad i scenens rutnät. Returnerar bredden. */
export function stampBuilding(g, type, x, base, level) {
  const fn = BUILDINGS[type];
  if (!fn) return 0;
  return fn(g, x, base, Math.max(1, Math.min(3, level || 1)));
}

/** Ritar en byggnad UNDER UPPFÖRANDE: huset självt reser sig ur sin grund i
 *  takt med `phase` (0–1), med ställning kvar tills det är klart.
 *
 *  Det gamla bygget var en genomskinlig ruta med två kryss över — samma bild
 *  oavsett vad som byggdes och hur långt det kommit. Här ritas den RIKTIGA
 *  formen, klippt vid bygghöjden: en Wanax ser vad som växer, inte bara att
 *  något gör det. */
export function stampUnderConstruction(g, type, x, base, level, phase) {
  const probe = { w: 400, h: 60, px: new Array(400 * 60).fill('.') };
  const w = stampBuilding(probe, type, 8, 40, level);
  if (!w) return 0;
  // Byggets överkant: hur högt huset har kommit.
  let top = 40;
  for (let y = 0; y < 60; y++)
    for (let xx = 0; xx < 400; xx++)
      if (probe.px[y * 400 + xx] !== '.') { top = Math.min(top, y); xx = 400; }
  const cutoff = 40 - Math.round((40 - top) * Math.max(0, Math.min(1, phase)));
  for (let y = cutoff; y < 60; y++)
    for (let xx = 0; xx < 400; xx++) {
      const ch = probe.px[y * 400 + xx];
      if (ch !== '.') set(g, x + xx - 8, base + y - 40, ch);
    }
  // Grundmuren syns alltid, även vid phase 0 — annars finns det inget att se
  // förrän bygget är halvvägs, och en tom tomt läser som en bugg.
  socle(g, x, base, w);
  // Ställningen. Utkastet la en bom tvärs ÖVER hela toppen, och bygget blev en
  // låda med ett hus i botten — man såg emballaget, inte huset. Nu: två stolpar
  // som visar den FÄRDIGA höjden, korta ledare som bara sticker ut ett par
  // pixlar, och en bom i arbetshöjd. Byggets översta rad läser då som "hit har
  // det kommit", och stolparnas topp som "dit ska det".
  // Stolparna står PÅ tomten, inte utanför den. Utkastet la dem två pixlar
  // utanför med tre pixlar breda ledare, och två byggen bredvid varandra
  // växte ihop till en enda ställningsklump i konturpasset.
  const sh = 40 - top + 2;
  col(g, x, base - sh, sh + 1, 'W');
  col(g, x + w - 1, base - sh, sh + 1, 'W');
  for (let i = 2; i < sh; i += 4) {
    set(g, x + 1, base - i, 'w');
    set(g, x + w - 2, base - i, 'w');
  }
  // Arbetsbommen ligger i byggets överkant och FÖLJER det uppåt — den är det
  // enda som rör sig mellan två besök i drawern.
  const workY = base - (40 - cutoff) - 1;
  row(g, x, workY, w, 'w');
  row(g, x, workY + 1, w, 'W');
  return w;
}

export const KNOWN_BUILDINGS = Object.keys(BUILDINGS);

