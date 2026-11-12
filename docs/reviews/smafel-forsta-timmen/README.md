# Small faults from the first-hour playtest (order-smafel-forsta-timmen-20261112)

Source: `docs/reviews/forsta-timmen/README.md` §8-§9 (main checkout). Pictures: Firefox, isolated acceptance world,
`python3 tools/smafel_bild.py` (desktop 1280x900, phone 390x844). "Before" is the playtest's own shot number in
`docs/reviews/forsta-timmen/shots/`; the "after" pictures are in this folder. One commit per fault (`git log smafel-forsta-timmen`).

| # | Fault | Reproduced? | Cause | Fix | Test | Picture |
|---|---|---|---|---|---|---|
| 1 | Two free-citizen counts | Yes: top "Free to work 4", grid "3 free" after one +1 | Top row came from `goods[0].idle_citizens` at drawer load; the grid reads `placement-options.pool_size` after every click | Grid render rewrites the top row from `pool_size` (one source) | `citygrid.test.mjs` SMAFEL1 | 01 |
| 2 | "None of yours to take off" on every row | Partly. On a row where my citizens stand, -1 was enabled and no reason shown. The line appears on every row with none of mine, also when other goods on the same hex have workers | The row never said where the hex's workers are | Hex panel names them ("Your citizens here: grain 4"); reason shortened to "None of yours" | SMAFEL2 | 02 |
| 3 | "arrives arrived" | Reproduced in code (a marching unit past its arrival time, before the server settles it) | `arrivalHTML` returns the done-word, the card prefixed "arrives " | "-> (q,r) · has arrived" | `force_card.test.mjs` SMAFEL3 | unit test only (state is a few seconds long) |
| 4 | Expedition refusal contradicts itself | Yes, via `march-preview` | Preview message names the public floor (4) and says "this one needs longer"; the route's own minimum is deliberately hidden | Server text now names the duration asked: "an expedition of 10 game days is not enough to reach the chosen area and return home - choose a longer duration or a nearer area (any expedition may last 4 to 365 game days)". The route minimum stays hidden (preview rule unchanged). Keryx and web show the server's sentence verbatim, so no other copy | `small_texts_test.go`, `unit_march_preview_test.go` | 05 |
| 5 | Duration field hidden under "More" | Yes (phone) | Field sat in a closed `<details>` | On a duration refusal "More" opens once and the field is focused; focus selects its content so typing replaces | `expedition.test.mjs` SMAFEL5 | 05 |
| 6 | Raw keys | `herd_surplus`: already "Herd" on master (food order), skipped. Tooltip "Coastal_sea": yes (code) | Tooltip capitalised the enum key | Tooltip uses the map's terrain label. Sweep found three more raw keys: held building in the Take button (`olive_press`), besieging troops in the City banner and lost troops in the foundering notice (`elite_infantry`). Now player labels. Other keys checked are single words (goods) | SMAFEL6 | 06 (a lake hex: no sea hex known in the acceptance world) |
| 7 | Red for good numbers | Yes | `.sr-val` (every stat value) and `.goods-mini` last column were `--accent` (clay red) | Neutral `--text`; `.sr-val.neg`/`.warn` red for a loss (negative net, upkeep, food flows) or "Above ceiling". Warehouse net, flows, Kult net/upkeep carry the class | updated drawer tests + SMAFEL7 | 07 |
| 8 | A name leaks to the neighbour | Not a bug | A named place is a whole region: Codex `naming-the-land` says hills must cover at least 3 hexes and "click any known hex of the place"; "a hex shows the smallest place it belongs to". In the acceptance world one hills place spans 14 hexes (`keryx places`) | Nothing changed | - | - |
| 9 | "Walls hold 20" vs Codex 2,000 | Yes | Screen counts citizens (cap/100); Codex wrote people | Codex buildings + growth say 20 / 50 / 100 / 200 citizens (a citizen is 100 people). Keryx status keeps "people" next to its "Pop: N people" | `codex.test.mjs`, `codex_facts.py --check` | 09 |
| 10 | Truncated notice banners | Yes | Chip text one line, ellipsis; the dismiss cross was 40% beige on the beige bar (invisible). Click already opens the notification and dismisses | Chip text wraps to two lines (chip 36 px in the 44 px bar); cross in `--text-dim`. Full text stays in the tooltip and on click | CSS only | 10 |
| 11 | Stance dropdown "stance..." | Yes | First option was a placeholder | Options: no stance / fortify / storm / sentry with the current one selected and labelled "stance: X" | SMAFEL11 | 11 |
| 12 | Phone: buttons move | Yes: stone row moved 24 px after one -1 | Reason line appeared/disappeared in the button column; "Fill" vanished when full so "-1" slid left | Fill stays (disabled), one reserved reason line, rows top-aligned. Measured: button y identical before and after +1/-1 | SMAFEL tests (CG fishing updated) | 12 |

## Not done / notes
- 3 has no picture (a transient state); 06 shows a lake hex.
- The dispatch (real order) refusal still names the route minimum ("give it at least 20 game days"); only the preview hides it. Not touched: that is a design call.
- Keryx status prints the wall ceiling in people; left as is.
