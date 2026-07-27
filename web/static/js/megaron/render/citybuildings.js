// ── Stadsvyns byggnader ──────────────────────────────────────────────────
//
// Kartan visar en stads STORLEK. Den här filen visar dess INNEHÅLL: varje
// byggnad staden faktiskt har, en form per typ, i samma akhaiska formspråk och
// ur samma palett som kartsiluetterna (`pixelgrid.js`). Kartan och stadsvyn ska
// vara samma värld sedd på två avstånd, inte två grafikstilar.
//
// Den gamla scenen hade FYRA fasta sidoslottar och en generisk klassisk tempel-
// fasad med kolonner i mitten. Med fjorton byggnadstyper i katalogen betydde
// det att en stad med sju byggnader visade fyra av dem, sorterade efter en
// prioritetslista — och att bygga något nytt ofta inte syntes alls.
//
// **Nivå ritas som mer byggnad, inte som en siffra.** En byggnad på nivå 2 får
// en tillbyggnad, nivå 3 en till. Det är samma sanning som `buildings.level`
// bär, uttryckt i det enda språk en bild har.
//
// **Bygge ritas som växande.** En byggnad i kön reser sig ur sin grund i takt
// med `complete_at`: grundmur → halva fasaden → tak. Ställningen står kvar tills
// den är klar. Det är hela svaret på "byggnader ska växa fram" — inget
// förloppsfält, utan huset självt.
//
// Varje form är stämplad in i SCENENS rutnät, inte i sitt eget. Konturpasset
// löper sist över hela scenen, så två byggnader som står tätt får en gemensam
// silhuett medan de som står isär får var sin — precis som i verkligheten, och
// utan att någon regel behöver upprätthållas för hand.

import { set, row, col, rect, cube } from './pixelgrid.js';

// Grunden alla verkstäder står på: en stensockel som gör att huset står PÅ
// marken i stället för att sväva i den. Två pixlar räcker vid den här skalan.
function socle(g, x, y, w) {
  row(g, x, y, w, 'T');
  row(g, x + 1, y + 1, w - 1, 't');
}

// ── En form per byggnadstyp ──────────────────────────────────────────────
// Signaturen är (g, x, base, level) där `base` är markraden: byggnaden ritas
// UPPÅT därifrån, precis som aktörerna står på sin fot. Returnerar bredden så
// scenen kan lägga nästa byggnad intill utan att känna till formerna.

const BUILDINGS = {
  // Gården — tröskplats, kärvar och odlingsrader. Den enda "byggnaden" vars
  // huvudsak ligger UTANFÖR huset, så marken framför bär formen.
  farm(g, x, base, lvl) {
    const w = 11 + (lvl - 1) * 3;
    cube(g, x, base - 9, 9, 8, { door: true });
    socle(g, x, base - 1, 9);
    // Tröskplatsen: ljus rund yta med kärvar. Halmens gula är stadsvyns enda
    // mättade gula — den hör till skörden och ingen annanstans.
    for (let i = 0; i < (lvl >= 2 ? 3 : 2); i++) {
      const sx = x + 9 + i * 3;
      col(g, sx, base - 4, 4, 'B');
      set(g, sx + 1, base - 4, 'B');
      set(g, sx + 1, base - 3, 'n');
      col(g, sx + 1, base - 2, 2, 'N');
    }
    if (lvl >= 3) { cube(g, x - 4, base - 6, 5, 5); socle(g, x - 4, base - 1, 5); }
    return w;
  },

  // Kasernen — lång låg länga, mörk portöppning, spjutställ på gaveln och en
  // sköld målad i ockra. Vapnen syns utifrån; det är hela poängen med en kasern.
  barracks(g, x, base, lvl) {
    const w = 13 + (lvl - 1) * 2;
    cube(g, x, base - 10, w, 9, { door: true });
    socle(g, x, base - 1, w);
    for (let i = 0; i < 3 + lvl; i++) col(g, x + w + 1 + i * 2, base - 7, 6, 'W');
    row(g, x + w + 1, base - 7, (3 + lvl) * 2, 'w');
    // Sköld på fasaden, ockra med mörk buckla.
    rect(g, x + 2, base - 8, 3, 3, 'O');
    set(g, x + 3, base - 7, 'o');
    return w + (3 + lvl) * 2 + 2;
  },

  // Gruvan — bergssidan är byggnaden. Mörk stollmynning under en timmerbock,
  // en malmhög bredvid. Ingen kalkputs alls: gruvan hör till berget.
  mine(g, x, base, lvl) { return _mine(g, x, base, lvl, 'W', 'w'); },
  silver_mine(g, x, base, lvl) { return _mine(g, x, base, lvl, 'S', 'T'); },

  // Sågverket — timmerstapel, sågbock och en plankhög. Trä överallt, ingen puts.
  lumbermill(g, x, base, lvl) {
    const w = 12 + (lvl - 1) * 2;
    cube(g, x, base - 8, 8, 7, { door: true });
    socle(g, x, base - 1, 8);
    for (let r2 = 0; r2 < 2 + lvl; r2++) {          // timmerstapel, staplad
      const sy = base - 2 - r2 * 2, sw = (2 + lvl - r2) * 3;
      row(g, x + 9, sy, sw, 'w');
      row(g, x + 9, sy + 1, sw, 'W');
    }
    // Sågbocken: två snedställda ben och ett blad.
    set(g, x + 6, base - 10, 'W'); set(g, x + 7, base - 11, 'W');
    return w + 6;
  },

  // Stenbrottet — trappstegsvis uthuggna block, det översta ännu på plats.
  // Formen ÄR ingreppet i berget, inte en byggnad framför det.
  stonequarry(g, x, base, lvl) {
    const steps = 2 + lvl;
    for (let i = 0; i < steps; i++) {
      const sw = 4 + (steps - i) * 2;
      rect(g, x + i * 2, base - 2 - i * 2, sw, 2, i % 2 ? 'q' : 'T');
      row(g, x + i * 2, base - 1 - i * 2, sw, 't');
    }
    rect(g, x + steps * 2 + 1, base - 4, 4, 3, 'T');   // löst block på släde
    row(g, x + steps * 2, base - 1, 6, 'W');
    return steps * 2 + 8;
  },

  // Marknaden — vävt soltak på stolpar, amforor under. Textil, inte sten:
  // marknaden är det enda i staden som kan tas ned och flyttas.
  market(g, x, base, lvl) {
    const w = 12 + (lvl - 1) * 3;
    row(g, x, base - 10, w, 'n');                      // soltak, solbelyst
    row(g, x, base - 9, w, 'N');
    for (const px of [x + 1, x + w - 2]) col(g, px, base - 8, 8, 'W');
    for (let i = 0; i < 2 + lvl; i++) {                // amforor
      const ax = x + 3 + i * 3;
      set(g, ax + 1, base - 6, 'W');
      rect(g, ax, base - 5, 3, 3, 'd');
      row(g, ax, base - 4, 3, 'p');
      row(g, ax + 1, base - 2, 1, 'W');
    }
    socle(g, x, base - 1, w);
    return w;
  },

  // Hamnen — bryggan sticker ut över vattnet med ett förtöjt skrov. Vattnet
  // hör till byggnaden: en hamn utan vatten är en lada.
  harbour(g, x, base, lvl) {
    const w = 14 + (lvl - 1) * 3;
    rect(g, x, base - 3, w, 3, 'z');                   // bassängen
    row(g, x, base - 3, w, 'Z');
    cube(g, x, base - 11, 8, 8, { door: true });       // magasinet
    row(g, x + 8, base - 5, w - 8, 'w');               // bryggan
    row(g, x + 8, base - 4, w - 8, 'W');
    for (let i = 0; i < 2 + lvl; i++) col(g, x + 10 + i * 3, base - 3, 3, 'W');
    // Skrovet, förtöjt vid bryggans yttre ände.
    rect(g, x + w - 8, base - 3, 7, 2, 'W');
    row(g, x + w - 7, base - 4, 5, 'w');
    col(g, x + w - 5, base - 8, 4, 'W');               // masten
    return w + 2;
  },

  // Gjuteriet — kupolugn med mörk skorsten och glöd i mynningen, tackor
  // staplade utanför. Den enda elden i staden utom härden i megaron.
  foundry(g, x, base, lvl) {
    const w = 12 + (lvl - 1) * 2;
    cube(g, x, base - 9, 8, 8, { door: true });
    socle(g, x, base - 1, w);
    const fx = x + 8;
    rect(g, fx, base - 8, 5, 7, 'q');                  // ugnen
    row(g, fx, base - 8, 5, 'T');
    rect(g, fx + 1, base - 4, 3, 3, 'F');              // glöden
    row(g, fx + 1, base - 2, 3, 'f');
    col(g, fx + 2, base - 12, 4, 'V');                 // skorstenen
    col(g, fx + 3, base - 12, 4, 't');
    if (lvl >= 2) { rect(g, x + 14, base - 3, 4, 2, 'q'); row(g, x + 15, base - 4, 2, 'T'); }
    return w + 6;
  },

  // Stallet — bred låg länga med öppen framsida och en hage. Hästen är
  // stridsvagnens förutsättning, så stallet ska läsa som fordonsförråd.
  stable(g, x, base, lvl) {
    const w = 14 + (lvl - 1) * 3;
    cube(g, x, base - 8, w, 7);
    socle(g, x, base - 1, w);
    for (let i = 0; i < 2 + lvl; i++) rect(g, x + 2 + i * 4, base - 4, 3, 3, 'V');
    for (let i = 0; i < 5; i++) col(g, x + w + 1 + i * 3, base - 5, 4, 'W');
    row(g, x + w + 1, base - 5, 14, 'w');
    row(g, x + w + 1, base - 3, 14, 'W');
    return w + 16;
  },

  // Templet — HORN OF CONSECRATION på taket, ockrapelare i fasaden, altare
  // framför. Det är kultens byggnad i ett spel där palatskulten är den
  // dominerande ideologin, och den ska gå att peka ut på en halv sekund.
  temple(g, x, base, lvl) {
    const w = 13 + (lvl - 1) * 2;
    cube(g, x, base - 12, w, 11, { roof: 3, band: true });
    socle(g, x, base - 1, w);
    // Invigningshornen — den minoisk-mykenska kultens tydligaste tecken.
    const hx = x + (w >> 1) - 3;
    for (const dx of [0, 5]) {
      col(g, hx + dx, base - 15, 3, 'P');
      set(g, hx + dx + (dx ? 1 : -1), base - 15, 'p');
    }
    row(g, hx, base - 13, 6, 'P');
    // Pelarna i antis, samma ordning som megarons.
    for (const cx2 of [x + 2, x + w - 4]) {
      set(g, cx2, base - 8, 'O'); set(g, cx2 + 1, base - 8, 'O');
      for (let yy = base - 7; yy < base - 1; yy++) { set(g, cx2, yy, 'O'); set(g, cx2 + 1, yy, 'o'); }
    }
    rect(g, x + (w >> 1) - 1, base - 5, 4, 4, 'V');    // celladörren
    if (lvl >= 2) { rect(g, x + w + 2, base - 3, 5, 2, 'T'); row(g, x + w + 3, base - 4, 3, 'P'); }
    return w + (lvl >= 2 ? 8 : 1);
  },

  // Oljepressen — pressbommen är formen. En lång vägd hävarm över en stenkar,
  // olivhögen bredvid. Utan bommen läser den som vilket skjul som helst.
  olive_press(g, x, base, lvl) {
    const w = 12 + (lvl - 1) * 2;
    cube(g, x, base - 8, 8, 7, { door: true });
    socle(g, x, base - 1, w + 4);
    for (let i = 0; i < 8; i++) set(g, x + 8 + i, base - 6 - ((i * 3) >> 3), 'W');
    col(g, x + 15, base - 5, 4, 'T');                  // motvikten
    rect(g, x + 9, base - 4, 5, 3, 'q');               // karet
    row(g, x + 9, base - 4, 5, 'T');
    for (let i = 0; i < 2 + lvl; i++) set(g, x + 9 + i, base - 5, 'g');
    return w + 6;
  },

  // Vineriet — raden av pithoi. Lagringskrukorna är större än en människa och
  // står utanför huset; det är den bilden som skiljer vin från olja.
  winery(g, x, base, lvl) {
    const w = 10;
    cube(g, x, base - 8, 8, 7, { door: true });
    socle(g, x, base - 1, w);
    for (let i = 0; i < 2 + lvl; i++) {                // pithoi
      const px = x + 9 + i * 4;
      row(g, px + 1, base - 8, 2, 'd');
      rect(g, px, base - 7, 4, 6, 'p');
      col(g, px, base - 7, 6, 'P');
      col(g, px + 3, base - 7, 6, 'd');
      row(g, px + 1, base - 5, 2, 'o');                // målad bård
    }
    return w + (2 + lvl) * 4;
  },
};

function _mine(g, x, base, lvl, oreLight, oreDark) {
  const w = 13 + (lvl - 1) * 2;
  for (let i = 0; i < 4; i++)                          // bergssidan, trappad
    rect(g, x + i, base - 12 + i * 2, w - i * 2, 3, i % 2 ? 't' : 'q');
  rect(g, x + 3, base - 6, w - 7, 6, 'q');
  rect(g, x + ((w - 4) >> 1), base - 5, 4, 5, 'V');    // stollmynningen
  // Timmerbocken över mynningen — gruvans läsbara tecken.
  row(g, x + ((w - 4) >> 1) - 1, base - 6, 6, 'W');
  col(g, x + ((w - 4) >> 1) - 1, base - 6, 6, 'W');
  col(g, x + ((w - 4) >> 1) + 4, base - 6, 6, 'W');
  for (let i = 0; i < 1 + lvl; i++) {                  // malmhögen
    set(g, x + w + 1 + i * 2, base - 2, oreLight);
    set(g, x + w + 2 + i * 2, base - 1, oreDark);
    set(g, x + w + 1 + i * 2, base - 1, oreDark);
  }
  return w + (1 + lvl) * 2 + 2;
}

/** Bredden en byggnad tar i anspråk, utan att rita den — scenen behöver den
 *  för att kunna layouta innan den ritar. */
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

