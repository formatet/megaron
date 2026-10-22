import { physicalHostState, hostStatusText } from '../ui/host_state.js';
import { State, ownCapital, activeCitySettlement } from '../state.js';
import { fetchAuth } from '../api.js';
import { serverNow } from '../clock.js';
import { anchorWorldTime, movementTickNow } from '../world_time.js';
import { routePosition, routeProgress, actorRoute, foreignArrow, drawMovement } from './routes.js';
import {
  LOCAL_ZOOM,
  GARRISON_DOT_ZOOM, ACTIVITY_BADGE_ZOOM, ROAD_DEPOSIT_ZOOM,
  PAN_SPEED_PX_PER_SEC,
} from '../config.js';
import { isTypingTarget, esc } from '../ui/format.js';
import { fmtNum, fmtDays } from '../ui/fmt_num.js';
import { canonicalUnitType, actorName } from '../ui/actornames.js';
import { drawActor, spriteRuns, FOREIGN_ACCENT, FOREIGN_OUTLINE } from './actorsprites.js';
import { eyeSees } from './sight.js';
import { drawRemembered } from './memory.js';
import { drawFire, FIRE_FRAMES } from './doomfire.js';
import { drawCityMass, citySprite, cityTop, cityFoot } from './citysprites.js';
import { cityIcon, drawCityIcon, drawStamp, ACTIVITY_ICONS, CITY_ICON_PALETTE } from './cityicons.js';
import { zoomStep, clampPan } from './camera.js';
import { bindMapInput } from './map_input.js';
import { confirmInline, showInlineResult } from '../ui/inline_result.js';
import { unitHoverLines } from '../ui/hover.js';
import { drawStorm, STORM_FRAMES } from './storms.js';
import { roadSegments, plannedSegments, drawRoads } from './roads.js';
import { stormPanelHTML } from '../ui/stormname.js';
import { placePanelHTML } from '../ui/placename.js';
import { incomingTargetKeys } from '../ui/movements.js';
import { namedPlaceTerrain, placeLabelLayout, PLACE_LABEL_STYLE, drawPlaceLabel } from './place_names.js';
import { createMarkRenderer } from './markraster.js';
import { createChunkWorker } from './chunk_worker.js';
const ground = createMarkRenderer(State, () => document.createElement('canvas'),
  !(typeof location !== 'undefined' && /[?&]markraster=0/.test(location.search)));
const {
  S, HEX_DIRS, CULT_FAR, HILL_RIM, HILL_HIGH, HILL_LIT, HILL_LOW, HILL_HOLLOW, hexPx, SCALE,
  hexPath, SQRT3, DIR_NX, DIR_NY, openEdges, rndInt, MARKRASTER, rnd, neighborDirs, hexPts,
  terrainAt, TERRAIN_BASE, R_IN, noiseAt, hash32, SHORE_WET, SHORE_SAND, SURF_FOAM,
  markTerrainAt, CHUNK, markCode, cultivationList, bakeChunk, tileKey, WARP_PAD,
  hexAtWorld, markHexAt, isSeaTerrain, ridgeAt, MTN_ROCK, drawDetail, isWaterTerrain,
  riverCurves, riverInkAt,
} = ground;
export { markHexAt, hexAtWorld, hexPx, SCALE, MARKRASTER, riverCurves, riverInkAt };


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

// The economic catchment: the settlement's own hex plus every hex within
// CATCHMENT_RADIUS. Mirrors server/internal/hexgrid.CatchmentRadius (=2):
// 19 hexes (1 + 6 + 12), 18 worked. P1 (2026-08-07) raised the radius 1→2 but
// only the DATA moved — the map's catchment highlight stayed on the old 7-hex
// (radius-1) shape, showing a player 7 of their 19 worked hexes, until this was
// fixed 2026-08-30. Full axial disc (|dq|+|dr|+|dq+dr| ≤ 2·radius), same
// convention as HEX_DIRS; radius 1 reproduces [[0,0], ...HEX_DIRS] exactly.
export const CATCHMENT_RADIUS = 2;
export const CATCHMENT_OFFSETS = (() => {
  const out = [];
  for (let dq = -CATCHMENT_RADIUS; dq <= CATCHMENT_RADIUS; dq++) {
    const lo = Math.max(-CATCHMENT_RADIUS, -dq - CATCHMENT_RADIUS);
    const hi = Math.min(CATCHMENT_RADIUS, -dq + CATCHMENT_RADIUS);
    for (let dr = lo; dr <= hi; dr++) out.push([dq, dr]);
  }
  return out;
})();

// Hex distance in axial coords (cube formula).
function hexDist(q1, r1, q2, r2) {
  return (Math.abs(q1-q2) + Math.abs(q1+r1-q2-r2) + Math.abs(r1-r2)) / 2;
}

// (q,r) → terrain over State.tileData for the FOV preview's sightline, memoised
// on the tileData array's identity (same pattern as panBounds above).
let terrainLookupSrc = null, terrainLookup = null;
function tileTerrainLookup() {
  if (State.tileData !== terrainLookupSrc) {
    terrainLookupSrc = State.tileData;
    const m = new Map();
    for (const t of State.tileData || []) m.set(`${t.q},${t.r}`, t.terrain);
    terrainLookup = (q, r) => m.get(`${q},${r}`);
  }
  return terrainLookup;
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
function pathPx(waypoints, progress, costs=null,durationTicks=null) {
  return routePosition(waypoints,progress,hexPx,costs,durationTicks);
}

// Hexen en enhet står på JUST NU — samma gren som ritningen av aktören använder
// (marching → interpolerad waypoint, positioned → u.q/u.r). Returnerar null för
// en enhet utan hexposition alls (garnison), som därför måste matchas på sitt
// settlement i stället. Finns för att tooltipen ska peka på samma hex som
// spriten; delad kod är hela poängen, en andra kopia av interpolationen skulle
// glida isär från renderaren.
function unitHexNow(u) {
 const pos=unitPixelNow(u);
 if (u.type==='nomadic_host' && State.unitsData.some(own=>own.id===u.id)) return pos ? {q:pos.q,r:pos.r} : null;
 return pos&&pos.q!=null?{q:pos.q,r:pos.r}:u.q!=null?{q:u.q,r:u.r}:null;
}

// legHexNow: the hex a caravan or runner walker is drawn on right now —
// the same hexPathPx interpolation render() steps 6–7 use.
function legHexNow(oq, or, dq, dr, startIso, endIso) {
  const start = new Date(startIso).getTime();
  const end = new Date(endIso).getTime();
  const progress = Math.min(1, Math.max(0, (serverNow() - start) / (end - start)));
  const pos = hexPathPx(oq, or, dq, dr, progress);
  return pos && pos.q != null ? {q: pos.q, r: pos.r} : {q: oq, r: or};
}

// unitPixelNow: the PIXEL position a unit sprite is drawn at right now —
// marching (interpolated along its waypoints/straight hex line) or positioned
// (its own hex). Extracted from render() step 5b (megaron_plan_budets_tre_ben.md,
// slice 3c) so step 6's runner-on-carrier lookup can find the SAME position
// the carrier's own sprite is drawn at, without a second copy of this math —
// 5b's own drawing is unchanged, it just calls this now. Returns null when u
// has neither a marching route nor a standing hex.
function unitPixelNow(u,nowMs=serverNow()) {
 if (u.type==='nomadic_host' && State.unitsData.some(own=>own.id===u.id)) {
  const state=physicalHostState(State.founderPhase,State.unitsData);
  return state && State.founderPhase?.host_unit_id===u.id ? {...hexPx(state.q,state.r),q:state.q,r:state.r} : null;
 }

 if(u.status==='marching'&&u.departs_at&&u.arrives_at&&u.q!=null&&u.target_q!=null){
  const progress=routeProgress(u,nowMs,movementTickNow(nowMs));
  return u.path?.length>1?pathPx(u.path,progress,u.step_costs,u.arrival_tick-u.departure_tick):hexPathPx(u.q,u.r,u.target_q,u.target_r,progress);
 }
 return u.status==='positioned'&&u.q!=null?{...hexPx(u.q,u.r),q:u.q,r:u.r}:null;
}

// transportPixelNow: same idea as unitPixelNow, for a /trades caravan —
// extracted from render() step 7 so step 6's runner-on-carrier lookup can
// place a runner marker on the exact same pixel a naval transport's own
// caravan sprite draws at.
function transportPixelNow(t,nowMs=serverNow()) {
 if(t.mine===false&&t.route_known!==true)return t.current_q!=null?{...hexPx(t.current_q,t.current_r),q:t.current_q,r:t.current_r}:null;
 const progress=routeProgress(t,nowMs,movementTickNow(nowMs));
 return t.path?.length>1?pathPx(t.path,progress,t.step_costs,t.arrival_tick-t.departure_tick):hexPathPx(t.origin_q,t.origin_r,t.dest_q,t.dest_r,progress);
}

// messengerLegPosition (megaron_plan_budets_tre_ben.md, slice 3c, R4): where
// and how to draw an own passage-lifted runner, given the leg the server
// (R3, MapMessengers) says it is actually on. Pure — takes units/trades and
// the current instant explicitly, rather than reading State/serverNow(),
// so it is testable without a DOM stub. Returns null when m.leg is unset
// (today's flat interpolation applies unchanged, R4's own "utan m.leg" rule).
//
// mode is one of: 'walk' (to_port/ashore — a gait, like any other walker),
// 'waiting' (stand still at the port), 'sealed' (stand still, dimmed),
// 'aboard-carrier' (drawn ON a located carrier — no gait, never dimmed) or
// 'aboard-reserve' (server's own leg, carrier not found client-side — no gait).
export function messengerLegPosition(m, units, trades, nowMs) {
  if (!m.leg) return null;
  const legStart = m.leg_start ? new Date(m.leg_start).getTime() : null;
  const legEnd = m.leg_end ? new Date(m.leg_end).getTime() : null;
  const progress = (legStart != null && legEnd != null && legEnd > legStart)
    ? Math.min(1, Math.max(0, (nowMs - legStart) / (legEnd - legStart)))
    : 1;
  if (m.leg === 'to_port' || m.leg === 'ashore') {
    const pos = m.path?.length>1 ? pathPx(m.path,progress) : hexPathPx(m.leg_from_q, m.leg_from_r, m.leg_to_q, m.leg_to_r, progress);
    return {mode: 'walk', x: pos.x, y: pos.y, q: pos.q, r: pos.r};
  }
  if (m.leg === 'waiting') {
    const pos = hexPx(m.leg_from_q, m.leg_from_r);
    return {mode: 'waiting', x: pos.x, y: pos.y, q: m.leg_from_q, r: m.leg_from_r};
  }
  if (m.leg === 'sealed') {
    const pos = hexPx(m.leg_from_q, m.leg_from_r);
    return {mode: 'sealed', x: pos.x, y: pos.y, q: m.leg_from_q, r: m.leg_from_r};
  }
  if (m.leg === 'aboard') {
    if (m.carrier_unit_id) {
      const u = (units || []).find(x => x.id === m.carrier_unit_id);
      const pos = u && unitPixelNow(u,nowMs);
      if (pos) return {mode: 'aboard-carrier', x: pos.x, y: pos.y};
    }
    if (m.carrier_transport_id) {
      const t = (trades || []).find(x => x.id === m.carrier_transport_id);
      if (t) {
        const pos = transportPixelNow(t,nowMs);
        return {mode: 'aboard-carrier', x: pos.x, y: pos.y};
      }
    }
    // R4's stop condition: the carrier isn't in the client's own data (an
    // allied ship, or a transport /trades didn't send) — use the server's
    // own reserve leg instead of hiding the runner.
    const pos = m.path?.length>1 ? pathPx(m.path,routeProgress(m,nowMs,movementTickNow(nowMs)),m.step_costs,m.arrival_tick-m.departure_tick) : hexPathPx(m.leg_from_q, m.leg_from_r, m.leg_to_q, m.leg_to_r, progress);
    return {mode: 'aboard-reserve', x: pos.x, y: pos.y, q: pos.q, r: pos.r};
  }
  return null;
}

// messengerHexNow: the hex an OWN runner's tooltip should match right now —
// legHexNow's own leg-aware cousin (megaron_plan_budets_tre_ben.md, slice 3c
// R5), so hovering the runner's REAL drawn position (waiting at a port,
// aboard a carrier, sealed) shows its hover line, not wherever the old flat
// interpolation would have put it. Reads State directly (unlike the pure
// messengerLegPosition) since it is only ever called from the hover
// hex-match filter below, which already does.
function messengerHexNow(m) {
  if(!m.own)return m.current_q!=null?{q:m.current_q,r:m.current_r}:null;
  if (!m.leg) { const p=m.path?.length>1?pathPx(m.path,routeProgress(m,serverNow())):null; return p?{q:p.q,r:p.r}:legHexNow(m.origin_q,m.origin_r,m.dest_q,m.dest_r,m.sent_at,m.arrives_at); }
  const leg = messengerLegPosition(m, State.unitsData, State.tradeData, serverNow());
  if (leg && leg.q != null) return {q: leg.q, r: leg.r};
  // 'aboard-carrier' has no q/r of its own — match the carrier's own hex.
  if (m.carrier_unit_id) {
    const u = (State.unitsData || []).find(x => x.id === m.carrier_unit_id);
    const at = u && unitHexNow(u);
    if (at) return at;
  }
  if (m.carrier_transport_id) {
    const t = (State.tradeData || []).find(x => x.id === m.carrier_transport_id);
    if (t) { const p=transportPixelNow(t);return p?{q:p.q,r:p.r}:null; }
  }
  return {q: m.leg_from_q, r: m.leg_from_r};
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
const FOREST_FLOOR = '#A6AA68';

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
  '...LLL.LL....',
  '..LLLMMMMEE..',
  '.LLLMMMMMDDE.',
  'LLLMMMMMDDDDE',
  'LMMMMMDMDDDDE',
  '.EMMMDDMMDDE.',
  '..EMMMTMMDE..',
  '....ETTE.....',
  '.....TT......',
  '....T..T.....',
];
const TREE_MID = [
  '..LL.LL..',
  '.LLLMMDE.',
  'LLLMMDDDE',
  'LMMMMDDDE',
  '.EMMTMDE.',
  '...TT....',
  '..T..T...',
];
const TREE_SMALL = [
  '..LL...',
  '.LLMME.',
  'LLMMDDE',
  '.EMMDE.',
  '...T...',
  '..T.T..',
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
    if (!footIsForest(cx + m.gx, cy + m.gy)) continue;
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

  if (MARKRASTER) {
    // Där skogens förvrängda region når ut över hexkanten står också träd,
    // och där slätten vandrat in över hexen står inga.
    for (let i = stand.length - 1; i >= 0; i--) if (!footIsForest(stand[i].x, stand[i].y)) stand.splice(i, 1);
    for (const p of spillStand(q, r, cx, cy)) stand.push({ x: p.x, y: p.y, sprite: p.roll > 0.65 ? SPRITE_MID : SPRITE_SMALL, warm: true });
  }
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
    if (!footIsForest(cx + m.gx, cy + m.gy)) continue;
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

  if (MARKRASTER) {
    // Där skogens förvrängda region når ut över hexkanten står också träd,
    // och där slätten vandrat in över hexen står inga.
    for (let i = stand.length - 1; i >= 0; i--) if (!footIsForest(stand[i].x, stand[i].y)) stand.splice(i, 1);
    for (const p of spillStand(q, r, cx, cy)) stand.push({ x: p.x, y: p.y, sprite: p.roll > 0.5 ? SPRITE_CEDAR_MID : SPRITE_CEDAR_SMALL, rim: true });
  }
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
}  // brytningen, referensens L 202 dämpad mot vårt mörkare hav
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
const isForestTerrain = t => t === 'forest_olive_grove' || t === 'forest_cedar';
// Ett träd står bara där marken under foten är skog — annars står det på
// slätt som vandrat in över hexen. Princip 12 i ny form: stammen står i SKOG.
function footIsForest(x, y) { return !MARKRASTER || isForestTerrain(markTerrainAt(x, y)); }
let chunks = new Map(), chunksFor = null, chunksWorld = null, shownChunks = [];
export const markrasterStats = { chunks: 0, bakeMs: 0, pending: 0, mode: 'budget' };
// Markrastrets kod (se markCode) i en världspunkt, ur den bakade biten.
function markCodeAt(wx, wy) {
  const ix = Math.floor(wx / CHUNK), iy = Math.floor(wy / CHUNK);
  let ch = chunks.get(ix * 100003 + iy);
  if (!ch || chunksFor !== State.tileData) return markCode(markTerrainAt(wx, wy));
  const x = Math.floor(wx) - ch.X0 + ch.M, y = Math.floor(wy) - ch.Y0 + ch.M;
  return ch.code[y * ch.W + x];
}
const markIsSea = (wx, wy) => { const c = markCodeAt(wx, wy); return c === 3 || c === 4; };

function chunkRange() {
  const z = State.camera.zoom * SCALE;
  const x0 = (-State.camera.x) / z, y0 = (-State.camera.y) / z;
  const x1 = (canvas.width - State.camera.x) / z, y1 = (canvas.height - State.camera.y) / z;
  return [Math.floor(x0 / CHUNK), Math.floor(y0 / CHUNK), Math.floor(x1 / CHUNK), Math.floor(y1 / CHUNK)];
}

// Vad en bit beror av: terräng och tier för varje hex som kan nå in i den,
// och städerna vars odling når dit. Hexens region når S + WARP_PAD från
// mitten, ritningen läser grannarna (en hex till) och strand och skum läser
// CHUNK_MARGIN över kanten — 4·S täcker allt. Odlingen når CULT_FAR plus
// brusets ±25 från staden, räknat från tegens hex.
const SIG_REACH = 4 * S, CULT_REACH = CULT_FAR + 25 + SIG_REACH;
let chunkSigs = new Map(), chunkSigsFor = null, chunkSigsCult = null;
function chunkSig(ix, iy) {
  const cult = cultivationList();
  if (chunkSigsFor !== State.tileData || chunkSigsCult !== cult) {
    chunkSigsFor = State.tileData; chunkSigsCult = cult;
    const parts = new Map();
    const add = (x, y, reach, part) => {
      for (let jy = Math.floor((y - reach) / CHUNK); jy <= Math.floor((y + reach) / CHUNK); jy++) {
        for (let jx = Math.floor((x - reach) / CHUNK); jx <= Math.floor((x + reach) / CHUNK); jx++) {
          const k = jx * 100003 + jy;
          let a = parts.get(k);
          if (!a) parts.set(k, a = []);
          a.push(part);
        }
      }
    };
    for (const t of State.tileData) {
      const { x, y } = hexPx(t.q, t.r);
      add(x, y, SIG_REACH, `${t.q},${t.r}:${t.terrain}:${t.tier}`);
    }
    for (const [x, y] of cult) add(x, y, CULT_REACH, `c${x},${y}`);
    chunkSigs = new Map([...parts].map(([k, a]) => [k, a.join('|')]));
  }
  return chunkSigs.get(ix * 100003 + iy) || '';
}

// Worker-backed baking keeps initial/zoom-out chunks off the main thread.
// While a refresh is pending, retain the previous chunk as in the budget path.
// Missing chunks arrive progressively; the live renderer keeps running. Without
// worker 2D support, use the original synchronous viewport + budgeted ring path.
// A fallback chunk can exceed the budget: it is indivisible.
const BAKE_BUDGET_MS = 8;
const closeChunk = ch => { ch?.img?.close?.(); ch?.mem?.close?.(); };
let workerSceneTiles = null, workerSceneCult = null, workerSceneWorld;
let chunkWorker = null, workerTried = false;
function ensureChunkWorker() {
  if (workerTried) return;
  workerTried = true;
  if (typeof Worker === 'undefined' || typeof OffscreenCanvas === 'undefined') return;
  chunkWorker = createChunkWorker(
    () => new Worker(new URL('./markraster_worker.js', import.meta.url), { type: 'module' }),
    (job, ch, bakeMs) => {
      ch.sig = job.sig;
      closeChunk(chunks.get(job.key)); chunks.set(job.key, ch);
      markrasterStats.chunks++; markrasterStats.bakeMs += bakeMs;
      markrasterStats.pending = chunkWorker?.pending || 0;
      State.dirty = true;
    },
    () => { chunkWorker = null; markrasterStats.mode = 'budget'; markrasterStats.pending = 0; State.dirty = true; },
  );
  if (chunkWorker) markrasterStats.mode = 'worker';
}

function updateChunks() {
  if (chunksWorld !== State.WORLD_ID) {
    for (const ch of chunks.values()) closeChunk(ch);
    chunks = new Map(); chunksWorld = State.WORLD_ID;
  }
  chunksFor = State.tileData;
  ensureChunkWorker();
  const cult = cultivationList();
  if (chunkWorker && (workerSceneTiles !== State.tileData || workerSceneCult !== cult || workerSceneWorld !== State.WORLD_ID)) {
    workerSceneTiles = State.tileData; workerSceneCult = cult; workerSceneWorld = State.WORLD_ID;
    // Send only the bake inputs; no units, names, rosters or other live State.
    chunkWorker.setScene({ WORLD_ID: State.WORLD_ID,
      tileData: State.tileData.map(({ q, r, terrain, tier }) => ({ q, r, terrain, tier })),
      provinceData: (State.provinceData || []).map(({ q, r }) => ({ q, r })),
    });
  }
  const t0 = performance.now();
  const bake = (ix, iy, k) => {
    const t = performance.now();
    const ch = bakeChunk(ix, iy);
    ch.sig = chunkSig(ix, iy);
    closeChunk(chunks.get(k)); chunks.set(k, ch);
    markrasterStats.chunks++; markrasterStats.bakeMs += performance.now() - t;
  };
  const [cx0, cy0, cx1, cy1] = chunkRange();
  const later = [];
  for (let iy = cy0 - 1; iy <= cy1 + 1; iy++) {
    for (let ix = cx0 - 1; ix <= cx1 + 1; ix++) {
      const k = ix * 100003 + iy, ch = chunks.get(k);
      const inView = ix >= cx0 && ix <= cx1 && iy >= cy0 && iy <= cy1;
      if (!chunkWorker && !ch && inView) bake(ix, iy, k);
      else if (!ch || ch.sig !== chunkSig(ix, iy)) later.push([ix, iy, k, inView]);
    }
  }
  later.sort((a, b) => b[3] - a[3]);   // utsnittets inaktuella bitar före ringen
  if (chunkWorker) {
    chunkWorker.request(later.map(([ix, iy, key]) => ({ ix, iy, key, sig: chunkSig(ix, iy) })));
    markrasterStats.pending = chunkWorker?.pending || 0;
  } else {
    markrasterStats.pending = later.length;
    for (const [ix, iy, k] of later) {
      if (performance.now() - t0 > BAKE_BUDGET_MS) { State.dirty = true; break; }
      bake(ix, iy, k);
      markrasterStats.pending--;
    }
  }
  shownChunks = [];
  for (let iy = cy0; iy <= cy1; iy++) for (let ix = cx0; ix <= cx1; ix++) {
    const ch = chunks.get(ix * 100003 + iy);
    if (ch) shownChunks.push(ch);
  }
}

let spillCache = new Map(), spillFor = null;
function spillStand(q, r, cx, cy) {
  if (spillFor !== State.tileData) { spillCache = new Map(); spillFor = State.tileData; }
  const key = tileKey(q, r);
  let out = spillCache.get(key);
  if (out) return out;
  out = [];
  const STEP = 7, R = S + WARP_PAD;
  for (let gy = Math.floor((cy - R) / STEP); gy <= Math.ceil((cy + R) / STEP); gy++) {
    for (let gx = Math.floor((cx - R) / STEP); gx <= Math.ceil((cx + R) / STEP); gx++) {
      const h = hash32(gx, gy, 8201);
      const x = gx * STEP + (h & 0x7) - 1, y = gy * STEP + ((h >>> 3) & 0x7) - 1;
      const [eq, er] = hexAtWorld(x, y);
      if (eq === q && er === r) continue;            // inne i polygonen sköter hexen själv
      const [mq, mr] = markHexAt(x, y);
      if (mq !== q || mr !== r) continue;            // regionen når inte hit
      out.push({ x, y, roll: (h >>> 8) / 16777216 });
    }
  }
  spillCache.set(key, out);
  return out;
}

function drawMarkraster(ctx) {
  ctx.save();
  ctx.imageSmoothingEnabled = false;
  updateChunks();
  for (const ch of shownChunks) ctx.drawImage(ch.img, ch.X0, ch.Y0);
  ctx.restore();
}

// Bränningen, live: vågkrön parallella med den förvrängda kusten som rullar
// in mot land med seaTick. Samma formel som drawSurf, men avståndet är
// avståndet till land i markrastrets mask, inte till en hexkant.
function drawMarkSurf(ctx, seaTick) {
  ctx.save();
  ctx.fillStyle = SURF_FOAM;
  for (const ch of shownChunks) {
    const a = ch.surf;
    for (let i = 0; i < a.length; i += 4) {
      const d = a[i + 2];
      const out = (d - 1) / 6;
      if (Math.sin(d * 0.9 + seaTick * 0.55 + a[i + 3]) < 0.35 + 0.45 * out) continue;
      ctx.globalAlpha = 0.62 - 0.42 * out;
      ctx.fillRect(a[i], a[i + 1], 2, 2);
    }
  }
  ctx.globalAlpha = 1;
  ctx.restore();
}

function drawMarkMemory(ctx) {
  ctx.save();
  ctx.imageSmoothingEnabled = false;
  ctx.globalCompositeOperation = 'saturation';
  for (const ch of shownChunks) if (ch.mem) ctx.drawImage(ch.mem, ch.X0, ch.Y0);
  ctx.restore();
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
      if (MARKRASTER) {
        // Dyningen följer den förvrängda kusten: både start, slut och en
        // marginal måste vara hav i markrastret, annars sköljer den upp på land.
        const crest0 = ((gy + (h >>> 6)) & 0x3) === 0;
        const len0 = crest0 ? 12 : 7;
        if (!markIsSea(px - 4, py) || !markIsSea(px + len0 + 6, py)
            || !markIsSea(px + len0 / 2, py - 4) || !markIsSea(px + len0 / 2, py + 5)) continue;
      } else if (!isSeaTerrain(terrainAt(q, r))) continue;
      // Nära en kant mot land får dyningen inte nå fram — den skulle skölja upp
      // på stranden, och strandens vatten är bränningens ärende. Punkter långt
      // in i hexen kan aldrig vara nära en kant: radien avgör det med en
      // multiplikation i stället för sex skalärprodukter, och de flesta celler
      // ligger långt in.
      const c = hexPx(q, r);
      const dx = px - c.x, dy = py - c.y;
      if (!MARKRASTER && dx * dx + dy * dy > (R_IN - 5) * (R_IN - 5)) {
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
// How far apart the two level samples sit, which is what sets the WIDTH of the
// lit rim and the shadow at the foot. At the mountains' distance of 2 px the
// hills came out as thin dark veins — a rim one or two pixels wide reads as a
// crack in the ground, not as a slope. A slope needs a band.
// Kept as documentation of the shape only — the comparison distance now comes
// from the block grid (two cells at STEP 3 = 6 world px), see drawHills.
const HILL_D = 4;
const MTN_STEPS = 5;

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

// ── Deposit resource icons (Sprint 4.5) — tiny pixel markers ────────────
function drawDepositIcons(ctx, cx, cy, tile) {
  const types = [];
  if (tile.copper_deposit) types.push('cu');
  if (tile.tin_deposit)    types.push('sn');
  if (tile.cedar_deposit)  types.push('cd');
  if (tile.silver_deposit) types.push('ag');
  if (tile.incense_deposit) types.push('in');
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
      case 'in':
        // Incense: a golden resin drop (point up, round belly) — a new shape, so
        // it is told apart from copper's disc, tin's slab, cedar's triangle and
        // silver's diamond by silhouette. BILD B4 (Timothy 2026-10-19: "gyllene,
        // ska lysa lite — dyrt, som guld"): a highlight pixel and three pixel
        // sparkles. Sparkles only on a live hex: they sit outside the memory
        // pass's grey and memory carries no glint.
        ctx.fillStyle = '#F2C230';
        ctx.beginPath(); ctx.moveTo(ox, oy - 4.5); ctx.lineTo(ox + 3, oy); ctx.arc(ox, oy + 0.5, 3, 0, Math.PI); ctx.lineTo(ox - 3, oy); ctx.closePath(); ctx.fill(); ctx.stroke();
        ctx.fillStyle = '#FFF6C8';
        ctx.fillRect(Math.round(ox - 1.5), Math.round(oy - 1), 1, 2);
        if (tileTier(tile.q, tile.r) === 'live') {
          ctx.fillStyle = '#FFE680';
          for (const [dx, dy] of [[5, -5], [-5, -3], [5, 3]]) {
            const sx = Math.round(ox + dx), sy = Math.round(oy + dy);
            ctx.fillRect(sx, sy - 1, 1, 3); ctx.fillRect(sx - 1, sy, 3, 1);
          }
        }
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
// Stadsikonerna (cityicons.js, Timothy 2026-10-10). `?stader=gamla` ger de
// inhägnade ¾-massorna för jämförelse.
export const CITYICONS = !(typeof location !== 'undefined' && /[?&]stader=gamla/.test(location.search));
const provinceSeed = p => (p.settlement_id || p.id || (p.q * 131 + p.r * 7)) | 0;
function provinceIcon(p) {
  const ruin = p.state === 'razed' || p.state === 'collapsed';
  // Kajen går ut åt den sida där havsgrannarna ligger (summan av deras
  // kantnormalers x). Hav rakt norr eller söder: höger, som förut.
  const sea = neighborDirs(p.q, p.r, isSeaTerrain);
  const coastal = sea.length ? (sea.reduce((a, i) => a + DIR_NX[i], 0) < -1e-9 ? 'left' : 'right') : false;
  return cityIcon(p.size_tier ? 1 : 0, Math.min(3, p.walls || 0), p.culture || 'akhaier',
                  provinceSeed(p) % 997, coastal, ruin);
}
function drawProvinceIcon(ctx, cx, cy, p) {
  const ruin = p.state === 'razed' || p.state === 'collapsed';
  const ic = provinceIcon(p);
  const { ox, oy } = drawCityIcon(ctx, ic, cx, cy);
  if (ruin) {
    if (p.burning) drawFire(ctx, cx, cy + 3, p.settlement_id || p.id, State.animFrame);
    return;
  }
  if (p.is_outpost) return;
  // Vimpeln: ägaren i FORM och färg. Egen = fyrkantig duk, annans = triangel.
  const accent = p.own ? '#D4AC0D' : (p.allied ? '#4CAF50' : '#C0392B');
  const fx = Math.round(cx) + 2, fy = oy - 6;
  ctx.fillStyle = '#1C1612';
  ctx.fillRect(fx, fy, 1, 7);
  const flag = p.own ? ['AAAA', 'AAAA', 'AAAA'] : ['AA..', 'AAAA', 'AA..'];
  drawStamp(ctx, flag, fx + 1, fy, { A: accent });
  // Garnisonen står i porten (servern nollar army_total utanför LIVE-synen).
  if (p.army_total > 0 && State.camera.zoom >= GARRISON_DOT_ZOOM) {
    const gx = Math.round(cx) - 1, gy = Math.round(cy) + 14 - 4;
    drawStamp(ctx, ['KKK', 'KAK', 'KKK'], gx, gy, { K: '#1C1612', A: '#D9A441' });
  }
  if (p.besieged) drawSiegeDust(ctx, ox, oy, ic.w, ic.h, provinceSeed(p));
  if (p.burning) drawFire(ctx, cx, cy + 3, p.settlement_id || p.id, State.animFrame);
}
// Belägringen: dammoln runt muren, pixelvis (ingen arc()).
function drawSiegeDust(ctx, ox, oy, w, h, seed) {
  for (let k = 0; k < 70; k++) {
    const t = hash32(seed, k, 9301) / 4294967296;
    const per = 2 * (w + h + 8);
    let d = t * per, x, y;
    if (d < w + 4) { x = ox - 2 + d; y = oy - 3; }
    else if ((d -= w + 4) < h + 4) { x = ox + w + 1; y = oy - 2 + d; }
    else if ((d -= h + 4) < w + 4) { x = ox + w + 2 - d; y = oy + h + 1; }
    else { d -= w + 4; x = ox - 2; y = oy + h + 2 - d; }
    x = Math.round(x + (hash32(seed, k, 9302) % 5) - 2);
    y = Math.round(y + (hash32(seed, k, 9303) % 5) - 2);
    ctx.fillStyle = k % 3 ? '#B0A48C' : '#7A6E62';
    ctx.fillRect(x, y, 2, 2);
  }
}
function drawProvince(ctx, cx, cy, p) {
  if (CITYICONS) { drawProvinceIcon(ctx, cx, cy, p); return; }
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
    // Sacked and burned this tick: the city burns for that one game day.
    if (p.burning) drawFire(ctx, cx, cy + 3, p.settlement_id || p.id, State.animFrame);
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
  // Belägring (megaron_plan_belagring.md): NOT FOW-gated on the server — the
  // owner learns it from the flag + falling stock, not from sight — but the
  // marker itself only ever reaches the client when the tile is already
  // visible (m.Visible upstream), so no separate check is needed here. Drawn
  // last, above the standard: a besieged city reads "threatened" before it
  // reads "whose flag flies here".
  if (p.besieged) drawSiegeCloud(ctx, cx, top);
  ctx.restore();
}

// ── Siege cloud — a besieged city's chokepoint is held, not its wall ──────
// Timothy 2026-08-08: "a threatening cloud above the city." Hovers over the
// standard, not the mass itself — the wall never falls (megaron_plan_belagring.md:
// muren rörs aldrig), so nothing about the BUILDING should look damaged; the
// threat sits in the sky over it, exactly like the real thing (an enemy
// holding the road in, not a breach in the stone).
//
// Centred on the STANDARD, not the city mass: the flag is anchored at a
// fixed cx (pole at cx, cloth cx+1..cx+5 above), independent of the mass's
// own width — sprite.w/2 landed the cloud off-centre from the flag it was
// meant to hover over (Timothy 2026-08-08, caught in the same eye-check).
function drawSiegeCloud(ctx, flagX, top) {
  ctx.save();
  const ccx = flagX + 2, ccy = top - 13;
  ctx.fillStyle = '#2E2A28';
  ctx.strokeStyle = '#14100E';
  ctx.lineWidth = 0.6;
  // Fyra överlappande loher — ett stormmoln, inte en enstaka rund fläck.
  for (const [dx, dy, r] of [[-4, 1, 3.2], [0, -1.5, 4], [4, 1, 3.2], [-0.5, 2.4, 3]]) {
    ctx.beginPath();
    ctx.arc(ccx + dx, ccy + dy, r, 0, Math.PI * 2);
    ctx.fill();
    ctx.stroke();
  }
  // Blodröd glöd i underkanten — det HOTFULLA, inte bara mulet väder.
  ctx.fillStyle = '#922B21';
  ctx.fillRect(ccx - 2, ccy + 3, 1, 1);
  ctx.fillRect(ccx + 3, ccy + 4, 1, 1);
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
  if (CITYICONS) {
    const ic = provinceIcon(p);
    const rows = p.build_active ? ACTIVITY_ICONS.build : p.train_active ? ACTIVITY_ICONS.train : ACTIVITY_ICONS.idle;
    drawStamp(ctx, rows, Math.round(cx) - 9, Math.round(cy) + 14 - ic.h - 6);
    ctx.restore();
    return;
  }
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
export function drawTransportActor(ctx, transport, x, y, walkPhase) {
  if (transport.category !== 'naval') return drawActor(ctx, 'caravan', x, y, '', walkPhase);
  const ship = State.unitsData.find(u => u.id === transport.ship_unit_id)
    || State.foreignUnitData.find(u => u.id === transport.ship_unit_id);
  const kind = canonicalUnitType(ship?.type) || 'galley';
  return drawActor(ctx, kind, x, y, '', walkPhase);
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

// ── Runner, waiting for passage (megaron_plan_budets_tre_ben.md, slice 3c) —
// standing still at the port, no gait (same frozen phase 0 as "delivering",
// which this deliberately does not reuse: delivering's gold pulse means
// "handing over the order", a different fact). A stalled wait (a
// PassageStalled dispatch already fired) adds a small blinking red mark,
// clocked on State.animFrame — NEVER the wall clock, so the frozen-frame BILD
// rigs stay deterministic across screenshots (see the foreign-blink comment
// on `blinkTick` above).
function drawMessengerWaiting(ctx, x, y, isOrder, stalled) {
  ctx.save();
  drawActor(ctx, 'runner', x, y, '', 0, isOrder ? '#A03A2A' : '#6B8B4A');
  ctx.fillStyle = '#D8B84A';
  ctx.fillRect(x - 4, y - 16, 1, 4);
  ctx.fillRect(x - 3, y - 16, 2, 1);
  if (stalled && (State.animFrame >> 4) % 2 === 0) {
    ctx.fillStyle = '#C0392B';
    ctx.strokeStyle = '#33291E';
    ctx.lineWidth = 0.7;
    ctx.beginPath();
    ctx.arc(x + 5, y - 15, 2, 0, Math.PI * 2);
    ctx.fill();
    ctx.stroke();
  }
  ctx.restore();
}

// ── Runner, sealed (megaron_plan_budets_tre_ben.md, slice 3c) — between a
// lost carrier and its port; real position unknown, so it stands at the port,
// dimmed (same idiom as fog dimming elsewhere in this file) with a small
// wax-seal mark instead of the gold "whose runner" pennant (it belongs to no
// visible journey right now).
function drawMessengerSealed(ctx, x, y) {
  ctx.save();
  ctx.globalAlpha = 0.4;
  drawActor(ctx, 'runner', x, y, '', 0, '#8A7A6A');
  ctx.globalAlpha = 1;
  ctx.fillStyle = '#5C3A28';
  ctx.strokeStyle = '#1F1712';
  ctx.lineWidth = 0.7;
  ctx.beginPath();
  ctx.arc(x, y - 11, 2.2, 0, Math.PI * 2);
  ctx.fill();
  ctx.stroke();
  ctx.restore();
}

// ── Runner, aboard a located carrier (megaron_plan_budets_tre_ben.md, slice
// 3c) — a small marker offset from the carrier's own sprite, never dimmed
// (the carrier is its own eye — a visible ship makes its passenger visible
// too) and with no gait (it is not moving under its own power).
function drawMessengerAboardCarrier(ctx, x, y, isOrder) {
  ctx.save();
  ctx.fillStyle = isOrder ? '#A03A2A' : '#6B8B4A';
  ctx.strokeStyle = '#33291E';
  ctx.lineWidth = 0.7;
  ctx.beginPath();
  ctx.arc(x + 4, y - 7, 2, 0, Math.PI * 2);
  ctx.fill();
  ctx.stroke();
  ctx.fillStyle = '#D8B84A';
  ctx.fillRect(x + 3, y - 12, 1, 3);
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

// A worked-hex marker: a small labourer on an own catchment hex that has at
// least one placed gubbe (settlement_placement, via /settlements/placement-
// roster). Player report 54f2b747: "indikeras PÅ HEXEN på kartan att det FINNS
// gubbar där och arbetar, sen får man gå in på hexen i stadsvyn för att
// faktiskt se hur många." So this marks PRESENCE only — the count and the good
// live in the city drawer's hex grid (citygrid.js), reached by clicking the
// hex. Drawn bottom-left of centre so it clears the rural building sprite
// (centre) and the deposit icons (top-right); 1px charcoal outline, saturated
// fill, no AA — temenos_designprinciper.
function drawWorkerMarker(ctx, cx, cy) {
  const x = cx - 7, y = cy + 6;
  ctx.save();
  ctx.strokeStyle = RURAL_OUTLINE; ctx.lineWidth = 0.7; ctx.lineJoin = 'round';
  ctx.fillStyle = '#C6603A';           // terracotta tunic — distinct from wheat gold, grey spoil, deposit hues
  ctx.beginPath();                     // body: trapezoid
  ctx.moveTo(x - 2.2, y + 3);
  ctx.lineTo(x - 1.2, y - 1);
  ctx.lineTo(x + 1.2, y - 1);
  ctx.lineTo(x + 2.2, y + 3);
  ctx.closePath(); ctx.fill(); ctx.stroke();
  ctx.fillStyle = '#E7C7A0';           // head: skin
  ctx.beginPath(); ctx.arc(x, y - 2.6, 1.5, 0, Math.PI * 2); ctx.fill(); ctx.stroke();
  ctx.restore();
}

// workedHexesFromRoster reduces the /settlements/placement-roster payload to a
// deduped [{q,r,count,goods}] of own catchment hexes that carry at least one
// placed gubbe on a HEX (building assignments have no q/r — they live inside
// the city, not on the map). One map marker per hex; count (total gubbar) and
// goods feed the hex panel's "Workers here" row.
export function workedHexesFromRoster(roster) {
  const byKey = new Map();
  for (const s of roster || []) {
    for (const a of s.assignments || []) {
      if (a.target_kind !== 'hex' || a.hex_q == null || a.hex_r == null) continue;
      const key = a.hex_q + ',' + a.hex_r;
      let wh = byKey.get(key);
      if (!wh) { wh = { q: a.hex_q, r: a.hex_r, count: 0, goods: [] }; byKey.set(key, wh); }
      wh.count += a.count ?? 1;
      if (a.good_key && !wh.goods.includes(a.good_key)) wh.goods.push(a.good_key);
    }
  }
  return [...byKey.values()];
}

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
// `camera.x += dx`): dragging the map right increases
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
  if (canvas.width === container.clientWidth && canvas.height === container.clientHeight) return;
  canvas.width  = container.clientWidth;
  canvas.height = container.clientHeight;
  State.dirty = true;
}
resizeCanvas();
// WebKit kan köra modulen innan stilmallen är applicerad: då mäts #map-root
// som 1264×182 och fönstret ändrar aldrig storlek efteråt, så kartan blev en
// remsa i Safari (browser_parity 2026-10-09). Följ behållaren, inte fönstret.
new ResizeObserver(resizeCanvas).observe(container);

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
  const stormTick = (State.animFrame / STORM_FRAMES) | 0;
  const stormChanged = (State.stormData || []).some(st => st.tier === 'live') && stormTick !== State.lastStormTick;
  if (stormChanged) State.lastStormTick = stormTick;
  const blinkChanged = State.foreignUnitData.length > 0 && blinkTick !== State.lastBlinkTick;
  if (blinkChanged) State.lastBlinkTick = blinkTick;
  // A burning city animates by itself, like the blink: wake on the fire's step.
  const fireTick = (State.animFrame / FIRE_FRAMES) | 0;
  const fireChanged = fireTick !== State.lastFireTick && State.provinceData.some(p => p.burning);
  if (fireChanged) State.lastFireTick = fireTick;

  if (!State.dirty && !seaChanged && !blinkChanged && !stormChanged && !fireChanged && State.marchData.length === 0 && State.messengerData.length === 0 && State.tradeData.length === 0
      && !State.unitsData.some(u => u.status === 'marching')
      && !State.foreignUnitData.some(u => u.status === 'marching')) {
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
  if (MARKRASTER) {
    // Bas, mark, övergång, strand, skum och hylla: bakade en gång (se Markrastret).
    drawMarkraster(ctx);
    drawMarkSurf(ctx, State.animFrame >> 5);
    pass('base');
  } else {
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
  // 2b. Built road edges (GET /roads, megaron_plan_vagar.md V2) — structure level:
  // under cities, units and the fog tint. The server only sends edges whose two
  // hexes the player has seen or remembers (F5), so nothing here reveals more.
  if ((State.roadEdges && State.roadEdges.length) || (State.roadSites && State.roadSites.length)) {
    drawRoads(ctx, roadSegments(State.roadEdges, hexPx), plannedSegments(State.roadSites, hexPx));
  }
  pass('roads');

  // 2.5 Catchment zone — subtle gold tint on the 19 catchment tiles of own
  // cities (the city's own hex + the radius-2 disc). [0,0] = the settlement's
  // own hex. See CATCHMENT_OFFSETS. Only for the city in focus (Timothy
  // 2026-10-11): the one the City drawer shows, or the own city selected on the
  // map. With the warped ground the tint read as a hex grid over every city.
  if (State.camera.zoom >= LOCAL_ZOOM) {
    for (const p of catchmentFocusCities()) {
      for (const [dq, dr] of CATCHMENT_OFFSETS) {
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
    // Worked-hex markers — own catchment hexes that carry placed gubbar
    // (report 54f2b747). Same LOCAL register as rural projections: "who works
    // here" is a local question. Skip any hex gone to fog since the last fetch.
    for (const wh of State.workedHexes) {
      const t = State.tileData.find(t => t.q === wh.q && t.r === wh.r);
      if (!t || t.terrain === 'fog') continue;
      const { x, y } = hexPx(wh.q, wh.r);
      drawWorkerMarker(ctx, x, y);
    }
  }
  pass('tint+rural');

  // Remembered buildings must join their terrain in the grayscale pass.
  // Live cities retain their existing position above player overlays.
  // 4. Province buildings + flags
  // Ritas norr→söder. Stadsmassorna är upp till 44×32 px och svämmar över
  // hexkanten med flit, så två grannstäder kan överlappa; den sydligare ska då
  // ligga överst, precis som träden och aktörerna sorteras.
  const byDepth = State.provinceData.slice()
    .sort((a, b) => hexPx(a.q, a.r).y - hexPx(b.q, b.r).y);
  for (const p of byDepth) {
    const {x,y} = hexPx(p.q, p.r);
    if (tileTier(p.q, p.r) === 'remembered') drawProvince(ctx, x, y, p);
  }
  pass('remembered-cities');

  // Knowledge tint includes the whole painted hex (ground, canopy, peaks,
  // deposits and remembered buildings), before player UI and live actors.
  if (MARKRASTER) drawMarkMemory(ctx);
  else drawRemembered(ctx, vis, t => { const p = hexPx(t.q, t.r); return hexPts(p.x, p.y); });
  pass('memory');

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
  // march affordance's target, per the server's Eye.Sees (render/sight.js).
  // Fog tiles use the conservative base radius (no mountain bonus, no sea
  // horizon) since we can't know their real terrain without leaking it through
  // the band shape; a fog hex on the line also blocks the sea horizon.
  if (State.fovPreview) {
    const { q: fq, r: fr, kind } = State.fovPreview;
    const terrainAt = tileTerrainLookup();
    for (const t of State.tileData) {
      if (!eyeSees(kind, fq, fr, t.q, t.r, terrainAt)) continue;
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

  // 3.6 Catchment preview — the 19 catchment hexes (target + radius-2 disc) of a
  // hovered/armed colonize affordance. This is deliberately NOT the FOV band
  // above: colonize's true footprint is the fixed catchment (same shape as
  // 2.5's own-city tint, CATCHMENT_OFFSETS), not the per-tile live-visibility
  // radius, which reads as a much bigger and irregular area than what the colony
  // will actually work (Bugg 3).
  if (State.catchmentPreview) {
    const { q: cq, r: cr } = State.catchmentPreview;
    for (const [dq, dr] of CATCHMENT_OFFSETS) {
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

  // 3b. Incoming attack glow — pulsing red on each own province a visible
  // hostile march is headed for. Read from the unit layer (foreign units in
  // live vision), not marchData alone: marching_armies is only written by
  // recall, so the glow never lit for a real march (same root as War→Movements).
  const attackTargets = incomingTargetKeys({
    foreign: State.foreignUnitData, marches: State.marchData, provinces: State.provinceData,
  });
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

  for (const p of byDepth) {
    const { x, y } = hexPx(p.q, p.r);
    if (tileTier(p.q, p.r) !== 'remembered') drawProvince(ctx, x, y, p);
    if (State.camera.zoom >= LOCAL_ZOOM) drawLabel(ctx, x, y, p.name, p.own);
  }
  // Geographic names: chosen italic lettering with a crisp dark halo, no plate.
  if (State.camera.zoom >= LOCAL_ZOOM) {
    const z=State.camera.zoom*SCALE,style=PLACE_LABEL_STYLE;
    ctx.save();
    const measureCity=(text,own)=>{ctx.font=own?'bold 7px monospace':'6px monospace';return ctx.measureText(text).width;};
    const measure=(text)=>{ctx.font=style.font;return ctx.measureText(text).width;};
    const labels=placeLabelLayout(State.tileData,State.provinceData,{
      hexPx,measure,measureCity,zoom:State.camera.zoom,
      viewport:{left:-State.camera.x/z,top:-State.camera.y/z,right:(canvas.width-State.camera.x)/z,bottom:(canvas.height-State.camera.y)/z},
    });
    for(const label of labels)drawPlaceLabel(ctx,label);
    ctx.restore();
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

  // 4c. Storms at sea (GET /storms): under the actors, so ships stay readable inside one.
  // Live ones only on tier-1 hexes is the SERVER's rule (sight); a remembered one is a ghost.
  for (const storm of State.stormData || []) drawStorm(ctx, storm, { S, hexPx, hexPts, frame: State.animFrame });

  drawJourneyOverlays(ctx);

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
      const pos = unitPixelNow(u);
      if (pos && isTileVisible(pos.q, pos.r)) {
        // explore/explore_return share the cyan "explore" sail; other legs use
        // the neutral default colour (intent is resolved server-side on arrival).
        const intent = (u.march_intent === 'explore' || u.march_intent === 'explore_return') ? 'explore' : (u.march_intent || '');
        drawActor(ctx, kind, pos.x, pos.y, intent, walkPhase);
      }
    } else if (u.status === 'positioned' && unitPixelNow(u) && isTileVisible(unitPixelNow(u).q, unitPixelNow(u).r)) {
      const point=unitPixelNow(u);
      if (point) drawActor(ctx, kind, point.x, point.y, '', walkPhase);
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
    // A foreign march no longer discloses its route (Timothy 2026-09-26,
    // "likrikta det"): the server sends only the hex it stands on now, so the
    // walker is drawn there — walking in place — and steps on at each refetch.
    if ((u.status === 'marching' || u.status === 'positioned') && u.q != null && isTileLive(u.q, u.r)) {
      const {x, y} = hexPx(u.q, u.r);
      drawActor(ctx, kind, x, y, '', walkPhase, FOREIGN_ACCENT, foreignOutline);
    }
  }

  // 6. Animated messengers — OWN couriers are drawn along their whole route,
  // dimmed over fog (the player's own runner is information they already
  // possess — temenos_orderlopare_plan.md Fas 5); foreign messengers only
  // inside live-visible tiles, as before.
  //
  // megaron_plan_budets_tre_ben.md (slice 3c): an OWN runner with m.leg set
  // (passage-lifted, R3) is drawn on the physical leg the server says it is
  // actually on, instead of the flat origin→destination line below, which has
  // no notion of "waiting at a port" or "aboard a ship". A runner without
  // m.leg (a plain land courier, or any foreign one) draws exactly as before.
  for (const m of State.messengerData) {
    if(!m.own){
      if(m.current_q!=null&&isTileLive(m.current_q,m.current_r)){
        const p=hexPx(m.current_q,m.current_r);drawMessenger(ctx,p.x,p.y,walkPhase,m.kind==='order',false,false);
      }
      continue;
    }
    if (m.own && m.leg) {
      const leg = messengerLegPosition(m, State.unitsData, State.tradeData, serverNow());
      if (!leg) continue;
      const visible = isTileVisible(leg.q, leg.r); // undefined q/r (waiting/sealed/aboard-carrier) → false, treated as "over fog"
      ctx.save();
      if (!visible) ctx.globalAlpha = 0.45;
      const x = Math.round(leg.x), y = Math.round(leg.y);
      const isOrder = m.kind === 'order';
      if (leg.mode === 'walk') {
        drawMessenger(ctx, x, y, walkPhase, isOrder, true, false);
      } else if (leg.mode === 'waiting') {
        drawMessengerWaiting(ctx, x, y, isOrder, !!m.stalled);
      } else if (leg.mode === 'sealed') {
        drawMessengerSealed(ctx, x, y);
      } else if (leg.mode === 'aboard-carrier') {
        // Full opacity even over fog/dimming: the carrier is its own eye
        // (R4's own rule) — a runner riding a visible ship is visible too.
        ctx.globalAlpha = 1;
        drawMessengerAboardCarrier(ctx, x, y, isOrder);
      } else {
        // 'aboard-reserve': the carrier isn't in the client's own data — draw
        // the server's own reserve leg, no gait (R4).
        drawMessenger(ctx, x, y, 0, isOrder, true, false);
      }
      ctx.restore();
      continue;
    }
    const now = serverNow();
    const sent   = new Date(m.sent_at).getTime();
    const arrives = new Date(m.arrives_at).getTime();
    const progress = Math.min(1, Math.max(0, (now - sent) / (arrives - sent)));
    const pos = m.path?.length>1 ? pathPx(m.path,progress) : hexPathPx(m.origin_q,m.origin_r,m.dest_q,m.dest_r,progress);
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

  // 7. Physical cargo: its carrying ship at sea, a caravan on land.
  for (const t of State.tradeData) {
    const pos = transportPixelNow(t);
    if (pos && (t.mine ? isTileVisible(pos.q,pos.r) : isTileLive(pos.q,pos.r))) {
      drawTransportActor(ctx, t, Math.round(pos.x), Math.round(pos.y), walkPhase);
    }
  }
  pass('actors');

  ctx.restore();
  requestAnimationFrame(render);
}

// Shared geometry is used by both the sprites and these forward-only trails.
export function journeyOverlays(nowMs=serverNow()){
 const out=[],tickNow=movementTickNow(nowMs);
 const own=(actor,kind,relation='own')=>{const p=actorRoute(actor,kind,nowMs,tickNow,hexPx,isTileLive);if(p){p.relation=relation;out.push(p);}};
 const foreign=(actor,kind)=>{const p=foreignArrow(actor,hexPx,isTileLive,kind);if(p)out.push(p);};
 for(const u of State.unitsData)if(u.status==='marching')own(u,u.category==='naval'?'ship':'army');
 for(const m of State.marchData)own(m,m.is_naval?'ship':'army');
 for(const t of State.tradeData)(t.mine||t.route_known===true)?own(t,t.category==='naval'?'ship':'caravan',t.mine?'own':'foreign'):foreign(t,t.category==='naval'?'ship':'caravan');
 for(const m of State.messengerData){
  if(!m.own){foreign(m,'runner');continue;}
  if(m.leg==='waiting'||m.leg==='sealed')continue;
  if(m.leg==='aboard'){
   const carrier=State.unitsData.find(u=>u.id===m.carrier_unit_id)||State.tradeData.find(t=>t.id===m.carrier_transport_id&&t.mine);
   own(carrier?{...carrier,id:m.id}:m,'runner');
  }else if(m.leg){own({...m,departs_at:m.leg_start,arrives_at:m.leg_end},'runner');}
  else own(m,'runner');
 }
 for(const u of State.foreignUnitData)foreign(u,u.category==='naval'?'ship':'army');
 return out;
}
function drawJourneyOverlays(ctx){const livePixel=(x,y)=>{const [q,r]=markHexAt(x,y);return isTileLive(q,r);};for(const p of journeyOverlays())drawMovement(ctx,p,livePixel);}

// ── Data loading ──────────────────────────────────────────────────────────
export async function loadMap() {
  const [tilesRes, provRes, marchRes, msgRes, tradeRes, unitsRes, ruralRes, foreignUnitsRes, rosterRes, stormsRes, roadsRes, roadSitesRes] = await Promise.all([
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/map`),
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces`),
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/marches`),
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/messengers`),
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/trades`),
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units`),
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/rural-projections`),
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/foreign-units`),
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/settlements/placement-roster`),
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/storms`),
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/roads`),
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/road-sites`),
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
  if (rosterRes.ok) {
    State.workedHexes = workedHexesFromRoster(await rosterRes.json());
  }
  if (stormsRes.ok) {
    State.stormData = (await stormsRes.json()).storms || [];
  }
  if (roadsRes.ok) {
    State.roadEdges = (await roadsRes.json()).edges || [];
  }
  if (roadSitesRes.ok) {
    State.roadSites = (await roadSitesRes.json()).sites || [];
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
  const host=physicalHostState(State.founderPhase,State.unitsData);
  if (host) return {q:host.q,r:host.r};
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
  scrub_maquis:       'wine (marginal), incense (if deposit)',
  semi_desert:        'incense (if deposit)',
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
// owner's name, and with no "Show →" button (there is nothing of theirs to
// act on).
function unitListHTML(units, foreignUnits) {
  let html = '';
  if (units.length) {
    const rows = units.map(u => {
      const lbl = actorName(u);
      return '<div style="display:flex;justify-content:space-between;align-items:center;gap:.4rem;padding:.2rem 0">'
        + '<span>' + lbl + ' <span style="color:var(--text-dim)">(' + u.status + ')</span></span>'
        + '<button data-unit-id="' + u.id + '" style="padding:.15rem .35rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.65rem;cursor:pointer">Show →</button>'
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
  river_ford: 'River Ford',
};

// The own cities whose catchment tint is shown: the one the City drawer is
// open on, and an own city whose hex is selected on the map.
export function catchmentFocusCities() {
  const out = [];
  if (State.activeDrawer === 'city') { const c = activeCitySettlement(); if (c) out.push(c); }
  const h = State.selectedHex;
  if (h) {
    const p = State.provinceData.find(x => x.q === h.q && x.r === h.r && x.own && !x.is_outpost);
    if (p && !out.includes(p)) out.push(p);
  }
  return out;
}

function terrainLabel(t) {
  return TERRAIN_LABELS[t] || (t.charAt(0).toUpperCase() + t.slice(1).replaceAll('_', ' '));
}

function fillTerrainFields(tile) {
  document.getElementById('ip-terrain').textContent = terrainLabel(tile.terrain);
  const deps = [tile.copper_deposit ? 'Copper' : null, tile.tin_deposit ? 'Tin' : null,
                tile.silver_deposit ? 'Silver' : null, tile.cedar_deposit ? 'Cedar' : null,
                tile.incense_deposit ? 'Incense' : null].filter(Boolean);
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
  ['ip-owner-row', 'ip-defence-row'].forEach(id => {
    document.getElementById(id).style.display = visible ? '' : 'none';
  });
}

// Build the same dest object the march-ctx menu consumes, whether the caller is
// the left-click affordance panel or the right-click context menu. target is the
// province marker at (h.q,h.r), or null for an empty/sea/mountain/fog hex.
// known mirrors the server's FOW tier ('fog' terrain means this Wanax has
// never seen it — same knowledge the march FOW rule in march_start.go checks).
export function destFromHex(h, tile, target) {
  const known = tile.terrain !== 'fog';
  const isSea = isWaterTerrain(tile.terrain);
  if (target) {
    return { q: h.q, r: h.r, terrain: tile.terrain, isSea, known,
             name: target.name, isSettlement: true, allied: target.own ? true : !!target.allied };
  }
  return { q: h.q, r: h.r, terrain: tile.terrain, isSea, known,
           name: known ? `${namedPlaceTerrain(tile, terrainLabel(tile.terrain))} (${h.q},${h.r})` : `Unexplored (${h.q},${h.r})`,
           isSettlement: false, allied: false };
}

// Fog: nothing known yet — no terrain/deposits/produces, no affordances.
function openFogPanel(h) {
  document.getElementById('ip-name').textContent = 'Unexplored land';
  setCityFieldsVisible(false);
  document.getElementById('ip-terrain').textContent = `(${h.q},${h.r})`;
  document.getElementById('ip-deposits-row').style.display = 'none';
  document.getElementById('ip-produces').textContent = '—';
  document.getElementById('ip-foot').innerHTML = '<p class="empty-state">Sail or march nearby to reveal.</p>';
  document.getElementById('inspect-panel').style.display = 'flex';
}

// Foreign/allied settlement — owner and qualitative defence from the same data,
// plus the units-here list and a Marschera button. Own settlements never reach
// this function — they bypass the panel for the city drawer (see openHexPanel).
function openCityPanel(h, tile, marker, units, foreignUnits) {
  document.getElementById('ip-name').textContent    = marker.name;
  setCityFieldsVisible(true);
  fillTerrainFields(tile);

  let ownerText = marker.owner || '(unoccupied)';
  if (marker.allied) ownerText += ' (allied)';
  document.getElementById('ip-owner').textContent = ownerText;
  const defence = document.getElementById('ip-defence');
  // Coarsen only the walls and land defenders this panel already exposed.
  // No combat odds/threshold are inferred. Refused data stays unknown.
  const showDefence = army => {
    if (State.selectedHex?.q !== h.q || State.selectedHex?.r !== h.r ||
        document.getElementById('ip-defence-row').style.display === 'none') return;
    const defended = marker.walls > 0 || (army &&
      ((army.Spearman || 0) > 0 || (army.EliteInfantry || 0) > 0 || (army.WarChariot || 0) > 0));
    defence.textContent = defended ? 'Defended' : army && Number.isFinite(marker.walls) ? 'No defenders seen' : 'unknown';
  };
  showDefence(null);
  fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${marker.id}/army`)
    .then(async r => showDefence(r.ok ? await r.json() : null))
    .catch(() => showDefence(null));

  const foot = document.getElementById('ip-foot');
  let footHtml = placePanelHTML(tile) + unitListHTML(units, foreignUnits);
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
  footHtml += '<button id="ip-march-btn" style="' + MARCH_BTN_STYLE + '">March here →</button>';
  foot.innerHTML = footHtml;
  bindUnitButtons(foot);
  bindMarchButton(document.getElementById('ip-march-btn'), destFromHex(h, tile, marker), 'land');

  document.getElementById('inspect-panel').style.display = 'flex';
}

// "Workers here" line for the player's own worked hexes (inspel 2026-09-08).
// The roster is owner-scoped, so a foreign or unworked hex simply has no line.
function workersHereHTML(h) {
  const wh = (State.workedHexes || []).find(w => w.q === h.q && w.r === h.r);
  if (!wh) return '';
  return `<p class="empty-state">Workers here: ${wh.count} (${wh.goods.join(', ')})</p>`;
}

// Mountain / sea / empty land — no province here. Mountains explain their own
// absence of affordances; sea gets galleys; empty land gets march + colonize.
function openTerrainPanel(h, tile, isMountain, isSea, units, foreignUnits) {
  document.getElementById('ip-name').textContent =
    isSeaTerrain(tile.terrain) ? `Sea (${h.q},${h.r})` : `${namedPlaceTerrain(tile, isMountain ? 'Mountains' : terrainLabel(tile.terrain))} (${h.q},${h.r})`;
  setCityFieldsVisible(false);
  fillTerrainFields(tile);

  const foot = document.getElementById('ip-foot');
  let footHtml = placePanelHTML(tile) + workersHereHTML(h) + unitListHTML(units, foreignUnits);
  if (isSea) footHtml += stormPanelHTML((State.stormData || []).filter(st => st.hexes.some(sh => sh.q === h.q && sh.r === h.r)));

  if (isMountain) {
    footHtml += '<p class="empty-state">Impassable — armies cannot go here.</p>';
    foot.innerHTML = footHtml;
    bindUnitButtons(foot);
    document.getElementById('inspect-panel').style.display = 'flex';
    return;
  }

  const dest = destFromHex(h, tile, null);
  if (isSea) {
    footHtml += '<button id="ip-march-btn" style="' + MARCH_BTN_STYLE + '">Send galleys →</button>';
  } else if (State.founderPhase) {
    // Founder phase: empty land is a POSSIBLE HOME, never a colony ("aldrig
    // Kolonisera"). March the host here; the founding forecast shows what the
    // ground would feed. Settle itself lives on the Host panel — the server
    // founds where the host stands, nowhere else.
    // The forecast belongs in the scrolling body, not beside the action in the
    // foot — same reason as the Host panel: it is unbounded and it was pushing
    // the foot past the panel's bottom edge.
    footHtml += '<button id="ip-march-btn" style="' + MARCH_BTN_STYLE + '">March here →</button>';
    document.getElementById('ip-body-extra').innerHTML =
      '<div id="ip-found-summary">Fetching founding forecast…</div>'
      + '<details class="dsec"><summary class="dsec-title">Details</summary>'
      + '<div id="ip-found-preview" style="font-size:.73rem"></div></details>';
  } else {
    footHtml += '<button id="ip-march-btn" style="' + MARCH_BTN_STYLE + '">March here →</button>'
             +  '<button id="ip-colonize-btn" style="' + MARCH_BTN_STYLE + '">Colonize →</button>';
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
    const summaryEl = document.getElementById('ip-found-summary');
    const previewEl = document.getElementById('ip-found-preview');
    const stillThisSite = () => State.founderPhase && State.selectedHex?.q === h.q && State.selectedHex?.r === h.r
      && document.getElementById('ip-found-summary') === summaryEl;
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/colonize-preview?q=${h.q}&r=${h.r}`
        + `&pop=${fp.population}&seed=${Math.max(0, fp.grain?.amount || 0)}`)
      .then(r => r.ok ? r.json() : null)
      .then(p => {
        if (!stillThisSite()) return;
        summaryEl.innerHTML = hostForecastSummaryHTML(p);
        if (p) {
          const total = (p.catchment || []).length;
          const known = total - (p.unknown_hexes || 0);
          summaryEl.innerHTML += `<div class="dim">Based on ${fmtNum(known)} of ${fmtNum(total)} hexes known.</div>`;
          previewEl.innerHTML = window.renderColonizePreviewHTML(p);
        } else previewEl.textContent = 'Founding forecast unavailable.';
      })
      .catch(() => {
        if (stillThisSite()) summaryEl.textContent = 'Founding forecast unavailable.';
      });
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
const RURAL_LABELS = {
  farm: 'Farm', mine: 'Mine', lumbermill: 'Lumbermill', stonequarry: 'Stone Quarry',
  silver_mine: 'Silver Mine', olive_press: 'Olive Press', winery: 'Winery',
};

function openRuralPanel(h, tile, rural, units, foreignUnits) {
  // Title is the ground, not the building (inspel 2026-09-08): the building is
  // a line in the card.
  document.getElementById('ip-name').textContent = `${namedPlaceTerrain(tile, terrainLabel(tile.terrain))} (${h.q},${h.r})`;
  setCityFieldsVisible(false);
  fillTerrainFields(tile);

  const foot = document.getElementById('ip-foot');
  const buildingLabel = RURAL_LABELS[rural.building_type] || rural.building_type;
  let footHtml = `<p class="empty-state">${buildingLabel} — part of ${rural.name}'s hinterland, worked from here.</p>`;
  footHtml += placePanelHTML(tile);
  footHtml += workersHereHTML(h);
  footHtml += unitListHTML(units, foreignUnits);
  footHtml += `<button id="ip-rural-city-btn" style="${MARCH_BTN_STYLE}">Open ${rural.name} →</button>`;
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

// ticks_left already counts game days (one tick = one day). Never divide
// by twenty-four or convert this player-facing store duration to wall time.
function hostStoreLine(label, s) {
  if (!s || s.ticks_left == null) return `${label} lasts indefinitely`;
  const days = Number(s.ticks_left);
  if (!Number.isFinite(days)) return `${label} duration unknown`;
  return `${label} lasts ${fmtDays(days)}`;
}

// Summarize the same existing forecast. Do not round before deciding yes/no.
export function hostForecastSummaryHTML(p) {
  const net = p?.grain?.food_net_per_tick ?? p?.grain?.est_net_per_tick;
  const feeds = typeof net === 'number' && Number.isFinite(net) ? (net >= 0 ? 'yes' : 'no') : 'unknown';
  const potential = good => {
    if (!p?.goods || typeof p.goods !== 'object') return 'unknown';
    const rate = p.goods[good] ?? 0;
    return typeof rate === 'number' && Number.isFinite(rate) ? (rate > 0 ? 'yes' : 'no') : 'unknown';
  };
  // The harbour rule is the one irreversible geography fact: an inland
  // metropolis never gets ships. It must not hide behind Details.
  const ships = typeof p?.harbour_note === 'string' ? (p.harbour_note.startsWith('Coastal') ? 'yes' : 'never — inland') : 'unknown';
  return `<div class="inspect-row"><span class="ir-label">Feeds itself:</span><span> ${feeds}</span></div>`
    + `<div class="inspect-row"><span class="ir-label">Timber:</span><span> ${potential('timber')}</span></div>`
    + `<div class="inspect-row"><span class="ir-label">Stone:</span><span> ${potential('stone')}</span></div>`
    + `<div class="inspect-row"><span class="ir-label">Ships:</span><span> ${ships}</span></div>`
    + (p?.timber_warning ? `<div class="dim">${esc(p.timber_warning)}</div>` : '');
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
  if (State.selectedHex?.q !== h.q || State.selectedHex?.r !== h.r) return;
  const fp = State.founderPhase;
  if (!fp) { openHexPanel(h); return; } // settled meanwhile — normal routing
  const hostState=physicalHostState(fp,State.unitsData);
  if (!hostState || hostState.q!==h.q || hostState.r!==h.r) {
    document.getElementById('ip-name').textContent='Nomadic Host';
    document.getElementById('ip-body-extra').textContent=hostState
      ? `The Host is now at (${hostState.q},${hostState.r}). Select its marker again.` : 'The Host has moved. Select its marker again.';
    document.getElementById('ip-foot').replaceChildren();
    document.getElementById('inspect-panel').style.display='flex';
    return;
  }


  document.getElementById('ip-name').textContent = 'Nomadic Host';
  setCityFieldsVisible(false);
  fillTerrainFields(tile);
  const depositsRow = document.getElementById('ip-deposits-row');
  const deposits = depositsRow.style.display === 'none' ? '—' : document.getElementById('ip-deposits').textContent;
  depositsRow.style.display = 'none';
  document.getElementById('ip-produces-row').style.display = 'none';

  // Stores and forecast go in the SCROLLING body; only the action stays in the
  // foot. Both used to live in the foot, which has no scroll of its own — so a
  // long forecast (the 7-hex list plus the gifts plus the goods rows) pushed the
  // founding button below the panel's bottom edge, where it was clipped and
  // unreachable. Measured before this change at 1280×720: the button sat 71 px
  // past the panel. The forecast is unbounded by nature; the button must not
  // share a container with it.
  document.getElementById('ip-body-extra').innerHTML =
    placePanelHTML(tile, {showName:true}) + `<div class="inspect-row" id="ip-host-state">${esc(hostStatusText(hostState))} · (${hostState.q},${hostState.r})</div><div id="ip-found-summary">Fetching founding forecast…</div>
     <details id="ip-host-details" class="dsec"><summary class="dsec-title">Details</summary>
     <div style="margin-bottom:.5rem;line-height:1.5">
       <div>Deposits: ${esc(deposits)}</div>
       <div>Produces: ${esc(producesText(tile))}</div>
       <div>${fmtNum(fp.population || 0)} people · cannot fight · sight 2 hexes (4 by water or on mountains)</div>
       <div>${hostStoreLine('Food', fp.grain)}</div>
       <div>${hostStoreLine('Silver reserve', fp.silver)}</div>
       <div>${fmtNum(fp.spearmen_in_field || 0)} Spearmen ${fp.spearmen_in_field === 1 ? 'cohort' : 'cohorts'} in the field</div>
       <div>Messengers free to send</div>
     </div>
     <div id="ip-found-preview" style="font-size:.73rem;border-top:1px solid var(--border);padding-top:.4rem">Fetching founding forecast…</div>
     </details>`;

  const foot = document.getElementById('ip-foot');
  foot.innerHTML =
    `<button id="ip-settle-btn" style="${MARCH_BTN_STYLE}">⚒ Found the metropolis here</button>
     <div id="ip-settle-err"></div>`;
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
  const previewEl = document.getElementById('ip-found-preview');
  const summaryEl = document.getElementById('ip-found-summary');
  const stillThisHost = () => State.founderPhase && State.selectedHex?.q === h.q && State.selectedHex?.r === h.r
    && document.getElementById('ip-found-preview') === previewEl;
  summaryEl.innerHTML = hostForecastSummaryHTML(null);
  fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/colonize-preview?q=${h.q}&r=${h.r}`
      + `&pop=${fp.population}&seed=${Math.max(0, fp.grain?.amount || 0)}&starter_farm=1`)
    .then(r => r.ok ? r.json() : null)
    .then(p => {
      if (!stillThisHost()) return;
      summaryEl.innerHTML = hostForecastSummaryHTML(p);
      if (p) previewEl.innerHTML = window.renderColonizePreviewHTML(p);
      else previewEl.textContent = 'Founding forecast unavailable.';
    })
    .catch(() => {
      if (stillThisHost()) previewEl.textContent = 'Founding forecast unavailable.';
    });

  const settleButton = document.getElementById('ip-settle-btn');
  const result = document.getElementById('ip-settle-err');
  settleButton.disabled = !hostState.can_found_here;
  let pending = false;
  settleButton.addEventListener('click', () => {
    if (pending) return;
    confirmInline(result, 'Found the metropolis here? The host dissolves — forever.', async () => {
      pending = true;
      settleButton.disabled = true;
      try {
        const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/founding/settle`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({expected_q:hostState.q,expected_r:hostState.r}),
        });
        if (res.ok) {
          // New province, new eyes, host gone: refresh the changed world.
          location.reload();
        } else {
          const err = await res.json().catch(() => ({ error: 'Unknown error' }));
          result.replaceChildren();
          showInlineResult(result, err.error || 'Error', true);
          pending = false;
          settleButton.disabled = false;
        }
      } catch (_) {
        result.replaceChildren();
        showInlineResult(result, 'Could not found the metropolis. Try again.', true);
        pending = false;
        settleButton.disabled = false;
      }
    }, { confirmLabel: 'Found the metropolis', cancelLabel: 'Keep travelling' });
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
  document.getElementById('ip-produces-row').style.display = '';

  const tile = State.tileData.find(t => t.q === h.q && t.r === h.r);
  const prov = State.provinceData.find(p => p.q === h.q && p.r === h.r);

  if (!tile || tile.terrain === 'fog') { openFogPanel(h); return; }

  // Founder phase: the host's own hex opens the Host panel, never a unit view.
  if (State.founderPhase && (State.unitsData || []).some(u =>
      u.type === 'nomadic_host' && unitHexNow(u)?.q === h.q && unitHexNow(u)?.r === h.r)) {
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
  const isSea = isWaterTerrain(tile.terrain);
  openTerrainPanel(h, tile, isMountain, isSea, units, foreignUnits);
}

// Re-render the open hex panel from fresh State (e.g. after a storm has been named).
export function reopenSelectedHex() {
  if (State.selectedHex) openHexPanel(State.selectedHex);
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
  const res = await fetchAuth(sendPath, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + token },
    body: JSON.stringify({ destination_id: destSettlementID, message: text }),
  });
  if (res.ok) {
    textEl.value = '';
    errEl.style.color = 'var(--safe-text)';
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

  // ── Input: one pointer path; game actions remain the existing map paths ──
  const cancelGesture = bindMapInput(canvas, {
    pan(dx, dy) {
      State.camera.x += dx; State.camera.y += dy;
      clampCamera(); State.dirty = true;
    },
    zoom(factor, clientX, clientY, dx = 0, dy = 0) {
      const rect = canvas.getBoundingClientRect();
      const next = zoomStep(State.camera, factor, clientX - rect.left, clientY - rect.top);
      Object.assign(State.camera, next, { x: next.x + dx, y: next.y + dy });
      clampCamera(); State.dirty = true;
    },
    tap(clientX, clientY) {
      const rect = canvas.getBoundingClientRect();
      const h = hexAtScreen(clientX - rect.left, clientY - rect.top);
      if (State.marchCtxUnitID) {
        const tile = State.tileData.find(t => t.q === h.q && t.r === h.r);
        const target = State.provinceData.find(p => p.q === h.q && p.r === h.r);
        if (tile) window.openMarchCtx(destFromHex(h, tile, target), clientX, clientY);
      } else openHexPanel(h);
    },
    orders: openMapOrders,
    hideHover() { tooltip.style.display = 'none'; },
    hover(clientX, clientY) {
      const rect = canvas.getBoundingClientRect();
      const h = hexAtScreen(clientX - rect.left, clientY - rect.top);
      const tile = State.tileData.find(t => t.q === h.q && t.r === h.r);
      const prov = State.provinceData.find(p => p.q === h.q && p.r === h.r);
      if (tile && tile.terrain !== 'fog') {
        tooltip.style.display = 'block';
        tooltip.style.left = (clientX + 14) + 'px';
        tooltip.style.top  = (clientY - 22) + 'px';
        const deposits = [tile.copper_deposit ? '⚒ Copper' : null, tile.tin_deposit ? '⚒ Tin' : null,
                          tile.silver_deposit ? '⚒ Silver' : null, tile.cedar_deposit ? '⚒ Cedar' : null,
                          tile.incense_deposit ? '⚒ Incense' : null].filter(Boolean).join(' · ');
        const tl = tile.terrain.charAt(0).toUpperCase() + tile.terrain.slice(1);
        // display_name är serverformaterat ("2nd Spearmen of Knossos") — webben
        // bygger aldrig om den grammatiken (se unit.go:DisplayName). Två fall som
        // en naiv u.q-jämförelse får fel: garnisonerade förband saknar helt
        // hexposition (matchas på provinsens settlement_id), och en MARSCHERANDE
        // enhets u.q/u.r är avgångshexen — gångaren ritas vid pathPx/hexPathPx
        // interpolerade waypoint, så tooltipen måste läsa samma position som
        // spriten. Annars pekar namnet på en tom hex enheten lämnat.
        const own = (State.unitsData || []).filter(u => {
          const at = unitHexNow(u);
          if (at) return at.q === h.q && at.r === h.r;
          return prov && u.settlement_id && u.settlement_id === prov.settlement_id;
        });
        // Every other actor is placed exactly where render() draws it — foreign
        // units only on live tiles, caravans and runners along their leg.
        const here = at => at && at.q === h.q && at.r === h.r;
        const foreign = (State.foreignUnitData || []).filter(u =>
          (u.status === 'marching' || u.status === 'positioned') && here(unitHexNow(u)) && isTileLive(h.q, h.r));
        const caravans = (State.tradeData || []).filter(t =>
          here(transportPixelNow(t)));
        const runners = (State.messengerData || []).filter(m => here(messengerHexNow(m)));
        const placeName = (q, r) => {
          const p = State.provinceData.find(p => p.q === q && p.r === r);
          return p && p.name ? p.name : `(${q},${r})`;
        };
        const storms = (State.stormData || []).filter(st => st.hexes.some(sh => here(sh)));
        const namedOwn=own.map(u=>u.type==='nomadic_host'?{...u,host_state:physicalHostState(State.founderPhase,State.unitsData)}:u);
        const names = unitHoverLines({ own:namedOwn, foreign, caravans, runners, storms, placeName });
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
    },
  });
  window.addEventListener('blur', cancelGesture);
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'hidden') cancelGesture();
  });

  window.addEventListener('resize', () => { State.dirty = true; });

  // Hide map tooltip when pointer enters any drawer
  document.querySelectorAll('.drawer').forEach(d => {
    d.addEventListener('mouseenter', () => { tooltip.style.display = 'none'; });
  });

  loadMap().then(() => { State.dirty = true; render(); });

  // Mouse right-click and touch/pen long-press share every affordance/FOW rule.
  function openMapOrders(clientX, clientY) {
    const capital = ownCapital();
    if (!capital) return;
    const rect = canvas.getBoundingClientRect();
    const h = hexAtScreen(clientX - rect.left, clientY - rect.top);
    const target = State.provinceData.find(p => p.q === h.q && p.r === h.r);
    const tile = State.tileData.find(t => t.q === h.q && t.r === h.r);
    if (!tile) { window.closeMarchCtx(); return; }
    if (State.marchCtxUnitID) {
      window.openMarchCtx(destFromHex(h, tile, target), clientX, clientY);
      return;
    }
    if (tile.terrain === 'fog') {
      // Never seen by this Wanax: terrain, ownership and occupants are all
      // unknown by definition, so none of the checks below (settlement,
      // mountain, own-unit-standing-here) apply. The only order an unseen
      // hex accepts is explore (march_start.go exempts intent=explore from
      // the "none of your men have ever seen it" FOW rule) — openMarchCtx
      // forces and locks that checkbox for an unknown dest.
      window.openMarchCtx(destFromHex(h, tile, null), clientX, clientY);
      return;
    }
    const isMountain = tile.terrain === 'mountain_limestone' || tile.terrain === 'mountain_red';
    if (target) {
      // Own settlement (capital included): march units home to reinforce the
      // garrison. Another Wanax's settlement: march to attack (or reinforce if
      // allied). Inspect lives on left-click — right-click is always orders.
      window.openMarchCtx(destFromHex(h, tile, target), clientX, clientY);
      return;
    }
    // Empty hex. Mountains are impassable; sea hexes take ships.
    if (isMountain) { window.closeMarchCtx(); return; }
    // Moment-22: an own field unit standing right here IS the order target,
    // not the hex — marching-to-your-own-hex is rejected server-side ("cannot
    // march to own hex"). Route to the same map→drawer bridge the left-click
    // "Show →" button uses (war.js warFocusUnit): the War drawer opens with
    // that unit's card focused, its March button ready. This detects "is a
    // friendly unit standing here" (u.deployable, not u.size — the server has
    // no size gate); it deliberately does NOT also exclude fortify stance like
    // war.js canMarch does — a fortified unit still occupies the hex, and
    // routing to its card (Stance selector to un-fortify) beats falling through
    // to a march-to-own-hex 422.
    const ownUnit = (State.unitsData || []).find(u =>
      u.q === h.q && u.r === h.r && (u.status === 'garrison' || u.status === 'positioned') &&
      u.deployable);
    if (ownUnit) { window.closeMarchCtx(); window.warFocusUnit(ownUnit.id); return; }
    window.openMarchCtx(destFromHex(h, tile, null), clientX, clientY);
  }

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
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/settlements/placement-roster`).then(r => r.ok && r.json().then(d => { State.workedHexes = workedHexesFromRoster(d); State.dirty = true; }));
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/storms`).then(r => r.ok && r.json().then(d => { State.stormData = d.storms || []; State.dirty = true; }));
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/roads`).then(r => r.ok && r.json().then(d => { State.roadEdges = d.edges || []; State.dirty = true; }));
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/road-sites`).then(r => r.ok && r.json().then(d => { State.roadSites = d.sites || []; State.dirty = true; }));
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
    const civilianMoving=State.tradeData.length>0||State.messengerData.some(m=>!m.own&&m.heading);
    if (!State.unitsData.some(u => u.status === 'marching') && !courierOut && !foreignMoving && !civilianMoving) return;
    refreshTiles();
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units`).then(r => r.ok && r.json().then(d => { State.unitsData = d.units || []; State.dirty = true; }));
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/foreign-units`).then(r => r.ok && r.json().then(d => { State.foreignUnitData = d; State.dirty = true; }));
    // A Runner en route needs the same fast cadence: its delivery flips
    // the unit to marching (or applies a stance) server-side — poll messengers
    // so the runner vanishes and the unit moves without waiting for the 30s tick.
    fetchAuth(`/api/v1/worlds/${State.WORLD_ID}`).then(r=>r.ok&&r.json().then(d=>anchorWorldTime(d)));
    if(State.tradeData.length)fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/trades`).then(r=>r.ok&&r.json().then(d=>{State.tradeData=d;State.dirty=true;}));
    if (courierOut || civilianMoving) {
      fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/messengers`).then(r => r.ok && r.json().then(d => { State.messengerData = d; State.dirty = true; }));
    }
  }, 3000);
}
