# Inventory (written before editing, base 1e1d7378)

## Part A - scarcity factor
- `templeOfferScarcity` (kharis/tick.go) returned the constant 1; readers: tick.go gain line, the `scarcity_factor` field of the KharisOfferingV2 payload (no reader anywhere in Go/SQL/web/keryx/Codex; only tools/silver_calibration_migration.py writes a fake one), one test.
- `clampTempleScarcity`, `templeScarcityCeil`: readers only `scarcityFromValues` and devotion_test.
- `scarcityFromValues`: readers only tests. `lockedTempleScarcity`: project.go + tick.go.
- `populationDecay` returned 0; carried by `DecayProjection.Residents`; readers: API field `kharis_population_decay` (cult.go, province.go), no web or keryx code reads it (only test fixtures), two kharis tests.
- `RecomputeDivineValuations`: NOT called every tick (the order's line 245 is `InitializeLaborValuations`, called at server boot and world creation, and only when the table is empty). Table `divine_labor_valuations` readers: only `LoadScarcityDivineValues` (readers: tests only). The god price list used by prayers, temple tithe and composed offerings is `religion.FixedDivineValues` via `LoadDivineValues`, which never touched the table.
- valuation.go (`Rarity`, `DivineValue`, `SmoothDivineValue`, `CountsAsHolder`, `goodDivisors`, constants): readers only RecomputeDivineValuations and its tests.

## Part B - cultures
- `province.NormaliseCulture` returns minoan for any input; used by join.go and found_metropolis.go. create_metropolis.go inserts the culture it is given; foundColony copies the owner's capital culture. No production path gives another culture. No migration normalises old rows (only 044, pelasger to minoan); a live DB row from before 2026-08-05 could still hold another culture value - cannot be checked from the worktree.
- Old events: RiteAnswered notifications store `prayer_name` and `name`; keryx and web render those, never look the id up in `PrayerSpecs`. No id to name table needed.
- `offer_multiplier`: forced to 1 for every prayer with a god (all 16 playable prayers). Still live only in the godless `minoan_oracle_deposits`/`minoan_harvest_blessing`/`minoan_battle_frenzy` specs, which are in no `CulturePrayers` list (unreachable in play) and in the tests.
- Departure prayers (`minoan_asterion_thread`, `minoan_stillness_passage`): `departure_prayers` rows may still be pending in a live DB and are honoured at departure; kept.

## Part C - goods
- horses, pottery, purple: `goods.status='parked'` since mig 115. Every production path (placement_yield, building_effect, combat/build, province catchment) joins `goods.status='active'`; hex_rules.go names none of them; the Go constants had no non-test reader; trade catalogue (`/api/v1/goods`) filters active, so the web market board pickers never offered them. `FixedDivineValues` never had them. No source exists, so all three are ghosts.
- Leftovers found: prayer taste tables (6 entries), `GoodHorses/GoodPurple/GoodPottery` constants, web `verb_telemetry.js` GOODS list, `map.js` plains text "grain, horses, a little oil", Codex rites table + reference.json (generated).
- Kept on purpose: the `horses` storage exemption in `province/building.go` + migration 244 (stored rows may exist, tests pin it), `goods` table rows, `production_rules` rows (no migration).
