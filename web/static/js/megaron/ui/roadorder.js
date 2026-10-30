// Build Road (megaron_plan_vagar.md V2) — the pure half of the War → Army order:
// which neighbouring hexes may a road reach from where the army stands, and the
// panel that lists them. The server judges everything (terrain, stone, route);
// this only avoids offering a hex the map already shows cannot carry a road, and
// never states a stone price of its own — the answer or the refusal does.
import { esc } from './format.js';

// Terrain a road edge may touch at BOTH ends (server: province.IsRoadableTerrain).
// Rivers, fords, sea and mountains never carry a road.
const ROADABLE_TERRAIN = [
  'plains', 'river_valley', 'river_delta', 'forest_olive_grove',
  'scrub_maquis', 'hills', 'forest_cedar', 'semi_desert',
];
const DIRS = [[1, 0], [-1, 0], [0, 1], [0, -1], [1, -1], [-1, 1]];

const edgeKey = (a, b) => {
  const [p, q] = (a.q < b.q || (a.q === b.q && a.r <= b.r)) ? [a, b] : [b, a];
  return `${p.q},${p.r},${q.q},${q.r}`;
};

// roadNeighbours lists the neighbours of `from` a road could reach: roadable
// terrain at both ends, on the map, and no road there already.
export function roadNeighbours(from, tiles, edges) {
  const terrain = new Map();
  for (const t of tiles || []) terrain.set(`${t.q},${t.r}`, t.terrain);
  if (!ROADABLE_TERRAIN.includes(terrain.get(`${from.q},${from.r}`))) return [];
  const built = new Set((edges || []).map(e => edgeKey({ q: e.q1, r: e.r1 }, { q: e.q2, r: e.r2 })));
  const out = [];
  for (const [dq, dr] of DIRS) {
    const to = { q: from.q + dq, r: from.r + dr };
    const t = terrain.get(`${to.q},${to.r}`);
    if (!ROADABLE_TERRAIN.includes(t)) continue;
    if (built.has(edgeKey(from, to))) continue;
    out.push({ q: to.q, r: to.r, terrain: t });
  }
  return out;
}

function terrainWords(t) { return String(t || '').replace(/_/g, ' '); }

// roadPanelHTML is the hidden order row under a unit card. `from` is the hex the
// army builds from (its field hex, or its city's hex).
export function roadPanelHTML(unitID, from, tiles, edges) {
  const opts = roadNeighbours(from, tiles, edges);
  const id = esc(String(unitID));
  if (!opts.length) {
    return '<div id="uroad-' + id + '" style="display:none;margin-top:.2rem;font-size:.65rem;color:var(--text-dim)">'
      + 'No neighbouring hex of (' + from.q + ',' + from.r + ') can take a new road edge.</div>';
  }
  const options = opts.map(o =>
    '<option value="' + o.q + ',' + o.r + '">(' + o.q + ',' + o.r + ') ' + esc(terrainWords(o.terrain)) + '</option>').join('');
  return '<div id="uroad-' + id + '" data-from="' + from.q + ',' + from.r + '" style="display:none;margin-top:.2rem;font-size:.65rem;color:var(--text-dim)">'
    + '<div>One road edge from (' + from.q + ',' + from.r + '). The stone leaves your nearest city now and travels as a caravan; building begins when it and the army are both at the first hex.</div>'
    + '<div style="margin-top:.2rem"><label>to <select id="uroad-to-' + id + '" style="font-size:.65rem;padding:.1rem;border:1px solid var(--border);background:var(--warm-white)">' + options + '</select></label> '
    + '<button onclick="unitBuildRoad(\'' + id + '\')" style="padding:.1rem .3rem;border:1px solid var(--border);background:var(--accent-war);color:#fff;font-size:.65rem;cursor:pointer">Send order →</button></div>'
    + '<div id="uroad-res-' + id + '" style="margin-top:.15rem"></div>'
    + '</div>';
}

// parseHex reads the "q,r" the select carries.
export function parseHex(v) {
  const m = /^(-?\d+),(-?\d+)$/.exec(String(v || ''));
  return m ? { q: Number(m[1]), r: Number(m[2]) } : null;
}

export function roadOrderBody(from, toValue) {
  const to = parseHex(toValue);
  if (!from || !to) return null;
  return { from_q: from.q, from_r: from.r, to_q: to.q, to_r: to.r };
}
