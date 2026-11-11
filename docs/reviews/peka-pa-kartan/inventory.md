# Inventory (before editing)

## Where map clicks are handled
- `web/static/js/megaron/render/map_input.js` `bindMapInput`: one pointer path. A pointerup that did not move >= 4 px and was not a
  long press/second finger calls `tap(clientX, clientY)`; right-click and the 500 ms long press call `orders`.
- `render/map.js` ~4056 `bindMapInput(canvas, {pan, zoom, tap, orders, hover})`. `tap` = `hexAtScreen` (the only hit-test) then
  `State.marchCtxUnitID ? window.openMarchCtx(...) : openHexPanel(h)`. `orders` = `openMapOrders` (march menu). Selected hex is
  outlined in the render loop at "3. Highlight selected hex" from `State.selectedHex`.
- Hook plan: `tap` and `orders` first ask the pick module (`pickActive()`); while picking, tap -> `pickTap(q,r)`, orders -> nothing.

## How drawers and panels open and close, and what they hold
- `main.js` `openDrawer/closeDrawer/toggleDrawer`: class `open` on `#drawer-<name>`, `State.activeDrawer`, `#map-dim`. Content is
  re-rendered by `loadDrawerContent` only on open; hiding with CSS keeps DOM, scroll, typed text and tabs.
- `#inspect-panel` (`closeInspect()` in map.js), `#march-ctx`, `#dispatch-window-overlay`, `#account-window-overlay`,
  `#search-overlay`, `#codex-panel`, `.map-dim`: all separate elements.
- `ui/mobile_panels.js`: at <= 480 px an open panel gets `mobile-fullscreen`, `#map-root.mobile-panel-blocked` and
  `canvas.inert = true`. Picking must lift that (the canvas must receive taps) and put it back afterwards.
- Plan: body class `hex-picking`; CSS makes those surfaces `visibility:hidden; pointer-events:none` (not display:none, so scroll
  offsets survive). Nothing is closed, so exactly the open ones return.

## Redirect, byte for byte (unchanged)
`ui/drawers/war.js unitRedirect`: `POST /api/v1/worlds/{WORLD_ID}/units/{unitID}/recall`, header `Content-Type: application/json`,
body `JSON.stringify({ target_q: q, target_r: r })` (ints via parseInt). Status line `#uorder-<id>`, `orderSentLine(d,'Redirect')`.
Card markup: `ui/force_card.js` ~324-336 (`uredir-<id>`, `uredir-typed-<id>`, `uredir-q-<id>`, `uredir-r-<id>`).
The hex panel's own "Redirect" (`data-hex-redirect`, map.js ~3399) uses `chooseMarchTarget` (march menu) and is not touched.

## Wake the Storm rite, byte for byte (unchanged)
`ui/drawers/kult.js castRite`: `POST /api/v1/worlds/{WORLD_ID}/settlements/{prayer.settlement_id}/rite`,
`Content-Type: application/json`, body `{ prayer: prayer.id, target: "q,r", offering? }`; `target` from
`kult_offer.js chosenTarget` = trimmed value of `.offer-target-hex`. Markup `stormChooserHTML` (kind `hex`).
