# dod-regelkod - dead rule code and goods that cannot exist (2026-11-10)

Base 1e1d7378, branch `dod-regelkod`, three commits (A, B, C). Net 36 files, +106 / -971 lines. See inventory.md.

## Part A (-731 / +21 in commit 43611fb1)
Removed: `templeOfferScarcity`, `lockedTempleScarcity`, `clampTempleScarcity`, `templeScarcityCeil`, `scarcityFromValues`, `templeOfferingRecipe` (kharis); `populationDecay`, `DecayProjection.Residents` and the API field `kharis_population_decay`; the `scarcity_factor` field in new KharisOfferingV2 events (old events keep it, nobody reads it); `RecomputeDivineValuations`, `LoadScarcityDivineValues`, `InitializeLaborValuations` (and its calls in main.go and world creation); all of `religion/valuation.go` and its test; the SQL threshold test.
Kept: `FixedDivineValues`/`LoadDivineValues` (the real god price list), `LoadLaborValues`, the DB tables `divine_labor_valuations` and `divine_valuations` (no migration; they are now only written by test fixtures).
Proof of unchanged outcome: `TestSilverCalibrationKharisFlowLeavesStalePriceClamp` (kharis/labor_flow_test.go) pins the projected net of a staffed, fed L1 temple (`2*(KharisPerTempleWeight/2) - KharisPerDemand`) and that the persisted tick equals it; it is green and its expected number is unchanged. The test lines asserting the factor equals 1 were deleted with the factor.

## Part B (commit 127c6b77)
Removed: the 15 `akhaier_/khemetiu_/knaani_/thrakes_/hatti_*` specs, their `CulturePrayers` rows, the `culture+"_battle_frenzy"` fallback; keryx `rite --offer-multiplier` (flag, request field, confirm line). Added a keryx test that an old RiteAnswered notification naming a retired id renders its stored name.
Kept and why: (1) the server `offer_multiplier` field and the whole godless "dice" path in the rite handler, because three unreachable `minoan_*` legacy specs and several tests still exercise it - tests that need the old akhaier prayers register them in `api/handlers/legacy_prayer_fixture_test.go`; delete that file together with the path in a follow-up. (2) Departure prayers (`minoan_asterion_thread`, `minoan_stillness_passage`, `departure.go`): pending `departure_prayers` rows may exist in a live DB.
Risk to read: a live settlement with a pre-2026-08-05 non-minoan culture value would lose its rites. Check `SELECT culture_id, count(*) FROM settlements GROUP BY 1` on the live DB before deploy.

## Part C (commit 34484d14)
All three goods have no source (inventory.md). Removed: taste-table entries, three Go constants, web telemetry GOODS entries, "horses" in the plains tooltip, regenerated reference.json and rites.md/articles.json. Board editor now shows a stored unknown good as written (new node test). Kept: the horses storage exemption, DB rows.

## Not mine / found
- `TestKharisPlayerRulesSourceParity` fails on master: the Codex kharis article (commit 379b8cb0) no longer contains "**40 Kharis**" and "Population does not contribute".
- Full suite (`tools/gotest.sh`, fresh DB, migration 255): every package ok except api/handlers `TestKharisPlayerRulesSourceParity`; web 960 pass / 2 fail (`Day guard codex` on marching.md:11, `retired Forces labels`). The two web failures reproduce on master 1e1d7378.
