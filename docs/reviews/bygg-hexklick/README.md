# Build by clicking, not by dropdown (web only)

Order `.agents/order-bygg-hexklick-20261111.md`. Web only: `git diff --stat master -- server` is empty; no keryx change;
the three request bodies are unchanged (pinned in `build_rows.test.mjs` BR6 and `city.test.mjs`).

## Before / after
| | before | after |
|---|---|---|
| native `<select>` in Construct | 2 (`#city-build-select`, `#city-build-hex`) | 0 |
| build buttons | 1 shared "+ Build" (could be disabled with no reason) | 1 per row: Build / Choose a hex →; locked rows have a visible reason and no button |
| clicks, city building (Buildings tab open) | 4 (open select, pick, [done], + Build; via keyboard fewer) | **2** (Buildings · Build) |
| clicks, hex building from the grid | not possible | **2** with Production open (hex · Build), 3 from another tab |
| clicks, hex building via the list | 6 (type select x2, hex select x2, + Build, plus tab) | **4** (Buildings · Choose a hex → · hex · Build) |

## Pictures (Firefox, fixture with readable names; `python3 tools/bygg_hexklick_bild.py`)
H1 Construct list · H2 locked rows (maximum level, not enough goods) · H3 pick mode (highlighted hexes with level step) ·
H4 hex clicked in pick mode · H5 normal hex with "Build here"; H6-H10 the same at 390 px.
Files `H*-*-desktop.png` / `H*-*-mobile.png`. The script asserts: no sideways scroll, my buttons >= 32 px, no `<select`
in the Buildings tab, and that the Build here click posts `{"building_type":"farm","hex_q":0,"hex_r":-1}`.

## What changed
- New `ui/build_rows.js` (+ test): rows, hex choices, pick-mode data, `buildBody`, catalogue fetch. Same data as before
  (catalogue, `build_quotes`, `placement-options`); no rules derived client side.
- `city.js`: `startBuild(spec)` is the one sender (returns `{ok,text}`), `pickBuildHex(type)`; list wired with listeners (no new `window` names).
- `citygrid.js`: pick mode (`opts.pick`), "Build here" block, "Build in the city → Buildings" link; `setBuildHandler` avoids a city<->grid import cycle.
- Codex: `buildings.md`, `city.md`, `farm.md`, `mine.md`, `lumbermill.md`, `stone-quarry.md`; `articles.json` regenerated.

## Removed
`#city-build-select`, `#city-build-hex`, `#city-build-quote`, `onCityBuildTypeChange` (also from `main.js` import + window),
`startBuild` window registration, `buildingOptionsHTML`/`buildingOptionHTML`/`templeOptionsHTML`/`wallOptionHTML`,
`hexBuildOptionsHTML`, `buildQuoteHTML`, `showBuildQuote`, `splitBuildValue`. Tests BK1-BK6, HX1-HX5, B1, ORDER15/17 (option-string tests)
replaced by `build_rows.test.mjs`; `late_dom_probe` cases build-hex/build-stale/build-refresh deleted (they tested the dropdowns);
`game_days_acceptance.py` and `bildsvep_20261101.py` selectors moved to the rows (not run: need a live world).

## Decisions / notes
- Row order stays alphabetical by the name read (Timothy 2026-10-24), not catalogue order.
- Locked reasons used: maximum level, price unavailable, the wall's Granary rule, "the build queue is full" (queue length vs `build_queue_max`),
  "not enough goods in store" (province `can_afford`, only for non-hex, non-temple, non-wall rows because those flags are per type, not per hex/god).
  "needs a harbour" and similar gates are NOT known to the client; they still arrive as the server's refusal in the result line.
- No confirmation step existed before; none added. The "if started now, ready ≈ HH:MM" line moved from the single quote line onto each row.
- Not run: live/acceptance world (not needed; BILD uses a fixture).
