# Vägar V3 — vagnens terrängregel (2026-10-15)

Plan: `myltavault/megaron_plan_vagar.md` V3. Märkning: **bevisar** vagnregeln; substrat för V2:s vagnfaktor.

> **VÄNTAR PÅ TIMOTHYS OK:** terrängtabellen nedan är *förslagstabellen* i planen §2 ("Bekräfta tabellen med Timothy innan V3"). Den är implementerad som förslag och får inte deployas före hans ja. Alla talen bor på ETT ställe (`chariotTerrainTicks` i `server/internal/province/movement.go`).

**Problem.** Stridsvagnen hade samma fart och terräng som fotfolk (Timothy: "snabba på slätt och väg, långsamma i skog och berg").
**Lösning.** Egen prissättningsklass `war_chariot` (kategori-parameter, inte ny kolumn): `unit.MoveCategory(type)` ger `"war_chariot"` för stridsvagnar, annars `CategoryOf`. Passabilitet som "land" (berg stängda); egen tabell; vägfaktor 0,60.

| terräng | vagn | fotfolk (oförändrat) |
|---|---|---|
| slätt / floddal / delta | 0,67 | 1 |
| olivlund / macchia | 2 | 1,5 |
| kulle / ceder / halvöken | 3 | 2 / 2 / 3 |
| vadställe (ej i planen) | 2,5 (som fotfolk) | 2,5 |

## Val att granska (planen bad mig flagga)
- **Kategori, inte parameter:** minst kod — alla prissättare tar redan en kategori-sträng, så A*, `StepHours`, heuristiken (`minTerrainCost`) och `EdgeCost` bär vagnen utan ny signatur. `units.category` är kvar `land`/`naval` (alla `== "land"`-kontroller orörda); prissättningsklassen räknas ur `units.type` på varje plats.
- Platser som nu använder `unit.MoveCategory`/`MoveCategoryOfRow`/`unitRow.moveCategory()`: `march_start.go` (marsch + expeditionsben + hemvändande), `unit_arrival.go` (FOW-svep, hemfärd, steg, rak-linjefallback), `expedition.go`, `expedition_outcomes.go`, `recall_redirect.go`, `api/handlers/unit.go` (recall/redirect, bud-aim, shore-preview, kartvägar), samt läs-tids-omräknarna `march_encounter`, `march_sighting`, `unit_intercept_scan`, `foreign_units`.
- `province` får inte importera `unit` (G1) → strängen "war_chariot" finns på två ställen; `TestChariotMoveCategoryAgreesWithProvince` låser att de är lika.
- **Vadställe** står inte i planens tabell → fotfolkets 2,5. Flagga om Timothy vill annat.
- Öppningsdispatchen (`opening_dispatch.go`) sa "An army crosses a plains hex in 1 day…" — falskt för vagnar. Nu: "Foot soldiers cross … War chariots cross plains in 0.67 day but need 2 in olive woodland and 3 in hills, cedar forest or desert." (talen ur tabellen). Codex `marching.md` ("Terrain and journey time") har fått vagnsraderna.

## Tester
`internal/province/chariot_test.go`: varje terrängrad (hårdkodad) vagn vs fotfolk; berg/flod/hav stängda; vagn + väg = 0,67×0,60 / 3×0,60, väg aldrig till vadställe; vagnen väljer plan omväg där fotfolk tar lunden. `roads_test.go`-egenskapstesterna (A* = Dijkstra, inga vägar ⇒ identiskt) kör nu även `war_chariot`. `internal/combat/chariot_march_test.go`: riktig `StartMarch` — vagn 3×670 milli/2 ticks, spearman 3×1000/3 ticks; samt att `unit.MoveCategory` ≡ `province.CategoryWarChariot`.

## Mutationsprov (alla RÖDA)
slätt 0,67→0,70 · olivlund 2→1,5 · kulle/ceder/halvöken 3→2,5 · ceder utesluten · heuristikgolv = fotfolkets slätt (A* blir icke-optimal) · vagnens vägfaktor · tabell-bypass · berg öppet för vagn · `march_start` prissätter vagn som land · `MoveCategory` glömmer vagn · `MoveCategory`-strängen driver isär.

## Svit
`tools/gotest.sh`, nyskapad DB (mig 180): 25 paket `ok`, 0 FAIL. `node --test codex.test.mjs` 12/12. `tools/day_guard.py server|codex` PASS.

## Flaggor
1. **Tabellen väntar Timothys OK** (se ovan).
2. `marching_armies` (äldre tabell, `world.go Marches`) prissätter inte om vagnar — orört (gammal modell, bara "land"/"naval"-ikon). Karavan/`transport.CurrentPosition`-läsarna (pre-mig-160-rader) orörda.
3. Läs-tids-omräkningen (F3) gäller även vagnar på marscher utan sparad rutt — samma flagga som V1.
