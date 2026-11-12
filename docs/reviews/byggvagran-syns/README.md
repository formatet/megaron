# byggvagran-syns: a refused build is seen, an unaffordable build looks it (2026-11-12)

Order: `.agents/order-byggvagran-syns-20261112.md`. Inventory: `inventory.md`. Pictures from Firefox on the REAL canvas map in
the isolated acceptance world (localhost:8097), 1280 and 390 wide. Script: `tools/byggvagran_bild.py poor|rich [desktop|mobile]`.
Setup: four players, the first founds its city with keryx. A fresh city holds 0 stone and 0 timber (the case pictured).

## What changed (web only)
- `ui/build_rows.js`: `shortfalls` / `shortfallText` / `loadStock` (one fetch of `.../provinces/:p/goods`, as `kult.js okRite`);
  rows with a known shortage get a disabled Build plus `.btn-reason` "needs 6 timber, city has 2"; `showBuildResult` puts the
  answer under the clicked row and `scrollIntoView({block:'nearest'})`. Used by both senders (Construct list in `drawers/city.js`,
  hex rows in `citygrid.js`). Refusals go through `playerError`.
- Convention followed: a disabled button with its reason as text (serverfel C). Hex-bound `Choose a hex` stays clickable (it starts nothing) but tells the shortage.
- When the client knows the stock, the specific reason replaces the old server-flag lock "not enough goods in store".
- `drawers/city_warehouse.js`: stone and timber always listed (0 shown); every other empty good stays hidden. Cause: the list was filtered `amount > 0`.
- The bottom-of-tab `#city-build-result` line is no longer rendered (startBuild still writes to it if present, so its old tests stand).

## Pictures
| | desktop | mobile |
|---|---|---|
| a Build here, unaffordable rows | a-buildhere-desktop | a-buildhere-mobile |
| b refusal under the row (hex) | b-refusal-hex-desktop | b-refusal-hex-mobile |
| c Buildings tab list (c0) and refusal under the row | c0-/c-refusal-buildings-desktop | c0-/c-refusal-buildings-mobile |
| d Warehouse, stone/timber 0 | d-warehouse-desktop | d-warehouse-mobile |
| e affordable build succeeds (e0 before) | e-success-desktop | e-success-mobile |

Staging, said plainly: (b) and (c) are taken by removing `disabled` from the button in the page and clicking, i.e. the case
"the client's stock was stale and the server refuses"; with fresh stock the button is disabled and the click cannot happen.
The real server refusal text is what shows. (e): stone and timber were set to 100 with `tools/acceptance.sh psql`
(`update settlement_goods`), and the queue was emptied with `delete from build_queue` between the desktop and mobile run.
The result scrolled into view without a hand scroll in every run (measured: result top/bottom inside the viewport; no sideways scroll).

## NOT tried
- Real iOS/Android devices; Chromium/Safari (Firefox only, at 390 px width emulation with touch).
- Upgrade of an existing building as a separate path (it is the same row and call; the rows were not pictured at L1 -> L2).
- A refusal with the panel scrolled so the row is above the fold (the `nearest` scroll was only seen moving down).
- Temple and wall rows with a shortage (same code path, not pictured).
- Cedar substitution in the client check is a mirror of the server rule, checked in a unit test only.
- Stock freshness: the stock is loaded when the Buildings tab / hex grid renders; it can age until the next redraw. The server stays the judge.
