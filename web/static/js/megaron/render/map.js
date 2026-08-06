import { State, ownCapital } from '../state.js';
import { fetchAuth } from '../api.js';
import { track } from '../telemetry.js';
import { serverNow } from '../clock.js';
import {
  LIVE_RADIUS_SEA, LIVE_RADIUS_BASE, LIVE_RADIUS_MOUNTAIN_BONUS, LOCAL_ZOOM,
  GARRISON_DOT_ZOOM, ACTIVITY_BADGE_ZOOM, ROAD_DEPOSIT_ZOOM,
  PAN_SPEED_PX_PER_SEC,
} from '../config.js';
import { isTypingTarget } from '../ui/format.js';
import { canonicalUnitType, actorName } from '../ui/actornames.js';
import { drawActor, spriteRuns, FOREIGN_ACCENT, FOREIGN_OUTLINE } from './actorsprites.js';
import { drawCityMass, citySprite, cityTop, cityFoot } from './citysprites.js';
import { zoomStep, clampPan } from './camera.js';

// ── Palette — Settlers 2 warmth, Mediterranean olive country ─────────────
const TERRAIN_BASE = {
  deep_sea:           {c0:'#1A5276', c1:'#154360'},
  // Darkened when the plains landed on #859248 (L 137.9) and left this at
  // L 138.1: the shoreline had exactly 0.2 luminance contrast, so in greyscale
  // it did not exist at all. Measured cross-section of the render confirmed it —
  // a flat 138 straight across land and sea. The value comes from sampling the
  // reference's sea, which is desaturated slate (deep ~#436881, shallow
  // ~#5C8C9C), not azure; this sits between them and gives ΔL 24.6 against the
  // plains, better than the 20.5 that existed before the plains changed.
  //
  // This is a REPAIR, not the sea step. The same sampling shows the reference's
  // own green-plain-to-shallow-water contrast is only ~7 — it does not separate
  // land from sea by value at all, it draws a BRIGHT SHORE BAND, which is what
  // the contrast budget (princip 6) asks for anyway. That band is the real fix
  // and belongs to the hav/kust slice.
  coastal_sea:        {c0:'#3E7C9E', c1:'#33667F'},
  plains:             {c0:'#859248', c1:'#6F7B3A'},
  // ── Flodfamiljen (2026-07-29) ────────────────────────────────────────────
  // `river` är sedan Timothys beslut 2026-07-29 en egen VATTENterräng: en
  // seglingsbar kedja, exakt en hex bred, källa → Thalassa. Den upphäver
  // princip 20:s parkering — mekaniken finns nu, alltså får floden renderas.
  //
  // Tonvalet styrs av EN gräns framför alla andra: floden och `river_valley`
  // delar varje hexkant per konstruktion (dalen ligger på var sida om floden),
  // så det paret måste separera hårdast på hela kartan. Floden ligger därför
  // mörkare än kustvattnet, inte ljusare — en smal inlandsflod mellan höga
  // stränder läser som en MÖRK tråd, och det är samma logik som säger att
  // strandbandets ljusa ytterlighet hör kustlinjen till (princip 6). Att
  // floden hamnar 16 L från `coastal_sea` är medvetet betalt: de två bär
  // samma affordans (vatten, seglingsbart, ogenomträngligt för landenheter),
  // alltså är paret gratis enligt princip 43 — spelaren har inget beslut som
  // hänger på att skilja dem åt, och vid mynningen SKA de flyta ihop.
  river:              {c0:'#39707C', c1:'#2F5F69'},
  // river_ford (megaron_plan_flodbudget_och_vadstalle.md, Timothy 2026-08-02):
  // the river's port, still water but the shallow kind — lighter than `river`
  // itself (which is deliberately darker than coastal_sea, see the note
  // above) so the hex reads as "shallower water" at a glance, distinct within
  // the same family rather than a whole new hue. TABLE ROW ONLY — this slice
  // is explicitly non-visual (no new hex art, no rit-switch case, isWaterTerrain
  // untouched): the ford's actual pixel art is its own slice with an eye-check
  // at 1:1. Treat this tone as provisional.
  river_ford:         {c0:'#5E9CA6', c1:'#4E8590'},
  // Dalen: bevattnad flodslätt, spelets bördigaste mark efter deltat. Den
  // lämnar den mättade 2020-talsgrönan (`#4CAF50`) som slätten just lämnade,
  // och landar MÖRKARE än slätten — princip 7 mätt ur referensen: bördig grön
  // är kartans mörka ände, torr guldmark den ljusa. Ligger 17 L från slätten
  // och 24 från floden.
  // Mätt ned från `#6D8A42` efter princip 17: den renderade dalen landade på
  // markton 127,6 mot slättens 133,7 — 6,1 isär, alltså samma yta i gråskala,
  // och de två möts längs varje flod på kartan. Basfärgernas nominella avstånd
  // (17 L) höll inte, för dalens eget fält ljusnar ytan och markövergången
  // blandar in slättens ton i kantzonen. Bara den renderade ytan räknas.
  river_valley:       {c0:'#627E3B', c1:'#516A31'},
  // Deltat är inte "mer dal". Det är alluvium — blek silt med bördiga fläckar
  // i, och det ligger per definition mot vatten på flera kanter. Därför går
  // det åt ANDRA hållet på valörstegen än dalen: ljust nog att aldrig kunna
  // förväxlas med havet eller floden det mynnar i.
  river_delta:        {c0:'#93A05A', c1:'#7C884A'},
  forest_olive_grove: {c0:'#9EA361', c1:'#848A4C'},
  hills:              {c0:'#C8A464', c1:'#B08C50'},
  // The mountains' scree covers the hex completely, so these two are no longer
  // a visible surface — they are what shows through the clip's antialiased rim
  // (princip 13b: a clipped layer only reaches ~75% coverage on the edge pixel,
  // and the base lights up underneath). With the old pale `#E0D4B8` under dark
  // scree that rim drew a bright halo around every mountain hex — a hex grid
  // painted by accident, the exact bug princip 13 exists to prevent. They now
  // carry the scree's own dominant tone, so there is nothing left to shine
  // through. Update these whenever MTN_ROCK moves.
  mountain_limestone: {c0:'#ABA692', c1:'#8E8A78'},
  mountain_red:       {c0:'#A07A5E', c1:'#82604A'},
  // Skruben lämnade det gula registret (`#A8B860`, L 169) och gick ner i
  // gråsage. Skälet står vid drawScrubField: 169 låg 2,3 L från kullarnas
  // markton och de delar 35 hexkanter i världsfixturen. Se den kommentaren
  // för varför 152 är max-min-valet och vad det kostar mot lundens golv.
  scrub_maquis:       {c0:'#A3AC6A', c1:'#8A9354'},
  semi_desert:        {c0:'#D4B878', c1:'#C0A060'},
  // Cederskogen — princip 8:s "skogen där arméer försvinner". Olivlunden är en
  // ODLING (blek, gles, framkomlig) och renderas som mörka objekt mot ljus
  // mark; cedern är VILDMARK och vänder därför separationsriktningen (princip
  // 7): ljusa kronor som löser ut ur en mörk sluten massa. Basen är den mörka
  // barrförnan under det slutna taket, inte en markton man ser mycket av.
  forest_cedar:       {c0:'#4E5C3C', c1:'#3E4A30'},
  fog:                {c0:'#1C1C1C', c1:'#252018'},
};

// Culture accent colours. Låg tidigare som en 1,5 px strimma på stadsrutan;
// den rutan är ersatt av stadssiluetterna och strimman följde med bort
// (megaron_stader_20260727). Kulturen ska bäras av HELA siluetten när de
// kulturspecifika leden byggs — idag är alla akhaiska. Tabellen står kvar
// oanvänd tills dess; den är sanningen om vilka kulturer som finns.
const CULTURE_ACCENT = {
  akhaier:  '#CA8A04',
  khemetiu: '#0E7490',
  knaani:   '#86198F',
  thrakes:  '#1D4ED8',
  minoan:   '#0891B2',
  hatti:    '#374151',
};

// ── Hex geometry ─────────────────────────────────────────────────────────
const S = 22;    // hex size in logical pixels — Settlers 2 scale
export const SCALE = 2; // canvas scale factor

// ── LOD: grovkorna markrastret när blocken blir mindre än en pixel värda ──
//
// PROBLEMET, mätt. Varje markpass ritar sitt raster på ett lattice angivet i
// VÄRLDSENHETER (STEP 3-4), aldrig i skärmpixlar. Antalet block per hex är
// alltså konstant medan hexen krymper — och vid minzoom syns HELA världen, så
// alla 13 225 hexar kör hela sitt raster samtidigt. Det är därför culling gav
// −47 % vid 1:1 men −0,6 % vid minzoom (det finns inget att culla när allt
// syns) och varför `ground` ensamt är 56 % av en 318 ms-frame på 230².
//
// LÖSNINGEN. Skala latticet så blocket håller sig över LOD_MIN_BLOCK_PX
// SKÄRMpixlar. Ett block på 2,4 px bär ingen form — det bidrar bara med ton,
// och tonen bevaras: samma brusfält, samma trösklar, samma andelar mellan de
// tre banden, bara färre och större sampel. Kostnaden faller kvadratiskt med
// faktorn.
//
// VIKTIGT: faktorn är 1 vid spelzoom och 1:1, så de zoomarna är
// BIT-IDENTISKA med före. LOD:en slår in först under ~0,42, alltså bara i det
// läge där hela världen syns och detaljen ändå inte kan läsas. Grinden mot
// princip 5 (1:1 är bedömningsskalan) är därmed uppfylld per konstruktion och
// inte per ögonmått.
const LOD_MIN_BLOCK_PX = 4;

// lodStep grovkornar ett världsenhets-lattice för aktuell zoom. `base` är
// passets egen STEP; returvärdet är alltid en heltalsmultipel av den, så
// `Math.floor(x / step) * step` fortsätter ligga på samma globala lattice och
// rastret betraktar fortfarande inte hexkanten (princip 2).
function lodStep(base) {
  const px = base * State.camera.zoom * SCALE;
  const k = Math.max(1, Math.round(LOD_MIN_BLOCK_PX / px));
  return base * k;
}

export function hexPx(q, r) {
  return {
    x: Math.round(S * 1.5 * q),
    y: Math.round(S * Math.sqrt(3) * (r + q / 2)),
  };
}

function hexPts(cx, cy) {
  const pts = [];
  for (let i = 0; i < 6; i++) {
    const a = Math.PI / 3 * i;
    pts.push([Math.round(cx + S * Math.cos(a)), Math.round(cy + S * Math.sin(a))]);
  }
  return pts;
}

// ── World-Rim, step 1: the camera may not leave the world ────────────────
// How far past the last hex the view may travel, in WORLD UNITS (≈2 hexes
// wide). Enough to see that the world ENDS; not enough to lose it. Never in
// screen pixels — that would be a thick frame at min zoom and a hairline at
// 1:1 (the LOD trap).
const PAN_MARGIN = S * 3;

// The map's bounding box in world units, memoised on the tileData array's
// identity — refreshTiles() assigns a fresh array, so a new fog fetch
// recomputes and nothing else does. Recomputing per frame would walk 52 900
// hexes on a 230² map on every drag event.
//
// Bounds come from the FULL tile list including fog: the world's extent is not
// a secret (the server returns a tile per hex either way), whereas clamping to
// the KNOWN tiles would make the pan limit itself leak the shape of what you
// have explored.
let panBoundsSrc = null;
let panBounds = null;
function worldPanBounds() {
  if (State.tileData === panBoundsSrc) return panBounds;
  panBoundsSrc = State.tileData;
  panBounds = null;
  if (!State.tileData || !State.tileData.length) return null;
  let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
  for (const t of State.tileData) {
    const p = hexPx(t.q, t.r);
    if (p.x < minX) minX = p.x;
    if (p.x > maxX) maxX = p.x;
    if (p.y < minY) minY = p.y;
    if (p.y > maxY) maxY = p.y;
  }
  panBounds = { minX, minY, maxX, maxY };
  return panBounds;
}

// Apply the clamp to the live camera. Called after EVERY camera move — drag,
// held key, wheel, zoom button, recentre, and search's centreOn (ui/search.js,
// the one site outside this file) — because a bound enforced at six of seven
// sites is not a bound.
export function clampCamera() {
  const next = clampPan(
    State.camera, SCALE,
    { w: canvas.width, h: canvas.height },
    worldPanBounds(), PAN_MARGIN,
  );
  State.camera.x = next.x;
  State.camera.y = next.y;
}

// Hit-test: nearest hex to canvas coords
function hexAtScreen(sx, sy) {
  const wx = (sx - State.camera.x) / (State.camera.zoom * SCALE);
  const wy = (sy - State.camera.y) / (State.camera.zoom * SCALE);
  const q = (2/3 * wx) / S;
  const r = (-1/3 * wx + Math.sqrt(3)/3 * wy) / S;
  const s = -q - r;
  let rq = Math.round(q), rr = Math.round(r), rs = Math.round(s);
  const dq = Math.abs(rq-q), dr = Math.abs(rr-r), ds = Math.abs(rs-s);
  if (dq > dr && dq > ds) rq = -rr - rs;
  else if (dr > ds) rr = -rq - rs;
  return {q: rq, r: rr};
}

// ── Hex path helpers ─────────────────────────────────────────────────────
// Six axial directions in cube space.
const HEX_DIRS = [[1,0],[-1,0],[0,1],[0,-1],[1,-1],[-1,1]];

// Hex distance in axial coords (cube formula).
function hexDist(q1, r1, q2, r2) {
  return (Math.abs(q1-q2) + Math.abs(q1+r1-q2-r2) + Math.abs(r1-r2)) / 2;
}

// Mirrors server/internal/province/hex.go LiveRadius(eyeKind, targetTerrain) —
// see config.js LIVE_RADIUS_* for the mirrored constants.
function liveRadius(kind, terrain) {
  if (terrain === 'coastal_sea' || terrain === 'deep_sea') return LIVE_RADIUS_SEA;
  let base = LIVE_RADIUS_BASE[kind] ?? LIVE_RADIUS_BASE.land;
  if (terrain === 'mountain_limestone' || terrain === 'mountain_red') base += LIVE_RADIUS_MOUNTAIN_BONUS;
  return base;
}

// Adjacent neighbor of (cq,cr) that is one step closer to (tq,tr).
// Ties broken by array order — stable for a given origin/destination.
function hexNeighborToward(cq, cr, tq, tr) {
  let best = null, bestD = Infinity;
  for (const [dq, dr] of HEX_DIRS) {
    const d = hexDist(cq+dq, cr+dr, tq, tr);
    if (d < bestD) { bestD = d; best = {q: cq+dq, r: cr+dr}; }
  }
  return best;
}

// Build an adjacency-guaranteed hex path from (q1,r1) to (q2,r2) via greedy steps.
// Cube-lerp has a known rounding defect where consecutive sampled hexes can be
// non-adjacent; this greedy approach is safe for small maps.
function buildHexPath(q1, r1, q2, r2) {
  const path = [{q: q1, r: r1}];
  let cq = q1, cr = r1;
  while (cq !== q2 || cr !== r2) {
    const n = hexNeighborToward(cq, cr, q2, r2);
    if (!n) break;
    path.push(n);
    cq = n.q; cr = n.r;
  }
  return path;
}

const hexPathCache = {};
function getHexPath(q1, r1, q2, r2) {
  const key = `${q1},${r1}-${q2},${r2}`;
  if (!hexPathCache[key]) hexPathCache[key] = buildHexPath(q1, r1, q2, r2);
  return hexPathCache[key];
}

// Interpolated pixel position along the hex path at fractional progress [0,1].
function hexPathPx(q1, r1, q2, r2, progress) {
  const path = getHexPath(q1, r1, q2, r2);
  if (path.length === 0) return hexPx(q1, r1);
  if (path.length === 1) return Object.assign({}, hexPx(q1, r1), {q: q1, r: r1});
  const idx   = Math.min(path.length - 2, Math.floor(progress * (path.length - 1)));
  const local = (progress * (path.length - 1)) - idx;
  const a = hexPx(path[idx].q, path[idx].r);
  const b = hexPx(path[idx+1].q, path[idx+1].r);
  return {
    x: a.x + (b.x - a.x) * local,
    y: a.y + (b.y - a.y) * local,
    q: path[idx].q,
    r: path[idx].r,
  };
}

// Interpolated pixel position along an explicit server-provided A* waypoint list
// [[q,r],...] at fractional progress [0,1]. Same interpolation as hexPathPx, but
// walks the true route (via sea / around mountains) instead of a straight hex
// line, so the walker is drawn where the unit actually is. Returns null for an
// empty list so callers can fall back to the straight-line hexPathPx.
function pathPx(waypoints, progress) {
  if (!waypoints || waypoints.length === 0) return null;
  if (waypoints.length === 1) {
    const p = hexPx(waypoints[0][0], waypoints[0][1]);
    return {x: p.x, y: p.y, q: waypoints[0][0], r: waypoints[0][1]};
  }
  const idx   = Math.min(waypoints.length - 2, Math.floor(progress * (waypoints.length - 1)));
  const local = (progress * (waypoints.length - 1)) - idx;
  const a = hexPx(waypoints[idx][0],   waypoints[idx][1]);
  const b = hexPx(waypoints[idx+1][0], waypoints[idx+1][1]);
  return {
    x: a.x + (b.x - a.x) * local,
    y: a.y + (b.y - a.y) * local,
    q: waypoints[idx][0],
    r: waypoints[idx][1],
  };
}

// Hexen en enhet står på JUST NU — samma gren som ritningen av aktören använder
// (marching → interpolerad waypoint, positioned → u.q/u.r). Returnerar null för
// en enhet utan hexposition alls (garnison), som därför måste matchas på sitt
// settlement i stället. Finns för att tooltipen ska peka på samma hex som
// spriten; delad kod är hela poängen, en andra kopia av interpolationen skulle
// glida isär från renderaren.
function unitHexNow(u) {
  if (u.status === 'marching' && u.departs_at && u.arrives_at && u.q != null && u.target_q != null) {
    const departs = new Date(u.departs_at).getTime();
    const arrives = new Date(u.arrives_at).getTime();
    const progress = Math.min(1, Math.max(0, (serverNow() - departs) / (arrives - departs)));
    const pos = (u.path && u.path.length > 1)
      ? pathPx(u.path, progress)
      : hexPathPx(u.q, u.r, u.target_q, u.target_r, progress);
    // hexPathPx faller tillbaka på en ren pixel utan q/r när ingen väg finns —
    // då är avgångshexen det ärligaste svaret, inte "ingenstans".
    return pos && pos.q != null ? {q: pos.q, r: pos.r} : {q: u.q, r: u.r};
  }
  return u.q != null ? {q: u.q, r: u.r} : null;
}

function isTileVisible(q, r) {
  return State.tileData.some(t => t.q === q && t.r === r && t.terrain !== 'fog');
}

// tileTierByPos: "q,r" -> tier ("live"|"remembered"|"fog"), indexed instead of
// scanned per-call like isTileVisible above — isTileVisible is a linear scan
// over up to 52,900 tiles PER CALL, fine at the call sites it already has, but
// not something to add a second per-actor, per-frame copy of
// (fow/frammande-enheter, 2026-08-03). isTileVisible's existing callers are
// untouched; this is a separate, additive index that only foreign units use.
//
// The index rebuilds itself whenever State.tileData is a DIFFERENT array than
// the one it was built from, rather than making every writer remember to call
// a rebuild hook. Every writer replaces the array wholesale (loadMap,
// refreshTiles, and the showcase fixtures), so identity is a sound trigger —
// and a fixture that sets State.tileData directly cannot silently end up with
// an empty index, which is exactly how showcase-units.html would have drawn
// zero foreign units while looking like a FOW bug.
let tileTierByPos = new Map();
let tileTierSource = null;
function tileTier(q, r) {
  if (tileTierSource !== State.tileData) {
    tileTierSource = State.tileData;
    tileTierByPos = new Map();
    for (const t of State.tileData) tileTierByPos.set(`${t.q},${t.r}`, t.tier);
  }
  return tileTierByPos.get(`${q},${r}`);
}

// isTileLive: strictly tier 1 (live) — unlike isTileVisible, which also
// passes tier 2 (remembered). A foreign unit must never be drawn on a
// remembered hex: memory carries no activity (temenos_synlighet.md,
// kanonbeslut Timothy 2026-08-03).
function isTileLive(q, r) {
  return tileTier(q, r) === 'live';
}

// Blinkperioden för främmande enheters kontur, i renderframes: 24 på, 24 av
// (~0,4 s vardera vid rAF-takt). Ligger här för att BÅDE väckningsvillkoret i
// render() och ritningen längre ned läser samma tal — två kopior skulle glida
// isär och ge en blink som väcker loopen i otakt med sig själv.
const FOREIGN_BLINK_FRAMES = 24;
// ── Hex fill — solid path fill + outline ─────────────────────────────────
function hexPath(ctx, pts) {
  ctx.beginPath();
  ctx.moveTo(pts[0][0], pts[0][1]);
  for (let i = 1; i < 6; i++) ctx.lineTo(pts[i][0], pts[i][1]);
  ctx.closePath();
}

function fillHex(ctx, pts, c0, c1, seed) {
  hexPath(ctx, pts);
  ctx.fillStyle = c0;
  ctx.fill();
  ctx.strokeStyle = c0;
  // Hexpolygonerna tesselerar inte exakt: hörnen rundas till heltal, så varje
  // delad kant lämnar en springa där ~8 % av bakgrunden lyser igenom. Stroken
  // finns för att täcka den — men den var 1 LOGISK pixel, och renderaren ritar
  // genom `ctx.scale(zoom × SCALE)`. Vid minzoom (k = 0,6) blev den 0,6 device-
  // pixlar, slutade täcka, och ett hexrutnät framträdde över hela kartan —
  // tydligast på havet där ingen textur döljer det. Uppmätt: 7,7 % av
  // havspixlarna mörkare än basfärgen vid zoom 0,30, 0 % vid ≥0,50. Princip 13
  // säger att inget hexrutnät får ritas; det ritades av misstag i tre år.
  // Stroken hålls nu vid minst ~1,25 device-pixlar. För k ≥ 1,25 (zoom ≥ 0,63)
  // är uttrycket exakt 1 och bilden pixelidentisk med tidigare.
  ctx.lineWidth = Math.max(1, 1.25 / (State.camera.zoom * SCALE));
  ctx.stroke();
}

// ── Deterministic per-hex variation ──────────────────────────────────────
// A hex must look identical on every reload and for every player, but must
// not read as a pattern. The old scheme — seed = (q*137 + r*31) & 0xff, then
// masked per detail with & 0x1b — could only reach eight offsets with gaps
// between them, which is why forest read as wallpaper rather than woodland.
//
// This is a 32-bit avalanche mix (Murmur3 finalizer) over (worldSalt, q, r,
// stream): neighbouring hexes share no visible structure, and each `stream`
// is an independent draw for the same hex.
//
// The world component is hashed from State.WORLD_ID, NOT worlds.map_seed —
// map_seed never leaves the server, and WORLD_ID gives the same guarantee
// (stable per world, different between worlds) with no server change. This is
// presentation only: it must never feed mapgen or any rule.
let worldSalt = 0;
let worldSaltFor;
function salt() {
  if (worldSaltFor !== State.WORLD_ID) {
    worldSaltFor = State.WORLD_ID;
    let h = 0x9e3779b9;
    const s = String(State.WORLD_ID ?? '');
    for (let i = 0; i < s.length; i++) h = Math.imul(h ^ s.charCodeAt(i), 0x85ebca6b) >>> 0;
    worldSalt = h >>> 0;
  }
  return worldSalt;
}

function hash32(q, r, stream) {
  let h = (salt() ^ Math.imul(q | 0, 0x27d4eb2d) ^ Math.imul(r | 0, 0x165667b1)
           ^ Math.imul(stream | 0, 0x9e3779b9)) >>> 0;
  h = Math.imul(h ^ (h >>> 16), 0x85ebca6b) >>> 0;
  h = Math.imul(h ^ (h >>> 13), 0xc2b2ae35) >>> 0;
  return (h ^ (h >>> 16)) >>> 0;
}
// Uniform float in [0,1) / integer in [0,n) from one stream of a hex's variation.
const rnd = (q, r, stream) => hash32(q, r, stream) / 4294967296;
const rndInt = (q, r, stream, n) => hash32(q, r, stream) % n;

// ── Tile lookup index ────────────────────────────────────────────────────
// Terrain that reacts to its neighbours needs six lookups per hex per frame,
// and State.tileData.find() is O(n) over the whole map (2240 tiles on the
// current world) — that would be ~400k comparisons a frame for one forest
// block. State.tileData is only ever REASSIGNED (loadMap/refreshTiles), never
// mutated in place, so caching on array identity is safe and rebuilds itself
// exactly when the fog map changes.
// Nyckeln är ETT TAL, inte `${q},${r}`. Strängvarianten allokerade en sträng
// per uppslag, och när dyningens rasterpass började fråga tiotusentals gånger
// per frame var strängbyggandet ensamt merparten av havspassets kostnad.
// Offseten rymmer negativa r (r = rad − ⌊q/2⌋ går ned mot −27 på en 56-bred
// karta) och håller nyckeln långt inom säkra heltal.
const tileKey = (q, r) => (q + 512) * 4096 + (r + 512);
let tileIndex = new Map();
let tileIndexFor;
function terrainAt(q, r) {
  if (tileIndexFor !== State.tileData) {
    tileIndexFor = State.tileData;
    tileIndex = new Map(State.tileData.map(t => [tileKey(t.q, t.r), t.terrain]));
  }
  return tileIndex.get(tileKey(q, r));
}

// Vilka av de sex riktningarna vars granne uppfyller ett villkor. Returnerar
// INDEX i HEX_DIRS, inte riktningsparen, eftersom varje konsument ändå behöver
// indexet för att slå upp kantnormalen (DIR_N nedan).
//
// Fog och utanför-kartan räknas ALDRIG som "öppet" hos någon konsument: den
// spelaren har inte sett rutan, och en kant som formar sig efter en osedd
// granne läcker grannens terräng genom sin egen form. Predikatet får se
// `undefined` och `'fog'` och ska svara false på båda.
function neighborDirs(q, r, pred) {
  const dirs = [];
  for (let i = 0; i < HEX_DIRS.length; i++) {
    if (pred(terrainAt(q + HEX_DIRS[i][0], r + HEX_DIRS[i][1]))) dirs.push(i);
  }
  return dirs;
}

const isSeaTerrain = t => t === 'deep_sea' || t === 'coastal_sea';

// Havet och floden är båda vatten, men de är INTE utbytbara i renderaren, och
// skillnaden är kontrastbudgeten (princip 6): strandbandets ljusa sand är
// reserverad åt kustlinjen. Fick floden samma band skulle varje flodhex rita
// en strand, och kustlinjen — kartans viktigaste gräns — skulle sluta vara
// unik. `isSeaTerrain` styr därför strand, bränning, dyning och djupbryt och
// får ALDRIG innehålla floden; `isWaterTerrain` är den bredare frågan "är det
// här vatten?" som flodens egen kedja och tooltipen ställer.
const isWaterTerrain = t => t === 'deep_sea' || t === 'coastal_sea' || t === 'river';

// Directions from this hex where the woodland ends. Fog and off-map count as
// "unknown", NOT as open ground: the player has not seen those tiles, and
// letting the treeline thin toward them would leak the neighbour's terrain
// through the shape of the edge. Unknown neighbours keep the forest dense.
function openEdges(q, r) {
  return neighborDirs(q, r, t => t && t !== 'fog' && t !== 'forest_olive_grove')
    .map(i => HEX_DIRS[i]);
}

// Kantnormalerna: enhetsvektorn från hexens mitt mot mitten av den kant som
// delas med grannen i riktning i. Med dem blir "hur nära den här punkten ligger
// kanten mot havet" en skalärprodukt, och strand, bränning och dyning kan dela
// EN mekanism i stället för att var och en få sin egen kustgeometri. Det är
// också stoppvillkoret för slicen: behöver en kustform ett eget specialfall är
// mekanismen fel.
// Två platta arrayer och inte en array av par: de här skalärprodukterna körs
// tiotusentals gånger per frame i havs- och strandpassen, och en nästlad
// arrayuppslagning per komponent var mätbart dyrare än allt annat de gör.
const SQRT3 = Math.sqrt(3);
const DIR_NX = new Float64Array(6);
const DIR_NY = new Float64Array(6);
HEX_DIRS.forEach(([dq, dr], i) => {
  const x = 1.5 * dq, y = SQRT3 * (dr + dq / 2);
  const m = Math.hypot(x, y);
  DIR_NX[i] = x / m; DIR_NY[i] = y / m;
});
// Hexens inradie i logiska pixlar — avståndet från mitten till en kant, alltså
// exakt det värde en skalärprodukt mot DIR_N måste nå för att ligga PÅ kanten.
const R_IN = S * Math.sqrt(3) / 2;

// ── World-space value noise ──────────────────────────────────────────────
// The per-hex hash gives every hex its own independent texture, and clipping
// that to the hex is what produced the grid: two neighbouring groves met at a
// hard value step, so the eye read containers rather than terrain. This noise
// is sampled in WORLD pixel coordinates on a global lattice, so the field is
// continuous across hex borders — the clip stays (terrain must not bleed onto
// plains) but between two forest hexes the seam has nothing to reveal.
// `cell` and `stream` are parameters so a terrain can pick its own grain
// without a second copy of the interpolation: the forest floor wants a fine
// 13 px lattice, the plains a much broader one (a parcel of worked land has to
// span more than a hex, or the field cannot read as continuous across the
// clip). Defaults reproduce the forest's original single-purpose version
// exactly, so the grove stays pixel-identical.
const NOISE_CELL = 13;
function noiseAt(wx, wy, cell = NOISE_CELL, stream = 7777) {
  const gx = Math.floor(wx / cell), gy = Math.floor(wy / cell);
  const fx = wx / cell - gx, fy = wy / cell - gy;
  // Smoothstep the interpolation so cells blend into a field instead of
  // reading as the lattice they are built on.
  const sx = fx * fx * (3 - 2 * fx), sy = fy * fy * (3 - 2 * fy);
  const c = (ix, iy) => hash32(ix, iy, stream) / 4294967296;
  const a = c(gx, gy),     b = c(gx + 1, gy);
  const d = c(gx, gy + 1), e = c(gx + 1, gy + 1);
  return (a + (b - a) * sx) * (1 - sy) + (d + (e - d) * sx) * sy;
}

// ── Forest floor ─────────────────────────────────────────────────────────
// Woodland is ground first, canopy second. The flat mid-green fill gave the
// canopy nothing to sit against and made a block of forest hexes read as one
// undifferentiated green field. This lays a shaded understory, patches of bare
// warm earth and low vegetation — all clipped to the hex so nothing bleeds
// into a neighbour's terrain.
// An olive grove is dry country, not a northern wood: the ground is stony
// ochre with sparse sage scrub, and the leaf colour is grey-green — the olive
// leaf's silvery underside is why a grove reads as dusty rather than lush.
// Saturated grass green is the single thing that made this look like a modern
// browser game instead of the Mediterranean.
const FOREST_FLOOR = '#A6AA68'; // shaded, dusty understory
const FOREST_EARTH = '#BCA872'; // dry ochre ground between the trees
const FOREST_SCRUB = '#8A9152'; // sage scrub catching light
const FOREST_EDGE  = '#BDB47C'; // sun-bleached ground where the grove opens up
function drawForestFloor(ctx, cx, cy, q, r) {
  ctx.save();
  hexPath(ctx, hexPts(cx, cy));
  ctx.clip();

  // Everything below is drawn as integer-aligned blocks, never as ellipses.
  // The trees above are pixel sprites, and a soft anti-aliased smudge under a
  // hard-edged sprite reads as two different pictures stacked in one hex.
  //
  // The ground is now ONE CONTINUOUS FIELD, not per-hex specks. Scattered
  // independent marks read as decoration at every scale — that was the finding
  // from both the plains probe and the tree-scale probe. Here the tone of each
  // block comes from world-space noise, so the field runs unbroken from hex to
  // hex and the clip between two groves has nothing to show.
  const STEP = lodStep(3);
  const x0 = Math.floor((cx - S) / STEP) * STEP, x1 = cx + S;
  const y0 = Math.floor((cy - S) / STEP) * STEP, y1 = cy + S;
  for (let wy = y0; wy <= y1; wy += STEP) {
    for (let wx = x0; wx <= x1; wx += STEP) {
      const n = noiseAt(wx, wy);
      // Three bands, thresholded rather than blended: dithered pixel ground,
      // not a gradient. Most of the hex stays in shade; open earth is the
      // exception the eye reads as a gap in the canopy.
      if (n < 0.30)      { ctx.globalAlpha = 0.22; ctx.fillStyle = FOREST_SCRUB; }
      else if (n < 0.66) continue;
      else if (n < 0.84) { ctx.globalAlpha = 0.20; ctx.fillStyle = FOREST_EARTH; }
      else               { ctx.globalAlpha = 0.26; ctx.fillStyle = '#CDBB86'; }
      ctx.fillRect(wx, wy, STEP, STEP);
    }
  }

  // Bryn — where the woodland meets open country the floor lightens toward the
  // neighbour instead of stopping dead at the hex edge.
  //
  // Scattered, not washed: a smooth disc along the edge reads as a halo drawn
  // AROUND the forest, which is decoration, not terrain. These are seeded
  // specks whose density falls off inward (the inward offset is raised to a
  // power, so most sit close to the edge and a few reach deeper), which reads
  // as a treeline breaking up. Specks that cross the edge are cut by the clip,
  // so the opening continues into the neighbouring hex.
  ctx.fillStyle = FOREST_EDGE;
  const mid = S * Math.sqrt(3) / 2; // centre → edge midpoint
  openEdges(q, r).forEach(([dq, dr], e) => {
    const ex = S * 1.5 * dq;
    const ey = S * Math.sqrt(3) * (dr + dq / 2);
    const len = Math.hypot(ex, ey) || 1;
    const ux = ex / len, uy = ey / len;
    for (let i = 0; i < 7; i++) {
      const along  = (rnd(q, r, 200 + e * 20 + i) - 0.5) * S * 1.15;
      const inward = Math.pow(rnd(q, r, 300 + e * 20 + i), 1.7) * S * 0.6;
      const px = cx + ux * (mid - inward) - uy * along;
      const py = cy + uy * (mid - inward) + ux * along;
      // Per-speck coverage: a uniform alpha over many small shapes reads as
      // film grain. Varying it makes the same specks read as depth instead.
      ctx.globalAlpha = 0.22 + rnd(q, r, 600 + e * 20 + i) * 0.3;
      ctx.fillRect(Math.round(px), Math.round(py),
                   2 + rndInt(q, r, 400 + e * 20 + i, 4),
                   1 + rndInt(q, r, 500 + e * 20 + i, 2));
    }
  });
  ctx.restore();
}

// ── Canopy ───────────────────────────────────────────────────────────────
// Trees grow in stands, not in an even scatter. Three lone dots per hex read
// as a repeating pattern; composed clumps with air between them read as
// woodland. Two rules carry the look:
//
//   Dark core, warm edge — crowns deepen toward the middle of the hex and
//   warm toward its rim, so a block of forest gets an inside and an outside.
//   Thinning at the bryn — a clump close to open country is dropped or
//   shrunk, so the treeline the floor already opens up is not contradicted
//   by trees standing to attention right at the edge.
//
// Everything is seeded per hex and clipped to it, like the floor.
// Trees are drawn as PIXEL SPRITES, not as arcs. An arc anti-aliases, and the
// canvas is scaled (zoom × SCALE), so a smooth curve gets resampled into mush
// at every zoom level — which is what made the old crowns read as flat
// boardgame counters. fillRect on integer coordinates scales into hard-edged
// blocks instead, which is the whole point of the pixel-art rule in
// temenos_designprinciper (1px charcoal outline, no anti-aliasing, no gradients).
//
// K charcoal outline · D shadowed side · M body · L sunlit side · T trunk
// Light falls from the upper left, so every sprite runs L → M → D across its
// width. A crown shaded on one side is what separates a tree from a green
// counter; the old arcs had a highlight dot but no shaded side at all.
//
// Shape matters as much as colour here. A round lollipop crown on a thin stem
// is a storybook tree — an olive is LOW and WIDE, its crown often split into
// two lobes, and it stands on a thick gnarled trunk you can actually see. The
// gaps punched in the canopy are deliberate: an olive crown is open enough to
// see sky through, and that sparseness is most of what makes a grove read as
// Mediterranean rather than as a forest of broccoli.
// Bigger trees, fewer of them. A small tree can only ever be a blob: at seven
// pixels there is no room for an irregular edge or for shading INSIDE the
// crown, so the silhouette collapses to an oval and reads as cute. At thirteen
// there is room for both.
//
// S is the fourth tone — shade within the foliage, not at its rim. It runs in
// short connected strokes, never as single pixels: an isolated dark pixel in a
// light crown reads as a speck or an eye, while a two-or-three pixel stroke
// reads as the gap between two boughs. It is what
// turns a solid lump into a crown of separate boughs, and it does more for the
// "richness" of a tree than any amount of outline detail. The silhouette is
// deliberately asymmetric and lumpy: a real olive crown is knotted, never an
// oval, and symmetry is most of what made these look like storybook trees.
const TREE_LARGE = [
  '.EE.EE.',
  'ELMMMDE',
  'ELMMMDE',
  'ELMSMDE',
  '.KMMDK.',
  '..ETE..',
  '..ETE..',
];
const TREE_MID = [
  '.EEE.',
  'ELMDE',
  'ELMDE',
  '.KMK.',
  '..T..',
  '..T..',
];
const TREE_SMALL = [
  '.EE..',
  'ELMDE',
  '.KMD.',
  '..T..',
];
const TREE_PALETTE = { K: '#1E2610', E: '#263013', S: '#283315', D: '#33401B', M: '#465428', L: '#5E6E38', T: '#4A3D28' };

// Same sprites at the sun-bleached end of the ramp — used for stands at the
// rim of the grove so a block of trees keeps an inside and an outside.
const TREE_PALETTE_WARM = { K: '#283014', E: '#303A18', S: '#323C1A', D: '#3E4C22', M: '#556430', L: '#708044', T: '#544632' };

// Träden delar sprite-hjälparen med kartaktörerna (render/actorsprites.js):
// en fillRect per horisontell löpa i stället för en per pixel, så en hex
// med träd kostar tiotals ritanrop i stället för hundratals.
const SPRITE_LARGE = spriteRuns(TREE_LARGE);
const SPRITE_MID = spriteRuns(TREE_MID);
const SPRITE_SMALL = spriteRuns(TREE_SMALL);

// Origin is the trunk's foot, so trees "stand" on their position and a lower
// tree overlaps a higher one correctly when the stand is drawn back to front.
function drawTree(ctx, sprite, palette, fx, fy) {
  const ox = Math.round(fx) - (sprite.w >> 1);
  const oy = Math.round(fy) - sprite.h;
  for (const r of sprite.runs) {
    ctx.fillStyle = palette[r.ch];
    ctx.fillRect(ox + r.x, oy + r.y, r.n, 1);
  }
}

// NOT clipped to the hex, unlike the floor. Once the stand reached the rim, the
// clip sliced crowns clean off along the hex border — straight cuts through the
// foliage, which is worse than the bare margin it replaced. A crown that hangs
// a few pixels over the border interleaves with the neighbouring hex's trees
// and reads as continuous woodland. That only works because the canopy is a
// separate pass: inside the tile loop, the next tile's ground would paint over
// the previous tile's overhanging branches.
function drawCanopy(ctx, cx, cy, q, r) {
  ctx.save();
  const mid = S * Math.sqrt(3) / 2;
  const edges = openEdges(q, r).map(([dq, dr]) => {
    const ex = S * 1.5 * dq;
    const ey = S * Math.sqrt(3) * (dr + dq / 2);
    const len = Math.hypot(ex, ey) || 1;
    return { x: (ex / len) * mid, y: (ey / len) * mid };
  });

  // Collect the stand first, then draw it back to front. The map is seen from
  // above but the trees are drawn in side elevation (¾, as in Settlers 2 /
  // Colonization): a tree stands on its position, so one further down the hex
  // must overlap one further up, and that only works if they are sorted.
  const stand = [];
  const masses = [];   // gemensam mörk volym per dunge, ritad före träden
  // Trees have to reach the hex's rim, not huddle in the middle. The hex is 19
  // across to an edge and 22 to a corner; when the stand only occupied the
  // inner ~16 the result read as an ICON placed in the centre to stand for the
  // hex, with a bare margin all round — a symbol of woodland rather than
  // woodland. Clump centres now spread to 13 and their trees a further 7, so
  // the grove runs out to the edges and meets the neighbouring hex's grove.
  const clumps = 2 + rndInt(q, r, 2, 2);
  for (let c = 0; c < clumps; c++) {
    const a = rnd(q, r, 700 + c) * Math.PI * 2;
    const d = Math.sqrt(rnd(q, r, 710 + c)) * 14;
    const gx = Math.cos(a) * d, gy = Math.sin(a) * d;

    // 0 deep inside the wood → 1 standing on an open edge.
    let openness = 0;
    for (const e of edges) {
      openness = Math.max(openness, 1 - Math.hypot(gx - e.x, gy - e.y) / (S * 0.9));
    }
    openness = Math.min(1, Math.max(0, openness));
    if (rnd(q, r, 720 + c) < openness * 0.55) continue;

    masses.push({ c, gx, gy, rad: 9 + rnd(q, r, 1580 + c) * 5 });
    const trees = 1 + rndInt(q, r, 730 + c, 2);
    for (let i = 0; i < trees; i++) {
      const ta = rnd(q, r, 740 + c * 8 + i) * Math.PI * 2;
      const td = rnd(q, r, 800 + c * 8 + i) * 6;
      // Three sizes mixed: a stand of identical silhouettes reads as a repeated
      // stamp. Out at the bryn the big ones drop away first, so the treeline
      // thins by losing its mature trees rather than by fading uniformly.
      const roll = rnd(q, r, 860 + c * 8 + i);
      const sprite = openness > 0.7 ? SPRITE_SMALL
                   : roll > 0.55 ? SPRITE_LARGE
                   : roll > 0.22 ? SPRITE_MID
                   : SPRITE_SMALL;
      stand.push({
        x: cx + gx + Math.cos(ta) * td,
        y: cy + gy + Math.sin(ta) * td,
        sprite,
        warm: openness > 0.35 || d > 8.5,
      });
    }
  }
  // Solitaries. Clumps alone leave wide empty lanes between them, and the eye
  // reads those lanes as "the hex is mostly bare with some tree groups on it".
  // A grove has strays: single trees out on their own between the stands, which
  // is what turns three groups into continuous tree cover without closing the
  // canopy into a Nordic forest.
  const strays = 1 + rndInt(q, r, 3, 3);
  for (let s = 0; s < strays; s++) {
    const a = rnd(q, r, 900 + s) * Math.PI * 2;
    const d = Math.sqrt(rnd(q, r, 920 + s)) * 18;
    const sx = Math.cos(a) * d, sy = Math.sin(a) * d;
    let openness = 0;
    for (const e of edges) {
      openness = Math.max(openness, 1 - Math.hypot(sx - e.x, sy - e.y) / (S * 0.9));
    }
    openness = Math.min(1, Math.max(0, openness));
    if (rnd(q, r, 940 + s) < openness * 0.5) continue;
    // Strays skew small: a lone mature olive out in the open would read as a
    // landmark, and every hex would have three of them.
    const roll = rnd(q, r, 960 + s);
    stand.push({
      x: cx + sx,
      y: cy + sy,
      sprite: roll > 0.72 && openness < 0.5 ? SPRITE_MID : SPRITE_SMALL,
      warm: openness > 0.35 || d > 11,
    });
  }

  // Shared under-mass, painted BEFORE any tree. A cluster of separate sprites
  // reads as N objects standing near each other; the same cluster over one dark
  // volume reads as a single canopy with trees resolving out of it. This is the
  // difference between "trees on a hex" and "a wood" — and it is why counting
  // or shrinking sprites alone never produced mass.
  // Built from many small offset blocks, not a few big ones: five large
  // axis-aligned rectangles read as exactly that — rectangles. A dozen small
  // ones at varying offsets dissolve into an irregular volume, and the blocky
  // edge stays consistent with the pixel idiom instead of fighting it.
  ctx.globalAlpha = 0.14;
  ctx.fillStyle = '#2A3318';
  for (const m of masses) {
    for (let i = 0; i < 14; i++) {
      const a = rnd(q, r, 1500 + m.c * 20 + i) * Math.PI * 2;
      const d = Math.sqrt(rnd(q, r, 1540 + m.c * 20 + i)) * m.rad * 0.8;
      const w = 3 + rndInt(q, r, 1580 + m.c * 20 + i, 5);
      const h = 2 + rndInt(q, r, 1620 + m.c * 20 + i, 4);
      ctx.fillRect(Math.round(cx + m.gx + Math.cos(a) * d - w / 2),
                   Math.round(cy + m.gy + Math.sin(a) * d - h / 2), w, h);
    }
  }
  ctx.globalAlpha = 1;

  stand.sort((a, b) => a.y - b.y);

  for (const t of stand) {
    // Ground shadow cast down-right: the light in this world comes from the
    // upper left (same side the sprites are lit on). It is what makes a tree
    // read as standing up rather than lying flat on the hex.
    ctx.globalAlpha = 0.38;
    ctx.fillStyle = '#1A2A10';
    ctx.fillRect(Math.round(t.x) - 1, Math.round(t.y) - 1, t.sprite.w - 1, 2);
    ctx.fillRect(Math.round(t.x) + 1, Math.round(t.y), t.sprite.w - 2, 1);
    ctx.globalAlpha = 1;
    drawTree(ctx, t.sprite, t.warm ? TREE_PALETTE_WARM : TREE_PALETTE, t.x, t.y);
  }
  ctx.restore();
}

// ── The plains ───────────────────────────────────────────────────────────
// The old plains were four wheat stalks scattered on a flat saturated green,
// and they broke three of the rendering rules at once: isolated marks on flat
// colour read as decoration at any scale, the stalks were the brightest thing
// on the whole map (the contrast budget reserves that for coastline, mountain
// edge, buildings and units), and they were drawn as anti-aliased strokes on
// fractional coordinates, which the scaled canvas resamples into mush.
// What replaces them is measured off the reference image, not invented, because
// eye and measurement disagreed twice here and the measurement was right twice.
//
// Tone: clean patches of the reference's green ground cluster at 8A964A, 8D9947,
// 7D8C47, 8C994A, 818B46, 7F8C46 — mean #859248, DARKER and far less saturated
// than the old #8AAF3A (green 175→146, blue 58→72). The same sampling settles a
// question the palette could not: the reference's dry gold ground is LIGHTER
// than its fertile green, so open country is not the bright part of a
// Mediterranean map. The grove keeps the pale-khaki slot it took yesterday and
// the plains sits darker beside it — which is also what pulls scrub_maquis
// (#A8B860) back out of collision with the plains.
//
// Amount: those clean patches run at a standard deviation of only 1–9, while
// the reference's dry gold ground runs 12–29. The green plain is the map's QUIET
// surface on purpose — texture belongs to the dry country, the mountains and the
// coast. So the field below is deliberately faint. The goal is that a block of
// plains stops being one flat colour, not that it becomes interesting.
const PLAINS_CELL = 78;    // a parcel of worked land spans several hexes
const PLAINS_DARK  = '#74813C'; // shaded ground, the hollow of a field
const PLAINS_LIGHT = '#93A053'; // ground catching the light
const PLAINS_DRY   = '#A0A159'; // baked-out patch where the crop thins

// Dither, so the bands meet as hard blocks instead of a soft smudge — a soft
// smudge under hard-edged pixel sprites reads as two different pictures stacked
// in one hex. PLAINS_CELL has to be this wide for the dither to be visible at
// all: the ±amplitude only spans several blocks if the field's gradient is
// gentle.
//
// REJECTED on the way here: an ordered 4×4 Bayer matrix, the textbook choice.
// On a gradient this slow it produces large runs of pure checkerboard, and a
// literal chessboard weave is the one texture this map must never have. Ordered
// dither's regularity IS the artefact. A per-block random threshold scatters
// the crossing instead and reads as ground.
const PLAINS_DITHER = 0.19;
// No q,r parameter: unlike the forest floor this field is purely world-space,
// which is the whole point of it.
function drawPlainsField(ctx, cx, cy) {
  ctx.save();
  hexPath(ctx, hexPts(cx, cy));
  ctx.clip();

  // Blocks on integer coordinates aligned to a GLOBAL lattice (floor to STEP,
  // not relative to the hex centre): two neighbouring plains hexes must place
  // their blocks on the same grid, or the field betrays the tile boundary it is
  // supposed to hide.
  const STEP = lodStep(4);
  const x0 = Math.floor((cx - S) / STEP) * STEP, x1 = cx + S;
  const y0 = Math.floor((cy - S) / STEP) * STEP, y1 = cy + S;
  for (let wy = y0; wy <= y1; wy += STEP) {
    for (let wx = x0; wx <= x1; wx += STEP) {
      // One independent draw per block, indexed on the GLOBAL block lattice so
      // the scatter is continuous across hex borders like the field itself.
      const jitter = hash32(Math.floor(wx / STEP), Math.floor(wy / STEP), 5151)
                     / 4294967296 - 0.5;
      // The field is ISOTROPIC on purpose. "Weak direction" was in the plan and
      // was tried here — the noise sampled through a ~23° rotation with one axis
      // compressed to 0.58 — and it made things worse: parcels stretched so far
      // that whole regions of screen went uniform, so the map read as LESS
      // structured, and the lie it implied answered to nothing in the world
      // (real field orientation follows contour and road, which Temenos does not
      // model — princip 1, detail must arise from structure).
      const n = noiseAt(wx, wy, PLAINS_CELL, 3131) + jitter * PLAINS_DITHER;
      if (n < 0.30)      { ctx.globalAlpha = 0.34; ctx.fillStyle = PLAINS_DARK; }
      else if (n < 0.63) continue;   // the base tone IS the dominant band
      else if (n < 0.86) { ctx.globalAlpha = 0.30; ctx.fillStyle = PLAINS_LIGHT; }
      else               { ctx.globalAlpha = 0.26; ctx.fillStyle = PLAINS_DRY; }
      ctx.fillRect(wx, wy, STEP, STEP);
    }
  }
  ctx.globalAlpha = 1;
  ctx.restore();
}

// ── Torrmarken — halvöknen och skruben ───────────────────────────────────
// De två sista legacy-markterrängerna, och EN slice därför att de delade rot:
// båda ritade isolerade märken på platt färg (fem `ctx.arc()` r=1,5 respektive
// tre 1×1-rutor), båda med kantutjämning som pixelidiomet förbjuder, och båda
// ur den gamla per-hex-seeden `(q*137+r*31)&0xff` — så texturen började om vid
// varje hexkant och hexen ritade sig själv (princip 2, 10, 14).
//
// VAR TEXTUREN HÖR HEMMA. Princip 15 säger att den största ytan ska vara den
// tystaste, och slätten är den ytan. Samma mätning ur referensen säger
// motsatsen om det HÄR registret: den gröna slätten löper på sd 1–9, den torra
// guldmarken på 12–29. Halvöknen är alltså den terräng där struktur är rätt
// svar — och den låg på sd 1,7, kartans plattaste yta, plattare än djuphavet.
//
// GRANNSKAPET STYR TONVALET, INTE TABELLEN. Räknat på världsfixturen (2 240
// hexar): skruben gränsar 67 gånger mot slätt, 35 mot kullar, 19 mot halvöken
// och 13 mot kustvatten. Halvöknen gränsar BARA mot skrub (19) och slätt (7) —
// noll gånger mot kullarna. Det förklarar mapgens egen tabell (`terrainTable`,
// mapgen.go): halvöken är bandMid+zoneArid, kullar är bandMid+zoneMoist, och
// mellan dem ligger zoneDry = skrub. De två kan bara mötas där fuktfältet
// hoppar två zoner över en hexkant. Det uppmätta paret `hills ↔ semi_desert`
// 3,9 är alltså sant OCH svarar på en fråga kartan inte ställer; den gräns som
// finns på riktigt är `hills ↔ scrub` (35 kanter, 2,3 L isär i markton).
const DRY_STEP = 3;   // samma korn som skogsbottnen — torr mark har finare grus än en åker

// TONVALET, OCH VARFÖR MITTEN INTE VAR SVARET. Skruben låg på 169 i markton,
// 2,3 från kullarna. Det matematiska max-min-läget mot dess två stora grannar
// är mitt emellan slätten (137) och kullarna (171), alltså 152 — och det
// PRÖVADES och föll vid 1:1: markkolumnen låg då mycket riktigt 15 L från
// båda, men hexens MEDELVÄRDE sjönk till 138,6 mot slättens 132,7, och
// gränsen skrub/slätt försvann ur bilden. Det är den vanligaste gränsen på
// kartan (67 kanter). Parningsmåttet är p75 och ögat läser medelvärdet; när
// en yta bär mörka objekt går de isär, och då är p75 ensamt fel grind.
//
// 162 är därför valt: kullarna 9,7 · slätten 24,5 i markton (medelvärden 153,5
// mot 132,7), halvöknen 28. Kvar som misstänkt par är OLIVLUNDENS GOLV (154,
// 7,8 ifrån) — medvetet betalt. Lunden är zoneWet och skruben zoneDry, alltså
// samma två-zoners-hopp som ovan (noll delade kanter i världsfixturen), och en
// lundhex LÄSER 20 L mörkare än sitt golv därför att kronorna ligger över det.
// Ingen ton klarar 12 L mot alla fyra — fyra marktoner delar redan spannet
// 137–190 — så valet står mellan vilken kollision man tar, och den mellan två
// terränger som aldrig möts är den billiga.
//
// Hue bär det som valören inte kan: lundens golv är khaki (varmt gult),
// skruben är gråsage (blått höjt). Det är också den sanna färgen — maquis är
// städsegrön hårdbladsvegetation med grå, läderartade blad, inte gräs.
const SCRUB_CELL  = 26;   // markens fläckighet — ett par snårlängder, inte ett par hexar
const SCRUB_CLUMP = 7;    // ETT snår: 5–8 px, ungefär ett träd i lunden
const SCRUB_BUSH  = '#5F6C3C'; // snårets massa
const SCRUB_CROWN = '#AFB87E'; // 1 px sol på snårets överkant

// Täckningen: `clump` moduleras av det grova fältet så att snåren står tätare
// i svackorna. Det är princip 1 — detaljen kommer ur en struktur (fukten
// följer terrängen) i stället för att strös ut för att ytan känns tom.
//
// Snårcellen är 7 px och inte 15: vid 15 blev "snåren" fläckar på en tredjedels
// hex och hela fältet läste som molnskuggor, alltså FORM där princip 3 kräver
// textur. Skalan är densamma som lundens träd (6–8 px) med flit — ett snår och
// ett olivträd är samma storleksordning i verkligheten, och kartan får inte
// säga något annat.
function scrubBush(wx, wy) {
  const coarse = noiseAt(wx, wy, SCRUB_CELL, 6161);
  return noiseAt(wx, wy, SCRUB_CLUMP, 6262) + 0.34 * (1 - coarse) > 0.86;
}

function drawScrubField(ctx, cx, cy) {
  ctx.save();
  hexPath(ctx, hexPts(cx, cy));
  ctx.clip();

  // Globalt raster, aldrig relativt hexmitten: två skrubhexar bredvid varandra
  // måste lägga sina block på samma rutnät, annars förråder fältet exakt den
  // brickkant det finns för att dölja.
  // EN bärande frekvens, och det är snåren. Ett eget markfält låg här och föll
  // på två mätningar i rad: med kontrast nog att synas konkurrerade det med
  // snåren och ytan läste som kamouflage (samma fel som bergsstrieringens
  // jämnt fördelade toner, princip 30), och nedskruvat till knappt synligt
  // kostade det 9,9 ms av `ground` vid minzoom för en skillnad man får leta
  // efter i en A/B. Bastonen räcker som mark; snåren gör resten.
  const step = lodStep(DRY_STEP);
  const x0 = Math.floor((cx - S) / step) * step, x1 = cx + S;
  const y0 = Math.floor((cy - S) / step) * step, y1 = cy + S;
  for (let wy = y0; wy <= y1; wy += step) {
    for (let wx = x0; wx <= x1; wx += step) {
      // Snåren. Sammanhängande fläckar ur ett fält, inte N spridda märken —
      // det är hela skillnaden mot de fem cirklarna som stod här. Överkanten
      // får 1 px sol: princip 4 säger att massa uppstår ur gemensam undervolym
      // och gemensamt valörfält, och en fläck utan ljus på sin övre gräns är
      // en fläck. Kanten följer FLÄCKENS form, inte blockets, eftersom den
      // läses ur samma villkor en rad upp.
      if (!scrubBush(wx, wy)) continue;
      ctx.globalAlpha = 0.66;
      ctx.fillStyle = SCRUB_BUSH;
      ctx.fillRect(wx, wy, step, step);
      if (!scrubBush(wx, wy - step)) {
        ctx.globalAlpha = 0.70;
        ctx.fillStyle = SCRUB_CROWN;
        ctx.fillRect(wx, wy, step, 1);
      }
    }
  }
  ctx.globalAlpha = 1;
  ctx.restore();
}

// Halvöknen: två frekvenser och ingenting annat. Den breda är basängen —
// bakad skorpa på höjderna, mörkare svacka där det samlas fukt nog för något
// att gro. Den fina är gruset: hamadan är den yta finkornet blåst bort ifrån,
// alltså ligger stenen kvar DÄR VINDEN SKALAT AV, och tätheten följer därför
// det grova fältet i stället för att vara jämn. En jämnt strödd grusmatta är
// filmkorn; en som följer basängen är mark.
const DESERT_CELL   = 58;
const DESERT_HOLLOW = '#B99757'; // skuggad svacka
const DESERT_CRUST  = '#E4CC94'; // solbakad skorpa
const DESERT_BLEACH = '#F2E0B4'; // blekt damm i det torraste
const DESERT_GRIT   = '#A6884E'; // grus som blivit kvar
const DESERT_DITHER = 0.22;      // per-block-brus, aldrig ordnad dither (princip 16)

function drawDesertField(ctx, cx, cy) {
  ctx.save();
  hexPath(ctx, hexPts(cx, cy));
  ctx.clip();

  const step = lodStep(DRY_STEP);
  const x0 = Math.floor((cx - S) / step) * step, x1 = cx + S;
  const y0 = Math.floor((cy - S) / step) * step, y1 = cy + S;
  for (let wy = y0; wy <= y1; wy += step) {
    for (let wx = x0; wx <= x1; wx += step) {
      const gx = Math.floor(wx / step), gy = Math.floor(wy / step);
      const jitter = hash32(gx, gy, 5252) / 4294967296 - 0.5;
      const n = noiseAt(wx, wy, DESERT_CELL, 2727) + jitter * DESERT_DITHER;
      // Basbandet är smalt med flit. På slätten ÄR bastonen det dominerande
      // bandet (princip 15); här är det tvärtom, för referensens torra guldmark
      // är kartans mest texturerade yta och halvöknen är dess enda representant.
      if (n < 0.34)      { ctx.globalAlpha = 0.44; ctx.fillStyle = DESERT_HOLLOW; }
      else if (n < 0.53) { ctx.globalAlpha = 0;    }
      else if (n < 0.80) { ctx.globalAlpha = 0.40; ctx.fillStyle = DESERT_CRUST; }
      else               { ctx.globalAlpha = 0.34; ctx.fillStyle = DESERT_BLEACH; }
      if (ctx.globalAlpha) ctx.fillRect(wx, wy, step, step);

      // Gruset. En enda logisk pixel, satt på en hash-vald plats inne i
      // blocket — en 3×3-fläck vore en stenhäll, inte grus.
      const g = hash32(gx, gy, 3939) / 4294967296;
      if (g > 0.30 * (1 - n)) continue;
      ctx.globalAlpha = 0.5;
      ctx.fillStyle = DESERT_GRIT;
      ctx.fillRect(wx + (hash32(gx, gy, 4141) % step),
                   wy + (hash32(gx, gy, 4242) % step), 1, 1);
    }
  }
  ctx.globalAlpha = 1;
  ctx.restore();
}

// ── Cederskogen ──────────────────────────────────────────────────────────
// Princip 8, ordagrant: *"Skogen där arméer försvinner ÄR cederskogen."*
// Olivlunden är en ODLING — planterad, gles, torr, medvetet framkomlig — och
// renderas därför som mörka träd mot ljus mark. Cedern är VILDMARK, och
// princip 7 säger att separationsriktningen då ska vändas: ljusa kronor som
// löser ut ur en mörk sluten massa. De två är alltså inte ljus och mörk
// variant av samma skog; de är varandras motsatser i varje parameter, och det
// är precis vad som gör att de går att skilja åt där de möts (princip 20 —
// skillnaden bor i den stora formen, aldrig i småpixlar).
//
// Formen är cederns SANNA form (princip 19) och inte ett generiskt träd:
// libanonceder växer i vågräta våningar med en platt bred hjässa och en synlig
// stam mellan våningarna. En kon hade varit gran, en klump hade varit oliv —
// och båda hade gjort de två skogarna till samma skog i två toner.
//
// Skillnaden mot lunden sitter dessutom i BRYNET. Lunden tunnas ut mot öppen
// mark: dess `openness` plockar bort träd nära kanten, för en odling fransar
// ut. Cedern gör tvärtom och står tät ända ut — en cederskog möter slätten som
// en vägg, och det är den väggen som är hela det spelmässiga löftet om att
// arméer försvinner här.
const CEDAR_LARGE = [
  '...LLL...',
  '..LMMML..',
  '.DMMMMMD.',
  '..DMTMD..',
  '.LMMMMML.',
  'DMMMMMMMD',
  '..DMTMD..',
  '.LMMMML..',
  '.DMMMMD..',
  '....T....',
  '....T....',
];
const CEDAR_MID = [
  '..LLL..',
  '.LMMML.',
  'DMMMMMD',
  '..DTD..',
  '.LMMML.',
  'DMMMMMD',
  '...T...',
  '...T...',
];
const CEDAR_SMALL = [
  '.LLL.',
  'LMMML',
  '.DTD.',
  'LMMML',
  '..T..',
  '..T..',
];
// Stammen är rödbrun med flit: cederträ ÄR rött, och det är den enda pixeln i
// hela hexen som säger vilken vara skogen bär.
const CEDAR_PALETTE = { L: '#7A8A52', M: '#485A34', D: '#2C3A22', T: '#4A3A2A' };
// Brynets träd står i fullt ljus. Samma sprite, ljusare ramp — så ett block
// cedrar får en insida och en utsida utan att en enda kontur ritas.
const CEDAR_PALETTE_RIM = { L: '#94A266', M: '#5C6E42', D: '#3A4A2C', T: '#5A4634' };

const SPRITE_CEDAR_LARGE = spriteRuns(CEDAR_LARGE);
const SPRITE_CEDAR_MID   = spriteRuns(CEDAR_MID);
const SPRITE_CEDAR_SMALL = spriteRuns(CEDAR_SMALL);

const CEDAR_LITTER = '#3E4C30'; // barrförna i skugga
const CEDAR_MOSS   = '#5A6A40'; // mossa i en glänta
const CEDAR_DUFF   = '#6A6A44'; // torr förna där taket öppnar sig

// Marken är EN bärande frekvens (princip 41). Cederskogens golv syns knappt —
// taket är slutet — så fältet är till för att bastonen inte ska vara platt där
// den ändå skymtar, ingenting mer.
function drawCedarFloor(ctx, cx, cy) {
  ctx.save();
  hexPath(ctx, hexPts(cx, cy));
  ctx.clip();
  const STEP = lodStep(3);
  const x0 = Math.floor((cx - S) / STEP) * STEP, x1 = cx + S;
  const y0 = Math.floor((cy - S) / STEP) * STEP, y1 = cy + S;
  for (let wy = y0; wy <= y1; wy += STEP) {
    for (let wx = x0; wx <= x1; wx += STEP) {
      const n = noiseAt(wx, wy, 21, 8181);
      if (n < 0.34)      { ctx.globalAlpha = 0.34; ctx.fillStyle = CEDAR_LITTER; }
      else if (n < 0.72) continue;
      else if (n < 0.89) { ctx.globalAlpha = 0.24; ctx.fillStyle = CEDAR_MOSS; }
      else               { ctx.globalAlpha = 0.20; ctx.fillStyle = CEDAR_DUFF; }
      ctx.fillRect(wx, wy, STEP, STEP);
    }
  }
  ctx.globalAlpha = 1;
  ctx.restore();
}

// Kronpasset. Oklippt av samma skäl som lunden: en krona måste få hänga över
// hexgränsen och fläta ihop sig med grannens, annars läser varje hex som en
// bricka med skog ritad i (princip 12 + 31).
function drawCedarCanopy(ctx, cx, cy, q, r) {
  ctx.save();

  // Var skogen möter något annat — men till skillnad från lunden används det
  // INTE för att tunna ut beståndet, bara för att ljussätta brynet.
  const mid = S * Math.sqrt(3) / 2;
  const rim = neighborDirs(q, r, t => t && t !== 'fog' && t !== 'forest_cedar')
    .map(i => ({ x: DIR_NX[i] * mid, y: DIR_NY[i] * mid }));

  const stand = [];
  const masses = [];
  const clumps = 4 + rndInt(q, r, 2102, 2);
  for (let c = 0; c < clumps; c++) {
    const a = rnd(q, r, 2110 + c) * Math.PI * 2;
    const d = Math.sqrt(rnd(q, r, 2130 + c)) * 15;
    const gx = Math.cos(a) * d, gy = Math.sin(a) * d;
    masses.push({ c, gx, gy, rad: 10 + rnd(q, r, 2150 + c) * 6 });

    const trees = 2 + rndInt(q, r, 2170 + c, 3);
    for (let i = 0; i < trees; i++) {
      const ta = rnd(q, r, 2200 + c * 8 + i) * Math.PI * 2;
      const td = rnd(q, r, 2260 + c * 8 + i) * 7;
      const tx = gx + Math.cos(ta) * td, ty = gy + Math.sin(ta) * td;
      let lit = 0;
      for (const e of rim) lit = Math.max(lit, 1 - Math.hypot(tx - e.x, ty - e.y) / (S * 0.85));
      const roll = rnd(q, r, 2320 + c * 8 + i);
      stand.push({
        x: cx + tx, y: cy + ty,
        sprite: roll > 0.52 ? SPRITE_CEDAR_LARGE : roll > 0.20 ? SPRITE_CEDAR_MID : SPRITE_CEDAR_SMALL,
        rim: lit > 0.42,
      });
    }
  }

  // Den gemensamma undervolymen — samma grepp som lunden, men mycket tätare
  // och mörkare, för ett cedertak är SLUTET. Princip 4: massa uppstår av
  // gemensam undervolym, aldrig av antal eller storlek.
  // Alpha 0,15 och MÅNGA block, inte 0,30 och få. Vid 0,30 läste varje block
  // som en egen rektangel — ett rutmönster av mörka fyrkanter bakom träden,
  // alltså precis princip 29:s fälla i undervolymen i stället för i basen.
  // Volym byggs av överlappning vid låg opacitet; opacitet per block bygger
  // bara block.
  ctx.globalAlpha = 0.15;
  ctx.fillStyle = '#1E2A18';
  for (const m of masses) {
    for (let i = 0; i < 26; i++) {
      const a = rnd(q, r, 2400 + m.c * 32 + i) * Math.PI * 2;
      const d = Math.sqrt(rnd(q, r, 2460 + m.c * 32 + i)) * m.rad * 0.9;
      const w = 3 + rndInt(q, r, 2520 + m.c * 32 + i, 5);
      const h = 2 + rndInt(q, r, 2580 + m.c * 32 + i, 4);
      ctx.fillRect(Math.round(cx + m.gx + Math.cos(a) * d - w / 2),
                   Math.round(cy + m.gy + Math.sin(a) * d - h / 2), w, h);
    }
  }
  ctx.globalAlpha = 1;

  stand.sort((a, b) => a.y - b.y);
  for (const t of stand) {
    ctx.globalAlpha = 0.42;
    ctx.fillStyle = '#141E10';
    ctx.fillRect(Math.round(t.x) - 1, Math.round(t.y) - 1, t.sprite.w - 1, 2);
    ctx.globalAlpha = 1;
    drawTree(ctx, t.sprite, t.rim ? CEDAR_PALETTE_RIM : CEDAR_PALETTE, t.x, t.y);
  }
  ctx.restore();
}

// ── Flodfamiljen ─────────────────────────────────────────────────────────
// Floden är sedan Timothys beslut 2026-07-29 en egen VATTENterräng: en
// seglingsbar kedja, exakt en hex bred, källa → Thalassa, ogenomtränglig för
// landenheter. Princip 20 förbjöd att rita henne så länge mekaniken saknades;
// nu finns mekaniken, alltså gäller förbudet inte längre.
//
// Felläget mättes innan något ritades: med bara bastonen läser en flodkedja
// som EN RAD BLÅ BRICKOR. Roten är geometrisk och inte kolorimetrisk — en hex
// är en fet hexagon, så en kedja av fyllda hexar blir en kedja av romber, inte
// en linje. Det är alltså BREDDEN som måste bort, inte tonen.
//
// Greppet: en mörk vassbård dras inåt från varje kant mot LAND, och det som
// blir kvar är en ljusare fåra som med nödvändighet löper längs flödesaxeln —
// den axel som ges av vilka grannar som är vatten. Bågen uppstår därmed ur
// KEDJANS STRUKTUR och inte ur en ritad linje (princip 1), och den ändrar form
// av sig själv i rakt genomlopp, i krök, vid källan och vid mynningen.
//
// Varför bården är MÖRK och inte ljus, tvärtemot kustens sand: kontrasten ska
// sitta i den kant spelaren fattar beslut om, och den kanten är flod↔dal — de
// två delar varje hexkant per konstruktion. En ljus bård hade lagt flodens
// ljusaste ton intill dalens ljusa mark (ΔL 6) och suddat just den gränsen; en
// mörk bård ger ΔL ~36 vid själva mötet. Samma lärdom som röda berget mot
// kustvattnet: mät VID gränsen, inte mellan två hexmedelvärden.
//
// Bården är dessutom textur och aldrig form (princip 3): bredden moduleras av
// världsrymdsbruset, så kanten fransar i stället för att rita en jämn skiva
// innanför hexen — en jämn skiva blev en gloria runt skogen när lunden prövade
// det, och den skulle bli en hexagonkontur här.
// Bården är VASS OCH VÅT DY, inte vatten — och det är slicens avgörande fynd.
// Första ansatsen gjorde bården till en mörkare blå: gränsen dal→bård mätte då
// ΔL 31,6, alltså en helt godkänd siffra, och kedjan läste ÄNDÅ som romber.
// Roten är geometrisk och inte kolorimetrisk: så länge vattnet når fram till
// hexkanten ÄR gränsen vatten↔land en hexagon, hur man än tonar insidan. Ingen
// behandling av ytan kan laga en kontur. Vattnet måste alltså sluta INNAN
// kanten, och det som ligger emellan kan inte vara vatten.
//
// Att bården dessutom är MÖRKARE än fåran är vad som gör att tråden läser som
// vatten: strömmen blir det ljusa elementet mellan två mörka stränder, i
// stället för en mörk fläck i ljus mark. Det är också sant — vass och våt dy i
// skugga är det mörkaste i en flodslätt — och det håller kontrastbudgeten
// (princip 6): flodens ytterligheter är blygsamma, kustlinjens ljusa sand är
// fortfarande kartans starkaste ljus.
const RIVER_REED    = '#47582F'; // vassbrynet ut mot dalen
const RIVER_MUD     = '#3E4E38'; // våt dy vid själva vattenlinjen
const RIVER_CURRENT = '#4E8590'; // ljuset som fångas av strömmen i fåran
// Fårans halvbredd. Hexens inradie är 19, så 12,5 låter vattnet fylla hela
// kanten där kedjan går vidare och lämnar bara två vasslober vinkelrätt mot
// flödet. Det är avsiktligt FETT: Timothy 2026-07-29 beskrev floden som
// *"grunt vatten fast aldrig mer än en hex breda"*, alltså ska hexen läsa som
// en vattenhex — inte som en tunn linje ritad genom mark, för då hade spelaren
// inte kunnat se VILKEN hex som är ogenomtränglig. Midjan finns för att döda
// hexagonen, inte för att göra floden smal.
const RIVER_CHANNEL_HALF = 12.5;
const RIVER_CELL    = 17;

// Flödesaxeln ur kedjan. Två vattengrannar (rakt genomlopp eller krök) ger
// kordan mellan dem; en enda (källa eller mynning) ger riktningen ut genom den
// kanten. Ingen vattengranne alls ska inte kunna hända — en flod är per
// definition en kedja — men en isolerad flodhex i en riggfixtur får inte
// krascha renderaren, så fallet har ett svar.
function riverAxis(q, r) {
  const wd = neighborDirs(q, r, isWaterTerrain);
  let ax, ay;
  if (wd.length >= 2) { ax = DIR_NX[wd[0]] - DIR_NX[wd[1]]; ay = DIR_NY[wd[0]] - DIR_NY[wd[1]]; }
  else if (wd.length === 1) { ax = DIR_NX[wd[0]]; ay = DIR_NY[wd[0]]; }
  else { ax = 1; ay = 0; }
  const m = Math.hypot(ax, ay) || 1;
  return [ax / m, ay / m];
}

function drawRiver(ctx, cx, cy, q, r) {
  ctx.save();
  hexPath(ctx, hexPts(cx, cy));
  ctx.clip();

  // Fåran genereras ur KEDJEGRAFEN, inte ur hexkanterna. Varje granne som är
  // vatten ger ett segment från hexens mitt ut till mitten av den delade
  // kanten, och fåran är allt som ligger närmare än halvbredden till något av
  // segmenten. Det är den konstruktionen som gör kedjan sammanhängande: två
  // flodhexar lägger sin fåra mot SAMMA kantmittpunkt med samma halvbredd, så
  // vattnet möts exakt över gränsen.
  //
  // Första ansatsen räknade i stället in från landkanterna, och den bröt
  // kedjan: vid en delad kant ligger punkten 9,5 från VARDERA angränsande
  // landkant, alltså innanför en bård på 11,5 — vattnet ströps just där det
  // aldrig får strypas. Symptomet var separata blå romber med mörka broar
  // emellan, och det syntes bara på bild.
  //
  // FOW-säkert av samma skäl som skogsbrynet (princip 40): `terrainAt` svarar
  // `'fog'` för en osedd ruta och `undefined` utanför kartan, och ingetdera är
  // vatten — en osedd granne ger alltså aldrig något segment, och fårans form
  // kan inte avslöja terräng spelaren inte sett.
  const segs = [];
  for (let i = 0; i < 6; i++) {
    if (isWaterTerrain(terrainAt(q + HEX_DIRS[i][0], r + HEX_DIRS[i][1]))) {
      segs.push([DIR_NX[i] * R_IN, DIR_NY[i] * R_IN]);
    }
  }
  // En flodhex utan vattengrannar kan inte finnas — en flod ÄR en kedja — men
  // en ensam flodhex i en riggfixtur får inte bli en solid vasslapp.
  if (!segs.length) segs.push([R_IN, 0], [-R_IN, 0]);
  const [axx, axy] = riverAxis(q, r);

  const STEP = 2;
  const x0 = Math.floor((cx - S) / STEP) * STEP, x1 = cx + S;
  const y0 = Math.floor((cy - S) / STEP) * STEP, y1 = cy + S;
  for (let wy = y0; wy <= y1; wy += STEP) {
    for (let wx = x0; wx <= x1; wx += STEP) {
      const dx = wx + STEP / 2 - cx, dy = wy + STEP / 2 - cy;

      // Avstånd till närmaste kedjesegment. Halvbredden moduleras av bruset på
      // det globala rastret, så strandlinjen fransar i stället för att bli en
      // jämn korridor — mark och bryn ska vara textur, aldrig form (princip 3).
      let dist = Infinity;
      for (const s of segs) {
        const L2 = s[0] * s[0] + s[1] * s[1];
        let t = (dx * s[0] + dy * s[1]) / L2;
        t = t < 0 ? 0 : t > 1 ? 1 : t;
        const ex = dx - t * s[0], ey = dy - t * s[1];
        const d = Math.hypot(ex, ey);
        if (d < dist) dist = d;
      }
      const half = RIVER_CHANNEL_HALF + (noiseAt(wx, wy, RIVER_CELL, 2411) - 0.5) * 5.0;
      if (dist > half) {
        // Vasslober vinkelrätt mot flödet. Två steg: våt dy vid vattenlinjen,
        // vass ut mot dalen. Utan det inre steget möter vassen vattnet i ett
        // enda hopp och stranden läser som en ritad kontur.
        ctx.globalAlpha = 0.92;
        ctx.fillStyle = dist < half + 3.5 ? RIVER_MUD : RIVER_REED;
        ctx.fillRect(wx, wy, STEP, STEP);
        continue;
      }

      // Strömmen i fåran. Bruset samplas i en bas som är KOMPRIMERAD längs
      // flödesaxeln, så fläckarna sträcks ut till strömdrag i stället för att
      // bli runda plumpar. Samma teknik som slättens "svaga riktning" — som
      // förkastades där, av precis det skäl som gör den rätt här: slättens
      // riktning svarade mot ingenting i världen, flodens svarar mot kedjan
      // (princip 1). Basen är linjär i världskoordinater, alltså löper
      // strömdragen obrutet mellan två hexar som delar axel.
      const u = wx * axx + wy * axy;
      const v = -wx * axy + wy * axx;
      const n = noiseAt(u * 0.32, v, RIVER_CELL, 5310);
      if (n > 0.56) {
        ctx.globalAlpha = n > 0.78 ? 0.52 : 0.28;
        ctx.fillStyle = RIVER_CURRENT;
        ctx.fillRect(wx, wy, STEP, STEP);
      }
    }
  }
  ctx.globalAlpha = 1;
  ctx.restore();
}

// ── Flodslätten ──────────────────────────────────────────────────────────
// Dalen ligger på var sida om floden (Timothy 2026-07-29) och är därmed inte
// en yta man ser lite av — den är ett band längs varje vattendrag på kartan.
// Den ärvde slättens gamla veteax: fyra `ctx.strokeStyle`-strån på platt färg,
// alltså exakt det princip 2 förbjuder, och de revs ur slätten av det skälet.
// De är borta här också.
//
// Vad som ersätter dem kommer ur strukturen: en flodslätt odlas i TEGAR SOM
// LÖPER LÄNGS VATTNET. Riktningen är alltså inte påhittad — den läses ur
// vilken granne som är flod, precis som flodens egen axel. Princip 15 gäller
// inte emot: den handlar om kartans STÖRSTA yta, och dalen är ett smalt band,
// samma undantag som halvöknen fick.
const VALLEY_CELL  = 46;
const VALLEY_DARK  = '#51692F'; // fuktig svacka, tegen närmast vattnet
const VALLEY_LIGHT = '#728E47'; // gröda som fångar ljuset
const VALLEY_SILT  = '#858C54'; // slamavlagring, ljusare och gråare

// Deltat är samma maskineri med annan palett och UTAN riktning. Ett delta
// solfjädrar — det har ingen enda axel — och det är dessutom blek silt snarare
// än bördig grön, för att aldrig kunna förväxlas med vattnet det mynnar i.
const DELTA_DARK  = '#7E8A4A';
const DELTA_LIGHT = '#A3AE68';
const DELTA_SILT  = '#B5B884';

function drawValleyField(ctx, cx, cy, q, r, isDelta) {
  ctx.save();
  hexPath(ctx, hexPts(cx, cy));
  ctx.clip();

  // Tegarnas riktning: LÄNGS vattnet, alltså vinkelrätt mot riktningen till
  // närmaste flodhex. En dalhex som inte rör vatten (andra ledet, eller en
  // fixtur som inte speglar mapgen) får ett isotropt fält i stället — hellre
  // ingen riktning än en påhittad.
  const wd = neighborDirs(q, r, isWaterTerrain);
  let ax = 1, ay = 0, anis = 1;
  if (!isDelta && wd.length) {
    let nx = 0, ny = 0;
    for (const i of wd) { nx += DIR_NX[i]; ny += DIR_NY[i]; }
    const m = Math.hypot(nx, ny);
    if (m > 0.05) { ax = -ny / m; ay = nx / m; anis = 0.38; }
  }

  const dark  = isDelta ? DELTA_DARK  : VALLEY_DARK;
  const light = isDelta ? DELTA_LIGHT : VALLEY_LIGHT;
  const silt  = isDelta ? DELTA_SILT  : VALLEY_SILT;

  const STEP = lodStep(3);
  const x0 = Math.floor((cx - S) / STEP) * STEP, x1 = cx + S;
  const y0 = Math.floor((cy - S) / STEP) * STEP, y1 = cy + S;
  for (let wy = y0; wy <= y1; wy += STEP) {
    for (let wx = x0; wx <= x1; wx += STEP) {
      const u = wx * ax + wy * ay;
      const v = -wx * ay + wy * ax;
      const jitter = hash32(Math.floor(wx / STEP), Math.floor(wy / STEP), 6262)
                     / 4294967296 - 0.5;
      const n = noiseAt(u * anis, v, VALLEY_CELL, 4242) + jitter * 0.17;
      if (n < 0.34)      { ctx.globalAlpha = 0.42; ctx.fillStyle = dark; }
      else if (n < 0.60) continue;   // bastonen är det dominerande bandet
      else if (n < 0.85) { ctx.globalAlpha = 0.36; ctx.fillStyle = light; }
      else               { ctx.globalAlpha = 0.30; ctx.fillStyle = silt; }
      ctx.fillRect(wx, wy, STEP, STEP);
    }
  }
  ctx.globalAlpha = 1;
  ctx.restore();
}

// ── Markövergången ───────────────────────────────────────────────────────
// Slätten mötte allt utom havet på en rak hexkant. Uppmätt tvärs en och samma
// bild vid 1:1: mot halvöknen ΔL 48 över TRE pixlar, mot skruben ΔL 15 över
// EN — medan kusten bär ΔL 28 över TOLV pixlar och fyra mellansteg. Det är
// därför kartan läser som brickor överallt utom vid vattnet, och varför den
// enda gräns som läser som landskap är den enda som fick en övergångszon.
//
// Att svaret ligger i KANTEN och inte i ytan är mätt, inte antaget: djuphavet
// mäter sd 7,3 och slätten 4,0 — nästan lika platta, och den ena läser som
// levande hav. Havet fick aldrig textur i mitten; det fick kanter och rörelse.
// Därför rör det här passet inte slättens fält, och princip 15 (den största
// ytan ska vara den tystaste) står orörd: den handlar om ytan, inte om mötet.
//
// Greppet är kustens, med en skillnad. Strandbandet har en EGEN ton — sanden
// är mätt ur referensen och är kartans ljusaste yta. Två marktyper som möts
// har ingen tredje ton mellan sig; där blöder de in i varandra. Passet ritar
// alltså GRANNENS baston in i vår kantzon med en täthet som avtar inåt,
// dithrad per block på det globala rastret (princip 16: ordnad dither ger
// schackväv, per-block-brus ger mark).
//
// Varje hex ritar bara IN I SIG SJÄLV. Grannen gör sin egen sida, så zonen
// blir tvåsidig utan att en enda pixel målas två gånger — och ingen pixel
// hamnar utanför den hex passet känner. Det är också vad som gör FOW-regeln
// gratis: `terrainAt` svarar `'fog'` för en osedd ruta och `undefined` utanför
// kartan, och ingetdera står i GROUND_BLEND, alltså kan zonen aldrig avslöja
// en granne spelaren inte sett. Samma regel som skogsbrynet och strandbandet.
//
// Bergen står UTANFÖR med flit. Deras massiv är siluetter som redan svämmar
// över hexkanten och spiller rasbrant nedför — de HAR sin övergång, och en
// dithrad markzon under en rasbrant vore två bilder staplade i samma hex.
// `forest_cedar` står MED, till skillnad från bergen: cederskogens golv är
// kartans mörkaste mark (L ~85 mot lundens 154), så utan zon ritar mötet
// mellan de två skogarna en hexagon i valör. Floden står UTANFÖR — den har
// sin egen strand i vassloberna, och en dithrad markzon under en vassbård
// vore två bilder staplade i samma hex (samma skäl som bergen).
const GROUND_BLEND = new Set([
  'plains', 'semi_desert', 'scrub_maquis', 'hills',
  'forest_olive_grove', 'forest_cedar', 'river_valley', 'river_delta',
]);

// Zonens djupaste räckvidd in i hexen. Bredare än strandbandets 6,8: en
// strandlinje ÄR en linje, en marktypsgräns är diffus. Men "lite" är
// instruktionen (Timothy 2026-07-28) — zonen ska läsa som att marken byter
// karaktär, inte som en gradient mellan två fält. Det är exponenten nedan som
// bär den återhållsamheten: kvadraten håller grannens ton till kantens
// närmaste tredjedel och lämnar resten av zonen åt vår egen mark.
// Amplituden är ett ÖGONBESLUT, inte ett mätbeslut, och det är värt att veta
// varför: övergångsbredden mättar. 7/0,60, 9/0,85 och 12/0,95 gav alla 31 px
// på samma snitt tvärs slätt→halvöken (mot masters 10). Måttet svarar på
// "finns det en zon?", inte på "hur stark är den" — så det kan grinda att
// slicen gjorde sitt jobb, men aldrig välja styrkan. Den valdes på bild vid
// 1:1 (Timothy 2026-07-28): 12/0,95 blandade grönt och gult så brett att
// gränsen slutade gå att identifiera på ett ögonkast, vilket är en
// spelbarhetskostnad och inte en smakfråga.
const BLEND_MAX  = 7.0;
const BLEND_STEP = 3;
const BLEND_PEAK = 0.60;   // tätheten VID kanten; aldrig 1, eller blir zonen en rand

function drawGroundBlend(ctx, cx, cy, q, r, terrain) {
  const pts = hexPts(cx, cy);
  for (let i = 0; i < HEX_DIRS.length; i++) {
    const nt = terrainAt(q + HEX_DIRS[i][0], r + HEX_DIRS[i][1]);
    if (nt === terrain || !GROUND_BLEND.has(nt)) continue;
    const nx = DIR_NX[i], ny = DIR_NY[i];
    ctx.fillStyle = TERRAIN_BASE[nt].c0;
    const [bx0, by0, bx1, by1] = edgeBox(pts, EDGE_OF_DIR[i], BLEND_MAX, 0);
    for (let wy = Math.floor(by0 / BLEND_STEP) * BLEND_STEP; wy <= by1; wy += BLEND_STEP) {
      for (let wx = Math.floor(bx0 / BLEND_STEP) * BLEND_STEP; wx <= bx1; wx += BLEND_STEP) {
        const dx = wx + BLEND_STEP / 2 - cx, dy = wy + BLEND_STEP / 2 - cy;
        const reach = dx * nx + dy * ny;
        if (reach < R_IN - BLEND_MAX || reach > R_IN) continue;
        // Sidledsvillkoret. En skalärprodukt mot en kantnormal beskriver en
        // OÄNDLIG remsa, inte en kant — utan det här rann zonen vidare längs
        // remsan och la grannens ton långt inne på fel del av hexen.
        if (Math.abs(-dx * ny + dy * nx) > S / 2) continue;
        // Bredden ur världsrymdsbruset i punkten själv, så zonen vandrar
        // obrutet vidare in i nästa hex längs samma gräns. Cellen är MINDRE
        // än en hexkant (11 mot 22 px) med flit: samplad grövre blev bredden
        // nästan konstant längs varje kant och zonen ritade hexagonen.
        const w = 3.5 + 5.5 * noiseAt(wx, wy, 11, 4242);
        if (reach < R_IN - w) continue;
        const t = (reach - (R_IN - w)) / w;
        const j = hash32(Math.floor(wx / BLEND_STEP), Math.floor(wy / BLEND_STEP), 8383)
                  / 4294967296;
        if (j > BLEND_PEAK * t * t) continue;
        ctx.fillRect(wx, wy, BLEND_STEP, BLEND_STEP);
      }
    }
  }
}

// ── Havet och kusten ─────────────────────────────────────────────────────
// Tre pass med samma mekanism (skalärprodukt mot DIR_N) och ett gemensamt
// ärende: säga vad som är vatten, vad som är land och var de möts.
//
// Tonerna är MÄTTA ur referensbilden, inte valda. Snittet tvärs dess kust ger
// gräs L140 → sand L180 → skum L202 → grunt L143 → mellandjupt L121 → djup L96.
// Två saker följer av den mätningen. Först: referensen skiljer inte land från
// hav med VALÖR alls (140 mot 143) — hela separationen bärs av det ljusa
// sand- och skumbandet, vilket är precis vad kontrastbudgeten (princip 6)
// reserverar sina ytterligheter åt. Sedan: sanden är ~40 L ljusare än marken
// den ligger på, alltså inte en nyans utan kartans ljusaste yta.
// Vilket hörnpar (pts[e] → pts[e+1]) som är kanten mot grannen i riktning i.
// Uträknad, inte skriven som tabell: en handskriven sexradig avbildning mellan
// två index är precis den sorts fel som ger ett band längs FEL kant och tar en
// halvtimme att hitta.
const EDGE_OF_DIR = HEX_DIRS.map((_, i) => {
  const e = Math.round(Math.atan2(DIR_NY[i], DIR_NX[i]) / (Math.PI / 3) - 0.5);
  return ((e % 6) + 6) % 6;
});

// Blockrutan runt EN kants band, i stället för runt hela hexen. Banden är
// smala remsor längs högst sex kanter; att skanna hexens hela ruta en gång per
// kantuppsättning betydde att fem sjättedelar av varje varv förkastades i
// villkoret. Uppmätt var det slicens dyraste rad.
function edgeBox(pts, e, inward, outward) {
  const a = pts[e], b = pts[(e + 1) % 6];
  const m = Math.max(inward, outward) + 2;
  return [Math.min(a[0], b[0]) - m, Math.min(a[1], b[1]) - m,
          Math.max(a[0], b[0]) + m, Math.max(a[1], b[1]) + m];
}

const SHORE_SAND  = '#D6B264';  // referensens strandband, L 180
const SHORE_WET   = '#B9924E';  // våt sand närmast vattnet — sandens skugga
const SURF_FOAM   = '#A8C8D2';  // brytningen, referensens L 202 dämpad mot vårt mörkare hav
const SWELL_LIGHT = '#5C93B4';  // dyningens rygg
const SWELL_DARK  = '#164A6B';  // dyningens dal

// Strandbandet. Ritas KLIPPT till landhexen: sanden hör till landet, och en
// sandpixel på öppet vatten är exakt det fel hela slicen finns för att stänga.
// Bredden varierar ur världsrymdsbruset, inte ur hexen, så bandet vandrar
// obrutet vidare in i nästa kusthex i stället för att byta karaktär vid varje
// hexkant (princip 10/16: blockigt, aldrig en jämn kurva).
const SHORE_MAX = 6.8;
function drawShore(ctx, cx, cy, seaDirs) {
  const STEP = 2;
  const pts = hexPts(cx, cy);
  for (const i of seaDirs) {
    const e = EDGE_OF_DIR[i], nx = DIR_NX[i], ny = DIR_NY[i];
    const [bx0, by0, bx1, by1] = edgeBox(pts, e, SHORE_MAX, 0);
    for (let wy = Math.floor(by0 / STEP) * STEP; wy <= by1; wy += STEP) {
      for (let wx = Math.floor(bx0 / STEP) * STEP; wx <= bx1; wx += STEP) {
        const dx = wx + STEP / 2 - cx, dy = wy + STEP / 2 - cy;
        const reach = dx * nx + dy * ny;
        // Innanför bandets maxbredd, och aldrig utanför hexen: sanden hör till
        // landet, och en sandpixel på öppet vatten är exakt det fel slicen
        // finns för att stänga. Sidledsvillkoret håller bandet på SIN kant —
        // en skalärprodukt mot en normal beskriver en oändlig remsa.
        if (reach < R_IN - SHORE_MAX || reach > R_IN) continue;
        if (Math.abs(-dx * ny + dy * nx) > S / 2) continue;
        // Bredden samplas på det globala rastret i punkten själv — därför är
        // den kontinuerlig över hexgränsen, och två grannkusthexar möts utan
        // söm. Cellen är MINDRE än en hexkant (9 mot 22 px) med flit: samplad
        // grövre blev bredden nästan konstant längs varje kant och bandet
        // ritade hexagonen med överstrykningspenna.
        const w = 1.2 + 5.6 * noiseAt(wx, wy, 9, 6161);
        if (reach < R_IN - w) continue;
        // Yttersta tredjedelen är våt sand: stranden får en egen inre kant mot
        // vattnet i stället för att sluta i ett steg.
        ctx.fillStyle = reach > R_IN - w * 0.35 ? SHORE_WET : SHORE_SAND;
        ctx.fillRect(wx, wy, STEP, STEP);
      }
    }
  }
}

// Bränningen. Ritas OKLIPPT UTÅT från LANDhexen, inte inåt från havshexen.
// Skälet är mätt, inte principiellt: ritad från havssidan lägger två grannhavs-
// hexar var sitt skum längs samma landkant, och där banden korsas dubblas
// alfan — resultatet blev vita stjärnkluster i varje kustvik som läste som
// snö. Från landsidan ritar varje kust sitt skum en gång.
//
// Bandet börjar strax innanför kanten och sträcker sig ut i vattnet, alltså
// STRADDLAR det hexkanten. Tätheten faller utåt, så vågen har en tydlig lipp
// mot stranden och löser upp sig i vattnet i stället för att sluta i ett steg.
// Fasen kommer ur seaTick: samma kust bryter på olika ställen över tid, men
// samma frame ger alltid samma bild.
function drawSurf(ctx, cx, cy, seaDirs, seaTick) {
  const STEP = 2;
  const IN = 2.0, OUT = 6.0;
  const pts = hexPts(cx, cy);
  ctx.fillStyle = SURF_FOAM;
  for (const i of seaDirs) {
    const e = EDGE_OF_DIR[i], nx = DIR_NX[i], ny = DIR_NY[i];
    const [bx0, by0, bx1, by1] = edgeBox(pts, e, IN, OUT);
    for (let wy = Math.floor(by0 / STEP) * STEP; wy <= by1; wy += STEP) {
      for (let wx = Math.floor(bx0 / STEP) * STEP; wx <= bx1; wx += STEP) {
        const dx = wx + STEP / 2 - cx, dy = wy + STEP / 2 - cy;
        const reach = dx * nx + dy * ny;
        if (reach < R_IN - IN || reach > R_IN + OUT) continue;
        // Skalärprodukten mot en kantnormal beskriver en OÄNDLIG remsa, inte en
        // kant. Utan sidledsvillkoret rann skummet vidare längs remsan och
        // hamnade som blek dis långt inne på grannlandet — den tydligaste
        // påminnelsen om att "nära kanten" och "nära kantens normal" inte är
        // samma sak. Marginalen släpper ut vågen runt hörnet där två havskanter
        // möts.
        if (Math.abs(-dx * ny + dy * nx) > S / 2 + 3) continue;
        // Utåt-andel: 0 vid strandkanten, 1 längst ut i vattnet.
        const out = (reach - (R_IN - IN)) / (IN + OUT);
        // Vågen är en funktion av AVSTÅNDET till stranden, inte av brus: då blir
        // skummet linjer parallella med kusten i stället för en jämn dis, och
        // eftersom fasen dras av seaTick rullar de INÅT mot land. Brustermen
        // förskjuter krönet olika längs kusten — utan den ligger vågorna som
        // perfekta hexagonoffset, alltså ritar de om hexen de skulle dölja.
        const phase = reach * 0.9 - seaTick * 0.55 + noiseAt(wx, wy, 23, 7171) * 5;
        if (Math.sin(phase) < 0.35 + 0.45 * out) continue;
        ctx.globalAlpha = 0.62 - 0.42 * out;
        ctx.fillRect(wx, wy, STEP, STEP);
      }
    }
  }
  ctx.globalAlpha = 1;
}

// Dyningen på öppet hav. Ersätter den animerade AA-ellipsen, som bröt princip
// 10 (ingen kantutjämning) och läste som utspridd lo i helvyn. Märkena ligger
// på hexen men får svämma över kanten, som lövverkets kronor — det är samma
// grepp och samma skäl: en yta som bara innehåller marker som slutar vid
// gränsen ÄR gränsen. Ett märke som skulle hamna på land klams in.
// Djupbrytet. `coastal_sea` möter `deep_sea` på en hexkant, och när dyningen
// väl gjort havsytan levande blev DEN kanten kartans tydligaste hexagon —
// mätningen (en tonskillnad på ~40 L) gör att inget svall kan dölja den. Den
// löses därför på samma sätt som stranden: grundvattnet blöder ut i djupet med
// en brusstyrd, blockig gräns, så hyllan får en form i stället för sex raka
// kanter. Ritas KLIPPT i djuphexen — grundvattnet ska växa utåt, aldrig
// tvärtom.
function drawDepthBand(ctx, cx, cy, shelfDirs) {
  const STEP = 2;
  const MAX = 10;
  const pts = hexPts(cx, cy);
  ctx.fillStyle = TERRAIN_BASE.coastal_sea.c0;
  for (const i of shelfDirs) {
    const e = EDGE_OF_DIR[i], nx = DIR_NX[i], ny = DIR_NY[i];
    const [bx0, by0, bx1, by1] = edgeBox(pts, e, MAX, 0);
    for (let wy = Math.floor(by0 / STEP) * STEP; wy <= by1; wy += STEP) {
      for (let wx = Math.floor(bx0 / STEP) * STEP; wx <= bx1; wx += STEP) {
        const dx = wx + STEP / 2 - cx, dy = wy + STEP / 2 - cy;
        const reach = dx * nx + dy * ny;
        if (reach < R_IN - MAX || reach > R_IN) continue;
        if (Math.abs(-dx * ny + dy * nx) > S / 2) continue;
        const w = 1 + (MAX - 1) * noiseAt(wx, wy, 15, 6262);
        if (reach < R_IN - w) continue;
        // Ytterkanten ditheras block för block — annars byter hyllan bara form
        // på en hexagon i stället för att sluta vara en. Tröskel och inte alfa:
        // två grannkanters band överlappar i hörnet, och genomskinliga block
        // som ritas två gånger blir ljusa fläckar just där.
        if (reach < R_IN - w * 0.4 && hash32(wx, wy, 6263) > 0x8CCCCCCC) continue;
        ctx.fillRect(wx, wy, STEP, STEP);
      }
    }
  }
}

// Världsrymd → axial hex. Samma avrundning som `hexAtScreen`, men utan
// kamerasteget: dyningens pass går över det globala rastret och behöver fråga
// "vilken hex ligger den här punkten i?", inte "var klickade någon?".
function hexAtWorld(wx, wy) {
  const q = (2 / 3 * wx) / S;
  const r = (-1 / 3 * wx + SQRT3 / 3 * wy) / S;
  const s = -q - r;
  let rq = Math.round(q), rr = Math.round(r);
  const rs = Math.round(s);
  const dq = Math.abs(rq - q), dr = Math.abs(rr - r), ds = Math.abs(rs - s);
  if (dq > dr && dq > ds) rq = -rr - rs;
  else if (dr > ds) rr = -rq - rs;
  return [rq, rr];
}

// Dyningen. ETT pass över hela det globala rastret — inte ett pass per hex.
// Skillnaden är inte kosmetisk: per hex ritas varje rastercell om av varje
// granne vars bbox den råkar ligga i, och ägarskapstestet som skulle hindra det
// kostade sex skalärprodukter per cell och hex. Uppmätt gick havspasset från
// 44 ms till en bråkdel när loopen vändes ut och in. Rastret är dessutom exakt
// vad ett kommande viewport-culling behöver: iterera bara de synliga cellerna.
// Vilka tiles som kan påverka bilden: de vars mitt ligger i duken plus en
// marginal. Se kommentaren vid anropet i render() för varför marginalen finns
// och varför den är så bred.
const CULL_MARGIN = 96;
function visibleTiles() {
  const z = State.camera.zoom * SCALE;
  const x0 = (-State.camera.x) / z - CULL_MARGIN;
  const y0 = (-State.camera.y) / z - CULL_MARGIN;
  const x1 = (canvas.width - State.camera.x) / z + CULL_MARGIN;
  const y1 = (canvas.height - State.camera.y) / z + CULL_MARGIN;
  const out = [];
  for (const t of State.tileData) {
    const { x, y } = hexPx(t.q, t.r);
    if (x >= x0 && x <= x1 && y >= y0 && y <= y1) out.push(t);
  }
  return out;
}

const SWELL_CELL = 13;
function drawSwellField(ctx, seaTick) {
  let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
  for (const t of State.tileData) {
    if (!isSeaTerrain(t.terrain)) continue;
    const { x, y } = hexPx(t.q, t.r);
    if (x < minX) minX = x; if (x > maxX) maxX = x;
    if (y < minY) minY = y; if (y > maxY) maxY = y;
  }
  if (minX === Infinity) return;
  // Rastret klipps mot det som syns. Det är inte viewport-culling som slice
  // (den gäller alla pass och mäts för sig) — det är att ett pass som itererar
  // ett VÄRLDSRASTER inte har någon anledning att besöka celler utanför duken.
  // Marginalen S släpper in märken vars vänsterkant ligger strax utanför.
  const k = State.camera.zoom * SCALE;
  minX = Math.max(minX - S, (-State.camera.x) / k - S);
  maxX = Math.min(maxX + S, (canvas.width - State.camera.x) / k + S);
  minY = Math.max(minY - S, (-State.camera.y) / k - S);
  maxY = Math.min(maxY + S, (canvas.height - State.camera.y) / k + S);
  const g0x = Math.floor(minX / SWELL_CELL), g1x = Math.ceil(maxX / SWELL_CELL);
  const g0y = Math.floor(minY / SWELL_CELL), g1y = Math.ceil(maxY / SWELL_CELL);
  for (let gy = g0y; gy <= g1y; gy++) {
    for (let gx = g0x; gx <= g1x; gx++) {
      const h = hash32(gx, gy, 9101);
      const px = gx * SWELL_CELL + (h & 0x7);
      const py = gy * SWELL_CELL + ((h >>> 3) & 0x7);
      const [q, r] = hexAtWorld(px, py);
      if (!isSeaTerrain(terrainAt(q, r))) continue;
      // Nära en kant mot land får dyningen inte nå fram — den skulle skölja upp
      // på stranden, och strandens vatten är bränningens ärende. Punkter långt
      // in i hexen kan aldrig vara nära en kant: radien avgör det med en
      // multiplikation i stället för sex skalärprodukter, och de flesta celler
      // ligger långt in.
      const c = hexPx(q, r);
      const dx = px - c.x, dy = py - c.y;
      if (dx * dx + dy * dy > (R_IN - 5) * (R_IN - 5)) {
        let toward = 0, best = -1e9;
        for (let i = 0; i < 6; i++) {
          const p = dx * DIR_NX[i] + dy * DIR_NY[i];
          if (p > best) { best = p; toward = i; }
        }
        if (best > R_IN - 5
            && !isSeaTerrain(terrainAt(q + HEX_DIRS[toward][0], r + HEX_DIRS[toward][1]))) continue;
      }
      // Var fjärde rad bär en längre kam: en dyning är rader, inte prickar.
      const crest = ((gy + (h >>> 6)) & 0x3) === 0;
      const len = crest ? 7 + (h >>> 8 & 0x5) : 3 + (h >>> 8 & 0x3);
      // Driften följer rastret, inte hexen, så hela fältet vandrar som ett.
      const x = Math.round(px + (((seaTick + gy) & 0x7) - 4)), y = Math.round(py);
      if (crest) {
        ctx.globalAlpha = 0.22;
        ctx.fillStyle = SWELL_DARK;
        ctx.fillRect(x, y + 1, len, 1);
      }
      ctx.globalAlpha = crest ? 0.30 : 0.18;
      ctx.fillStyle = SWELL_LIGHT;
      ctx.fillRect(x, y, len, 1);
    }
  }
  ctx.globalAlpha = 1;
}

// ── Relief — hills and mountains ─────────────────────────────────────────
// The old high ground was three brown ellipses on flat tan (hills) and two
// pale triangles on the map's brightest fill (mountains). Both broke the same
// two rules at once: isolated marks on a flat colour read as decoration at any
// scale (princip 2), and neither said anything about the one thing high ground
// IS — height. A mountain that is a flat hex with an icon on it is a legend
// entry, not terrain.
//
// The measured state before this slice, which is what made it a priority
// rather than a polish job: in luminance, `hills` (167.5) and `scrub_maquis`
// (169.2) sat 1.7 apart, `mountain_red` (156.4) sat 2.4 from the olive grove,
// and `mountain_limestone` (212.4) was the brightest surface on the whole map
// by 27 — spending the contrast budget (princip 6) on a field rather than on
// an edge. Three terrains that are all "high rocky ground" occupied three
// unrelated slots on the value ladder and shared no identity at all.
//
// What replaces them is a HEIGHT FIELD, not a set of sprites. The field is
// sampled in world pixel coordinates on the global lattice (same machinery as
// the plains parcel and the forest floor), so a range of hills runs unbroken
// across hex borders: the clip still keeps rock off the neighbouring plain,
// but between two hill hexes the seam has nothing to reveal. Relief is the
// one terrain where this matters most — a ridge that stops at a hex border is
// not a ridge, it is a tile.
//
// The shading is real, not decorative: two samples of the same field taken
// along the light axis give the slope, and the slope decides the tone. Light
// falls from the upper left, as it does on every sprite in this renderer
// (princip 11), so a flank facing upper-left is lit and the flank behind the
// crest is in shadow. That is why the result reads as ground that rises
// instead of as mottling — the light is consistent with the trees, the units
// and the cities standing on it.
const RELIEF_D = 2; // sample distance along the light axis, in logical px

// REJECTED on the way here: shading straight from the slope, i.e. the field
// sampled twice along the light axis and banded on the difference. It is the
// textbook hillshade and it fails here for a reason the plains slice already
// paid for once — a single directional derivative produces DIAGONAL STREAKS.
// The map filled with NW–SE smears that read as dunes or as a smudged
// photograph, never as ground that rises, because a streak has no top and no
// bottom. Anisotropy that answers to nothing in the world was rejected on the
// plains for the same reason (princip 1).
//
// What replaces it is TERRACING. The height field is quantised into a handful
// of levels, and the tone is decided by comparing this block's level with its
// neighbours' along the light axis:
//
//   this level > the level below-right  → a crest with ground falling away
//                                         behind it: rim catches the light
//   this level < the level above-left   → higher ground stands between this
//                                         block and the sun: cast shadow
//
// Those two tests produce CLOSED contours — a lit rim curving over the top of
// each swell and a shadow pooling at its foot — instead of stripes, because a
// contour of a closed hummock is itself closed. It also puts the strongest
// tones exactly where a landform is legible (its edge) and leaves the interior
// quiet, which is what princip 15 asks of any large surface.
function reliefLevel(wx, wy, cell, stream, steps) {
  return Math.floor(noiseAt(wx, wy, cell, stream) * steps);
}

// A swell spans about one and a half hexes: hills are ROUNDED and broad, and a
// cell small enough to fit inside one hex would make every hex its own bump —
// which is the per-hex-icon failure again, only in shading.
const HILL_CELL = 20;

// The six tones the hillside is actually painted in, OPAQUE. They started life
// as four colours drawn at six different alphas over the base fill, and that
// cost far more than it looked like it should: profiling the relief terrain
// found the three passes costing 14 ms together while summing to 4.5 ms apart,
// which is the signature of canvas state thrash — setting `globalAlpha` between
// every fillRect defeats the batching, and the hills set it several hundred
// times per hex. Baking the blend into a flat colour is the same move princip
// 13b already prescribes for a translucent layer over a base: compute it once,
// draw it opaque, delete the layer.
//
// Derived as alpha·tone + (1−alpha)·#C8A464, the hills' base fill:
//   crest rim   #E4C283 @0.75   flank body   #E4C283 @0.46
//   cast shadow #A8834A @0.55   lit terrace  #D2AE6E @0.40
//   low terrace #A8834A @0.34   hollow       #8A6A3C @0.42
const HILL_RIM    = '#DDBB7B'; // crest catching the light
const HILL_CAST   = '#B69256'; // the shadow a crest throws down its own foot
const HILL_HIGH   = '#D5B272'; // the highest terrace
const HILL_LIT    = '#CCA868'; // sunlit terrace
const HILL_LOW    = '#BD995B'; // the terrace below the base
const HILL_HOLLOW = '#AE8C53'; // the hollow between two swells

// Five terraces over the field. Fewer and the hex holds one lazy step; many
// more and the rims crowd into hatching, which is texture again rather than
// form.
const HILL_STEPS = 5;
// How far apart the two level samples sit, which is what sets the WIDTH of the
// lit rim and the shadow at the foot. At the mountains' distance of 2 px the
// hills came out as thin dark veins — a rim one or two pixels wide reads as a
// crack in the ground, not as a slope. A slope needs a band.
// Kept as documentation of the shape only — the comparison distance now comes
// from the block grid (two cells at STEP 3 = 6 world px), see drawHills.
const HILL_D = 4;

// One field sample per block, not three.
//
// The first version called the noise three times per block — here, one step up
// the light axis, one step down — and each of those walks four hashes. Measured
// against the pre-slice renderer on the same fixture, the relief terrain took a
// frame from 0.7 ms to 16.5 ms, a 24× regression that shipped unnoticed because
// nothing in the visual gates looks at time. The samples overlap almost
// completely between neighbouring blocks, so the fix is to compute the field
// ONCE over the hex's block lattice and index into it. Choosing the comparison
// distance to BE one block step is what makes that possible: the neighbour up
// the light axis is simply the previous cell on the diagonal.
//
// `pad` gives the grid a one-cell margin on every side so the diagonal
// neighbours exist for blocks on the hex's rim.
// Half-width of the hex at a vertical offset dy from its centre. The hexagon
// has vertices at (±S, 0) and (±S/2, ±S√3/2), so its upper-right edge runs
// x = S − |dy|/√3.
//
// This exists to replace `ctx.clip()`. Clipping to a hex path costs a
// save/clip/restore per tile — sixty-odd a frame in a relief-heavy view — and
// the profile showed the three relief passes costing 14 ms together while
// summing to 4.5 ms apart, which is what a per-tile clip does to batching.
// Computing the row's span is exact and free. It also removes princip 13b's
// residual seam at the source: a clipped layer reaches only ~75 % coverage on
// the boundary pixel and lets the base shine through, whereas a span that stops
// one block short simply leaves the base showing — the same tone by
// construction, since TERRAIN_BASE is kept in sync with the texture.
function hexHalfWidth(dy) {
  return S - Math.abs(dy) / Math.sqrt(3);
}

function reliefGrid(cx, cy, step, cell, stream, steps, ridged) {
  const x0 = Math.floor((cx - S) / step) * step - step;
  const y0 = Math.floor((cy - S) / step) * step - step;
  const nx = Math.ceil((2 * S) / step) + 3, ny = Math.ceil((2 * S) / step) + 3;
  const lv = new Int8Array(nx * ny);
  for (let j = 0; j < ny; j++) {
    for (let i = 0; i < nx; i++) {
      const wx = x0 + i * step, wy = y0 + j * step;
      const n = ridged ? ridgeAt(wx, wy, cell, stream) : noiseAt(wx, wy, cell, stream);
      lv[j * nx + i] = Math.floor(n * steps);
    }
  }
  return { lv, x0, y0, nx, ny, step };
}

function drawHills(ctx, cx, cy) {
  // STEP stays at 2 for the hills, unlike the mountains. It was tried at 3 to
  // save fills and the terracing washed out: a hill terrace is only about four
  // world pixels wide where the slope is steep, so a three-pixel block aliases
  // it away and the swells flatten into a smudge. Widening HILL_CELL to 30 to
  // compensate flattened them further. The saving here comes from the grid —
  // one field sample per block instead of three — not from coarser blocks.
  const STEP = 2;
  const g = reliefGrid(cx, cy, STEP, HILL_CELL, 4242, HILL_STEPS, false);
  for (let j = 1; j < g.ny - 1; j++) {
    // The row's horizontal span inside the hex, measured at whichever of the
    // block's two edges lies further from the centre so a block never pokes out
    // past the boundary. This replaces ctx.clip().
    const wyRow = g.y0 + j * STEP;
    const hw = hexHalfWidth(Math.max(Math.abs(wyRow - cy), Math.abs(wyRow + STEP - cy)));
    if (hw <= 0) continue;
    const xa = cx - hw, xb = cx + hw;
    for (let i = 1; i < g.nx - 1; i++) {
      const wx = g.x0 + i * STEP, wy = g.y0 + j * STEP;
      if (wx < xa || wx + STEP > xb) continue;
      // Two cells along the diagonal = 4 world px at STEP 2, which is exactly
      // the comparison distance the shape was tuned to. It sets the WIDTH of
      // the lit rim and the shadow at the foot; at one block the bands thin
      // back toward the veins-in-the-ground look the first attempt had.
      const i0 = Math.max(0, i - 2), j0 = Math.max(0, j - 2);
      const i1 = Math.min(g.nx - 1, i + 2), j1 = Math.min(g.ny - 1, j + 2);
      const lv = g.lv[j * g.nx + i];
      const up = g.lv[j0 * g.nx + i0];
      const dn = g.lv[j1 * g.nx + i1];
      // Edge first, body second. The rim and the cast shadow are what make the
      // form legible, but a rim with no body behind it is filigree: the first
      // attempt tinted only the highest and lowest terrace, faintly, and the
      // result read as pale veins scratched into flat tan. Each terrace has to
      // carry a tone of its own so the eye sees high ground and hollow as
      // AREAS, with the rim sharpening them rather than standing in for them.
      if (lv > dn)      ctx.fillStyle = HILL_RIM;
      else if (lv < up) ctx.fillStyle = HILL_CAST;
      else if (lv >= 4) ctx.fillStyle = HILL_HIGH;
      else if (lv === 3)ctx.fillStyle = HILL_LIT;
      else if (lv === 1)ctx.fillStyle = HILL_LOW;
      else if (lv === 0)ctx.fillStyle = HILL_HOLLOW;
      else continue;    // terrace 2 is the base tone: the hillside itself
      ctx.fillRect(wx, wy, STEP, STEP);
    }
  }
}

// ── Mountains ────────────────────────────────────────────────────────────
// Same height field, same terracing, same light — a mountain and a hill are
// the same kind of fact about the ground, and drawing them with two unrelated
// techniques is what let the old map put one of them at the top of the value
// ladder and the other in the middle of it. What separates them is the BIG
// form (princip 20), and it separates them three ways:
//
//   RIDGED, not rounded. The field is folded — `1 - |2n-1|` — which turns
//   every level crossing of the underlying noise into a crease. Rounded swells
//   become sharp ridge lines with steep faces, which is the difference between
//   grazing country and rock.
//   MORE TERRACES over the same distance, so the ground climbs faster.
//   A WIDER VALUE RANGE, and this is where the contrast budget (princip 6) is
//   spent on purpose: the deepest shadow and the brightest lit face on the
//   whole map both belong to the mountain, because "the mountain edge" is one
//   of the four things the budget is reserved for. The old limestone spent the
//   same brightness on a FLAT FILL, which bought nothing — brightness on a
//   field is not an edge.
const MTN_CELL = 22;
const MTN_STEPS = 5;

function ridgeAt(wx, wy, cell, stream) {
  const n = noiseAt(wx, wy, cell, stream);
  return 1 - Math.abs(2 * n - 1);
}

// Rock is shaded by FACET, not by contour. Terracing the folded field — the
// obvious reuse of the hills machinery — was tried twice and failed twice, and
// the two failures are worth keeping because they are the same mistake at two
// frequencies: fine, it broke into scattered dark dashes that read as rubble
// or as printed text; coarse, the nested rings of a folded field turned every
// summit into a FINGERPRINT. Contours drawn as lines have no body, and a
// folded field's contours nest so tightly that they are all line and no body.
//
// The slope does the work instead. One difference of the folded field along
// the light axis, banded into four tones with nothing left showing through,
// fills the hex with broad planes of light and shade. The fold is what makes
// them angular: across a crease the slope flips sign in one step, so two
// facets meet at a hard edge — which is the whole visual difference between
// rock and pasture. On the smooth field of the hills the same technique gave
// soft diagonal smears, which is why the hills do NOT use it.
function ridgeSlope(wx, wy, cell, stream) {
  return ridgeAt(wx + RELIEF_D, wy + RELIEF_D, cell, stream)
       - ridgeAt(wx - RELIEF_D, wy - RELIEF_D, cell, stream);
}

// Limestone is the pale rock of the Aegean and red mountain is its iron-stained
// cousin; they are the SAME landform, so they share every shape parameter and
// differ only in stone colour. That is the honest reading of princip 20 in the
// other direction: two things that really are the same kind of object must not
// be pulled apart by form, or the map lies about what they are. The player's
// reason to tell them apart is tin, and tin is carried by the deposit marker.
// Five tones, opaque: the facets cover the hex completely, so the base fill is
// no longer visible inside a mountain and the rock carries its own value range.
//
// The range is deliberately NOT the full one available. A first pass ran the
// limestone from near-black to near-white and the massif turned into marble
// camouflage — it read as an abstract pattern, it pulled the eye off every
// city and unit on the map, and a large dark facet beside a large light one
// reads as a HOLE rather than a peak. Princip 6 reserves the extremes for the
// mountain's EDGE, not for its interior: inside the massif the rock is modelled
// in a middle band, and the hard dark line goes around the outside of the whole
// range (see the massif outline pass).
//
// The scree and the summits carry SEPARATE ramps. Sharing one ramp forced a
// choice between a mountain that reads as rock and a mountain that keeps its
// place on the value ladder, and measurement showed it losing both: with one
// ramp dark enough to let the peaks stand out, limestone's whole hex fell from
// L 203 to L 143 — it stopped being pale stone, which is the one thing
// limestone is — and red mountain fell to L 106, three units from the coastal
// sea it borders, which would have wiped out the shoreline exactly the way the
// plains once wiped it out (princip 17). Two ramps let the ground sit where the
// ladder needs it while the summits keep the range they need.
//
// The summit ramp's dark end was deepened again on 2026-07-27 after Timothy's
// reference image became available and could be MEASURED: its rock runs from
// about L 59 in the shadow to L 164 on the lit flank, while ours bottomed out
// at L 118 and the massif read as pale on pale against its own scree. Depth in
// the shadow is a large part of what the reference calls "fine detail", and it
// is where princip 6 says the extremes belong — the mountain's edge.
const MTN_ROCK = {
  mountain_limestone: {
    screeLo: '#8E8A78', screeMid: '#ABA692', screeHi: '#C6C0A8',
    black: '#4A473C', deep: '#6E6A5A', shade: '#B0A992', lit: '#D8D0B6', sun: '#F0EAD6',
  },
  mountain_red: {
    screeLo: '#82604A', screeMid: '#A07A5E', screeHi: '#BE9070',
    black: '#43281C', deep: '#6E4632', shade: '#AE7C5C', lit: '#CE9C74', sun: '#E8BC96',
  },
};

function drawMountain(ctx, cx, cy, terrain) {
  const rock = MTN_ROCK[terrain];

  // Same one-sample-per-block grid as the hills, and the same reason: the scree
  // called the folded field twice per block through `ridgeSlope`, which is four
  // noise walks, and the two samples of neighbouring blocks are the same points.
  const STEP = 3;
  const g = reliefGrid(cx, cy, STEP, MTN_CELL, 7373, 64, true);
  // Scree, and only scree. Banding the facets across the FULL tonal range was
  // tried and produced marble camouflage: soft organic swirls, because the
  // slope of a smooth field varies smoothly even after folding, so no amount
  // of tuning turns it into a peak. A peak is defined by its silhouette
  // against what lies behind it — by occlusion — and a ground texture cannot
  // occlude anything. The field's job here is therefore the one the forest
  // floor already does: be the dark shared volume that the silhouettes resolve
  // OUT of (princip 4 and princip 7 — wildland separates as light objects
  // against dark mass, the opposite direction from cultivated open country).
  for (let j = 1; j < g.ny - 1; j++) {
    const wy = g.y0 + j * STEP;
    const hw = hexHalfWidth(Math.max(Math.abs(wy - cy), Math.abs(wy + STEP - cy)));
    if (hw <= 0) continue;
    const xa = cx - hw, xb = cx + hw;
    for (let i = 1; i < g.nx - 1; i++) {
      const wx = g.x0 + i * STEP;
      if (wx < xa || wx + STEP > xb) continue;
      // The slope along the light axis, read as the difference between two
      // cells of the precomputed grid instead of two fresh field walks. The
      // grid holds 64 levels, so one level is ~0.016 of the field.
      const d = g.lv[(j + 1) * g.nx + (i + 1)] - g.lv[(j - 1) * g.nx + (i - 1)];
      if (d > 3)       ctx.fillStyle = rock.screeHi;
      else if (d > -2) ctx.fillStyle = rock.screeMid;
      else             ctx.fillStyle = rock.screeLo;
      ctx.fillRect(wx, wy, STEP, STEP);
    }
  }
}

// ── Peaks ────────────────────────────────────────────────────────────────
// NOT clipped to the hex, and drawn in a pass of its own after every tile's
// ground is down — exactly like the canopy, and for exactly the same reason.
// A summit that stops dead at the hex border is a tile; a summit allowed to
// rise a few pixels above it OCCLUDES the hex behind, and occlusion is the
// only cue a top-down map has for "this is tall". It is also princip 12's rule
// applied to rock: the foot stays inside its hex, only the peak hangs over, so
// a plain next to a mountain can never be misread as mountain.
//
// Princip 18 governs the inside of the range: ONE mass, no ink between the
// peaks. The first Spearmen sprite outlined every man separately and read as a
// barcode; a range with a line around every summit reads as a row of tents.
// The peaks separate from each other by TONE alone — lit left face, shadowed
// right face — and from the ground by being the light thing on a dark mass.
//
// A RIDGE, not a cone. Three things carry it, all read off Timothy's reference
// image (2026-07-27) rather than invented:
//
//   SEVERAL SUMMITS WITH SADDLES. The reference has no single tent anywhere —
//   every massif is a main summit with one or two lower spurs beside it and a
//   saddle between. The profile is therefore the MAX of a few straight-sided
//   tents, which also gives the near-straight flanks real rock has (a curved
//   flank reads as a dune).
//   STRIATION. The reference's sunlit flanks are combed with long thin tonal
//   streaks running down the fall line — that is the "very fine detail", and it
//   is the single biggest difference between its rock and a flat plane. Here
//   the tone is picked per one-or-two-column run from a three-tone ramp, so the
//   flank reads as combed stone instead of as a painted triangle.
//   A RAGGED FOOT. The reference's mountains do not end on a line; the base
//   frays into the ground in fingers of talus. Hence a skirt BELOW the base,
//   deepest at the middle of the mass and tapering out — which is also what
//   lets a range spill downhill onto the hex below it.
function peakProfile(q, r, i) {
  // Big. The previous pass made 17–27 px summits, two to four per hex, all
  // comfortably inside their own hex — and a hex full of small summits reads as
  // a texture of mountains rather than as A mountain. The reference puts ONE
  // massif across roughly two hexes. At 26–42 px wide against a 44 px hex, a
  // massif necessarily overruns its borders, which is the intent.
  // BIGGER THAN THE HEX, deliberately. At 30–44 px against a 44 px hex the mass
  // fitted inside its own tile, and a lone mountain hex came out as a brown
  // hexagon with a mountain drawn in it — an icon on a counter, which is the
  // one thing this is meant to stop being. A massif has to be a landform bigger
  // than the grid before a range stops reading as tiles.
  const w = 42 + rndInt(q, r, 2100 + i * 9, 20);   // 42–61 px — the hex is 44
  const h = 15 + rndInt(q, r, 2200 + i * 9, 11);   // 15–25 px tall
  const apex = Math.round(w * (0.28 + rnd(q, r, 2300 + i * 9) * 0.30));

  // Main summit plus two or three spurs. With only one spur, far from the
  // apex, the silhouette came out as a clean tent with a bump beside it —
  // paper cutouts. The reference has no clean tent anywhere: its ridges are
  // broken, the subsidiary summits crowd the main one, and the outline changes
  // pitch several times on the way down.
  const tops = [{ x: apex, h }];
  const spurs = 2 + rndInt(q, r, 2350 + i * 9, 2);
  for (let s = 0; s < spurs; s++) {
    tops.push({
      x: Math.round(w * (0.10 + rnd(q, r, 2400 + i * 9 + s * 3) * 0.84)),
      h: h * (0.42 + rnd(q, r, 2450 + i * 9 + s * 3) * 0.43),
    });
  }

  const prof = new Array(w);
  for (let x = 0; x < w; x++) {
    let y = 0;
    for (const t of tops) {
      // Straight-sided, shallower on the sunward side so the lit flank is the
      // broad one — that is where the striation needs room to read. The reach
      // is a SLOPE, not a width: at 1.9× the height it exceeded the sprite, the
      // tent never came back down inside its own width, and every summit came
      // out as a flat-topped mesa with a lit tabletop along the crest.
      const dx = x - t.x;
      const reach = t.h * (dx < 0 ? 1.15 : 0.75);
      y = Math.max(y, t.h * Math.max(0, 1 - Math.abs(dx) / reach));
    }
    prof[x] = Math.max(0, Math.round(y));
  }
  // Knock the single-column spikes off the crest. Three tents maxed together
  // leave one-pixel needles where two slopes cross, and a needle at map scale
  // reads as a tooth or a splinter, not as a summit.
  for (let x = 1; x < w - 1; x++) {
    const nb = Math.min(prof[x - 1], prof[x + 1]);
    if (prof[x] > nb + 1) prof[x] = nb + 1;
  }

  // The talus skirt: how far the mass reaches BELOW its base line. Deepest at
  // the middle, seeded in two-column runs so the edge frays instead of
  // wobbling per pixel.
  const skirt = new Array(w);
  for (let x = 0; x < w; x++) {
    const mid = 1 - Math.abs(x - (w - 1) / 2) / ((w - 1) / 2);
    skirt[x] = Math.round(mid * (2 + rnd(q, r, 2600 + i * 40 + (x >> 1)) * 4.5));
  }

  // Striation, as a FAN. The first attempt picked one tone per column, which
  // gave dead-straight vertical bars of even width — legible as texture but
  // wrong: the reference's streaks radiate from the summit, so they are narrow
  // at the crest and splay apart toward the foot, and that divergence is most
  // of what makes its rock read as a slope rather than as a painted plane.
  // `ray()` below indexes this table by the ANGLE from the summit instead of by
  // x, so a run drawn low on the flank is automatically wider than the same run
  // near the crest. Runs, never single pixels — an isolated off-tone pixel in a
  // rock face is a speck, the lesson the tree crowns already taught.
  const grain = new Array(48);
  for (let k = 0; k < 48; k++) grain[k] = rnd(q, r, 2800 + i * 60 + k);
  return { w, h, apex, prof, skirt, grain };
}

// Which ray from the summit a point lies on. `depth` is how far below the crest
// the point sits; dividing the horizontal offset by it is what turns parallel
// stripes into a fan.
function peakRay(p, x, depth) {
  // Coarse on purpose. At seven rays per unit the streaks converged into
  // needle-thin white spikes at every summit and the range read as crystals or
  // as a picket fence; the reference's rays are broad enough that three or four
  // of them cover a whole flank.
  const u = (x - p.apex) / (depth + 3.0);
  return p.grain[(Math.round(u * 4.5) + 96) % 48];
}

function drawPeak(ctx, rock, px, py, p) {
  const x0 = Math.round(px - p.w / 2), y0 = Math.round(py);

  // Contact shadow, cast down-right like every other shadow in this renderer,
  // and laid under the talus so the whole mass sits on ground rather than on
  // top of it.
  ctx.globalAlpha = 0.30;
  ctx.fillStyle = '#241F18';
  for (let x = 0; x < p.w; x++) {
    if (p.prof[x] < 2) continue;
    ctx.fillRect(x0 + x + 2, y0 + p.skirt[x], 1, 2);
  }
  ctx.globalAlpha = 1;

  for (let x = 0; x < p.w; x++) {
    const hgt = p.prof[x];
    if (hgt < 1 && p.skirt[x] < 1) continue;
    const top = y0 - hgt;
    const lit = x < p.apex;
    // Light from the upper left: the flank before the apex faces it, the flank
    // beyond it is turned away. Each flank is a THREE-tone ramp — one tone was
    // a solid, two were a pyramid — and the tone is chosen per RAY from the
    // summit rather than per column, so the striation fans out downslope.
    // Three segments down the column is the cheap version of that fan: it costs
    // three fills instead of one, and it is enough for the streaks to visibly
    // widen toward the foot, which is the whole effect.
    const total = hgt + p.skirt[x];
    const segs = total > 9 ? 3 : total > 4 ? 2 : 1;
    for (let s = 0; s < segs; s++) {
      const yA = Math.round(top + (total * s) / segs);
      const yB = Math.round(top + (total * (s + 1)) / segs);
      const g = peakRay(p, x, yA - top);
      // Weighted toward the flank's OWN tone. Splitting a flank evenly across
      // three tones made every ray shout: the streaks carried as much contrast
      // as the light/shadow split itself, and the rock read as fur. In the
      // reference the striation is a modulation of one plane, not a set of
      // competing planes — so most rays keep the flank tone and only a few
      // step lighter or darker.
      ctx.fillStyle = lit ? (g < 0.20 ? rock.shade : g < 0.86 ? rock.lit : rock.sun)
                          : (g < 0.22 ? rock.black : g < 0.88 ? rock.deep : rock.shade);
      ctx.fillRect(x0 + x, yA, 1, Math.max(1, yB - yA));
    }

    // The crest: one bright pixel along the whole sunlit ridge, so every summit
    // and every saddle is drawn by its edge. Reserved and thin — this is the
    // brightest rock on the map, and princip 6 spends brightness on an edge.
    if (hgt > 0 && x <= p.apex) {
      ctx.fillStyle = rock.sun;
      ctx.fillRect(x0 + x, top, 1, 1);
    }
    // The cap on the main summit, two pixels across and two down. The reference
    // gives its tallest summit a small pale crown and nothing else — it is what
    // tells the eye which of the summits is the high one.
    if (Math.abs(x - p.apex) <= 1 && hgt > 6) {
      ctx.fillStyle = rock.sun;
      ctx.fillRect(x0 + x, top, 1, 2);
    }
    // The talus itself is loose broken stone, not cliff: it takes the darker
    // half of the ramp so the mass grounds out instead of ending in a bright
    // line against the neighbouring terrain.
    if (p.skirt[x] > 0) {
      ctx.fillStyle = peakRay(p, x, hgt) < 0.45 ? rock.black : rock.deep;
      ctx.fillRect(x0 + x, y0 + p.skirt[x] - 1, 1, 1);
    }
  }
}

// ── Kullarnas kroppar ────────────────────────────────────────────────────
// Timothy 2026-07-28: *"kullarna ser ut som om att de är ritade ovanifrån med
// en svag penna."* Det är en exakt diagnos av vad koden faktiskt gjorde, och
// den namnger princip 26: höjd läses som OCKLUSION, inte som markskuggning.
// `drawHills` är ett terrasserat höjdfält — en hillshade, ritad i PLAN. Den
// säger var det är högt. Den säger aldrig att något står i vägen för något
// annat, och det är det senare ögat läser som höjd. Bergen ritas i PROFIL, och
// bergen fungerar.
//
// Fältet blir kvar. Det är kullens MARK, precis som skreen är bergets, och det
// är fältet som håller ihop landformen över hexgränserna. Det som saknades var
// kroppen ovanpå den.
//
// Tre saker skiljer kullen från berget, och alla tre är det som gör den till en
// kulle:
//   1. **Profilen är en kupol, inte ett tält.** Bergets flanker är raka linjer
//      mot en spets; en cosinusklocka rundar krönet. Rak flank + låg höjd gav en
//      pyramid som såg ut som ett berg någon satt sig på.
//   2. **Låg och bred.** 8–14 px hög mot bergets 15–25, och 46–70 px bred mot
//      hexens 44. Höjd/bredd-förhållandet ÄR skillnaden — inte storleken.
//   3. **Inget vitt krön.** Kontrastbudgeten (princip 6) har lagt kartans
//      ljusaste pixlar på bergstoppen och strandbandet. En kulle som tar samma
//      valör stjäl bergets läsning; kullens krön får HILL_RIM, som är den ton
//      fältet redan använder för sina krönkanter.
//
// Som bergen får kroppen luta ut över grannhexen och över fog — Timothys
// stående beslut 2026-07-27: ett massiv som syns resa sig in i dimman är en
// feature, för dimman ska ge ett löfte. MARKEN förblir klippt till sin hex, så
// ingen slätt blir kullterräng; bara kroppen lutar.
const SWELL_TONES = [HILL_RIM, HILL_HIGH, HILL_LIT, HILL_LOW, HILL_HOLLOW];

function swellProfile(q, r, i) {
  const w = 32 + rndInt(q, r, 3100 + i * 7, 18);   // 32–49 px — hexen är 44
  const h = 7 + rndInt(q, r, 3200 + i * 7, 7);     // 7–13 px, bergens är 15–25
  const apex = Math.round(w * (0.34 + rnd(q, r, 3300 + i * 7) * 0.24));

  // En eller två bikrön. Fler gjorde ryggen knölig — en kulle har ett par
  // svällningar, inte en kam. Bergens 2–3 spurs finns för att bryta tältet;
  // kupolen behöver inte brytas lika hårt, den är redan rund.
  const tops = [{ x: apex, h }];
  const spurs = 1 + rndInt(q, r, 3350 + i * 7, 2);
  for (let s = 0; s < spurs; s++) {
    tops.push({
      x: Math.round(w * (0.12 + rnd(q, r, 3400 + i * 7 + s * 3) * 0.80)),
      h: h * (0.45 + rnd(q, r, 3450 + i * 7 + s * 3) * 0.40),
    });
  }

  const prof = new Array(w);
  for (let x = 0; x < w; x++) {
    let y = 0;
    for (const t of tops) {
      const dx = x - t.x;
      // Räckvidden härleds ur BREDDEN, inte ur höjden. Bergen skalar sin reach
      // på höjden (1,15/0,75 × h) och kan göra det för att de är höga; med en
      // kulles 7–13 px gav samma grepp en reach på 47 px mot en halvbredd på 35
      // — kupolen kom aldrig ner inom sin egen bredd, klipptes rakt av och blev
      // ett brett platt BAND som läste som sanddyn. Det är precis det
      // peakProfile varnar för ("a flat-topped mesa"), och samma fälla gäller
      // dubbelt när formen är låg. Solsidan är den flackare, som hos bergen.
      const reach = w * (dx < 0 ? 0.34 : 0.26);
      const u = Math.min(1, Math.abs(dx) / reach);
      // Cosinusklocka: rundat krön OCH mjuk utlöpning i foten. En linjär ramp
      // (bergens) gav en pyramid; den här ger en kulle.
      y = Math.max(y, t.h * 0.5 * (1 + Math.cos(Math.PI * u)));
    }
    prof[x] = Math.max(0, Math.round(y));
  }
  return { w, h, apex, prof };
}

function drawSwell(ctx, px, py, p) {
  const x0 = Math.round(px - p.w / 2), y0 = Math.round(py);

  // Kontaktskuggan, nedåt-höger som allt annat i renderaren. Den är vad som
  // säger att kroppen VILAR på marken i stället för att sväva ovanpå den.
  ctx.globalAlpha = 0.22;
  ctx.fillStyle = '#241F18';
  for (let x = 0; x < p.w; x++) {
    if (p.prof[x] < 2) continue;
    ctx.fillRect(x0 + x + 2, y0, 1, 2);
  }
  ctx.globalAlpha = 1;

  for (let x = 0; x < p.w; x++) {
    const hgt = p.prof[x];
    if (hgt < 1) continue;
    const top = y0 - hgt;
    const lit = x < p.apex;
    // Tonen väljs på HÖJDEN i kolumnen, inte per ray som hos bergen. Bergens
    // fan-striering hör till klippa; en gräsklädd kulle har inga ådror, och
    // strieringen gjorde den fjällig. Här är det en ren höjdramp: krönet
    // ljusast, foten mörkast, och skuggsidan hela rampen ett steg ned.
    // Rampen går VERTIKALT i kolumnen: krön ljust, fot mörkt. Första versionen
    // valde en ton per kolumn ur kolumnens HÖJD — höga kolumner blev ljusa hela
    // vägen ner, låga mörka — och det är en horisontell gradient, alltså en
    // mjuk kudde, inte en kropp. Volym kommer av att tonen ändras NEDFÖR
    // formen; bergen gör samma sak per ray. Skuggsidan är hela rampen ett steg
    // ned, vilket är vad som ger krönet en kant att skymma bakom.
    const nT = SWELL_TONES.length;
    for (let y = 0; y < hgt; y++) {
      const v = hgt > 1 ? y / (hgt - 1) : 0;      // 0 = krön, 1 = fot
      const i = Math.min(nT - 1, Math.floor(v * (nT - 1)) + (lit ? 0 : 1));
      ctx.fillStyle = SWELL_TONES[i];
      ctx.fillRect(x0 + x, top + y, 1, 1);
    }
    // Krönkanten: EN pixel längs solsidans rygg, i fältets egen krönton. Det är
    // den kant som gör att ögat läser en linje att gå bakom — men den är
    // HILL_RIM och inte bergens vita, för ljusaste-pixeln är upptagen.
    if (lit && hgt > 1) {
      ctx.fillStyle = HILL_RIM;
      ctx.fillRect(x0 + x, top, 1, 1);
    }
  }
}

// Kullhexens kroppar. En eller två per hex — fler gör en hexagon full av
// knölar, vilket är den texturläsning princip 1 finns för att stoppa.
function drawSwells(ctx, cx, cy, q, r) {
  const n = 1 + rndInt(q, r, 3000, 2);
  const swells = [];
  for (let i = 0; i < n; i++) {
    const a = rnd(q, r, 3010 + i) * Math.PI * 2;
    // Förskjutningen är HALVERAD mot bergens tolv px. Kroppen får luta ut över
    // grannen och över fog (Timothy 2026-07-27) — men på en kullhex ensam i
    // dimman blev en förskjuten låg kupol en tunn kil med hård kant långt ute i
    // det svarta, och det läser som en trasig sprite, inte som ett massiv som
    // reser sig in i dimman. Bergen bär samma utstick för att deras siluett är
    // hög nog att läsas som form på egen hand; en låg kropp är det inte.
    const d = Math.sqrt(rnd(q, r, 3050 + i)) * 7;
    swells.push({
      x: cx + Math.cos(a) * d,
      // Foten under hexmitten, av samma skäl som bergens: med foten i mitten
      // blir det ett band bar mark längs nederkanten, och bar mark under en
      // landform är precis vad som gör hexagonen synlig igen.
      y: cy + Math.sin(a) * d * 0.6 + 7,
      p: swellProfile(q, r, i),
    });
  }
  swells.sort((a, b) => a.y - b.y);
  for (const s of swells) drawSwell(ctx, s.x, s.y, s.p);
}

function drawPeaks(ctx, cx, cy, q, r, terrain) {
  const rock = MTN_ROCK[terrain];
  const peaks = [];
  // ONE massif per hex, occasionally two. The reference draws a single mountain
  // across roughly two hexes, and at 26–42 px against a 44 px hex that is what
  // this produces: the mass necessarily runs over its own borders in every
  // direction — including DOWNHILL, over the hex below (Timothy 2026-07-27).
  // That is the intent, not a leak. The foot no longer stays inside the hex the
  // way princip 12 asks of a tree, because a mountain is not a tree: it is a
  // landform bigger than the grid, and a range that respects the grid reads as
  // tiles. What keeps it honest is that the SCREE — the ground — is still
  // clipped to its own hex, so a plain never turns into mountain terrain; only
  // the mass leans over it, exactly as a real massif shadows its foothills.
  const n = 1 + rndInt(q, r, 2000, 2);
  for (let i = 0; i < n; i++) {
    const a = rnd(q, r, 2010 + i) * Math.PI * 2;
    const d = Math.sqrt(rnd(q, r, 2050 + i)) * 10;
    peaks.push({
      x: cx + Math.cos(a) * d,
      // The foot sits BELOW the hex centre. With the foot on the centre a
      // 20 px massif rose past the top edge while leaving nineteen pixels of
      // bare scree along the bottom — and a band of flat ground under a
      // mountain is exactly what makes the hexagon visible again. Dropping the
      // foot balances the mass over its own tile and lets the talus reach the
      // hex below, which is the downhill spill Timothy asked for.
      y: cy + Math.sin(a) * d * 0.6 + 8,
      p: peakProfile(q, r, i),
    });
  }
  // Back to front: a summit lower down the hex is NEARER, so it must overlap
  // the one behind it. Sorting is what makes the group read as one mass with
  // depth rather than as several separate objects (princip 11).
  peaks.sort((a, b) => a.y - b.y);
  ctx.save();
  for (const pk of peaks) drawPeak(ctx, rock, pk.x, pk.y, pk.p);
  ctx.restore();
}

// ── Terrain detail — Settlers 2 quality ──────────────────────────────────
// `frame` togs bort med havsshimret: efter att dyningen flyttat till sitt eget
// pass finns ingen animerad MARK kvar, och en frame-parameter som ingen läser
// antyder att marken kan röra sig.
function drawDetail(ctx, cx, cy, terrain, seed, q, r) {
  ctx.save();
  switch (terrain) {
    case 'plains': {
      drawPlainsField(ctx, cx, cy);
      break;
    }
    case 'river_valley': {
      drawValleyField(ctx, cx, cy, q, r, false);
      break;
    }
    case 'river_delta': {
      drawValleyField(ctx, cx, cy, q, r, true);
      break;
    }
    case 'river': {
      drawRiver(ctx, cx, cy, q, r);
      break;
    }
    case 'forest_cedar': {
      // Golv bara. Kronorna är ett eget pass, av samma skäl som lunden.
      drawCedarFloor(ctx, cx, cy);
      break;
    }
    case 'forest_olive_grove': {
      // Floor only. The canopy is a second pass over every tile (see render()),
      // because a crown has to be allowed to hang over the hex border.
      drawForestFloor(ctx, cx, cy, q, r);
      break;
    }
    case 'hills': {
      drawHills(ctx, cx, cy);
      break;
    }
    case 'mountain_limestone':
    case 'mountain_red': {
      drawMountain(ctx, cx, cy, terrain);
      break;
    }
    case 'scrub_maquis': {
      drawScrubField(ctx, cx, cy);
      break;
    }
    case 'coastal_sea':
    case 'deep_sea':
      // Havet ritas inte här. Dyning och bränning är egna OKLIPPTA pass
      // (render() 1a3) — en havsyta vars enda märken slutar vid hexkanten
      // ritar hexkanten. Se drawSwell/drawSurf.
      break;
    case 'semi_desert': {
      drawDesertField(ctx, cx, cy);
      break;
    }
  }
  ctx.restore();
}

// ── Deposit resource icons (Sprint 4.5) — tiny pixel markers ────────────
function drawDepositIcons(ctx, cx, cy, tile) {
  const types = [];
  if (tile.copper_deposit) types.push('cu');
  if (tile.tin_deposit)    types.push('sn');
  if (tile.cedar_deposit)  types.push('cd');
  if (tile.silver_deposit) types.push('ag');
  if (!types.length) return;
  ctx.save();
  // Charcoal outline on every marker. These are game information sitting on
  // top of terrain that now has its own texture and tone, and an unoutlined
  // solid loses its silhouette the moment the ground behind it stops being
  // flat — cedar's dark green triangle disappeared into the canopy outright.
  // The outline is also what the project's own pixel-art rule asks for
  // (1px charcoal on solids, temenos_designprinciper.md).
  ctx.strokeStyle = '#221E18';
  ctx.lineWidth = 0.8;
  ctx.lineJoin = 'round';
  types.forEach((t, i) => {
    // Ögonkoll 2026-08-05 (Timothy): alla fyra markörerna uppförstorade ca 1,4×.
    // Radsteget följer med från 5 till 7 px — annars överlappar en hex med
    // flera fyndigheter sina egna symboler.
    const ox = cx + 9, oy = cy - 8 + i * 7;
    switch (t) {
      case 'cu':
        ctx.fillStyle = '#C47C20';
        ctx.beginPath(); ctx.arc(ox, oy, 3, 0, Math.PI*2); ctx.fill(); ctx.stroke();
        break;
      case 'sn':
        // #909090 was metallic tin — a smelted product, not what sits in the
        // ground. A tin deposit is cassiterite, dark reddish-brown to black.
        // Measured 2026-08-04: at #909090, ΔE76 tenn↔silver was 22.5 against
        // tenn↔copper 61.0 and silver↔copper 72.7 — three times closer than
        // any other pair, and the same pair whose SHAPES (rect 4×3, diamond
        // 4×5) read most alike at 1:1. Both channels were weakest on the same
        // pair. #4C2208 (tools/deposit_contrast.py) moves the separation into
        // lightness, the channel that survives small sizes best: ΔE76
        // tenn↔silver 72.2 (ΔL* -61.7, the separation is almost entirely
        // lightness), tenn↔contour(#221E18) 27.1, tenn↔copper 51.5,
        // tenn↔cedar 64.2, tenn↔mountain_red 36.3/26.7, tenn↔hills 53.8/45.0
        // — held against everything tin can sit on or next to, not just
        // silver, since tin sits on height per mapgen (showcase-forest.html).
        // Ögonkoll 2026-08-05 (Timothy): markören läste som ett svart klot —
        // #4C2208 låg bara ΔE76 27,1 från sin EGEN kontur (#221E18), och vid
        // 4×3 px upptar en 0,8 px-kontur så stor andel av rutan att fyllningen
        // aldrig hinner synas. Konturen tas inte bort (pixelregeln i
        // temenos_designprinciper.md); i stället ljusas fyllningen och rutan
        // växer, så fyllningen dominerar ytan. #6B3410 mätt med
        // tools/deposit_contrast.py: tenn↔kontur 27,1 → 38,6, tenn↔silver
        // kvar på 69,7 (separationen ligger fortfarande i ljushet, ΔL* -52,0),
        // tenn↔koppar 38,9 — långt över de 22,5 som utlöste omarbetningen, och
        // koppar är en cirkel mot tennets rektangel.
        ctx.fillStyle = '#6B3410';
        ctx.fillRect(ox - 4, oy - 2.5, 8, 5);
        ctx.strokeRect(ox - 4, oy - 2.5, 8, 5);
        break;
      case 'cd':
        ctx.fillStyle = '#2A7010';
        ctx.beginPath(); ctx.moveTo(ox, oy - 4); ctx.lineTo(ox + 3.5, oy + 2); ctx.lineTo(ox - 3.5, oy + 2); ctx.closePath(); ctx.fill(); ctx.stroke();
        break;
      case 'ag':
        ctx.fillStyle = '#C0C8D8';
        ctx.beginPath(); ctx.moveTo(ox, oy - 3.5); ctx.lineTo(ox + 3, oy); ctx.lineTo(ox, oy + 3.5); ctx.lineTo(ox - 3, oy); ctx.closePath(); ctx.fill(); ctx.stroke();
        break;
    }
  });
  ctx.restore();
}

// ── Kuststadens bank (E3) ────────────────────────────────────────────────
// Stadsmassan är 62 px mot hexens 44 och terrängblind, så en stad vid kusten
// lade sin gård och sina hus rakt ut på öppet vatten (Timothy 2026-07-27).
// Banken är svaret: en fransad strandplatta under massans sjövända fot, i
// STRANDBANDETS palett (E2), så att udden fortsätter samma sand som ligger
// längs kusten runt omkring i stället för att bli ett eget föremål.
//
// Mekanismen är avsiktligt geometrifri. Den frågar inte "vilken kustform är
// det här" utan bara två saker per rasterblock: *hur långt är blocket från
// massans fot* och *ligger det i en havshex*. Därför faller hav i N, i S, mot
// ett hörn, en udde med två havssidor och en nästan-ö ur samma tio rader —
// vilket är slicens stoppvillkor: krävs ett specialfall per kustgeometri är
// mekanismen fel.
//
// Havstestet är också FOW-grinden. `terrainAt` svarar `'fog'` för en osedd
// granne och `undefined` utanför kartan, och ingetdera är hav — en bank kan
// alltså aldrig avslöja att rutan bakom dimman är vatten (samma regel som
// skogsbrynet och strandbandet).
// Banken följer massans SILUETT, inte dess fotlinje. Första ansatsen mätte
// några pixlar under foten och lade ett band där: resultatet blev en rak tunga
// ut i vattnet som läste som en brygga, medan gårdens övre halva fortfarande
// låg direkt på blått. I ¾-elevation lutar marken bort från betraktaren och
// sträcker sig alltså UPPÅT i bild — banken måste följa den konturen, annars
// beskriver den en annan mark än den staden står på.
//
// Uppåt slutar den vid GÅRDENS översta pixel. Ovanför den är allt tak, murkrön
// och torn — sand däruppe vore sand i luften, och det är exakt vad hav i norr
// hade gett.
// Bankens yttersta räckvidd i logiska pixlar: både loopens gräns och det tak
// brusbredden nedan klipps mot.
const BANK_MAX = 7;

/** Massans mått som banken behöver, allt uttryckt PER KOLUMN: gårdens översta
 *  rad (takets gräns nedåt), massans understa rad (marken den vilar på), och
 *  vilka kolumner massan över huvud taget har pixlar i.
 *
 *  Ett utkast blandade en per-RAD-kontur (siluettens vänster/höger) med den här
 *  per-kolumn-foten, och de två beskrev olika saker: strax under en kolumns fot
 *  har raden redan smalnat av, så det vågräta avståndet sköt i höjden och
 *  banken uteblev precis där staden stod med fötterna i vattnet. Måttet visade
 *  bara 10 kolumner torrare medan bilden såg löst. **En bas, inte två.**
 *
 *  Gårdens övre gräns kommer ur `sprite.yardTop`, som bygget lägger dit — den
 *  går inte att läsa ur pixlarna, för gårdens bakre rader är övertäckta.
 *  Räknas en gång per sprite och sparas på den: spritarna är åtta stycken och
 *  byggs vid modulladdning, så cachen kan aldrig bli inaktuell. */
export function spriteGround(sprite) {
  if (sprite.ground) return sprite.ground;
  const { w, runs } = sprite;
  const foot = new Int16Array(w).fill(-1);
  let botMax = 0, yardMin = sprite.h, colL = w, colR = -1;
  for (const r of runs) {
    if (r.y > botMax) botMax = r.y;
    for (let x = r.x; x < r.x + r.n; x++) if (r.y > foot[x]) foot[x] = r.y;
  }
  for (let x = 0; x < w; x++) if (foot[x] >= 0) { if (x < colL) colL = x; colR = x; }
  // Tomma kolumner i kanterna ärver närmaste grannes värde, annars faller
  // konturen till noll just där siluetten smalnar av. Fyllningen sker i KOPIOR:
  // `foot` med sina −1 kvar är den enda ärliga uppgiften om var massan faktiskt
  // har pixlar, och tools/footing.py mäter mot den — ett mått som probar under
  // en tom kolumn mäter öppet hav och kallar det blöta fötter.
  const yard = Int16Array.from(sprite.yardTop), footFill = Int16Array.from(foot);
  for (const a of [footFill, yard]) {
    let last = -1;
    for (let x = 0; x < w; x++) { if (a[x] < 0) a[x] = last; else last = a[x]; }
    last = -1;
    for (let x = w - 1; x >= 0; x--) { if (a[x] < 0) a[x] = last; else last = a[x]; }
  }
  // Markens övre gräns per kolumn. Normalt är den gårdens topp — men i de
  // yttersta kolumnerna sticker ett hus ut FÖRBI gårdspolygonen, och där ligger
  // dess fot ovanför gårdens topprad. En gräns som bara läste gården lämnade
  // just de husen hängande över vattnet (mätt: 6 blöta kolumner som inte
  // rörde sig när banken lades till). Ett hus står alltid på mark, så gränsen
  // är den av de två som ligger högst.
  for (let x = 0; x < w; x++) if (footFill[x] < yard[x]) yard[x] = footFill[x];
  for (let x = colL; x <= colR; x++) if (yard[x] < yardMin) yardMin = yard[x];
  sprite.ground = { yard, yardMin, colL, colR, botMax, foot, footFill };
  return sprite.ground;
}

function drawCityBank(ctx, cx, cy, sprite) {
  const STEP = 2;
  const { yard, yardMin, colL, colR, botMax, footFill } = spriteGround(sprite);
  const ox = Math.round(cx) - (sprite.w >> 1);
  const oy = Math.round(cy) + cityTop(sprite);
  const xL = ox + colL, xR = ox + colR;
  const y1 = oy + botMax + BANK_MAX;
  // En rad ovanför gården: konturpasset lägger bläck runt hela massan, och en
  // bank som slutar exakt vid gårdens översta rad lämnar den svarta linjen
  // liggande i vattnet.
  for (let wy = Math.floor((oy + yardMin - 1) / STEP) * STEP; wy <= y1; wy += STEP) {
    for (let wx = Math.floor((xL - BANK_MAX) / STEP) * STEP; wx <= xR + BANK_MAX; wx += STEP) {
      // Blocket täcker TVÅ kolumner, och längs en diagonal kant skiljer deras
      // fot två pixlar. Läses bara mittkolumnens fot hamnar bankens överkant
      // två pixlar för högt för den andra, och kvar blir en enpixels ränna av
      // vatten längs hela den snedställda kanten — den rännan var de sista sex
      // blöta kolumnerna i mätningen. Blocket tar därför den LÄGSTA foten och
      // den HÖGSTA marken av de två: bandet ska rymma båda kolumnerna, inte
      // den ena.
      const s0 = Math.min(colR, Math.max(colL, wx - ox));
      const s1 = Math.min(colR, Math.max(colL, wx + 1 - ox));
      // Uppåt är gränsen HÅRD, inte ett avstånd: sand får aldrig krypa upp
      // ovanför marken, oavsett hur nära massan blocket ligger.
      if (wy + STEP / 2 < oy + Math.min(yard[s0], yard[s1]) - 1) continue;
      const dx = wx < xL ? xL - wx : Math.max(0, wx - xR);
      // Utåt och nedåt mäts avståndet. INNANFÖR kolumnens markband är det noll
      // eller negativt, och då fylls blocket — det havet ligger BAKOM stadens
      // mark och ska vara mark, annars lyser vatten genom gårdens fransade kant.
      const d = Math.max(dx, wy + STEP / 2 - (oy + Math.max(footFill[s0], footFill[s1])));
      if (d > BANK_MAX) continue;
      // Bredden ur det globala bruset, precis som strandbandets: banken och
      // bandet den möter måste variera i samma fält, annars läser övergången
      // vid hexkanten som en söm mellan två olika stränder.
      const w = 3.5 + 4.0 * noiseAt(wx, wy, 9, 5151);
      if (d > w) continue;
      const mx = wx + STEP / 2, my = wy + STEP / 2;
      let [q, r] = hexAtWorld(mx, my);
      if (!isSeaTerrain(terrainAt(q, r))) {
        // Blocket är 2×2 och tilldelas den hex dess MITT ligger i. Vid
        // kustlinjen straddlar det kanten: mitten hamnar på land medan halva
        // blocket ligger i havshexen och blir stående blått under stadens fot.
        // Testa därför även blockets yttersta hörn — det som pekar bort från
        // landhexens mitt. Bias:et går alltid mot MER bank, och den pixel sand
        // som då hamnar på landsidan möter strandbandets egen sand. Mätt värde
        // på egen hand: en enda pixel i hela kustscenen — men det är den pixeln
        // som skiljer noll från nästan noll.
        const c = hexPx(q, r);
        [q, r] = hexAtWorld(mx + (mx >= c.x ? 1 : -1), my + (my >= c.y ? 1 : -1));
        if (!isSeaTerrain(terrainAt(q, r))) continue;
      }
      ctx.fillStyle = d > w * 0.55 ? SHORE_WET : SHORE_SAND;
      ctx.fillRect(wx, wy, STEP, STEP);
    }
  }
}

// ── Province building sprite + flag ──────────────────────────────────────
function drawProvince(ctx, cx, cy, p) {
  // Razed (Del 2b sack) or collapsed: an abandoned ruin, not a standing city —
  // no owner, no flag, no garrison dot. Dim broken rubble instead of the building.
  if (p.state === 'razed' || p.state === 'collapsed') {
    ctx.save();
    ctx.fillStyle = '#5A5048';
    ctx.strokeStyle = '#2A2420';
    ctx.lineWidth = 0.8;
    ctx.fillRect(cx - 4, cy - 2, 3, 4);
    ctx.strokeRect(cx - 4, cy - 2, 3, 4);
    ctx.fillRect(cx, cy - 4, 4, 3);
    ctx.strokeRect(cx, cy - 4, 4, 3);
    ctx.fillStyle = '#3A342E';
    ctx.fillRect(cx - 1, cy + 1, 2, 1);
    ctx.restore();
    return;
  }
  const walls = Math.min(3, p.walls || 0);
  const accent = p.own ? '#D4AC0D' : (p.allied ? '#4CAF50' : '#C0392B');
  ctx.save();

  // Outposten är ingen bosättning (province-rad utan settlement-rad) och får
  // därför ingen stadsmassa — den skulle ljuga om en by som inte finns.
  // Behåller den gamla lilla rutan tills outposterna rivs ur MVP:n.
  if (p.is_outpost) {
    ctx.fillStyle = '#D4B890';
    ctx.strokeStyle = '#7A5030';
    ctx.lineWidth = 0.8;
    ctx.fillRect(cx - 3, cy - 2.5, 6, 5);
    ctx.strokeRect(cx - 3, cy - 2.5, 6, 5);
    ctx.fillStyle = accent;
    ctx.fillRect(cx - 1, cy - 7, 2, 4);
    ctx.restore();
    return;
  }

  // Massan: två befolkningsled (serverns size_tier, gräns 800 inv.) × fyra
  // murnivåer. Leden var fyra; de två största gick inte att få att läsa som
  // städer och ströks (Timothy 2026-07-27).
  // Kulturstrimman som låg på den gamla rutan är BORTA — den satt på ett
  // föremål som inte finns längre, och kulturen ska bäras av hela siluetten
  // när de kulturspecifika leden byggs (Timothy 2026-07-27: idag bara akhaier).
  // Banken FÖRE massan: den är mark, massan står på den. Grinden är
  // grannskapet — en stad utan havsgranne betalar ingenting alls.
  const tier = p.size_tier || 0;
  if (neighborDirs(p.q, p.r, isSeaTerrain).length)
    drawCityBank(ctx, cx, cy, citySprite(tier, walls));

  const sprite = drawCityMass(ctx, tier, walls, cx, cy);
  const top = cy + cityTop(sprite);

  // Standaret på taknocken — ägarskapets enda färgsignal på kartan. Den reser
  // sig UR massan, inte ovanför den: 0,6 px stång i accentfärg svävade löst över
  // en palatsstad och lästes som skräp. Stången är mörk och stången RÖR taket.
  ctx.fillStyle = '#1F1A14';
  ctx.fillRect(cx, top - 5, 1, 6);
  ctx.fillStyle = accent;
  ctx.fillRect(cx + 1, top - 5, 4, 1);
  ctx.fillRect(cx + 1, top - 4, 3, 1);
  ctx.fillRect(cx + 1, top - 3, 2, 1);

  // Garnisonsprick — hör till PORTEN, inte till taket. Den satt tidigare i
  // luften till höger om massan där en palatsstad inte har någon bebyggelse.
  // p.own dropped (fow/frammande-enheter, 2026-08-03): the server now zeroes
  // army_total itself for anything not in the viewer's LIVE tier (world.go),
  // so a positive count here already means "garrison visible right now" —
  // no separate FOW check needed on this side.
  if (p.army_total > 0 && State.camera.zoom >= GARRISON_DOT_ZOOM) {
    ctx.fillStyle = '#8B1A1A';
    ctx.strokeStyle = '#3A0A0A';
    ctx.lineWidth = 0.5;
    const gx = cx + (sprite.w >> 1) - 4, gy = cy + cityFoot(sprite) - 4;
    ctx.fillRect(gx, gy, 3, 3);
    ctx.strokeRect(gx, gy, 3, 3);
    ctx.fillStyle = '#E8B0B0';
    ctx.fillRect(gx + 1, gy + 1, 1, 1);
  }
  ctx.restore();
}

// ── Province name label (two-pass for legibility) ────────────────────────
function drawLabel(ctx, cx, cy, text, own) {
  ctx.save();
  ctx.font = own ? 'bold 7px monospace' : '6px monospace';
  ctx.textAlign = 'center';
  ctx.textBaseline = 'top';
  ctx.strokeStyle = '#000000aa';
  ctx.lineWidth = 2;
  // Etiketten står PÅ hexens underkant (halva höjden är 19), vilket är precis
  // där Timothy vill ha den (2026-07-27): namnet är massans sockel, inte en lös
  // rad under den. Sedan centreringen 2026-08-04 (`cityTop`) räcker inte längre
  // ett fast talpar — `TOWN` är 41 px hög i en 38 px hög hex och når därmed 2 px
  // NEDANFÖR hexstrecket. Etiketten ligger kvar på 19 och massans fransade
  // gårdskant får löpa in bakom textens mörka stroke; det är fransen, inte
  // byggda pixlar, som möter bokstäverna. Ändras ankringen måste den här raden
  // provas om i `tools/shot.py cities` vid 1:1.
  ctx.strokeText(text, cx, cy + 19);
  ctx.fillStyle = own ? '#F9E79F' : '#E8D0A8';
  ctx.fillText(text, cx, cy + 19);
  ctx.restore();
}

// ── Activity overlay badge — build/train/idle indicator ──────────────────
function drawActivityBadge(ctx, cx, cy, p) {
  ctx.save();
  // Strax till vänster om standaret, på samma höjd: de två är stadens
  // statusmärken och ska läsa som EN grupp. Den gamla fasta −13 hamnar mitt
  // inne i en palatsstads takrader, och ett märke placerat vid massans
  // vänstra kant svävar i tomrummet ovanför den lägre flygeln.
  const sprite = citySprite(p.size_tier || 0, p.walls || 0);
  const bx = cx - 9, by = cy + cityTop(sprite) - 5;
  if (p.build_active) {
    // Hammer head (orange)
    ctx.fillStyle = '#D4780A';
    ctx.fillRect(bx, by, 5, 3);
    ctx.fillStyle = '#7A5030';
    ctx.fillRect(bx + 2, by + 2, 1, 3);
  } else if (p.train_active) {
    // Sword point (grey-blue)
    ctx.fillStyle = '#A8B8C8';
    ctx.beginPath();
    ctx.moveTo(bx + 2, by);
    ctx.lineTo(bx + 4, by + 5);
    ctx.lineTo(bx + 2, by + 4);
    ctx.lineTo(bx, by + 5);
    ctx.closePath();
    ctx.fill();
    ctx.fillStyle = '#7A5030';
    ctx.fillRect(bx + 1, by + 4, 2, 2);
  } else {
    // Idle — dim grey dot
    ctx.fillStyle = '#606060';
    ctx.beginPath();
    ctx.arc(bx + 2, by + 2, 2, 0, Math.PI * 2);
    ctx.fill();
  }
  ctx.restore();
}

// ── Dirt road between two province centres ────────────────────────────────
function drawRoad(ctx, ax, ay, bx, by) {
  ctx.save();
  ctx.strokeStyle = '#9A7850';
  ctx.lineWidth = 1.2;
  ctx.setLineDash([3, 4]);
  ctx.beginPath();
  ctx.moveTo(ax, ay);
  ctx.lineTo(bx, by);
  ctx.stroke();
  ctx.setLineDash([]);
  ctx.restore();
}

// ── Karavan — last och riktning först, ägandet sekundärt ─────────────────
function drawCaravan(ctx, x, y, walkPhase) {
  drawActor(ctx, 'caravan', x, y, '', walkPhase);
}

// ── Runner — order-Runners (kind='order') bär karmosin accent så de läses som
// befäl, inte diplomati; egna Runners bär dessutom en liten guldvimpel så det
// syns VEMS Runner det är (temenos_orderlopare_plan.md Fas 5).
function drawMessenger(ctx, x, y, walkPhase, isOrder, isOwn, delivering) {
  ctx.save();
  // Levererar: Runnern har nått fram och stannat för att lämna över kuvertet —
  // frys gången så hon inte joggar på stället (Timothy 2026-07-17) under
  // worker-pollfönstret innan ordern tillämpas serversidan.
  const phase = delivering ? 0 : walkPhase;
  drawActor(ctx, 'runner', x, y, '', phase, isOrder ? '#A03A2A' : '#6B8B4A');
  const bob = (delivering || walkPhase < 2) ? 0 : -1;
  if (isOwn) {
    ctx.fillStyle = '#D8B84A';
    ctx.fillRect(x - 4, y - 16 + bob, 1, 4);
    ctx.fillRect(x - 3, y - 16 + bob, 2, 1);
  }
  if (delivering) {
    // Svag guldpuls — ett medvetet "lämnar över ordern"-slag.
    ctx.globalAlpha = 0.3 + 0.25 * Math.sin(State.animFrame * 0.12);
    ctx.strokeStyle = '#D8B84A';
    ctx.lineWidth = 0.6;
    ctx.beginPath();
    ctx.arc(x, y - 5, 8, 0, Math.PI * 2);
    ctx.stroke();
  }
  ctx.restore();
}

// ── Rural projections (Fas A2, megaron_lokal_varld.md) ───────────────────
// A rural sprite is NOT a new building — it is a cartographic projection of an
// existing city building onto a compatible catchment hex, placed server-side
// (rural-projections endpoint). Static (no gait): these are places, not movers.
// 1px charcoal outline, saturated foreground, no AA — temenos_designprinciper.
const RURAL_OUTLINE = '#33291E';

function drawFarm(ctx, x, y) {
  ctx.fillStyle = '#C9A227';         // ripe wheat
  ctx.strokeStyle = RURAL_OUTLINE; ctx.lineWidth = 0.7;
  ctx.fillRect(x - 4, y - 1, 8, 4);  // tilled plot
  ctx.strokeRect(x - 4, y - 1, 8, 4);
  ctx.strokeStyle = '#8A6A12';       // furrows
  ctx.beginPath();
  ctx.moveTo(x - 2, y - 1); ctx.lineTo(x - 2, y + 3);
  ctx.moveTo(x, y - 1);     ctx.lineTo(x, y + 3);
  ctx.moveTo(x + 2, y - 1); ctx.lineTo(x + 2, y + 3);
  ctx.stroke();
}

function drawMine(ctx, x, y) {
  ctx.fillStyle = '#8A8078';         // grey spoil heap
  ctx.strokeStyle = RURAL_OUTLINE; ctx.lineWidth = 0.7;
  ctx.beginPath();
  ctx.moveTo(x - 4, y + 3);
  ctx.lineTo(x, y - 3);
  ctx.lineTo(x + 4, y + 3);
  ctx.closePath();
  ctx.fill(); ctx.stroke();
  ctx.fillStyle = '#241C16';         // adit mouth
  ctx.fillRect(x - 1, y + 1, 2, 2);
}

function drawLumbermill(ctx, x, y) {
  ctx.strokeStyle = RURAL_OUTLINE; ctx.lineWidth = 0.7;
  ctx.fillStyle = '#7B4F28';         // stacked logs (end-on)
  for (const [dx, dy] of [[-3, 1], [0, 1], [3, 1], [-1.5, -2], [1.5, -2]]) {
    ctx.beginPath();
    ctx.arc(x + dx, y + dy, 1.6, 0, Math.PI * 2);
    ctx.fill(); ctx.stroke();
  }
  ctx.fillStyle = '#C8A464';         // core rings
  for (const [dx, dy] of [[-3, 1], [0, 1], [3, 1], [-1.5, -2], [1.5, -2]]) {
    ctx.fillRect(x + dx - 0.5, y + dy - 0.5, 1, 1);
  }
}

const RURAL_SPRITES = { farm: drawFarm, mine: drawMine, lumbermill: drawLumbermill };

// ── Keyboard pan (WASD / arrows) ────────────────────────────────────────────
// Currently-held direction keys, maintained by the keydown/keyup listeners
// registered in initMap() below and consumed once per rendered frame inside
// render(). A Set rather than four booleans mainly so panDelta() below reads
// as "which of these are down" without four separate branches per key alias.
const PAN_KEYS = new Set(['w', 'a', 's', 'd', 'arrowup', 'arrowdown', 'arrowleft', 'arrowright']);
const heldKeys = new Set();

// Held-keys + elapsed-ms → screen-space camera delta. Exported standalone
// (not inlined into render()) so the direction/diagonal math is unit-testable
// without a canvas: given a key set and a frame's dt, what does the camera
// move by.
//
// Sign convention (easy to get backwards — verified against the existing
// mouse-drag pan a few lines down in initMap(), which does
// `camera.x += e.clientX - lastMouse.x`): dragging the map right increases
// camera.x and moves the *view* west, because camera.x is the translate
// applied before the world is drawn — moving the content right on screen is
// the same as the viewpoint sliding west. A direction key reproduces the drag
// that would visually pan that way: D (east) replays a leftward drag, so it
// DECREASES camera.x; A (west) replays a rightward drag, so it increases
// camera.x. Same logic on the vertical axis: S (south) decreases camera.y,
// W (north) increases it.
export function panDelta(held, dtMs, speedPxPerSec = PAN_SPEED_PX_PER_SEC) {
  let dx = 0, dy = 0;
  if (held.has('w') || held.has('arrowup'))    dy += 1;
  if (held.has('s') || held.has('arrowdown'))  dy -= 1;
  if (held.has('a') || held.has('arrowleft'))  dx += 1;
  if (held.has('d') || held.has('arrowright')) dx -= 1;
  if (!dx && !dy) return { dx: 0, dy: 0 };
  // Normalize so a diagonal (W+D) isn't sqrt(2)x faster than a single direction.
  const norm = Math.hypot(dx, dy);
  const dist = speedPxPerSec * (dtMs / 1000);
  return { dx: (dx / norm) * dist, dy: (dy / norm) * dist };
}

// ── Main renderer ─────────────────────────────────────────────────────────

export function toggleActivityOverlay() {
  State.activityOverlay = !State.activityOverlay;
  document.getElementById('activity-btn').classList.toggle('active', State.activityOverlay);
  State.dirty = true;
}

export const canvas = document.getElementById('hex-canvas');
const tooltip = document.getElementById('tile-tooltip');
const container = document.getElementById('map-root');

function resizeCanvas() {
  canvas.width  = container.clientWidth;
  canvas.height = container.clientHeight;
}
resizeCanvas();
window.addEventListener('resize', resizeCanvas);

const ctx = canvas.getContext('2d');
ctx.imageSmoothingEnabled = false;

let lastPanFrameMs = performance.now();

// Rendertid per pass i millisekunder, skriven om varje ritad frame. Läses av
// showcase-world.html och tools/rendertime.py. Arbetsregel 0
// (megaron_terrangrendering) kräver att rendertiden mäts före och efter varje
// ändring med identisk fixtur — och en totalsiffra säger inte VILKET pass som
// blev dyrt, vilket är hela frågan när ett nytt terrängfält läggs till. De
// dryga tiotalet performance.now()-anropen per frame ligger i mikrosekunder
// mot en frame som mäts i millisekunder.
export const renderTimings = {};
let passT0 = 0;
function pass(name) {
  const t = performance.now();
  renderTimings[name] = t - passT0;
  passT0 = t;
}

// Exported for the offline visual harness (web/static/showcase-forest.html),
// which stubs requestAnimationFrame and drives one frozen frame at a time so
// terrain screenshots are pixel-deterministic. Game code still enters the loop
// through initMap() only. The harness never holds a pan key, so the block
// below is a no-op there and the frozen frame stays deterministic.
export function render() {
  // Apply held-key panning before the dirty check below — a held key must
  // keep moving the camera (and re-marking State.dirty) every frame even on
  // an otherwise-quiet map, or panning freezes as soon as nothing else is
  // animating (see render()'s early-return a few lines down).
  const nowMs = performance.now();
  const dtMs = nowMs - lastPanFrameMs;
  lastPanFrameMs = nowMs;
  if (heldKeys.size) {
    const { dx, dy } = panDelta(heldKeys, dtMs);
    if (dx || dy) {
      State.camera.x += dx;
      State.camera.y += dy;
      clampCamera();
      State.dirty = true;
    }
  }

  State.animFrame++;
  const seaTick = State.animFrame >> 5;
  const seaChanged = seaTick !== State.lastSeaTick;
  if (seaChanged) State.lastSeaTick = seaTick;

  // Främmande enheters kontur blinkar, och en blink kräver att duken faktiskt
  // ritas om. Loopen nedan hoppar över omritning när ingenting "rör sig" — och
  // en stillastående fiende i din synrand är precis det fallet, så konturen
  // frös och blinkade aldrig (funnet i acceptanskörningen 2026-08-03, inte av
  // något test: en fryst bild ser identisk ut med en korrekt bild). Väck
  // loopen på FASBYTET, som havstakten gör, i stället för varje frame.
  const blinkTick = (State.animFrame / FOREIGN_BLINK_FRAMES) | 0;
  const blinkChanged = State.foreignUnitData.length > 0 && blinkTick !== State.lastBlinkTick;
  if (blinkChanged) State.lastBlinkTick = blinkTick;

  if (!State.dirty && !seaChanged && !blinkChanged && State.marchData.length === 0 && State.messengerData.length === 0 && State.tradeData.length === 0
      && !State.unitsData.some(u => u.status === 'marching')) {
    requestAnimationFrame(render);
    return;
  }
  State.dirty = false;
  passT0 = performance.now();

  ctx.clearRect(0, 0, canvas.width, canvas.height);
  ctx.save();
  ctx.translate(State.camera.x, State.camera.y);
  ctx.scale(State.camera.zoom * SCALE, State.camera.zoom * SCALE);

  // 0. Viewport-culling. Renderaren ritade varje tile i `State.tileData` oavsett
  // om den låg i bild, och terrängpassen har vuxit sedan det var billigt: mätt
  // på världsfixturen kostade 1:1 fyrtio ms trots att bara en bråkdel av kartan
  // syns där. Att kostnaden bara var 2,4× minzoomens — som visar hela världen —
  // är beviset på att den var i stort sett OBEROENDE av synlighet.
  //
  // Marginalen finns för att flera pass med flit ritar UTANFÖR sin egen hex:
  // massivet är upp till 61 px brett med 12 px förskjutning, kullkroppen 49/7,
  // kronorna hänger över kanten och stadsmassan är bredare än hexen. En tile
  // vars mitt ligger strax utanför duken måste alltså ändå få rita, annars
  // saknas dess överlutande del vid kanten — vilket syns som att berg och träd
  // klipps bort när man panorerar. 96 px är två hexbredder: bekvämt över det
  // största kända utsticket, och billigt nog att inte vara värt att snåla på.
  //
  // `drawSwellField` står UTANFÖR det här och läser fortfarande hela
  // `tileData`. Den räknar ut dyningsrastrets utsträckning ur havstilarnas
  // bounding box, så en cullad lista skulle flytta rastrets origin och
  // förskjuta hela dyningen när man panorerar. Den är redan klippt mot duken.
  const vis = visibleTiles();

  // 1. Base terrain fill — ALL tiles first, before any ground texture.
  // fillHex strokes the hex outline with lineWidth 1, which reaches half a
  // pixel outside the polygon. Filling and texturing in one loop therefore let
  // each tile's base paint over its ALREADY-TEXTURED neighbour along the shared
  // edge, leaving a light line on every border — the residual "grid" inside the
  // forest was this overdraw, not a drawn grid (none exists; the stroke uses
  // the fill colour). Separating the passes removes it outright.
  for (const t of vis) {
    const {x,y} = hexPx(t.q, t.r);
    const base = TERRAIN_BASE[t.terrain] || TERRAIN_BASE.fog;
    const seed = (t.q*137 + t.r*31) & 0xff;
    fillHex(ctx, hexPts(x, y), base.c0, base.c1, seed);
  }
  pass('base');

  // 1a. Ground texture pass.
  for (const t of vis) {
    if (t.terrain === 'fog') continue;
    const {x,y} = hexPx(t.q, t.r);
    const seed = (t.q*137 + t.r*31) & 0xff;
    drawDetail(ctx, x, y, t.terrain, seed, t.q, t.r);
  }
  pass('ground');

  // 1a1. Markövergången — grannens ton blöder in i vår kantzon där två
  // marktyper möts. Egen loop av samma skäl som strandbandet nedan: zonen är
  // en egenskap hos MÖTET mellan två hexar, inte hos terrängen i en av dem.
  // Före strandbandet, för sanden är kartans ljusaste yta och ska inte dithras
  // sönder av en markzon ritad ovanpå den.
  for (const t of vis) {
    if (!GROUND_BLEND.has(t.terrain)) continue;
    const { x, y } = hexPx(t.q, t.r);
    drawGroundBlend(ctx, x, y, t.q, t.r, t.terrain);
  }
  pass('blend');

  // 1a2. Strandbandet — sand på LANDSIDAN längs varje kant mot hav. Klippt.
  // Egen loop och inte en gren i drawDetail: bandet är en egenskap hos MÖTET
  // mellan två hexar, inte hos terrängen i en av dem, och samma sand ska ligga
  // under slätt, lund, kulle och berg utan att var och en får sin egen kopia.
  // 1a3. Havet — dyning över hela ytan, bränning där den möter land. OKLIPPT.
  for (const t of vis) {
    // Gallringen går på `isWaterTerrain`, inte `isSeaTerrain`. Floden är inte
    // hav, alltså släpptes en flodhex förbi hit och fick strandband + bränning
    // mot sina havsgrannar — en sandstrand runt vattnet vid varje mynning.
    // Enhetsriggen fångade det; ingen tonmätning kunde ha gjort det.
    // Grannskapstestet nedan står kvar på `isSeaTerrain` med flit: det är LAND
    // som ska få strand mot HAV, och en landhex vid en flod ska inte få det
    // (kontrastbudgeten, princip 6 — den ljusa sanden hör kustlinjen till).
    if (t.terrain === 'fog' || isWaterTerrain(t.terrain)) continue;
    const seaDirs = neighborDirs(t.q, t.r, isSeaTerrain);
    if (!seaDirs.length) continue;
    const { x, y } = hexPx(t.q, t.r);
    drawShore(ctx, x, y, seaDirs);
    drawSurf(ctx, x, y, seaDirs, State.animFrame >> 5);
  }
  pass('shore');

  for (const t of vis) {
    if (t.terrain !== 'deep_sea') continue;
    const shelf = neighborDirs(t.q, t.r, n => n === 'coastal_sea');
    if (!shelf.length) continue;
    const { x, y } = hexPx(t.q, t.r);
    drawDepthBand(ctx, x, y, shelf);
  }
  drawSwellField(ctx, State.animFrame >> 5);
  pass('sea');

  // 1a4. Kullarnas kroppar — efter all mark, före lövverket. Efter marken av
  // samma skäl som bergen: en kropp som ska SKYMMA får inte målas över av
  // grannens grundfyllning. Före lövverket för att träd står PÅ en kulle, inte
  // bakom den.
  //
  // Sorterade N→S över hela kartan, inte bara inom hexen: en sydligare kulle är
  // NÄRMARE och måste skymma den nordligare, och det avgörs mellan hexar lika
  // ofta som inom dem. `State.tileData` har ingen ORDER BY från servern, så utan
  // den här sorteringen vore ritordningen mellan två kullhexar godtycklig —
  // samma latenta indeterminism som lövverket bär och som E4 ska lösa där.
  const swellTiles = vis.filter(t => t.terrain === 'hills');
  swellTiles.sort((a, b) => (2 * a.r + a.q) - (2 * b.r + b.q));
  for (const t of swellTiles) {
    const { x, y } = hexPx(t.q, t.r);
    drawSwells(ctx, x, y, t.q, t.r);
  }
  pass('swells');

  // 1b. Canopy pass — after every tile's ground is down, so a crown may hang
  // over the hex border without the next tile's floor painting over it.
  //
  // Sorterat N→S av samma skäl som bergen och kullarna: en krona som hänger
  // över hexgränsen överlappar grannen, och VEM som hamnar överst avgjordes
  // tidigare av arrayordningen. `/map` har ingen `ORDER BY`, så den ordningen
  // var godtycklig — samma latenta indeterminism som `worldfixture.py` tvingades
  // sortera bort för att två dumpar av samma värld skulle ge samma pixlar
  // ([[megaron_helvyrigg_20260727]] metodlärdom 3). Culling gör den dessutom
  // AKTIV och inte bara latent: den cullade listan är en delmängd vars inbördes
  // ordning ändras när kameran flyttas, så utan sorteringen kunde två träd byta
  // överlapp mitt under en panorering.
  // Båda skogarna i SAMMA sorterade pass, inte i två. En cederhex och en
  // lundhex som gränsar till varandra har kronor som hänger in över samma
  // gräns, och vem som hamnar överst måste avgöras av läget i N→S-ordningen —
  // inte av vilket pass som råkade köra sist. Två pass hade lagt hela
  // cederskogen ovanpå hela lunden oavsett var träden står.
  const canopyTiles = vis
    .filter(t => t.terrain === 'forest_olive_grove' || t.terrain === 'forest_cedar')
    .map(t => ({ t, p: hexPx(t.q, t.r) }))
    .sort((a, b) => a.p.y - b.p.y);
  for (const { t, p } of canopyTiles) {
    if (t.terrain === 'forest_cedar') drawCedarCanopy(ctx, p.x, p.y, t.q, t.r);
    else drawCanopy(ctx, p.x, p.y, t.q, t.r);
  }
  pass('canopy');

  // 1b2. Peak pass — same reasoning as the canopy: a summit has to be allowed
  // to rise above its own hex's border and occlude the hex behind it, which is
  // the only way a map seen from above can say "tall". Drawn north-to-south
  // over the whole map, not just within a hex, so a peak in the row below
  // correctly overlaps the range behind it.
  const peakTiles = vis
    .filter(t => t.terrain === 'mountain_limestone' || t.terrain === 'mountain_red')
    .map(t => ({ t, p: hexPx(t.q, t.r) }))
    .sort((a, b) => a.p.y - b.p.y);
  for (const { t, p } of peakTiles) drawPeaks(ctx, p.x, p.y, t.q, t.r, t.terrain);
  pass('peaks');

  // 1c. Deposit markers — game information, so above all terrain passes.
  if (State.camera.zoom >= ROAD_DEPOSIT_ZOOM) {
    for (const t of vis) {
      if (t.terrain === 'fog') continue;
      const {x, y} = hexPx(t.q, t.r);
      drawDepositIcons(ctx, x, y, t);
    }
  }
  pass('deposits');

  // 2. Roads between adjacent own/allied provinces
  if (State.camera.zoom >= ROAD_DEPOSIT_ZOOM) {
    const owned = State.provinceData.filter(p => p.own || p.allied);
    for (let i = 0; i < owned.length; i++) {
      for (let j = i+1; j < owned.length; j++) {
        const a = owned[i], b = owned[j];
        const dq = a.q-b.q, dr = a.r-b.r;
        if ((Math.abs(dq) + Math.abs(dq+dr) + Math.abs(dr)) / 2 <= 1) {
          const fa = hexPx(a.q, a.r), fb = hexPx(b.q, b.r);
          drawRoad(ctx, fa.x, fa.y, fb.x, fb.y);
        }
      }
    }
  }
  pass('roads');

  // 2.5 Catchment zone — subtle gold tint on the 7 catchment tiles of own cities
  // (the city's own hex + the 6 adjacent). [0,0] = the settlement's own hex.
  if (State.camera.zoom >= LOCAL_ZOOM) {
    for (const p of State.provinceData) {
      if (!p.own || p.is_outpost) continue;
      for (const [dq, dr] of [[0, 0], ...HEX_DIRS]) {
        const nq = p.q + dq, nr = p.r + dr;
        if (dq !== 0 || dr !== 0) {
          if (State.provinceData.find(x => x.q === nq && x.r === nr)) continue;
        }
        const t = State.tileData.find(t => t.q === nq && t.r === nr);
        if (!t || t.terrain === 'fog') continue;
        const {x, y} = hexPx(nq, nr);
        const pts = hexPts(x, y);
        ctx.save();
        hexPath(ctx, pts);
        ctx.globalAlpha = 0.11;
        ctx.fillStyle = '#D4AC0D';
        ctx.fill();
        ctx.globalAlpha = 0.25;
        ctx.strokeStyle = '#D4AC0D';
        ctx.lineWidth = 0.7;
        ctx.stroke();
        ctx.restore();
      }
    }
  }

  // 2.6 Rural projections — own city buildings drawn on their catchment hexes.
  // LOCAL register only (LOCAL_ZOOM): "why does THIS place work?" is a local
  // question; the strategic overview tones them down. Server places them within
  // the owner's FOW; skip any hex that has gone to fog since the last fetch.
  if (State.camera.zoom >= LOCAL_ZOOM) {
    for (const rp of State.ruralData) {
      const sprite = RURAL_SPRITES[rp.building_type];
      if (!sprite) continue;
      const t = State.tileData.find(t => t.q === rp.q && t.r === rp.r);
      if (!t || t.terrain === 'fog') continue;
      const { x, y } = hexPx(rp.q, rp.r);
      ctx.save();
      sprite(ctx, x, y);
      ctx.restore();
    }
  }
  pass('tint+rural');

  // 3. Highlight selected hex
  if (State.selectedHex) {
    const {x,y} = hexPx(State.selectedHex.q, State.selectedHex.r);
    const pts = hexPts(x, y);
    ctx.save();
    ctx.strokeStyle = '#F9E79F'; ctx.lineWidth = 2.5;
    ctx.beginPath(); ctx.moveTo(pts[0][0], pts[0][1]);
    for (let i=1;i<6;i++) ctx.lineTo(pts[i][0], pts[i][1]);
    ctx.closePath(); ctx.stroke(); ctx.restore();
  }

  // 3.5 FOV preview band — hexes that would become live-visible from the hovered
  // march affordance's target, per server/internal/province/hex.go LiveRadius.
  // Fog tiles use the conservative base radius (no mountain bonus) since we
  // can't know their real terrain without leaking it through the band shape.
  if (State.fovPreview) {
    const { q: fq, r: fr, kind } = State.fovPreview;
    for (const t of State.tileData) {
      const known = t.terrain !== 'fog';
      const radius = known ? liveRadius(kind, t.terrain) : LIVE_RADIUS_BASE[kind];
      if (hexDist(fq, fr, t.q, t.r) > radius) continue;
      const {x, y} = hexPx(t.q, t.r);
      const pts = hexPts(x, y);
      ctx.save();
      hexPath(ctx, pts);
      ctx.globalAlpha = 0.16;
      ctx.fillStyle = '#C87F2A';
      ctx.fill();
      ctx.globalAlpha = 0.45;
      ctx.strokeStyle = '#C87F2A';
      ctx.lineWidth = 1;
      ctx.stroke();
      ctx.restore();
    }
  }

  // 3.6 Catchment preview — the 7 catchment hexes (target + 6 neighbours) of a
  // hovered/armed colonize affordance. This is deliberately NOT the FOV band
  // above: colonize's true footprint is the fixed 7-hex catchment (same shape
  // as 2.5's own-city tint), not the per-tile live-visibility radius, which
  // reads as a much bigger and irregular area than what the colony will
  // actually work (Bugg 3).
  if (State.catchmentPreview) {
    const { q: cq, r: cr } = State.catchmentPreview;
    for (const [dq, dr] of [[0, 0], ...HEX_DIRS]) {
      const nq = cq + dq, nr = cr + dr;
      const t = State.tileData.find(t => t.q === nq && t.r === nr);
      if (!t || t.terrain === 'fog') continue;
      const {x, y} = hexPx(nq, nr);
      const pts = hexPts(x, y);
      ctx.save();
      hexPath(ctx, pts);
      ctx.globalAlpha = 0.22;
      ctx.fillStyle = '#F5B041';
      ctx.fill();
      ctx.globalAlpha = 0.5;
      ctx.strokeStyle = '#F5B041';
      ctx.lineWidth = 1;
      ctx.stroke();
      ctx.restore();
    }
  }

  // 3b. Incoming attack glow — pulsing red on target hex of any visible attack march
  const attackTargets = new Set(
    State.marchData.filter(m => m.intent === 'attack').map(m => `${m.target_q},${m.target_r}`)
  );
  if (attackTargets.size > 0) {
    const pulse = 0.25 + 0.15 * Math.sin(State.animFrame * 0.08);
    for (const p of State.provinceData) {
      if (!attackTargets.has(`${p.q},${p.r}`)) continue;
      const {x, y} = hexPx(p.q, p.r);
      const pts = hexPts(x, y);
      ctx.save();
      hexPath(ctx, pts);
      ctx.globalAlpha = pulse;
      ctx.fillStyle = '#C0392B';
      ctx.fill();
      ctx.globalAlpha = pulse + 0.2;
      ctx.strokeStyle = '#E74C3C';
      ctx.lineWidth = 2;
      ctx.stroke();
      ctx.restore();
    }
  }
  pass('overlays');

  // 4. Province buildings + flags
  // Ritas norr→söder. Stadsmassorna är upp till 44×32 px och svämmar över
  // hexkanten med flit, så två grannstäder kan överlappa; den sydligare ska då
  // ligga överst, precis som träden och aktörerna sorteras.
  const byDepth = State.provinceData.slice()
    .sort((a, b) => hexPx(a.q, a.r).y - hexPx(b.q, b.r).y);
  for (const p of byDepth) {
    const {x,y} = hexPx(p.q, p.r);
    drawProvince(ctx, x, y, p);
    if (State.camera.zoom >= LOCAL_ZOOM) drawLabel(ctx, x, y, p.name, p.own);
  }
  pass('cities');

  // 4b. Activity overlay badges (own non-outpost cities, zoom >= ACTIVITY_BADGE_ZOOM)
  if (State.activityOverlay && State.camera.zoom >= ACTIVITY_BADGE_ZOOM) {
    for (const p of State.provinceData) {
      if (p.is_outpost || !p.own) continue;
      const {x, y} = hexPx(p.q, p.r);
      drawActivityBadge(ctx, x, y, p);
    }
  }

  // 5. Animated walkers for marching armies
  const walkPhase = Math.floor(State.animFrame / 8) % 4;
  for (const m of State.marchData) {
    const now = serverNow();
    const departs = new Date(m.departs_at).getTime();
    const arrives  = new Date(m.arrives_at).getTime();
    const progress = Math.min(1, Math.max(0, (now - departs) / (arrives - departs)));
    const pos = (m.path && m.path.length > 1)
      ? pathPx(m.path, progress)
      : hexPathPx(m.origin_q, m.origin_r, m.target_q, m.target_r, progress);
    if (isTileVisible(pos.q, pos.r)) {
      // Aggregatmarschernas /marches-markör bär bara `is_naval` — sammansätt-
      // ningen finns inte i svaret, så här går det inte att välja aktörsform.
      // Det är legacy-lagret (marching_armies skrivs numera bara av recall- och
      // utpostvägarna); per enhet-lagret nedan har `u.type` och ritas riktigt.
      const kind = m.is_naval ? 'galley' : 'spearman';
      drawActor(ctx, kind, pos.x, pos.y, m.intent, walkPhase);
    }
  }

  // 5b. Per-unit armies & fleets (per-unit march model). Marching units animate
  // along their route (interpolated like marches); positioned units (on the map
  // without a settlement) sit where they stand. Garrison/forming/embarked units
  // live at a settlement and are already represented by its province, so they
  // are not drawn here.
  for (const u of State.unitsData) {
    const naval = u.category === 'naval';
    // `u.type` har alltid funnits i renderarens data (etiketter, host-detektion)
    // men skickades aldrig till spriten — det var därför nio aktörer delade
    // fyra former. Rördragningen är hela skillnaden.
    const kind = canonicalUnitType(u.type) || (naval ? 'galley' : 'spearman');
    if (u.status === 'marching' && u.departs_at && u.arrives_at && u.q != null && u.target_q != null) {
      const now = serverNow();
      const departs = new Date(u.departs_at).getTime();
      const arrives = new Date(u.arrives_at).getTime();
      const progress = Math.min(1, Math.max(0, (now - departs) / (arrives - departs)));
      const pos = (u.path && u.path.length > 1)
        ? pathPx(u.path, progress)
        : hexPathPx(u.q, u.r, u.target_q, u.target_r, progress);
      if (isTileVisible(pos.q, pos.r)) {
        // explore/explore_return share the cyan "explore" sail; other legs use
        // the neutral default colour (intent is resolved server-side on arrival).
        const intent = (u.march_intent === 'explore' || u.march_intent === 'explore_return') ? 'explore' : (u.march_intent || '');
        drawActor(ctx, kind, pos.x, pos.y, intent, walkPhase);
      }
    } else if (u.status === 'positioned' && u.q != null && isTileVisible(u.q, u.r)) {
      const {x, y} = hexPx(u.q, u.r);
      drawActor(ctx, kind, x, y, '', walkPhase);
    }
  }

  // 5c. Foreign (non-owned) units — GET /foreign-units, fow/frammande-enheter
  // 2026-08-03. Same client-side interpolation as 5b for smooth animation
  // between polls, but the gate is isTileLive, NEVER isTileVisible: a foreign
  // unit must never be drawn on a remembered (dimmed) hex — remembered tiles
  // carry no activity by kanonbeslut, and isTileVisible would let one through.
  // Drawn with FOREIGN_ACCENT (neutral/unknown, not a hostile red — MVP has no
  // declared war) plus a blinking 1px FOREIGN_OUTLINE, the redundant carrier of
  // the owner signal (Timothy 2026-08-03: "helt ok och väldigt tydligt").
  //
  // The blink is clocked off State.animFrame, never the wall clock: the frozen-
  // frame rigs pin animFrame, so a wall-clock blink would make every screenshot
  // non-deterministic and every pixel diff meaningless. blinkTick is computed
  // before the redraw early-out above, which also wakes the loop on phase change.
  const foreignOutline = blinkTick % 2 === 0 ? FOREIGN_OUTLINE : null;
  for (const u of State.foreignUnitData) {
    const naval = u.category === 'naval';
    const kind = canonicalUnitType(u.type) || (naval ? 'galley' : 'spearman');
    if (u.status === 'marching' && u.departs_at && u.arrives_at && u.q != null && u.target_q != null) {
      const now = serverNow();
      const departs = new Date(u.departs_at).getTime();
      const arrives = new Date(u.arrives_at).getTime();
      const progress = Math.min(1, Math.max(0, (now - departs) / (arrives - departs)));
      const pos = (u.path && u.path.length > 1)
        ? pathPx(u.path, progress)
        : hexPathPx(u.q, u.r, u.target_q, u.target_r, progress);
      if (isTileLive(pos.q, pos.r)) {
        drawActor(ctx, kind, pos.x, pos.y, '', walkPhase, FOREIGN_ACCENT, foreignOutline);
      }
    } else if (u.status === 'positioned' && u.q != null && isTileLive(u.q, u.r)) {
      const {x, y} = hexPx(u.q, u.r);
      drawActor(ctx, kind, x, y, '', walkPhase, FOREIGN_ACCENT, foreignOutline);
    }
  }

  // 6. Animated messengers — OWN couriers are drawn along their whole route,
  // dimmed over fog (the player's own runner is information they already
  // possess — temenos_orderlopare_plan.md Fas 5); foreign messengers only
  // inside live-visible tiles, as before.
  for (const m of State.messengerData) {
    const now = serverNow();
    const sent   = new Date(m.sent_at).getTime();
    const arrives = new Date(m.arrives_at).getTime();
    const progress = Math.min(1, Math.max(0, (now - sent) / (arrives - sent)));
    const pos = hexPathPx(m.origin_q, m.origin_r, m.dest_q, m.dest_r, progress);
    const visible = isTileVisible(pos.q, pos.r);
    // An order runner at journey's end hasn't "failed to move" — it has arrived
    // and is delivering; the unit starts marching once the worker applies the
    // order (a poll away). Draw a settled handover instead of a jog-in-place.
    const delivering = m.kind === 'order' && progress >= 1;
    if (m.own) {
      ctx.save();
      if (!visible) ctx.globalAlpha = 0.45;
      drawMessenger(ctx, Math.round(pos.x), Math.round(pos.y), walkPhase, m.kind === 'order', true, delivering);
      ctx.restore();
    } else if (visible) {
      drawMessenger(ctx, Math.round(pos.x), Math.round(pos.y), walkPhase, m.kind === 'order', false, delivering);
    }
  }

  // 7. Animated trade caravans
  for (const t of State.tradeData) {
    const now = serverNow();
    const departs = new Date(t.departs_at).getTime();
    const arrives = new Date(t.arrives_at).getTime();
    const progress = Math.min(1, Math.max(0, (now - departs) / (arrives - departs)));
    const pos = hexPathPx(t.origin_q, t.origin_r, t.dest_q, t.dest_r, progress);
    if (isTileVisible(pos.q, pos.r)) {
      drawCaravan(ctx, Math.round(pos.x), Math.round(pos.y), walkPhase);
    }
  }
  pass('actors');

  ctx.restore();
  requestAnimationFrame(render);
}

// ── Data loading ──────────────────────────────────────────────────────────
export async function loadMap() {
  const [tilesRes, provRes, marchRes, msgRes, tradeRes, unitsRes, ruralRes, foreignUnitsRes] = await Promise.all([
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/map`),
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces`),
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/marches`),
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/messengers`),
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/trades`),
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units`),
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/rural-projections`),
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/foreign-units`),
  ]);

  if (tilesRes.ok) {
    State.tileData = await tilesRes.json();
  }
  if (provRes.ok) {
    // Must land before centreCamera() below — homePosition() reads the
    // capital out of State.provinceData.
    State.provinceData = await provRes.json();
  }
  centreCamera();
  if (marchRes.ok) {
    State.marchData = await marchRes.json();
  }
  if (unitsRes.ok) {
    State.unitsData = (await unitsRes.json()).units || [];
  }
  if (msgRes.ok) {
    State.messengerData = await msgRes.json();
  }
  if (tradeRes.ok) {
    State.tradeData = await tradeRes.json();
  }
  if (ruralRes.ok) {
    State.ruralData = await ruralRes.json();
  }
  if (foreignUnitsRes.ok) {
    State.foreignUnitData = await foreignUnitsRes.json();
  }
  window.MusicPlayer.update();
}

// The player's home hex: capital settlement, or — during the founder phase,
// before any settlement exists — the Nomadic Host's position. Camera should
// always open on this, never on the mean of every visible tile (that reads
// as a meaningless empty mid-ocean centre on a large/sparse map).
function homePosition() {
  const capital = ownCapital();
  if (capital) return { q: capital.q, r: capital.r };
  if (State.founderPhase && State.founderPhase.q != null) {
    return { q: State.founderPhase.q, r: State.founderPhase.r };
  }
  return null;
}

function centreCamera() {
  const home = homePosition();
  if (home) {
    const { x, y } = hexPx(home.q, home.r);
    State.camera.x = canvas.width/2  - x*SCALE;
    State.camera.y = canvas.height/2 - y*SCALE;
    clampCamera();
    return;
  }
  // Fallback for a transitional state with neither a capital nor an active
  // founder phase yet (e.g. mid-succession) — centre on visible tiles rather
  // than leaving the camera wherever it last was.
  const visible = State.tileData.filter(t => t.terrain !== 'fog');
  if (!visible.length) return;
  const sumX = visible.reduce((s,t) => s + hexPx(t.q,t.r).x, 0);
  const sumY = visible.reduce((s,t) => s + hexPx(t.q,t.r).y, 0);
  State.camera.x = canvas.width/2  - (sumX/visible.length)*SCALE;
  State.camera.y = canvas.height/2 - (sumY/visible.length)*SCALE;
  clampCamera();
}

// Reload provinces, marches, messengers and trades every 30s
// Refetch the fog-of-war map (State.tileData) WITHOUT recentring the State.camera. Fog
// changes — a scout/explore revealing new tiles, a unit's live vision moving —
// only land here, so this must run on the poll (and after unit arrivals);
// otherwise the canvas keeps the fog it had at page load and exploration looks
// like it did nothing.
export function refreshTiles() {
  fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/map`).then(r => r.ok && r.json().then(d => { State.tileData = d; State.dirty = true; }));
}

// ── Zoom helpers ──────────────────────────────────────────────────────────
// clampZoom + the translate-with-effective-factor arithmetic live in
// camera.js (a pure module, importable outside the browser for tests) —
// see there for why the camera's translation must use the CLAMPED factor,
// not the requested one.
export function zoom(factor) {
  const cx = canvas.width/2, cy = canvas.height/2;
  const next = zoomStep(State.camera, factor, cx, cy);
  State.camera.x = next.x;
  State.camera.y = next.y;
  State.camera.zoom = next.zoom;
  clampCamera();
  State.dirty = true;
}
export function resetView() {
  State.camera.zoom = 1;
  centreCamera();
  State.dirty = true;
}

// ── Inspect panel ─────────────────────────────────────────────────────────
// Canonical keys as of migration 028 (terrain enum) — mountain/forest/coast/sea
// were stale pre-rename leftovers that never matched a real tile.terrain value.
// "coast" has no terrain-enum replacement: migration 050 replaced the coast_beach
// terrain with a `coastal` boolean flag on land tiles, so fish is folded into
// producesText() below via that flag instead of a dict key.
const TERRAIN_GOODS = {
  plains:             'grain, horses, wine',
  river_valley:       'grain ×3 (very fertile)',
  river_delta:        'grain ×4 (richest — exposed coast)',
  hills:              'copper (if deposit), wine, oil',
  scrub_maquis:       'wine (marginal)',
  mountain_limestone: 'stone, tin (if deposit)',
  mountain_red:       'stone, tin (if deposit)',
  forest_olive_grove: 'oil, timber',
  forest_cedar:       'cedar, timber',
  coastal_sea:        'fish',
  deep_sea:           'fish',
  river:              'fish',
  // river_ford (megaron_plan_flodbudget_och_vadstalle.md): same fish rate as
  // river (mig 108 reads it straight out of production_rules, not restated
  // here) — "crossable" is the one fact this tooltip needs to add, since
  // that's the whole reason a ford is a different terrain from river.
  river_ford:         'fish (crossable)',
};

function producesText(tile) {
  const base = TERRAIN_GOODS[tile.terrain] || '—';
  if (!tile.coastal) return base;
  return base === '—' ? 'fish' : base + ', fish';
}

// foreignUnits (optional): non-owned units on this same hex (GET /foreign-units,
// fow/frammande-enheter) — listed below the player's own, tagged with the
// owner's name, and with no "Visa →" button (there is nothing of theirs to
// act on).
function unitListHTML(units, foreignUnits) {
  let html = '';
  if (units.length) {
    const rows = units.map(u => {
      const lbl = actorName(u);
      return '<div style="display:flex;justify-content:space-between;align-items:center;gap:.4rem;padding:.2rem 0">'
        + '<span>' + lbl + ' <span style="color:var(--text-dim)">(' + u.status + ')</span></span>'
        + '<button data-unit-id="' + u.id + '" style="padding:.15rem .35rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.65rem;cursor:pointer">Visa →</button>'
        + '</div>';
    }).join('');
    html += '<div style="margin-bottom:.5rem"><div class="ir-label" style="margin-bottom:.2rem">Units here</div>' + rows + '</div>';
  }
  if (foreignUnits && foreignUnits.length) {
    const rows = foreignUnits.map(u => {
      const lbl = actorName(u);
      const owner = u.owner || '—';
      return '<div style="display:flex;justify-content:space-between;align-items:center;gap:.4rem;padding:.2rem 0">'
        + '<span>' + lbl + ' ×' + u.size + ' <span style="color:var(--text-dim)">(' + owner + ', ' + u.status + ')</span></span>'
        + '</div>';
    }).join('');
    html += '<div style="margin-bottom:.5rem"><div class="ir-label" style="margin-bottom:.2rem">Foreign units here</div>' + rows + '</div>';
  }
  return html;
}

function bindUnitButtons(foot) {
  foot.querySelectorAll('[data-unit-id]').forEach(b => {
    b.addEventListener('click', () => window.warFocusUnit(b.dataset.unitId));
  });
}

const MARCH_BTN_STYLE = 'display:block;width:100%;text-align:center;padding:.3rem;background:var(--bg-raised);border:1px solid var(--border);color:var(--text);font-size:.8rem;cursor:pointer;margin-top:.3rem;';

// Wire a march-affordance button: click opens march-ctx pre-filled with dest,
// hover sets the FOV preview band (map.js render §3.5) for `kind` ('land'|'ship').
function bindMarchButton(btn, dest, kind) {
  if (!btn) return;
  btn.addEventListener('click', e => window.openMarchCtx(dest, e.clientX, e.clientY));
  btn.addEventListener('mouseenter', () => { State.fovPreview = { q: dest.q, r: dest.r, kind }; State.dirty = true; });
  btn.addEventListener('mouseleave', () => { State.fovPreview = null; State.dirty = true; });
}

// Wire the colonize affordance button: same click behaviour as a march button
// (opens march-ctx pre-filled with dest), but hover previews the 7-hex
// catchment (render §3.6) instead of the FOV band — colonize's footprint is
// the fixed catchment, not live-visibility (Bugg 3).
function bindCatchmentPreviewButton(btn, dest) {
  if (!btn) return;
  btn.addEventListener('click', e => window.openMarchCtx(dest, e.clientX, e.clientY));
  btn.addEventListener('mouseenter', () => { State.catchmentPreview = { q: dest.q, r: dest.r }; State.dirty = true; });
  btn.addEventListener('mouseleave', () => { State.catchmentPreview = null; State.dirty = true; });
}

// Human names for the terrain enum — the raw keys leaked into panels and menu
// headers as "River_valley"/"Empty hex", machine-speak in the most prominent slot.
const TERRAIN_LABELS = {
  plains: 'Plains', hills: 'Hills', forest_olive_grove: 'Olive Grove',
  scrub_maquis: 'Maquis Scrub', semi_desert: 'Semi-Desert',
  river_valley: 'River Valley', river_delta: 'River Delta',
  river: 'River', forest_cedar: 'Cedar Forest',
  coastal_sea: 'Coastal Sea', deep_sea: 'Deep Sea',
  mountain_limestone: 'Limestone Mountains', mountain_red: 'Red Mountains',
  river_ford: 'Ford',
};

function terrainLabel(t) {
  return TERRAIN_LABELS[t] || (t.charAt(0).toUpperCase() + t.slice(1).replaceAll('_', ' '));
}

function fillTerrainFields(tile) {
  document.getElementById('ip-terrain').textContent = terrainLabel(tile.terrain);
  const deps = [tile.copper_deposit ? 'Copper' : null, tile.tin_deposit ? 'Tin' : null,
                tile.silver_deposit ? 'Silver' : null, tile.cedar_deposit ? 'Cedar' : null].filter(Boolean);
  const depRow = document.getElementById('ip-deposits-row');
  if (deps.length > 0) {
    document.getElementById('ip-deposits').textContent = deps.join(' · ');
    depRow.style.display = '';
  } else {
    depRow.style.display = 'none';
  }
  document.getElementById('ip-produces').textContent = producesText(tile);
}

function setCityFieldsVisible(visible) {
  ['ip-culture-row', 'ip-owner-row', 'ip-walls-row', 'ip-army-row'].forEach(id => {
    document.getElementById(id).style.display = visible ? '' : 'none';
  });
}

// Build the same dest object the march-ctx menu consumes, whether the caller is
// the left-click affordance panel or the right-click context menu. target is the
// province marker at (h.q,h.r), or null for an empty/sea/mountain hex.
function destFromHex(h, tile, target) {
  const isSea = tile.terrain === 'coastal_sea' || tile.terrain === 'deep_sea';
  if (target) {
    return { q: h.q, r: h.r, terrain: tile.terrain, isSea,
             name: target.name, isSettlement: true, allied: target.own ? true : !!target.allied };
  }
  return { q: h.q, r: h.r, terrain: tile.terrain, isSea,
           name: `${terrainLabel(tile.terrain)} (${h.q},${h.r})`,
           isSettlement: false, allied: false };
}

// Fog: nothing known yet — no terrain/deposits/produces, no affordances.
function openFogPanel(h) {
  document.getElementById('ip-name').textContent = 'Outforskat land';
  setCityFieldsVisible(false);
  document.getElementById('ip-terrain').textContent = `(${h.q},${h.r})`;
  document.getElementById('ip-deposits-row').style.display = 'none';
  document.getElementById('ip-produces').textContent = '—';
  document.getElementById('ip-foot').innerHTML = '<p class="empty-state">Segla eller marschera i närheten för att avslöja.</p>';
  document.getElementById('inspect-panel').style.display = 'flex';
}

// Foreign/allied settlement — today's openInspect content (Wanax/culture/walls/DP),
// plus the units-here list and a Marschera button. Own settlements never reach
// this function — they bypass the panel for the city drawer (see openHexPanel).
function openCityPanel(h, tile, marker, units, foreignUnits) {
  document.getElementById('ip-name').textContent    = marker.name;
  setCityFieldsVisible(true);
  document.getElementById('ip-culture').textContent = marker.culture;
  fillTerrainFields(tile);

  let ownerText = marker.owner || '(unoccupied)';
  if (marker.allied) ownerText += ' (allied)';
  document.getElementById('ip-owner').textContent = ownerText;
  document.getElementById('ip-walls').textContent = '▓'.repeat(marker.walls) + '░'.repeat(Math.max(0,3-marker.walls));
  document.getElementById('ip-army').textContent = '…';
  fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${marker.id}/army`).then(r => {
    if (!r.ok) { document.getElementById('ip-army').textContent = '—'; return; }
    r.json().then(a => {
      const dp = (a.Spearman||0)*1 + (a.EliteInfantry||0)*3 + (a.WarChariot||0)*4;
      document.getElementById('ip-army').textContent = dp > 0 ? `${dp} DP` : '—';
    });
  }).catch(() => { document.getElementById('ip-army').textContent = '—'; });

  const foot = document.getElementById('ip-foot');
  let footHtml = unitListHTML(units, foreignUnits);
  // A settlement OR a wandering host can send — the host's first contact with a
  // met city is one of its designed uses (mig 087; sendMessengerFromInspect
  // picks the endpoint).
  if ((State.MY_SETTLEMENT_ID || State.founderPhase) && marker.settlement_id) {
    footHtml += `
      <textarea id="ip-msg-text" class="msg-textarea" placeholder="Write message…" maxlength="1000" rows="3"></textarea>
      <div class="msg-foot">
        <button class="msg-send" onclick="sendMessengerFromInspect('${marker.settlement_id}')">Send Messenger</button>
        <span class="msg-err" id="ip-msg-err"></span>
      </div>`;
  }
  footHtml += '<button id="ip-march-btn" style="' + MARCH_BTN_STYLE + '">Marschera hit →</button>';
  foot.innerHTML = footHtml;
  bindUnitButtons(foot);
  bindMarchButton(document.getElementById('ip-march-btn'), destFromHex(h, tile, marker), 'land');

  document.getElementById('inspect-panel').style.display = 'flex';
}

// Mountain / sea / empty land — no province here. Mountains explain their own
// absence of affordances; sea gets galleys; empty land gets march + colonize.
function openTerrainPanel(h, tile, isMountain, isSea, units, foreignUnits) {
  document.getElementById('ip-name').textContent =
    isSea ? `Sea (${h.q},${h.r})` : (isMountain ? `Mountains (${h.q},${h.r})` : `Empty hex (${h.q},${h.r})`);
  setCityFieldsVisible(false);
  fillTerrainFields(tile);

  const foot = document.getElementById('ip-foot');
  let footHtml = unitListHTML(units, foreignUnits);

  if (isMountain) {
    footHtml += '<p class="empty-state">Ogenomträngligt — arméer kan inte gå här.</p>';
    foot.innerHTML = footHtml;
    bindUnitButtons(foot);
    document.getElementById('inspect-panel').style.display = 'flex';
    return;
  }

  const dest = destFromHex(h, tile, null);
  if (isSea) {
    footHtml += '<button id="ip-march-btn" style="' + MARCH_BTN_STYLE + '">Skicka galärer →</button>';
  } else if (State.founderPhase) {
    // Founder phase: empty land is a POSSIBLE HOME, never a colony ("aldrig
    // Kolonisera"). March the host here; the founding forecast shows what the
    // ground would feed. Settle itself lives on the Host panel — the server
    // founds where the host stands, nowhere else.
    // The forecast belongs in the scrolling body, not beside the action in the
    // foot — same reason as the Host panel: it is unbounded and it was pushing
    // the foot past the panel's bottom edge.
    footHtml += '<button id="ip-march-btn" style="' + MARCH_BTN_STYLE + '">Marschera hit →</button>';
    document.getElementById('ip-body-extra').innerHTML =
      '<div id="ip-found-preview" style="font-size:.73rem;border-top:1px solid var(--border);padding-top:.4rem">Hämtar grundningsprognos…</div>';
  } else {
    footHtml += '<button id="ip-march-btn" style="' + MARCH_BTN_STYLE + '">Marschera hit →</button>'
             +  '<button id="ip-colonize-btn" style="' + MARCH_BTN_STYLE + '">Kolonisera →</button>';
  }
  foot.innerHTML = footHtml;
  bindUnitButtons(foot);
  // Founder phase: marching the host to an empty hex FOUNDS there — so hovering
  // the march button previews the 7-hex catchment (bug 3 shape), not the FOV
  // band. Every other case keeps the plain march affordance.
  if (!isSea && State.founderPhase) {
    bindCatchmentPreviewButton(document.getElementById('ip-march-btn'), dest);
  } else {
    bindMarchButton(document.getElementById('ip-march-btn'), dest, isSea ? 'ship' : 'land');
  }

  if (!isSea && State.founderPhase) {
    const fp = State.founderPhase;
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/colonize-preview?q=${h.q}&r=${h.r}`
        + `&pop=${fp.population}&seed=${Math.max(0, Math.round(fp.grain?.amount || 0))}`)
      .then(r => r.ok ? r.json() : null)
      .then(p => {
        const el = document.getElementById('ip-found-preview');
        if (el && p) el.innerHTML = window.renderColonizePreviewHTML(p);
      })
      .catch(() => {});
  }

  const colBtn = document.getElementById('ip-colonize-btn');
  if (colBtn) {
    bindCatchmentPreviewButton(colBtn, dest);
    // Same march-ctx as Marschera, just pre-check the colonize box on open —
    // no second order code path (plan §"Målbild").
    colBtn.addEventListener('click', () => {
      const chk = document.getElementById('mctx-colonize-chk');
      if (chk) { chk.checked = true; window.onColonizeToggle(); }
    });
  }

  document.getElementById('inspect-panel').style.display = 'flex';
}

// Rural projection object card (Fas A2). Identity (what + whose omland) + the
// terrain it sits on, and a single primary CTA back to the owning city — the
// projection is a representation, so its card leads to the real building
// (megaron_lokal_varld.md). Any unit standing on the hex stays reachable via
// the units list; marching here is a right-click order, not a panel button.
const RURAL_LABELS = { farm: 'Farm', mine: 'Mine', lumbermill: 'Lumbermill' };

function openRuralPanel(h, tile, rural, units, foreignUnits) {
  document.getElementById('ip-name').textContent = RURAL_LABELS[rural.building_type] || rural.building_type;
  setCityFieldsVisible(false);
  fillTerrainFields(tile);

  const foot = document.getElementById('ip-foot');
  let footHtml = `<p class="empty-state">Del av ${rural.name}s omland — brukas härifrån.</p>`;
  footHtml += unitListHTML(units, foreignUnits);
  footHtml += `<button id="ip-rural-city-btn" style="${MARCH_BTN_STYLE}">Öppna ${rural.name} →</button>`;
  foot.innerHTML = footHtml;
  bindUnitButtons(foot);
  document.getElementById('ip-rural-city-btn').addEventListener('click', () => {
    document.getElementById('inspect-panel').style.display = 'none';
    State.cityViewID = rural.province_id;
    window.openDrawer('city');
  });

  document.getElementById('inspect-panel').style.display = 'flex';
}

// ── Founder phase: the Host panel (temenos_nomadic_host_fas4_plan.md 4.3) ──
// The people-on-the-move's own surface: status from /founding/status, the
// founding forecast from /colonize-preview with ?pop=&seed= (the metropolis's
// 4 000 and the carried grain — same endpoint and renderer as colonization,
// never its own), and the irreversible Settle. Disappears entirely the moment
// founder_phase.active goes false.

// One store line: "X tick kvar (≈ Y verklig tid)" — both derived from
// ticks_left at render time (B2: never a stored wall clock).
// ticks_left IS the tick count (tick == day, mig 109) — no ÷24 here; that
// used to convert an hourly tick count into game-days and is the same class
// of stale scaling as cmd_goods.go's Rate/d bug (mirrors keryx's already-
// correct foundingStoreLine, cmd_founding.go).
function hostStoreLine(label, s, tickSeconds) {
  if (!s || s.ticks_left == null) return `${label}: räcker tills vidare`;
  const ticksLeft = s.ticks_left;
  const realH = Math.round(s.ticks_left * tickSeconds / 3600);
  const real = realH >= 48 ? `≈ ${Math.round(realH / 24)} dygn` : `≈ ${realH} h`;
  return `${label}: ${ticksLeft} tick kvar (${real} verklig tid)`;
}

async function openHostPanel(h, tile) {
  // Refresh the store numbers on every open — they drain per tick.
  try {
    const r = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/founding/status`);
    if (r.ok) {
      const fp = await r.json();
      State.founderPhase = fp.active ? fp : null;
    }
  } catch (_) {}
  const fp = State.founderPhase;
  if (!fp) { openHexPanel(h); return; } // settled meanwhile — normal routing

  document.getElementById('ip-name').textContent = 'Nomadic Host';
  setCityFieldsVisible(false);
  fillTerrainFields(tile);

  // Stores and forecast go in the SCROLLING body; only the action stays in the
  // foot. Both used to live in the foot, which has no scroll of its own — so a
  // long forecast (the 7-hex list plus the gifts plus the goods rows) pushed the
  // founding button below the panel's bottom edge, where it was clipped and
  // unreachable. Measured before this change at 1280×720: the button sat 71 px
  // past the panel. The forecast is unbounded by nature; the button must not
  // share a container with it.
  document.getElementById('ip-body-extra').innerHTML =
    `<div style="margin-bottom:.5rem;line-height:1.5">
       <div>${(fp.population || 0).toLocaleString('sv-SE')} folk · Kan inte strida · Syn: 1 hex</div>
       <div>${hostStoreLine('Grain', fp.grain, fp.tick_seconds)}</div>
       <div>${hostStoreLine('Silver för Spearmen', fp.silver, fp.tick_seconds)}</div>
       <div>${fp.spearmen_in_field || 0} Spearmen-kohort${fp.spearmen_in_field === 1 ? '' : 'er'} i fält</div>
       <div>Budbärare: fria att sända</div>
     </div>
     <div id="ip-found-preview" style="font-size:.73rem;border-top:1px solid var(--border);padding-top:.4rem">Hämtar grundningsprognos…</div>`;

  const foot = document.getElementById('ip-foot');
  foot.innerHTML =
    `<button id="ip-settle-btn" style="${MARCH_BTN_STYLE}">⚒ Grunda huvudstaden här</button>
     <span class="msg-err" id="ip-settle-err"></span>`;
  document.getElementById('inspect-panel').style.display = 'flex';

  // Glow the 7 catchment hexes the host would found on, for as long as the Host
  // panel is open — the map then shows exactly the ground the forecast below
  // describes, so "granska catchmenten före grundning" (design 2026-07-15) is
  // visible, not just tabular. Cleared by openHexPanel on the next hex click.
  State.catchmentPreview = { q: h.q, r: h.r };
  State.dirty = true;

  // The forecast for the hex the host STANDS on — settle founds here, nowhere else.
  // starter_farm=1: a metropolis founding seeds a starter farm (createMetropolis),
  // unlike a plain colony — see founding-forecast-fix-plan.
  fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/colonize-preview?q=${h.q}&r=${h.r}`
      + `&pop=${fp.population}&seed=${Math.max(0, Math.round(fp.grain?.amount || 0))}&starter_farm=1`)
    .then(r => r.ok ? r.json() : null)
    .then(p => {
      const el = document.getElementById('ip-found-preview');
      if (el && p) el.innerHTML = window.renderColonizePreviewHTML(p);
    })
    .catch(() => {});

  document.getElementById('ip-settle-btn').addEventListener('click', async () => {
    if (!confirm('Grunda din huvudstad här? Hostet upplöses — för alltid.')) return;
    const errEl = document.getElementById('ip-settle-err');
    const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/founding/settle`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({}),
    });
    if (res.ok) {
      track('settle');
      // The world changed shape: new province, new eyes, host gone. Reload
      // outright — founding happens once per world, a full refresh is honest.
      location.reload();
    } else {
      const err = await res.json().catch(() => ({ error: 'Unknown error' }));
      errEl.style.color = 'var(--accent)';
      errEl.textContent = err.error || 'Error';
    }
  });
}

// Dispatcher for a left-click (non-drag) on any hex — the affordance matrix.
// Always selects the hex (highlight), then routes to the right view.
function openHexPanel(h) {
  State.selectedHex = { q: h.q, r: h.r };
  State.fovPreview = null;
  State.catchmentPreview = null;
  State.dirty = true;
  // Every panel routes through here, so this is the one place the variable body
  // region gets reset. Without it the Host panel's stores would still be showing
  // after the next click on a sea hex.
  document.getElementById('ip-body-extra').innerHTML = '';

  const tile = State.tileData.find(t => t.q === h.q && t.r === h.r);
  const prov = State.provinceData.find(p => p.q === h.q && p.r === h.r);

  if (!tile || tile.terrain === 'fog') { openFogPanel(h); return; }

  // Founder phase: the host's own hex opens the Host panel, never a unit view.
  if (State.founderPhase && (State.unitsData || []).some(u =>
      u.type === 'nomadic_host' && u.q === h.q && u.r === h.r)) {
    openHostPanel(h, tile);
    return;
  }

  if (prov && prov.own) {
    // Own settlement — no mid-panel, the city drawer IS the info (framgångskriterium 2).
    document.getElementById('inspect-panel').style.display = 'none';
    State.cityViewID = prov.id;
    window.openDrawer('city');
    return;
  }

  const units = (State.unitsData || []).filter(u =>
    u.q === h.q && u.r === h.r && (u.status === 'positioned' || u.status === 'marching'));
  // Foreign (non-owned) units on this same hex (GET /foreign-units,
  // fow/frammande-enheter) — listed under the player's own, owner-tagged,
  // no action button (see unitListHTML).
  const foreignUnits = (State.foreignUnitData || []).filter(u =>
    u.q === h.q && u.r === h.r && (u.status === 'positioned' || u.status === 'marching'));

  if (prov) { openCityPanel(h, tile, prov, units, foreignUnits); return; }

  // Rural projection hex — a catchment hex carrying one of the player's own city
  // buildings (Fas A2). The card names it, says whose omland it is, and its
  // primary CTA leads back to the owning city's building context (the doc's
  // rule: the projection is a representation, its card leads to the real
  // building). Marching still works via right-click, so no march button here.
  const rural = (State.ruralData || []).find(rp => rp.q === h.q && rp.r === h.r);
  if (rural) { openRuralPanel(h, tile, rural, units, foreignUnits); return; }

  const isMountain = tile.terrain === 'mountain_limestone' || tile.terrain === 'mountain_red';
  const isSea = tile.terrain === 'coastal_sea' || tile.terrain === 'deep_sea';
  openTerrainPanel(h, tile, isMountain, isSea, units, foreignUnits);
}

export function closeInspect() {
  State.selectedHex = null;
  State.fovPreview = null;
  document.getElementById('inspect-panel').style.display = 'none';
  State.dirty = true;
}

export async function sendMessengerFromInspect(destSettlementID) {
  const textEl = document.getElementById('ip-msg-text');
  const errEl  = document.getElementById('ip-msg-err');
  const text = textEl ? textEl.value.trim() : '';
  if (!text) { if (errEl) { errEl.style.color='var(--accent)'; errEl.textContent='Write a message first.'; } return; }
  const token = localStorage.getItem('poleia_token');
  // Founder phase: no settlement to send from — the host itself is the origin.
  const sendPath = State.MY_SETTLEMENT_ID
    ? `/api/v1/worlds/${State.WORLD_ID}/settlements/${State.MY_SETTLEMENT_ID}/messengers`
    : `/api/v1/worlds/${State.WORLD_ID}/founding/messengers`;
  const res = await fetch(sendPath, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + token },
    body: JSON.stringify({ destination_id: destSettlementID, message: text }),
  });
  if (res.ok) {
    textEl.value = '';
    errEl.style.color = 'var(--safe)';
    errEl.textContent = 'Messenger sent.';
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/messengers`).then(r => r.ok && r.json().then(d => { State.messengerData = d; State.dirty = true; }));
  } else {
    const err = await res.json().catch(() => ({error: 'Unknown error'}));
    errEl.style.color = 'var(--accent)';
    errEl.textContent = err.error || 'Error';
  }
}

// ── initMap — everything that used to run immediately at the bottom of the
// old single <script> and needs State.WORLD_ID (or otherwise must not fire
// before bootstrap() has populated State). Called once by main.js right
// after bootstrap resolves. Input-handler *registration* below has no
// bootstrap dependency of its own (the handlers only read State when they
// fire, well after bootstrap is done) but is grouped here anyway so this
// module's entire "starts doing things" surface lives in one place — see
// the FAS 2 execution report for the full init()-function inventory. ──────
export function initMap() {
  // ── Input: keyboard pan (WASD / arrows) ─────────────────────────────────
  // Listens on document, not canvas, so panning works without clicking the
  // map first — matching Escape/f//'s existing document-level listeners.
  document.addEventListener('keydown', e => {
    // Ctrl/Alt/Meta held: leave browser/OS shortcuts (e.g. Ctrl+F) alone.
    if (e.ctrlKey || e.altKey || e.metaKey) return;
    if (isTypingTarget(document.activeElement)) return;
    const k = e.key.toLowerCase();
    if (!PAN_KEYS.has(k)) return;
    e.preventDefault(); // stop arrow keys from scrolling the page
    heldKeys.add(k);
  });
  document.addEventListener('keyup', e => {
    heldKeys.delete(e.key.toLowerCase());
  });
  // No keyup fires if the window loses focus mid-hold (e.g. alt-tab) — without
  // this the map would pan forever in whatever direction was held.
  window.addEventListener('blur', () => heldKeys.clear());
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'hidden') heldKeys.clear();
  });

  // ── Input: drag + zoom + click ──────────────────────────────────────────
  canvas.addEventListener('mousedown', e => {
    State.dragging = true;
    State.lastMouse = {x: e.clientX, y: e.clientY};
  });
  canvas.addEventListener('mouseup', e => {
    if (!State.dragging) return;
    const dx = e.clientX - State.lastMouse.x, dy = e.clientY - State.lastMouse.y;
    State.dragging = false;
    if (Math.abs(dx) < 4 && Math.abs(dy) < 4) {
      const rect = canvas.getBoundingClientRect();
      const h = hexAtScreen(e.clientX - rect.left, e.clientY - rect.top);
      openHexPanel(h);
    }
  });
  canvas.addEventListener('mouseleave', () => { State.dragging = false; tooltip.style.display = 'none'; });
  canvas.addEventListener('mousemove', e => {
    if (State.dragging && State.lastMouse) {
      State.camera.x += e.clientX - State.lastMouse.x;
      State.camera.y += e.clientY - State.lastMouse.y;
      clampCamera();
      State.lastMouse = {x: e.clientX, y: e.clientY};
      State.dirty = true;
    }
    const rect = canvas.getBoundingClientRect();
    const h = hexAtScreen(e.clientX - rect.left, e.clientY - rect.top);
    const tile = State.tileData.find(t => t.q === h.q && t.r === h.r);
    const prov = State.provinceData.find(p => p.q === h.q && p.r === h.r);
    if (tile && tile.terrain !== 'fog') {
      tooltip.style.display = 'block';
      tooltip.style.left = (e.clientX + 14) + 'px';
      tooltip.style.top  = (e.clientY - 22) + 'px';
      const deposits = [tile.copper_deposit ? '⚒ Copper' : null, tile.tin_deposit ? '⚒ Tin' : null,
                        tile.silver_deposit ? '⚒ Silver' : null, tile.cedar_deposit ? '⚒ Cedar' : null].filter(Boolean).join(' · ');
      const tl = tile.terrain.charAt(0).toUpperCase() + tile.terrain.slice(1);
      // display_name är serverformaterat ("2nd Spearmen of Knossos") — webben
      // bygger aldrig om den grammatiken (se unit.go:DisplayName). Två fall som
      // en naiv u.q-jämförelse får fel: garnisonerade förband saknar helt
      // hexposition (matchas på provinsens settlement_id), och en MARSCHERANDE
      // enhets u.q/u.r är avgångshexen — gångaren ritas vid pathPx/hexPathPx
      // interpolerade waypoint, så tooltipen måste läsa samma position som
      // spriten. Annars pekar namnet på en tom hex enheten lämnat.
      const names = (State.unitsData || []).filter(u => {
        const at = unitHexNow(u);
        if (at) return at.q === h.q && at.r === h.r;
        return prov && u.settlement_id && u.settlement_id === prov.settlement_id;
      }).map(u => u.display_name).filter(Boolean);
      if (prov) {
        const parts = [prov.name, tl];
        if (prov.owner) parts.push(`Wanax: ${prov.owner}`);
        if (prov.walls > 0) parts.push(`Walls L${prov.walls}`);
        if (prov.culture) parts.push(prov.culture);
        if (prov.own) parts.push('(you)');
        else if (prov.allied) parts.push('(ally)');
        if (deposits) parts.push(deposits);
        if (names.length) parts.push(names.join(', '));
        tooltip.textContent = parts.join(' · ');
      } else {
        const parts = [`(${h.q},${h.r}) ${tl}`];
        if (deposits) parts.push(deposits);
        if (names.length) parts.push(names.join(', '));
        tooltip.textContent = parts.join(' · ');
      }
    } else {
      tooltip.style.display = 'none';
    }
  });
  canvas.addEventListener('wheel', e => {
    e.preventDefault();
    const rect = canvas.getBoundingClientRect();
    const mx = e.clientX - rect.left, my = e.clientY - rect.top;
    const factor = e.deltaY < 0 ? 1.1 : 0.91;
    const next = zoomStep(State.camera, factor, mx, my);
    State.camera.x = next.x;
    State.camera.y = next.y;
    State.camera.zoom = next.zoom;
    clampCamera();
    State.dirty = true;
  }, {passive:false});

  window.addEventListener('resize', () => { State.dirty = true; });

  // Hide map tooltip when pointer enters any drawer
  document.querySelectorAll('.drawer').forEach(d => {
    d.addEventListener('mouseenter', () => { tooltip.style.display = 'none'; });
  });

  loadMap().then(() => { State.dirty = true; render(); });

  // Right-click to open march menu
  canvas.addEventListener('contextmenu', e => {
    e.preventDefault();
    const capital = ownCapital();
    if (!capital) return;
    const rect = canvas.getBoundingClientRect();
    const h = hexAtScreen(e.clientX - rect.left, e.clientY - rect.top);
    const target = State.provinceData.find(p => p.q === h.q && p.r === h.r);
    const tile = State.tileData.find(t => t.q === h.q && t.r === h.r);
    if (!tile || tile.terrain === 'fog') { window.closeMarchCtx(); return; }
    const isMountain = tile.terrain === 'mountain_limestone' || tile.terrain === 'mountain_red';
    if (target) {
      // Own settlement (capital included): march units home to reinforce the
      // garrison. Another Wanax's settlement: march to attack (or reinforce if
      // allied). Inspect lives on left-click — right-click is always orders.
      window.openMarchCtx(destFromHex(h, tile, target), e.clientX, e.clientY);
      return;
    }
    // Empty hex. Mountains are impassable; sea hexes take ships.
    if (isMountain) { window.closeMarchCtx(); return; }
    // Moment-22: an own field unit standing right here IS the order target,
    // not the hex — marching-to-your-own-hex is rejected server-side ("cannot
    // march to own hex"). Route to the same map→drawer bridge the left-click
    // "Visa →" button uses (war.js warFocusUnit): the War drawer opens with
    // that unit's card focused, its March button ready. This detects "is a
    // friendly unit standing here" (u.deployable, not u.size — the server has
    // no size gate); it deliberately does NOT also exclude fortify stance like
    // war.js canMarch does — a fortified unit still occupies the hex, and
    // routing to its card (Stance selector to un-fortify) beats falling through
    // to a march-to-own-hex 422.
    const ownUnit = (State.unitsData || []).find(u =>
      u.q === h.q && u.r === h.r && (u.status === 'garrison' || u.status === 'positioned') &&
      u.type !== 'priest' && u.deployable);
    if (ownUnit) { window.closeMarchCtx(); window.warFocusUnit(ownUnit.id); return; }
    window.openMarchCtx(destFromHex(h, tile, null), e.clientX, e.clientY);
  });

  // Reload provinces, marches, messengers and trades every 30s
  setInterval(() => {
    refreshTiles();
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces`).then(r => r.ok && r.json().then(d => { State.provinceData = d; State.dirty = true; window.MusicPlayer.update(); }));
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/marches`).then(r => r.ok && r.json().then(d => { State.marchData = d; State.dirty = true; window.MusicPlayer.update(); }));
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/messengers`).then(r => r.ok && r.json().then(d => { State.messengerData = d; State.dirty = true; }));
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/trades`).then(r => r.ok && r.json().then(d => { State.tradeData = d; State.dirty = true; }));
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units`).then(r => r.ok && r.json().then(d => { State.unitsData = d.units || []; State.dirty = true; }));
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/rural-projections`).then(r => r.ok && r.json().then(d => { State.ruralData = d; State.dirty = true; }));
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/foreign-units`).then(r => r.ok && r.json().then(d => { State.foreignUnitData = d; State.dirty = true; }));
  }, 30000);

  // While any own unit is marching, refresh units + fog fast so the fog visibly
  // sweeps around the moving unit during the trip (the ship's on-screen position
  // already interpolates every frame; this is only needed so server-computed fog
  // keeps up). Idle — and cheap — whenever nothing is moving.
  setInterval(() => {
    const courierOut = State.messengerData.some(m => m.own);
    // A foreign march in live vision needs the same fast cadence as our own.
    // The guard used to ask only "is anything of MINE moving?", so a Wanax
    // standing still sampled an approaching army every 30s and saw it jump
    // across the map. That is the surface the incoming-march warning is meant
    // to make watchable — an alert you cannot then follow is half a warning.
    const foreignMoving = (State.foreignUnitData || []).some(u => u.status === 'marching');
    if (!State.unitsData.some(u => u.status === 'marching') && !courierOut && !foreignMoving) return;
    refreshTiles();
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units`).then(r => r.ok && r.json().then(d => { State.unitsData = d.units || []; State.dirty = true; }));
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/foreign-units`).then(r => r.ok && r.json().then(d => { State.foreignUnitData = d; State.dirty = true; }));
    // A Runner en route needs the same fast cadence: its delivery flips
    // the unit to marching (or applies a stance) server-side — poll messengers
    // so the runner vanishes and the unit moves without waiting for the 30s tick.
    if (courierOut) {
      fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/messengers`).then(r => r.ok && r.json().then(d => { State.messengerData = d; State.dirty = true; }));
    }
  }, 3000);
}
