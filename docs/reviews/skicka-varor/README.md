# Send goods — one form (web only)

Base master b99527ce, branch skicka-varor. Order: `.agents/order-skicka-varor-20261110.md`. Go, keryx, request bodies untouched
(`git diff --stat master -- server` empty). Not merged, pushed or deployed. Inventory: [inventory.md](inventory.md).

## What changed
- Economy tabs: **Goods · Send goods · On the road** (was Goods · Transfer · Routes & cargo).
- **Send goods** (`ui/drawers/send_goods.js`, new): From/To/route are buttons (no `<select>` anywhere), one row per good the From city
  holds with `[ n ] [All]`, one route line (two buttons only when both land and sea are available), "Too much: the Naia carries 120"
  only when the cargo does not fit, **Keep it stocked** checkbox (own destinations only) turning Send into "Keep it stocked →"
  (POST standing-orders, `crewed_by_settlement_id` = From, `return` = rows under **More**), foreign destination turns Send into
  "Send as a gift →" + "A gift: nothing comes back." The result line is followed by the fresh row (the new cargo or standing order).
- **On the road**: standing orders list (pause/resume/delete unchanged) then "Other cargo in transit"; no form.
- Map: foreign city panel gets **Send goods →** next to "March here →" (opens the form with To preselected, From = capital).
  Own cities bypass the inspect panel (they open the City drawer), so no button there - see "Not done".
- No "Crewed by" in the web DOM. Existing orders crewed by the destination still list ("By <city>" unchanged).

## Counts
- Click count, open map to sent caravan: foreign city (1) · Send goods → (2) · All (3) · Send (4) = **4**.
  Via Economy instead: Economy · Send goods · To · All · Send = 5. Before: Economy · Transfer · To (open + pick) · Add good · pick good · type · Send ≥ 7-8.
- Controls for one caravan with 3 goods in stock: before 2 selects + Add good + (good select, number, remove) + 2 radios + Send = 9;
  standing order 3 selects + 2 Add + rows + 2 radios + Create = 12+. After: one form, From/To buttons + 3 number fields + 3 All + route + Send.

## Pictures (Firefox via Playwright, `tools/skicka_varor_bild.py`; real modules and megaron.css, fixture HTTP: Phaistos, Kyme, Gournia, ruler Myrine)
1280 px: S1 empty form · S2 foreign chosen, gift · S3 Keep it stocked checked with More open · S9 sent result + fresh row · S4 On the road.
390 px: S5 empty · S6 foreign · S7 keep · S10 sent · S8 On the road. Every shot asserts no sideways scroll.
Files: `S<n>-<name>-<desktop|mobile>.png` in this folder. Fixture, not a live world.

## Removed
Functions: loadEconomyTransfer, loadEconomyAutomation, loadTransferDestinations/Goods/Preview, addTransferGood, updateTransferCargo, startTransfer,
transferItems, cargoStatus, createStandingOrder, refreshCargoInTransit, the standing-good row helpers; window bridges for them in main.js.
Tests deleted: `transfer_cargo.test.mjs` (cargoStatus/transferItems), the loadTransferDestinations test in `gifts.test.mjs`, and the old
Transfer/automation tests in `economy_rows.test.mjs` (replaced by two real-form POST tests + `send_goods.test.mjs`).
Edited guard text: format.test.mjs + format.js insufficient-goods hint now says "Economy → Send goods" (the Transfer tab is gone).

## Not done / for review
- Codex articles (`web/static/codex/routes.md`, `goods.md`, `articles.json`) still say "Routes & cargo"/Transfer/Crewed by: out of this web-only order; needs a Codex slice.
- No "Send goods →" for own cities from the map (their click opens the City drawer); Economy → Send goods covers them.
- "Bring home" rows list goods the own destination holds (fetched from its province goods); blank = none.
- Route times come from `/trade/preview` for both modes (standing orders previously used transport-options; same handler).
