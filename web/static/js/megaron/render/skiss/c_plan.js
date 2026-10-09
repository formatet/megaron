// ── Skiss C: PLAN — kartan inzoomad, rakt uppifrån ────────────────────────
//
// Samma blick som kartan, bara närmare: man ser staden som en stadsplan —
// takens lappverk, gator, ringmuren med torn, borgen med megarons härd, torget,
// och utanför fält, gruvor och hamn. Byggnaderna läses på sin PLAN (tempel =
// pelarkrans, stall = hage med hästar, gjuteri = glödande ugn).

import { rect, row, col } from '../pixelgrid.js';
import {
  W, H, newGrid, set, at, hash, fillGround, inked, over, blit, smoke, houseCount, OUTSIDE,
} from './common.js';

const inEll = (x, y, cx, cy, rx, ry) => ((x - cx) / rx) ** 2 + ((y - cy) / ry) ** 2;
const isSea = ch => ch >= '0' && ch <= '3';

// Ett tak sett uppifrån: ljus kant uppe/vänster, skugga nere/höger.
function roof(g, x, y, w, h, base = 'M', shade = 'm') {
  rect(g, x, y, w, h, base);
  row(g, x, y, w, 'P'); col(g, x, y, h, 'P');
  row(g, x, y + h - 1, w, shade); col(g, x + w - 1, y, h, shade);
}

const PLAN = {
  temple(g, x, y, l) {          // pelarkrans kring en röd cella
    const w = 12 + l * 2, h = 9 + l;
    rect(g, x, y, w, h, 'T');
    for (let i = 1; i < w - 1; i += 2) { set(g, x + i, y + 1, 'P'); set(g, x + i, y + h - 2, 'P'); }
    for (let j = 1; j < h - 1; j += 2) { set(g, x + 1, y + j, 'P'); set(g, x + w - 2, y + j, 'P'); }
    rect(g, x + 3, y + 3, w - 6, h - 6, 'O'); row(g, x + 3, y + h - 4, w - 6, 'o');
    return [w, h];
  },
  market(g, x, y, l) {          // öppet torg med randiga markiser
    const w = 14 + l * 3, h = 10 + l;
    rect(g, x, y, w, h, 'E');
    for (let i = 0; i < 3 + l; i++) {
      const sx = x + 1 + (i * 5) % (w - 4), sy = y + 1 + Math.floor(i * 5 / (w - 4)) * 4;
      rect(g, sx, sy, 3, 3, i % 2 ? 'O' : 'z'); row(g, sx, sy + 1, 3, 'P');
    }
    return [w, h];
  },
  foundry(g, x, y, l) {         // mörkt tak, glödande ugn
    const w = 9 + l * 2, h = 7 + l;
    roof(g, x, y, w, h, 'r', 'W');
    rect(g, x + w - 4, y + 2, 2, 2, 'F'); set(g, x + w - 3, y + 3, 'f');
    rect(g, x - 3, y + 2, 3, 3, 'q');           // malmhög
    return [w, h];
  },
  barracks(g, x, y, l) {        // långhus + exercisgård med spjutställ
    const w = 16 + l * 3, h = 9;
    roof(g, x, y, w, 4);
    rect(g, x, y + 4, w, h - 4, 'E');
    for (let i = 2; i < w - 1; i += 3) col(g, x + i, y + 5, 3, 'W');
    return [w, h];
  },
  stable(g, x, y, l) {          // stall + inhägnad hage med hästar
    const w = 18 + l * 3, h = 11;
    roof(g, x, y, 6, h);
    for (let i = 6; i < w; i++) { set(g, x + i, y, 'W'); set(g, x + i, y + h - 1, 'W'); }
    col(g, x + w - 1, y, h, 'W');
    for (let i = 0; i < l + 1; i++) { const hx = x + 8 + i * 4, hy = y + 3 + (i % 2) * 3; row(g, hx, hy, 3, 'w'); set(g, hx + 3, hy - 1, 'w'); }
    return [w, h];
  },
  olive_press(g, x, y, l) {     // verkstad + rund presssten
    const w = 8 + l, h = 7;
    roof(g, x, y, w, h);
    const px = x + w + 1, py = y + 1;
    rect(g, px + 1, py, 3, 5, 'T'); rect(g, px, py + 1, 5, 3, 'T'); set(g, px + 2, py + 2, 'q');
    return [w + 6, h];
  },
  winery(g, x, y, l) {          // verkstad + kar
    const w = 9 + l, h = 7;
    roof(g, x, y, w, h);
    for (let i = 0; i < l + 1; i++) { rect(g, x + w + 1, y + i * 3, 2, 2, 'o'); }
    return [w + 4, h];
  },
};

function underConstruction(g, x, y, w, h, phase) {
  // Grunden syns alltid; murarna fylls i takt med bygget, ställningen runtom.
  rect(g, x, y, w, h, 'E');
  const filled = Math.round(h * phase);
  for (let j = 0; j < filled; j++) row(g, x, y + h - 1 - j, w, j % 2 ? 'p' : 'd');
  for (let i = 0; i < w; i += 2) { set(g, x + i, y - 1, 'W'); set(g, x + i, y + h, 'W'); }
  for (let j = 0; j < h; j += 2) { set(g, x - 1, y + j, 'W'); set(g, x + w, y + j, 'W'); }
}

export function render(ctx, scene) {
  const { sett, terrain, coastal } = scene;
  const pop = sett.population, walls = sett.walls;
  const t = Math.max(0, Math.min(1, Math.log10(pop / 100) / Math.log10(280)));
  const g = newGrid(W, H);
  fillGround(g, 0, 0, W, H, terrain, 21);

  // ── Havet i sydost, som på kartan ──
  const shore = (x, y) => x * 0.55 + y - (coastal ? 150 : 999) + 4 * Math.sin(x / 9) + 2 * Math.sin(y / 5);
  if (coastal) for (let y = 0; y < H; y++) for (let x = 0; x < W; x++) {
    const d = shore(x, y);
    if (d < -1) continue;
    let ch = d < 0 ? 'E' : d < 1.5 ? '3' : d < 4 ? '2' : d < 16 ? '1' : '0';
    if ((ch === '1' || ch === '0') && hash(x, y, 4) % 31 === 0) ch = '3';
    set(g, x, y, ch);
  }

  const cx = coastal ? 92 : 105, cy = coastal ? 56 : 64;
  const rx = Math.round(14 + 52 * t), ry = Math.round(10 + 36 * t);
  const taken = [];
  const free = (x, y, w, h) => !taken.some(([a, b, c, d]) => x < a + c && a < x + w && y < b + d && b < y + h);
  const insideTown = (x, y, w, h) => inEll(x, y, cx, cy, rx, ry) < 0.85 && inEll(x + w, y + h, cx, cy, rx, ry) < 0.85
    && inEll(x + w, y, cx, cy, rx, ry) < 0.85 && inEll(x, y + h, cx, cy, rx, ry) < 0.85
    && ![[x, y], [x + w, y], [x, y + h], [x + w, y + h]].some(([a, b]) => isSea(at(g, a, b)) || isSea(at(g, a + 2, b + 2)));

  // ── Gatumarken innanför ──
  for (let y = 0; y < H; y++) for (let x = 0; x < W; x++)
    if (inEll(x, y, cx, cy, rx + 1, ry + 1) < 1 && !isSea(at(g, x, y))) set(g, x, y, hash(x, y, 2) % 11 === 0 ? 'e' : 'E');
  // Vägar ut: genom porten söderut, och en mot väster.
  for (let y = cy; y < H; y++) for (let k = -1; k <= 1; k++) if (!isSea(at(g, cx + k, y))) set(g, cx + k, y, 'E');
  for (let x = 0; x < cx; x++) for (let k = -1; k <= 0; k++) set(g, x, cy + 6 + Math.round(3 * Math.sin(x / 14)) + k, 'E');
  taken.push([cx - 2, cy, 5, H]);                     // huvudgatan hålls fri

  // ── Fält och utomhusverk ──
  const farmLvl = (sett.buildings.find(b => b.type === 'farm') || {}).level || 0;
  const fields = [[6, 6], [150, 6], [6, 90], [30, 6], [176, 34]];
  for (let i = 0; i < Math.max(1, farmLvl * 2); i++) {
    const [fx, fy] = fields[i]; const fw = 22, fh = 14;
    if (inEll(fx + fw / 2, fy + fh / 2, cx, cy, rx + 14, ry + 10) < 1) continue;
    for (let y = 0; y < fh; y++) for (let x = 0; x < fw; x++) {
      if (isSea(at(g, fx + x, fy + y))) continue;
      const vertical = i % 2;
      const k = vertical ? x : y;
      set(g, fx + x, fy + y, k % 3 === 0 ? '=' : ['-', '_', '6'][i % 3]);
    }
    taken.push([fx, fy, fw, fh]);
  }
  const OUT = {
    mine: (gg, x, y) => { rect(gg, x, y, 8, 6, '8'); rect(gg, x + 2, y + 1, 3, 3, 'V'); rect(gg, x + 5, y + 3, 4, 3, 'q'); },
    silver_mine: (gg, x, y) => { rect(gg, x, y, 8, 6, '8'); rect(gg, x + 2, y + 1, 3, 3, 'V'); rect(gg, x + 5, y + 3, 4, 3, 'S'); },
    stonequarry: (gg, x, y) => { for (let i = 0; i < 4; i++) rect(gg, x + i * 2, y + i * 2, 10 - i * 2, 2, i % 2 ? 'T' : 'P'); },
    lumbermill: (gg, x, y) => { roof(gg, x, y, 6, 5); for (let i = 0; i < 3; i++) row(gg, x + 7, y + i * 2, 5, 'w'); },
  };
  let seed = 5;
  for (const b of sett.buildings.filter(b => OUTSIDE.has(b.type) && b.type !== 'farm')) {
    for (let tries = 0; tries < 300; tries++, seed++) {
      const x = 3 + hash(seed, 1, 3) % (W - 18), y = 3 + hash(seed, 2, 3) % (H - 12);
      if (inEll(x + 6, y + 4, cx, cy, rx + 12, ry + 10) < 1 || !free(x - 1, y - 1, 15, 10)) continue;
      if (isSea(at(g, x, y)) || isSea(at(g, x + 13, y + 8))) continue;
      taken.push([x - 1, y - 1, 15, 10]);
      inked(g, gg => OUT[b.type](gg, x, y));
      break;
    }
  }

  // ── Borgen: berghällen, innermuren, megaron med härd ──
  const smokes = [];
  if (pop >= 1000) {
    const bw = Math.round(22 + 22 * t), bh = Math.round(14 + 12 * t);
    const bx = cx - (bw >> 1), by = cy - ry + 3 + Math.round(4 * t);
    taken.push([bx - 2, by - 2, bw + 4, bh + 4]);
    inked(g, gg => {
      rect(gg, bx, by, bw, bh, 'T');
      for (let x = bx; x < bx + bw; x++) for (let y = by; y < by + bh; y++) if (hash(x, y, 9) % 9 === 0) set(gg, x, y, 'q');
      row(gg, bx, by, bw, 'P'); col(gg, bx, by, bh, 'P');
      row(gg, bx, by + bh - 1, bw, 't'); col(gg, bx + bw - 1, by, bh, 't');
      row(gg, bx, by + bh - 2, bw, 't'); col(gg, bx + bw - 2, by, bh, 't');
      // Megaron: förhall, förrum, hallen med den runda härden.
      const mw = 9 + Math.round(4 * t), mh = 12 + Math.round(4 * t);
      const mx = cx - (mw >> 1), my = by + 2;
      roof(gg, mx, my, mw, mh, 'R', 'r');
      row(gg, mx + 1, my + mh - 4, mw - 2, 'O');                 // ockrabandet
      rect(gg, cx - 1, my + 3, 3, 3, 'F'); set(gg, cx, my + 4, 'f');    // härden
      set(gg, mx + 2, my + mh, 'P'); set(gg, mx + mw - 3, my + mh, 'P'); // förhallens pelare
      if (t > 0.5) {                                              // palatsflyglar runt en gård
        roof(gg, bx + 2, my, mx - bx - 4, 6); roof(gg, mx + mw + 2, my, bx + bw - mx - mw - 4, 6);
        roof(gg, bx + 2, my + 8, 5, mh - 6); roof(gg, bx + bw - 7, my + 8, 5, mh - 6);
      }
    });
    smokes.push([cx, by + 3]);
  }

  // ── Verkstäder och tempel i stadens kvarter ──
  const items = [
    ...sett.buildings.filter(b => PLAN[b.type]).map(b => ({ ...b, phase: 1 })),
    ...sett.build_queue.map(b => ({ ...b })),
  ];
  for (const b of items) {
    const probe = newGrid(60, 30);
    const [w, h] = PLAN[b.type] ? PLAN[b.type](probe, 4, 4, b.level || 1) : [10, 8];
    const extra = b.type === 'foundry' ? 3 : 0;
    for (let tries = 0; tries < 600; tries++, seed++) {
      const x = cx - rx + extra + hash(seed, 3, 3) % (2 * rx), y = cy - ry + hash(seed, 4, 3) % (2 * ry);
      if (!insideTown(x - extra, y, w + extra, h) || !free(x - 2 - extra, y - 2, w + 4 + extra, h + 4)) continue;
      taken.push([x - 2 - extra, y - 2, w + 4 + extra, h + 4]);
      if (b.phase >= 1) inked(g, gg => PLAN[b.type](gg, x, y, b.level || 1));
      else inked(g, gg => underConstruction(gg, x, y, w, h, b.phase));
      if (b.type === 'foundry' && b.phase >= 1) smokes.push([x + w - 3, y + 1]);
      break;
    }
  }

  // ── Bostadskvarteren: tak i lappverk, gränder emellan ──
  const nHouses = Math.round(houseCount(pop) * 5);
  let placed = 0;
  for (let tries = 0; tries < 6000 && placed < nHouses; tries++, seed++) {
    const w = 4 + hash(seed, 5, 3) % 4, h = 3 + hash(seed, 6, 3) % 4;
    // Kvartersraster: husen hamnar i block, med en gränd var tionde pixel.
    const x = cx - rx + hash(seed, 7, 3) % (2 * rx), y = cy - ry + hash(seed, 8, 3) % (2 * ry);
    if ((x % 11) > 11 - w || (y % 9) > 9 - h) continue;
    if (!insideTown(x, y, w, h) || !free(x, y, w + 1, h + 1)) continue;
    taken.push([x, y, w + 1, h + 1]);
    const court = w >= 6 && h >= 5 && hash(seed, 9, 3) % 3 === 0, s = seed;
    inked(g, gg => {
      roof(gg, x, y, w, h);
      if (court) rect(gg, x + 2, y + 2, w - 4, h - 3, 'E');
    });
    if (hash(s, 10, 3) % 13 === 0) smokes.push([x + 1, y]);
    placed++;
  }

  // ── Ringmuren med torn och port ──
  if (walls) {
    const th = walls + 1;
    const towers = [];
    inked(g, gg => {
      for (let y = cy - ry - th - 1; y <= cy + ry + th + 1; y++) for (let x = cx - rx - th - 1; x <= cx + rx + th + 1; x++) {
        const e = inEll(x, y, cx, cy, rx + 1 + th, ry + 1 + th), ei = inEll(x, y, cx, cy, rx + 1, ry + 1);
        if (e < 1 && ei >= 1) {
          if (y > cy && Math.abs(x - cx) <= 2) continue;              // porten
          if (isSea(at(g, x, y))) continue;
          set(gg, x, y, (x > cx && y > cy) ? 'q' : 'T');
        }
      }
      if (walls >= 2) {
        const n = walls === 2 ? 8 : 12;
        for (let i = 0; i < n; i++) {
          const a = (i + 0.5) / n * Math.PI * 2;
          const tx = Math.round(cx + Math.cos(a) * (rx + 1 + th / 2)), ty = Math.round(cy + Math.sin(a) * (ry + 1 + th / 2));
          if (isSea(at(g, tx, ty))) continue;
          const s = walls + 2;
          rect(gg, tx - (s >> 1), ty - (s >> 1), s, s, 'T');
          row(gg, tx - (s >> 1), ty - (s >> 1), s, 'P'); col(gg, tx + (s >> 1), ty - (s >> 1), s, 't');
          towers.push([tx, ty]);
        }
        for (const sx of [cx - 5, cx + 3]) { rect(gg, sx, cy + ry, 3, th + 3, 'T'); col(gg, sx + 2, cy + ry, th + 3, 't'); }
      }
    });
  }

  // ── Hamnen: kajer ut i vattnet, skepp uppifrån ──
  if (coastal) {
    const hl = (sett.buildings.find(b => b.type === 'harbour') || {}).level || 0;
    let qx = cx + 30, qy = 0;
    for (let y = 0; y < H; y++) if (isSea(at(g, qx, y))) { qy = y; break; }
    if (hl) inked(g, gg => {
      for (let i = 0; i < hl + 1; i++) rect(gg, qx + i * 10, qy - 2 - i * 5, 3, 12, 'T');
      row(gg, qx - 4, qy - 2, 8 + hl * 10, 'T');
    });
    const ship = (gg, x, y, L) => {
      row(gg, x + 1, y, L - 2, '<'); row(gg, x, y + 1, L, 'w'); row(gg, x + 1, y + 2, L - 2, '<');
      set(gg, x + L, y + 1, '<');
      for (let i = 2; i < L - 2; i += 2) { set(gg, x + i, y - 1, 'W'); set(gg, x + i, y + 3, 'W'); }
    };
    for (let i = 0; i < (hl ? 2 + hl : 1); i++) {
      const sx = qx + 4 + i * 11, sy = qy + 4 + (i % 2) * 9 + i * 2;
      if (sy < H - 4 && sx < W - 14) inked(g, gg => ship(gg, sx, sy, 10 + (i % 2) * 3));
    }
  }

  // ── Träd uppifrån ──
  const grove = (x, y) => Math.sin(x / 11 + 0.7) + Math.sin(y / 8 + x / 27) + Math.sin((x - y) / 17);
  for (let i = 0; i < 700; i++) {
    const x = 2 + hash(i, 3, 41) % (W - 4), y = 2 + hash(i, 4, 41) % (H - 4);
    if (grove(x, y) < 1.0 || inEll(x, y, cx, cy, rx + 4, ry + 4) < 1) continue;
    if (isSea(at(g, x, y)) || !free(x - 2, y - 2, 4, 4)) continue;
    taken.push([x - 2, y - 2, 4, 4]);
    inked(g, gg => { row(gg, x - 1, y - 1, 2, '*'); row(gg, x - 1, y, 3, '+'); set(gg, x, y - 1, '+'); row(gg, x, y + 1, 2, '#'); });
  }

  // ── Folk på gatorna, rök ──
  const nPeople = Math.min(90, 3 + Math.round(pop / 300));
  over(g, gg => {
    let n = 0;
    for (let i = 0; i < 4000 && n < nPeople; i++) {
      const x = cx - rx - 6 + hash(i, 1, 8) % (2 * rx + 12), y = cy - ry + hash(i, 2, 8) % (2 * ry + 30);
      if (at(g, x, y) !== 'E' || at(g, x + 1, y) !== 'E') continue;
      set(gg, x, y, ['$', '$', 'O', 'z', 'w', '@'][hash(i, 3, 8) % 6]); n++;
    }
    smokes.forEach(([x, y], i) => smoke(gg, x, y, 7, i));
  });

  blit(ctx, g);
}
