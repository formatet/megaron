import { State } from './state.js';
import { fetchAuth } from './api.js';
import { omenMap } from './render/omen.js';

// ETag of the map body State.tileData was built from, per world. Every accepted
// response records it, so a poll's 304 always means "what you hold is current".
let held = { world: null, etag: null };

// All map responses share the pending omen mask, including naming and polls.
export function acceptMapTiles(tiles, etag = null) {
  held = { world: State.WORLD_ID, etag };
  if (State.omen) {
    State.omen.latest = tiles;
    State.tileData = omenMap(State.omen, tiles, performance.now());
  } else State.tileData = tiles;
  State.dirty = true;
}

// The poll asks conditionally. On 304 the tile array keeps its identity, so the
// raster caches keyed on it are not rebuilt every few seconds for nothing.
export async function pollMapTiles(world) {
  const headers = held.world === world && held.etag ? { 'If-None-Match': held.etag } : {};
  const res = await fetchAuth(`/api/v1/worlds/${world}/map`, { headers });
  if (!res.ok) return;
  const tiles = await res.json();
  if (State.WORLD_ID === world) acceptMapTiles(tiles, res.headers.get('ETag'));
}
