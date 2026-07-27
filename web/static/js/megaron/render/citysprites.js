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

export const CITY_PALETTE = {
  K: '#1F1A14',  // charcoal — samma bläck som aktörerna. En karta, ett bläck.
  P: '#EFE4CB',  // kalkputs, solbelyst kant. L≈228, kartans ljusaste ton —
                 // kontrastbudgetens mottagare (princip 6).
  p: '#D2C3A0',  // kalkputs, kropp. L≈196
  d: '#A08D71',  // kalkputs, skuggad högerkant och förhallens innervägg. L≈143
  R: '#8A6A44',  // platt jordtak, solbelyst. L≈111 — MÖRKARE än väggen, för att
                 // jord är mörkare än kalk. Bandet tak/vägg är hela den kubiska
                 // stadens rytm, och det som saknades i första utkastet: allt
                 // låg då i spannet 170–217 och massan läste som en tvål.
                 // VARM, inte olivgrå: samma valör som utkastets #7C7059 men
                 // med hue flyttad till jord. Det grå utkastet höll strukturen
                 // och tappade kulturen — en akhaisk stad i egeisk sol är inte
                 // grå, och taket är dess största yta.
  r: '#5E4630',  // platt jordtak, skuggat. L≈75
  T: '#B5A992',  // kyklopisk sten, ljus sida (muren, palatsterrassen). L≈170.
                 // Utkastets #8E8474 låg L 133 — en halv enhet från slätten
                 // (134). Muren hade varit OSYNLIG i valör mot kartans största
                 // yta (princip 17), och den är just det spelaren ska kunna
                 // fatta beslut om.
  q: '#9C907A',  // kyklopisk sten, murliv. L≈145
  t: '#6E6552',  // kyklopisk sten, skuggad sida och blockfog. L≈102
  O: '#B0442A',  // röd ockra — målad puts, pelarskaft, lintel. Tjurpolen ur
                 // [[megaron_minoisk_visuell_riktning]]: det som håller
                 // stenmassan från att bli grå.
  o: '#7C2A18',  // röd ockra, skuggad
  W: '#6B5334',  // timmer, bjälke (samma som aktörernas trä)
  V: '#141110',  // portgap, fönsterglugg, rökglugg. L≈18, kartans djupaste
                 // mörker — kontrastbudgetens andra mottagare.
  G: '#2A2419',  // markskugga (samma som aktörerna)
};

// ── Kubstämpeln ──────────────────────────────────────────────────────────
// Ett akhaiskt hus i ¾-elevation: mörkt platt jordtak överst, kalkad fasad
// under, mörk dörröppning i botten. Ljuset kommer uppifrån vänster, så vänster
// kolumn är högdager och de två högra är skugga — det är den kanten som skiljer
// två grannhus åt utan bläck.
function cube(g, x, y, w, h, opts = {}) {
  const roofH = opts.roof ?? 2;
  for (let yy = 0; yy < h; yy++) {
    for (let xx = 0; xx < w; xx++) {
      let ch;
      if (yy < roofH) {
        ch = xx >= w - 2 ? 'r' : 'R';
      } else {
        ch = xx === 0 ? 'P' : (xx >= w - 2 ? 'd' : 'p');
      }
      set(g, x + xx, y + yy, ch);
    }
  }
  // Dörren: mörk, i fasadens botten, alltid minst 2 px bred — en 1 px-glugg
  // försvinner i skalningen och läser som smuts.
  if (opts.door && h - roofH >= 3 && w >= 5) {
    const dx = x + ((w - 2) >> 1);
    for (let yy = y + h - 3; yy < y + h; yy++) { set(g, dx, yy, 'V'); set(g, dx + 1, yy, 'V'); }
  }
  if (opts.window && h - roofH >= 4 && w >= 6) {
    set(g, x + 1, y + roofH + 1, 'V');
    set(g, x + w - 4, y + roofH + 1, 'V');
  }
  // Målat ockraband längs fasadens överkant — palatsarkitekturens signatur.
  if (opts.band) {
    for (let xx = 1; xx < w - 1; xx++) set(g, x + xx, y + roofH, xx >= w - 3 ? 'o' : 'O');
  }
}

// ── Megaron ──────────────────────────────────────────────────────────────
// Salen är inte ett större hus. Den har tre saker inget hus har: pelare i
// antis, mörk lintel över porten, och en rökglugg (lanternin) över härden som
// bryter takets horisontal. Det är de tre som gör att siluetten byter läsning
// från "hus" till "säte", och därmed hela palatskulten synlig på kartan.
function megaron(g, x, y, w, h) {
  const roofH = 3;
  cube(g, x, y, w, h, { roof: roofH });
  // Lanternin över härden — ett upphöjt block med mörk rökglugg som bryter
  // takets horisontal. BRED och låg: det första utkastets 4×3 stod som ett
  // vattentorn ovanpå salen. Den ska läsa som en del av taket, inte som en
  // påbyggnad, alltså minst halva salens bredd och bara två rader hög.
  const lw = Math.max(6, (w >> 1) | 1), lx = x + ((w - lw) >> 1);
  for (let xx = 0; xx < lw; xx++) {
    set(g, lx + xx, y - 2, xx >= lw - 2 ? 'r' : 'R');
    set(g, lx + xx, y - 1, xx <= 1 || xx >= lw - 2 ? 'r' : 'V');
  }
  // Portalen i antis. Första utkastet gjorde hela förhallen till ett svart hål
  // — salen läste som en grottmynning, och pelarna försvann i mörkret de skulle
  // stå emot. Förhallen är i stället en SKUGGAD putsyta (d): då syns ockran, och
  // salen läser som arkitektur i stället för som ett gap.
  const py = y + roofH + 2;
  const c1 = x + 2, c2 = x + w - 4;
  for (let yy = py; yy < y + h; yy++)
    for (let xx = c1; xx <= c2 + 1; xx++) set(g, xx, yy, 'd');
  // Mörk lintel som bär taket över förhallen — den vågräta skuggan som säger
  // att det finns ett rum bakom.
  for (let xx = c1 - 1; xx <= c2 + 2; xx++) set(g, xx, py - 1, 'V');
  // Pelarna: ockraskaft som smalnar NEDÅT — kapitälet är en pixel bredare än
  // skaftet. Det är den minoisk-mykenska ordningens signatur, och vid den här
  // skalan ryms den i exakt en pixel.
  for (const c of [c1, c2]) {
    set(g, c, py, 'O'); set(g, c + 1, py, 'O');            // kapitäl, 2 px
    for (let yy = py + 1; yy < y + h; yy++) {
      set(g, c, yy, 'O'); set(g, c + 1, yy, 'o');          // skaft, 2 px
    }
  }
  // Porthålet mellan pelarna — SMALT, i botten. Utkastets 4×4 svalde hela
  // förhallen: salen läste som en garageöppning med två röda stolpar bredvid,
  // inte som ett rum med pelare framför. Djupet ligger i lintelns skugga, inte
  // i hålets storlek.
  const dw = w >= 15 ? 3 : 2, dx = x + ((w - dw) >> 1);
  for (let yy = y + h - 3; yy < y + h; yy++)
    for (let xx = dx; xx < dx + dw; xx++) set(g, xx, yy, 'V');
}

// ── Kyklopisk mur ────────────────────────────────────────────────────────
// Fusk med ett sekel, med Timothys ord: kyklopisk fortifikation hör till
// 1300-talet, inte 1400. Muren är det som gör en stor stad IMPONERANDE, och
// den är dessutom den enda byggnad spelaren fattar taktiska beslut om på
// kartan — därför får den kontrastbudgetens ljusa sten och sin egen tyngd.
//
// Den ritas EFTER husen och FÖRE konturpasset: efter, för att en mur står
// framför staden och ska skymma husens fötter; före, för att den ska ingå i
// samma silhuett och inte få en egen kontur (princip 18).
//
// Nivåerna är murens verkliga wall_level, inte en estetisk skala:
//   1 — ringmur: låg, obruten, ett band framför staden
//   2 — + torn i båda ändar och en LEJONPORT: mörkt portgap med avlastnings-
//       triangeln över, den mykenska arkitekturens mest kända gest
//   3 — + högre torn med tinnar; muren blir stadens ansikte
function rampart(g, x, y, w, level, gateW = 4) {
  const h = 2 + level;                     // 3 · 4 · 5 px murliv
  // Muren är LÅG med flit. Första utkastets 4–6 px höga band gick tvärs över
  // hela massan och åt upp den lägre staden: markören blev en ljus platta med
  // ett tak ovanpå, och det imponerande försvann just för att det inte fanns
  // någon stad kvar att imponera för. En mur ska ha en stad RESANDE SIG bakom
  // sig — det är kontrasten låg mur / hög sal som bär tyngden.
  // Skuggraden över krönet. Utan den läser muren som en TERRASS staden står på
  // i stället för som en mur staden står bakom: krönlisten (L 170) mot
  // kalkputsen (L 196) är bara 26 i valör, och den kanten är hela beskedet om
  // vad som är framför vad. En pixel mörk sten i övergången gör muren till ett
  // eget djupplan — det är inte en kontur inne i massan (princip 18), det är
  // ockluderingskanten mellan två plan.
  for (let xx = 0; xx < w; xx++) set(g, x + xx, y - 1, 't');
  for (let yy = 0; yy < h; yy++) {
    for (let xx = 0; xx < w; xx++) {
      // Ljus krönlist, mellanton i livet, mörk fotskugga: tre valörer, annars
      // läser muren som en enda platt yta.
      let ch = yy === 0 ? 'T' : (yy >= h - 1 ? 't' : 'q');
      // Kyklopiska block: oregelbundna fogar, deterministiskt utlagda. Ett
      // regelbundet rutmönster skulle läsa som tegel — och skillnaden mellan
      // kyklopisk och klassisk mur syns även vid 5 px.
      if (yy > 0 && yy < h - 1 && ((xx * 7 + yy * 23) % 11) < 2) ch = 't';
      set(g, x + xx, y + yy, ch);
    }
  }
  // Porten. En mur utan port är en kaj.
  const gx = x + ((w - gateW) >> 1);
  for (let yy = y + h - Math.min(h, 3); yy < y + h; yy++)
    for (let xx = gx; xx < gx + gateW; xx++) set(g, xx, yy, 'V');
  if (level >= 2) {
    // Avlastningstriangeln över lintelen — Lejonporten i Mykene, den mykenska
    // arkitekturens mest kända gest, och den ryms i sex pixlar.
    for (let k = 0; k < 2; k++)
      for (let xx = gx + k; xx < gx + gateW - k; xx++) set(g, xx, y + h - 4 - k, 'V');
    // Tinnar längs krönet: det är TANDNINGEN mot himlen som gör en mur
    // imponerande vid den här skalan, inte höjden.
    //
    // De ritas bara där murkrönet faktiskt möter TOM bakgrund. Utkastet satte
    // dem villkorslöst och stansade in tandrader i de hus som står innanför
    // muren — kreneleringen läste som brus mitt i staden. Att bara rita mot
    // himlen är dessutom fysiskt sant: en tinne bakom ett hustak syns inte.
    for (let xx = 1; xx < w - 1; xx += 3) {
      setIfEmpty(g, x + xx, y - 1, 'T');
      if (level >= 3) setIfEmpty(g, x + xx, y - 2, 'T');
    }
    // Torn i båda ändar, flankerande porten på avstånd. Samma regel: de reser
    // sig i det fria, aldrig genom bebyggelsen.
    const th = level >= 3 ? 4 : 2, tw = 4;
    for (const tx of [x, x + w - tw]) {
      for (let yy = 1; yy <= th; yy++)
        for (let xx = 0; xx < tw; xx++)
          setIfEmpty(g, tx + xx, y - yy, xx >= tw - 1 ? 't' : (yy === th ? 'T' : 'q'));
      for (let xx = 0; xx < tw; xx += 2) setIfEmpty(g, tx + xx, y - th - 1, 'T');
    }
  }
}

// ── Rutnät, kontur, skugga ───────────────────────────────────────────────
const newGrid = (w, h) => ({ w, h, px: new Array(w * h).fill('.') });
function set(g, x, y, ch) {
  if (x < 0 || y < 0 || x >= g.w || y >= g.h) return;
  g.px[y * g.w + x] = ch;
}
function setIfEmpty(g, x, y, ch) { if (at(g, x, y) === '.') set(g, x, y, ch); }
const at = (g, x, y) =>
  (x < 0 || y < 0 || x >= g.w || y >= g.h) ? '.' : g.px[y * g.w + x];

/** Princip 18 som maskineri: bläck ritas i de TOMMA pixlarna runt massan, så
 *  konturen alltid är silhuettens gräns mot bakgrunden — aldrig en linje inne
 *  i massan. Diagonalgrannar räknas med, annars läcker bakgrunden in i varje
 *  trappsteg där två kuber möts i höjd. */
function outline(g) {
  const add = [];
  for (let y = 0; y < g.h; y++) {
    for (let x = 0; x < g.w; x++) {
      if (at(g, x, y) !== '.') continue;
      let touches = false;
      for (let dy = -1; dy <= 1 && !touches; dy++)
        for (let dx = -1; dx <= 1; dx++) {
          const n = at(g, x + dx, y + dy);
          if (n !== '.' && n !== 'K' && n !== 'G') { touches = true; break; }
        }
      if (touches) add.push([x, y]);
    }
  }
  for (const [x, y] of add) set(g, x, y, 'K');
}

/** Markskuggan: EN sammanhängande rad under massans nedersta kant, förskjuten
 *  ett steg åt höger — samma riktning som aktörernas. Första utkastet la skugga
 *  under varje kolumns egen underkant och fick en prickad diagonal ut ur
 *  siluetten: ett kometsvans-spår, inte en skugga. Skuggan hör till marken
 *  massan STÅR på, inte till varje enskild taknock. */
function groundShadow(g) {
  let bottom = -1, left = g.w, right = -1;
  for (let y = 0; y < g.h; y++)
    for (let x = 0; x < g.w; x++)
      if (at(g, x, y) !== '.') {
        if (y > bottom) bottom = y;
        if (x < left) left = x;
        if (x > right) right = x;
      }
  if (bottom < 0) return;
  for (let x = left + 1; x <= right + 1; x++)
    if (at(g, x, bottom + 1) === '.') set(g, x, bottom + 1, 'G');
}

function build(w, h, draw, wall) {
  const g = newGrid(w, h);
  draw(g);
  if (wall) rampart(g, wall.x, wall.y, wall.w, wall.level, wall.gate);
  outline(g);
  groundShadow(g);
  const runs = [];
  for (let y = 0; y < h; y++) {
    let x = 0;
    while (x < w) {
      const ch = at(g, x, y);
      let n = 1;
      while (x + n < w && at(g, x + n, y) === ch) n++;
      if (ch !== '.') runs.push({ ch, x, y, n });
      x += n;
    }
  }
  return { runs, w, h };
}

// ── De fyra leden ────────────────────────────────────────────────────────
// Ledens innehåll, inte bara storlek, bär berättelsen: ett hus → en by →
// ett säte → ett rike. Måtten står mot hexen, som är 44×38 logiska px.
//
// Varje led byggs i fyra murnivåer (0–3). Sexton siluetter handritade hade
// varit orimligt; genererade är det inte, och muren MÅSTE bakas in i massan för
// att konturpasset ska se stad och mur som ett enda föremål.

// Led 0 — HAMLET. Nygrundad koloni: ett hus och ett skjul. Samma bläck och
// samma tak som palatset — det är samma folk, bara färre av dem.
const HAMLET = wl => build(16, 16, g => {
  cube(g, 4, 2, 8, 8, { door: true });
  cube(g, 1, 7, 4, 4);
}, wl && { x: 1, y: 9,  w: 14, level: wl, gate: 3 });

// Led 1 — TOWN. Fem hus i tre höjdplan kring en gränd. Ingen megaron ännu:
// palatset är inte något man reser, det är vad en plats BLIR.
const TOWN = wl => build(30, 22, g => {
  cube(g, 11, 2, 9, 10, { door: true, window: true });
  cube(g, 4, 6, 8, 7, { door: true });
  cube(g, 19, 5, 7, 8, { door: true });
  cube(g, 7, 11, 7, 5, { door: true });
  cube(g, 16, 12, 8, 4);
}, wl && { x: 0, y: 13, w: 30, level: wl, gate: 3 });

// Led 2 — CITY. Här reser sig MEGARON ur massan och staden byter läsning från
// "hus" till "säte". Husen terrasseras nedåt åt båda håll så salen bär toppen
// ensam — hierarkin är hela poängen, palatskulten är den dominerande ordningen.
const CITY = wl => build(40, 28, g => {
  megaron(g, 14, 5, 13, 12);
  cube(g, 6, 9, 8, 8, { door: true, window: true });
  cube(g, 27, 8, 8, 9, { door: true, window: true });
  cube(g, 3, 14, 7, 5, { door: true });
  cube(g, 10, 16, 8, 4, { door: true });
  cube(g, 23, 16, 8, 4, { door: true });
  cube(g, 33, 13, 4, 5);
}, wl && { x: 0, y: 17, w: 40, level: wl, gate: 4 });

// Led 3 — ANAKTORON. Palatskomplex på terrass, flankerande magasinsflyglar och
// en lägre stad som rinner ned åt båda håll och nästan svämmar över hexen.
// Läckaget över hexkanten är avsiktligt: en palatsstad SKA se ut att inte få
// plats. Murad är det här kartans tyngsta föremål, och det ska den vara.
const PALACE = wl => build(44, 32, g => {
  megaron(g, 15, 4, 15, 14);
  // Magasinsflyglarna bar först var sitt ockraband. De blev markörens
  // STARKASTE färg — på sekundära byggnader, tvärs emot kontrastbudgeten, och
  // de läste som två röda snedstreck. Ockran hör till salen och ingen
  // annanstans: en accent som sitter på allt pekar inte på något.
  cube(g, 7, 8, 9, 10, { door: true, window: true });
  cube(g, 29, 7, 9, 11, { door: true, window: true });
  cube(g, 3, 12, 7, 7, { door: true });
  cube(g, 35, 11, 7, 8, { door: true });
  cube(g, 6, 18, 8, 5, { door: true });
  cube(g, 14, 19, 8, 4, { door: true });
  cube(g, 23, 19, 8, 4, { door: true });
  cube(g, 31, 18, 8, 5, { door: true });
}, wl && { x: 0, y: 20, w: 44, level: wl, gate: 5 });

/** CITY_SPRITES[led][murnivå] — 4×4, genererade en gång vid modulladdning. */
export const CITY_SPRITES = [HAMLET, TOWN, CITY, PALACE]
  .map(make => [0, 1, 2, 3].map(wl => make(wl)));

// Basen (massans nedersta rad) hamnar så här långt under hexcentrum. Staden
// står på sin hex som aktörerna står på sin, men tyngre: massan tillåts växa
// UPPÅT ur hexen, aldrig nedåt — under den ligger namnetiketten, och en stad
// som hänger ned över sitt eget namn gör namnet oläsligt.
export const CITY_BASE_OFFSET = 6;

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
