# Phone: inspect panel as a half-height bottom sheet (branch telefon-halvpanel)

Order: `.agents/order-telefon-inspektion-halvpanel-20261112.md`. Script: `tools/telefon_halvpanel_bild.py` (real canvas map, Firefox 390x844, isolated acceptance world; results in `results.json`). Inventory: `inventory.md`.

## What changed
- `ui/mobile_panels.js`: `openMobilePanel(el, close, {sheet:true})`; a sheet gets `mobile-sheet` (phone only), never marks the map blocked / canvas inert / body `mobile-panel-open`; a full-screen panel opened later closes it. New pure `sheetPanDelta`.
- `render/map.js`: `showInspect()` replaces the 8 open calls; pans the camera up just enough (existing `hexClientPos`/`clampCamera`) when the selected hex is under the sheet; a `ResizeObserver` repeats the check when the sheet grows (a force card expanded).
- `megaron.css` (inside the existing 480 px block): `.inspect-panel.mobile-sheet` absolute at the bottom of the map, `max-height:50dvh`, top border, body scrolls, close 44x44.
- Moved: nothing. The sheet lives in `#map-root`, above the nav bar; zoom buttons are top-left.

## Pictures (390x844 unless stated)
- a-tap-upper-half-sheet: tap a hex in the upper half; sheet at the bottom, hex clear above it, map live (not inert).
- b-tap-lower-half-panned: tap a hex in the lower half (y 644); map panned up so the hex ends at y 535, above the sheet; an already-visible hex does not move the map.
- c-tap-second-hex-sheet-open: second tap replaces the content, one sheet.
- d-long-content-scrolled: two expanded force cards; body scrolled inside the sheet (header, close, foot stay), sheet capped at 50 %, hex still visible.
- e1-force-hex-sheet: hex with two forces. e2-march-pressed-pick-bar: March on a force card: pick bar on top, sheet hidden, map free.
- f1-rural-sheet-before-drawer / f2-drawer-full-screen-sheet-gone: "Open ... ->" opens the City drawer full screen, sheet gone.
- g-desktop-master / g-desktop-worktree (1280x800): same hex, master files served via route interception vs this branch; the panel strip is pixel-identical (the map's water animates, the top bar has a clock, so only the panel strip is compared).

## Not tried
Real device / on-screen keyboard / iOS safe-area (env() inset not exercised); landscape phone; pinch-zoom with a sheet open (drag was driven, pinch was not); the foreign-city and host panels as sheets (same code path, not photographed); `hex-picking` started from the Kult drawer (needs a temple chain).
