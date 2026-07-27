// ── Stadssiluetterna (megaron_stader_20260727.md) ────────────────────────
//
// Kartmarkören för en bosättning var en 6–9 px ruta med en vimpel. En nygrundad
// koloni på 101 invånare och Knossos på 30 000 ritades som **samma** ruta —
// kartan bar ingen storlekssignal alls. Här får varje led sin egen massa,
// från ett stenhus till ett palatskomplex som nästan svämmar över hexen.
//
// **Kulturen är akhaisk, tiden 1400–1300 f.Kr.** Referensbildens italienska
// takpannestad är fel epok: sadeltak i tegel hör hemma tvåtusen år senare.
// Ett akhaiskt hus har PLATT jordtak på stensockel och soltorkat tegel, och
// från ¾-elevation ser man takets ovansida OCH fasaden under. Kalkputsen är
// kartans ljusaste ton, jordtaket ett av dess mörkaste — den kontrasten är hela
// den kubiska egeiska stadens läsbarhet, och den är dessutom fysiskt sann.
//
// **Palatset är ideologin, inte en byggnad i katalogen.** Palatskulten är
// spelets dominerande ordning, och därför föds megaron ur STORLEK: från led 2
// reser sig salen ur massan med pelare i antis, mörk lintel och en rökglugg
// över härden. (Minoisk-mykensk pelare smalnar NEDÅT — den är bredare upptill.
// Vid den här skalan är det en enda pixel, och den pixeln är kulturens
// signatur.) Vid led 3 bär terrassen målade ockrabband.
//
// Idag bara akhaier. Paletten och ledindelningen är gemensamma och tänkta att
// bära syskon per kultur senare; siluetterna är det inte.
//
// ── Varför kuber och inte handritad ASCII ────────────────────────────────
// Aktörerna är 14–16 px och ritas tecken för tecken. En palatsstad är 42×28 =
// 1 176 tecken, och den ska itereras om ett tjugotal gånger. Leden byggs därför
// av **stämplade kuber** — varje hus är (x, y, bredd, höjd) plus taktjocklek —
// och konturen läggs på i ett efterpass som ritar bläck i de TOMMA pixlarna
// runt hela massan.
//
// Det efterpasset ÄR princip 18 gjord till maskineri: **en kontur runt hela
// massan, aldrig mellan husen.** Utkastet som outlinade varje hus blev en
// streckkod på aktörerna och en tegelvägg här. Husen skiljs i stället åt av att
// varje kub har ljus vänsterkant och skuggad högerkant — två grannar möts som
// ljus mot mörk utan en enda bläckpixel emellan.
//
// Samma pixelregler som aktörerna och träden: fillRect på heltal, aldrig arc(),
// ¾-elevation med ljus uppifrån vänster, ingen gradient, inga rundade hörn.
//
// Iterera med `node tools/citysheet.mjs` — hela tabellen förstorad mot tre
// terränger, med en Spearmen-formation som skalreferens.

// Paletten och stämplarna bor i `pixelgrid.js` — stadsvyn behöver exakt samma
// bläck, tak och ljusriktning, och två uppsättningar hållna i synk för hand
// hade glidit isär vid första tonändringen.
import {
  CITY_PALETTE, newGrid, set, at, setIfEmpty, cube, outline, footSpans, paintFeet, toRuns,
  polyRows, plateFill, ringPixels, raiseWall,
} from './pixelgrid.js';
export { CITY_PALETTE };


// ── Megaron ──────────────────────────────────────────────────────────────
// Salen är inte ett större hus. Den har tre saker inget hus har: pelare i
// antis, mörk lintel över porten, och en rökglugg (lanternin) över härden som
// bryter takets horisontal. Det är de tre som gör att siluetten byter läsning
// från "hus" till "säte", och därmed hela palatskulten synlig på kartan.
function megaron(g, x, y, w, h) {
  // `h` räknar HELA salen inklusive terrassen; själva huskroppen är h − 2.
  const roofH = 3, bh = h - 2;
  cube(g, x, y, w, bh, { roof: roofH });
  // Lanternin över härden — ett upphöjt block med mörk rökglugg som bryter
  // takets horisontal. BRED och låg: ett utkasts 4×3 stod som ett vattentorn
  // ovanpå salen. Den ska läsa som en del av taket, inte som en påbyggnad.
  const lw = Math.max(6, (w >> 1) | 1), lx = x + ((w - lw) >> 1);
  for (let xx = 0; xx < lw; xx++) {
    set(g, lx + xx, y - 2, xx >= lw - 2 ? 'r' : 'R');
    set(g, lx + xx, y - 1, xx <= 1 || xx >= lw - 2 ? 'r' : 'V');
  }
  // Portalen i antis. "In antis" betyder bokstavligen TVÅ pelare mellan de
  // framskjutna väggändarna — inte en kolonnad. Ett utkast satte en pelare var
  // fjärde pixel över hela salens bredd och fick en röd streckkod som var det
  // klart starkaste på hela kartan: fem ockrastaplar på 24 px slog ut
  // kontrastbudgeten (princip 6) och gjorde salen till en reklamskylt.
  //
  // Förhallen är dessutom SMAL — ungefär tre åttondelar av salens bredd. Bred
  // blev den en stor skuggad putsyta, och ett utkast gjorde den helt mörk: salen
  // läste som en grottmynning och pelarna försvann i mörkret de skulle stå emot.
  // Djupet ligger i lintelns skugga, inte i hålets storlek.
  const py = y + roofH + 2;
  const pw = Math.max(8, (w * 3) >> 3), px0 = x + ((w - pw) >> 1);
  for (let yy = py; yy < y + bh; yy++)
    for (let xx = px0; xx < px0 + pw; xx++) set(g, xx, yy, 'd');
  // Mörk lintel som bär taket över förhallen.
  for (let xx = px0 - 1; xx <= px0 + pw; xx++) set(g, xx, py - 1, 'V');
  // Pelarna: ockraskaft som smalnar NEDÅT — kapitälet är en pixel bredare än
  // skaftet. Det är den minoisk-mykenska ordningens signatur, och vid den här
  // skalan ryms den i exakt den pixeln.
  for (const c of [px0 + 1, px0 + pw - 3]) {
    set(g, c, py, 'O'); set(g, c + 1, py, 'O');
    for (let yy = py + 1; yy < y + bh; yy++) {
      set(g, c, yy, 'O'); set(g, c + 1, yy, 'o');
    }
  }
  // Porthålet mellan pelarna — SMALT, i botten.
  const dw = 3, dx = px0 + ((pw - dw) >> 1);
  for (let yy = y + bh - 3; yy < y + bh; yy++)
    for (let xx = dx; xx < dx + dw; xx++) set(g, xx, yy, 'V');
  // Terrassen. Salen står på en stenplattform som skjuter ut ett par pixlar åt
  // varje håll — det är den som lyfter megaron ur myllret utan att göra den hög.
  for (let k = 0; k < 2; k++)
    for (let xx = -2 + k; xx < w + 2 - k; xx++)
      set(g, x + xx, y + bh + k, k === 0 ? 'T' : 't');
}

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
function build(w, h, pts, wl, place) {
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
    ([x, foot, bw, bh, depth, opts]) => (opts === 'megaron'
      ? megaron(g, x, foot - bh, bw, bh)
      : cube(g, x, foot - bh, bw, bh, { depth, door: true, ...(opts || {}) })));
  if (wl) {
    raiseWall(g, ring.front, wallH);
    gate(g, poly, wl, 5 + wl);
    if (wl >= 2) crenels(g, poly, wl);
    const first = poly.rows.find(Boolean);
    if (wl >= 2) tower(g, first[1] - 4, poly.y0 + 3, 3 + wl);
    if (wl >= 3) tower(g, first[0], poly.y0 + 3, 3 + wl);
  }
  const trimmed = trimTop(g);
  const feet = footSpans(trimmed);
  outline(trimmed);
  paintFeet(trimmed, feet);
  return toRuns(trimmed);
}

/** Skalar bort tomma rader överst. `CITY_BASE_OFFSET` räknar från massans FOT,
 *  och allt chrome (standar, aktivitetsmärke) ankras mot `sprite.h` — en handfull
 *  tomma rader i toppen hade fått vimpeln att sväva långt ovanför taket den ska
 *  röra (princip 25). Bara toppen trimmas: bredden är författad symmetrisk och
 *  botten ÄR foten. */
function trimTop(g) {
  let first = g.h;
  for (let y = 0; y < g.h && first === g.h; y++)
    for (let x = 0; x < g.w; x++) if (at(g, x, y) !== '.') { first = y; break; }
  if (first <= 0 || first >= g.h) return g;
  const out = newGrid(g.w, g.h - first);
  for (let y = first; y < g.h; y++)
    for (let x = 0; x < g.w; x++) set(out, x, y - first, at(g, x, y));
  return out;
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
  for (let i = 4, band = 0; i < n - 6; i += 5, band++) {
    const r = poly.rows[i];
    if (!r) continue;
    const y = poly.y0 + i;
    let x = r[0] + 2, k = 0;
    while (x < r[1] - 6) {
      const hs = hash3(seed, band, k++);
      // Vridningen måste sitta i PROPORTIONEN, inte i en pixels takskevning.
      // Ett utkast gav alla hus bredd 8–13 och djup 1–2, och då stod förstås
      // varenda hus vänt mot spelaren — en två pixlar sned takkant syns inte.
      // Nu dras huset som antingen LÅNGSIDA (bred fasad, grunt djup) eller
      // GAVEL (smal fasad, stort djup), och det är den skillnaden ögat läser
      // som två olika orienteringar (princip 20).
      const sideOn = ((hs >> 2) & 3) === 0;
      const w = sideOn ? 5 + (hs % 3) : 8 + (hs % 6);
      const depth = (sideOn ? 5 + ((hs >> 18) % 3) : 1 + ((hs >> 18) % 2))
        * ((hs >> 15) & 1 ? 1 : -1);
      const bh = 7 + ((hs >> 5) % 5);
      const foot = y + (((hs >> 10) % 5) - 2);
      const box = [x, foot - bh, x + w, foot];
      const blocked = reserved.some(q =>
        box[0] < q[2] && box[2] > q[0] && box[1] < q[3] && box[3] > q[1]);
      if (!blocked) out.push([x, foot, w, bh, depth]);
      x += w + 1 + ((hs >> 20) % 2);
    }
  }
  return out;
}

// ── De fyra leden ────────────────────────────────────────────────────────
// Ledens innehåll, inte bara storlek, bär berättelsen: ett hus → en by →
// ett säte → ett rike. Måtten står mot hexen, som är 44×38 logiska px.
//
// **Gården är ledets riktiga mått.** Massan växer i UTBREDNING, inte i höjd:
// anaktoron är 58 px bred mot hexens 44 och kan inte få plats (princip 31), men
// bara 34 mot dess 38 på höjden. Bergen får torna; städerna ska ligga
// innästlade i landskapet (Timothy 2026-07-27). Behöver ett led mer tyngd —
// gör gården bredare, aldrig staden högre.
//
// Husen anges som [x, FOT, bredd, höjd, djup] och sorteras på foten. `depth`
// vrider dem (se `cube`): tecknet växlar över klungan, bred fasad + grunt djup
// är ett hus vänt mot oss och smal fasad + djup ett vänt på tvären. Djupet
// hålls på 1–2: utkast på 5 gav långa diagonala takband tvärs massan och utkast
// på 3 gjorde takytan större än väggen, så husen läste som limpor.
//
// **Salen får inte torna.** Ett utkast gav anaktoron en 16×15 megaron mitt i
// massan. Den blev *"slottet i Agrabah"* och tog bort känslan av liv. Salen står
// nu bland husen, bara ett par pixlar högre, och bär hierarkin genom att vara
// den ENDA frontala volymen — pelare och lanternin, inte höjd.
//
// **Skalan är beslutad, inte ärvd** (Timothy 2026-07-27). Det fanns ingen regel
// som höll städerna små — bara min egen försiktighet inför att grannhexars
// centrum ligger 33 px isär. Referensens stad är ungefär tre hexbredder, och
// Timothy valde den proportionen: anaktoron är 100 px mot hexens 44. Två städer
// i grannhexar täcker alltså varandra till stor del, och det är ett medvetet
// pris för att stadsmiljön ska få plats att FINNAS. Detaljbudgeten var det som
// stoppade allt annat — på 58 px rymdes gård + mur + sju hus och inte ett hus
// till.
//
// Gården ska vara TÄTT bebyggd. Ett utkast la sju hus på anaktorons 93 px breda
// gård och staden läste som en sandbank med hus på — inhägnaden blev sin egen
// största yta. Nu står nitton hus där, och den öppna marken syns bara som
// gränder emellan dem, en gårdsplan innanför porten och en fri anmarsch
// framför salen (den senare är dessutom vad en mykensk borggård ÄR).
//
// Gårdens FRÄMRE tredjedel lämnas tom. Referensens städer har en öppen
// gårdsplan innanför porten, och det är den tomma ytan som säger att muren
// omsluter ett rum. Fyller man den blir inhägnaden en fylld klump igen.

// Led 0 — HAMLET (24×20). Nygrundad koloni: tre hus på en liten slagen gård.
// Samma bläck och samma tak som palatset — det är samma folk, bara färre av dem.
// Det ENDA ledet som ryms innanför hexen, och det är dess besked: här bor ännu
// inte tillräckligt många för att marken ska märka det.
const HAMLET = wl => build(40, 30, [
  [10, 10], [26, 10], [34, 12], [38, 17], [31, 24], [18, 27], [7, 26], [1, 18], [2, 13],
], wl, poly => packHouses(poly, 0x481, []));

// Led 1 — TOWN (36×26). En by som vuxit ihop kring sin gård: sex hus, ingen
// megaron ännu — palatset är inte något man reser, det är vad en plats BLIR.
const TOWN = wl => build(62, 42, [
  [15, 12], [38, 12], [50, 15], [60, 23], [53, 33], [34, 39], [15, 38], [2, 27], [3, 18],
], wl, poly => packHouses(poly, 0x1d7, []));

// Led 2 — CITY (50×31). Här reser sig MEGARON ur massan och staden byter läsning
// från "hus" till "säte". Gården är sex pixlar bredare än hexen: staden har
// börjat ta mark.
const CITY = wl => build(82, 52, [
  [20, 14], [50, 14], [66, 18], [80, 29], [72, 42], [47, 49], [21, 48], [3, 35], [5, 22],
], wl, poly => [
  [30, 35, 20, 18, 0, 'megaron'],
  // Salens fotavtryck plus anmarschen framför den — den designade tomheten.
  ...packHouses(poly, 0x3b9, [[27, 15, 53, 37], [31, 37, 50, 46]]),
]);

// Led 3 — ANAKTORON (58×34). Palatskomplex på en gård som inte får plats i
// rutan: 58 px mot hexens 44 på bredden (princip 31), 34 mot dess 38 på höjden.
// Murad är det här kartans tyngsta föremål, och tyngden kommer ur UTBREDNING.
const PALACE = wl => build(100, 62, [
  [24, 16], [60, 16], [80, 21], [97, 34], [88, 50], [58, 59], [26, 58], [4, 43], [6, 27],
], wl, poly => [
  // Magasinsflyglarna bar först var sitt ockraband. De blev markörens
  // STARKASTE färg — på sekundära byggnader, tvärs emot kontrastbudgeten, och
  // de läste som två röda snedstreck. Ockran hör till salen och ingen
  // annanstans: en accent som sitter på allt pekar inte på något.
  [38, 42, 24, 21, 0, 'megaron'],
  ...packHouses(poly, 0x7c5, [[35, 18, 65, 44], [39, 44, 61, 54]]),
]);

/** CITY_SPRITES[led][murnivå] — 4×4, genererade en gång vid modulladdning. */
export const CITY_SPRITES = [HAMLET, TOWN, CITY, PALACE]
  .map(make => [0, 1, 2, 3].map(wl => make(wl)));

// Basen (massans nedersta rad) hamnar så här långt under hexcentrum.
//
// Staden sitter **inte** centrerad på hexen utan står PÅ dess nedre del
// (Timothy 2026-07-27). Centrerad läste massan och namnet som två skilda
// föremål. Med foten nere pekar massan ut hexen den står på, och namnet blir
// dess sockel i stället för en lös rad under den.
//
// **17, inte 11** (Timothy 2026-07-27, andra omgången): med 11 stannade massan
// sex pixlar ovanför hexens underkant och den nedre fjärdedelen läste som TOM —
// en marginal runt märket, alltså en bricka igen, även när etiketten stod där.
// Namnet får stå i princip på hexstrecket, och då kan foten gå ned till 17.
// Hexens halva höjd är 19, så massan når nu marken den står på.
//
// Massan växer alltid UPPÅT ur foten — aldrig nedåt. Under den ligger etiketten,
// och en stad som hänger ned över sitt eget namn gör namnet oläsligt.
// Värdet är parat med `drawLabel`-offseten i `render/map.js`: ändras det ena
// måste det andra följa med, annars äter massan sitt namn.
//
// ── Städer TORNAR inte (Timothy 2026-07-27) ──────────────────────────────
// Bergen får resa sig över allt; staden får inte. En liten stad ska se
// **innästlad** ut i sin omgivning och en stor ska vara **en del av** den.
// Därför växer leden i BREDD, inte i höjd: anaktoron är 58 px mot hexens 44 men
// bara 36 mot dess 38, alltså precis så hög att den fyller sin ruta och inte en
// pixel högre. Höjd är bergens språk, utbredning är stadens. Den dagen ett led
// behöver mer tyngd: gör det bredare och lägre, aldrig högre.
export const CITY_BASE_OFFSET = 17;

const clamp3 = n => Math.max(0, Math.min(3, n | 0));
export const citySprite = (tier, walls) => CITY_SPRITES[clamp3(tier)][clamp3(walls)];

/** Ritar bosättningens massa centrerad på hexen. */
export function drawCityMass(ctx, tier, walls, cx, cy) {
  const sprite = citySprite(tier, walls);
  const ox = Math.round(cx) - (sprite.w >> 1);
  const oy = Math.round(cy) + CITY_BASE_OFFSET - sprite.h;
  for (const r of sprite.runs) {
    ctx.fillStyle = CITY_PALETTE[r.ch];
    ctx.fillRect(ox + r.x, oy + r.y, r.n, 1);
  }
  return sprite;
}
