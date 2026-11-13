# First hour, round three: the result appears where the player acts

Order: `.agents/order-forsta-timmen-varv3-20261113.md` (base master 21f2316a, branch `forsta-timmen-varv3`, one commit per item).
Proof: `tools/varv3_bevis.py` drives the REAL page in Firefox (1280x800 and 390x844) with real clicks/taps and asserts on
what is in the viewport. Run BEFORE each fix (output kept in `before-item*.log`, pictures in `before/`) and AFTER
(`after-item*.log`, `after-all.log`, pictures in `after/`). The acceptance world is `tools/acceptance.sh` (localhost:8097,
tick 120 s), four players Varv1-Varv4 with founded cities, Varv5 left in the founder phase.

Setup done through the world's own database (acceptance only, never the live server):
- `tools/acceptance.sh psql` adds stone/timber to the Varv3 store (the start store has none) and, for Varv1, reveals the map
  (`player_scouted_tiles`) so a lake hex and an unreachable land hex can be clicked.
- `reset_units()` puts a player's spearmen back in the garrison between marches (earlier marches had used them up).

A lesson built into the script: the first version of the worker-button check "passed" vacuously because the press never
reached the button (it was off screen). Every press step now asserts that the press TOOK EFFECT (the workers line and the
"N free" count changed) before it compares rects.

## Per item

| # | Before (master) | After |
|---|---|---|
| 1 | FAIL on the page. Lake + soldier: "Explore reachable sea nearby, then sail home." / "No galleys available..." then "Choose the forces to send by name." with no list; the answer sat 20 px below the dialog's visible edge. "Nothing sent - No route ..." 84 px below the edge (desktop). Phone success left the sheet open on an almost empty page. | PASS 17/17. `setMarchErr()` scrolls the answer into view beside March; soldiers aimed at water get "Soldiers cannot cross water. They need a ship: build one in a coastal city." and no sea text; an empty list never asks to choose from itself; on a phone the sheet closes and the receipt shows in `#map-notice` (see inventory). |
| 2 | FAIL. Picker 147 px below the visible panel; Build button below the fold on a phone; the confirmation (the `.gubbe-refusal` fallback) 300 px below the visible panel. | PASS 10/10. The picker scrolls into view after it is drawn, the chosen hex's Build row is scrolled into view, the answer "Farm queued at (q,r) - ... ready ..." sits under the hex title and stays until the next click (`gridNote`). A refusal likewise. The receipt now says "at (q,r)" instead of "@ (q,r)". |
| 3 | FAIL on phone: the rows below moved 22.4 px on the first +1 / the last -1 / FILL (row text re-wrapped). Header: count and hint printed in one span. | PASS 32/32 (first +1, second +1, -1, last -1, FILL on a 4-place hex and first +1 / last -1 on a one-place hex, desktop and phone; every button rect identical). Each text line of a row is one line; on a phone the buttons sit under the text; the hint has its own always-reserved line at both widths. "The hint only on phone" was NOT reproduced: before the fix it showed on desktop too. |
| 4 | FAIL. Warehouse stock line never changed in 42 s although the store changed (the Buildings rows did follow). | PASS. `paintCityFigures()` (stock line, food, population, last-day sections) is now also repainted by the 30 s refresh, only when a section's text changed. Store changed through `tools/acceptance.sh psql`; both followed within 24 s. |
| 5 | Not a fault. Economy Settlements table, City production Food and Warehouse Food read the same value in the same game day (+0.6 and +0.3 in two cities); per-good rates ("Grain -1.6 per game day") sit in rows that name the good. Nothing changed. Note: "Growing: growth surplus +1.8 per game day before army" in the Population block is a different quantity (population growth) that could be mistaken for a food rate - left as is, named for Timothy. |
| 6 | FAIL. "1st Spearmen of CityVarv3 - explore_return -> (39,23)"; "this hills" (and "this strait/cape/..." had no "no one has named it" clause). | PASS 18/18. `marchIntentWords()` says what the march is for ("exploring", "on the way home from exploring", ...; unknown intents never show an underscore); the naming line is "Your unit was the first to reach these hills. No one has named them yet. Give them a name ..." for every kind (web inspect panel, dispatch text, keryx notification). |
| 7 | FAIL. "Unavailable: trade offer, trade accept, trade decline, trade cancel, transfer ?" on Economy, "Unavailable: recall, redirect, unload, land ?" on War (Recruit tab), "Unavailable: rite ?" on Kult. | PASS 18/18. `lockedActionsHTML` (renders for Economy, War, Kult and City): one line "Not open to you yet: 5 orders [?]" in the existing `<details>` style, opening to "Trade offer: <reason>" per order; keryx command snippets in reasons are stripped on the web; nothing at all when nothing is locked (unit test, the page has no such drawer to show). |
| 8 | FAIL. Confirmation said only "The host dissolves - forever."; forecast "5 eaten". | PASS 6/6. Verified in `found_metropolis.go`: host population becomes the city's population (`Population: pop`); every spearman cohort gets the city as payer and those standing on the founding hex become its garrison, others stay in the field. Text: "Your people become the city's citizens; your spearmen stay yours, and those standing here become its garrison." (web + keryx). Forecast: "..., your people eat 5". |
| 9 | FAIL. Promise "turns home by 07:11; home by 07:21" with no hint it is an upper bound. `expedition.go` confirms the rule (the leg loop ends at the turn tick, or earlier when the area holds nothing more to see / nothing reachable: `turn_reason` area_known / no_path). | PASS 4/4. "home by 07:23 at the latest (sooner if there is nothing more to see)" in the forecast, the receipt (web), keryx `march-preview` and expedition mission line, and the Codex "Exploration" article. Once the unit has turned the plain "home by" stays (it is then the estimate). |
| 10 | The spill was NOT reproduced: at 390 px the label "L0 -> L1" is 46 px inside a 63 px hex. The label form fails the ordered check. | PASS. At <= 480 px the pick label is the level it becomes ("L1"). |

## Inventory for item 1c (what exists as a notice place)
There is no toast. The existing candidates: the Dispatches chip strip (server notifications only, not local receipts), the
pick bar (`#hexpick-bar`, a status line over the top of the map, used while picking a hex). I reused the pick bar's look
(class `hexpick-bar` plus `map-notice`) for a one-line status over the map: `showMapNotice()` in `marchctx.js`, dismissible,
gone after 15 s. Desktop keeps the receipt in the dialog (it is visible there).

## For the reviewer
- Item 5 changed nothing on purpose. If you want "Grain" rows quieter under the Food figure, that is a design call.
- The Codex article bundle was regenerated (`python3 tools/codex_facts.py`) for the exploration sentence.
- Not done / out of scope: kharis, recruit list, clock, start stores, any rate or cost.
