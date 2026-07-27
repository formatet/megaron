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
export function groundShadow(g) {
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
