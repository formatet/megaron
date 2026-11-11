# Inventory — War → March through pickHex (2026-11-11)

- `ui/marchctx.js chooseMarchTarget(unitID)`: closes the menu, sets `State.marchCtxUnitID`, shows `#march-ctx` emptied with the hint
  "Choose a destination on the map." and calls `loadMarchCities()` (fills `#mctx-city`, the cities select). Both strings in
  `loadMarchCities` ("No known cities yet. Choose a destination on the map." / "... or reopen to retry.") also serve the OPEN menu's city select and stay.
- Callers: `ui/drawers/war.js unitMarch` (closes the drawer first) and `render/map.js bindUnitButtons` (`data-hex-march` and `data-hex-redirect`,
  both call `chooseMarchTarget(unitID)`; the hex panel is closed first). No caller distinguishes march from redirect today.
- Map branches on `State.marchCtxUnitID`: `tap()` (~4073) and `openMapOrders()` (~4166). After the pick, the real menu is open with the unit
  still pinned; a further tap or right-click re-targets the open menu. So these branches are NOT dead: they stay for re-targeting.
- `openMarchCtx(dest, x, y)` needs `dest` from `destFromHex(h, tile, target)` (map.js export; tile = `tileAt(q,r)`, target = `State.provinceData` row) and
  reads `State.marchCtxUnitID` for preselection (`marchEligibleUnits(all, dest, unitID)`). It does not call `closeMarchCtx`, so setting the pin just before is enough.
- `closeMarchCtx()` clears `State.marchCtxUnitID`.
- Position: `positionMarchCtx(x, y)` puts the 210 px menu at x+8; desktop needs the chosen hex's screen position (new `hexClientPos` in map.js).
