# slakt-bort — grep list

Order: vault `megaron_plan_slakt_bort.md` (Timothy 2026-10-15). Patterns grepped across server, web, tools, docs:
`slaughter`, `166`, `166.67`, `livestock`.

| Hit | Action |
|---|---|
| `server/api/handlers/settlement_placement.go` `SlaughterLivestock` | handler removed |
| `server/cmd/server/main.go` route `.../slaughter-livestock` | removed |
| `server/api/handlers/settlement_placement_test.go` route mount | removed; added `TestSlaughterLivestockRouteIsGone` (404/405) |
| `settlement_slaughter_livestock_test.go` | deleted |
| `placement_concurrency_test.go` `TestPlacementSlaughterLocksSettlementBeforeGoods` | removed (tested the removed handler) |
| `economy.LivestockSlaughterPopGain` | removed |
| `economy/recompute.go` `livestockFoodValue = 166.67` | now 3, comment rewritten |
| `cmd/keryx/cmd_status.go` mirror | now 3 |
| `cmd/keryx/cmd_slaughter.go`, `main.go` registration | deleted / removed |
| `economy/founding_placement.go`, `placement_prune_test.go` comments naming SlaughterLivestock | reworded to applyDecay growth |
| `food_consumption_split_test.go` AK4 (ceil(5/200)) | updated to ceil(5/3)=2; new `TestFoodConsumptionSplit_OneAnimalCoversExactlyThreeGrain` |
| `livestock_fallback_test.go` comments citing 166.67 | updated (assertions unchanged: ceil(2/3)=1) |
| `cmd_status_surplus_test.go` uses `livestockFoodValue` symbolically | unchanged |
| web `city_production.js` button + result div | removed (Livestock count row kept) |
| web `city.js` `slaughterLivestock()`, `main.js` import/export | removed |
| `verb_telemetry.js` / `.test.mjs` slaughter + `livestock_slaughtered` | removed (eight -> seven historical events) |
| `city_production.test.mjs` | now asserts no button / no "slaughter" |
| `tools/verb_parity_allowlist.json` slaughter-livestock | removed |
| `tools/umami_verb_proof.py` HISTORICAL set | `livestock_slaughtered` removed |
| `tools/production_live.py` slaughter step | removed |
| `docs/reviews/umami-verb/verblista.md` snapshot + vault `megaron_verblista.md` | slaughter row removed |
| Codex `city.md` slaughter sentence | removed |
| Codex `food.md` | kept automatic emergency slaughter; added "each animal feeds for 3 grain" |
| `food.md`, `recompute.go` comments on AUTOMATIC slaughter, `FoodConsumptionSplit` | kept (automatic grain -> fish -> livestock stays) |
| SQL migrations: `166` hits | only `mig 166` mentions in Go comments (migration number, unrelated); no SQL function mirrors 166.67, so no new migration |
| `religion/prayers.go` "slaughtered" | lore text, unrelated |
| `pixelgrid.js` "L≈166" | colour luminance, unrelated |
| historical `docs/reviews/*` logs | left as history |
