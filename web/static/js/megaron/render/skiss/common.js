// ── Stadsvyskisser: gemensamt maskineri ───────────────────────────────────
//
// Tre skisser av stadsvyn (A diorama · B panorama · C plan) som Timothy ska
// tycka om. De delar palett med kartan (CITY_PALETTE) och utökar den med mark,
// hav, himmel, folk och rök. Ingenting här är produktionskod ännu.

import { CITY_PALETTE, newGrid, set, at, outline } from '../pixelgrid.js';

export const W = 210, H = 128, S = 2;

export const PAL = {
  ...CITY_PALETTE,
  '0': '#2B5160',  // djuphav
  '1': '#3B6E74',  // kusthav
  '2': '#5F9089',  // grunt vatten
  '3': '#C9D6C2',  // skum
  '4': '#7A8642',  // gräs
  '5': '#636F37',  // gräs, skugga
  '6': '#959A5C',  // gräs, ljus
  '7': '#B89E66',  // torr ockra
  '8': '#957D4C',  // ockra, skugga
  '9': '#D3C8A6',  // himmel nära horisonten
  '~': '#C2BBA0',  // himmel, högre upp
  '`': '#B3AE98',  // himmel, zenit
  '^': '#8E8A70',  // fjärran berg
  ':': '#A39C80',  // fjärran berg, solsida
  '#': '#3E4B30',  // trädkrona, skugga
  '+': '#56653E',  // trädkrona
  '*': '#76844E',  // trädkrona, ljus
  '=': '#8A7350',  // åkerfåra
  '-': '#BCA765',  // mogen åker
  '_': '#A08C55',  // åker, mellan
  '@': '#B07A52',  // hud
  '$': '#E6DCC4',  // vit tunika
  '%': '#8A877E',  // rök
  '&': '#B2AEA2',  // rök, ljus
  '<': '#4F3A26',  // skeppsskrov
  '>': '#E9E0C8',  // segel
  'X': '#B81E1E',  // blodröd — hot, fiende (signalnivån; en betydelse)
  'x': '#6E1212',  // blodröd, skuggad
  'Y': '#E8C040',  // aktivt bygge — signalgul markör
};

// Murmur-finalizer — samma familj som terrängens seed. Deterministisk: samma
// stad ser alltid likadan ut.
export function hash(x, y, s = 0) {
  let h = Math.imul(x | 0, 0x27d4eb2d) ^ Math.imul(y | 0, 0x165667b1) ^ Math.imul(s | 0, 0x3c6ef372);
  h = Math.imul(h ^ (h >>> 15), 0x2545f491);
  return (h ^ (h >>> 13)) >>> 0;
}
export const rnd = (x, y, s) => (hash(x, y, s) % 10000) / 10000;

export const GROUND = {
  plains: ['4', '5', '6'], river_valley: ['4', '5', '6'], river_delta: ['6', '4', '-'],
  hills: ['7', '8', '6'], scrub_maquis: ['6', '5', '7'], semi_desert: ['7', '8', '9'],
  forest_olive_grove: ['6', '5', '4'], forest_cedar: ['5', '#', '4'],
  mountain_limestone: ['T', 'q', 'P'], mountain_red: ['7', '8', 'E'],
};

export function fillGround(g, x0, y0, x1, y1, terrain, seed = 1) {
  const [b, d, l] = GROUND[terrain] || GROUND.plains;
  for (let y = y0; y < y1; y++)
    for (let x = x0; x < x1; x++) {
      const h = hash(x, y, seed) % 23;
      set(g, x, y, h < 2 ? d : h < 4 ? l : b);
    }
}

/** Ritar `fn` i ett eget rutnät, ger det bläckkontur och lägger det över `g`.
 *  Varje föremål får sin egen kontur — utan det rinner grannhus ihop. */
export function inked(g, fn) {
  const t = newGrid(g.w, g.h);
  fn(t);
  outline(t);
  for (let i = 0; i < t.px.length; i++) if (t.px[i] !== '.') g.px[i] = t.px[i];
}

/** Ett överlägg utan kontur (folk, rök, skum). */
export function over(g, fn) {
  const t = newGrid(g.w, g.h);
  fn(t);
  for (let i = 0; i < t.px.length; i++) if (t.px[i] !== '.') g.px[i] = t.px[i];
}

export function blit(ctx, g, s = S) {
  for (let y = 0; y < g.h; y++) {
    let x = 0;
    while (x < g.w) {
      const ch = at(g, x, y);
      let n = 1;
      while (x + n < g.w && at(g, x + n, y) === ch) n++;
      if (ch !== '.') { ctx.fillStyle = PAL[ch] || '#FF00FF'; ctx.fillRect(x * s, y * s, n * s, s); }
      x += n;
    }
  }
}

/** En liten människa: 1 px huvud, 2 px kropp. Tunikan bär sysslan. */
export function person(g, x, y, tunic = '$') {
  set(g, x, y - 2, '@');
  set(g, x, y - 1, tunic);
  set(g, x, y, tunic === '$' ? 'd' : tunic);
}

/** Rökpelare — glesnar uppåt, driver åt höger (vinden från väster). */
export function smoke(g, x, y, n = 8, seed = 0) {
  for (let i = 0; i < n; i++) {
    const xx = x + Math.floor(i / 3) + (hash(i, seed, 7) % 2);
    if (hash(i, seed, 9) % 5 === 0 && i > 3) continue;
    set(g, xx, y - i, i < 3 ? '%' : '&');
  }
}

/** Antal hus ur befolkningen: log-skala, så 100 → en handfull och 28 000 → en stad. */
export const houseCount = pop => Math.max(3, Math.round(3 + 9 * Math.log10(Math.max(100, pop) / 100)));

export const SCENES = [
  { title: 'Nygrundad — pop 101, inga byggnader, ingen mur, slätt',
    terrain: 'plains', coastal: false,
    sett: { population: 101, walls: 0, buildings: [], build_queue: [] } },
  { title: 'Växande — pop 3 200, mur 1, gård + marknad + kasern, gjuteri halvbyggt, kullar',
    terrain: 'hills', coastal: false,
    sett: { population: 3200, walls: 1,
      buildings: [{ type: 'farm', level: 1 }, { type: 'market', level: 1 }, { type: 'barracks', level: 1 }],
      build_queue: [{ type: 'foundry', level: 1, phase: 0.5 }] } },
  { title: 'Kuststad — pop 9 000, mur 2, hamn, marknad, tempel, olivpress, gård',
    terrain: 'plains', coastal: true,
    sett: { population: 9000, walls: 2,
      buildings: [{ type: 'harbour', level: 2 }, { type: 'market', level: 2 }, { type: 'temple', level: 1 },
        { type: 'olive_press', level: 1 }, { type: 'farm', level: 2 }],
      build_queue: [] } },
  { title: 'Palatsstad — pop 28 000, mur 3, varje byggnadstyp, kust',
    terrain: 'river_valley', coastal: true,
    sett: { population: 28000, walls: 3,
      buildings: ['farm', 'olive_press', 'winery', 'market', 'harbour', 'lumbermill', 'stonequarry',
        'mine', 'silver_mine', 'foundry', 'barracks', 'stable', 'temple'].map((t, i) => ({ type: t, level: 1 + (i % 3) })),
      build_queue: [] } },
];

export const has = (sett, t) => sett.buildings.some(b => b.type === t);
export const level = (sett, t) => (sett.buildings.find(b => b.type === t) || {}).level || 0;
// Byggnader som hör hemma UTANFÖR staden — marken, inte gatan.
export const OUTSIDE = new Set(['farm', 'mine', 'silver_mine', 'stonequarry', 'lumbermill']);

export { newGrid, set, at };
