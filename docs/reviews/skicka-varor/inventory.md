# Inventory (master b99527ce) — web/static/js/megaron/ui/drawers/economy.js

Window wiring: `main.js` imports from `ui/drawers/economy.js` (line 52-54) and lists them in
`Object.assign(window, {...})` (line ~175+); inline `onclick=`/`onchange=` in the markup call these globals.
Other importers: `ui/dispatch_actions.js` (`focusCargo`, `focusStandingOrder`, dynamic import),
`ui/drawers/kult.js`/`city_warehouse.js`/`city.js` mention economy only for `focusEconomyCity`/ids.

Transfer tab (removed): loadEconomyTransfer, loadTransferDestinations, loadTransferPreview, loadTransferGoods,
addTransferGood, updateTransferCargo, startTransfer, transferItems, cargoStatus.
DOM ids: ec-tr-from, ec-tr-to, ec-tr-items, ec-tr-add, ec-tr-route, ec-tr-capacity, ec-tr-send, ec-tr-result, ec-tr-cargo.
Automation tab (removed as a form): loadEconomyAutomation, createStandingOrder, standingGoodRowHTML,
bindStandingGoodsRows, readStandingGoodsRows, parseGoodAmountPairs (reference parser, kept: tested, unused by the form).
DOM ids: ec-so-from, ec-so-to, ec-so-crew, ec-so-out(-add), ec-so-home(-add), ec-so-route, ec-so-result, ec-so-list.
Kept unchanged: goods tab, renderStandingOrdersHTML, pause/resume/delete, formatCargoRows, renderCargoHTML, routeCargoHTML.
focusCargo/focusStandingOrder (dispatch_actions) now both open the "On the road" tab.

## POST bodies today (byte for byte, JSON.stringify key order)
Transfer `POST /api/v1/worlds/{w}/provinces/{fromProvinceId}/trade`:
`{"destination_id":to,"items":[{"good_key":..,"quantity":..}],"by":"land|sea"}`  (`by` omitted if no choice)
Standing order `POST /api/v1/worlds/{w}/standing-orders`:
`{"from_settlement_id":..,"to_settlement_id":..,"crewed_by_settlement_id":..,"outbound":[{"good_key":..,"threshold":..}],"return":[{"good_key":..,"floor":..}],"by":"land|sea"}`
Responses used: trade -> transport_id, ship_name, kind('gift'), arrives_at, arrival_tick, travel_ticks, route_version;
standing order -> id.
