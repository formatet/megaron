import fs from 'node:fs';
import { buildCityScene } from '../../../web/static/js/megaron/render/city.js';
const dir = new URL('./', import.meta.url);
const buildings = [
  { type: 'temple', level: 1 },
  { type: 'farm', level: 2 },
  { type: 'harbour', level: 1 },
  { type: 'shipyard', level: 1 },
  { type: 'mine', level: 1 }
];
const scenes = [];
for (const population of [1000, 4500, 10000])
  for (const state of ['normal', 'construction', 'siege', 'starvation']) {
    const sett = {
      population,
      walls: 1,
      buildings,
      build_queue: state === 'construction' ? [{ type: 'temple' }] : [],
      besieged: state === 'siege',
      food: { growth_state: state === 'starvation' ? 'shrinking' : 'growing' }
    };
    scenes.push({
      name: `fixture-${population}-${state}`,
      label: `Fixtur ${population} · ${state} · mur 1`,
      group: 'fixture',
      tile: { terrain: 'hills' },
      sett
    });
  }
for (const walls of [0, 2, 3])
  scenes.push({
    name: `walls-${walls}`,
    label: `Fixtur 4500 · mur ${walls}`,
    group: 'walls',
    tile: { terrain: 'hills' },
    sett: { population: 4500, walls, buildings, build_queue: [] }
  });
for (const r of JSON.parse(fs.readFileSync(new URL('dev-snapshots.json', dir))))
  scenes.push({
    name: `dev-${r.name}`,
    label: `Dev ${r.name} · ${r.payload.settlement.population} · mur ${r.payload.settlement.walls}`,
    group: 'dev',
    tile: { terrain: r.payload.terrain_type },
    sett: r.payload.settlement,
    health: r.health,
    captured_at: r.captured_at
  });
fs.writeFileSync(new URL('inputs.json', dir), JSON.stringify(scenes, null, 2) + '\n');
const result = scenes.map((s) => ({
  ...s,
  raster: buildCityScene(s.tile, s.sett.buildings, s.sett.build_queue, s.sett)
}));
for (const s of result) s.raster.housing = Array.from(s.raster.housing);
fs.writeFileSync(new URL('scenes-raster.json', dir), JSON.stringify(result));
console.log(
  result.map((s) => [s.name, s.raster.terraceLevels, s.raster.housing.reduce((a, b) => a + b, 0)])
);
