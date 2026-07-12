// Shared mutable state for the map client. Every module that used to read or
// reassign one of these top-level `let`/`const` bindings in the old single
// <script> now goes through this one object instead — a bare top-level
// binding can only be reassigned by the module that declared it, but a
// property on a shared object can be mutated from anywhere, which is what
// the old single-scope script relied on implicitly.
//
// Named `State`, not `S` — `S` is already the hex-size constant in
// render/map.js (`const S = 22`, present in the original script before this
// split existed); reusing it here would have collided with that.
export const State = {
  // Bootstrap values, populated by main.js's bootstrap() before any other
  // module's init() runs (see main.js for the fetch sequence + ordering).
  WORLD_ID: null,
  MY_SETTLEMENT_ID: null,
  MY_PLAYER_ID: null,
  WORLD_CREATED_AT: null,
  TIME_SCALE: 1,

  // Server-fetched map data (render/map.js loadMap()/refreshTiles() and the
  // WebSocket handler in ws.js keep these current).
  tileData: [],
  provinceData: [],
  marchData: [],
  messengerData: [],
  tradeData: [],
  unitsData: [],  // per-unit armies/fleets (units table) — drawn on the canvas

  // Canvas camera + interaction state (render/map.js).
  camera: { x: 0, y: 0, zoom: 1 },
  dragging: false,
  lastMouse: null,
  selectedProvince: null,
  animFrame: 0,
  dirty: true,
  lastSeaTick: -1,
  activityOverlay: false,

  // City drawer + generic drawer system.
  cityViewID: null, // province ID of the settlement the City drawer currently shows
  activeDrawer: null,

  // March context menu (ui/marchctx.js).
  marchCtxDest: null,   // { q, r, terrain, isSea, name, isSettlement, allied }
  marchCtxUnits: [],    // eligible units currently listed
  marchCtxGroups: [],   // eligible units grouped by type+origin (quantity picker)

  // Search overlay (ui/search.js).
  searchFocusIdx: -1,
};

// The player's capital ("Metropolis"), or — if it was lost and the server has
// not yet promoted a replacement — any surviving owned settlement. Without this
// fallback, losing the capital blanked the whole UI (every drawer keys off the
// capital) even though the player still held colonies. Capital loss is meant to
// be recoverable (Timothy 2026-07-10), so the UI keeps working on a colony.
export function ownCapital() {
  return State.provinceData.find(p => p.own && p.is_capital)
      || State.provinceData.find(p => p.own && !p.is_outpost)
      || State.provinceData.find(p => p.own)
      || null;
}

// The settlement the City drawer currently shows. Defaults to the capital,
// but the player can cycle through their own settlements via the drawer's
// prev/next arrows (cycleCityView, in ui/drawers/city.js) or by clicking a
// settlement on the map.
export function activeCitySettlement() {
  const mine = State.provinceData.filter(p => p.own && !p.is_outpost);
  if (!mine.length) return null;
  const chosen = State.cityViewID && mine.find(p => p.id === State.cityViewID);
  return chosen || mine.find(p => p.is_capital) || mine[0];
}
