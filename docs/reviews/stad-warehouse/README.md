# City: Warehouse tab + Garrison -> Recruit (2026-10-31)

Plan: `.agents/plan-stad-warehouse-20261031.md`. No server change: the tab reads fields the server already sends
(`/provinces/:id/goods` `amount`/`cap`, province `food.coverage_ticks` and `food.herd`).

## What the tab shows
- Food: "Food for N game days"; grain, fish, oil as amount / granary cap (`?` opens the Granary article).
- Herd: the existing herd line (count / pen cap, calves).
- Goods: every other good with amount > 0, alphabetical by label, amount / cap where a cap exists; a store at 99% of cap is marked "full" (`.wh-full` in megaron.css).
- Production keeps "Food net +x per game day", the harvest blessing, storm line and "fields cannot feed everyone".
- Garrison: "Recruit" button beside the title -> `warOpenRecruit(id)` opens War on the Recruit tab with that city selected.

## Surfaces
temenos: unchanged. keryx: unchanged (no new field; `status` already prints stocks). megaron: city.js, city_warehouse.js, city_production.js, war.js, main.js, megaron.css. codex: city.md (+ articles.json).

## Tests
`city_warehouse.test.mjs` (5), `city_production.test.mjs`. Mutants killed: sort removed (warehouse test :21), "Food for" back in Production (production test :18), Recruit button removed (warehouse test :45).
Gates green: `tools/test-web.sh` 883/0, `day_guard_test.py`, `test_verb_parity.py`, `codex_facts.py --check`.

## BILD
Not taken: no screenshot from a real browser in this slice. Open City -> Warehouse and Garrison on the acceptance world.
