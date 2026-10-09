// ── Skiss A: DIORAMA — staden snett ovanifrån ─────────────────────────────
//
// Samma vinkel som kartans stadsmassa, bara närmare: borgen på sin höjd längst
// bak, terrasser, den lägre staden, ringmuren runt alltihop och marken utanför.
// Allt i bild är data: husens antal = befolkning, murens höjd/tinnar/torn =
// murnivå, verkstäderna = byggnaderna, hamnen och havet = kust.

import { cube, rect, row, col, polyRows, ringPixels, raiseWall } from '../pixelgrid.js';
import { stampBuilding, stampUnderConstruction, buildingWidth } from '../citybuildings.js';
import {
  W, H, newGrid, set, at, hash, rnd, fillGround, inked, over, blit, person, smoke,
  houseCount, OUTSIDE,
} from './common.js';

function ellipsePoly(cx, cy, rx, ry, n = 40) {
  const pts = [];
  for (let i = 0; i < n; i++) {
    const a = (i / n) * Math.PI * 2;
    pts.push([Math.round(cx + Math.cos(a) * rx), Math.round(cy + Math.sin(a) * ry)]);
  }
  return pts;
}
const inEll = (x, y, cx, cy, rx, ry) => ((x - cx) / rx) ** 2 + ((y - cy) / ry) ** 2;

function shoreY(x) { return 106 + Math.round(4 * Math.sin(x / 19) + 2 * Math.sin(x / 7 + 1)); }

function tree(g, x, y, olive) {
  const a = olive ? '+' : '#', b = olive ? '*' : '+';
  row(g, x - 1, y - 4, 3, b); row(g, x - 2, y - 3, 5, a); set(g, x - 1, y - 3, b);
  row(g, x - 2, y - 2, 5, a); row(g, x - 1, y - 1, 3, a); set(g, x, y, 'W');
}

function ship(g, x, y, big) {
  const L = big ? 18 : 11;
  row(g, x + 1, y, L - 2, '<'); row(g, x, y - 1, L, '<'); set(g, x - 1, y - 2, '<'); set(g, x + L, y - 2, '<');
  row(g, x, y - 2, L, 'w');
  if (big) {
    const m = x + (L >> 1);
    col(g, m, y - 12, 10, 'W');
    rect(g, m - 4, y - 11, 9, 7, '>');
    col(g, m + 4, y - 11, 7, 'P');
  } else {
    col(g, x + 5, y - 7, 5, 'W'); rect(g, x + 3, y - 7, 5, 4, '>');
  }
}

export function render(ctx, scene) {
  const { sett, terrain, coastal } = scene;
  const pop = sett.population, walls = sett.walls;
  const g = newGrid(W, H);

  // ── Fond: himmel, fjärran åsar, mark ──
  for (let y = 0; y < 12; y++) for (let x = 0; x < W; x++) set(g, x, y, y < 3 ? '`' : y < 7 ? '~' : '9');
  fillGround(g, 0, 12, W, H, terrain, 3);
  for (let x = 0; x < W; x++) {
    const top = 8 + Math.round(3 * Math.sin(x / 17) + 2 * Math.sin(x / 6.3 + 2));
    for (let y = top; y < 15; y++) {
      const slope = Math.sin((x + 1) / 17) - Math.sin(x / 17);
      set(g, x, y, y === top ? ':' : (slope > 0 ? ':' : '^'));
    }
  }
  if (coastal) {
    for (let x = 0; x < W; x++) {
      const s = shoreY(x);
      for (let y = s - 1; y < H; y++) {
        const d = y - s;
        let ch = d < 0 ? 'E' : d < 1 ? '3' : d < 3 ? '2' : d < 9 ? '1' : '0';
        if ((ch === '1' || ch === '0') && hash(x, y, 5) % 37 === 0) ch = '3';
        set(g, x, y, ch);
      }
    }
  }

  // ── Stadens mått ur befolkningen ──
  const t = Math.max(0, Math.min(1, Math.log10(pop / 100) / Math.log10(280)));
  const cx = 105, rx = Math.round(22 + 48 * t), ry = Math.round(10 + 22 * t);
  const cy = coastal ? 100 - ry - 6 : 66;
  const wallRx = rx + 3, wallRy = ry + 2;

  // Gården innanför: slagen jord.
  for (let y = 12; y < H; y++) for (let x = 0; x < W; x++) {
    const e = inEll(x, y, cx, cy, rx + 1, ry + 1);
    if (e < 1 && (e < 0.8 || hash(x, y, 2) % 3)) set(g, x, y, hash(x, y, 4) % 9 === 0 ? 'e' : 'E');
  }
  // Vägen ut genom porten och ner mot vattnet / ut ur bild.
  const gateY = cy + wallRy;
  for (let y = gateY; y < H; y++) {
    if (coastal && y > shoreY(cx) - 1) break;
    const wob = Math.round(2 * Math.sin(y / 6));
    for (let k = -2; k <= 2; k++) set(g, cx + wob + k, y, Math.abs(k) === 2 ? 'e' : 'E');
  }

  const taken = [];
  const free = (x0, y0, w, h) => !taken.some(([a, b, c, d]) => x0 < a + c && a < x0 + w && y0 < b + d && b < y0 + h);
  const items = [];   // { y, draw }

  // ── Fält och utomhusbyggnader ──
  const farmLvl = (sett.buildings.find(b => b.type === 'farm') || {}).level || 0;
  const fieldSpots = [[18, 30], [176, 34], [14, 70], [186, 74], [40, 22], [160, 20]];
  const nFields = farmLvl ? 1 + farmLvl : 1;
  for (let i = 0; i < nFields; i++) {
    const [fx, fy] = fieldSpots[i];
    const fw = 22, fh = 10;
    if (inEll(fx + fw / 2, fy + fh / 2, cx, cy, wallRx + 8, wallRy + 6) < 1) continue;
    for (let y = 0; y < fh; y++) for (let x = 0; x < fw; x++) {
      const xx = fx + x + (y >> 1);
      if (coastal && fy + y >= shoreY(xx) - 2) continue;
      set(g, xx, fy + y, y % 2 ? '=' : (hash(x, i, 1) % 4 ? '-' : '_'));
    }
    taken.push([fx, fy, fw + fh / 2, fh]);
  }
  const outside = sett.buildings.filter(b => OUTSIDE.has(b.type));
  let seed = 11;
  for (const b of outside) {
    const w = buildingWidth(b.type, b.level);
    for (let tries = 0; tries < 300; tries++, seed++) {
      const x = 2 + (hash(seed, 1, 9) % (W - w - 4)), base = 30 + (hash(seed, 2, 9) % (H - 34));
      if (coastal && base > shoreY(x) - 4) continue;
      if (inEll(x + w / 2, base - 6, cx, cy, wallRx + 4 + w / 2, wallRy + 8) < 1) continue;
      if (!free(x - 1, base - 16, w + 2, 18)) continue;
      taken.push([x - 1, base - 16, w + 2, 18]);
      items.push({ y: base, draw: gg => stampBuilding(gg, b.type, x, base, b.level) });
      break;
    }
  }

  // ── Borgen: terrasser och megaron ──
  const nTer = pop < 1000 ? 0 : pop < 5000 ? 1 : pop < 15000 ? 2 : 3;
  let hillTop = cy;
  if (nTer) {
    const hy = cy - Math.round(ry * 0.25);
    const terraces = [];
    for (let k = 0; k < nTer; k++) {
      const trx = Math.round((11 + 15 * t) * (1 - k * 0.24)), tr = Math.round(trx * 0.38);
      terraces.push({ cy: hy - k * 6, rx: trx, ry: tr });
    }
    hillTop = terraces[nTer - 1].cy;
    taken.push([cx - terraces[0].rx, hy - terraces[0].ry - 6 * nTer - 14, terraces[0].rx * 2, terraces[0].ry * 2 + 6 * nTer + 18]);
    items.push({ y: hy + terraces[0].ry + 4, draw: gg => {
      for (const tr of terraces) {
        for (let y = tr.cy - tr.ry; y <= tr.cy + tr.ry + 4; y++)
          for (let x = cx - tr.rx; x <= cx + tr.rx; x++) {
            const e = inEll(x, y, cx, tr.cy, tr.rx, tr.ry);
            const eFace = inEll(x, y - 4, cx, tr.cy, tr.rx, tr.ry);
            if (e < 1) {
              const lit = (x - cx) / tr.rx + (y - tr.cy) / tr.ry < -0.9;
              set(gg, x, y, lit ? 'P' : (e > 0.82 && x > cx ? 'q' : 'T'));
            } else if (eFace < 1 && y > tr.cy) {
              // Stödmuren: kyklopiska block, skugga åt höger.
              const blk = ((x >> 2) + (y >> 1)) % 2;
              set(gg, x, y, x > cx + tr.rx * 0.4 ? 't' : (blk ? 'q' : 'T'));
              if (hash(x, y, 8) % 7 === 0) set(gg, x, y, 't');
            }
          }
      }
    } });
    // Megaron på krönet — större med befolkningen.
    const mw = 14 + Math.round(10 * t), mh = 9 + Math.round(3 * t);
    const top = hillTop;
    items.push({ y: hy + terraces[0].ry + 5, draw: gg => {
      cube(gg, cx - (mw >> 1), top - mh + 2, mw, mh, { depth: 2, parapet: true, band: true, door: true, roof: 2 });
      if (t > 0.6) {
        cube(gg, cx - (mw >> 1) - 9, top - 6 + 3, 9, 6, { depth: 1, window: true });
        cube(gg, cx + (mw >> 1) + 1, top - 7 + 3, 10, 7, { depth: 1, window: true });
      }
      // Förhallens pelare.
      for (let i = 1; i < 4; i++) col(gg, cx - (mw >> 1) + Math.round(i * mw / 4), top - mh + 5, mh - 4, i === 2 ? 'V' : 'P');
    } });
  }

  // ── Stadens verkstäder och hus ──
  const inside = [
    ...sett.buildings.filter(b => !OUTSIDE.has(b.type) && b.type !== 'harbour').map(b => ({ ...b, phase: 1 })),
    ...sett.build_queue.map(b => ({ ...b })),
  ];
  for (const b of inside) {
    const w = buildingWidth(b.type, b.level);
    for (let tries = 0; tries < 400; tries++, seed++) {
      const x = cx - rx + (hash(seed, 3, 9) % Math.max(1, 2 * rx - w));
      const base = cy - Math.round(ry * 0.1) + (hash(seed, 4, 9) % Math.max(1, Math.round(ry * 1.0)));
      if (inEll(x, base, cx, cy, rx, ry) > 0.95 || inEll(x + w, base, cx, cy, rx, ry) > 0.95) continue;
      if (!free(x - 1, base - 15, w + 2, 16)) continue;
      taken.push([x - 1, base - 15, w + 2, 16]);
      items.push({ y: base, draw: gg => (b.phase >= 1
        ? stampBuilding(gg, b.type, x, base, b.level)
        : stampUnderConstruction(gg, b.type, x, base, b.level, b.phase)) });
      if (b.type === 'foundry' && b.phase >= 1) items.push({ y: -1, smoke: [x + 10, base - 14] });
      break;
    }
  }
  const nHouses = Math.round(houseCount(pop) * 4);
  let placed = 0;
  for (let tries = 0; tries < 3000 && placed < nHouses; tries++, seed++) {
    const w = 5 + (hash(seed, 5, 9) % 4), h = 5 + (hash(seed, 6, 9) % 3);
    const dep = [-2, -1, 1, 2][hash(seed, 7, 9) % 4];
    const x = cx - rx + (hash(seed, 8, 9) % (2 * rx));
    const base = cy - ry + (hash(seed, 10, 9) % (2 * ry + 1));
    if (inEll(x + w / 2, base, cx, cy, rx, ry) > 0.9) continue;
    const fx = x - (dep < 0 ? 2 * -dep : 0), fw = w + 2 * Math.abs(dep), fy = base - h - Math.abs(dep) - 1;
    if (!free(fx, fy, fw, h + Math.abs(dep) + 2)) continue;
    taken.push([fx, fy, fw, h + Math.abs(dep) + 2]);
    const s2 = seed;
    items.push({ y: base, draw: gg => cube(gg, x, base - h, w, h, { depth: dep, parapet: hash(s2, 1, 3) % 2 === 0, door: true }) });
    if (hash(seed, 11, 9) % 7 === 0) items.push({ y: -1, smoke: [x + 1, base - h - 2] });
    placed++;
  }

  // ── Ringmuren ──
  let wallFront = [], wallBack = [];
  const wh = [0, 4, 6, 8][walls];
  if (walls) {
    const ring = ringPixels(polyRows(ellipsePoly(cx, cy, wallRx, wallRy)), 2);
    wallBack = ring.back; wallFront = ring.front.filter(([x]) => Math.abs(x - cx) > 3);
  }
  const drawWall = (gg, px) => {
    raiseWall(gg, px, wh);
    if (walls >= 2) for (const [x, y] of px) if (x % 3 === 0) set(gg, x, y - wh + 1, '.');
  };

  // ── Sammanställningen bakifrån och fram ──
  if (walls) inked(g, gg => drawWall(gg, wallBack));
  if (walls >= 3) for (const a of [0.9, 2.2]) {
    const tx = Math.round(cx + Math.cos(Math.PI + a) * wallRx), ty = Math.round(cy + Math.sin(Math.PI + a) * wallRy);
    inked(g, gg => { rect(gg, tx - 3, ty - wh - 4, 7, wh + 5, 'q'); col(gg, tx - 3, ty - wh - 4, wh + 5, 'T'); row(gg, tx - 3, ty - wh - 4, 7, 'T'); col(gg, tx + 3, ty - wh - 4, wh + 5, 't'); });
  }
  const smokes = [];
  items.sort((a, b) => a.y - b.y);
  for (const it of items) { if (it.smoke) smokes.push(it.smoke); else inked(g, it.draw); }
  if (walls) {
    inked(g, gg => drawWall(gg, wallFront));
    // Porten, och för mur ≥ 2 flankerande torn.
    inked(g, gg => {
      const gy = cy + wallRy;
      rect(gg, cx - 3, gy - wh, 7, wh + 1, 'q');
      rect(gg, cx - 1, gy - wh + 2, 3, wh - 1, 'V');
      row(gg, cx - 3, gy - wh, 7, 'T');
      if (walls >= 2) for (const sx of [cx - 9, cx + 4]) {
        rect(gg, sx, gy - wh - 3, 6, wh + 4, 'q');
        col(gg, sx, gy - wh - 3, wh + 4, 'T'); col(gg, sx + 5, gy - wh - 3, wh + 4, 't');
        row(gg, sx, gy - wh - 3, 6, 'T'); set(gg, sx + 2, gy - wh - 4, 'T'); set(gg, sx + 4, gy - wh - 4, 'T');
      }
    });
  }

  // ── Hamnen och skeppen ──
  if (coastal) {
    const hl = (sett.buildings.find(b => b.type === 'harbour') || {}).level || 0;
    const qy = shoreY(cx) + 1;
    if (hl) inked(g, gg => {
      rect(gg, cx - 22, qy - 1, 44, 3, 'T'); row(gg, cx - 22, qy + 2, 44, 'q');
      for (let i = 0; i < hl; i++) rect(gg, cx - 18 + i * 14, qy + 2, 3, 8, 'T');
    });
    const ships = hl ? hl + 1 : 1;
    for (let i = 0; i < ships; i++) {
      const sx = 30 + i * 46 + (hash(i, 1, 2) % 10), sy = shoreY(sx) + 12 + (hash(i, 2, 2) % 6);
      inked(g, gg => ship(gg, sx, Math.min(H - 2, sy), hl > 0 && i < hl));
    }
  }

  // ── Träd utanför ──
  const olive = terrain === 'forest_olive_grove' || terrain === 'plains' || terrain === 'river_valley';
  const grove = (x, y) => Math.sin(x / 13 + 1.3) + Math.sin(y / 9 + x / 31) + Math.sin((x + y) / 21);
  for (let i = 0; i < 400; i++) {
    const x = 3 + hash(i, 3, 77) % (W - 6), y = 20 + hash(i, 4, 77) % (H - 22);
    if (grove(x, y) < 1.1) continue;
    if (inEll(x, y, cx, cy, wallRx + 4, wallRy + 4) < 1) continue;
    if (coastal && y > shoreY(x) - 3) continue;
    if (!free(x - 2, y - 4, 4, 5)) continue;
    if (at(g, x, y) !== '4' && at(g, x, y) !== '5' && at(g, x, y) !== '6' && at(g, x, y) !== '7' && at(g, x, y) !== '8') continue;
    taken.push([x - 2, y - 4, 4, 5]);
    inked(g, gg => tree(gg, x, y, olive));
  }

  // ── Livet: folk på gården och vägen, rök ──
  const nPeople = Math.min(70, 2 + Math.round(pop / 450));
  over(g, gg => {
    let n = 0;
    for (let i = 0; i < 2000 && n < nPeople; i++) {
      const x = cx - rx - 4 + hash(i, 5, 13) % (2 * rx + 8), y = cy - ry + hash(i, 6, 13) % (2 * ry + 18);
      if (at(g, x, y) !== 'E' || at(g, x, y - 1) !== 'E' || at(g, x, y - 2) !== 'E') continue;
      const tunic = ['$', '$', '$', 'O', 'z', 'w', 'N'][hash(i, 7, 13) % 7];
      person(gg, x, y, tunic); n++;
    }
    smokes.forEach(([x, y], i) => smoke(gg, x, y, 9, i));
  });

  blit(ctx, g);
}
