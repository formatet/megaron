# B1 — muren som befolkningstak (gren claude/murtak)

## Kontrakt (B0, Timothy 2026-10-21)
| Mur | Tak | Kräver Granary | Kostnad |
|---|---|---|---|
| ingen | 2 000 | — | — |
| Palisade (1) | 5 000 | 1 | timber 12, stone 4 |
| Stone Wall (2) | 10 000 | 2 | timber 20, stone 40 |
| Bronze Wall (3) | 20 000 | 3 | stone 80, bronze 20 |

Granary = `BuildingSilo "silo"`, nivå 1–3, ×2/×5/×10 på lagertaket för grain/fish/oil/wine (ej livestock).
Över taket: ingen tillväxt, ingen förlust. Ytan visar bara taket.

**Valda Granary-kostnader:** nivå 1 timber 12 + stone 8 (8 dagar); nivå 2 timber 24 + stone 16 (16 d); nivå 3 timber 48 + stone 32 + bronze 8 (24 d). Standardtrappan ×1/×2/×4, brons bara på nivå 3 (`levelBronzeFrom`) så att Stone Wall nås före bronsåldern.

## Ändrade filer
- `server/internal/economy/population_cap.go` (ny) — ENDA ägaren: `PopulationCap(wall)`, `PopulationCaps()`.
- `server/internal/kharis/tick.go` — growth-SQL: `30000` borta; tak per stad ur `$9` (= `PopulationCaps()`), mjuktak `max(0,1−pop/cap)`, tillväxt klampad till `cap−pop`, `pop ≥ cap` → håller; ingen yttre `LEAST(…,30000)` som kunde skära ner.
- `server/internal/province/{model,building}.go` — `BuildingSilo`, katalograd, `LevelledBuildings`, murkostnader, `SiloFoodStorageMult`, `SiloLevelForWall`, `WallNeedsSiloRefusal`.
- `server/internal/province/roads.go` — vägpriset läste `WallLevelSpecs[2].stone` (24); Stone Wall blev 40 så basen frystes till 24 (annars omprisades alla vägar och `roads.md` röda).
- `server/db/migrations/197_granary_storage.{up,down}.sql` — `silo_food_mult`, `silo_stored_good`, `settlement_silo_level`, BEFORE INSERT-trigger på `settlement_goods` (ny rad får rätt tak) + AFTER-trigger på `buildings` (bygge/nivåhöjning/rivning re-capar befintliga rader). `building_type` är fri TEXT, ingen enum/check att ändra. (Nummer 197 per koordinatorns besked; 196 = A1.)
- `server/api/handlers/province.go` — murgrind i `Build` (422 "Stone Wall needs a Granary of level 2"), `population_cap` + `silo_level` i province-svaret, `wall_ladder` i `GET /buildings`, tillväxtläget `capped` (`economy.GrowthStateAtWall`). `settlement.go`: `population_cap`. `web.go`: namn "Granary".
- `server/internal/economy/{food_basket,recompute}.go` — `SiloLevel`, `GrowthStateAtWall`/`GrowthCapped`.
- keryx: `cmd_status.go` (`wallsHoldLine`, growth "capped"), `cmd_build.go` (`--list` visar murtrappan med Granary-krav och tak).
- Webb: `ui/drawers/city.js` (`wallOptionHTML`: grå mur med orsak, "Granary" i etiketter), `city_production.js` ("Walls hold N", growth "capped"), `verb_telemetry.js`.
- Codex: `buildings.md` (Granary, murtaket), `growth.md`, `roads.md` (24-stenmeningen). Vault: `megaron_verblista.md` (build-raden).
- Testfixturer: `newGrowthFixture` har nu `wall_level=3` (growth-MEKANIKTESTER vid 5 000+ folk skulle annars frysas); `eaten_variety_test` likaså; `restfolk_soak` tröskel 1100→1042 (se simulering); `ai_round4_test` byter "wall"→"barracks" som "annan giltig stadsbyggnad".

## Tester (nya)
`server/internal/kharis/wall_cap_test.go`: `TestMurtak_PalisadeCityAtCapDoesNotGrow` (a), `TestMurtak_SameCityGrowsAfterStoneWall` (b), `TestMurtak_NewCityGrowsWhileNeighbourAtCapHolds` (d), `TestMurtak_CityOverCapLosesNoOne` (f), `TestMurtak_GrowthStopsExactlyAtCap`.
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

## Simulering (SQL:ens formel, `max(1, round(pop·0,005·variety·(1−pop/cap)))`, klampad till cap, variety 1,0 / 1,1 / 1,3)
Kolonin startar på 100 (`ColonyForecastPopulation`, `foundColony`: pop = enhetens storlek).
- Ny koloni 100 → 2 000 (tak 2 000): **1 267 / 1 125 / 1 006 ticks**; till 1 900: 1 167 / 1 025 / 906; till 1 000: 584 / 513 / 454.
- Palisade-stad 2 000 → 5 000 (tak 5 000): **937 / 862 / 759 ticks**; till 4 500: 527 / 468 / 402.
- Jämförelse gamla taket 30 000: 100→2 000 tog 590 / 545 / 472, 2 000→5 000 tog 207 / 188 / 159. Det låga taket gör mjuktaket (1−pop/cap) bitande: tillväxten **halveras redan vid 1 000** i en stad utan mur. Dessutom 5 000→10 000: 995, 10 000→20 000: 1 133 ticks.

## Öppna frågor
1. **Mjuktaket är brant.** Planen säger `1−pop/cap`; med cap 2 000 tar en ny koloni 1 267 ticks till taket (dubbelt mot förr). Alternativ om det är för segt: full takt tills t.ex. 80 % av taket, eller `1−(pop/cap)²`. Kräver Timothys avgörande (alfa: hellre generöst).
2. **`joinStartingPopulation = 5000` (join.go) ligger över taket 2 000 utan mur.** En vanlig join-huvudstad står alltså still på 5 000 tills Granary 2 + Stone Wall (cap 10 000) — Palisade (cap 5 000) ger ingen tillväxt. Nomadic Host grundar med bärda civila. Ej ändrat; beslut behövs (sänk start, eller ge startmur?).
3. **Livevärlden:** Damon 19 232, Petras 14 535 m.fl. (muren 0–1) hamnar över taket och står still — enligt B0, ingen förlust. Men de behöver Granary 1–3 + murar för att växa igen (och Bronze Wall kräver brons 20).
4. **Silo-rivning/erövring:** rivning re-capar matraderna ner till bas (lagrat över taket klipps nästa mutation med `StorageOverflow`-notis). Erövring: buildings följer staden (ej ändrat); muren kvarstår efter att silon rivits (grinden gäller bara byggtillfället). Enklaste valet, ingen kanon rörd.
5. **Lagertaket är idag en teknisk flat 1 000 000** (`DefaultGoodStorageCap`), så ×2/×5/×10 är praktiskt taget osynligt för spelaren tills lagerpoolsmodellen (`megaron_plan_lagerpooler`) ersätter den. Silons verkliga värde idag = ingen synlig; fråga om det ska ge något annat (t.ex. mindre svinn) — flaggat, ej byggt.
6. `MaxGenesisPopulation = 30000` (silversäkring) lämnad; kommentaren rättad. `sizetier.go` (kartsiluett 101–30 000) lämnad — kartans led är omräknade ur pop, inte tak.
7. Murkostnaderna ändrades (24→40 sten, brons 8→20): Stone Wall dyrare än förr; byggda murar påverkas ej.
8. Granary-demolering: `building_demolish.go` kräver ingen extra grind.
