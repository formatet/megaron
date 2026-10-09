// ── Skiss C7: C6 + volym ──────────────────────────────────────────────────
//
// Sana om C6 (2026-10-09): rätt som informationssystem, för platt som bild.
// "Reduktion betyder färre men starkare former, inte plattare former."
// C7 behåller C6:s läsordning, reduktion och statuslager, och ger tillbaka
// volym: relief i stadsgolvet, halvkontur på bostäderna, fysisk mur, borgen
// på en trappad platå, lugna vågband i havet, stora terrängformer.
//
// Nedan C6:s ursprungliga huvud.
// ── Skiss C6: reduktionsskiss enligt de nya grafikreglerna ────────────────
//
// Regler: myltavault/megaron_grafikregler.md (Timothy/Sana 2026-10-09).
// Sanas sex steg för den här skissen:
//   1. småhusen −60 % (mot C5)      4. terrängen ~40 % lugnare
//   2. starkare centrum, mur, port, vimpel   5. status som eget lager
//   3. byggnadsmoduler med unik silhuett     6. samma fyra städer
//
// Konturhierarki: bläck (K) bara på spelviktiga former — mur, port, borg,
// vimpel, skepp, funktionsbyggnader, statusobjekt. Bostadsmassan får valör,
// skugga och rundhet men ingen kontur. Kamera: strategisk top-down med lätt
// snedställd volym (södra fasader syns), inte teknisk planritning.
//
// Lager, i ritordning: mark · stadsmassa · mur/port · borg · moduler · hamn ·
// vimpel · liv · STATUS (statusLayer, eget lager ovanpå allt).

import { rect, row, col } from '../pixelgrid.js';
import { W, H, newGrid, set, at, hash, inked, over, blit, smoke, GROUND } from './common.js';
import { PLAN, WIDE, underConstruction } from './c2_plan.js';

const inEll = (x, y, cx, cy, rx, ry) => ((x - cx) / rx) ** 2 + ((y - cy) / ry) ** 2;
const isSea = ch => ch >= '0' && ch <= '3';
const BW = 10, BH = 6, FH = 2, SX = 13, SY = BH + FH + 3;

// Moduler i prioritetsordning. Högst fem visas; aktivt bygge går alltid först.
const PRIORITY = ['temple', 'market', 'barracks', 'foundry', 'silver_mine', 'mine', 'stable', 'olive_press', 'winery', 'stonequarry', 'lumbermill'];
const MAX_MODULES = 5;

// Folk i band, inte linjärt (grafikreglerna, "Defaults").
const peopleBand = pop => pop < 300 ? 1 : pop < 3000 ? 3 : pop < 10000 ? 5 : 8;

const OUT = {
  mine: (gg, x, y) => { rect(gg, x, y, 9, 6, '8'); rect(gg, x + 2, y + 1, 3, 3, 'V'); rect(gg, x + 6, y + 3, 4, 3, 'q'); },
  silver_mine: (gg, x, y) => { rect(gg, x, y, 9, 6, '8'); rect(gg, x + 2, y + 1, 3, 3, 'V'); rect(gg, x + 6, y + 3, 4, 3, 'S'); },
  stonequarry: (gg, x, y) => { for (let i = 0; i < 3; i++) rect(gg, x + i * 2, y + i * 2, 10 - i * 2, 2, i % 2 ? 'T' : 'P'); },
  lumbermill: (gg, x, y) => { rect(gg, x, y, 5, 5, 'R'); col(gg, x + 4, y, 5, 'r'); for (let i = 0; i < 3; i++) row(gg, x + 6, y + i * 2, 5, 'w'); },
};

// En bostadsmassa: tak med ljus vänster/överkant och skuggad högerkant,
// avrundade takhörn, en kort fasad i puts. Ingen kontur — valören bär formen.
function mass(g, x, y, w, h, seed) {
  row(g, x + 2, y + h + FH + 1, w, 'J'); col(g, x + w + 1, y + 2, h + FH, 'J');   // mjuk slagskugga
  row(g, x + 1, y + h + FH, w, 'r'); col(g, x + w, y + 1, h + FH, 'r');           // halvkontur nedre/höger
  rect(g, x, y, w, h, 'M');
  row(g, x + 1, y, w - 2, 'P'); col(g, x, y + 1, h - 1, 'P');
  col(g, x + w - 1, y + 1, h - 1, 'm');
  set(g, x, y, at(g, x - 1, y)); set(g, x + w - 1, y, at(g, x + w, y));   // rundade hörn
  if (w > 7) col(g, x + 3 + seed % (w - 6), y + 1, h - 1, 'm');           // en husgräns, inte fem
  row(g, x, y + h - 1, w, 'R');                                             // takkant: tak ≠ vägg
  rect(g, x, y + h, w, FH, 'p'); col(g, x + w - 1, y + h, FH, 'd');
  if (seed % 3 === 0) set(g, x + 2 + seed % (w - 3), y + h + 1, 'd');      // dörr, nedtonad
}

export const render = (ctx, scene) => draw(ctx, scene, null);

export function draw(ctx, scene, status) {
  const { sett, terrain, coastal } = scene;
  const pop = sett.population, walls = sett.walls;
  const t = Math.max(0, Math.min(1, Math.log10(pop / 100) / Math.log10(280)));
  const g = newGrid(W, H);
  const [gb, gd] = GROUND[terrain] || GROUND.plains;
  for (let y = 0; y < H; y++) for (let x = 0; x < W; x++) set(g, x, y, hash(x, y, 31) % 110 === 0 ? gd : gb);

  // ── Bakgrund: lugnt hav, inget glitter ──
  if (coastal) for (let y = 0; y < H; y++) for (let x = 0; x < W; x++) {
    const d = x * 0.55 + y - 150 + 4 * Math.sin(x / 9) + 2 * Math.sin(y / 5);
    if (d < 0) continue;
    let ch = d < 1.2 ? '3' : d < 4 ? '2' : d < 20 ? '1' : '0';
    // Lugn rörelse: några långa vågband, inget glitter.
    const band = (d + 2.5 * Math.sin(x / 17)) % 9;
    if (d > 6 && band > 4 && band < 5 && Math.sin(x / 11 + d / 5) > -0.1) ch = ch === '1' ? 'Q' : 'U';
    set(g, x, y, ch);
  }

  const cx = coastal ? 90 : 105, cy = coastal ? 56 : 62;
  const rx = Math.round(12 + 50 * t), ry = Math.round(8 + 34 * t);
  const taken = [];
  const free = (x, y, w, h) => !taken.some(([a, b, c, d]) => x < a + c && a < x + w && y < b + d && b < y + h);

  // Fält: två toner, ingen fåra.
  const farmLvl = (sett.buildings.find(b => b.type === 'farm') || {}).level || 0;
  // Lågkontrast jordband — en sluttning, inte brus.
  for (let y = 0; y < H; y++) for (let x = 0; x < W; x++)
    if (!isSea(at(g, x, y)) && Math.sin((x + 2 * y) / 29) > 0.75 && hash(x, y, 19) % 5 === 0) set(g, x, y, gd);
  const fields = [[4, 6], [156, 6], [4, 88], [36, 6]];
  for (let i = 0; i < Math.min(3, Math.max(1, farmLvl + 1)); i++) {
    const [fx, fy] = fields[i]; const fw = 32, fh = 18;
    if (inEll(fx + fw / 2, fy + fh / 2, cx, cy, rx + 16, ry + 12) < 1) continue;
    for (let y = 0; y < fh; y++) for (let x = 0; x < fw; x++)
      if (!isSea(at(g, fx + x, fy + y))) set(g, fx + x, fy + y, ((i % 2 ? x : y) % 4 === 0) ? '_' : '-');
    taken.push([fx, fy, fw, fh]);
  }
  // Träd: färre, i dungar, ingen kontur.
  const grove = (x, y) => Math.sin(x / 11 + 0.7) + Math.sin(y / 8 + x / 27) + Math.sin((x - y) / 17);
  // Träd i kluster: tätt där dungen är, ingenting annars. Dungen delar skugga.
  for (let i = 0; i < 1400; i++) {
    const x = 3 + hash(i, 3, 43) % (W - 6), y = 3 + hash(i, 4, 43) % (H - 6);
    if (grove(x, y) < 2.0 || inEll(x, y, cx, cy, rx + 8, ry + 8) < 1 || at(g, x, y) === '+' || at(g, x, y) === '#') continue;
    if (isSea(at(g, x, y)) || isSea(at(g, x + 2, y + 2)) || !free(x - 2, y - 2, 4, 4)) continue;
    set(g, x + 2, y + 1, '5'); row(g, x, y + 2, 3, '5');
    row(g, x - 1, y - 1, 2, '*'); row(g, x - 1, y, 3, '+'); row(g, x, y + 1, 2, '#');
  }

  // ── Vilka moduler visas: aktivt bygge först, sedan prioritet, högst fem ──
  const built = sett.buildings.filter(b => PLAN[b.type] || OUT[b.type])
    .sort((a, b) => PRIORITY.indexOf(a.type) - PRIORITY.indexOf(b.type));
  const queued = sett.build_queue.filter(b => PLAN[b.type] || OUT[b.type]);
  const modules = [...queued, ...built.map(b => ({ ...b, phase: 1 }))].slice(0, MAX_MODULES);

  // Verk utanför muren.
  const outsideJobs = [];
  let oseed = 5;
  for (const b of modules.filter(b => OUT[b.type])) {
    for (let tries = 0; tries < 300; tries++, oseed++) {
      const x = 3 + hash(oseed, 1, 3) % (W - 16), y = 3 + hash(oseed, 2, 3) % (H - 10);
      if (inEll(x + 6, y + 3, cx, cy, rx + 12, ry + 10) < 1 || !free(x - 1, y - 1, 14, 9)) continue;
      if (isSea(at(g, x, y)) || isSea(at(g, x + 12, y + 7))) continue;
      taken.push([x - 1, y - 1, 14, 9]); outsideJobs.push([b.type, x, y]); break;
    }
  }

  // ── Struktur: vägar, gatumark ──
  const gateY = cy + ry + 2;
  for (let y = gateY; y < H; y++) { const x = cx + Math.round(Math.sin((y - gateY) / 8) * 2); if (isSea(at(g, x, y))) break; row(g, x - 1, y, 3, 'e'); }
  for (let x = 0; x < cx - rx; x++) set(g, x, cy + 4 + Math.round(3 * Math.sin(x / 14)), 'e');
  for (let y = 0; y < H; y++) for (let x = 0; x < W; x++)
    if (inEll(x, y, cx, cy, rx + 1, ry + 1) < 1 && !isSea(at(g, x, y))) {
      // 2–3 stora valörfält, murens skugga på insidan, ingen prickighet.
      const field = Math.sin(x / 23 + 1.3) + Math.sin(y / 13 + x / 41);
      const shade = walls && inEll(x - 2, y - 3, cx, cy, rx + 1, ry + 1) >= 1;
      set(g, x, y, shade ? 'e' : field > 0.9 ? 'J' : hash(x, y, 2) % 41 ? 'E' : 'e');
    }
  // Sliten jord: kring porten och upp längs huvudgatan.
  for (let y = cy - ry; y <= gateY + 12; y++) for (let x = cx - 12; x <= cx + 12; x++) {
    const near = ((x - cx) / 10) ** 2 + ((y - gateY + 2) / 6) ** 2 < 1 || (Math.abs(x - cx) <= 1 && y > cy - ry / 2);
    if (near && !isSea(at(g, x, y)) && (at(g, x, y) === 'E' || at(g, x, y) === 'J' || (y >= gateY && (x + y) % 2))) set(g, x, y, 'j');
  }

  // Kvartersceller.
  const cells = [];
  for (let r = 0; r * SY < 2 * ry; r++) {
    const y = cy - ry + 1 + r * SY;
    for (const side of [-1, 1]) for (let k = 0; k < 12; k++) {
      const x = side > 0 ? cx + 3 + k * SX : cx - 2 - BW - k * SX;
      const ok = [[x, y], [x + BW, y], [x, y + BH + FH], [x + BW, y + BH + FH]]
        .every(([a, b]) => inEll(a, b, cx, cy, rx, ry) < 0.92 && !isSea(at(g, a, b)) && !isSea(at(g, a + 3, b + 3)));
      if (ok) cells.push({ x, y, r, side, k, used: false });
    }
  }
  // Borgen: större, tyngre — centrum ska läsas direkt efter muren.
  const hasCitadel = pop >= 1000;
  const cit = hasCitadel ? { w: Math.round(24 + 10 * t), h: Math.round(13 + 3 * t) } : null;
  if (cit) {
    cit.x = cx - (cit.w >> 1); cit.y = cy - ry + 1;
    for (const c of cells) if (c.x < cit.x + cit.w + 6 && c.x + BW > cit.x - 6 && c.y < cit.y + cit.h + 12) c.used = true;
  }
  // Moduler innanför: var och en på sin default-plats.
  const score = (type, c) => {
    const dy = c.y - cy, outer = c.k;
    switch (type) {
      case 'market': return Math.abs(dy - ry) + outer * 6;
      case 'barracks': return Math.abs(dy - ry * 0.6) + outer * 4;
      case 'temple': return Math.abs(dy + ry * 0.4) + outer * 5;
      case 'stable': return -outer * 6 + (c.side > 0 ? 20 : 0);
      default: return Math.abs(dy) + (c.side < 0 ? 20 : 0) + outer * 3;
    }
  };
  const specials = [];
  for (const b of modules.filter(b => PLAN[b.type])) {
    const wide = WIDE.has(b.type);
    let best = null, bs = Infinity;
    for (const c of cells) {
      if (c.used) continue;
      const pair = wide ? cells.find(o => !o.used && o.r === c.r && o.side === c.side && o.k === c.k + 1) : null;
      if (wide && !pair) continue;
      const s = score(b.type, c) + (hash(c.x, c.y, 3) % 3);
      if (s < bs) { bs = s; best = [c, pair]; }
    }
    if (!best) continue;
    const [c, pair] = best;
    c.used = true; if (pair) pair.used = true;
    specials.push({ b, x: pair ? Math.min(c.x, pair.x) : c.x, y: c.y, w: pair ? BW + SX : BW, h: BH });
  }

  // ── Stadsmassan: få, stora, sammanhängande massor. Var tredje cell blir gård
  // eller trädgård, och massorna smälter oftare ihop — så ~60 % färre hus än C5.
  const blocks = [];
  for (const c of cells) {
    if (c.used) continue;
    const d = inEll(c.x + BW / 2, c.y + BH / 2, cx, cy, rx, ry);
    if (!hasCitadel && d > 0.6) continue;
    const h0 = hash(c.x, c.y, 5);
    if (h0 % (t > 0.8 ? 5 : 3) === 0) {                    // öppen gård med ett olivträd (tätare i storstaden)
      if (h0 % 2) { row(g, c.x + 3, c.y + 2, 2, '+'); row(g, c.x + 2, c.y + 3, 3, '+'); row(g, c.x + 3, c.y + 4, 2, '#'); }
      continue;
    }
    const nb = cells.find(o => !o.used && o.r === c.r && o.side === c.side && o.k === c.k + (c.side > 0 ? 1 : -1));
    let x = c.x, w = BW;
    if (nb && h0 % 2 === 0) { nb.used = true; if (c.side < 0) x -= SX - BW; w += SX - BW + (h0 % 4 === 0 ? 0 : -2); }
    const h = BH - (h0 >> 4) % 2;
    blocks.push([x, c.y + (BH - h), w, h, h0]);
  }
  for (const s of specials) if (s.b.type === 'market')        // torget: sliten ljus jord runt
    for (let y = s.y - 2; y < s.y + s.h + 4; y++) for (let x = s.x - 2; x < s.x + s.w + 2; x++) if (at(g, x, y) === 'E' || at(g, x, y) === 'J') set(g, x, y, 'j');
  for (const [x, y, w, h, s] of blocks) mass(g, x, y, w, h, s);
  // Nygrundad: en jordplätt, två massor, en gemensam kontur (stadens silhuett).
  if (pop < 300) {
    for (let y = cy - 7; y < cy + 8; y++) for (let x = cx - 12; x < cx + 13; x++)
      if (inEll(x, y, cx, cy, 12, 7) < 1) set(g, x, y, 'E');
    for (let y = cy + 3; y < cy + 10; y++) for (let x = cx - 3; x < cx + 4; x++) if ((x + y) % 2 || y > cy + 5) set(g, x, y, 'j');   // där vägen möter byn
    rect(g, cx - 6, cy + 3, 16, 2, 'J');                                                   // gruppens skugga
    inked(g, gg => { for (const [dx, dy, w, h] of [[-8, -4, 8, 4], [1, -2, 7, 4]]) mass(gg, cx + dx, cy + dy, w, h, dx & 7); });
  }

  // ── Signal och struktur med bläck ──
  const smokes = pop < 300 ? [[cx - 4, cy - 5]] : [];
  for (const [type, x, y] of outsideJobs) inked(g, gg => OUT[type](gg, x, y));
  for (const s of specials) {
    inked(g, gg => {
      if (s.b.phase >= 1) PLAN[s.b.type](gg, s.x, s.y, s.w, s.h); else underConstruction(gg, s.x, s.y, s.w, s.h, s.b.phase);
      if (s.b.phase < 1 || s.b.type === 'market' || s.b.type === 'stable') return;
      if (s.b.type === 'temple') { rect(gg, s.x, s.y + s.h, s.w, FH, 'd'); for (let i = 0; i < s.w; i += 2) col(gg, s.x + i, s.y + s.h, FH, 'P'); }
      else { rect(gg, s.x, s.y + s.h, s.w, FH, 'p'); col(gg, s.x + s.w - 1, s.y + s.h, FH, 'd'); }
    });
    if (s.b.type === 'foundry' && s.b.phase >= 1) smokes.push([s.x + 5, s.y + 1]);
    if (s.b.phase < 1) over(g, gg => {                    // arbetare vid bygget
      for (const [dx, dy] of [[-2, 2], [s.w + 1, 4]]) { set(gg, s.x + dx, s.y + dy, '@'); set(gg, s.x + dx, s.y + dy + 1, '$'); }
      set(gg, s.x - 1, s.y + 3, 'w');
    });
  }

  // Borgen: ljus sten, tung slagskugga, hög klippfot, rött megaron.
  let bannerAt = [cx + 8, cy - 4];
  if (cit) {
    const { x, y, w, h } = cit;
    // Platån: en lägre, bredare terrass som knyter borgen till staden.
    const px = x - 4, pw = w + 8, py = y + 4, ph = h + 2;
    rect(g, px + 4, py + 3, pw, ph + 4, 'e');               // bred skugga nedre/höger
    rect(g, px + pw, py + 2, 2, ph + 2, 'G');
    inked(g, gg => {
      rect(gg, px, py, pw, ph, 'q'); row(gg, px, py, pw, 'T'); col(gg, px, py, ph, 'T');
      rect(gg, px, py + ph, pw, 3, 't');                    // platåns fot
      for (let i = 0; i < pw; i += 6) col(gg, px + i + (i % 12 ? 2 : 0), py + ph, 3, 'q');   // blockskift
      for (const [ax, ay] of [[px, py + ph + 2], [px + pw - 2, py + ph + 2]]) { rect(gg, ax, ay - 2, 2, 2, '.'); set(gg, ax + (ax === px ? 1 : 0), ay - 3, '.'); }   // mindre perfekt fot
      for (const sx of [px + 3, px + pw - 5]) { rect(gg, sx, py + ph + 3, 2, 2, 'T'); col(gg, sx + 1, py + ph + 3, 2, 'q'); }   // murarmar ned mot staden
      for (let k = 0; k < 6; k++) row(gg, cx - 2, y + h + 4 + k, 5, k % 2 ? 'q' : 'P');   // trappan ned till gatan
    });
    inked(g, gg => {
      rect(gg, x, y, w, h, 'T');
      row(gg, x, y, w, 'P'); col(gg, x, y, h, 'P'); col(gg, x + w - 1, y, h, 'q');
      rect(gg, x, y + h, w, 4, 'q');                       // klippfoten, ditherad
      for (let i = 0; i < w; i++) for (let k = 0; k < 4; k++) if ((i + k) % 3 === 0) set(gg, x + i, y + h + k, 't');
      const mw = 10 + Math.round(4 * t), mh = h - 4, mx = cx - (mw >> 1), my = y + 2;
      rect(gg, mx, my, mw, mh, 'R'); row(gg, mx, my, mw, 'M'); col(gg, mx + mw - 1, my, mh, 'r');
      row(gg, mx, my + mh - 3, mw, 'O'); row(gg, mx, my + mh - 2, mw, 'o');
      rect(gg, cx - 2, my + 3, 4, 4, 'O'); rect(gg, cx - 1, my + 4, 2, 2, 'F');
      if (t > 0.5) {
        rect(gg, x + 2, my, mx - x - 4, mh - 2, 'p'); col(gg, mx - 3, my, mh - 2, 'd');
        rect(gg, mx + mw + 2, my, x + w - mx - mw - 4, mh - 2, 'p'); col(gg, x + w - 3, my, mh - 2, 'd');
      }
    });
    smokes.push([cx, y + 3]);
    bannerAt = [x + 3, y + 1];
  }

  // Muren: sval sten, synlig yttersida, torn från nivå 2, tinnar på nivå 3.
  const gate = { x: cx, y: cy + ry + 1 };
  if (walls) for (const sx of [cx - 4 - (walls + 4), cx + 4]) rect(g, sx + walls + 4, gate.y - 6, 2, walls + 9, 'G');   // porttornens skugga
  if (walls) {
    const th = walls + 1, face = walls + 1;
    inked(g, gg => {
      for (let y = cy - ry - th - 2; y <= cy + ry + th + 2; y++) for (let x = cx - rx - th - 2; x <= cx + rx + th + 2; x++) {
        const e = inEll(x, y, cx, cy, rx + 1 + th, ry + 1 + th), ei = inEll(x, y, cx, cy, rx + 1, ry + 1);
        if (e < 1 && ei >= 1 && !(y > cy && Math.abs(x - cx) <= 3) && !isSea(at(g, x, y))) {
          // Ljus ovansida (inre kanten), mörk yttersida åt söder/öster, några blockskift.
          const inner = inEll(x - Math.sign(x - cx), y - Math.sign(y - cy), cx, cy, rx + 1, ry + 1) < 1;
          const outer = inEll(x + Math.sign(x - cx), y + Math.sign(y - cy), cx, cy, rx + 1 + th, ry + 1 + th) >= 1;
          const a = Math.atan2((y - cy) / ry, (x - cx) / rx);
          const shift = Math.floor((a + 4) * 3.2) % 4 === 0;
          set(gg, x, y, inner ? 'P' : outer && (x > cx || y > cy) ? 'q' : shift ? 'q' : 'T');
        }
      }
      // Yttersidan mot söder, ditherad sten.
      const edge = [];
      for (let y = cy; y <= cy + ry + th + 2; y++) for (let x = cx - rx - th - 2; x <= cx + rx + th + 2; x++)
        if (at(gg, x, y) !== '.' && at(gg, x, y + 1) === '.' && inEll(x, y + 1, cx, cy, rx + 1, ry + 1) >= 1) edge.push([x, y]);
      for (const [x, y] of edge) for (let k = 1; k <= face; k++) if (!isSea(at(g, x, y + k))) set(gg, x, y + k, (x + k) % 3 === 0 ? 't' : 'q');
      for (let y = cy - ry; y <= cy + ry; y++) for (let x = cx; x <= cx + rx + th + 2; x++)       // östra yttersidan
        if (at(gg, x, y) !== '.' && at(gg, x + 1, y) === '.' && inEll(x + 1, y, cx, cy, rx + 1, ry + 1) >= 1 && !isSea(at(g, x + 1, y))) { set(gg, x + 1, y, 't'); break; }
      if (walls >= 3) for (let x = cx - rx - th; x <= cx + rx + th; x += 2) {   // tinnar på norra krönet
        for (let y = cy - ry - th - 2; y < cy; y++) if (at(gg, x, y) !== '.') { set(gg, x, y, 'q'); break; }
      }
      if (walls >= 2) {
        const n = walls === 2 ? 6 : 10;
        for (let i = 0; i < n; i++) {
          const a = (i + 0.5) / n * Math.PI * 2 - Math.PI / 2;
          const tx = Math.round(cx + Math.cos(a) * (rx + 1 + th / 2)), ty = Math.round(cy + Math.sin(a) * (ry + 1 + th / 2));
          if (isSea(at(g, tx, ty)) || (ty > cy && Math.abs(tx - cx) < 9)) continue;
          const s = walls + 4;
          rect(gg, tx - (s >> 1), ty - (s >> 1) - 2, s, s, 'T');
          row(gg, tx - (s >> 1), ty - (s >> 1) - 2, s, 'P'); col(gg, tx + (s >> 1), ty - (s >> 1) - 2, s, 'q');
          rect(gg, tx - (s >> 1), ty + (s >> 1) - 1, s, 3, 't');
        }
      }
      // Porten: två höga torn och en djup passage.
      const s = walls + 4;
      for (const sx of [cx - 4 - s, cx + 4]) {
        rect(gg, sx, gate.y - 3, s, th + 3, 'T'); row(gg, sx, gate.y - 3, s, 'P'); col(gg, sx + s - 1, gate.y - 3, th + 3, 'q');
        rect(gg, sx, gate.y + th, s, 3, 't');
      }
      rect(gg, cx - 3, gate.y, 7, th + 2, 'V');
    });
  }

  // Hamnen: kaj, bryggor, skepp med kontur.
  if (coastal) {
    const hl = (sett.buildings.find(b => b.type === 'harbour') || {}).level || 0;
    const qx = cx + Math.round(rx * 0.55);
    let qy = 0;
    for (let y = 0; y < H; y++) if (isSea(at(g, qx, y))) { qy = y; break; }
    if (hl) inked(g, gg => {
      rect(gg, qx - 6, qy - 1, 14 + hl * 8, 3, 'W'); row(gg, qx - 6, qy - 1, 14 + hl * 8, 'w');
      for (let i = 0; i <= hl; i++) rect(gg, qx - 2 + i * 8, qy + 2, 2, 7 + i * 2, 'W');
    });
    const ship = (gg, x, y, L) => {
      row(gg, x + 1, y, L - 2, '<'); row(gg, x, y + 1, L, 'w'); row(gg, x + 1, y + 2, L - 2, '<');
      set(gg, x + L, y + 1, '<');
      for (let i = 2; i < L - 2; i += 2) { set(gg, x + i, y - 1, 'W'); set(gg, x + i, y + 3, 'W'); }
    };
    for (let i = 0; i < (hl ? 1 + hl : 1); i++) {
      const sx = qx + 2 + i * 10, sy = qy + 6 + (i % 2) * 7 + i * 2 + (hl ? 0 : 6);
      if (sy < H - 4 && sx < W - 14) {
        const L = 10 + (i % 2) * 3;
        row(g, sx + 2, sy + 4, L, 'U');                       // skuggan i vattnet
        inked(g, gg => ship(gg, sx, sy, L));
        set(g, sx - 2, sy + 1, '3'); set(g, sx - 3, sy + 2, '3'); set(g, sx + L + 2, sy + 1, '3');   // skum vid stäv och akter
      }
    }
    // Skum kring bryggorna.
    for (let y = qy - 2; y < qy + 16; y++) for (let x = qx - 8; x < qx + 14 + hl * 8; x++)
      if (isSea(at(g, x, y)) && at(g, x - 1, y) === 'K' && hash(x, y, 13) % 2) set(g, x, y, '3');
  }

  // Vimpeln: högre stång, kluven flagga, en glyf. Ockra = neutral default.
  inked(g, gg => {
    const [bx, by] = bannerAt;
    col(gg, bx, by - 11, 12, 'W');
    rect(gg, bx + 1, by - 11, 7, 5, 'O'); row(gg, bx + 1, by - 7, 7, 'o');
    set(gg, bx + 7, by - 9, '.');                          // kluven flik
    set(gg, bx + 3, by - 10, 'P'); set(gg, bx + 4, by - 9, 'P'); set(gg, bx + 3, by - 8, 'P');   // glyf
  });

  // ── Liv: folk i band, rök ──
  const geo = { cx, cy, rx, ry, gate, walls, blocks, cit, specials, coastal };
  const kinds = [].concat(status || []);
  const starving = kinds.includes('svält');
  over(g, gg => {
    const nPeople = starving ? 0 : peopleBand(pop);
    let n = 0;
    for (let i = 0; i < 4000 && n < nPeople; i++) {
      const x = cx - 10 + hash(i, 1, 8) % 20, y = gate.y - 8 + hash(i, 2, 8) % 16;
      if (at(g, x, y) !== 'E' && at(g, x, y) !== 'e') continue;
      set(gg, x, y, ['$', 'O', 'z'][n % 3]); if (pop >= 10000) set(gg, x + 1, y, '$');
      n++;
    }
    if (!starving) smokes.forEach(([x, y], i) => smoke(gg, x, y, 7, i));
  });

  for (const k of kinds) statusLayer(g, geo, k);
  blit(ctx, g);
}

// ── STATUSLAGRET — ritas ovanpå grundstaden, aldrig inbakat i den ──
export function statusLayer(g, geo, kind) {
  const { cx, cy, rx, ry, gate, blocks, specials, cit } = geo;
  if (kind === 'svält') {
    // Kallare ton: åkrarna blir bar jord, gatorna tomma, en tunn rökstrimma.
    for (let i = 0; i < g.px.length; i++) if (g.px[i] === '-' || g.px[i] === '_') g.px[i] = i % 7 ? '8' : '7';
    over(g, gg => smoke(gg, cx + 2, cy - 2, 4, 9));
  }
  if (kind === 'bygge') {
    // Signalgul markör över det aktiva bygget.
    const s = specials.find(s => s.b.phase < 1);
    if (s) inked(g, gg => { const mx = s.x + (s.w >> 1); col(gg, mx, s.y - 7, 4, 'Y'); set(gg, mx, s.y - 2, 'Y'); });
  }
  if (kind === 'brand') {
    // Tre kvarter brinner: eld, sot, svart rök.
    const burning = blocks.filter((b, i) => i % 5 === 1).slice(0, 3);
    for (const [x, y, w] of burning) {
      for (let i = 0; i < w; i += 2) set(g, x + i, y + 1 + (i % 3), 'G');
      inked(g, gg => {
        rect(gg, x + 1, y - 1, 5, 3, 'F'); rect(gg, x + 2, y - 3, 3, 2, 'F'); set(gg, x + 3, y - 4, 'F');
        row(gg, x + 2, y, 3, 'Y'); set(gg, x + 3, y - 1, 'Y'); row(gg, x + 1, y + 1, 5, 'f');
      });
      over(g, gg => { for (let k = 0; k < 12; k++) { const sx = x + 3 + Math.round(Math.sin(k / 2) * 1.5); set(gg, sx, y - 5 - k, k % 3 ? 'G' : '%'); if (k > 3) set(gg, sx + 1, y - 5 - k, 'G'); } });
    }
  }
  if (kind === 'offer') {
    // Offerrök: eld på altaret framför templet, en tjock rökpelare som står högre än härdarnas.
    const s = specials.find(s => s.b.type === 'temple' && s.b.phase >= 1);
    if (s) {
      const ax = s.x + (s.w >> 1) - 1, ay = s.y + s.h + FH + 2;
      inked(g, gg => { rect(gg, ax, ay, 3, 2, 'T'); row(gg, ax, ay + 1, 3, 'q'); set(gg, ax + 1, ay - 1, 'F'); set(gg, ax, ay - 1, 'f'); set(gg, ax + 2, ay - 1, 'f'); });
      over(g, gg => {
        set(gg, ax - 2, ay + 1, '@'); set(gg, ax - 2, ay + 2, '$');      // prästen vid altaret
        for (let k = 0; k < 24; k++) {                     // pelaren vidgas och glesnar uppåt
          const sx = ax + Math.round(k / 4 + Math.sin(k / 3)), w = 2 + (k >> 3);
          for (let i = 0; i < w; i++) {
            if (k > 12 && (i + k) % 2) continue;
            set(gg, sx + i, ay - 2 - k, k < 10 ? '%' : '&');
          }
        }
      });
    }
  }
  if (kind === 'belägrad') {
    // Stängd port, damm runt muren, fiendeläger med blodröd fana utanför.
    inked(g, gg => { rect(gg, cx - 3, gate.y, 7, 3, 'W'); row(gg, cx - 3, gate.y + 1, 7, 'w'); });
    for (let i = 0; i < 900; i++) {
      const a = (hash(i, 1, 77) % 628) / 100, r = 1.06 + (hash(i, 2, 77) % 26) / 100;
      const x = Math.round(cx + Math.cos(a) * (rx + 4) * r), y = Math.round(cy + Math.sin(a) * (ry + 4) * r);
      if (at(g, x, y) >= '4' && at(g, x, y) <= '8' && (x + y) % 2) set(g, x, y, '7');
    }
    const camp = [[cx - rx - 14, cy + ry - 2], [cx - 26, cy + ry + 12], [cx - rx + 2, cy + ry + 10],
      [cx - rx - 16, cy + 4], [cx - 14, cy + ry + 16], [cx - rx - 8, cy + ry + 8]];
    for (const [x, y] of camp) {
      if (isSea(at(g, x, y)) || x < 2 || y > H - 6) continue;
      inked(g, gg => { row(gg, x + 1, y, 2, 'P'); row(gg, x, y + 1, 4, 'P'); row(gg, x, y + 2, 4, 'p'); set(gg, x + 1, y + 2, 'V'); });
    }
    const [fx, fy] = camp[0];
    inked(g, gg => { col(gg, fx + 6, fy - 9, 11, 'W'); rect(gg, fx + 7, fy - 9, 7, 5, 'X'); row(gg, fx + 7, fy - 5, 7, 'x'); });
  }
}
