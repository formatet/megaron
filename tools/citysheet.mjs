#!/usr/bin/env node
// Kontaktark över stadsleden — pixelkonstens iterationsloop.
//
//     node tools/citysheet.mjs [utfil.png] [--zoom N] [--terrain slätt|olivlund|kalksten]
//
// Rutnätet är 4 led (rader) × 4 murnivåer (kolumner), var och en med hexen
// (44×38 logiska px) utritad och en Spearmen-formation bredvid som
// SKALREFERENS: en stad måste vara större än en armé, och det går bara att
// bedöma i samma bild. Ingen browser, ingen server, ingen deploy per iteration.
//
// **Bedöm alltid också 1:1** (`--zoom 1`, eller remsan till höger). Förstoringen
// ljuger: fyra iterationer i rad såg bra ut vid 5× och läste som en grå klump på
// kartan. Det som räknas är hur märket ser ut i den storlek det faktiskt får.
import { execFileSync } from 'node:child_process';
import { CITY_PALETTE, CITY_SPRITES, CITY_BASE_OFFSET } from
  '../web/static/js/megaron/render/citysprites.js';
import { ACTOR_PALETTE, ACTOR_SPRITES, NEUTRAL_ACCENT } from
  '../web/static/js/megaron/render/actorsprites.js';

const argv = process.argv.slice(2);
const flag = (name, def) => {
  const i = argv.indexOf(name);
  return i > -1 ? argv[i + 1] : def;
};
const TERRAIN = { 'slätt': '#859248', 'olivlund': '#9EA361', 'kalksten': '#E0D4B8' };
const bgName = flag('--terrain', 'slätt');
const bg = TERRAIN[bgName] || TERRAIN['slätt'];
const ZOOM = Number(flag('--zoom', 5));

const HEX_W = 44, HEX_H = 38, PAD = 6;
const CELL_W = HEX_W + PAD * 2, CELL_H = HEX_H + PAD * 2;
const COLS = 4, ROWS = CITY_SPRITES.length;

// Förstorat rutnät + en 1:1-remsa längst till höger.
const W = COLS * CELL_W * ZOOM + COLS * CELL_W;
const H = ROWS * CELL_H * ZOOM;
const buf = Buffer.alloc(W * H * 3, 0x18);

const rgb = hex => [1, 3, 5].map(i => parseInt(hex.slice(i, i + 2), 16));

function put(x, y, colour, w = 1, h = 1) {
  const [r, g, b] = colour;
  for (let yy = y; yy < y + h; yy++) {
    if (yy < 0 || yy >= H) continue;
    for (let xx = x; xx < x + w; xx++) {
      if (xx < 0 || xx >= W) continue;
      const i = (yy * W + xx) * 3;
      buf[i] = r; buf[i + 1] = g; buf[i + 2] = b;
    }
  }
}

// Hexen ritad som riktig sexhörning (platt topp, S=22) — en rektangulär ram
// räcker inte, för frågan är just hur mycket massan svämmar över KANTEN.
function hexEdge(px, py, zoom, colour) {
  const S = HEX_W / 2, cx = PAD + HEX_W / 2, cy = PAD + HEX_H / 2, pts = [];
  for (let i = 0; i < 6; i++) {
    const a = Math.PI / 3 * i;
    pts.push([cx + S * Math.cos(a), cy + S * Math.sin(a)]);
  }
  for (let i = 0; i < 6; i++) {
    const [x0, y0] = pts[i], [x1, y1] = pts[(i + 1) % 6];
    const n = Math.ceil(Math.max(Math.abs(x1 - x0), Math.abs(y1 - y0)) * zoom);
    for (let k = 0; k <= n; k++)
      put(px + Math.round((x0 + (x1 - x0) * k / n) * zoom),
          py + Math.round((y0 + (y1 - y0) * k / n) * zoom), colour, zoom, zoom);
  }
}

function blit(sprite, palette, px, py, bx, by, zoom, accent) {
  for (const r of sprite.runs) {
    const c = rgb(r.ch === 'A' ? accent : palette[r.ch]);
    put(px + (bx + r.x) * zoom, py + (by + r.y) * zoom, c, r.n * zoom, zoom);
  }
}

const actor = ACTOR_SPRITES.spearman;
CITY_SPRITES.forEach((byWall, row) => {
  const oy = row * CELL_H * ZOOM;
  byWall.forEach((s, col) => {
    const bx = PAD + (HEX_W >> 1) - (s.w >> 1);
    const by = PAD + (HEX_H >> 1) + CITY_BASE_OFFSET - s.h;
    const abx = PAD + HEX_W - 8 - (actor.w >> 1);
    const aby = PAD + (HEX_H >> 1) + 8 - actor.h + 2;

    const ox = col * CELL_W * ZOOM;
    put(ox, oy, rgb(bg), CELL_W * ZOOM, CELL_H * ZOOM);
    hexEdge(ox, oy, ZOOM, rgb('#3A3226'));
    blit(s, CITY_PALETTE, ox, oy, bx, by, ZOOM);
    blit(actor, ACTOR_PALETTE, ox, oy, abx, aby, ZOOM, NEUTRAL_ACCENT);

    // 1:1-remsan — så stort märket faktiskt blir på kartan.
    const sx = COLS * CELL_W * ZOOM + col * CELL_W;
    put(sx, oy, rgb(bg), CELL_W, CELL_H * ZOOM);
    blit(s, CITY_PALETTE, sx, oy + PAD, bx, by, 1);
    blit(actor, ACTOR_PALETTE, sx, oy + PAD, abx, aby, 1, NEUTRAL_ACCENT);
  });
});

const out = argv.find(a => a.endsWith('.png')) || 'citysheet.png';
execFileSync('magick', ['ppm:-', out],
  { input: Buffer.concat([Buffer.from(`P6\n${W} ${H}\n255\n`), buf]) });

const LED = ['hamlet', 'town', 'city', 'anaktoron'];
console.log(`${out}  ${W}×${H}  ${bgName}  ${ZOOM}×`);
console.log(CITY_SPRITES.map((byWall, i) =>
  `${LED[i]} ${byWall.map(s => `${s.w}×${s.h}`).join('/')}`).join('  ·  ')
  + `   (hex ${HEX_W}×${HEX_H})`);
