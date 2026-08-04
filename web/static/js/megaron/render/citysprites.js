// ── Stadssiluetterna (megaron_stader_20260727.md) ────────────────────────
//
// Kartmarkören för en bosättning var en 6–9 px ruta med en vimpel. En nygrundad
// koloni på 101 invånare och Knossos på 30 000 ritades som **samma** ruta —
// kartan bar ingen storlekssignal alls.
//
// **En stad är ett OMRÅDE, inte en byggnadsklump.** Det var den insikten som
// löste allt annat: en ringmur runt en gård av slagen jord med husen inne på
// den. Sex omgångar la om massans FORM — bruten baslinje, porös frans, lutande
// klunga, djupaxel, färre och större hus — och ingen av dem tog bort känslan av
// en bricka på ett bräde, för staden saknade ett rum man kunde gå in i. Muren
// var dessförinnan ett vågrätt band framför husen, och ett band kan bara skilja
// fram från bak; det kan aldrig omsluta något. (megaron_terrangrendering
// princip 33. Referens: `img/f0382384-32ad-45b1-8195-b7f2903524ac.jpeg`.)
//
// Bieffekten är en spelvinst: med ringen blev **murnivån läsbar för första
// gången**. Nivå 0 är en öppen bosättning helt utan mur; förut var 0 och 3
// nästan omöjliga att skilja åt på kartan, och muren är det enda på markören
// spelaren fattar taktiska beslut om.
//
// **TVÅ led, inte fyra** (Timothy 2026-07-27). Serverns `size_tier` hade fyra
// steg — hamlet, town, city, anaktoron — men de två största siluetterna gick
// inte att få att läsa som städer: de blev modeller stående på en bricka, utan
// liv och med muren knappt synlig. Hellre två led som båda fungerar än fyra där
// hälften ljuger om vad en stad är. Gränsen går vid 800 invånare
// (`settlement.SizeTierThreshold`). Priset är att megaron inte längre reser sig
// ur massan på kartan; palatskulten bärs tills vidare av stadsvyn.
//
// **Kulturen är akhaisk, tiden 1400–1300 f.Kr.** Båda referensbildernas
// takpannestäder är fel epok: sadeltak i tegel hör hemma tvåtusen år senare.
// Det som hämtas ur dem är STRUKTUREN — inhägnaden, myllret, orienteringarna —
// aldrig taken. Ett akhaiskt hus har PLATT jordtak på stensockel och soltorkat
// tegel, och från ¾-elevation ser man takets ovansida OCH fasaden under.
// Kalkputsen är kartans ljusaste ton, jordtaket ett av dess mörkaste — den
// kontrasten är hela den kubiska egeiska stadens läsbarhet, och den är
// dessutom fysiskt sann.
//
// Idag bara akhaier. Paletten och ledindelningen är gemensamma och tänkta att
// bära syskon per kultur senare; siluetterna är det inte.
//
// ── Varför stämplar och inte handritad ASCII ─────────────────────────────
// Aktörerna är 14–16 px och ritas tecken för tecken. En stad är tio gånger så
// många pixlar och ska itereras om ett tjugotal gånger. Husen stämplas därför
// som kuber — (x, y, bredd, höjd, djup) — och konturen läggs på i ett efterpass
// som ritar bläck i de TOMMA pixlarna runt massan.
//
// Det efterpasset ÄR princip 18 gjord till maskineri: **en kontur runt hela
// massan, aldrig mellan husen.** Utkastet som outlinade varje hus blev en
// streckkod på aktörerna och en tegelvägg här. Husen skiljs i stället åt av att
// varje kub har ljus vänsterkant och skuggad högerkant. Marken får ingen kontur
// alls (princip 35) — en charcoalkant runt gården gjorde staden till ett
// klistermärke som låg PÅ terrängen i stället för i den.
//
// Samma pixelregler som aktörerna och träden: fillRect på heltal, aldrig arc(),
// ¾-elevation med ljus uppifrån vänster, ingen gradient, inga rundade hörn.
//
// Iterera med `node tools/citysheet.mjs` — men **avgör storleken mot
// `tools/shot.py cities`** (princip 37). Arket visar formen; kartan avgör
// måttet, och de två har redan sagt emot varandra en gång.

// Paletten och stämplarna bor i `pixelgrid.js` — stadsvyn behöver exakt samma
// bläck, tak och ljusriktning, och två uppsättningar hållna i synk för hand
// hade glidit isär vid första tonändringen.
import {
  CITY_PALETTE, newGrid, set, at, setIfEmpty, cube, outline, footSpans, paintFeet, toRuns,
  polyRows, plateFill, ringPixels, raiseWall,
} from './pixelgrid.js';
export { CITY_PALETTE };


// ── Kyklopisk ringmur ────────────────────────────────────────────────────
// Fusk med ett sekel, med Timothys ord: kyklopisk fortifikation hör till
// 1300-talet, inte 1400. Muren är det som gör en stor stad IMPONERANDE, och den
// är dessutom den enda byggnad spelaren fattar taktiska beslut om på kartan.
//
// Den är en RING, inte ett band. Utkasten ritade den som ett vågrätt stycke
// framför husen — och ett band kan bara skilja fram från bak, aldrig omsluta
// något. Med ringen får staden en insida, och det är insidan som gör den till
// en plats i stället för ett märke (referensbilden, Timothy 2026-07-27).
//
// Nivåerna är murens verkliga wall_level:
//   0 — ingen mur: en öppen bosättning, bara gården och husen. Att det syns är
//       en vinst i sig — förut var mur 0 och mur 3 nästan omöjliga att skilja.
//   1 — låg obruten ringmur
//   2 — + LEJONPORT (mörkt portgap med avlastningstriangeln över, den mykenska
//       arkitekturens mest kända gest) och ett flankerande torn
//   3 — + högre liv, tinnar längs krönet och torn i två hörn
function gate(g, poly, level, gateW) {
  // Porten sitter i den främre murens mitt — vägen ut ur staden.
  const last = poly.rows[poly.rows.length - 1] || poly.rows[poly.rows.length - 2];
  const y = poly.y0 + poly.rows.length - 1;
  const cx = (last[0] + last[1]) >> 1, gx = cx - (gateW >> 1);
  for (let yy = y - 2; yy <= y; yy++)
    for (let x = gx; x < gx + gateW; x++) set(g, x, yy, 'V');
  if (level >= 2)
    for (let k = 0; k < 2; k++)
      for (let x = gx + k; x < gx + gateW - k; x++) set(g, x, y - 3 - k, 'V');
}

function crenels(g, poly, level) {
  // Tinnar bara mot HIMLEN — det är tandningen mot bakgrunden som gör en mur
  // imponerande vid den här skalan, inte höjden. Satta villkorslöst stansade de
  // in tandrader i husen innanför muren och lästes som brus.
  const first = poly.rows.find(Boolean);
  const y = poly.y0;
  for (let x = first[0]; x <= first[1]; x += 3) setIfEmpty(g, x, y - 4, 'T');
  if (level >= 3) for (let x = first[0] + 1; x <= first[1]; x += 3) setIfEmpty(g, x, y - 5, 'T');
}

function tower(g, x, y, h) {
  for (let k = 0; k < h; k++)
    for (let xx = 0; xx < 5; xx++)
      set(g, x + xx, y - k, xx >= 4 ? 't' : (k === h - 1 ? 'T' : 'q'));
  for (let xx = 0; xx < 5; xx += 2) setIfEmpty(g, x + xx, y - h, 'T');
}

// ── Bygget ───────────────────────────────────────────────────────────────
// Lagerordningen ÄR djupordningen, och den är inte förhandlingsbar:
//   gård → bakre mur → husen bakifrån och fram → främre mur → port → tinnar.
// Den bakre muren måste ligga under husen (vi ser den bortom dem) och den
// främre över (den står framför dem och skymmer deras fötter, princip 23).
function build(w, h, pts, wl, place, towers = []) {
  const g = newGrid(w, h);
  const poly = polyRows(pts);
  plateFill(g, poly);
  const wallH = wl ? 3 + wl : 0;                   // 4 · 5 · 6 px murliv
  const ring = ringPixels(poly, 2);
  // Bakre muren är LÅG (2 px) oavsett nivå: den ska antyda att ringen sluter
  // sig bakom staden, inte skymma husen den ligger bakom. Nivån läses på den
  // främre muren, som spelaren faktiskt ser rakt på.
  if (wl) raiseWall(g, ring.back, 3);
  // Husen sorteras på FOTEN: det som står längre fram ritas sist och skymmer
  // det bakom. Utan sorteringen blir klungan platt oavsett hur volymerna ser ut.
  place(poly).sort((a, b) => a[1] - b[1]).forEach(
    ([x, foot, w2, h2, depth]) =>
      cube(g, x, foot - h2, w2, h2, { depth, door: true }));
  if (wl) {
    raiseWall(g, ring.front, wallH);
    gate(g, poly, wl, 5 + wl);
    if (wl >= 2) crenels(g, poly, wl);
    // Tornen satt förut på ÖVERSTA radens två ändar, uträknade ur `poly.rows`.
    // Med ett långt vågrätt bakre murlöp hamnar båda i samma ände av staden, och
    // de bröt aldrig siluetten i sidled — de stod mitt inne i husfältet. Nu är
    // de en handsatt lista per led (princip 34: ett halvdussin element bärs av
    // en koordinatlista), ankrad i murringens FAKTISKA hörn. `[x, y, minNivå]`.
    //
    // De ritas sist, som förut. Det är rätt just därför att de sitter i
    // ytterhörnen: ingen husrad ligger framför ett hörntorn, så inget behöver
    // skymma det — och därmed rubbas inte djupordningen på rad 118.
    for (const [tx, ty, min] of towers)
      if (wl >= min) tower(g, tx, ty, 3 + wl);
  }
  const trimmed = trimBlank(g);
  const feet = footSpans(trimmed);
  outline(trimmed);
  paintFeet(trimmed, feet);
  const sprite = toRuns(trimmed);
  sprite.yardTop = yardTop(poly, w, trimmed.dy);
  return sprite;
}

/** Gårdens översta rad per kolumn — MARKENS kontur, inte siluettens.
 *  Kuststadens bank (`render/map.js drawCityBank`) behöver veta var marken
 *  slutar och taken tar vid: sand ovanför ett tak är sand i luften. Den går
 *  inte att läsa ur den färdiga spriten, för gårdens övre rader är regelmässigt
 *  övertäckta av bakre muren och husen — mätt ur pixlarna började marken fyra
 *  rader för lågt och havet lyste in genom gårdens fransade bakkant.
 *  Polygonen VET var gården ligger; den är samma `poly` bygget redan använder. */
function yardTop(poly, w, dy) {
  const top = new Int16Array(w).fill(-1);
  poly.rows.forEach((r, i) => {
    if (!r) return;
    const y = poly.y0 + i - dy;
    for (let x = Math.max(0, r[0]); x <= Math.min(w - 1, r[1]); x++)
      if (top[x] < 0) top[x] = y;
  });
  return top;
}

/** Skalar bort tomma rader i BÅDA ändar. Ankringen (`cityTop`/`cityFoot`) och allt
 *  chrome (standar, aktivitetsmärke, garnisonsprick) räknar ur `sprite.h` — så
 *  `h` måste vara massans verkliga höjd, annars ankras allting mot luft.
 *
 *  Funktionen trimmade förut bara toppen, med motiveringen "botten ÄR foten".
 *  Det stämde inte: rutnätet är författat med marginal, och `TOWN` bar fem tomma
 *  rader under foten (h = 41 för en massa som slutar på rad 35). Vimpeln satt
 *  därmed rätt — den ankras i toppen — men garnisonspricken pekade fem pixlar ned
 *  i tomrummet, och centreringen 2026-08-04 centrerade rutnätet i stället för
 *  staden. Hittat genom att dumpa spriten som ASCII, inte i en skärmdump: fem
 *  tomma rader syns inte i en bild, bara i datan.
 *
 *  Bredden lämnas: den är författad symmetrisk och `ox` räknar ur `w >> 1`.
 *  Returnerar rutnätet med `dy` = antal bortskalade rader i TOPPEN, som `yardTop`
 *  behöver för att räkna om gårdens koordinater. */
function trimBlank(g) {
  let first = -1, last = -1;
  for (let y = 0; y < g.h; y++)
    for (let x = 0; x < g.w; x++)
      if (at(g, x, y) !== '.') { if (first < 0) first = y; last = y; break; }
  if (first < 0) return Object.assign(g, { dy: 0 });
  const out = newGrid(g.w, last - first + 1);
  for (let y = first; y <= last; y++)
    for (let x = 0; x < g.w; x++) set(out, x, y - first, at(g, x, y));
  return Object.assign(out, { dy: first });
}


// ── Kvartersgeneratorn ───────────────────────────────────────────────────
// Handplacerade koordinatlistor bär upp till ett halvdussin hus. Över det
// tappar man kompositionen: ett utkast med nitton handsatta hus på anaktorons
// gård la dem som en RING längs kanten med ett tomt torg i mitten, och både
// rymden och myllret försvann (Timothy 2026-07-27). Samma lärdom som lunden och
// bergsmassiven redan gett — täthet är ett FÄLT MED REGLER, inte en lista.
//
// Reglerna:
//   · Husen läggs i kvartersband tvärs gården, ett band var sjätte rad. Bandet
//     är bara utgångsläget: varje hus förskjuts ±2 rader, så raden bryts.
//   · Bredd 8–13, höjd 7–11, gränd 1–3 px. Spridningen — inte antalet — är det
//     som gör en klunga till bebyggelse (princip 2).
//   · Vridningen (`depth`) dras ur samma hash, så orienteringarna blandas utan
//     att någon rad blir spegelsymmetrisk.
//   · RESERVAT hålls fria: salens fotavtryck och dess anmarsch. Det är den
//     designade tomheten som ger rymd — inte det som råkar bli över.
//   · Allt är härlett ur ett fast frö per led. Samma led ger alltid samma stad;
//     två led ger aldrig samma (princip 14, presentationslagret).
const hash3 = (a, b, c) => {
  let h = Math.imul(a + 0x9e3779b9, 0x85ebca6b) ^ Math.imul(b + 0x165667b1, 0xc2b2ae35);
  h = Math.imul(h ^ c, 0x27d4eb2d);
  h = Math.imul(h ^ (h >>> 15), 0x2545f491);
  return (h ^ (h >>> 13)) >>> 0;
};

function packHouses(poly, seed, reserved = []) {
  const out = [], n = poly.rows.length;
  // Banden går ända fram till murens fot — annars samlas bebyggelsen i gårdens
  // övre halva och den nedre blir en sandbank; referensarkets städer har hus
  // ända ut i muren och marken syns bara som gränder.
  //
  // Men de får inte ligga TÄTARE än var femte rad. Ett utkast på var fjärde lät
  // varje band svälja bandet bakom sig och hela staden blev en grötig kulle:
  // det är takraderna som skiljer husen åt, och de måste synas.
  for (let i = 4, band = 0; i < n - 2; i += 5, band++) {
    const r = poly.rows[i];
    if (!r) continue;
    const y = poly.y0 + i;
    let x = r[0] + 4, k = 0;
    while (x < r[1] - 8) {
      const hs = hash3(seed, band, k++);
      // Vridningen kommer ur takytans RIKTNING, inte ur att huset sträcks ut.
      // Två utkast föll på samma missförstånd: att ett hus vänt på tvären måste
      // vara smalt och djupt. Ett hus med 5 px fasad och 6 px djup får en takyta
      // som är 17 px bred och 6 hög — nittio procent tak, alltså en planka som
      // skjuter tvärs över staden. Referensens hus är i stort sett kubiska; det
      // ögat läser som orientering är åt vilket håll takrutan viker bort, och
      // det syns redan vid tre pixlars djup.
      //
      // Djupet hålls därför i proportion till bredden och TECKNET bär
      // vridningen. Höger gavel ligger i skugga och vänster i ljus, så
      // blandningen ger klungan valörvariation på köpet.
      const w = 7 + (hs % 6);
      // Djup 2–3, inte 1–2. Takets OVANSIDA (M/m) är den enda stora ljusa ytan
      // ett hus har, och dess höjd ÄR djupet: vid djup 1 var den en enda rad och
      // klungan läste som en fasadrad utan tak. Vid 2–3 blir ytan 2–3 rader och
      // varje hus får en läsbar takruta att vika bort med. Kubstämpeln bär enligt
      // sin egen kommentar 5–6 utan att formen havererar; 3 är taket här därför
      // att gårdens övre band annars trycker upp taken ur rutnätet.
      const depth = (1 + ((hs >> 18) % 2)) * ((hs >> 15) & 1 ? 1 : -1);
      const bh = 7 + ((hs >> 5) % 5);
      const foot = y + (((hs >> 10) % 5) - 2);
      const box = [x, foot - bh, x + w, foot];
      const blocked = reserved.some(q =>
        box[0] < q[2] && box[2] > q[0] && box[1] < q[3] && box[3] > q[1]);
      if (!blocked) {
        out.push([x, foot, w, bh, depth]);
        // Indragen ÖVERVÅNING. Referensarket (Timothy 2026-07-27) visar en
        // egeisk lerstad som en stapel: husen har våningar som dras in och
        // terrasseras, och det är staplingen — inte antalet hus — som ger
        // massan liv. Den sätts på huset TAK och får därmed en mindre fot, så
        // sorteringen ritar den före sin egen bas: en indragen våning står
        // längre BORT från betraktaren, precis som den ska.
        if (w >= 9 && ((hs >> 25) & 1) === 0)
          out.push([x + 2, foot - bh + 2, w - 5, 4 + ((hs >> 7) % 3), depth]);
      }
      x += w + 1 + ((hs >> 20) % 2);
    }
  }
  return out;
}

// ── De två leden ────────────────────────────────────────────────────────
// Måtten står mot hexen, som är 44×38 logiska px.
//
// **Gården är ledets riktiga mått.** Massan växer i UTBREDNING, inte i höjd:
// staden är 62 px bred mot hexens 44 och kan inte få plats (princip 31), men
// bara 40 mot dess 38 på höjden. Bergen får torna; städerna ska ligga
// innästlade i landskapet (Timothy 2026-07-27). Behöver ett led mer tyngd —
// gör gården bredare, aldrig staden högre.
//
// **Gården ska vara TÄTT bebyggd.** Ett utkast la sju hus på en 93 px bred gård
// och staden läste som en sandbank med hus på — inhägnaden blev sin egen största
// yta. Marken ska synas som GRÄNDER mellan hus, inte som ett torg. Ett utkast
// reserverade dessutom 30×26 px mitt på gården åt en sal, och då stod hela
// staden som en modell på en platta.
//
// Polygonen är gårdens fotavtryck. Den ska vara påtagligt oregelbunden: en jämn
// sexhörning gör inhägnaden till en förstorad hex, alltså en bricka igen.

// Led 0 — HAMLET (40 px). Nygrundad koloni: en handfull hus på en liten slagen
// gård. Samma bläck och samma tak som staden — det är samma folk, bara färre av
// dem. Det ENDA ledet som ryms innanför hexen, och det är dess besked: här bor
// ännu inte tillräckligt många för att marken ska märka det.
const HAMLET = wl => build(40, 30, [
  [10, 11], [27, 11], [28, 14], [33, 14], [33, 21], [27, 24], [12, 24], [4, 20], [4, 14],
], wl, poly => packHouses(poly, 0x481, []),
  [[29, 17, 2], [2, 17, 2], [23, 23, 3], [9, 23, 3]]);

// Led 1 — TOWN (62 px). Allt från 800 invånare och uppåt: myllret innanför
// ringmuren. Det här ledet bär numera hela spannet upp till Knossos, så det får
// inte läsa som "mellanstor" — det ska läsa som EN STAD.
const TOWN = wl => build(62, 42, [
  [13, 13], [40, 13], [41, 17], [56, 17], [56, 30], [48, 35], [20, 35], [8, 29], [7, 18],
], wl, poly => packHouses(poly, 0x1d7, []),
  [[52, 21, 2], [5, 22, 2], [44, 33, 3], [14, 33, 3]]);

/** CITY_SPRITES[led][murnivå] — 2×4, genererade en gång vid modulladdning. */
export const CITY_SPRITES = [HAMLET, TOWN]
  .map(make => [0, 1, 2, 3].map(wl => make(wl)));

// ── Var massan sitter i hexen ────────────────────────────────────────────
//
// **Staden utgår från hexens CENTRUM** (Timothy 2026-08-04). Den växer symmetriskt
// uppåt och nedåt ur mittpunkten, så ett led som växer breder ut sig åt båda hållen
// i stället för att skjuta upp ur sin fot.
//
// Detta ERSÄTTER den fasta `CITY_BASE_OFFSET = 17` (Timothy 2026-07-27, som i sin
// tur ersatte 11). Den regeln la foten på cy+17 och lät massan växa uppåt därifrån,
// vilket gjorde att `TOWN` — 41 px hög i en 38 px hög hex — hamnade med sitt spann på
// −24..+17: fyra pixlar tyngdpunkt ovanför hexens mitt, alltså "övre mitten".
// Det ursprungliga skälet till 17 (namnet ska vara massans sockel, ingen tom
// marginal under staden) bärs nu av att massan är HÖGRE än hexen — den når
// underkanten ändå, utan att behöva ankras i den.
//
// Sex ställen räknade förut ut `cy + CITY_BASE_OFFSET - sprite.h` var för sig
// (massan, banken, standaret, garnisonspricken, riggen ×2). Ankringen bor nu i
// EN funktion; en offset som beräknas på sex ställen är sex chanser att de glider isär.
//
// ── Städer TORNAR inte (Timothy 2026-07-27, bekräftad 2026-08-04) ────────
// Bergen får resa sig över allt; staden får inte. En liten stad ska se
// **innästlad** ut i sin omgivning och en stor ska vara **en del av** den.
// Därför växer leden i BREDD, inte i höjd. Sedan 2026-08-04 gäller taket åt BÅDA
// hållen: inget dominant block, varken högre eller bredare än sina grannar —
// anaktoron syns i stadsvyn, inte i stadsmassan på kartan.

/** Massans överkant relativt hexcentrum. Enda sanningen om ankringen. */
export const cityTop = sprite => -(sprite.h >> 1);
/** Massans fot (raden UNDER nedersta pixeln) relativt hexcentrum. */
export const cityFoot = sprite => sprite.h - (sprite.h >> 1);

const clamp = (n, hi) => Math.max(0, Math.min(hi, n | 0));
export const citySprite = (tier, walls) =>
  CITY_SPRITES[clamp(tier, CITY_SPRITES.length - 1)][clamp(walls, 3)];

/** Ritar bosättningens massa centrerad på hexen. */
export function drawCityMass(ctx, tier, walls, cx, cy) {
  const sprite = citySprite(tier, walls);
  const ox = Math.round(cx) - (sprite.w >> 1);
  const oy = Math.round(cy) + cityTop(sprite);
  for (const r of sprite.runs) {
    ctx.fillStyle = CITY_PALETTE[r.ch];
    ctx.fillRect(ox + r.x, oy + r.y, r.n, 1);
  }
  return sprite;
}
