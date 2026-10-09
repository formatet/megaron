// ── Skiss C2: PLAN, reduktionspass ─────────────────────────────────────────
//
// Timothy valde C (2026-10-09). Sanas kritik av C1: småhusbrus, allt lika
// kontrastrikt, ingen läsordning, byggnader som bara skiljer sig på färg.
//
// Läsordningen den här skissen bygger för:
//   stad → storlek → mur/port → ägare (vimpel) → borg → funktionsbyggnader
//   → bostäder → mark.
//
// Det görs med TRE kontrastnivåer:
//   låg     mark, fält, träd, hav — ingen kontur, svagt brus
//   mellan  bostadskvarter — sammanhängande massor, ingen egen kontur
//   hög     mur, port, borg, vimpel, funktionsbyggnader, bygge — bläckkontur
//
// Bostäderna ritas inte som hus utan som KVARTER (insulae) mellan gator.
// Varje funktionsbyggnad har en egen planform som går att läsa i gråskala.

import { rect, row, col } from '../pixelgrid.js';
import { W, H, newGrid, set, at, hash, inked, over, blit, smoke, GROUND } from './common.js';

const inEll = (x, y, cx, cy, rx, ry) => ((x - cx) / rx) ** 2 + ((y - cy) / ry) ** 2;
const isSea = ch => ch >= '0' && ch <= '3';
const BW = 10, BH = 7, SX = 12, SY = 9;           // kvarter + gata

// ── Funktionsbyggnadernas planformer. (gg, x, y, w, h, lvl) ──
const PLAN = {
  // Tempel: pelarkrans (prickar) runt röd cella, altare framför.
  temple(g, x, y, w, h) {
    rect(g, x, y, w, h, 'T');
    for (let i = 1; i < w - 1; i += 2) { set(g, x + i, y, 'P'); set(g, x + i, y + h - 1, 'P'); }
    for (let j = 1; j < h - 1; j += 2) { set(g, x, y + j, 'P'); set(g, x + w - 1, y + j, 'P'); }
    rect(g, x + 3, y + 2, w - 6, h - 4, 'O'); row(g, x + 3, y + h - 3, w - 6, 'o');
    set(g, x + (w >> 1), y + h + 1, 'F');
  },
  // Marknad: öppet torg, randiga tygtak.
  market(g, x, y, w, h) {
    rect(g, x, y, w, h, 'E');
    for (let i = 0; i < 4; i++) {
      const sx = x + 1 + i * 5, sy = y + 1 + (i % 2) * 3;
      for (let k = 0; k < 4; k++) col(g, sx + k, sy, 3, k % 2 ? 'P' : 'O');
    }
  },
  // Kasern: långhus + gård med spjutrad.
  barracks(g, x, y, w, h) {
    rect(g, x, y, w, 3, 'R'); row(g, x, y + 2, w, 'r');
    rect(g, x, y + 3, w, h - 3, 'E');
    for (let i = 1; i < w - 1; i += 2) { set(g, x + i, y + 4, 'S'); col(g, x + i, y + 5, h - 6, 'W'); }
  },
  // Gjuteri: mörkt tak, stor glödande ugn.
  foundry(g, x, y, w, h) {
    rect(g, x, y, w, h, 'r'); row(g, x, y + h - 1, w, 'W');
    rect(g, x + 3, y + 2, 4, 3, 'f'); rect(g, x + 4, y + 2, 2, 2, 'F');
  },
  // Stall: litet stall + hage med staket och hästar.
  stable(g, x, y, w, h) {
    rect(g, x, y, 5, h, 'R'); col(g, x + 4, y, h, 'r');
    for (let i = 5; i < w; i++) { set(g, x + i, y, 'W'); set(g, x + i, y + h - 1, 'W'); }
    col(g, x + w - 1, y, h, 'W');
    rect(g, x + 5, y + 1, w - 6, h - 2, '6');
    for (const [hx, hy] of [[8, 2], [13, 4], [18, 2]]) if (hx + 3 < w) { row(g, x + hx, y + hy, 3, 'w'); set(g, x + hx + 3, y + hy - 1, 'w'); }
  },
  // Olivpress: rund presssten och amforor.
  olive_press(g, x, y, w, h) {
    rect(g, x, y, w, h, 'E');
    rect(g, x + 2, y + 1, 3, 5, 'T'); rect(g, x + 1, y + 2, 5, 3, 'T'); set(g, x + 3, y + 3, 'V');
    for (let j = 1; j < h - 1; j += 2) set(g, x + 7, y + j, 'o');
  },
  // Vinpress: runda kar.
  winery(g, x, y, w, h) {
    rect(g, x, y, w, h, 'E');
    for (const [vx, vy] of [[1, 1], [5, 1], [3, 4]]) { rect(g, x + vx, y + vy, 3, 2, 'o'); set(g, x + vx + 1, y + vy, 'V'); }
  },
};
const WIDE = new Set(['temple', 'market', 'barracks', 'stable']);

function underConstruction(g, x, y, w, h, phase) {
  rect(g, x, y, w, h, 'E');
  const filled = Math.round(h * phase);
  for (let j = 0; j < filled; j++) row(g, x, y + h - 1 - j, w, 'd');
  for (let i = 0; i < w; i += 3) col(g, x + i, y, h, 'W');
  row(g, x, y, w, 'w'); row(g, x, y + h - 1, w, 'w');
}

// Mark med SVAGT brus: en pixel av sextio avviker.
function quietGround(g, terrain) {
  const [b, d] = GROUND[terrain] || GROUND.plains;
  for (let y = 0; y < H; y++) for (let x = 0; x < W; x++) set(g, x, y, hash(x, y, 31) % 60 === 0 ? d : b);
}

export function render(ctx, scene) {
  const { sett, terrain, coastal } = scene;
  const pop = sett.population, walls = sett.walls;
  const t = Math.max(0, Math.min(1, Math.log10(pop / 100) / Math.log10(280)));
  const g = newGrid(W, H);
  quietGround(g, terrain);
  const shade = (GROUND[terrain] || GROUND.plains)[1];

  // ── Lugnt hav: stora ytor, skum bara vid stranden ──
  if (coastal) for (let y = 0; y < H; y++) for (let x = 0; x < W; x++) {
    const d = x * 0.55 + y - 150 + 4 * Math.sin(x / 9) + 2 * Math.sin(y / 5);
    if (d < 0) continue;
    set(g, x, y, d < 1.2 ? '3' : d < 4 ? '2' : d < 18 ? '1' : '0');
  }

  const cx = coastal ? 90 : 105, cy = coastal ? 54 : 62;
  const rx = Math.round(12 + 52 * t), ry = Math.round(8 + 36 * t);

  // ── Fält och träd — låg kontrast, ingen kontur ──
  const farmLvl = (sett.buildings.find(b => b.type === 'farm') || {}).level || 0;
  const fields = [[6, 8], [150, 8], [6, 92], [32, 8], [176, 36]];
  const taken = [];
  const free = (x, y, w, h) => !taken.some(([a, b, c, d]) => x < a + c && a < x + w && y < b + d && b < y + h);
  for (let i = 0; i < Math.max(1, farmLvl * 2); i++) {
    const [fx, fy] = fields[i]; const fw = 24, fh = 14;
    if (inEll(fx + fw / 2, fy + fh / 2, cx, cy, rx + 16, ry + 12) < 1) continue;
    for (let y = 0; y < fh; y++) for (let x = 0; x < fw; x++) {
      if (isSea(at(g, fx + x, fy + y))) continue;
      const k = i % 2 ? x : y;
      set(g, fx + x, fy + y, k % 4 === 0 ? '_' : '-');
    }
    taken.push([fx, fy, fw, fh]);
  }
  const grove = (x, y) => Math.sin(x / 11 + 0.7) + Math.sin(y / 8 + x / 27) + Math.sin((x - y) / 17);
  for (let i = 0; i < 500; i++) {
    const x = 2 + hash(i, 3, 41) % (W - 4), y = 2 + hash(i, 4, 41) % (H - 4);
    if (grove(x, y) < 1.4 || inEll(x, y, cx, cy, rx + 8, ry + 8) < 1) continue;
    if (isSea(at(g, x, y)) || !free(x - 2, y - 2, 4, 4)) continue;
    taken.push([x - 2, y - 2, 4, 4]);
    row(g, x - 1, y - 1, 2, '+'); row(g, x - 1, y, 3, '+'); row(g, x, y + 1, 2, '#');
  }

  // ── Verk utanför muren: gruva, stenbrott, sågverk (konturerade senare) ──
  const OUT = {
    mine: gg => (x, y) => { rect(gg, x, y, 9, 6, '8'); rect(gg, x + 2, y + 1, 3, 3, 'V'); rect(gg, x + 6, y + 3, 4, 3, 'q'); },
    silver_mine: gg => (x, y) => { rect(gg, x, y, 9, 6, '8'); rect(gg, x + 2, y + 1, 3, 3, 'V'); rect(gg, x + 6, y + 3, 4, 3, 'S'); },
    stonequarry: gg => (x, y) => { for (let i = 0; i < 3; i++) rect(gg, x + i * 2, y + i * 2, 10 - i * 2, 2, i % 2 ? 'T' : 'P'); },
    lumbermill: gg => (x, y) => { rect(gg, x, y, 5, 5, 'R'); col(gg, x + 4, y, 5, 'r'); for (let i = 0; i < 3; i++) row(gg, x + 6, y + i * 2, 5, 'w'); },
  };
  const outsideJobs = [];
  let oseed = 5;
  for (const b of sett.buildings.filter(b => OUT[b.type])) {
    for (let tries = 0; tries < 300; tries++, oseed++) {
      const x = 3 + hash(oseed, 1, 3) % (W - 16), y = 3 + hash(oseed, 2, 3) % (H - 10);
      if (inEll(x + 6, y + 3, cx, cy, rx + 12, ry + 10) < 1 || !free(x - 1, y - 1, 14, 9)) continue;
      if (isSea(at(g, x, y)) || isSea(at(g, x + 12, y + 7))) continue;
      taken.push([x - 1, y - 1, 14, 9]);
      outsideJobs.push([b.type, x, y]);
      break;
    }
  }

  // ── Vägar: smala, från porten ut ──
  const gateY = cy + ry + 2;
  for (let y = gateY; y < H; y++) { const x = cx + Math.round(Math.sin((y - gateY) / 8) * 2); if (isSea(at(g, x, y))) break; set(g, x, y, 'e'); set(g, x + 1, y, 'e'); }
  for (let x = 0; x < cx - rx; x++) set(g, x, cy + 4 + Math.round(3 * Math.sin(x / 14)), 'e');

  // ── Gatumarken innanför muren ──
  for (let y = 0; y < H; y++) for (let x = 0; x < W; x++)
    if (inEll(x, y, cx, cy, rx + 1, ry + 1) < 1 && !isSea(at(g, x, y))) set(g, x, y, 'e');

  // ── Kvarterscellerna ──
  const cells = [];
  for (let r = 0; r * SY < 2 * ry; r++) {
    const y = cy - ry + 1 + r * SY;
    for (let side of [-1, 1]) for (let k = 0; k < 12; k++) {
      const x = side > 0 ? cx + 2 + k * SX : cx - 1 - BW - k * SX;
      const ok = [[x, y], [x + BW, y], [x, y + BH], [x + BW, y + BH]]
        .every(([a, b]) => inEll(a, b, cx, cy, rx, ry) < 0.92 && !isSea(at(g, a, b)) && !isSea(at(g, a + 3, b + 3)));
      if (ok) cells.push({ x, y, r, side, k, used: false });
    }
  }
  // Borgen tar de översta cellerna närmast mittgatan.
  const hasCitadel = pop >= 1000;
  const cit = hasCitadel ? { w: Math.round(24 + 16 * t), h: Math.round(14 + 6 * t) } : null;
  if (cit) {
    cit.x = cx - (cit.w >> 1); cit.y = cy - ry + 1;
    for (const c of cells) if (c.x < cit.x + cit.w + 2 && c.x + BW > cit.x - 2 && c.y < cit.y + cit.h + 2) c.used = true;
  }
  // Funktionsbyggnader: var och en på sin plats i staden.
  const want = [
    ...sett.buildings.filter(b => PLAN[b.type]).map(b => ({ ...b, phase: 1 })),
    ...sett.build_queue.filter(b => PLAN[b.type]).map(b => ({ ...b })),
  ];
  const score = (type, c) => {
    const dy = c.y - cy, outer = c.k;
    switch (type) {
      case 'market': return Math.abs(dy - ry) + outer * 6;           // vid porten
      case 'barracks': return Math.abs(dy - ry * 0.6) + outer * 4;
      case 'temple': return Math.abs(dy + ry * 0.4) + outer * 5;      // nära borgen
      case 'stable': return -outer * 6 + (c.side > 0 ? 20 : 0);       // västra utkanten
      default: return Math.abs(dy) + (c.side < 0 ? 20 : 0) + outer * 3; // verkstäder österut
    }
  };
  const specials = [];
  for (const b of want) {
    const wide = WIDE.has(b.type);
    let best = null, bs = Infinity;
    for (const c of cells) {
      if (c.used) continue;
      let pair = null;
      if (wide) {
        pair = cells.find(o => !o.used && o.r === c.r && o.side === c.side && o.k === c.k + 1);
        if (!pair) continue;
      }
      const s = score(b.type, c) + (hash(c.x, c.y, 3) % 3);
      if (s < bs) { bs = s; best = [c, pair]; }
    }
    if (!best) continue;
    const [c, pair] = best;
    c.used = true; if (pair) pair.used = true;
    const x = pair ? Math.min(c.x, pair.x) : c.x;
    specials.push({ b, x, y: c.y, w: pair ? BW + SX : BW, h: BH });
  }

  // ── Bostadskvarteren: mellankontrast, ingen kontur ──
  for (const c of cells) {
    if (c.used) continue;
    // Små städer fylls glest: kvarteren närmast mitten först.
    const d = inEll(c.x + BW / 2, c.y + BH / 2, cx, cy, rx, ry);
    if (!hasCitadel && d > 0.6) continue;
    // Oregelbundet: kvarteret krymper, sväller över gränden eller tappar ett hörn,
    // så att planen läser som vuxen stad och inte som rutpapper.
    const h0 = hash(c.x, c.y, 5);
    const nb = cells.find(o => !o.used && o.r === c.r && o.side === c.side && o.k === c.k + (c.side > 0 ? 1 : -1));
    const merge = nb && h0 % 4 === 0;
    let x = c.x + (h0 % 3 === 1 ? 1 : 0), y = c.y + ((h0 >> 3) % 3 === 1 ? 1 : 0);
    let w = BW - (h0 % 3 === 2 ? 2 : 0) - (x - c.x), h = BH - ((h0 >> 3) % 3 === 2 ? 1 : 0) - (y - c.y);
    if (merge) { if (c.side > 0) w += SX - BW; else { x -= SX - BW; w += SX - BW; } }
    rect(g, x + 1, y + 1, w, h, 'W');                   // skugga ner-höger
    rect(g, x, y, w, h, 'M');
    row(g, x, y, w, 'P'); col(g, x, y, h, 'P');
    const seam = 3 + hash(x, y, 5) % 4;
    col(g, x + seam, y + 1, h - 1, 'm');                // husgräns
    row(g, x + seam + 1, y + 3 + hash(x, y, 6) % 2, w - seam - 1, 'm');
    if (hash(x, y, 7) % 3 === 0) rect(g, x + 1 + (hash(x, y, 8) % 2), y + 2, 2, 2, 'e');  // gård
    const cut = (h0 >> 6) % 5;                            // ett avbitet hörn
    if (cut === 0) rect(g, x + w - 3, y + h - 2, 3, 2, 'e');
    if (cut === 1) { rect(g, x, y, 3, 2, 'e'); }
  }
  // Nygrundad: tre sammanhållna block, en jordplätt.
  if (!cells.some(c => !c.used) || pop < 300) {
    for (let y = cy - 7; y < cy + 8; y++) for (let x = cx - 12; x < cx + 13; x++)
      if (inEll(x, y, cx, cy, 12, 7) < 1) set(g, x, y, 'e');
    for (const [dx, dy, w, h] of [[-9, -4, 7, 5], [0, -5, 6, 4], [-3, 1, 8, 4]]) {
      rect(g, cx + dx + 1, cy + dy + 1, w, h, 'W'); rect(g, cx + dx, cy + dy, w, h, 'M');
      row(g, cx + dx, cy + dy, w, 'P'); col(g, cx + dx, cy + dy, h, 'P');
    }
  }

  // ── HÖG kontrast från här: konturerade spelobjekt ──
  const smokes = [];
  for (const [type, x, y] of outsideJobs) inked(g, gg => OUT[type](gg)(x, y));
  for (const s of specials) {
    inked(g, gg => (s.b.phase >= 1 ? PLAN[s.b.type](gg, s.x, s.y, s.w, s.h) : underConstruction(gg, s.x, s.y, s.w, s.h, s.b.phase)));
    if (s.b.type === 'foundry' && s.b.phase >= 1) smokes.push([s.x + 5, s.y + 1]);
  }

  // Borgen: berghäll med tung skugga, megaron med härd.
  let bannerAt = [cx + 6, cy - 6];
  if (cit) {
    const { x, y, w, h } = cit;
    rect(g, x + 2, y + 2, w, h, 'K');                    // borgens slagskugga
    inked(g, gg => {
      rect(gg, x, y, w, h, 'T');
      row(gg, x, y, w, 'P'); col(gg, x, y, h, 'P'); col(gg, x + w - 1, y, h, 'q'); row(gg, x, y + h - 1, w, 'q');
      const mw = 10 + Math.round(4 * t), mh = h - 4, mx = cx - (mw >> 1), my = y + 2;
      rect(gg, mx, my, mw, mh, 'R'); row(gg, mx, my, mw, 'M'); col(gg, mx + mw - 1, my, mh, 'r');
      row(gg, mx, my + mh - 3, mw, 'O');                  // förhallens ockraband
      rect(gg, cx - 2, my + 3, 4, 4, 'O'); rect(gg, cx - 1, my + 4, 2, 2, 'F');   // härden
      if (t > 0.5) {                                       // palatsflyglar
        rect(gg, x + 2, my, mx - x - 4, mh - 2, 'M'); col(gg, mx - 3, my, mh - 2, 'm');
        rect(gg, mx + mw + 2, my, x + w - mx - mw - 4, mh - 2, 'M'); col(gg, x + w - 3, my, mh - 2, 'm');
      }
    });
    smokes.push([cx, y + 3]);
    bannerAt = [x + 2, y + 1];
  }

  // Ringmuren — tjocklek = murnivå, torn från nivå 2, porten tydlig.
  if (walls) {
    const th = walls + 1;
    inked(g, gg => {
      for (let y = cy - ry - th - 2; y <= cy + ry + th + 2; y++) for (let x = cx - rx - th - 2; x <= cx + rx + th + 2; x++) {
        const e = inEll(x, y, cx, cy, rx + 1 + th, ry + 1 + th), ei = inEll(x, y, cx, cy, rx + 1, ry + 1);
        if (e < 1 && ei >= 1) {
          if (y > cy && Math.abs(x - cx) <= 2) continue;
          if (isSea(at(g, x, y))) continue;
          const outerLit = x < cx && y < cy;
          set(gg, x, y, outerLit ? 'P' : (x > cx && y > cy ? 'q' : 'T'));
        }
      }
      if (walls >= 2) {
        const n = walls === 2 ? 6 : 10;
        for (let i = 0; i < n; i++) {
          const a = (i + 0.5) / n * Math.PI * 2 - Math.PI / 2;
          const tx = Math.round(cx + Math.cos(a) * (rx + 1 + th / 2)), ty = Math.round(cy + Math.sin(a) * (ry + 1 + th / 2));
          if (isSea(at(g, tx, ty)) || (ty > cy && Math.abs(tx - cx) < 8)) continue;
          const s = walls + 3;
          rect(gg, tx - (s >> 1), ty - (s >> 1), s, s, 'T');
          row(gg, tx - (s >> 1), ty - (s >> 1), s, 'P'); col(gg, tx + (s >> 1), ty - (s >> 1), s, 'q');
        }
      }
      // Porten: två tornstumpar och en mörk passage.
      const py = cy + ry + 1, s = walls + 3;
      for (const sx of [cx - 3 - s, cx + 4]) { rect(gg, sx, py - 1, s, th + 3, 'T'); row(gg, sx, py - 1, s, 'P'); col(gg, sx + s - 1, py - 1, th + 3, 'q'); }
      rect(gg, cx - 2, py, 5, th + 1, 'V');
    });
  }

  // Hamnen: kaj + bryggor + skepp — en tydlig, separat modul.
  if (coastal) {
    const hl = (sett.buildings.find(b => b.type === 'harbour') || {}).level || 0;
    const qx = cx + Math.round(rx * 0.55);
    let qy = 0;
    for (let y = 0; y < H; y++) if (isSea(at(g, qx, y))) { qy = y; break; }
    if (hl) inked(g, gg => {
      rect(gg, qx - 6, qy - 1, 14 + hl * 8, 3, 'T'); row(gg, qx - 6, qy - 1, 14 + hl * 8, 'P');
      for (let i = 0; i <= hl; i++) rect(gg, qx - 2 + i * 8, qy + 2, 2, 7 + i * 2, 'T');
    });
    const ship = (gg, x, y, L) => {
      row(gg, x + 1, y, L - 2, '<'); row(gg, x, y + 1, L, '<'); row(gg, x + 1, y + 2, L - 2, '<');
      row(gg, x + 2, y + 1, L - 4, 'w'); set(gg, x + L, y + 1, '<'); rect(gg, x + (L >> 1) - 1, y - 1, 3, 5, '>');
    };
    for (let i = 0; i < (hl ? 1 + hl : 1); i++) {
      const sx = qx + 1 + i * 8, sy = qy + 6 + i * 4 + (hl ? 0 : 8);
      if (sy < H - 4 && sx < W - 12) inked(g, gg => ship(gg, sx, sy, 10));
    }
  }

  // Vimpeln — ägaren. Högsta kontrast i bilden.
  inked(g, gg => {
    const [bx, by] = bannerAt;
    col(gg, bx, by - 8, 9, 'W');
    rect(gg, bx + 1, by - 8, 5, 3, 'O'); row(gg, bx + 1, by - 6, 5, 'o'); set(gg, bx + 6, by - 7, 'O');
  });

  // ── Liv: få människor på gatorna, rök ──
  const nPeople = Math.min(36, 2 + Math.round(pop / 800));
  over(g, gg => {
    let n = 0;
    for (let i = 0; i < 4000 && n < nPeople; i++) {
      const x = cx - rx + hash(i, 1, 8) % (2 * rx), y = cy - ry + hash(i, 2, 8) % (2 * ry + 12);
      if (at(g, x, y) !== 'e' || at(g, x + 1, y) !== 'e' || at(g, x - 1, y) !== 'e') continue;
      set(gg, x, y, ['$', '$', 'O', 'z'][hash(i, 3, 8) % 4]); n++;
    }
    smokes.forEach(([x, y], i) => smoke(gg, x, y, 7, i));
  });

  blit(ctx, g);
}
