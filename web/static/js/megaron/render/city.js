import { serverNow } from '../clock.js';
import {
  CITY_PALETTE, newGrid, set, row, col, rect, cube, outline, toRuns,
} from './pixelgrid.js';
import { stampBuilding, stampUnderConstruction, buildingWidth, houseDetails } from './citybuildings.js';

// Scenen ritas i LOGISKA pixlar och skalas upp med heltal — samma pixeldisciplin
// som kartan (princip 10: aldrig arc(), aldrig halva pixlar).
const SCENE_W = 224, SCENE_H = 170, S = 2;
export const SCENE_CANVAS = { w: SCENE_W * S, h: SCENE_H * S };

// Citadellet och två gatuled ritas bakifrån och fram. Den sammanhängande
// gården binder ihop dem utan att lägga markband framför byggnadernas fötter.
const CITADEL = 45;         // citadellets fot: megaron och den lägre staden
const RAMPART = 48;         // murens krön, vid terrassens kant
const ROW_BACK = 86;        // bortre gatuledet

// Ordningen byggnaderna radas upp i. Den är INTE en prioritering (allt ritas)
// utan en gruppering: försörjning, hantverk, makt. En stad ska läsa likadant
// varje gång man öppnar den, annars kan man inte se vad som är nytt.
const STREET_ORDER = [
  'farm', 'olive_press', 'winery', 'market', 'harbour',
  'lumbermill', 'stonequarry', 'mine', 'silver_mine', 'foundry',
  'barracks', 'stable', 'temple',
];

let _animId = null;
export function stopCityAnim() {
  if (_animId !== null) { cancelAnimationFrame(_animId); _animId = null; }
}

/**
 * @param sett  provinsens settlement-payload (population, walls, buildings,
 *              build_queue). Scenen läser bara serverns sanning — den hittar
 *              aldrig på en byggnad som inte finns (princip 9).
 */
export function startCityAnim(canvas, tile, buildings, buildQueue, sett) {
  stopCityAnim();
  if (!canvas) return;
  canvas.width = SCENE_CANVAS.w;
  // A long queue may require more terraces; never silently clip its buildings.
  const n = (buildings || []).filter(b => STREET_ORDER.includes(b.type)).length + (buildQueue || []).filter(b => STREET_ORDER.includes(b.type)).length;
  canvas.height = (SCENE_H + Math.max(0, Math.ceil(n / 4) - 3) * 36) * S;
  const ctx = canvas.getContext('2d');
  ctx.imageSmoothingEnabled = false;

  const walker = { x: 20, dir: 1, t: 0, frame: 0 };
  const smoke = [];
  smokeClock.t = 0;
  let last = 0;

  function tick(ts) {
    const dt = last ? Math.min((ts - last) / 1000, 0.1) : 0.016;
    last = ts;
    walker.t += dt;
    if (walker.t > 0.28) { walker.frame ^= 1; walker.t = 0; }
    walker.x += walker.dir * 9 * dt;
    if (walker.x > SCENE_W - 12 || walker.x < 8) walker.dir = -walker.dir;
    for (let i = smoke.length - 1; i >= 0; i--) {
      smoke[i].y -= dt * 3.5; smoke[i].age += dt;
      if (smoke[i].age > 2.2) smoke.splice(i, 1);
    }
    drawScene(ctx, tile, buildings, buildQueue, sett, walker, smoke, dt);
    _animId = requestAnimationFrame(tick);
  }
  _animId = requestAnimationFrame(tick);
}

// Bakgrundsmarken tas ur hexens terräng, dämpad. Staden ska sitta i sitt
// landskap: en hamnstad och en bergsstad får inte ha samma fond.
const GROUND_BY_TERRAIN = {
  plains: '#7A8642', river_valley: '#657E3E', river_delta: '#87934F',
  hills: '#A08A52', mountain_limestone: '#B8AC90', mountain_red: '#A87A5A',
  forest_olive_grove: '#8A8E55', forest_cedar: '#4A5738',
  scrub_maquis: '#8F975D', semi_desert: '#B89E66',
  // Vatten kan aldrig bära en stad, men nyckeln måste finnas: `river` är sedan
  // 2026-07-29 en terräng en provins kan ligga INTILL, och en saknad nyckel
  // hade gett stadsvyn en tyst `undefined` som fond.
  coastal_sea: '#7A8642', deep_sea: '#7A8642', river: '#7A8642',
};

const smokeClock = { t: 0 };

function drawScene(ctx, tile, buildings, buildQueue, sett, walker, smoke, dt) {
  const terrain = tile?.terrain || 'plains';
  ctx.fillStyle = '#C6BC9E';                       // dis: himlen bakom citadellet
  ctx.fillRect(0, 0, SCENE_CANVAS.w, ctx.canvas.height);

  const height = ctx.canvas.height / S;
  const g = newGrid(SCENE_W, height);
  const pop = sett?.population || 0;
  const walls = Math.max(0, Math.min(3, sett?.walls || 0));

  homes(g, pop, (buildings || []).length + (buildQueue || []).length);
  citadel(g, pop, walls);
  const smokeVents = street(g, buildings, buildQueue);
  if (pop >= 1000) smokeVents.push([SCENE_W / 2 + 2, CITADEL - 30]);
  garden(g);
  outline(g);
  const sprite = toRuns(g);

  // One continuous courtyard joins the gate and the workshops. Terrain stays
  // at its edges; bare earth is ground, so it never receives a sprite outline.
  const base = GROUND_BY_TERRAIN[terrain] || GROUND_BY_TERRAIN.plains;
  ctx.fillStyle = base;
  ctx.fillRect(0, (CITADEL + 1) * S, SCENE_CANVAS.w, ctx.canvas.height);
  // A sunlit sloping court, bounded by terrain rather than a striped backdrop.
  for (let y = CITADEL; y < height; y++) {
    const edge = Math.max(6, 24 - Math.floor((y - CITADEL) / 6));
    ctx.fillStyle = CITY_PALETTE.E;
    ctx.fillRect(edge * S, y * S, (SCENE_W - edge * 2) * S, S);
    if (y % 11 === 0) {
      ctx.fillStyle = CITY_PALETTE.e;
      for (let x = edge + 6 + (y % 7); x < SCENE_W - edge; x += 23)
        ctx.fillRect(x * S, y * S, 3 * S, S);
    }
  }

  for (const r of sprite.runs) {
    ctx.fillStyle = CITY_PALETTE[r.ch];
    ctx.fillRect(r.x * S, r.y * S, r.n * S, S);
  }

  // Activity stays on the lanes, below the buildings rather than across roofs.
  drawWalker(ctx, 32 + Math.round(walker.x % 53), 166, walker.frame);
  if ((buildings || []).length) {
    drawWalker(ctx, 132 + Math.round(walker.x % 35), 130, walker.frame ^ 1);
    drawWalker(ctx, 56 + Math.round(walker.x % 37), 94, walker.frame);
  }

  // Röken hör till härden i megaron och till gjuteriets ugn — alltså till
  // eldar servern faktiskt har. Ingen dekorativ skorsten någonstans.
  //
  // Utsläppet är en TIDSACKUMULATOR, inte Math.random(). Slumpen gjorde två
  // körningar av samma scen olika (64 px i riggen) och därmed varje pixeldiff
  // oanvändbar — och den bröt mot princip 14: all variation i den här grafiken
  // är deterministisk. En puff per eld var 0,9 s ser precis lika levande ut.
  smokeClock.t += dt;
  if (smokeClock.t >= 0.9) {
    smokeClock.t = 0;
    if (smoke.length < 10) smokeVents.forEach(v => smoke.push({ x: v[0], y: v[1], age: 0 }));
  }
  for (const p of smoke) {
    const a = Math.max(0, (1 - p.age / 2.2) * 0.42).toFixed(2);
    const r = 1 + Math.floor(p.age * 1.4);
    ctx.fillStyle = `rgba(180,168,150,${a})`;
    ctx.fillRect(Math.round(p.x - r) * S, Math.round(p.y - r) * S, r * 2 * S, r * 2 * S);
  }
}

// ── Citadellet i fonden ──────────────────────────────────────────────────
// Megaron, den lägre staden och den kyklopiska muren, skalade av samma
// befolkning och murnivå. Stadsvyn har fyra lokala bildnivåer; kartan
// visar serverns två grövre storleksled utan exakt befolkning.
function citadel(g, pop, walls) {
  const tier = pop >= 15000 ? 3 : pop >= 5000 ? 2 : pop >= 1000 ? 1 : 0;

  // Den lägre staden: takmassa som växer med befolkningen. Den bär ingen
  // detalj — den är kropp bakom huvudmotivet, och det är hela dess uppgift.
  const houses = [5, 5, 7, 9][tier];
  for (let i = 0; i < houses; i++) {
    const side = i % 2 ? 1 : -1, step = (i >> 1) + 1;
    const hw = 14 - step, hh = 13 - step * 2;
    const hx = SCENE_W / 2 + side * (tier ? 16 + step * 12 : step * 13) - (hw >> 1);
    if (hh < 5) continue;
    cube(g, Math.round(hx), CITADEL - hh, hw, hh, { door: step < 3, depth: side * 3, parapet: true });
  }

  // **Megaron föds ur STORLEK i stadsvyn.** Kartan visar ingen palatssal. En bosättning på 101
  // invånare har ingen sal, och att rita en åt den vore att ljuga om
  // palatskulten — den är något en plats VÄXER in i.
  if (tier >= 1) megaronHall(g, tier);

  // Muren står vid TERRASSENS KANT, mellan citadellet och gatan. Utkastet la
  // den högst upp i himlen där den hängde som ett staket utan mark under sig.
  // Här gör den i stället scenens arbete: den skiljer de två planen åt, och
  // det är precis vad en citadellmur gör sett från den lägre staden.
  if (walls > 0) rampart(g, walls);
  // Stone steps link the upper court to the street.
  for (let i = 0; i < 7; i++) {
    row(g, 81 - i, 54 + i * 2, 13 + i, 'T');
    row(g, 81 - i, 55 + i * 2, 13 + i, 't');
  }
}

// Terrassen och muren kring citadellet.
function rampart(g, level) {
  const h = 4 + level * 2;
  rect(g, 0, RAMPART, SCENE_W, h, 'q');
  row(g, 0, RAMPART, SCENE_W, 'T');
  row(g, 0, RAMPART + h - 1, SCENE_W, 't');
  // Kyklopiska fogar: KORTA, förskjutna segment. Utkastet drog dem genom hela
  // murlivet och muren blev en plankstaket-palissad — regelbundna lodräta
  // linjer från krön till fot är exakt vad kyklopiskt murverk inte har.
  // Blocken är olika stora och fogarna hamnar aldrig ovanför varandra.
  for (let bandY = RAMPART + 1; bandY < RAMPART + h - 1; bandY += 3) {
    const band = bandY - RAMPART;
    for (let x = (band * 5) % 7; x < SCENE_W; x += 5 + ((x * 3 + band) % 4))
      col(g, x, bandY, Math.min(3, RAMPART + h - 1 - bandY), 't');
    row(g, 0, bandY, SCENE_W, 't');                   // horisontell skiftfog
    row(g, 0, bandY + 1, SCENE_W, 'q');
  }
  if (level >= 2)
    for (let x = 2; x < SCENE_W - 2; x += 7) {        // tinnar
      col(g, x, RAMPART - 2, 2, 'T');
      col(g, x + 1, RAMPART - 2, 2, 'q');
    }
  // Lejonporten mitt i muren, med avlastningstriangeln över lintelen.
  // Porten ligger UR centrum. Rakt under megarons egen port bildade de två
  // mörka öppningarna en enda lodrät svart ränna genom hela scenen — två sanna
  // detaljer som tillsammans blev en artefakt.
  const gw = 10, gx = ((SCENE_W - gw) >> 1) - 26;
  rect(g, gx, RAMPART + h - 9, gw, 9, 'V');
  if (level >= 2)
    for (let k = 0; k < 3; k++)
      row(g, gx + 1 + k, RAMPART + h - 12 + k, gw - 2 - k * 2, 'V');
  row(g, gx - 1, RAMPART + h - 10, gw + 2, 'T');
  if (level >= 3)
    for (const tx of [4, SCENE_W - 18]) {             // flanktorn
      rect(g, tx, RAMPART - 6, 14, h + 6, 'q');
      row(g, tx, RAMPART - 6, 14, 'T');
      for (let x = 0; x < 14; x += 3) col(g, tx + x, RAMPART - 8, 2, 'T');
    }
}

// Megaron. Samma tre tecken som på kartan — pelare i antis, mörk lintel,
// rökglugg över härden — men här stora nog att faktiskt se ut som en sal.
function megaronHall(g, tier) {
  const mw = [0, 24, 30, 36][tier], mh = [0, 20, 24, 28][tier];
  const mx = (SCENE_W - mw) >> 1, my = CITADEL - mh;
  cube(g, mx, my, mw, mh, { roof: 3, depth: 5, parapet: true });
  row(g, mx - 3, CITADEL, mw + 6, 'T');               // terrassen salen står på
  row(g, mx - 2, CITADEL + 1, mw + 4, 't');
  const lw = mw >> 1, lx = mx + ((mw - lw) >> 1);
  rect(g, lx, my - 4, lw, 3, 'R');                    // lanterninen
  row(g, lx + lw - 2, my - 4, 2, 'r');
  row(g, lx + 1, my - 2, lw - 2, 'V');                // rökgluggen
  row(g, mx + 1, my + 4, mw - 2, 'O');                // målat ockraband
  row(g, mx + 1, my + 5, mw - 2, 'o');
  const py = my + 9;
  rect(g, mx + 4, py, mw - 8, CITADEL - py, 'p');     // förhallen
  row(g, mx + 3, py - 1, mw - 6, 'V');                // lintelen
  const cols = tier >= 3 ? 4 : tier >= 2 ? 3 : 2;
  const span = mw - 12;
  for (let i = 0; i < cols; i++) {
    const cxp = mx + 5 + Math.round(i * span / (cols - 1));
    row(g, cxp, py, 3, 'O');                          // kapitälet, en px bredare
    for (let yy = py + 1; yy < CITADEL; yy++) {
      set(g, cxp, yy, 'O'); set(g, cxp + 1, yy, 'O'); set(g, cxp + 2, yy, 'o');
    }
  }
  rect(g, mx + (mw >> 1) - 1, CITADEL - 6, 3, 6, 'V');  // porten
}

// ── Gatan ────────────────────────────────────────────────────────────────
// Varje byggnad staden har, i fast ordning, med gluggar emellan. Kön ritas
// sist så det som byggs står längst fram i blicken.
function street(g, buildings, buildQueue) {
  const vents = [];
  const byType = new Map();
  for (const b of buildings || []) {
    // Murar är ingen byggnad i gatan — de är citadellets mur och ritas där.
    if (b.type === 'wall' || b.type === 'bronze_wall' || b.type === 'tower') continue;
    byType.set(b.type, Math.max(byType.get(b.type) || 0, b.level || 1));
  }
  const built = STREET_ORDER.filter(t => byType.has(t));

  const queued = (buildQueue || []).map(item => ({
    type: item.type, phase: buildPhase(item),
    level: (byType.get(item.type) || 0) + 1,
  })).filter(q => q.type);

  const items = [
    ...built.map(t => ({ type: t, level: byType.get(t), phase: 1 })),
    ...queued,
  ];
  if (items.length === 0) return vents;

  // Pack complete footprints before drawing: no clipping at high levels.
  const rows = [[]];
  let used = 0;
  for (const it of items) {
    const width = buildingWidth(it.type, it.level);
    if (!width) continue;
    if (used && used + width + 6 > SCENE_W - 20) { rows.push([]); used = 0; }
    rows.at(-1).push(it);
    used += width + 6;
  }
  // Balance short last terraces instead of leaving one isolated building.
  for (let i = rows.length - 1; i > 0; i--) {
    while (rows[i - 1].length > rows[i].length + 1) {
      const candidate = rows[i - 1].at(-1);
      const width = [...rows[i], candidate].reduce((n, it) => n + buildingWidth(it.type, it.level) + 6, -6);
      if (width > SCENE_W - 20) break;
      rows[i].unshift(rows[i - 1].pop());
    }
  }
  rows.forEach((items, i) => layoutRow(g, items, ROW_BACK + i * 36, vents, i));
  return vents;
}

function layoutRow(g, items, base, vents, rowIndex) {
  const widths = items.map(it => buildingWidth(it.type, it.level));
  const total = widths.reduce((a, b) => a + b, 0);
  const gap = Math.min(18, Math.floor((SCENE_W - 22 - total) / Math.max(1, items.length - 1)));
  let x = Math.round((SCENE_W - total - gap * (items.length - 1)) / 2);
  // Low retaining walls and offset steps make places out of the rows.
  const left = Math.max(12, x - 5), right = Math.min(SCENE_W - 12, SCENE_W - x + 5);
  // Broken retaining edges leave the work yards open onto the street.
  const wallWidth = Math.max(12, Math.floor((right - left) * .35));
  for (const wx of [left, right - wallWidth]) {
    rect(g, wx, base + 5, wallWidth, 3, 'q');
    row(g, wx, base + 5, wallWidth, 'T');
    for (let xx = wx + 5; xx < wx + wallWidth; xx += 9) set(g, xx, base + 7, 't');
  }
  const stair = rowIndex % 2 ? left + 12 : right - 22;
  for (let i = 0; i < 3; i++) row(g, stair - i, base + 6 + i * 2, 12 + i, i % 2 ? 'T' : 'q');
  items.forEach((it, i) => {
    const foot = base - (i % 2 ? 3 : 0);
    if (it.phase >= 1) stampBuilding(g, it.type, x, foot, it.level);
    else stampUnderConstruction(g, it.type, x, foot, it.level, it.phase);
    if (it.type === 'foundry' && it.phase >= 1) vents.push([x + 28, foot - 22]);
    x += widths[i] + gap;
  });
}

// Residences express population, as in the citadel, not an invented workplace.
// Leave busy cities' lower terraces to the actual building payload.
function homes(g, pop, count) {
  if (count > 5) return;
  const houses = pop >= 1000 ? [[27, 125, 18, 17], [49, 137, 23, 18], [160, 132, 22, 19], [186, 120, 15, 15]]
    : [[61, 95, 20, 16], [88, 104, 16, 14], [133, 100, 22, 18]];
  for (const [x, b, w, h] of houses) {
    cube(g, x, b - h, w, h, {depth: 4, parapet: true, door: true});
    rect(g, x + 3, b - 10, 3, 3, 'V'); row(g, x + 2, b - 7, 5, 'P');
    row(g, x - 2, b + 1, w + 10, 'T');
    houseDetails(g, x, b, w, h);
    // Small stack of firewood against the wall.
    for (let j = 0; j < 2; j++) { row(g, x + 2, b - 2 - j * 2, 6, 'w'); set(g, x + 2, b - 2 - j * 2, 'M'); }
  }
  // Shade cloth, a bench, and a small planted domestic court.
  row(g, 77, 130, 20, 'W'); col(g, 77, 129, 8, 'W'); col(g, 96, 129, 8, 'W');
  rect(g, 80, 131, 5, 5, 'P'); row(g, 80, 135, 5, 'O');
  rect(g, 88, 131, 5, 4, 'n'); row(g, 88, 134, 5, 'N');
  // A shaded meeting place, with a reed pergola and vessels beside the bench.
  for (const xx of [114, 143]) { col(g, xx, 120, 22, 'W'); set(g, xx + 1, 141, 'T'); }
  for (let yy = 0; yy < 5; yy++) row(g, 114 + yy, 120 - yy, 30, yy % 2 ? 'w' : 'N');
  for (const [xx, yy] of [[116,117],[122,115],[131,116],[139,119],[142,122]]) {
    row(g, xx, yy, 5, 'n'); row(g, xx + 1, yy + 1, 4, 'g');
  }
  rect(g, 121, 137, 14, 2, 'w'); col(g, 122, 139, 2, 'W'); col(g, 133, 139, 2, 'W');
  for (const [xx, yy] of [[140,142],[109,144]]) {
    row(g, xx + 1, yy - 6, 3, 'r'); rect(g, xx, yy - 4, 5, 4, 'R'); col(g, xx, yy - 3, 2, 'M');
  }
  // A patterned mat spread on the earth in front of the shaded bench.
  for (let yy = 0; yy < 4; yy++) row(g, 119 - yy, 145 + yy, 19, yy % 2 ? 'O' : 'B');
  row(g, 116, 149, 19, 'o');
}

// Domestic greenery at the court's edge; these are not production buildings.
function garden(g) {
  for (const [x, y] of [[6, 77], [218, 107], [7, 139], [216, 153]]) {
    col(g, x, y - 8, 9, 'W');
    rect(g, x - 4, y - 12, 9, 5, 'N');
    row(g, x - 3, y - 14, 6, 'n'); row(g, x - 5, y - 11, 10, 'n');
    row(g, x - 4, y - 13, 7, 'n'); row(g, x - 2, y - 10, 4, 'g');
    row(g, x - 3, y - 6, 5, 'N');
    row(g, x - 4, y + 1, 9, 't');
  }
}

/** Byggets förlopp 0–1 ur `created_at`/`complete_at`. */
function buildPhase(item) {
  const now = serverNow();
  const end = new Date(item.complete_at).getTime();
  if (!(end > 0)) return 1;
  if (end <= now) return 1;
  const start = item.created_at ? new Date(item.created_at).getTime() : now - 60000;
  if (!(start < end)) return 0.5;
  return Math.min(1, Math.max(0, (now - start) / (end - start)));
}

// ── Gångaren ─────────────────────────────────────────────────────────────
// En enda figur på gatan, i aktörernas skala och hudton. Fler skulle göra
// scenen till en myrstack; en gör den bebodd.
function drawWalker(ctx, x, base, frame) {
  const px = (cx, cy, colour, w = 1, h = 1) => {
    ctx.fillStyle = colour;
    ctx.fillRect(cx * S, cy * S, w * S, h * S);
  };
  px(x, base - 7, CITY_PALETTE.K, 3, 1);
  px(x, base - 6, '#A87548', 3, 2);                   // aktörernas hudton
  px(x, base - 4, CITY_PALETTE.P, 3, 3);
  px(x + (frame ? 0 : 2), base - 1, CITY_PALETTE.K, 1, 1);
  px(x + (frame ? 2 : 0), base - 1, CITY_PALETTE.G, 1, 1);
}
