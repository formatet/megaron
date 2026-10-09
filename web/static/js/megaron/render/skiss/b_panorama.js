// ── Skiss B: PANORAMA — staden sedd från vägen ────────────────────────────
//
// Man står på slätten och ser upp mot borgen: himmel, horisont, borgklippan med
// sin kyklopiska mur, megaron på krönet, och nedanför den lägre staden med
// verkstäder, fält och vägen in. Mykene-modellen: muren omger BORGEN, den lägre
// staden ligger utanför — så verkstäderna syns i stället för att skymmas.

import { cube, rect, row, col } from '../pixelgrid.js';
import { stampBuilding, stampUnderConstruction, buildingWidth } from '../citybuildings.js';
import {
  W, H, newGrid, set, at, hash, fillGround, inked, over, blit, person, smoke, houseCount, OUTSIDE,
} from './common.js';

const HORIZON = 52, FOOT = 86;

export function render(ctx, scene) {
  const { sett, terrain, coastal } = scene;
  const pop = sett.population, walls = sett.walls;
  const t = Math.max(0, Math.min(1, Math.log10(pop / 100) / Math.log10(280)));
  const g = newGrid(W, H);
  const cx = coastal ? 88 : 105;

  // ── Himmel i stegade band, fjärran berg ──
  for (let y = 0; y < HORIZON; y++) for (let x = 0; x < W; x++)
    set(g, x, y, y < 16 ? '`' : y < 34 ? '~' : '9');
  for (let i = 0; i < 4; i++) {           // ett par molnstrimmor
    const y = 8 + i * 9, x0 = hash(i, 1, 4) % 150, n = 18 + hash(i, 2, 4) % 30;
    row(g, x0, y, n, '$'); row(g, x0 + 4, y + 1, n - 8, '9');
  }
  for (let x = 0; x < W; x++) {
    const top = HORIZON - 7 + Math.round(4 * Math.sin(x / 23 + 1) + 2 * Math.sin(x / 8));
    for (let y = top; y < HORIZON + 2; y++) set(g, x, y, y < top + 1 ? ':' : '^');
  }
  fillGround(g, 0, HORIZON + 2, W, H, terrain, 9);
  if (coastal) {
    for (let y = HORIZON - 1; y < H; y++) {
      const edge = 140 + Math.round((y - HORIZON) * 0.9 + 3 * Math.sin(y / 5));
      for (let x = edge; x < W; x++) {
        const d = x - edge;
        let ch = d < 1 ? '3' : d < 3 ? '2' : y < HORIZON + 6 ? '0' : '1';
        if (ch !== '3' && ch !== '2' && hash(x, y, 3) % 29 === 0) ch = '3';
        set(g, x, y, ch);
      }
    }
  }

  // ── Fälten i förgrunden, horisontella fåror som vidgas mot betraktaren ──
  const farmLvl = (sett.buildings.find(b => b.type === 'farm') || {}).level || 0;
  const fieldsX = [[4, 46], [150, 200], [48, 80]];
  for (let i = 0; i < Math.max(1, farmLvl); i++) {
    const [x0, x1] = fieldsX[i];
    for (let y = FOOT + 4; y < H; y++) {
      const band = Math.floor(Math.sqrt(y - FOOT) * 2);
      for (let x = x0; x < x1; x++) {
        if (coastal && at(g, x, y) >= '0' && at(g, x, y) <= '3') continue;
        set(g, x, y, band % 2 ? '=' : (hash(band, i, 7) % 3 ? '-' : '_'));
      }
    }
  }

  // ── Vägen in, i perspektiv ──
  for (let y = FOOT; y < H; y++) {
    const hw = 1 + Math.round((y - FOOT) / 6);
    for (let k = -hw; k <= hw; k++) set(g, cx + k + Math.round(Math.sin(y / 9) * 2), y, Math.abs(k) === hw ? 'e' : 'E');
  }

  // ── Borgklippan ──
  const hasCitadel = pop >= 1000;
  const hillH = hasCitadel ? Math.round(14 + 20 * t) : 4;
  const hw = hasCitadel ? Math.round(30 + 34 * t) : 22;
  const hillTop = FOOT - hillH;
  const ridge = x => {
    const u = Math.abs(x - cx) / hw;
    if (u >= 1) return null;
    const shoulder = u < 0.4 ? 0 : ((u - 0.4) / 0.6) ** 1.6;
    return Math.round(hillTop + shoulder * hillH + (hash(x, 1, 6) % 2));
  };
  inked(g, gg => {
    for (let x = cx - hw; x <= cx + hw; x++) {
      const top = ridge(x); if (top === null) continue;
      for (let y = top; y <= FOOT; y++) {
        const shade = x > cx + hw * 0.35;
        const lit = x < cx - hw * 0.35 && y - top < 3;
        let ch = lit ? 'P' : shade ? 'q' : 'T';
        if (hash(x, y, 2) % 11 === 0) ch = 't';
        // Terrassmurar: vågräta stenband var sjätte rad.
        if (hasCitadel && (FOOT - y) % 7 === 0 && y > top + 2) ch = 't';
        set(gg, x, y, ch);
      }
    }
  });

  // ── Husen på terrasserna, ovanifrån och ned ──
  const smokes = [];
  if (hasCitadel) {
    const mw = 16 + Math.round(12 * t), mh = 10 + Math.round(4 * t);
    const wallTop = FOOT - [0, 5, 8, 11][walls];
    // Terrasshus: rad för rad, smalare upptill.
    const rows = [];
    for (let b = hillTop + 9; b < wallTop; b += 6) rows.push(b);
    let seed = 1;
    rows.forEach((b) => {
      let x = cx - hw;
      while (x < cx + hw) {
        const w = 5 + hash(seed, 1, 1) % 4, h = 5 + hash(seed, 2, 1) % 3;
        const top = ridge(x), top2 = ridge(x + w);
        seed++;
        if (top === null || top2 === null || top > b - h + 2 || top2 > b - h + 2) { x += 3; continue; }
        if (Math.abs(x + w / 2 - cx) < mw / 2 + 2 && b < hillTop + 14) { x += w; continue; }
        if (hash(seed, 3, 1) % 10 < 7 - Math.round(4 * (1 - t))) {
          const d = [0, 0, 1, -1][hash(seed, 4, 1) % 4], s = seed;
          inked(g, gg => cube(gg, x, b - h, w, h, { depth: d, door: hash(s, 5, 1) % 2 === 0, parapet: d !== 0 }));
          if (hash(seed, 6, 1) % 9 === 0) smokes.push([x + 1, b - h - 2]);
        }
        x += w + (hash(seed, 7, 1) % 2);
      }
    });
    // Megaron på krönet med förhall och pelare.
    inked(g, gg => {
      const x = cx - (mw >> 1), y = hillTop - mh + 2;
      cube(gg, x, y, mw, mh, { depth: 1, parapet: true, band: true, roof: 2 });
      for (let i = 1; i < 4; i++) col(gg, x + Math.round(i * mw / 4), y + 4, mh - 4, i === 2 ? 'V' : 'P');
      row(gg, x - 1, y + mh, mw + 2, 'T');
      if (t > 0.6) {
        cube(gg, x - 10, y + 4, 10, mh - 4, { depth: -1, window: true });
        cube(gg, x + mw, y + 3, 11, mh - 3, { depth: 1, window: true });
      }
    });
  }

  // ── Borgmuren framför klippan ──
  if (walls && hasCitadel) {
    const wh = [0, 5, 8, 11][walls];
    const x0 = cx - hw + 4, x1 = cx + hw - 4;
    inked(g, gg => {
      for (let x = x0; x <= x1; x++) for (let y = FOOT - wh; y <= FOOT; y++) {
        // Kyklopiska block: oregelbundna stenar, inte tegel.
        const bx = (x + ((y >> 2) % 2) * 3) >> 2 | 0, by = y >> 2;
        let ch = (hash(bx, by, 4) % 3 === 0) ? 'q' : 'T';
        if ((x + ((y >> 2) % 2) * 3) % 4 === 0 || y % 4 === 0) ch = 't';
        if (y === FOOT - wh) ch = 'P';
        set(gg, x, y, ch);
      }
      if (walls >= 2) for (let x = x0; x <= x1; x += 3) set(gg, x, FOOT - wh - 1, 'T');
      const towers = walls >= 3 ? [x0 - 2, cx - 11, cx + 6, x1 - 4] : walls >= 2 ? [cx - 11, cx + 6] : [];
      for (const tx of towers) {
        rect(gg, tx, FOOT - wh - 4, 6, wh + 5, 'T');
        col(gg, tx + 5, FOOT - wh - 4, wh + 5, 'q'); col(gg, tx, FOOT - wh - 4, wh + 5, 'P');
        set(gg, tx + 1, FOOT - wh - 5, 'T'); set(gg, tx + 4, FOOT - wh - 5, 'T');
      }
      // Lejonporten: tung överliggare, mörk öppning.
      rect(gg, cx - 2, FOOT - wh + 3, 5, wh - 2, 'V');
      row(gg, cx - 4, FOOT - wh + 2, 9, 'P');
      row(gg, cx - 4, FOOT - wh + 1, 9, 'T');
    });
  }

  // ── Den lägre staden: verkstäder och hus i två led framför ──
  const lower = [
    ...sett.buildings.filter(b => !OUTSIDE.has(b.type) && b.type !== 'harbour').map(b => ({ ...b, phase: 1 })),
    ...sett.build_queue.map(b => ({ ...b })),
  ];
  const outside = sett.buildings.filter(b => OUTSIDE.has(b.type) && b.type !== 'farm');
  const rowsBase = [FOOT + 13, FOOT + 27, FOOT + 41];
  const slots = rowsBase.map(() => []);
  const span = coastal ? [6, 136] : [6, 204];
  // Byggnader varannan sida om vägen, främre ledet först.
  const queue = [...lower, ...outside];
  let ri = 0, cursorL = rowsBase.map(() => cx - 9), cursorR = rowsBase.map(() => cx + 9), side = 0;
  for (const b of queue) {
    const w = buildingWidth(b.type, b.level);
    for (let tries = 0; tries < 6; tries++) {
      const r = (ri + tries) % rowsBase.length;
      if (side === 0 && cursorL[r] - w >= span[0]) { slots[r].push({ x: cursorL[r] - w, b }); cursorL[r] -= w + 3; break; }
      if (side === 1 && cursorR[r] + w <= span[1]) { slots[r].push({ x: cursorR[r], b }); cursorR[r] += w + 3; break; }
      side ^= 1;
    }
    side ^= 1; if (side === 0) ri++;
  }
  // Hus fyller ut kring verkstäderna; antalet följer befolkningen.
  const nHouses = hasCitadel ? Math.round(houseCount(pop) * 1.4) : 5;
  let seed = 400, placed = 0;
  const homes = rowsBase.map(() => []);
  for (let tries = 0; tries < 800 && placed < nHouses; tries++, seed++) {
    const r = hasCitadel ? hash(seed, 1, 2) % 2 : 0;
    const w = 6 + hash(seed, 2, 2) % 4;
    const x = cx - 50 - Math.round(30 * t) + hash(seed, 3, 2) % (100 + Math.round(60 * t));
    if (x < span[0] || x + w > span[1] || Math.abs(x + w / 2 - cx) < 6 + w / 2) continue;
    const base = hasCitadel ? rowsBase[r] - 7 : FOOT + 4;
    const all = [...slots[r].filter(s => s.b).map(s => [s.x, buildingWidth(s.b.type, s.b.level)]), ...homes[r]];
    if (all.some(([a, ww]) => x < a + ww + 1 && a < x + w + 1)) continue;
    homes[r].push([x, w]); placed++;
    const h = 5 + hash(seed, 4, 2) % 3, d = [0, 1, -1][hash(seed, 5, 2) % 3], s = seed;
    slots[r].push({ x, house: { w, h, d, base, s } });
  }
  // Bakre led först, så främre skymmer.
  for (let r = 0; r < rowsBase.length; r++) {
    const list = slots[r].slice().sort((a, b) => (a.house ? a.house.base : rowsBase[r]) - (b.house ? b.house.base : rowsBase[r]));
    for (const s of list) {
      if (s.house) {
        const { w, h, d, base } = s.house;
        inked(g, gg => cube(gg, s.x, base - h, w, h, { depth: d, door: true, parapet: d !== 0 }));
        if (hash(s.house.s, 9, 2) % 6 === 0) smokes.push([s.x + 1, base - h - 2]);
      } else {
        const b = s.b;
        inked(g, gg => (b.phase === undefined || b.phase >= 1
          ? stampBuilding(gg, b.type, s.x, rowsBase[r], b.level)
          : stampUnderConstruction(gg, b.type, s.x, rowsBase[r], b.level, b.phase)));
        if (b.type === 'foundry' && (b.phase ?? 1) >= 1) smokes.push([s.x + 10, rowsBase[r] - 14]);
      }
    }
  }

  // ── Hamnen ──
  if (coastal) {
    const hl = (sett.buildings.find(b => b.type === 'harbour') || {}).level || 0;
    if (hl) inked(g, gg => stampBuilding(gg, 'harbour', 150, FOOT + 4, hl));
    for (let i = 0; i < (hl ? hl + 1 : 1); i++) {
      const sx = 168 + i * 12, sy = HORIZON + 10 + i * 14;
      inked(g, gg => {
        const L = 12 + i * 2;
        row(gg, sx + 1, sy, L - 2, '<'); row(gg, sx, sy - 1, L, '<'); set(gg, sx - 1, sy - 2, '<'); set(gg, sx + L, sy - 2, '<');
        col(gg, sx + (L >> 1), sy - 10, 8, 'W'); rect(gg, sx + (L >> 1) - 3, sy - 9, 7, 5, '>');
      });
    }
  }

  // ── Livet ──
  const nPeople = Math.min(40, 2 + Math.round(pop / 700));
  over(g, gg => {
    let n = 0;
    for (let i = 0; i < 3000 && n < nPeople; i++) {
      const y = FOOT + 2 + hash(i, 1, 5) % (H - FOOT - 3), x = hash(i, 2, 5) % W;
      if (at(g, x, y) !== 'E' || at(g, x, y - 2) !== 'E') continue;
      person(gg, x, y, ['$', '$', 'O', 'z', 'w', 'N'][hash(i, 3, 5) % 6]); n++;
    }
    for (let i = 0; i < 12 && n < nPeople + 6; i++) {        // folk på åkrarna
      const x = 6 + hash(i, 4, 5) % 190, y = FOOT + 8 + hash(i, 5, 5) % 30;
      if ('=-_'.includes(at(g, x, y))) { person(gg, x, y, '$'); n++; }
    }
    smokes.forEach(([x, y], i) => smoke(gg, x, y, 10, i));
  });

  blit(ctx, g);
}
