# Stormar 3a — stormar föds och dör (bevispaket)

Order: `.agents/order-stormar-livscykel-3a-20261107.md`. Gren `stormar-livscykel` (från master 98feb3d2), migration **248**. Inventering och
avvikelser: `inventory.md`. Inga böner (3b). Ingen ny notiskind (stormarna syns på kartan).

## Kriterium -> test
| Kriterium | Test |
|---|---|
| Livstid: spår t.o.m. T-1, inget från T, `died_tick` sätts, raden kvar | `TestStormLifecycle_DeadStormWritesNoMoreTrackAndKeepsItsRow` (combat) |
| Livstidstärning 200–300, spridd | `TestStormLife_IsRolledWithinTheRange` |
| Födsel: förväntat antal med fast tärning | `TestStormBirthCount_IsWantOverMeanLifeWithRemainderAsProbability` |
| Födsel: snitt inom ±20 % av `want` över 3000 tick (full och tom start, want 4 och 10) | `TestStormLifecycle_AverageStaysAtWant` |
| Födsel genom riktiga skanningen, lagrad livstid, dubbelskanning slår inget | `TestStormLifecycle_ScanBirthsNewStormsWithStoredLife` |
| Ny värld: spridda `dies_tick`; alla döda -> ingen omseed | `TestStormLifecycle_NewWorldSeedsSpreadAgesAndNeverReseeds` |
| Sen skanning = dagliga skanningar med dödsfall och födslar i intervallet (36 stormar, 30 tick) | `TestStormLifecycle_LateScanEqualsDailyScansAcrossDeathsAndBirths` (+ äldre `TestSeaStormWeather_LateScanEqualsDailyScans`) |
| Form: StepStorm på 1 och 2 hex sammanhängande, aldrig på land, muterar inte indata | `TestStepStorm_OneAndTwoHexStormsStayConnectedAndOnSea`, `TestStepStorm_DoesNotMutateItsInput`, `TestStormShape_ConnectedForEverySize` |
| `latestStormShape` godtar 1..3, vägrar 4 hex och slotthål | `TestLatestStormShape_ReadsOneToThreeHexesAndRejectsTheRest` |
| `/storms`: död storm inte live | `TestStorms_DeadStormIsNotLive` |
| `/storms`: ihågkommen död storm kvar utom sikt, glöms när platsen ses | `TestStorms_DeadStormMemoryIsForgottenOnlyWhenThePlaceIsSeen` |
| Omen: död storm inget faktum | `TestOmenFacts_DeadStormIsNotAFact` (kharis) |
| Namn på död storm -> 409 | `TestNameStorm_ADeadStormCannotBeNamed` |
| Webb: 1 och 2 hex ritas, blixt alltid i stormen | `render/storms.test.mjs` (2 nya) |
| Migration 248: spridda `dies_tick`, backfyll, down | `migration_check.sh` (körd; distinct=36 av 40, storm_weather_tick=1000) |

## Mutanter (lagrad regel: röd på `_test.go:NN`, inte `[build failed]`; committat före, återställt efter)
| Mutant | Utfall |
|---|---|
| M1 död storm får fortsatt spår (`t >= s.dies` borttagen) | DÖDAD: storm_lifecycle_test.go:265 och :419 |
| M2 `/storms` utan levande-filtret | DÖDAD: storms_fow_test.go:193 |
| M3 födelsechans x0 | DÖDAD: storm_lifecycle_test.go:54, :99, :345, :416 |
| M4 `ensureStorms` räknar bara levande (seedar om när alla dött) | DÖDAD: storm_lifecycle_test.go:322 |
| M5 `ensureStorms` ger alla samma `dies_tick` | DÖDAD: storm_lifecycle_test.go:310, :356 |
| M5b migrationen ger alla samma `dies_tick` (`random()` -> 0) | DÖDAD via script (migrationer körs före testerna): distinct=1 mot 36 |
| M-A (Opus) `UPDATE worlds SET storm_weather_tick` blir no-op | DÖDAD: storm_lifecycle_test.go:455 (`TestStormLifecycle_DuplicateScanWithNoLivingStormDoesNotRollBirthsAgain`) |
| M-B (Opus) nyfödd storm seedas utan `used` (kan stå på levande storm) | DÖDAD: storm_lifecycle_test.go:495 (`TestStormLifecycle_NewbornNeverOverlapsALivingStorm`) |
| M6 omen utan levande-filter | DÖDAD: omen_truth_test.go:259 |
| M7 namnge död storm tillåts | DÖDAD: storm_names_test.go:150 |

Ett riktigt fynd under bygget: `/storms` itererade bara över stormar med "nu"-spår, så en död storms minne hade försvunnit helt;
fångat av testet ovan innan mutationerna, rättat i `storms.go`.

## Ytor
temenos (mig 248, combat, handlers) · keryx `storms` (bara hjälptext: levande + minne; inget nytt kommando) · webb (kartritaren, blixt-index `% hexes.length`) ·
Codex `storms.md` ("lives some 200 to 300 game days", död storm minns men är borta).
`megaron_verblista` rad för stormar/döp uppdaterad; `megaron_plan_stormar` har "BYGGT".

## BILD-vyer (Timothy dömer efter merge)
Ingen ny grafik. Att titta på i Firefox 1:1: en storm som krympt till 1–2 hex (3b) ritas med samma kontur; en död storms ghost-moln kvar tills platsen ses.

## Sviten
Helkörning `tools/gotest.sh` (GOTEST_PORT=56912, färsk DB mig 248): Go alla paket ok, webb 915 tester, 915 pass, 0 fail (efter omgenererad Codex-bundle).
