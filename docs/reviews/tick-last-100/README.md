# Tick-last vid full värld (2026-11-22)

**Fråga:** klarar dygnstickens hanterare en full värld? Varje hanterare hade en hård gräns på 5 s
(`events.DefaultHandlerTimeout`). Efter tre misslyckanden läggs händelsen åt sidan, och då står världens
förfall, svält, kharis och dagbok still.

**Rigg:** acceptansvärlden, 230×230, `TICK_SECONDS=60`, 100 Wanax registrerade och anslutna. Var och en har
grundat sin första stad via `POST /founding/settle` (100 städer, 1000 invånare var, startplaceringar,
inga arméer i rörelse). Tider ur en tillfällig logg i `Worker.dispatch` och deltider i `KharisTick`. Båda
mätpatcharna är borttagna; kvar finns bara varningen över en femtedel av timeouten.

## Mätt per dygn
| hanterare | ms |
|---|---|
| KharisTick | 3913 (första dygnet), sedan 1610–1659 |
| UpkeepTick | 400–420 |
| SeaStormScan | 180–410 |
| FoodTick | ~128 |
| PlaceReachScan | ~60 |
| övriga scanningar | < 20 |

KharisTick uppdelad (ett dygn, 1659 ms): underhåll per Wanax 281 · tempelvila + förfall/tillväxt 845
(loop per stad: `PrunePlacementsToPopulation`, `RecomputeProduction`, `SyncHexBlockade`, `SyncSiegeState`) ·
förrådsobservation 338 · matobservation 101 · dagbok 56 · svält 31.
Ungefär **14 ms per stad**. Med 5 s tak räcker det till ungefär 350 städer, alltså 100 Wanax med 3–4 städer var.
Riktiga städer (fler byggnader, enheter, tempel) kostar mer. Tabellskanningar per dygn är måttliga, så tiden går
till antalet frågor per stad, inte till saknade index.

## Åtgärd
`Worker.RegisterWithTimeout` och `events.WorldTickHandlerTimeout` (60 s) för KharisTick, UpkeepTick och FoodTick.
Dispatch sker i följd i kanonisk prioritetsordning, så dygnets ordning är orörd. En lång hanterare skjuter bara
nästa händelse i samma tick några sekunder. `slow scheduled event handler` loggas över en femtedel av respektive
timeout (1 s för vanliga, 12 s för världstickarna).

**Prov:** `TestWorker_TimeoutForRegisteredHandler` och `TestWorker_DispatchUsesRegisteredTimeout`. Den senare läser
handlerns faktiska deadline. Mutanten där dispatch använder standardtimeouten dödades (deadline 4,99 s, rött).
Hela `tools/gotest.sh` grön. Riggen med slutlig kod: dygn 14 klart på första försöket för alla tre, 0 ERROR.

**Kvar (inte byggt):** det skalbara greppet är att dela upp arbetet per stad i egna händelser. Det rör dygnets
kanoniska ordning (`megaron_tickordning`), så det tas när slow-loggen visar att världstickarna närmar sig 12 s.
