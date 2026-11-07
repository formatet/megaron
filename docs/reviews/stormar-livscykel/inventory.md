# Inventering — läsare av stormar (master 98feb3d2, före ändring)

Grep: `sea_storms|sea_storm_track|StormShape|StormHexes|ensureStorms|advanceStorms` över server/, cmd/, web/static/js, tools/.

| Läsare (fil:rad före ändring) | Antar exakt 3 hex? | Antar "alla stormar lever"? | Åtgärd |
|---|---|---|---|
| combat/sea_storm_weather.go `StormShape [3]`, `centroid` (/StormHexes), `Connected` (>=2 par), `StepStorm` (slot < StormHexes), `latestStormShape` (n != 3) | ja | ja (`advanceStorms` gick över alla rader) | omskriven: slice 1..3, generell sammanhang, levande-filter |
| combat/sea_storm.go:527 `loadStormOccupancy` (träffar per tick) | nej (per hex-rad) | nej, läser spår per tick, död storm har inga rader från dies_tick | verifierad, orörd |
| combat/sea_storm.go:578 `recomputeStormFishing` (tick IN (t-1,t)) | nej | nej | verifierad, orörd; storm som dör vid t har spår t-1 och omräknas (rätt: fisket släpps) |
| combat/sea_storm.go:606 `claimStorm` | nej | storm träffar skepp, alltså levande | orörd |
| api/handlers/storms.go:66–71 `max(tick)` per storm | nej | **ja — död storm hade visats live för alltid** | filter `died_tick IS NULL AND dies_tick > tick` |
| api/handlers/storms.go:101, 209 (minne, namn) | nej | **ja i loopen**: utmatningen gick bara över stormar med spår "nu", så en död storms minne hade försvunnit helt | minnesstormar utan live-rad läggs till i loopen (sorterade på id); fångat av testet `TestStorms_DeadStormMemoryIsForgottenOnlyWhenThePlaceIsSeen` |
| api/handlers/storm_names.go:94 NameStorm | nej | **ja** | död storm -> 409 (finns, men kan inte namnges längre; inte 404 eftersom id:t är känt och giltigt). Admin-namn oförändrat. |
| kharis/omen.go:212 `max(t2.tick) <= current` | nej | **ja — samma fel som /storms** (senaste spåret på döda storm kvarstår) | filter `died_tick IS NULL AND dies_tick > current` |
| economy/storm_fishing.go:21,44 (`tick = current_world_tick()`) | nej | nej (exakt tick) | verifierad, orörd |
| cmd/scrub-coarse-names | nej | nej (bara name) | orörd |
| web render/storms.js: `% 3` för blixt-hex | **ja (bugg för 1–2 hex)** | nej | `% storm.hexes.length` + test |
| web render/storms.js `stormOuterEdges`, hover.js, state.js, march_preview.js | nej | nej | verifierade (testade 1 och 2 hex) |
| keryx cmd_storms.go | nej (loopar hexes) | server filtrerar | bara hjälptext |
| Codex storms.md, sea.md | text "tre hex" | text | storms.md uppdaterad; sea.md ("A storm is three sea hexes") lämnad, sant vid födsel |
| tools/*.py (omen_dimma, storm_hex_repro, byggplan, bildsvep) | skriver 3 spårrader (giltigt) | skriver INSERT utan dies_tick | **ja behövde ändras**: dies_tick är NOT NULL utan default; lade till `dies_tick=100000` |
| 9 Go-testfixturer med `INSERT INTO sea_storms` | – | – | samma tillägg `dies_tick` |
| docs/reviews/**/fixture.sql (13 historiska) | – | – | orörda (arkiv, körs inte av sviten) |

## Avvikelser från ordern (att granska)
1. **Ny kolumn `worlds.storm_weather_tick`** (ordern nämnde den inte). Skäl: födelsetärningen "ingen födelse" lämnar inget spår, så en dubbelskanning skulle slå om samma tick, och en värld där alla stormar dött har inget spår att förankra "senast skannade tick" i. Utan den går korrekthet "sen skanning = dagliga" inte att garantera.
2. `advanceStorms` går nu **tick för tick** (alla stormar steg, sedan födslar) i stället för storm för storm. Krävs för att tärningsordningen ska vara identisk mellan daglig och sen skanning när det finns flera stormar och födslar. Stormar ordnas `created_tick, första hexens q,r, id` (id är slumpat; födslar samma tick ska ändå få stabil ordning).
3. Seed-ålder: `dies_tick = tick + 1 + rand(0..livstid-1)` (ordern: `tick + rand(0..livstid-1)`); +1 så ingen storm föds död.
4. Migrationen: `dies_tick = current_tick + 1 + floor(random()*300)`, samma skäl.
5. `StormShape` blev en **slice** (inte array+längd): längden *är* storleken, litteraler `StormShape{{..}}` fungerar kvar, `clone()` krävs vid steg (aliasering). Prislapp: `==` på former finns inte längre (testerna använder `sameShape`).
6. `died_tick` sätts till `dies_tick` (dödstickens värde), inte skanningens tick, så att en sen skanning ger samma värde.
7. "Ingen storm någonsin": `ensureStorms` villkoret `count(*) > 0` är oförändrat och redan korrekt, eftersom döda rader aldrig raderas. Mutanten ändrar det till "levande" och faller.
