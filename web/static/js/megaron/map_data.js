import { State } from './state.js';
import { omenMap } from './render/omen.js';

// All map responses share the pending omen mask, including naming and polls.
export function acceptMapTiles(tiles) {
  if (State.omen) {
    State.omen.latest = tiles;
    State.tileData = omenMap(State.omen, tiles, performance.now());
  } else State.tileData = tiles;
  State.dirty = true;
}
