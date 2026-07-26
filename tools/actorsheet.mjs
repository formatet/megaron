#!/usr/bin/env node
// Kontaktark över kartaktörerna — pixelkonstens iterationsloop.
//
//     node tools/actorsheet.mjs [utfil.png]
//
// Ritar varje aktör förstorad mot de tre terrängerna, med en 1:1-ruta bredvid
// och en hexram i rätt storlek (44×38 logiska px) så proportionen mot hexen går
// att se direkt. Ingen browser, ingen server, ingen deploy per iteration.
//
// Skriver PPM till ImageMagick. Node kan importera renderarens sprite-modul
// direkt, så arket kan inte glida isär från vad kartan faktiskt ritar.
import { execFileSync } from 'node:child_process';
import { ACTOR_PALETTE, ACTOR_SPRITES, NEUTRAL_ACCENT } from
  '../web/static/js/megaron/render/actorsprites.js';

const TERRAINS = [
  ['slätt',    '#859248'],
  ['olivlund', '#9EA361'],
  ['kalksten', '#E0D4B8'],
];
const ZOOM = 6;
const HEX_W = 44, HEX_H = 38;   // hexens logiska mått — proportionsreferensen
const PAD = 4;

const names = Object.keys(ACTOR_SPRITES);
const cellW = HEX_W + PAD * 2;
const cellH = HEX_H + PAD * 2;

// Bilden: en rad per aktör, tre terrängkolumner, varje ruta ZOOM× förstorad,
// plus en 1:1-remsa längst till höger.
const W = TERRAINS.length * cellW * ZOOM + cellW * TERRAINS.length;
const H = names.length * cellH * ZOOM;
const buf = Buffer.alloc(W * H * 3);

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

names.forEach((name, row) => {
  const oy = row * cellH * ZOOM;
  TERRAINS.forEach(([, bg], col) => {
    const ox = col * cellW * ZOOM;
    put(ox, oy, rgb(bg), cellW * ZOOM, cellH * ZOOM);
    // Hexramen som proportionsreferens: aktören ska vara ca 40 % av dess höjd.
    const fr = rgb('#00000040' ? '#3A3226' : '#3A3226');
    put(ox + PAD * ZOOM, oy + PAD * ZOOM, fr, HEX_W * ZOOM, ZOOM);
    put(ox + PAD * ZOOM, oy + (PAD + HEX_H) * ZOOM - ZOOM, fr, HEX_W * ZOOM, ZOOM);

    const s = ACTOR_SPRITES[name];
    // Foten i hexcentrum, precis som drawActor gör det.
    const bx = PAD + (HEX_W >> 1) - (s.w >> 1);
    const by = PAD + (HEX_H >> 1) - s.h + 2;
    for (const r of s.runs) {
      const c = rgb(r.ch === 'A' ? NEUTRAL_ACCENT : ACTOR_PALETTE[r.ch]);
      put(ox + (bx + r.x) * ZOOM, oy + (by + r.y) * ZOOM, c, r.n * ZOOM, ZOOM);
    }
    // 1:1-remsan: samma aktör i sin verkliga storlek, en gång per terräng.
    const sx = TERRAINS.length * cellW * ZOOM + col * cellW;
    put(sx, oy, rgb(bg), cellW, cellH * ZOOM);
    for (const r of s.runs) {
      const c = rgb(r.ch === 'A' ? NEUTRAL_ACCENT : ACTOR_PALETTE[r.ch]);
      put(sx + bx + r.x, oy + by + r.y + PAD, c, r.n, 1);
    }
  });
});

const out = process.argv[2] || 'actorsheet.png';
execFileSync('magick', ['ppm:-', out],
  { input: Buffer.concat([Buffer.from(`P6\n${W} ${H}\n255\n`), buf]) });

// --dump <katalog>: en 1:1-PNG per aktör, för tools/discriminability.py.
// Acceptanskriteriet "Spearmen, Elite Infantry och War Chariot är parvis
// distinkta vid 14 px" är en MÄTNING, inte en åsikt — den ska gå att köra.
const dumpIdx = process.argv.indexOf('--dump');
if (dumpIdx > -1 && process.argv[dumpIdx + 1]) {
  const dir = process.argv[dumpIdx + 1];
  for (const name of names) {
    const s = ACTOR_SPRITES[name];
    const px = Buffer.alloc(s.w * s.h * 3, 255);
    for (const r of s.runs) {
      // Silhuett, inte färg: mätningen frågar om FORMEN går att skilja åt.
      for (let i = 0; i < r.n; i++) {
        const o = ((r.y) * s.w + r.x + i) * 3;
        px[o] = px[o + 1] = px[o + 2] = 0;
      }
    }
    execFileSync('magick', ['ppm:-', `${dir}/${name}.png`],
      { input: Buffer.concat([Buffer.from(`P6\n${s.w} ${s.h}\n255\n`), px]) });
  }
  console.log(`dumpade ${names.length} siluetter till ${dir}/`);
}

console.log(`${out}  ${W}×${H}  ${names.length} aktörer`);
console.log(names.map(n => `${n} ${ACTOR_SPRITES[n].w}×${ACTOR_SPRITES[n].h}`).join(' · '));
