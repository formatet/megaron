# Inventory (before building)

- Call sites: 8 `openMobilePanel(document.getElementById('inspect-panel'), closeInspect)` in `render/map.js` (fog, foreign city, terrain, rural, host, ...), all followed by `style.display='flex'`; 3 `closeMobilePanel` (own city and rural CTA open a drawer; `closeInspect`). Replaced by one helper `showInspect()`.
- `.inspect-panel` base styles: `web/static/map.html` ~380-398 (absolute, right, 220 px, flex column; `.inspect-body` already `overflow-y:auto`). Phone window rules: `web/static/megaron.css` `@media (max-width:480px)` block (`.mobile-fullscreen`).
- Camera: `State.camera {x,y,zoom}`, `hexClientPos(q,r)` (inverse of `hexAtScreen`), `clampCamera()`. No second camera written.
- Bottom-anchored things at 390 px: only `.game-nav` (68 px, outside `#map-root`, so a sheet absolutely positioned at the bottom of the map sits above it). Zoom buttons are top-left, the pick bar top, the legend overlay is not in the page. Nothing had to be moved.
- Tests touching `mobile_panels.js`: `ui/mobile_panels.test.mjs` (3 new tests added); fake-DOM suites that load `map.js` (founding_inline etc.) needed `classList?.contains?.` tolerance in the helper.
- Note: own-city hexes open the City drawer directly (never the inspect panel); the sheet appears for terrain, fog, foreign city, rural hex, host.
