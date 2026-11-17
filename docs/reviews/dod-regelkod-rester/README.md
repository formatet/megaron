# Leftovers after the dead religion code (order 2026-11-17)

Branch `dod-regelkod-rester`, from master 833ec99c. Removal only; no live rule, rate or player-visible behaviour changed.

## Step 0 - inventory (before code)
| Identifier | Class |
|---|---|
| `OfferMod`, `OfferModWithIncense`, `OfferModCap`, `IncenseShare`, `DistinctGoods`, `ComposedOfferMod`, `offerModMax/Min/Slope`, `stingyRatioFloor`, `varietyBonus*`, `incenseCapLift`, `IncenseKey` | grep over server/ web/ tools/ cmd/keryx: only `religion/offering.go` and `offering_composition_test.go`. Test only. |
| `units.stillness_*` readers | `combat/sea_storm.go` (protected check), `combat/speedup.go` (skip guard): reachable only if a column is set. Writers: NULL-resets in `unit_arrival/recall_redirect/ship_hull`, `march_start.go` (always nil), `transport.insertRow` (sets ticks NULL since `Journey.Stillness` is never true, **but stamps `stillness_owner_id` with the ship owner on every ship dispatch**). |
| `TradeJourney.Thread/Stillness`, `departureInterceptRadius`, `nullableBlessingTick` | nothing sets the fields; readers `Validate`, `scanRadius`, `insertRow`. `DepartureChecked` is a different, live field - kept. |
| `tools/religion_beta_*.py` | only departure prayers (Thread/Passage). |
| `tools/silver_calibration_live.py` | partly live (goods drawer, three live prayers); Thread/Passage parts dead. |
| `rites.md` "Ares' Fury (Akhaier)", "unused departure gift", "one awaiting departure" | dead text. |
| `verb_telemetry.js RITES` | live: whitelist for the `rite` telemetry property (`include('rite', body.prayer, RITES)`), but held 18 stale culture ids. |

No item was reachable from a live player path.

## Live counts (CT 126, read-only SELECTs)
- `units`: `stillness_depart_tick` set 0, `stillness_arrive_tick` set 0, `stillness_owner_id` set 5 (of 6 units): the owner stamp from `insertRow`, never read without both ticks (reader required all three equal), so semantically dead.
- `transports.journey` containing `"stillness"`: 0, `asterion_thread`: 0, `departure_checked`: 0 (of 0 rows in `transports`).
Old event payloads untouched.

## Removed (commits)
1. `religion/offering.go`: the OfferMod family and its constants (`IncenseKey` too: only `IncenseShare` read it). Kept `OfferingWorth`, `affinityDefault`, `OfferingShortfall`, `TraditionalBaseline`. Header rewritten. Tests of removed functions deleted; `TestOfferMod_IncenseLiftsTheCeiling` replaced by `TestOfferingWorth_IncenseCountsForMoreThanGrain` (real taste tables).
2. Mig 257 `drop_stillness` (down re-adds the three columns). Readers in `sea_storm.go`, `speedup.go`; NULL-resets and `$22-24` in `march_start.go`; `insertRow` ship UPDATE and `nullableBlessingTick`; `departureInterceptRadius` (`scanRadius` now returns `ThreadInterceptRadius` for a Thread-covered caravan, else `interceptRadius`: same values as before); `TradeJourney.Thread/Stillness` (`Validate` now accepts only factor 1000, a speed-blessed journey keeps factor 1000). Tests: `combat/departure_prayer_test.go` deleted (both tests only pinned the stillness stamp), the "protected voyage" subtest in `blessed_hull_test.go` and the `Stillness` case in `storm_encounters_test.go` removed.
3. Tools: `religion_beta_live.py`, `religion_beta_mutations.py` deleted (+ their section in `tools/README.md`); `silver_calibration_live.py` cut: Thread/Passage recipes, names, the desktop click-through, the "awaiting next departure" assertion. **It was not run** (needs docker + Firefox) and holds other pre-existing staleness I did not chase (e.g. recipes/ids from the silver era, "Britomartis" check).
4. Codex: `rites.md` three clauses; `articles.json` regenerated (`tools/codex_facts.py`, `--check` clean). `codex.test.mjs` 37 pass / 0 fail. `the-thread.md` line "The older The Thread Shows a Path ... can no longer be offered" left on purpose (true, explains the retired prayer).
5. `verb_telemetry.js RITES`: replaced by the 16 live ids from `PrayerSpecs`; test fixture id updated. `verb_telemetry.test.mjs` 77 pass / 0 fail.

## Mutant
In `favoursByEffect`/the sea archetype set the incense taste to 0.1 (4 values): `TestOfferingWorth_IncenseCountsForMoreThanGrain` RED for both prayers (`incense 3.0 vs grain 18.0`, `3.0 vs 30.0`); restored, GREEN.

## Proof
See final report: `go build ./...`, `go vet ./...` clean; full `tools/gotest.sh` result below.

## Left
- `departure_checked` / `TimeFactorMilli == 750` checks in `transport/speedup.go`, `departure.go`: harmless; `TimeFactorMilli` can no longer be 750 (not touched).
- `EffectBattleFrenzy` (shared archetype key) as in the previous slice.
- Pre-existing staleness in `silver_calibration_live.py` (above).

## Vault files that mention removed code (Opus edits)
`megaron_rokelse_plan_20261019.md`, `megaron_religion_sammanstallning_20261012.md`, `megaron_todo.md`, `temenos_prayers_komposition_plan.md`, `megaron_styrande_beslut.md`, `temenos_plan_graveyard/megaron_kult_legibilitet_plan.md` (terms: OfferMod, ComposedOfferMod, IncenseShare, stillness_*, religion_beta, asterion_thread).
