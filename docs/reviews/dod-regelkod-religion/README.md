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

### Item 6 - RestTicks float noise: see below.
