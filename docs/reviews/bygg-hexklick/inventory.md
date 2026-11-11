# Inventory (before the change, master 99296076)

## Functions / DOM ids touched
| Name | File | Imported by |
|---|---|---|
| `startBuild()` (reads DOM selects) | ui/drawers/city.js | main.js (import + window), city.test.mjs, tools/late_dom_probe.mjs |
| `onCityBuildTypeChange(hex)` | city.js | main.js (import + window), tools/late_dom_probe.mjs |
| `buildingOptionsHTML`, `buildingOptionHTML`, `templeOptionsHTML`, `wallOptionHTML` | city.js | city.test.mjs |
| `hexBuildOptionsHTML` | city.js | city.test.mjs |
| `buildQuoteHTML`, `showBuildQuote` | city.js | city.test.mjs |
| `splitBuildValue` | city.js | city.test.mjs |
| `getBuildingCatalogue`, `fmtBuildCost`, `templeName`, `TEMPLE_GODS` | city.js (private) | - |
| `renderGubbeGrid`, `terrainLabel` | ui/citygrid.js | city.js, citygrid.test.mjs |
| DOM ids `city-build-select`, `city-build-hex`, `city-build-quote`, `city-build-result`, `#city-bld-sec button[onclick="startBuild()"]` | city.js markup | tools/game_days_acceptance.py, tools/bildsvep_20261101.py, tools/late_dom_probe.mjs, docs/reviews/bygg-niva/browser_check.py (historic) |

## Build POST today (`POST /api/v1/worlds/{w}/provinces/{id}/build`, JSON.stringify of this object, key order as shown)
- (a) city building (granary): `{"building_type":"silo"}`
- (b) hex-bound (farm at 3,-1): `{"building_type":"farm","hex_q":3,"hex_r":-1}`
- (c) temple with god: `{"building_type":"temple","god":"velchanos"}`
