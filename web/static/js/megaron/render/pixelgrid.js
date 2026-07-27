// ── Pixelrutnät: den akhaiska arkitekturens gemensamma maskineri ─────────
//
// Bruten ur `citysprites.js` när stadsvyn (`render/city.js`) behövde samma
// stämplar. Kartans siluetter och stadsvyns byggnader MÅSTE vara samma värld —
// samma bläck, samma tak, samma ljusriktning — och det är billigare att dela
// verktyget än att hålla två uppsättningar i synk för hand.
//
// Ingenting här vet något om vare sig hexar eller drawers. Det är ett rutnät av
// palettbokstäver, ett par stämplar, och två efterpass.

export const CITY_PALETTE = {
  K: '#1F1A14',  // charcoal — samma bläck som aktörerna. En karta, ett bläck.
  P: '#EFE4CB',  // kalkputs, solbelyst kant. L≈228, kartans ljusaste ton —
                 // kontrastbudgetens mottagare (princip 6).
  p: '#D2C3A0',  // kalkputs, kropp. L≈196
  d: '#A08D71',  // kalkputs, skuggad högerkant och förhallens innervägg. L≈143
  R: '#A88A5E',  // platt jordtak, fascians solbelysta sida. L≈140 — mörkare än
                 // väggen, för att jord är mörkare än kalk, men i SAMMA
                 // lerfamilj. Ett utkast la taket på L 111, ett 85-stegs hopp
                 // från putsen, och staden blev randig: krämvita väggar med
                 // mörkbruna lock. En egeisk lerstad är EN massa av soltorkad
                 // lera där tak och vägg skiljs av valör och kant, inte av två
                 // olika material (Timothys referensark 2026-07-27). Steget är
                 // nu ~55 — nog för form (princip 22), inte nog för ränder.
  r: '#6E5838',  // platt jordtak, skuggat. L≈92 — DJUPARE än en ren
                 // valörinterpolation skulle ge. Det är den här tonen som är
                 // gränsen mellan två tak som möts, och när hela paletten
                 // flyttades in i lerfamiljen blev den kanten för svag: husen
                 // rann ihop till en kulle. Skuggsidan bär separationen.
  M: '#C0A473',  // jordtakets OVANSIDA, solbelyst. L≈163. R/r är takets lodräta
                 // kant sedd framifrån; M/m är den vågräta ytan ovanpå, och den
                 // ser hela himlen. Ger vi dem samma ton blir ett hus med djup
                 // en enda platt klump.
  m: '#836A44',  // jordtakets ovansida, skuggad gavelsida. L≈110
  E: '#C4A578',  // slagen jord — stadens INRE mark, gården innanför ringmuren.
                 // L≈166. Det är den här ytan som gör en stad till en PLATS och
                 // inte till ett föremål: en inhägnad man ser marken i. Vald med
                 // hue åt jord så den skiljs från både slätten (L 138, grön) och
                 // kalkstenen (L 212, kall) — mot olivlunden (157) är valören
                 // svag och separationen bärs av muren, inte av tonen.
  e: '#A88A5E',  // slagen jord, trampad/skuggad. L≈140
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
  w: '#8A6C44',  // timmer, solbelyst
  V: '#141110',  // portgap, fönsterglugg, rökglugg. L≈18, kartans djupaste
                 // mörker — kontrastbudgetens andra mottagare.
  G: '#2A2419',  // markskugga (samma som aktörerna)
  // Stadsvyns tillskott — färger som bara förekommer nära håll, där en
  // verkstad, en last eller en glöd faktiskt går att se.
  F: '#E07020',  // ugnsglöd, härdens eld
  f: '#A33C10',  // glöd, skuggad
  N: '#6E7A46',  // last, säck, packning (samma som aktörernas)
  n: '#8B9459',  // last, solbelyst
  g: '#7C9440',  // gröda, blad
  B: '#C8B24A',  // moget säd, halm
  Z: '#8FA8B4',  // vatten, hamnbassäng
  z: '#5E7C8C',  // vatten, skuggat
  S: '#C9CBD2',  // silver, ljus metall
};

// ── Rutnät ───────────────────────────────────────────────────────────────
export const newGrid = (w, h) => ({ w, h, px: new Array(w * h).fill('.') });

export function set(g, x, y, ch) {
  if (x < 0 || y < 0 || x >= g.w || y >= g.h) return;
  g.px[y * g.w + x] = ch;
}
export const at = (g, x, y) =>
  (x < 0 || y < 0 || x >= g.w || y >= g.h) ? '.' : g.px[y * g.w + x];
export function setIfEmpty(g, x, y, ch) { if (at(g, x, y) === '.') set(g, x, y, ch); }

/** Vågrät löpa. */
export function row(g, x, y, n, ch) { for (let i = 0; i < n; i++) set(g, x + i, y, ch); }
/** Lodrät löpa. */
export function col(g, x, y, n, ch) { for (let i = 0; i < n; i++) set(g, x, y + i, ch); }
/** Fylld rektangel. */
export function rect(g, x, y, w, h, ch) {
  for (let yy = 0; yy < h; yy++) row(g, x, y + yy, w, ch);
}

// ── Kubstämpeln ──────────────────────────────────────────────────────────
// Ett akhaiskt hus i ¾-elevation: mörkt platt jordtak överst, kalkad fasad
// under, mörk dörröppning i botten. Ljuset kommer uppifrån vänster, så vänster
// kolumn är högdager och de två högra är skugga — det är den kanten som skiljer
// två grannhus åt utan bläck.
export function cube(g, x, y, w, h, opts = {}) {
  const roofH = opts.roof ?? 2;
  // ── Djupaxeln ──────────────────────────────────────────────────────────
  // `depth` skjuter en sned volym bakåt från fasaden: positivt = huset viker
  // bort upp-HÖGER, negativt upp-VÄNSTER, 0 = rakt framifrån (och då är
  // stämpeln exakt vad den alltid varit — stadsvyn rör sig inte).
  //
  // Utan den axeln är varje hus en frontal rektangel, och en klunga frontala
  // rektanglar läser som en fasadrad, inte som en stad: "alla hus kan inte vara
  // vända mot spelaren" (Timothy 2026-07-27). Vridningen får inte fuskas med
  // ljusriktningen — den är fix uppifrån vänster — utan måste komma ur
  // GEOMETRIN: takytan är en parallellogram som viker bort, och gaveln som
  // följer med den är husets verkliga sida.
  //
  // Proportionen bär orienteringen och därmed formationen: bred fasad + grunt
  // djup = ett hus vänt mot oss, smal fasad + stort djup = ett hus vänt på
  // tvären. Samma stämpel, två läsningar — princip 20, skillnaden sitter i den
  // stora formen.
  //
  // Lutningen är **2:1** — två pixlar i sidled per pixel i höjdled. Det är inte
  // en smaksak: utkastet sköt volymen 45° (ett steg i sidled per steg uppåt) och
  // då blev ett smalt djupt hus en lång diagonal KIL, en ramp snarare än ett
  // tak. 2:1 är den isometriska konventionen just för att 45° läser som en
  // sluttning i stället för som en låda. Med den kan djupet gå till 5–6 utan att
  // formen havererar, och först DÅ finns det tillräckligt med sida för att ett
  // hus ska kunna stå vänt på tvären (princip 36).
  const d = opts.depth | 0, a = Math.abs(d), s = Math.sign(d);
  for (let k = 1; k <= 2 * a; k++) {
    const dy = Math.ceil(k / 2), dx = s * k;
    // Takets ovansida — den vikande parallellogrammen. Egen, LJUSARE ton än
    // fascian: se M/m i paletten. Med samma ton blev huset en brun klump.
    for (let xx = 0; xx < w; xx++)
      set(g, x + xx + dx, y - dy, xx >= w - 2 ? 'm' : 'M');
    // Gaveln längs den sida volymen viker bort åt. Höger sida ligger i skugga,
    // vänster fångar ljuset — det är därför en vridning åt andra hållet också
    // ger klungan valörvariation, inte bara formvariation.
    const gx = x + (s > 0 ? w - 1 : 0) + dx;
    for (let yy = y - dy; yy < y + roofH - dy; yy++) set(g, gx, yy, 'r');
    for (let yy = y + roofH - dy; yy < y + h - dy; yy++) set(g, gx, yy, s > 0 ? 'd' : 'p');
  }
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

// ── Efterpassen ──────────────────────────────────────────────────────────

/** Princip 18 som maskineri: bläck ritas i de TOMMA pixlarna runt massan, så
 *  konturen alltid är silhuettens gräns mot bakgrunden — aldrig en linje inne
 *  i massan. Diagonalgrannar räknas med, annars läcker bakgrunden in i varje
 *  trappsteg där två kuber möts i höjd. */
export function outline(g) {
  const add = [];
  for (let y = 0; y < g.h; y++) {
    for (let x = 0; x < g.w; x++) {
      if (at(g, x, y) !== '.') continue;
      let touches = false;
      for (let dy = -1; dy <= 1 && !touches; dy++)
        for (let dx = -1; dx <= 1; dx++) {
          const n = at(g, x + dx, y + dy);
          // Slagen jord (E/e) drar ingen kontur. Gården är MARK, och en
          // charcoalkant runt marken gör hela staden till en urklippt bricka
          // som ligger PÅ terrängen i stället för i den (Timothy 2026-07-27).
          // Bara det BYGGDA — murar och hus — får silhuett mot bakgrunden.
          if (n !== '.' && n !== 'K' && n !== 'G' && n !== 'E' && n !== 'e') {
            touches = true; break;
          }
        }
      if (touches) add.push([x, y]);
    }
  }
  for (const [x, y] of add) set(g, x, y, 'K');
}

/** Markskuggan, en per FOT. Varje sammanhängande underkant av minst `minRun`
 *  pixlar får sin egen skugga, förskjuten ett steg åt höger — samma ljusriktning
 *  som aktörernas.
 *
 *  Historik, för den här funktionen har gått ett varv: första utkastet la skugga
 *  under varje KOLUMNS underkant och fick en prickad diagonal ut ur siluetten,
 *  ett kometsvans-spår. Rättningen blev EN gemensam rad under massans lägsta
 *  punkt — rätt så länge massan var ett kompakt block, men på en utspridd stad
 *  ritar den en genomgående SOCKEL under hus som står långt bakom, och sockeln
 *  är precis det som gör markören till en bricka på ett bräde.
 *
 *  Löpkravet är det som skiljer den här versionen från det första utkastet: ett
 *  trappsteg där två kuber möts är 1–2 px och får ingen skugga, ett hus är 5+ och
 *  får en. Skuggan ersätter konturbläcket under foten i stället för att läggas
 *  under det — två mörka rader hade blivit en list, och en list är en sockel.
 *
 *  Fötterna måste läsas av FÖRE `outline()` (konturen fyller ju pixeln under
 *  varje fot) och målas EFTER den. Därför två steg. */
export function footSpans(g, minRun = 4) {
  const spans = [];
  const solid = (x, y) => { const c = at(g, x, y); return c !== '.' && c !== 'G'; };
  for (let y = 0; y < g.h; y++) {
    let x = 0;
    while (x < g.w) {
      if (!solid(x, y) || solid(x, y + 1)) { x++; continue; }
      let n = 0;
      while (solid(x + n, y) && !solid(x + n, y + 1)) n++;
      if (n >= minRun) spans.push([x, y, n]);
      x += n;
    }
  }
  return spans;
}

export function paintFeet(g, spans) {
  for (const [x, y, n] of spans)
    for (let i = 1; i <= n; i++) set(g, x + i, y + 1, 'G');
}

/** Rutnät → horisontella löpor av samma färg: en fillRect per löpa i stället
 *  för en per pixel. Samma teknik som aktörerna och träden. */
export function toRuns(g) {
  const runs = [];
  for (let y = 0; y < g.h; y++) {
    let x = 0;
    while (x < g.w) {
      const ch = at(g, x, y);
      let n = 1;
      while (x + n < g.w && at(g, x + n, y) === ch) n++;
      if (ch !== '.') runs.push({ ch, x, y, n });
      x += n;
    }
  }
  return { runs, w: g.w, h: g.h };
}

/** Ritar en runs-siluett med övre vänstra hörnet i (ox, oy). */
export function blitRuns(ctx, sprite, ox, oy, scale = 1) {
  for (const r of sprite.runs) {
    ctx.fillStyle = CITY_PALETTE[r.ch];
    ctx.fillRect(ox + r.x * scale, oy + r.y * scale, r.n * scale, scale);
  }
}


// ── Inhägnaden ───────────────────────────────────────────────────────────
// En stad är inte en byggnadsklump — den är ett OMRÅDE. Referensbilden
// (`img/f0382384…jpeg`, Timothy 2026-07-27) gör det med en ringmur runt en
// gård av slagen jord, med husen utspridda inne på den och salen bland dem.
// Det är hela skillnaden mellan "miljö" och "overlay": ögat läser en plats man
// kan gå in i, inte ett märke som ligger ovanpå rutan.
//
// Tidigare utkast ritade muren som ett vågrätt BAND framför husen. Ett band
// kan bara skilja fram från bak; det kan inte omsluta något, och därför blev
// staden en bricka hur mycket massan än varierades.

/** Murmur3-finalizerns avalanche, samma familj som terrängens seed (princip 14). */
const hash2 = (x, y) => {
  let h = Math.imul(x, 0x27d4eb2d) ^ Math.imul(y, 0x165667b1);
  h = Math.imul(h ^ (h >>> 15), 0x2545f491);
  return (h ^ (h >>> 13)) >>> 0;
};

/** Vänster/höger kant per rad för en enkel (i praktiken konvex) polygon.
 *  Returnerar `{ y0, rows: [[xl, xr], …] }`. */
export function polyRows(pts) {
  const y0 = Math.min(...pts.map(p => p[1]));
  const y1 = Math.max(...pts.map(p => p[1]));
  const rows = [];
  for (let y = y0; y <= y1; y++) {
    let lo = Infinity, hi = -Infinity;
    for (let i = 0; i < pts.length; i++) {
      const [xa, ya] = pts[i], [xb, yb] = pts[(i + 1) % pts.length];
      if (ya === yb) {
        if (ya !== y) continue;
        lo = Math.min(lo, xa, xb); hi = Math.max(hi, xa, xb);
        continue;
      }
      if (y < Math.min(ya, yb) || y > Math.max(ya, yb)) continue;
      const x = xa + (xb - xa) * (y - ya) / (yb - ya);
      lo = Math.min(lo, x); hi = Math.max(hi, x);
    }
    rows.push(hi < lo ? null : [Math.round(lo), Math.round(hi)]);
  }
  return { y0, rows };
}

/** Gården: fyller polygonen med slagen jord. Stipplet är ett fast hash-mönster
 *  — marken får inte vara en jämn platta (princip 16: regelbundenhet är
 *  artefakten), men den ska heller inte konkurrera med husen.
 *
 *  Hashen måste vara en AVALANCHE-hash, inte en linjärkombination. Utkastets
 *  `(x*73 + y*151) & 7` ritade tydliga diagonala ränder rakt över gården: 73 och
 *  151 är kongruenta med 1 och −1 modulo 8, så villkoret blev i praktiken
 *  "x − y jämnt delbart". Samma fälla som princip 14 beskriver för terrängen. */
export function plateFill(g, poly) {
  poly.rows.forEach((r, i) => {
    if (!r) return;
    const y = poly.y0 + i;
    for (let x = r[0]; x <= r[1]; x++) {
      // Kanten FRANSAS. En skarp polygonkant läser som ett klistermärke; de
      // yttersta pixlarna gallras därför bort med hashen, så marken tunnas ut
      // och löses upp i terrängen i stället för att sluta i en linje.
      const edge = Math.min(x - r[0], r[1] - x, i, poly.rows.length - 1 - i);
      if (edge < 2 && hash2(x, y) % 3 !== 0) continue;
      set(g, x, y, hash2(x, y) % 9 === 0 ? 'e' : 'E');
    }
  });
}

/** Ringmurens pixlar i markplanet, `wt` tjocka, delade i bakre och främre
 *  halva. Muren reses sedan ur dem: varje markpixel extruderas UPPÅT, och
 *  eftersom raderna ritas i växande y skymmer varje närmare bit den bakom sig —
 *  det är den staplingen som ger krönet sin trappade linje längs kanten. */
export function ringPixels(poly, wt = 2) {
  const back = [], front = [];
  const n = poly.rows.length, mid = poly.y0 + (n >> 1);
  poly.rows.forEach((r, i) => {
    if (!r) return;
    const y = poly.y0 + i;
    const edgeRow = i < wt || i >= n - wt;
    for (let x = r[0]; x <= r[1]; x++) {
      if (!edgeRow && x > r[0] + wt - 1 && x < r[1] - wt + 1) continue;
      (y < mid ? back : front).push([x, y]);
    }
  });
  return { back, front };
}

/** Reser muren ur markpixlarna. Höjden är murnivåns, inte en estetisk skala:
 *  livet växer med nivån och först på nivå 2 kommer tinnar och torn. */
export function raiseWall(g, pixels, h) {
  for (const [x, y] of pixels) {
    for (let k = 0; k < h; k++) {
      // Ljus krönlist, mellanton i livet, mörk fot: tre valörer, annars läser
      // muren som en platt yta. Kyklopiska fogar deterministiskt utlagda — ett
      // regelbundet rutmönster hade lästs som tegel, och skillnaden syns även
      // vid 5 px.
      const yy = y - k;
      let ch = k === h - 1 ? 'T' : (k === 0 ? 't' : 'q');
      if (k > 0 && k < h - 1 && ((x * 7 + yy * 23) % 11) < 2) ch = 't';
      set(g, x, yy, ch);
    }
  }
}
