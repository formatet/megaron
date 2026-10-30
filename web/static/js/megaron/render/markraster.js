// Shared terrain renderer: explicit scene input, private per-instance caches.
// No DOM, global game State or worker dependency. Both canvas kinds run this code.
import { isSea, isWaterBarrier } from '../terrain.js';

export function createMarkRenderer(scene, createCanvas, markraster = true) {

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

  // ── Hex geometry ─────────────────────────────────────────────────────────
  const S = 22;    // hex size in logical pixels — Settlers 2 scale

  const SCALE = 2; // canvas scale factor

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
    if (BAKE) return base;  // markrastret bakas alltid i full upplösning
    const px = base * scene.camera.zoom * SCALE;
    const k = Math.max(1, Math.round(LOD_MIN_BLOCK_PX / px));
    return base * k;
  }

  function hexPx(q, r) {
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

  // ── Hex path helpers ─────────────────────────────────────────────────────
  // Six axial directions in cube space.
  const HEX_DIRS = [[1,0],[-1,0],[0,1],[0,-1],[1,-1],[-1,1]];

  // ── Hex fill — solid path fill + outline ─────────────────────────────────
  function hexPath(ctx, pts) {
    ctx.beginPath();
    ctx.moveTo(pts[0][0], pts[0][1]);
    for (let i = 1; i < 6; i++) ctx.lineTo(pts[i][0], pts[i][1]);
    ctx.closePath();
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
  // The world component is hashed from scene.WORLD_ID, NOT worlds.map_seed —
  // map_seed never leaves the server, and WORLD_ID gives the same guarantee
  // (stable per world, different between worlds) with no server change. This is
  // presentation only: it must never feed mapgen or any rule.
  let worldSalt = 0;

  let worldSaltFor;

  function salt() {
    if (worldSaltFor !== scene.WORLD_ID) {
      worldSaltFor = scene.WORLD_ID;
      let h = 0x9e3779b9;
      const s = String(scene.WORLD_ID ?? '');
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
  // and scene.tileData.find() is O(n) over the whole map (2240 tiles on the
  // current world) — that would be ~400k comparisons a frame for one forest
  // block. scene.tileData is only ever REASSIGNED (loadMap/refreshTiles), never
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
  let riverEdgeIndex = new Map();

  function terrainAt(q, r) {
    if (tileIndexFor !== scene.tileData) {
      tileIndexFor = scene.tileData;
      tileIndex = new Map(scene.tileData.map(t => [tileKey(t.q, t.r), t.terrain]));
      riverEdgeIndex = new Map(scene.tileData.filter(t => Array.isArray(t.river_edges)).map(t => [tileKey(t.q,t.r),t.river_edges]));
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

  const isSeaTerrain = isSea;

  // Havet och floden är båda vatten, men de är INTE utbytbara i renderaren, och
  // skillnaden är kontrastbudgeten (princip 6): strandbandets ljusa sand är
  // reserverad åt kustlinjen. Fick floden samma band skulle varje flodhex rita
  // en strand, och kustlinjen — kartans viktigaste gräns — skulle sluta vara
  // unik. `isSeaTerrain` styr därför strand, bränning, dyning och djupbryt och
  // får ALDRIG innehålla floden; `isWaterTerrain` är den bredare frågan "är det
  // här vatten?" som flodens egen kedja och tooltipen ställer.
  const isWaterTerrain = isWaterBarrier;

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

  const FOREST_EARTH = '#BCA872'; // dry ochre ground between the trees

  const FOREST_SCRUB = '#8A9152'; // sage scrub catching light

  const FOREST_EDGE  = '#BDB47C'; // sun-bleached ground where the grove opens up

  function drawForestFloor(ctx, cx, cy, q, r) {
    ctx.save();
    hexClip(ctx, cx, cy);

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
    const x0 = Math.floor((cx - padS()) / STEP) * STEP, x1 = cx + padS();
    const y0 = Math.floor((cy - padS()) / STEP) * STEP, y1 = cy + padS();
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
    hexClip(ctx, cx, cy);

    // Blocks on integer coordinates aligned to a GLOBAL lattice (floor to STEP,
    // not relative to the hex centre): two neighbouring plains hexes must place
    // their blocks on the same grid, or the field betrays the tile boundary it is
    // supposed to hide.
    const STEP = lodStep(4);
    const x0 = Math.floor((cx - padS()) / STEP) * STEP, x1 = cx + padS();
    const y0 = Math.floor((cy - padS()) / STEP) * STEP, y1 = cy + padS();
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
    if (MARKRASTER) drawPlainsParcels(ctx, cx, cy);
    ctx.restore();
  }

  // Lapptäcket (Timothy 2026-10-10: princip 15 ger vika — slätten är ODLAD mark
  // och ska se brukad ut). Tegar om ~15×10 px i förskjutna rader, kanterna
  // lätt vågiga ur ett världsrymdsfält så att inget rutnät syns. Varje teg
  // drar en gröda ur sin hash: mörk gröda, ljus gröda, mogen säd, träda —
  // och basen själv som vanligaste val, så ytan fortfarande vilar. Fårorna
  // följer tegen; en mörk 1 px-ren skiljer tegarna åt. Ritas bara i
  // markrastret, där det bakas en gång.
  const PARCEL_W = 15, PARCEL_H = 10;

  // Tegen syns genom sin TON, inte genom en kontur: renarna (en prick varannan
  // pixel) läste som streckade staket och gjorde marken för kultiverad
  // (Timothy 2026-10-11). Utan dem flöt de tre trädesteg som bar bastonen ihop
  // med grannarna och fälten försvann — därför bara en träda av sju, och
  // plöjd jord i stället. Ljusa tegar högst två av sju: fler (en stubbton
  // prövades) lyfte slättens markton 139,5 → 148 och gav paret
  // plains ↔ olivlund 9,8 L (princip 17).
  const PARCEL_TONES = [
    null,
    ['#6A7834', 0.55],
    ['#7C7440', 0.50],
    ['#6A7834', 0.55],
    ['#9DAA5A', 0.55],
    ['#B7AA5C', 0.55],
    ['#A38C54', 0.60],
  ];

  // Odlingsgrad 0–1: full inom en stads upptagningsområde, avtagande ut till
  // ungefär fem hexar. Källorna är städerna spelaren känner till — samma data
  // kartan redan visar — så lapptäcket säger något sant: här bor folk.
  const CULT_NEAR = 55, CULT_FAR = 125;

  let cultSrc = [], cultSig = null;

  function cultivationList() {
    const sig = (scene.provinceData || []).map(p => p.q + ',' + p.r).join(';');
    if (sig !== cultSig) {
      cultSig = sig;
      cultSrc = (scene.provinceData || []).map(p => { const c = hexPx(p.q, p.r); return [c.x, c.y]; });
    }
    return cultSrc;
  }

  function cultivationSources(cx, cy) {
    const R = CULT_FAR + S + WARP_PAD;
    return cultivationList().filter(([x, y]) => Math.abs(x - cx) < R && Math.abs(y - cy) < R);
  }

  function cultivationAt(near, wx, wy) {
    let c = 0;
    for (const [x, y] of near) {
      const d = Math.hypot(wx - x, wy - y) + (noiseAt(wx, wy, 29, 3146) - 0.5) * 50;
      c = Math.max(c, Math.min(1, Math.max(0, (CULT_FAR - d) / (CULT_FAR - CULT_NEAR))));
    }
    return c;
  }

  function wildBush(wx, wy) {
    return noiseAt(wx, wy, 5, 6363) + 0.25 * (1 - noiseAt(wx, wy, 33, 6364)) > 0.965;
  }

  function plainsParcel(wx, wy) {
    const ox = (noiseAt(wx, wy, 37, 3141) - 0.5) * 12;
    const oy = (noiseAt(wx, wy, 37, 3142) - 0.5) * 8;
    const py = Math.floor((wy + oy) / PARCEL_H);
    // Bredd och förskjutning per rad, så tegarna inte murar sig som tegel.
    const rh = hash32(py, 0, 3144);
    const w = PARCEL_W - 5 + (rh % 13);
    const px = Math.floor((wx + ox + ((rh >>> 8) % w)) / w);
    return px * 65536 + py;
  }

  function drawPlainsParcels(ctx, cx, cy) {
    const STEP = 2;
    const near = cultivationSources(cx, cy);
    const x0 = Math.floor((cx - padS()) / STEP) * STEP, x1 = cx + padS();
    const y0 = Math.floor((cy - padS()) / STEP) * STEP, y1 = cy + padS();
    for (let wy = y0; wy <= y1; wy += STEP) {
      for (let wx = x0; wx <= x1; wx += STEP) {
        const id = plainsParcel(wx, wy);
        const h = hash32(id, 0, 3143);
        // Brukad bara där någon bor: tegen finns om dess dragning understiger
        // odlingsgraden här. Utanför är slätten vild — gräs och buskage.
        if (hash32(id, 0, 3145) / 4294967296 >= cultivationAt(near, wx, wy)) {
          if (wildBush(wx, wy)) {
            ctx.globalAlpha = 0.55;
            ctx.fillStyle = '#56652E';
            ctx.fillRect(wx, wy, STEP, STEP);
            if (!wildBush(wx, wy - STEP)) {
              ctx.globalAlpha = 0.6;
              ctx.fillStyle = '#A2AD68';
              ctx.fillRect(wx, wy, STEP, 1);
            }
          }
          continue;
        }
        const tone = PARCEL_TONES[h % PARCEL_TONES.length];
        if (tone) {
          ctx.globalAlpha = tone[1];
          ctx.fillStyle = tone[0];
          ctx.fillRect(wx, wy, STEP, STEP);
        }
        // Fåror: var tredje rad i tegen, bara på bruten jord.
        if (tone && ((wy >> 1) + (h >>> 8)) % 3 === 0) {
          ctx.globalAlpha = 0.16;
          ctx.fillStyle = '#3E4620';
          ctx.fillRect(wx, wy, STEP, 1);
        }
        // Ingen markerad ren mellan tegarna: en prick varannan pixel läste som
        // streckade linjer (Timothy 2026-10-11, "ska bort"). Tegens ton bär gränsen.
      }
    }
    ctx.globalAlpha = 1;
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
  const SCRUB_CELL  = 26;   // fuktfältet: svacka (tätt snår) och krön (bar jord), ett par snårlängder

  // MACCHIAN SOM LANDSKAP (Timothy 2026-10-11: "den behöver bli mer levande").
  // Det gamla fältet var en enda frekvens — mörka 3 px-block i en ton på platt
  // bas — och läste som kamouflage. Grafikprovet Timothy pekade på hade
  // enskilda buskar med egen skugga och varm jord emellan. Två lager, inte fler:
  //   1. bar, varm jord på de torra krönen (motsatsen till fuktfältet)
  //   2. buskar — pixelfigurer med ljus krona, mörk undersida och slagskugga mot
  //      sydost — på ett globalt jittrat rutnät. Tätheten följer fuktfältet:
  //      i svackorna står de så tätt att de växer ihop till snår, på krönen
  //      står de glest i jorden. Ritas uppifrån och ned så att de nedre
  //      överlappar de övre. Ett fåtal blommar (cistusrosa, ginstergul).
  // Ett första försök hade dessutom ett reliefskuggat block-lager och de gamla
  // snårblocken kvar under buskarna: tre lager i samma valörband läste som
  // grumligt korn — samma fel som princip 30 beskriver. Buskarna bär ytan själva.
  // Bastonen (markton 162, se ovan) är orörd.
  const SCRUB_GRID   = 5;   // högst en buske per 5×5-cell

  const SCRUB_TONES  = ['#4E5F33', '#5A6A3A', '#677442', '#737A48']; // kermesek, mastix, cistus, torr

  const SCRUB_SUN    = '#A9B676';

  const SCRUB_DARK   = '#3A4726';

  const SCRUB_SHADOW = '#4A4630';

  const SCRUB_EARTH  = '#C4A970'; // bar, varm jord

  const SCRUB_BLOOM  = ['#D890A4', '#E2C452'];

  // L sol · B kropp · D undersida · s slagskugga · . tomt
  const SCRUB_SHAPES = [
    ['.L.', 'LBD', '.Ds', '..s'],
    ['.LL.', 'LBBD', 'BBDD', '.DDs', '..ss'],
    ['.LLB.', 'LBBBB', 'BBBBD', 'BBBDD', '.DDDs', '..sss'],
    ['..LL..', '.LBBB.', 'LBBBBD', 'BBBBDD', '.BDDD.', '..sss.'],
  ];

  function drawScrubField(ctx, cx, cy) {
    ctx.save();
    hexClip(ctx, cx, cy);
    const step = lodStep(DRY_STEP);
    const x0 = Math.floor((cx - padS()) / step) * step, x1 = cx + padS();
    const y0 = Math.floor((cy - padS()) / step) * step, y1 = cy + padS();
    // 1. Bar jord, med per-block-brus i kanten (princip 16).
    ctx.fillStyle = SCRUB_EARTH;
    for (let wy = y0; wy <= y1; wy += step) {
      for (let wx = x0; wx <= x1; wx += step) {
        const dry = noiseAt(wx, wy, SCRUB_CELL, 6161) + (hash32(wx / step | 0, wy / step | 0, 6172) / 4294967296 - 0.5) * 0.10;
        if (dry < 0.62) continue;
        ctx.globalAlpha = Math.min(0.5, (dry - 0.62) * 2.2);
        ctx.fillRect(wx, wy, step, step);
      }
    }
    // 2. Buskarna.
    const G = SCRUB_GRID;
    for (let gy = Math.floor((cy - padS()) / G) - 2; gy <= Math.ceil((cy + padS()) / G); gy++) {
      for (let gx = Math.floor((cx - padS()) / G) - 2; gx <= Math.ceil((cx + padS()) / G); gx++) {
        const h = hash32(gx, gy, 6174);
        const x = gx * G + (h & 3), y = gy * G + ((h >>> 2) & 3);
        const wet = 1 - noiseAt(x, y, SCRUB_CELL, 6161);
        if ((h >>> 4) / 268435456 > -0.10 + 0.72 * wet) continue;
        const r = hash32(gx, gy, 6175);
        // Större buskar där det är fuktigt.
        const shape = SCRUB_SHAPES[Math.min(3, Math.floor((r % 1000) / 1000 * 2.2 + wet * 1.8))];
        const body = SCRUB_TONES[(r >>> 10) % SCRUB_TONES.length];
        const b = (r >>> 14) % 100;
        const bloom = shape.length > 4 && b < 3 ? SCRUB_BLOOM[b & 1] : null;
        for (let dy = 0; dy < shape.length; dy++) {
          const row = shape[dy];
          for (let dx = 0; dx < row.length; dx++) {
            const c = row[dx];
            if (c === '.') continue;
            if (c === 's') { ctx.globalAlpha = 0.40; ctx.fillStyle = SCRUB_SHADOW; }
            else { ctx.globalAlpha = 1; ctx.fillStyle = c === 'L' ? SCRUB_SUN : c === 'D' ? SCRUB_DARK : body; }
            ctx.fillRect(x + dx, y + dy, 1, 1);
          }
        }
        if (bloom) { ctx.globalAlpha = 1; ctx.fillStyle = bloom; ctx.fillRect(x + 2, y + 1, 1, 1); }
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
    hexClip(ctx, cx, cy);

    const step = lodStep(DRY_STEP);
    const x0 = Math.floor((cx - padS()) / step) * step, x1 = cx + padS();
    const y0 = Math.floor((cy - padS()) / step) * step, y1 = cy + padS();
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

  const CEDAR_LITTER = '#3E4C30'; // barrförna i skugga

  const CEDAR_MOSS   = '#5A6A40'; // mossa i en glänta

  const CEDAR_DUFF   = '#6A6A44'; // torr förna där taket öppnar sig

  // Marken är EN bärande frekvens (princip 41). Cederskogens golv syns knappt —
  // taket är slutet — så fältet är till för att bastonen inte ska vara platt där
  // den ändå skymtar, ingenting mer.
  function drawCedarFloor(ctx, cx, cy) {
    ctx.save();
    hexClip(ctx, cx, cy);
    const STEP = lodStep(3);
    const x0 = Math.floor((cx - padS()) / STEP) * STEP, x1 = cx + padS();
    const y0 = Math.floor((cy - padS()) / STEP) * STEP, y1 = cy + padS();
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
    const wd = riverEdges(q, r);
    let ax, ay;
    if (wd.length >= 2) { ax = DIR_NX[wd[0]] - DIR_NX[wd[1]]; ay = DIR_NY[wd[0]] - DIR_NY[wd[1]]; }
    else if (wd.length === 1) { ax = DIR_NX[wd[0]]; ay = DIR_NY[wd[0]]; }
    else { ax = 1; ay = 0; }
    const m = Math.hypot(ax, ay) || 1;
    return [ax / m, ay / m];
  }

  // Floden i markrastret (Timothy 2026-10-10: "floder och levande värld").
  // Samma graf som drawRiver — varje vattengranne ger en gren ut till den
  // delade kantens mittpunkt (princip 45) — men grenen är en KURVA: en kubisk
  // Bézier som lämnar kantmitten längs kantnormalen och når hexens mitt längs
  // flödesaxeln. Två grannhexar möter alltså varandra i samma punkt med samma
  // tangent, och kedjan blir en mjuk, obruten fåra. Mitten förskjuts lite
  // tvärs flödet ur hexens hash: det är meandern.
  //
  // Fåran är smalare än förut och vassen bara ett band. Utanför bandet ritas
  // INGENTING här — bakningen har redan lagt dalens fält under hela flodhexen,
  // så hexkanten mot grannens dal finns inte längre att se (princip 44: det var
  // vassen ut till kanten som ritade hexagonen).
  //
  // Vadstället (`river_ford`) är samma flod, grundare: ljusare vatten och en
  // rad stenar tvärs fåran — en passage som syns från kartan.
  const isChainTerrain = t => isWaterTerrain(t) || t === 'river_ford';
  // The server permits only these connection edges to cross the knowledge boundary.
  // Reading an edge never changes the neighbour's terrain or its fog mask.
  function riverEdges(q,r) {
    terrainAt(q,r);
    const declared=riverEdgeIndex.get(tileKey(q,r));
    if(declared)return [...new Set(declared.filter(i=>Number.isInteger(i)&&i>=0&&i<6))];
    return neighborDirs(q,r,isChainTerrain);
  }
  function hasRiverEdges(q,r) { terrainAt(q,r);return riverEdgeIndex.has(tileKey(q,r)); }

  function riverCurves(q, r) {
    const dirs = riverEdges(q,r);
    if (!dirs.length && !hasRiverEdges(q,r)) dirs.push(0, 3);
    let [axx, axy] = riverAxis(q, r);
    const m = (rnd(q, r, 2490) - 0.5) * 7;          // meander, px tvärs flödet
    const ccx = -axy * m, ccy = axx * m;
    const k = R_IN * 0.5;
    if(!dirs.length)return [[[ccx,ccy],[ccx,ccy]]];
    return dirs.map(i => {
      const nx = DIR_NX[i], ny = DIR_NY[i];
      const sgn = (nx * axx + ny * axy) >= 0 ? 1 : -1;
      // Havets kant ligger i den förvrängda masken, inte vid hexkantens mitt.
      // Fortsätt längs samma tangent till första havspixeln; grannens mitt är
      // alltid i sin egen region (WARP_AMP < R_IN).
      let reach = R_IN;
      const nt = terrainAt(q + HEX_DIRS[i][0], r + HEX_DIRS[i][1]);
      if (isSeaTerrain(nt)) {
        const { x, y } = hexPx(q, r);
        while (reach < 2 * R_IN && !isSeaTerrain(markTerrainAt(x + nx * reach, y + ny * reach))) reach++;
      }
      const p0 = [nx * reach, ny * reach], p1 = [nx * (R_IN - k), ny * (R_IN - k)];
      const p2 = [ccx + sgn * axx * k * 0.6, ccy + sgn * axy * k * 0.6], p3 = [ccx, ccy];
      const pts = [];
      for (let j = 0; j <= 12; j++) {
        const t = j / 12, u = 1 - t;
        pts.push([u*u*u*p0[0] + 3*u*u*t*p1[0] + 3*u*t*t*p2[0] + t*t*t*p3[0],
                  u*u*u*p0[1] + 3*u*u*t*p1[1] + 3*u*t*t*p2[1] + t*t*t*p3[1]]);
      }
      return pts;
    });
  }

  const RIVER_HALF_M = 5.5;

  function drawRiverCurve(ctx, cx, cy, q, r, isFord, ink = null) {
    ctx.save();
    hexClip(ctx, cx, cy);
    const curves = riverCurves(q, r);
    const [axx, axy] = riverAxis(q, r);
    const water = (isFord ? TERRAIN_BASE.river_ford : TERRAIN_BASE.river).c0;
    const STEP = 2;
    const x0 = Math.floor((cx - padS()) / STEP) * STEP, x1 = cx + padS();
    const y0 = Math.floor((cy - padS()) / STEP) * STEP, y1 = cy + padS();
    for (let wy = y0; wy <= y1; wy += STEP) {
      for (let wx = x0; wx <= x1; wx += STEP) {
        const dx = wx + STEP / 2 - cx, dy = wy + STEP / 2 - cy;
        let dist = Infinity;
        for (const pts of curves) {
          for (let j = 0; j < pts.length - 1; j++) {
            const ax = pts[j][0], ay = pts[j][1], bx = pts[j + 1][0] - ax, by = pts[j + 1][1] - ay;
            const L2 = bx * bx + by * by || 1;
            let t = ((dx - ax) * bx + (dy - ay) * by) / L2;
            t = t < 0 ? 0 : t > 1 ? 1 : t;
            const d = Math.hypot(dx - ax - t * bx, dy - ay - t * by);
            if (d < dist) dist = d;
          }
        }
        const half = RIVER_HALF_M + (noiseAt(wx, wy, RIVER_CELL, 2411) - 0.5) * 4.0;
        if (dist > half) {
          const reed = 3 + 3 * noiseAt(wx, wy, 11, 2412);
          if (isFord || dist > half + 2 + reed) continue;
          if (ink) ink.rect(wx, wy, STEP, STEP);
          ctx.globalAlpha = dist < half + 2 ? 0.9 : 0.55;
          ctx.fillStyle = dist < half + 2 ? RIVER_MUD : RIVER_REED;
          ctx.fillRect(wx, wy, STEP, STEP);
          continue;
        }
        if (ink) ink.rect(wx, wy, STEP, STEP);
        ctx.globalAlpha = 1;
        ctx.fillStyle = water;
        ctx.fillRect(wx, wy, STEP, STEP);
        if (isFord) {
          // Stenarna i vadet: en rad tvärs flödet, ljusa block med skuggsida.
          const along = dx * axx + dy * axy;
          if (Math.abs(along) < 4 && hash32(wx >> 1, wy >> 1, 2413) > 0x70000000) {
            ctx.fillStyle = hash32(wx, wy, 2414) & 1 ? '#C8B98C' : '#8E8164';
            ctx.fillRect(wx, wy, STEP, STEP);
          }
          continue;
        }
        const u = wx * axx + wy * axy, v = -wx * axy + wy * axx;
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

  function drawRiver(ctx, cx, cy, q, r, isFord = false) {
    if (MARKRASTER) { drawRiverCurve(ctx, cx, cy, q, r, isFord); return; }
    ctx.save();
    hexClip(ctx, cx, cy);

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
    for (const i of riverEdges(q,r)) segs.push([DIR_NX[i]*R_IN,DIR_NY[i]*R_IN]);
    // En flodhex utan vattengrannar kan inte finnas — en flod ÄR en kedja — men
    // en ensam flodhex i en riggfixtur får inte bli en solid vasslapp.
    if (!segs.length) { if(hasRiverEdges(q,r))segs.push([0,0]);else segs.push([R_IN, 0], [-R_IN, 0]); }
    const [axx, axy] = riverAxis(q, r);

    const STEP = 2;
    const x0 = Math.floor((cx - padS()) / STEP) * STEP, x1 = cx + padS();
    const y0 = Math.floor((cy - padS()) / STEP) * STEP, y1 = cy + padS();
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
    hexClip(ctx, cx, cy);

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
    const x0 = Math.floor((cx - padS()) / STEP) * STEP, x1 = cx + padS();
    const y0 = Math.floor((cy - padS()) / STEP) * STEP, y1 = cy + padS();
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

  const SHORE_SAND  = '#D6B264';  // referensens strandband, L 180

  const SHORE_WET   = '#B9924E';  // våt sand närmast vattnet — sandens skugga

  const SURF_FOAM   = '#A8C8D2';  // brytningen, referensens L 202 dämpad mot vårt mörkare hav

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

  // ── Markrastret ──────────────────────────────────────────────────────────
  // Varför: varje markpass klipptes mot sin hexpolygon, så varje gräns mellan
  // två terränger — kust, skogsbryn, slättens kant — var en sexhörning. Princip
  // 18: "samma yta med hexkanter läser som bricka". Det här är punkt 3b:s
  // "enda pass över det globala rutnätet där varje block slår upp sin hex",
  // med ett tillägg: uppslagningen sker i FÖRVRÄNGDA koordinater. Varje pixel
  // frågar "vilken hex ligger jag i?" en bit bredvid där den står, ur ett
  // världsrymdsfält. Gränserna blir organiska; hexens mitt tillhör alltid sin
  // egen hex (amplituden < inradien), så hexen anas men ritas inte.
  //
  // Ritfunktionerna är OFÖRÄNDRADE. De klipps bara mot sin förvrängda region
  // i stället för polygonen (`hexClip`, `BAKE.region`), och marken bakas en gång
  // till en cache i världsupplösning. Därmed ritas marken inte om varje frame,
  // och regionernas raka 1 px-spann ger noll kantutjämning på varje zoom.
  //
  // Flodens mark förvrängs liksom annan mark. Fåran följer kedjegrafen
  // (princip 45) i ett separat pass så att marken inte stryper den.
  //
  // Jämför: lägg till `markraster=0` i riggens URL för den gamla renderaren.

  const MARKRASTER = markraster;

  let BAKE = null;                // { region: Path2D } medan en markbit bakas

  const WARP_AMP = 13;            // max förskjutning; < R_IN (19) så hexmitten alltid är sin egen

  const WARP_PAD = WARP_AMP + 2;  // hur långt en region kan nå utanför polygonen

  const padS = () => (BAKE ? S + WARP_PAD : S);

  function hexClip(ctx, cx, cy) {
    if (BAKE) { ctx.clip(BAKE.region); return; }
    hexPath(ctx, hexPts(cx, cy));
    ctx.clip();
  }

  const isRiverTerrain = t => t === 'river' || t === 'river_ford';

  // [cell, ström] för warpens fyra brusfält: x bred, x smal, y bred, y smal.
  const WARP_NOISE = [[41, 8101], [13, 8102], [41, 8103], [13, 8104]];

  // nz/i: förberäknade brusvärden (noiseGrid, ordnade som WARP_NOISE) när
  // bakningen slår upp en hel bit; utan dem räknas bruset på plats.
  function warpOffset(wx, wy, nz, i) {
    // Två oktaver: en bred som ger kustens vikar och uddar, en smalare som
    // fransar dem. Plus en pixels hashbrus, så att gränsen blir blockig (princip
    // 16: per-block-brus, aldrig ordnad dither) i stället för en jämn kurva.
    const ax = nz ? 0.7 * nz[0][i] + 0.3 * nz[1][i] : 0.7 * noiseAt(wx, wy, 41, 8101) + 0.3 * noiseAt(wx, wy, 13, 8102);
    const ay = nz ? 0.7 * nz[2][i] + 0.3 * nz[3][i] : 0.7 * noiseAt(wx, wy, 41, 8103) + 0.3 * noiseAt(wx, wy, 13, 8104);
    const jx = hash32(wx | 0, wy | 0, 8105) / 4294967296 - 0.5;
    const jy = hash32(wx | 0, wy | 0, 8106) / 4294967296 - 0.5;
    return [(ax - 0.5) * 2.6 * WARP_AMP + jx * 1.6, (ay - 0.5) * 2.6 * WARP_AMP + jy * 1.6];
  }

  // Vilken hex en världspunkt HÖR TILL i markrastret.

  function markHexAt(wx, wy, nz, i) {
    const [eq, er] = hexAtWorld(wx, wy);
    const [dx, dy] = warpOffset(wx, wy, nz, i);
    let [q, r] = hexAtWorld(wx + Math.max(-WARP_AMP, Math.min(WARP_AMP, dx)),
                            wy + Math.max(-WARP_AMP, Math.min(WARP_AMP, dy)));
    const t = terrainAt(q, r);
    if (t === undefined) return [eq, er];
    return [q, r];
  }

  // noiseAt i varje punkt av rutnätet xs × ys — bitidentiskt med noiseAt (samma
  // aritmetik i samma ordning), men varje gitterhörn hashas en gång i stället
  // för fyra gånger per punkt. Hela bakningen gick från 38 till 24 ms per bit (helvyn 1,07 → 0,68 s).
  function noiseGrid(xs, ys, cell, stream) {
    const prep = v => {
      const g0 = Math.floor(v[0] / cell);
      const ci = new Int32Array(v.length), sv = new Float64Array(v.length);
      for (let k = 0; k < v.length; k++) {
        const g = Math.floor(v[k] / cell), f = v[k] / cell - g;
        ci[k] = g - g0; sv[k] = f * f * (3 - 2 * f);
      }
      return [g0, ci, sv, ci[v.length - 1] + 2];
    };
    const [gx0, cx, sx, gw] = prep(xs), [gy0, cy, sy, gh] = prep(ys);
    const lat = new Float64Array(gw * gh);
    for (let j = 0; j < gh; j++) for (let k = 0; k < gw; k++) lat[j * gw + k] = hash32(gx0 + k, gy0 + j, stream) / 4294967296;
    const out = new Float64Array(xs.length * ys.length);
    for (let y = 0; y < ys.length; y++) {
      const row = cy[y] * gw, t = sy[y];
      for (let x = 0; x < xs.length; x++) {
        const o = row + cx[x], u = sx[x];
        const a = lat[o], b = lat[o + 1], d = lat[o + gw], e = lat[o + gw + 1];
        out[y * xs.length + x] = (a + (b - a) * u) * (1 - t) + (d + (e - d) * u) * t;
      }
    }
    return out;
  }

  const markTerrainAt = (wx, wy) => { const [q, r] = markHexAt(wx, wy); return terrainAt(q, r); };

  let tileObjIndex = new Map(), tileObjFor;

  function tileByKey(k) {
    if (tileObjFor !== scene.tileData) {
      tileObjFor = scene.tileData;
      tileObjIndex = new Map(scene.tileData.map(t => [tileKey(t.q, t.r), t]));
    }
    return tileObjIndex.get(k);
  }

  const CHUNK = 256;        // världspixlar per cachebit

  const CHUNK_MARGIN = 12;  // grannskap som strand och skum läser över bitkanten

  // Kod per pixel för strand/skum/djup: 0 utanför · 1 fog · 2 land · 3 kusthav · 4 djuphav · 5 flod
  function markCode(t) {
    if (t === undefined) return 0;
    if (t === 'fog') return 1;
    if (t === 'coastal_sea') return 3;
    if (t === 'deep_sea') return 4;
    if (isRiverTerrain(t)) return 5;
    return 2;
  }

  // Avstånd (chamfer 3-4, i px) till närmaste källcell. Två svep räcker.
  // Utan en enda källcell (inlandsbit, öppet hav) är svaret oändligt överallt
  // och svepen hoppas över — de var bakningens dyraste enskilda steg.
  const CHAMFER_FAR = 1e6;

  function chamfer(src, w, h) {
    const d = new Float32Array(w * h);
    if (!src.includes(1)) return d.fill(CHAMFER_FAR);
    // Heltal under svepen (3/4-stegen är exakta), delat med 3 på slutet.
    const BIG = 1 << 30, a = new Int32Array(w * h);
    for (let i = 0; i < w * h; i++) a[i] = src[i] ? 0 : BIG;
    for (let y = 0; y < h; y++) for (let x = 0; x < w; x++) {
      const i = y * w + x; let v = a[i], u;
      if (x > 0 && (u = a[i - 1] + 3) < v) v = u;
      if (y > 0) {
        if ((u = a[i - w] + 3) < v) v = u;
        if (x > 0 && (u = a[i - w - 1] + 4) < v) v = u;
        if (x < w - 1 && (u = a[i - w + 1] + 4) < v) v = u;
      }
      a[i] = v;
    }
    for (let y = h - 1; y >= 0; y--) for (let x = w - 1; x >= 0; x--) {
      const i = y * w + x; let v = a[i], u;
      if (x < w - 1 && (u = a[i + 1] + 3) < v) v = u;
      if (y < h - 1) {
        if ((u = a[i + w] + 3) < v) v = u;
        if (x < w - 1 && (u = a[i + w + 1] + 4) < v) v = u;
        if (x > 0 && (u = a[i + w - 1] + 4) < v) v = u;
      }
      a[i] = v;
    }
    for (let i = 0; i < w * h; i++) d[i] = a[i] / 3;
    return d;
  }

  const hexRGB = h => [parseInt(h.slice(1, 3), 16), parseInt(h.slice(3, 5), 16), parseInt(h.slice(5, 7), 16)];

  const RGB_SAND = hexRGB(SHORE_SAND), RGB_WET = hexRGB(SHORE_WET), RGB_FOAM = hexRGB(SURF_FOAM);

  const RGB_SHALLOW = hexRGB('#5C9AB2'), RGB_SHELF = hexRGB('#3E7C9E');

  const RGB_HATCH = hexRGB('#40362A');

  // Fårans kedja är fysisk; bara marken följer den förvrängda uppslagningen.
  // Klipp fåran mot sin exakta hex och havsgrannarnas exakta hexar, men aldrig
  // över den förvrängda havsytan eller okänd mark. Havsgrenens kurva avgör hur
  // mycket av grannhexen som faktiskt får bläck.

  function riverInkAt(q, r, wx, wy) {
    const t = markTerrainAt(wx, wy);
    const [eq, er] = hexAtWorld(wx, wy);
    if (eq === q && er === r && t === 'fog' && hasRiverEdges(q,r) && riverEdges(q,r).length) return true;
    if (t === undefined || t === 'fog' || isSeaTerrain(t)) return false;
    if (eq === q && er === r) return true;
    return HEX_DIRS.some(([dq, dr]) => eq === q + dq && er === r + dr && isSeaTerrain(terrainAt(eq, er)));
  }

  function bakeChunk(ix, iy) {
    const X0 = ix * CHUNK - 1, Y0 = iy * CHUNK - 1, CS = CHUNK + 2;   // 1 px överlapp mot sömmar
    const M = CHUNK_MARGIN, W = CS + 2 * M;
    const lab = new Float64Array(W * W);   // tileKey + 1, 0 = ingen hex
    const code = new Uint8Array(W * W);
    const xs = new Float64Array(W), ys = new Float64Array(W);
    for (let k = 0; k < W; k++) { xs[k] = X0 - M + k + 0.5; ys[k] = Y0 - M + k + 0.5; }
    const nz = WARP_NOISE.map(([cell, stream]) => noiseGrid(xs, ys, cell, stream));
    for (let y = 0; y < W; y++) {
      for (let x = 0; x < W; x++) {
        const i = y * W + x;
        const [q, r] = markHexAt(xs[x], ys[y], nz, i);
        const t = terrainAt(q, r);
        code[i] = markCode(t);
        lab[i] = t === undefined ? 0 : tileKey(q, r) + 1;
      }
    }
    const cv = createCanvas();
    cv.width = CS; cv.height = CS;
    const c = cv.getContext('2d');
    c.translate(-X0, -Y0);
    // Regioner som spann om 1 px höjd — exakt, ingen kantutjämning.
    const regions = new Map();
    for (let y = 0; y < CS; y++) {
      let x = 0;
      while (x < CS) {
        const k = lab[(y + M) * W + x + M];
        let e = x + 1;
        while (e < CS && lab[(y + M) * W + e + M] === k) e++;
        if (k) {
          let p = regions.get(k);
          if (!p) regions.set(k, p = new Path2D());
          p.rect(X0 + x, Y0 + y, e - x, 1);
        }
        x = e;
      }
    }
    const tiles = [...regions.keys()].map(k => [tileByKey(k - 1), regions.get(k)]).filter(([t]) => t);
    for (const [t, path] of tiles) {
      // En flodhex står på dalens mark; fåran ritas ovanpå (drawRiverCurve).
      c.fillStyle = (TERRAIN_BASE[isRiverTerrain(t.terrain) ? 'river_valley' : t.terrain] || TERRAIN_BASE.fog).c0;
      c.fill(path);
    }
    for (const [t, path] of tiles) {
      if (t.terrain === 'fog') continue;
      const { x, y } = hexPx(t.q, t.r);
      BAKE = { region: path };
      try {
        if (isRiverTerrain(t.terrain)) drawValleyField(c, x, y, t.q, t.r, false);
        else drawDetail(c, x, y, t.terrain, (t.q * 137 + t.r * 31) & 0xff, t.q, t.r);
      } finally { BAKE = null; }
    }
    // Kusten ur samma mask: strand på land, skum och grundvatten på havet,
    // hyllan där kusthavet möter djupet. En gräns, ritad en gång.
    const isSea = new Uint8Array(W * W), isLand = new Uint8Array(W * W), isShelf = new Uint8Array(W * W);
    for (let i = 0; i < W * W; i++) {
      isSea[i] = code[i] === 3 || code[i] === 4;
      isLand[i] = code[i] === 2;
      isShelf[i] = code[i] === 3;
    }
    const dSea = chamfer(isSea, W, W), dLand = chamfer(isLand, W, W), dShelf = chamfer(isShelf, W, W);
    const img = c.getImageData(0, 0, CS, CS), px = img.data;
    const put = (o, rgb, a) => {
      px[o] += (rgb[0] - px[o]) * a; px[o + 1] += (rgb[1] - px[o + 1]) * a; px[o + 2] += (rgb[2] - px[o + 2]) * a;
    };
    for (let y = 0; y < CS; y++) {
      for (let x = 0; x < CS; x++) {
        const i = (y + M) * W + x + M, o = (y * CS + x) * 4;
        const wx = X0 + x, wy = Y0 + y, k = code[i];
        if (k === 1) {
          // Aldrig sett: skraffering, så okänt aldrig kan läsas som djuphav
          // (dimmans register — kunskap och geografi ska inte förväxlas).
          if ((((wx + wy) % 7) + 7) % 7 === 0) put(o, RGB_HATCH, 1);
        } else if (k === 2) {
          // Bruset räknas bara där avståndet kan klara sin tröskel (max 5,6 /
          // 12 / 10) — samma utfall, en bråkdel av anropen.
          const d = dSea[i];
          if (d > 5.6) continue;
          const w = 1.2 + 4.4 * noiseAt(wx, wy, 9, 6161);
          if (d <= w) put(o, d > w * 0.62 ? RGB_SAND : (d <= 1.01 ? RGB_WET : RGB_SAND), 1);
        } else if (k === 3 || k === 4) {
          const d = dLand[i];
          if (d <= 1.01) put(o, RGB_FOAM, 0.75);
          else if (k === 3 && d <= 12 && d <= 7 + 5 * noiseAt(wx, wy, 17, 7173)) put(o, RGB_SHALLOW, 0.45 * (1 - d / 13));
          if (k === 4 && dShelf[i] <= 10) {
            const s = dShelf[i], w = 1 + 9 * noiseAt(wx, wy, 15, 6262);
            if (s <= w * 0.5 || (s <= w && hash32(wx, wy, 6263) > 0x8ccccccc)) put(o, RGB_SHELF, 1);
          }
        }
      }
    }
    c.putImageData(img, 0, 0);
    // Rita fårorna sist så att varken grannens mark eller strandpasset stryper
    // kedjan. Heltalsspann ger samma hårda pixelklipp som markens regioner.
    const riverPaths = [];
    for (const t of scene.tileData) {
      if (!isRiverTerrain(t.terrain)) continue;
      const { x: cx, y: cy } = hexPx(t.q, t.r), pad = S + WARP_PAD;
      const x0 = Math.max(X0, Math.floor(cx - pad)), x1 = Math.min(X0 + CS, Math.ceil(cx + pad));
      const y0 = Math.max(Y0, Math.floor(cy - pad)), y1 = Math.min(Y0 + CS, Math.ceil(cy + pad));
      if (x0 >= x1 || y0 >= y1) continue;
      const path = new Path2D();
      for (let y = y0; y < y1; y++) {
        let x = x0;
        while (x < x1) {
          if (!riverInkAt(t.q, t.r, x + 0.5, y + 0.5)) { x++; continue; }
          const start = x++;
          while (x < x1 && riverInkAt(t.q, t.r, x + 0.5, y + 0.5)) x++;
          path.rect(start, y, x - start, 1);
        }
      }
      const ink = t.tier === 'remembered' ? new Path2D() : null;
      BAKE = { region: path };
      try { drawRiverCurve(c, cx, cy, t.q, t.r, t.terrain === 'river_ford', ink); }
      finally { BAKE = null; }
      if (ink) riverPaths.push([path, ink]);
    }
    // Bränningen är animerad och ritas live (drawMarkSurf); här samlas bara
    // de 2 px-block i havet som ligger inom räckhåll från land, med avstånd
    // och fasbrus, så att livepasset inte behöver räkna något.
    const surf = [];
    for (let y = 0; y < CS; y++) {
      const wy = Y0 + y;
      if (wy & 1) continue;
      for (let x = 0; x < CS; x++) {
        const wx = X0 + x;
        if (wx & 1) continue;
        const i = (y + M) * W + x + M, k = code[i];
        if (k !== 3 && k !== 4) continue;
        const d = dLand[i];
        if (d <= 1.01 || d > 7) continue;
        surf.push(wx, wy, d, noiseAt(wx, wy, 23, 7171) * 5);
      }
    }
    // Minnesmasken: grått där regionen hör till en minnesruta. Läggs på med
    // 'saturation' efter terrängen, precis som drawRemembered — men i samma
    // form som marken, inte som hexpolygoner.
    let mem = null;
    const memTiles = tiles.filter(([t]) => t.tier === 'remembered' && t.terrain !== 'fog');
    if (memTiles.length || riverPaths.length) {
      mem = createCanvas();
      mem.width = CS; mem.height = CS;
      const mc = mem.getContext('2d');
      mc.translate(-X0, -Y0);
      mc.fillStyle = '#808080';
      for (const [, path] of memTiles) mc.fill(path);
      for (const [path, ink] of riverPaths) {
        mc.save(); mc.clip(path); mc.fill(ink); mc.restore();
      }
    }
    return { img: cv, mem, X0, Y0, code, W, M, surf: new Float32Array(surf) };
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
    if (BAKE) return 1e9;   // markrastret: regionens klipp gör jobbet
    return S - Math.abs(dy) / Math.sqrt(3);
  }

  function reliefGrid(cx, cy, step, cell, stream, steps, ridged) {
    const x0 = Math.floor((cx - padS()) / step) * step - step;
    const y0 = Math.floor((cy - padS()) / step) * step - step;
    const nx = Math.ceil((2 * padS()) / step) + 3, ny = Math.ceil((2 * padS()) / step) + 3;
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

  function ridgeAt(wx, wy, cell, stream) {
    const n = noiseAt(wx, wy, cell, stream);
    return 1 - Math.abs(2 * n - 1);
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

  // ── Terrain detail — Settlers 2 quality ──────────────────────────────────
  // `frame` togs bort med havsshimret: efter att dyningen flyttat till sitt eget
  // pass finns ingen animerad MARK kvar, och en frame-parameter som ingen läser
  // antyder att marken kan röra sig.
  function drawDetail(ctx, cx, cy, terrain, seed, q, r) {
    ctx.save();
    if (BAKE) ctx.clip(BAKE.region);
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
      case 'river_ford': {
        if (MARKRASTER) drawRiver(ctx, cx, cy, q, r, true);
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
  return {
    S, HEX_DIRS, CULT_FAR, HILL_RIM, HILL_HIGH, HILL_LIT, HILL_LOW, HILL_HOLLOW, hexPx, SCALE,
    hexPath, SQRT3, DIR_NX, DIR_NY, openEdges, rndInt, MARKRASTER, rnd, neighborDirs, hexPts,
    terrainAt, TERRAIN_BASE, R_IN, noiseAt, hash32, SHORE_WET, SHORE_SAND, SURF_FOAM,
    markTerrainAt, CHUNK, markCode, cultivationList, salt, bakeChunk, tileKey, WARP_PAD,
    hexAtWorld, markHexAt, isSeaTerrain, ridgeAt, MTN_ROCK, drawDetail, isWaterTerrain,
    riverCurves, riverInkAt,
  };
}
