# Dead rule code in religion (order 2026-11-17)

Branch `dod-regelkod-religion`, from master 7c12d0a3. Removal only.

## Step 0 - inventory (before any code change)

Live facts: every `PrayerSpecs` entry registered in `prayers.go` `init()` has a `GodID`. The only godless
specs are the three `minoan_*` map entries (item 2) and, in tests only, the `akhaier_*` fixture. `CulturePrayers`
lists no godless spec for minoan. `DefaultBattleFrenzyFor` returns `PrayerPotniaWalls` (godded). So the
godless/dice branch of `Rite` is reachable only from tests.

### Item 1 - offer_multiplier / godless dice branch
| Reference | Class |
|---|---|
| `settlement.go` `riteOfferMultiplier*`, `riteOfferMod*`, `riteSuccessChance`, `riteFloor/Ceil`, request field `OfferMultiplier`, `scaleOffering(...,offerMultiplier*scope)`, dice roll, `spec.GodID==""` cooldown (events-based), `offerMod/offerCap` verdict branches | production, reachable only for godless specs -> unreachable in production |
| `offer_multiplier` in response + RiteCast payload (`offer_multiplier`, `offer_mod`) | produced; no reader in web/, web/static/codex/, tools/, cmd/keryx (grep) |
| web / keryx / codex sending `offer_multiplier` | none (keryx sends `offering` only) |
| `ComposedOfferMod` (religion/offering.go) | production caller only in the godless branch; test-only after removal |
| tests: `settlement_rite_math_test.go`, `settlement_rite_offering_test.go`, `legacy_prayer_fixture_test.go`, `rite_relation_test.go` TestRelationLegacyRoll..., `religion_alpha_test.go` TestBattleFrenzyWrites..., `oracle_reveal_map_test.go` | test only |

### Item 2 - minoan_oracle_deposits / minoan_harvest_blessing / minoan_battle_frenzy
No `CulturePrayers` entry, no god, no temple route offers them (rite returns 403 "not available to culture").
References outside `prayers.go`: tests (`prayers_test.go`, `alpha_test.go`, `rite_harvest_outcome_test.go`), the stale
web whitelist `verb_telemetry.js RITES` (+ its test), tools/bon_*.py strings. Orphan candidates:
`EffectOracleRevealDeposits` (const, handler case, `applyOracleRevealDeposits`, favoursByEffect key) - orphan.
`EffectBattleFrenzy`, `EffectHarvestBlessing` - still used (favour archetypes, GreenShoots, garrison SQL) - KEPT.
`potniaTheronOracleFavours` used by PrayerPotniaVision - KEPT. `havsdaimonernaBattleFrenzyFavours` used by sea prayers - KEPT.

### Item 4 - departure prayers
Creator: `Rite` case `EffectNextLandDeparture/EffectNextShipDeparture` - no `PrayerSpecs` entry has those
effects (Thread/Passage were retired in 2b), so unreachable. Readers: `religion.DeparturePending/ConsumeDeparture`
from `transport.BlessDeparture`, `messenger.BlessRunner`, `combat/march_start.go`, `PrayerActiveRemaining`,
`cult_prayers.go` (`departure_pending`), keryx `departure_pending`. Live row count: see below (0).
`DepartureTimeFactor` is NOT dead: speed blessings, `BuildTimeFactor`, payloads use it -> kept (moved to speed.go).
`TradeJourney.Thread/Stillness` and `units.stillness_*` columns carry persisted in-flight data: left alone.

### Item 5 - tables
`departure_prayers` (176) only. Migrations 029, 193, 234 create no tables (only columns) -> nothing else orphaned.
Live (CT 126, read-only `SELECT count(*)`): `departure_prayers` = 0 rows.


## What was removed (commits on the branch)

- **Item 6** `religion.RestTicks`: product rounded to 1e-9 before `Ceil`. New table test
  `TestRestTicksGiftStepsExactDespiteFloatNoise` (cooldown 6/12/24/25 x gift step 1/2/3/5 x level 1-3, expected by hand
  in integers, r fed as N and one ulp either side). `TestGiftStepsGiveNTimesTheLength` tightened from "+1 at most" to exact.
- **Items 1+2+3** `settlement.go`: `offer_multiplier` request field, `riteOfferMultiplier*`, `riteOfferMod*`,
  `riteSuccessChance`, `riteFloor/Ceil`, the dice roll and its "gods are silent" else branch, the events-based cooldown
  for godless prayers, `offerMod/offerCap`, the non-everyday verdict branches, `applyOracleRevealDeposits` (+ its case),
  `math/rand` and `errors` imports. `offer_multiplier` and `offer_mod` are no longer written to the RiteCast payload or
  the response (no reader anywhere in server/web/codex/tools; a stray `offer_multiplier` in a request body is ignored).
  `prayers.go`: the three `minoan_*` specs, `EffectOracleRevealDeposits`, its `favoursByEffect` entry.
  Tests deleted (legacy only): `legacy_prayer_fixture_test.go`, `settlement_rite_math_test.go`,
  `settlement_rite_offering_test.go` (`riteTestPool` helper moved to `religion_alpha_test.go`), `oracle_reveal_map_test.go`
  (4 tests of `applyOracleRevealDeposits`), `TestRelationLegacyRollAndMultiplierWithoutStandingDebit`,
  `TestOracleDepositsDescriptionNotNearby`. Rewritten to the live path: `TestBattleFrenzyWritesOwnerStampedGarrisonDefense`
  -> `TestWallsWritesOwnerStampedGarrisonDefense` (casts `minoan_potnia_walls`, same 106/owner/6/0/0 assertions),
  `TestSeaGods_*` (now Potnia's Vision and Stout Hulls), `TestRelationEverydayExact...` (keeps a stray
  `offer_multiplier` in the request and still proves it is ignored), oracle tier gate tests.
- **Item 4** `religion/departure.go` deleted (`DeparturePending`, `ConsumeDeparture`); `EffectNextLandDeparture/ShipDeparture`
  gone; `DepartureTimeFactor` kept (live: speed prayers, builder's blessing) and moved to `speed.go`;
  `transport.BlessDeparture` now only applies a live speed blessing; `messenger.BlessRunner` -> `messenger.RunnerArrival`
  (pure time maths, callers in `messenger.go`/`unit.go` lost an error branch that could no longer fire);
  `combat/march_start.go` lost the ship stillness consumption; the Rite apply case and the "awaits your next departure" 409;
  `departure_pending` removed from `cult_prayers.go`, `province.go` prayerRow, keryx (`cmd_rite.go` + test), web `kult.js`
  (+ 2 tests), Codex (`rites.md` section "Gifts awaiting departure" and its anchor `minoan-prayers-3` in index.json and
  `codex.test.mjs`; "a departure prayer" clauses in `swift-feet.md`/`following-seas.md`; `articles.json` regenerated with
  `tools/codex_facts.py`). Tests deleted: `religion_departure_test.go`, `transport/departure_test.go`,
  `TestSpeedUpTransport_DepartureSpedIsLeftAlone`, three departure tests in `combat/departure_prayer_test.go`;
  `transport_options_test.go` lost its departure-prayer row but still proves the preview is read-only.
- **Item 5** migration `256_drop_dead_religion`: `DROP TABLE departure_prayers`; down recreates it empty. Up/down/up
  verified on a fresh DB. Live count read-only on CT 126: **departure_prayers = 0 rows**. 029/193/234 create no tables.

## Left on purpose (for Opus to decide)

- `units.stillness_depart_tick/arrive_tick/owner_id` (mig 176 columns), their readers (`combat/sea_storm.go`,
  `combat/speedup.go`), the NULL-resets in `recall_redirect.go`/`unit_arrival.go`/`ship_hull.go`, `march_start.go`
  `stillnessDepart/Arrive` (now always nil), `transport.insertRow` `nullableBlessingTick`, `TradeJourney.Thread/Stillness`
  and `departureInterceptRadius`: nothing sets them any more, but persisted in-flight journeys/ships may carry them and
  `blessed_hull_test`/`storm_encounters_test`/two tests in `combat/departure_prayer_test.go` still pin the storm reader.
  A follow-up order could drop the columns after an in-flight check.
- `religion/offering.go` `OfferMod`, `OfferModWithIncense`, `OfferModCap`, `IncenseShare`, `DistinctGoods`, `ComposedOfferMod`
  and their tests: the success-chance nudge has no production caller now (the dice are gone); they are test-only.
  Not removed: outside the order's file range and they carry the 2026-10-19 incense decision.
- `EffectBattleFrenzy` and the `battle_frenzy` handler/catalogue branches (`cult_prayers.go`, `settlement.go` garrison case,
  `PrayerActiveRemaining` SQL): no spec uses the effect any more, but it is the key of the shared `favoursByEffect`
  archetype that live prayers use, so the constant stays.
- `web/static/js/megaron/verb_telemetry.js` `RITES` whitelist (18 stale ids incl. the three removed specs) and its test:
  already stale before this slice (holds no live prayer id); left.
- `tools/religion_beta_mutations.py`, `tools/religion_beta_live.py`, `tools/silver_calibration_live.py`: old
  checkpoint scripts that exercise departure prayers; they will fail if run now.
- `codex/rites.md` still says "Ares' Fury (Akhaier) works like ..." (dead culture text, outside this order).

## Vault files that mention removed code (Opus edits)
`megaron_plan_ritens_utfall.md`, `megaron_styrande_beslut.md`, `megaron_plan_varukatalogen.md`, `temenos_movement_motor.md`,
`temenos_ekonomi.md`, `megaron_bonesystem_v3_20261031.md`, `megaron_todo.md`, `temenos_prayers_komposition_plan.md`,
`megaron_bonernas_oversyn.md`, `megaron_ai_spelning_20261010.md`, `rapport_speldygnstest_20260831.md`,
`temenos_plan_graveyard/megaron_kult_legibilitet_plan.md` (terms: offer_multiplier, departure_prayers, departure_pending,
minoan_oracle_deposits/harvest_blessing/battle_frenzy, riteOfferMod, BlessRunner).

## Mutants
- (a) revert item 6 rounding to a plain `math.Ceil(rest)`: `TestRestTicksGiftStepsExactDespiteFloatNoise` and
  `TestGiftStepsGiveNTimesTheLength` RED (`RestTicks(24, 3.0000000000000004, 2) = 37, want 36`); with the fix GREEN.
- (b) live gift-step rite: in `Rite`, `giftFactor = religion.GiftFactor(...)` -> `giftFactor = 1.0`: RED in
  `TestBlessingCast_WiderTargetNeedsTheWiderGift`, `TestGiftSetsPrayerLengthWithCap`,
  `TestTempleRestShrinksWithLevelAndGrowsWithGift`. Mutants restored; tree clean.
