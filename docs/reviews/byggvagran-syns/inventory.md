# Inventory: where a build or upgrade starts (2026-11-12)

Server route: `POST /api/v1/worlds/:w/provinces/:p/build` (`buildBody` in `ui/build_rows.js`). Client senders: exactly one,
`startBuild()` in `ui/drawers/city.js`. An upgrade is the same call (the row says `L1 -> L2`); there is no separate upgrade verb.
Other things named "build" are not construction starts: `build-road` (a unit, `ui/drawers/war.js`), ship fitting and recruiting (units).

| Start point | Where | Result printed today | Costs / stock known there |
|---|---|---|---|
| City > Buildings > Construct rows (city buildings, upgrades, walls, temples; `Choose a hex` for hex-bound) | `build_rows.js buildRowHTML`, click wired in `city.js refreshCityBuildings` | `#city-build-result`, a line at the very bottom of the Buildings tab (after the whole list) | cost: catalogue/quote; stock: not loaded, only the server `can_afford` flag (locks the row with "not enough goods in store") |
| Production tab > hex > "Build here" / pick mode rows | `build_rows.js hexBuildRowHTML`, click `citygrid.js buildOnHex` | `.gubbe-refusal` appended to the END of `#gubbe-detail` (below every good row and every build row) | cost: placement-options `build_quotes`; stock: not loaded at all |

Stock source in the client: `GET .../provinces/:p/goods` (array of `{key, amount, cap}`), already fetched by `loadCityDrawer`
and by `kult.js okRite`. `pd.resources` exists on the province response too but is not used for this.

Warehouse (`ui/drawers/city_warehouse.js`): the `Goods` list is filtered `g.amount > 0`, so stone and timber at 0 are dropped
on purpose together with every other empty good; only the Economy screen lists zeros.
