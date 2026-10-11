// ── Stadsikonerna (Timothy 2026-10-10) ───────────────────────────────────
//
// Timothy om de två stadsspråken: provets städer "ser ut som ett spel" och
// "som arketypen för stadsikoner"; de inhägnade ¾-massorna "ser ut som ett
// försök till en teckning". Det här är provets generator
// (~/megaron-grafikprov/generera.py, city_grid) överförd till spelet och
// uppskalad till spelets hex (44 px). Bakgrund och beslut:
// myltavault/megaron_markrastret_20261010.md §8.
//
// Läsordningen är grafikreglernas regel 5: stad → storlek → mur → ägare →
// status. Därför:
//   - CITADELLET överst bär KULTUREN i silhuetten (megaron, torn, pylon).
//   - HUSEN i terrasser bär STORLEKEN (fler och bredare rader i det större ledet).
//   - MUREN ligger i förgrunden och är det tydligaste på ikonen; fyra nivåer
//     med olika höjd, port, tinnar och torn — aldrig bara tjocklek.
//   - VIMPELN bär ägaren med FORM + färg: egen fyrkantig, annans triangulär.
//   - Status (garnison, aktivitet, belägring, brand) är eget lager ovanpå
//     (regel 10) och ritas i map.js.
//
// Bilden byggs som text — en bokstav per pixel, samma format som
// actorsprites — ur stämplar, och cachas per (led, mur, kultur, frö, kust, ruin).

export const CITY_ICON_PALETTE = {
  K: '#1C1612', L: '#EADEC0', S: '#B8A686', R: '#B25434', r: '#8C3E28',
  D: '#523E2C', W: '#704A2A', G: '#ACA492', g: '#7C766A', F: '#78944A',
  A: '#C49A3C', Q: '#3A708C', q: '#6096A8',
};

const H2 = (x, y, s) => {
  let n = (Math.imul(x | 0, 374761393) + Math.imul(y | 0, 668265263) + Math.imul(s | 0, 1442695041)) >>> 0;
  n = Math.imul(n ^ (n >>> 13), 1274126177) >>> 0;
  return ((n ^ (n >>> 16)) & 0xffff) / 65535;
};

const HOUSE   = ['KKKKKK', 'KLLLSK', 'KLLLSK', 'KLDLSK'];
const HOUSE_R = ['KKKKKK', 'KRRRrK', 'KLLLSK', 'KLDLSK'];
const HOUSE_W = ['KKKKKKK', 'KLLLLSK', 'KLLDLSK', 'KLLDLSK'];
const HOUSE_O = ['KKKKK', 'KSSgK', 'KSSgK', 'KSDgK'];            // sten/lera, mörkare
const HOUSE_T = ['KKKK.', 'KLSK.', 'KLSKK', 'KLSSK', 'KDSSK'];   // tornhus, en rad högre

// Citadellet per kultur. Okända kulturer faller tillbaka på megaronet.
const CITADEL = {
  megaron: [
    '....KK....', '...KRRK...', '..KRRRrK..', '.KRRRRRrK.', 'KKKKKKKKKK',
    'KLLKLLKLSK', 'KLLKDDKLSK', 'KLLKDDKLSK', 'KKKKKKKKKK',
  ],
  torn: [
    'KK.K....K.KK', 'KKKK....KKKK', 'KLSK....KLSK', 'KLSKKKKKKLSK',
    'KLLLLLLLLLSK', 'KLLLLDDLLLSK', 'KLLLLDDLLLSK', 'KKKKKKKKKKKK',
  ],
  pylon: [
    'KKKKK..KKKKK', 'KLLLK..KLLSK', 'KLLLK..KLLSK', 'KLLLKKKKLLSK',
    'KLLLKAAKLLSK', 'KLLLKDDKLLSK', 'KLLLKDDKLLSK', 'KKKKKKKKKKKK',
  ],
};
const CULTURE_CITADEL = {
  akhaier: 'megaron', minoan: 'megaron', thrakes: 'megaron',
  hatti: 'torn', khemetiu: 'pylon', knaani: 'pylon',
};

function stamp(g, rows, x, y) {
  for (let dy = 0; dy < rows.length; dy++) {
    for (let dx = 0; dx < rows[dy].length; dx++) {
      const ch = rows[dy][dx];
      if (ch === '.') continue;
      const yy = y + dy, xx = x + dx;
      if (yy >= 0 && yy < g.length && xx >= 0 && xx < g[0].length) g[yy][xx] = ch;
    }
  }
}

/** Stadens bild som textrader. size 0 = liten (under 800 inv.), 1 = stor.
 *  coastal: false, 'left' eller 'right' — åt vilket håll kajen går ut mot havet. */
export function cityIconGrid(size, wall, culture, seed, coastal = false, ruin = false) {
  const W = size ? 43 : 31, H = size ? 31 : 25;
  const g = Array.from({ length: H }, () => Array(W).fill('.'));
  const cit = CITADEL[CULTURE_CITADEL[culture] || 'megaron'];
  const cx = W >> 1;
  // Husen i terrasser bakifrån och fram, bredare för varje rad.
  const rowsY = [];
  for (let y = cit.length - 2; y + 4 <= H - 4; y += 3) rowsY.push(y);
  // Bredd och mitt varierar per rad, ytterhusen faller ibland bort och ett
  // och annat hus är högre — staden ska se VUXEN ut, inte staplad.
  rowsY.forEach((yy, i) => {
    const span = (size ? 8 + i * 4 : 6 + i * 3) + Math.floor(H2(i, seed, 5) * 5) - 2;
    const mid = cx + Math.floor(H2(i, seed, 6) * 5) - 2;
    let x = Math.max(1, mid - span) + Math.floor(H2(i, seed, 3) * 3);
    const x1 = Math.min(W - 7, mid + span - 3);
    let first = true;
    while (x <= x1) {
      const r = H2(x, yy, seed);
      const st = r < 0.28 ? HOUSE_R : r < 0.42 ? HOUSE_O : r > 0.9 ? HOUSE_T : r > 0.75 ? HOUSE_W : HOUSE;
      const last = x + st[0].length > x1;
      const skip = (first || last) && H2(x, yy, seed + 4) < 0.3;
      if (!skip) stamp(g, st, x, st === HOUSE_T ? yy - 1 : yy);
      x += st[0].length - 1 + (H2(x, yy, seed + 9) > 0.65 ? 1 : 0);
      first = false;
    }
  });
  stamp(g, cit, cx - (cit[0].length >> 1), 0);

  // Muren i förgrunden. Nivå 0: ingen. 1: låg stensättning. 2: mur med port
  // och sidomurar. 3: högre cyklopisk mur med tinnar, hörntorn och lejonport.
  const base = H - 1;
  if (wall >= 1) {
    for (let x = 0; x < W; x++) { g[base][x] = 'K'; g[base - 1][x] = x % 2 ? 'G' : 'g'; }
  }
  if (wall >= 2) {
    for (let y = base - 4; y <= base - 2; y++) {
      for (let x = 0; x < W; x++) g[y][x] = ((x + (y % 2) * 2) >> 1) % 2 ? 'G' : 'S';
    }
    for (let x = 0; x < W; x++) g[base - 5][x] = 'K';
    for (let y = base - 8; y <= base; y++) { g[y][0] = 'K'; g[y][W - 1] = 'K'; }
    for (let y = base - 4; y <= base - 1; y++) { g[y][cx - 2] = 'K'; g[y][cx - 1] = 'D'; g[y][cx] = 'D'; g[y][cx + 1] = 'D'; g[y][cx + 2] = 'K'; }
  }
  if (wall >= 3) {
    for (let y = base - 7; y <= base - 6; y++) {
      for (let x = 0; x < W; x++) g[y][x] = y === base - 7 ? (x % 3 === 2 ? '.' : 'K') : (x % 2 ? 'G' : 'S');
    }
    g[base - 5] = g[base - 5].map((c, x) => (x % 2 ? 'G' : 'S'));
    const T = ['KKKKK', 'KGSGK', 'KSGSK', 'KGSGK', 'KSGSK', 'KGSGK', 'KSGSK', 'KGSGK', 'KSGSK', 'KGSGK', 'KKKKK'];
    stamp(g, T, 0, base - 10);
    stamp(g, T, W - 5, base - 10);
    stamp(g, ['..K..', '.KAK.', 'KAKAK'], cx - 2, base - 7);   // lejonportens avlastningstriangel
  }
  if (coastal) {   // kaj och brygga åt havets sida
    const left = coastal === 'left';
    g.push(Array(W).fill('.'), Array(W).fill('.'));
    for (let x = left ? 0 : cx; x <= (left ? cx : W - 1); x++) { g[H][x] = x % 2 ? 'G' : 'S'; g[H + 1][x] = 'K'; }
    if (left) stamp(g, ['..W', '..W', '.WK', 'KKK'], 0, H - 2);
    else stamp(g, ['W..', 'W..', 'KW.', 'KKK'], W - 3, H - 2);
  }
  if (ruin) {
    for (let y = 0; y < g.length; y++) {
      for (let x = 0; x < W; x++) {
        const ch = g[y][x], r = H2(x, y, seed + 77);
        if ('RrA'.includes(ch)) g[y][x] = r < 0.3 ? 'g' : '.';
        else if (ch === 'L' || ch === 'S') g[y][x] = r < 0.45 ? 'G' : (r > 0.92 ? 'F' : '.');
        else if (ch === 'D') g[y][x] = '.';
        else if (ch === 'K' && r < 0.35) g[y][x] = '.';
      }
    }
  }
  return g.map(r => r.join(''));
}

const cache = new Map();
/** {w, h, runs} för en stad. Cachas — samma stad ritas varje frame. */
export function cityIcon(size, wall, culture, seed, coastal, ruin) {
  const key = `${size}|${wall}|${culture}|${seed}|${coastal || 0}|${ruin ? 1 : 0}`;
  let ic = cache.get(key);
  if (ic) return ic;
  const rows = cityIconGrid(size, wall, culture, seed, coastal, ruin);
  const runs = [];
  rows.forEach((row, y) => {
    let x = 0;
    while (x < row.length) {
      const ch = row[x];
      let e = x + 1;
      while (e < row.length && row[e] === ch) e++;
      if (ch !== '.') runs.push({ x, y, n: e - x, ch });
      x = e;
    }
  });
  ic = { w: rows[0].length, h: rows.length, runs };
  cache.set(key, ic);
  return ic;
}

/** Ritar ikonen med foten FOOT px under hexmitten. Returnerar ikon + origo. */
export const CITY_ICON_FOOT = 14;
export function drawCityIcon(ctx, ic, cx, cy) {
  const ox = Math.round(cx) - (ic.w >> 1), oy = Math.round(cy) + CITY_ICON_FOOT - ic.h;
  // Ikonen ritas EN gång till en egen duk och kopieras sedan; run för run
  // varje frame kostade 7,6 ms i världsriggen.
  if (!ic.canvas && typeof document !== 'undefined') {
    ic.canvas = document.createElement('canvas');
    ic.canvas.width = ic.w; ic.canvas.height = ic.h;
    const c = ic.canvas.getContext('2d');
    for (const r of ic.runs) { c.fillStyle = CITY_ICON_PALETTE[r.ch]; c.fillRect(r.x, r.y, r.n, 1); }
  }
  if (ic.canvas) {
    const prev = ctx.imageSmoothingEnabled;
    ctx.imageSmoothingEnabled = false;
    ctx.drawImage(ic.canvas, ox, oy);
    ctx.imageSmoothingEnabled = prev;
  } else {
    for (const r of ic.runs) { ctx.fillStyle = CITY_ICON_PALETTE[r.ch]; ctx.fillRect(ox + r.x, oy + r.y, r.n, 1); }
  }
  return { ox, oy };
}

// Statusstämplar (provets ACTIVITY): hammare = bygger, spjut = tränar,
// prickad ram = gör ingenting. Form, inte bara färg.
export const ACTIVITY_ICONS = {
  build: ['KKKKK', 'KWWWK', 'KKKKK', '.KWK.', '.KWK.', '.KKK.'],
  train: ['....K', '...KA', '..KW.', '.KW..', 'KW...'],
  idle:  ['K.K.K', '.....', 'K...K', '.....', 'K.K.K'],
};
export function drawStamp(ctx, rows, x, y, pal = CITY_ICON_PALETTE) {
  rows.forEach((row, dy) => {
    for (let dx = 0; dx < row.length; dx++) {
      if (row[dx] === '.') continue;
      ctx.fillStyle = pal[row[dx]];
      ctx.fillRect(x + dx, y + dy, 1, 1);
    }
  });
}
