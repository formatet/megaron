# Vägar V1 — substrat: lagring + kantkostnad (2026-10-15 17:14)

Plan: `myltavault/megaron_plan_vagar.md` V1. Märkning: **blockerar** V2/V3 (substrat). Inget spelarsynligt.

**Problem.** Världen kunde inte ha väg-kanter och rörelsemotorn prissatte bara målhexens terräng.
**Lösning.** Tabell `road_edges` (mig 180), `province.RoadSet`/`EdgeCost`, och alla prissättare går genom EN kantmedveten kostnadsfunktion.

## Vad
- `server/db/migrations/180_road_edges.{up,down}.sql` — kanonisk kant (CHECK `(q1,r1)<(q2,r2)` + grannskap), FK `worlds` CASCADE, `built_by_wanax` FK `players` **ON DELETE SET NULL** (vägar står kvar om kontot raderas).
- `server/internal/province/roads.go` — `RoadSet`, `roadKey` (F6), `LoadRoadSet`, `LoadWorldGraph`, `EdgeCost`, **en** faktortabell `roadFactors` (land 0,75 · courier/courier_land 0,70 · caravan 0,60), `CategoryCaravan`, `TradePricingCategory`, `IsRoadableTerrain` (för V2).
- `pathfind.go`: `TileGraph.FindPath/StepHours` och `PlanTradeJourney` tar `roads RoadSet` (kompilatorn tvingar varje anropare — F2). Paketnivåns `FindPath`/`StepHoursDB`/`PlanTradeJourney(ctx…)` laddar vägarna själva (alla ~40 anropare går därmed rätt utan ändring). Heuristiken `minPassableCost(category, roads)` skalas med kategorins vägfaktor **bara när vägar finns** (F1) — utan vägar är beräkningen bit-identisk med förut.
- Anroparna som laddar graf själva byter till `LoadWorldGraph`: combat (expedition, expedition_outcomes, march_encounter, march_sighting, march_start), messenger (`CourierTravelOnGraph`, `InterceptAlongPath*`, `interceptScan` får `roads`-parameter), handlers (world, unit, foreign_units, messenger_passage).
- Karavan: land-resor prissätts i klassen `caravan` (0,60); `TradeJourney.Category` förblir `"land"`.
- Skepp/storm: `nil` roads (sjö påverkas inte; `LoadTileGraph` kvar i storm/siege/oracle/intercept som bara slår upp terräng).

## Tester (alla nya, `internal/province/roads_test.go`, `internal/combat/roads_march_test.go`)
(a) `TestRoadDetourIsChosen` — omväg 5,5 (väg 4,125) slår direkt 5. (b) `TestRoadKeyIsUnordered`. (c) `TestEdgeCostIgnoresNonRoadableEndpoints` — ford/flod/hav/berg åt båda håll. (d) `TestEdgeCostFactorsPerCategory` (hårdkodade förväntningar), `TestPlanTradeJourneyUsesCaravanRoadFactor`, `TestRoadEdgesTableAndLoad` (DB-rader → A*, StepHoursDB, PlanTradeJourney; CHECK avvisar oordnad kant/icke-grannar/dublett). (e) `TestRoads_StartMarch_SavesRoadScaledCosts` — riktig `StartMarch` över `road_edges`-rader: duration 3→2 ticks, sparade steg 1000→750 milli; en tidigare startad marsch behåller 1000 (F3); `TestRoadEdgesTableAndLoad` visar att en redan planerad karavanresa är oförändrad när en kant tillkommer. (f) `TestAStarStaysOptimalWithRoads` — 40 slumpgrafer × 4 kategorier × 12 par mot Dijkstra-orakel. Bevis "inga vägar ⇒ identiskt": `TestNoRoadsIsIdenticalToTerrainPricing` (nil vs tom mängd: samma väg/kostnad; kostnad = gamla terrängsumman).

## Mutationsprov (kör via scratchpad-`mutate.py`; alla RÖDA, ingen överlevde)
heuristik tappar faktorn → `TestAStarStaysOptimalWithRoads` · land 0,75→0,80 · courier 0,70→0,71 · courier_land 0,70→0,71 · caravan 0,60→0,61 · ford vägbar · vägbarhet bara på mål / bara på källa · icke-kanonisk nyckel · karavanklass bortkopplad · EdgeCost läser källterräng · `StepHours` ignorerar vägar · A*-kostnad ignorerar vägar · (DB) `StepHoursDB` ignorerar vägar · (DB) paketets `FindPath` ignorerar vägar · (DB) `LoadRoadSet` laddar inget. Ett första försök på `FindPath`-mutationen överlevde (testet kollade bara steg-kostnader) → `DurationTicks` lades till i testet, sedan rött.

## Svit
`tools/gotest.sh` på nyskapad DB (migration 180): alla paket `ok` (api/handlers, cmd/*, internal/* inkl. combat, province, messenger, transport, world). Baslinjen (commit a11d7766) kördes på egen färsk DB: allt grönt utom `cmd/server TestHealthz_ReportsCommitAndMigration` — det var ett **artefakt** (jag skapade mig 180 medan baslinjen kördes: filen fanns men DB:n låg på 179); samma test är grönt i slutsviten.

## Flaggor / öppet
1. **Läs-tids-omräkning (F3, befintlig design):** `march_encounter`, `march_sighting`, `foreign_units`, `world.go` (marsch/karavan/bud utan sparad rutt) kör fortfarande A* vid läsning för att tolka position. De använder nu *aktuella* vägar. Om en väg byggs mitt under en redan startad marsch kan den räknade positionen avvika från den frusna rutten. Gäller bara marscher utan `march_route` (äldre/fallback) eller sightings; ny väg under färd är sällsynt i v1. Fixas rent av att läsarna går via sparad rutt — utanför V1, nämner.
2. `unit_arrival.go` ~1408 (rak-linje-fallback när FindPath misslyckas, dvs ingen väg alls) prissätts utan vägar medvetet — det finns ingen väg att ta ut.
3. **Ej gjort:** "bit-identiska rutter mot master i acceptansvärlden" — acceptanscontainern får inte röras av utföraren. Ersatt med egenskapstestet ovan + hela sviten; Claude kan köra jämförelsen i V4/acceptans.
4. Gren: worktree-grenen behöll sitt namn `worktree-agent-aaf3c92b1937dca46` (inte omdöpt till `claude/vagar`).
5. Migration 180 deployas inte av `air` (V5: `systemctl restart poleia`).
