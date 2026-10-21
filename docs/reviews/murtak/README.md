# B1 — muren som befolkningstak (gren claude/murtak)

## Kontrakt (B0, Timothy 2026-10-21)
| Mur | Tak | Kräver Granary | Kostnad |
|---|---|---|---|
| ingen | 2 000 | — | — |
| Palisade (1) | 5 000 | 1 | timber 12, stone 4 |
| Stone Wall (2) | 10 000 | 2 | timber 20, stone 40 |
| Bronze Wall (3) | 20 000 | 3 | stone 80, bronze 20 |

Granary = `BuildingSilo "silo"`, nivå 1–3, ×2/×5/×10 på lagertaket för grain/fish/oil/wine (ej livestock).
Hårt tak: full tillväxttakt till taket, sedan stopp; över taket: ingen tillväxt, ingen förlust. Ytan visar bara taket.

**Valda Granary-kostnader:** nivå 1 timber 12 + stone 8 (8 dagar); nivå 2 timber 24 + stone 16 (16 d); nivå 3 timber 48 + stone 32 + bronze 8 (24 d). Standardtrappan ×1/×2/×4, brons bara på nivå 3 (`levelBronzeFrom`) så att Stone Wall nås före bronsåldern.

## Ändrade filer
- `server/internal/economy/population_cap.go` (ny) — ENDA ägaren: `PopulationCap(wall)`, `PopulationCaps()`.
- `server/internal/kharis/tick.go` — growth-SQL: `30000` borta; tak per stad ur `$9` (= `PopulationCaps()`), tillväxt klampad till `cap−pop`, `pop ≥ cap` → håller; ingen yttre `LEAST(…,30000)` som kunde skära ner.
- `server/internal/province/{model,building}.go` — `BuildingSilo`, katalograd, `LevelledBuildings`, murkostnader, `SiloFoodStorageMult`, `SiloLevelForWall`, `WallNeedsSiloRefusal`.
- `server/internal/province/roads.go` — vägpriset läste `WallLevelSpecs[2].stone` (24); Stone Wall blev 40 så basen frystes till 24 (annars omprisades alla vägar och `roads.md` röda).
- `server/db/migrations/197_granary_storage.{up,down}.sql` — `silo_food_mult`, `silo_stored_good`, `settlement_silo_level`, BEFORE INSERT-trigger på `settlement_goods` (ny rad får rätt tak) + AFTER-trigger på `buildings` (bygge/nivåhöjning/rivning re-capar befintliga rader). `building_type` är fri TEXT, ingen enum/check att ändra. (Nummer 197 per koordinatorns besked; 196 = A1.)
- `server/api/handlers/province.go` — murgrind i `Build` (422 "Stone Wall needs a Granary of level 2"), `population_cap` + `silo_level` i province-svaret, `wall_ladder` i `GET /buildings`, tillväxtläget `capped` (`economy.GrowthStateAtWall`). `settlement.go`: `population_cap`. `web.go`: namn "Granary".
- `server/internal/economy/{food_basket,recompute}.go` — `SiloLevel`, `GrowthStateAtWall`/`GrowthCapped`.
- keryx: `cmd_status.go` (`wallsHoldLine`, growth "capped"), `cmd_build.go` (`--list` visar murtrappan med Granary-krav och tak).
- Webb: `ui/drawers/city.js` (`wallOptionHTML`: grå mur med orsak, "Granary" i etiketter), `city_production.js` ("Walls hold N", growth "capped"), `verb_telemetry.js`.
- Codex: `buildings.md` (Granary, murtaket), `growth.md`, `roads.md` (24-stenmeningen). Vault: `megaron_verblista.md` (build-raden).
- Testfixturer: `newGrowthFixture` har nu `wall_level=3` (growth-MEKANIKTESTER vid 5 000+ folk skulle annars frysas); `eaten_variety_test` likaså; `ai_round4_test` byter "wall"→"barracks" som "annan giltig stadsbyggnad".

## Tester (nya)
`server/internal/kharis/wall_cap_test.go`: `TestMurtak_PalisadeCityAtCapDoesNotGrow` (a), `TestMurtak_SameCityGrowsAfterStoneWall` (b), `TestMurtak_NewCityGrowsWhileNeighbourAtCapHolds` (d), `TestMurtak_CityOverCapLosesNoOne` (f), `TestMurtak_GrowthStopsExactlyAtCap`, `TestMurtak_FullRateRightUpToTheCap`.
`server/api/handlers/murtak_test.go`: `TestMurtak_StoneWallNeedsGranary2` (c), `TestMurtak_PalisadeNeedsGranary1`, `TestMurtak_GranaryMultipliesFoodStoreNotHerd` (e, inkl. rad född efter silon, nivå 3, rivning), `TestMurtak_SiloStorageSQLMirror`, `TestMurtak_PopulationCapTable`.
JS: `wallOptionHTML` i `city.test.mjs`. keryx: `walls_hold_test.go`.

## Red före / grönt efter
Nya growth-tester mot GAMMAL `tick.go` (e6b5d0f9):
```
--- FAIL: TestMurtak_PalisadeCityAtCapDoesNotGrow   5000 -> 5021, want it to hold
--- FAIL: TestMurtak_NewCityGrowsWhileNeighbourAtCapHolds  neighbour at its cap 2000 -> 2009
--- FAIL: TestMurtak_CityOverCapLosesNoOne   19000 -> 19035
--- FAIL: TestMurtak_GrowthStopsExactlyAtCap   1999 -> 2008, want exactly 2000
```
(b) är en regressionsvakt och är grön även mot gammal kod (gamla koden växer alltid). (c)/(e) kan inte kompilera/gå mot gammal kod (ingen silo, ingen migration). Efter: alla gröna. Hela sviten `tools/gotest.sh`: 26 paket ok, 0 FAIL; `node --test`: 696 pass, 0 fail.

## Mutation
Murgrinden släppt (`if false && economy.SiloLevel(...) < …`, fil kopierad undan, återställd):
```
--- FAIL: TestMurtak_StoneWallNeedsGranary2  ... = 201, want 422 naming Granary of level 2
--- FAIL: TestMurtak_PalisadeNeedsGranary1   ... = 201, want 422
```

## Simulering (HÅRT TAK, Timothy 2026-10-21: full takt `max(1, round(pop·0,005·variety))` ända till taket, klampad till `cap−pop`; variety 1,0 / 1,1 / 1,3)
Kolonin startar på 100 (`ColonyForecastPopulation`).
- Ny koloni 100 → 2 000: **577 / 533 / 462 ticks**.
- Palisade-stad 2 000 → 5 000: **184 / 168 / 142 ticks**.
- 5 000 → 10 000: 139 / 127 / 107. 10 000 → 20 000: 139 / 127 / 108.
(Det första utkastet med mjuktak `1−pop/cap` gav 1 267 respektive 937 ticks för de två första stegen; borttaget.)

## Öppna frågor
1. ~~Mjuktaket~~ AVGJORT (Timothy 2026-10-21): hårt tak, ingen mjuktaksfaktor. Test: `TestMurtak_FullRateRightUpToTheCap` (1 900 under taket 2 000 växer med full takt, +10).
2. ~~joinStartingPopulation~~ felläsning: huvudstäder grundas via Nomadic Host (`nomadicHostPopulation = 1000`, `api/handlers/nomadic_host.go:38`). `joinStartingPopulation = 5000` i `join.go` är en oanvänd konstant (död kod) — rörd inte.
3. **Livevärlden:** Damon 19 232, Petras 14 535 m.fl. (muren 0–1) hamnar över taket och står still — enligt B0, ingen förlust. Men de behöver Granary 1–3 + murar för att växa igen (och Bronze Wall kräver brons 20).
4. **Silo-rivning/erövring:** rivning re-capar matraderna ner till bas (lagrat över taket klipps nästa mutation med `StorageOverflow`-notis). Erövring: buildings följer staden (ej ändrat); muren kvarstår efter att silon rivits (grinden gäller bara byggtillfället). Enklaste valet, ingen kanon rörd.
5. **Lagertaket är idag en teknisk flat 1 000 000** (`DefaultGoodStorageCap`), så ×2/×5/×10 är praktiskt taget osynligt för spelaren tills lagerpoolsmodellen (`megaron_plan_lagerpooler`) ersätter den. Silons verkliga värde idag = ingen synlig; fråga om det ska ge något annat (t.ex. mindre svinn) — flaggat, ej byggt.
6. `MaxGenesisPopulation = 30000` (silversäkring) lämnad; kommentaren rättad. `sizetier.go` (kartsiluett 101–30 000) lämnad — kartans led är omräknade ur pop, inte tak.
7. Murkostnaderna ändrades (24→40 sten, brons 8→20): Stone Wall dyrare än förr; byggda murar påverkas ej.
8. Granary-demolering: `building_demolish.go` kräver ingen extra grind.
