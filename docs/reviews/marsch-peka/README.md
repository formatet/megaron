# War -> March through pickHex (web only, branch marsch-peka)

Order: `.agents/order-marsch-peka-20261111.md`. No server change (`git diff --stat master -- server` empty). Inventory: `inventory.md`. BILD: `tools/marsch_peka_bild.py` (Firefox).

## What it does
- `chooseMarchTarget(unitID, verb='March')` (ui/marchctx.js) starts `pickHex` at once: "March <unit>: tap the destination" ("Redirect ..." from the hex panel's Redirect button, which now passes the verb). No menu while picking; every panel hides.
  `accept` refuses only a hex with no tile ("Not on the map.").
- Confirm: pins `State.marchCtxUnitID`, opens the real march menu via `openMarchCtx(destFromHex(...))`, placed beside the hex (new `hexClientPos` in render/map.js, inverse of `hexAtScreen`, now exported). Cancel: nothing opens, `marchCtxUnitID` is null.
- Cities: `pickHex` got optional shortcuts (`setShortcuts`, `shortcut(i)`, `sheetHTML`); the bar shows a **Cities** button only when known cities exist; the sheet under the bar holds "Name — ruler" / "Name — your city" (max 40 % height, scrolls); choosing one confirms its hex.
- Codex `marching.md` updated inside its 1032-word cap (dropped two sentences of equal size: "Check each named unit to order." and the ETA sentence that the forecast bullet already covers).

## Removed
The armed empty menu in `chooseMarchTarget` (hint "Choose a destination on the map.", emptied menu, `loadMarchCities()` call there). NOT removed: the `State.marchCtxUnitID` branches in map.js `tap()` and `openMapOrders()` are live: after confirming, the open menu keeps the unit pinned and a further tap or right-click re-targets it. The city-select notes in `loadMarchCities` belong to the open menu and stay.

## Counts
Web suite 981 pass (969 + 12 new in `ui/march_pick.test.mjs`; red first, verified 11 red). `codex_facts.py --check` clean. At 390 px: War, March, tap hex, Use this hex, March -> = 5 taps, map visible while choosing; the BILD asserts no drawer/panel is visible while picking, canvas not inert, bar <= 64 px, sheet <= 40 % height, menu does not cover the chosen hex on desktop, the unit is checked, and the POST body is `{target_q,target_r}` unchanged.

## Pictures (docs/reviews/marsch-peka/)
M1/M5 March pressed, bar only · M2/M6 hex selected · M3/M7 march menu open, unit checked · M4/M8 Cities sheet (desktop 1280x800 / mobile 390x844).

## NOT exercised against the real canvas map
`render/map.js` cannot run in node and the BILD uses a stand-in canvas wired to the real `pickTap`; the real map's `hexClientPos` is only tested as the inverse of `hexAtScreen` in node. The hex panel's March/Redirect buttons (`bindUnitButtons`) and the camera/zoom case for menu placement were not driven. The cities request is faked in the BILD.
