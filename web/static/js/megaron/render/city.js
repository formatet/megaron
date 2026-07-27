// ── Stadsvyn: staden inifrån ─────────────────────────────────────────────
//
// Kartmarkören svarar på "hur stor är den?". Den här scenen svarar på "vad ÄR
// den?" — vilka byggnader staden faktiskt har, på vilken nivå, och vad som
// håller på att resa sig just nu.
//
// Den gamla scenen hade FYRA fasta sidoslottar sorterade efter en
// prioritetslista, plus en generisk klassisk tempelfasad i mitten. En stad med
// sju byggnader visade fyra av dem, och att bygga något nytt syntes ofta inte
// alls. Den hade dessutom en helt egen palett: kartan och stadsvyn såg ut att
// komma från två olika spel.
//
// Nu: **varje** byggnad ritas, i sin egen akhaiska form, ur samma palett som
// kartsiluetterna. Citadellet i fonden bär megaron och den kyklopiska muren och
// skalas av befolkning och murnivå — samma `size_tier` som kartan använder, så
// de två bilderna aldrig kan säga olika saker om samma stad.
//
// Blickpunkten är INIFRÅN staden: muren löper i fonden, bakom citadellet, och
// gatan med verkstäderna ligger framför. En mur ritad framför hade varit
// vackrare och hade skymt precis det scenen finns till för att visa.

import { serverNow } from '../clock.js';
import {
  CITY_PALETTE, newGrid, set, row, col, rect, cube, outline, toRuns,
} from './pixelgrid.js';
import { stampBuilding, stampUnderConstruction, buildingWidth } from './citybuildings.js';

// Scenen ritas i LOGISKA pixlar och skalas upp med heltal — samma pixeldisciplin
// som kartan (princip 10: aldrig arc(), aldrig halva pixlar).
const SCENE_W = 160, SCENE_H = 76, S = 2;
export const SCENE_CANVAS = { w: SCENE_W * S, h: SCENE_H * S };

// Tre plan, bakifrån och fram. Djupet ligger i att de tre banden ALDRIG korsar
// varandra — scenen har ingen enda diagonal, precis som resten av grafiken.
const CITADEL = 34;         // citadellets fot: megaron och den lägre staden
const RAMPART = 36;         // murens krön, vid terrassens kant
const ROW_BACK = 56;        // bortre gatuledet
const ROW_FRONT = 72;       // främre gatuledet

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
  canvas.height = SCENE_CANVAS.h;
  const ctx = canvas.getContext('2d');
  ctx.imageSmoothingEnabled = false;

  const walker = { x: 20, dir: 1, t: 0, frame: 0 };
  const smoke = [];
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
  plains: '#7A8642', river_valley: '#6E8A46', river_delta: '#6E8A46',
  hills: '#A08A52', mountain_limestone: '#B8AC90', mountain_red: '#A87A5A',
  forest_olive_grove: '#8A8E55', scrub_maquis: '#93A055', semi_desert: '#B89E66',
  coastal_sea: '#7A8642', deep_sea: '#7A8642',
};

const smokeClock = { t: 0 };

function drawScene(ctx, tile, buildings, buildQueue, sett, walker, smoke, dt) {
  const terrain = tile?.terrain || 'plains';
  ctx.fillStyle = '#C6BC9E';                       // dis: himlen bakom citadellet
  ctx.fillRect(0, 0, SCENE_CANVAS.w, SCENE_CANVAS.h);

  const g = newGrid(SCENE_W, SCENE_H);
  const pop = sett?.population || 0;
  const walls = Math.max(0, Math.min(3, sett?.walls || 0));

  citadel(g, pop, walls);
  const smokeVents = street(g, buildings, buildQueue);
  outline(g);
  const sprite = toRuns(g);

  // Marken i tre band, bakifrån och fram: citadellklippan, den bortre gatan,
  // den främre. Varje band är en nyans mörkare — luftperspektiv utan gradient,
  // och utan en enda diagonal.
  const base = GROUND_BY_TERRAIN[terrain] || GROUND_BY_TERRAIN.plains;
  ctx.fillStyle = base;
  ctx.fillRect(0, (CITADEL + 1) * S, SCENE_CANVAS.w, SCENE_CANVAS.h);
  ctx.fillStyle = '#9E8E5E';                       // bortre gatans trampade jord
  ctx.fillRect(0, (ROW_BACK + 1) * S, SCENE_CANVAS.w, SCENE_CANVAS.h);
  ctx.fillStyle = '#8C7C4E';                       // främre gatan
  ctx.fillRect(0, (ROW_FRONT + 1) * S, SCENE_CANVAS.w, SCENE_CANVAS.h);
  ctx.fillStyle = '#7C6C42';
  for (let sx = 4; sx < SCENE_W; sx += 9) ctx.fillRect(sx * S, (ROW_BACK + 2) * S, S, S);

  for (const r of sprite.runs) {
    ctx.fillStyle = CITY_PALETTE[r.ch];
    ctx.fillRect(r.x * S, r.y * S, r.n * S, S);
  }

  drawWalker(ctx, Math.round(walker.x), ROW_FRONT, walker.frame);

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
// befolkning och murnivå som kartsiluetten. Öppnar man kartan och stadsvyn
// bredvid varandra ska de säga samma sak.
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
    cube(g, Math.round(hx), CITADEL - hh, hw, hh, { door: step < 3 });
  }

  // **Megaron föds ur STORLEK, precis som på kartan.** En bosättning på 101
  // invånare har ingen sal, och att rita en åt den vore att ljuga om
  // palatskulten — den är något en plats VÄXER in i.
  if (tier >= 1) megaronHall(g, tier);

  // Muren står vid TERRASSENS KANT, mellan citadellet och gatan. Utkastet la
  // den högst upp i himlen där den hängde som ett staket utan mark under sig.
  // Här gör den i stället scenens arbete: den skiljer de två planen åt, och
  // det är precis vad en citadellmur gör sett från den lägre staden.
  if (walls > 0) rampart(g, walls);
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
  cube(g, mx, my, mw, mh, { roof: 4 });
  row(g, mx - 3, CITADEL, mw + 6, 'T');               // terrassen salen står på
  row(g, mx - 2, CITADEL + 1, mw + 4, 't');
  const lw = mw >> 1, lx = mx + ((mw - lw) >> 1);
  rect(g, lx, my - 4, lw, 3, 'R');                    // lanterninen
  row(g, lx + lw - 2, my - 4, 2, 'r');
  row(g, lx + 1, my - 2, lw - 2, 'V');                // rökgluggen
  row(g, mx + 1, my + 4, mw - 2, 'O');                // målat ockraband
  row(g, mx + 1, my + 5, mw - 2, 'o');
  const py = my + 9;
  rect(g, mx + 4, py, mw - 8, CITADEL - py, 'd');     // förhallen
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
  rect(g, mx + (mw >> 1) - 2, CITADEL - 8, 5, 8, 'V');  // porten
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

  // **Två gatuled.** Ett enda led med tretton byggnader gav elva logiska pixlar
  // per byggnad — då är en hamn och ett gjuteri samma suddiga klump, och
  // "enskilda byggnader ska synas" är inte uppfyllt. Bygget som PÅGÅR hamnar
  // alltid i det främre ledet: det är det spelaren tittar efter.
  const twoRows = items.length > 5;
  const back = [], front = [];
  items.forEach((it, i) => {
    if (it.phase < 1) front.push(it);
    else if (!twoRows) front.push(it);
    else (i % 2 ? front : back).push(it);
  });

  // Bortre ledet ritas FÖRST så det främre skymmer det — samma djupsortering
  // som kartans provinser, av samma skäl.
  layoutRow(g, back, ROW_BACK, vents);
  layoutRow(g, front, ROW_FRONT, vents);
  vents.push([SCENE_W / 2 + 2, CITADEL - 30]);        // härden i megaron
  return vents;
}

/** Radar upp ett gatuled centrerat, med gluggarna klämda när staden växer.
 *  Layouten MÄTS före den ritas: en stad med tolv byggnader ska få trängre
 *  gluggar, inte byggnader som ramlar ut ur bilden. */
function layoutRow(g, items, base, vents) {
  if (!items.length) return;
  const widths = items.map(it => buildingWidth(it.type, it.level));
  const total = widths.reduce((a, b) => a + b, 0);
  const gap = items.length < 2 ? 0
    : Math.max(1, Math.min(6, Math.floor((SCENE_W - 6 - total) / (items.length - 1))));
  let x = Math.max(2, Math.round((SCENE_W - (total + gap * (items.length - 1))) / 2));
  items.forEach((it, i) => {
    if (it.phase >= 1) stampBuilding(g, it.type, x, base, it.level);
    else stampUnderConstruction(g, it.type, x, base, it.level, it.phase);
    if (it.type === 'foundry' && it.phase >= 1) vents.push([x + 10, base - 13]);
    x += widths[i] + gap;
  });
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
