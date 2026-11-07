# Proof package — stormbönerna (3b): stilla stormen och väck stormen

Order: `.agents/order-stormboner-3b-20261107.md` · gren `stormboner` från master `49c1283d` (3a) · migration **249** · ej pushad, ej mergad.
Inventering (steg 0, inget STOPP: krympningen kan skrivas säkert mot väderskanningen): `inventory.md`.

## Slice-kontrakt (kort)
- **Problem:** stormarna levde och dog av sig själva (3a) men bönerna kunde inte påverka dem.
- **Spelarsanning:** en Wanax med Havsdaimonernas tempel kan krympa en namngiven storm på sin stormkarta med en hex (växer tillbaka efter 24 game days, tre böner dödar en 3-hexstorm) eller väcka en vanlig storm på en havshex hen känner. Ingen ser att en storm är väckt.
- **Invarianter:** utfallet slås en gång och lagras; samma `cast_key` två gånger = en effekt; krympning/återväxt/födsel under samma lås som väderskanningen; sen skanning = dagliga; bönen läcker aldrig var stormen är nu.
- **Non-scope:** 4 (permanent på enhet), 5 (världshandlingar), ägarkontroll av stormar.

## Val (ordern bad om besked)
- **Offer:** stilla = fish 10 + oil 6, väck = fish 10 + wine 6, vila 12 game days (dyrare än Following Seas 6+3, billigare än murar). Rikare offer ger ingen större verkan.
- **Hex på webben:** koordinatfält `q,r` i offer-raden (enklaste fungerande), inte kartklick.
- **Mig 249:** `sea_storm_calms`, `sea_storm_wakes`.

## Kriterium -> test
| Kriterium | Test |
|---|---|
| Stilla: 3-hex -> 2 nästa spårtick, sammanhängande, mitten av en rad tas aldrig | `combat: TestCalmStorm_ALineLosesAnEndNeverTheMiddle`, `TestCalmStorm_NextTickHasOneHexLessAndTheHexGrowsBackAfter24` |
| Efter 24 ticks 3 hex igen, inte före, och växer inte vidare | `TestCalmStorm_NextTickHasOneHexLessAndTheHexGrowsBackAfter24` |
| Tre böner inom 24 ticks -> död (`died_tick`, inget mer spår); bön på död storm vägras | `TestCalmStorm_ThreePrayersWithin24TicksKillTheStorm` |
| Bön på 1-hexstorm dödar den | `TestCalmStorm_APrayerOnAOneHexStormKillsIt` |
| Idempotens: samma `cast_key` -> en effekt (stilla, väck) | `TestCalmStorm_TheSameCastKeyTwiceIsOneEffect`, `TestWakeStorm_RefusesLandAndOccupiedWaterAndIsIdempotent` |
| Sen skanning = dagliga med en krympning och en återväxt i intervallet | `TestStormRegrow_LateScanEqualsDailyScansWithAShrinkAndARegrowth` |
| Återväxt landar aldrig på annan storms hex, väntar utan plats | `TestRegrowHex_SkipsOccupiedHexesAndWaitsWhenThereIsNoRoom` |
| Stilla, mål: onamngiven / ej i din karta / död / tom / okänd -> 4xx, offret orört | `handlers: TestStillTheStorm_RefusesWhatIsNotANamedLivingStormOnYourMap` |
| Stilla lyckas, svaret döljer var stormen är, eventet bär `removed_hex` | `TestStillTheStorm_ShrinksANamedStormOnYourMapAndHidesWhereItIsNow` |
| Väck: storm föds på målhex + två grannar, vanlig livstid, inget namn/claim | `combat: TestWakeStorm_IsBornOnTheTargetWithTwoConnectedNeighboursAndNoName`, `handlers: TestWakeTheStorm_BirthsAnOrdinaryStormOnAKnownSeaHex` |
| Väck: okänd hex 404, land 400, upptagen 409, ingen avdragen gåva | `TestWakeTheStorm_RefusesUnknownLandAndCrowdedHexesWithoutTakingTheGift` |
| Den väckta stormen slår väckarens egna skepp | `combat: TestWakeStorm_StrikesTheWakersOwnShipLikeAnyStorm` |
| `/storms` för annan Wanax: inga extra fält | `TestWakeTheStorm_AnotherWanaxSeesAnOrdinaryStorm` |
| Katalog: namngivna stormar + hex-kind | `TestStormPrayers_CatalogueListsNamedStormsAndAsksAHexForTheWake` |
| Keryx: `--list`, `--storm`, `--hex` | `cmd/keryx: TestRiteList_ShowsNamedStormsAndAsksAHexForTheWake`, `TestRiteCast_StormByNameAndHexAsGiven` |
| Webb: väljare, hexfält, post med `target` | `kult_offer.test.mjs` (4 nya), `kult.test.mjs` (1 ny) |
| Codex | `codex.test.mjs`, `TestCodexEntityCoverageFromRuntimeCatalogues`, `TestCodexReferenceMatchesAuthoritativeCatalogues`, `tools/codex_facts.py --check` |

## Mutantlogg (committat före varje mutation; dödad = rött på `_test.go:NN`)
| # | Mutant | Dödad av |
|---|---|---|
| M1 | återväxten ignoreras | `storm_prayers_test.go:114`, `:239` |
| M2 | krympningen kan ta mitten (`removableSlots` godtar alla) | `storm_prayers_test.go:80` |
| M3 | 0 hex sätter inte `died_tick` | `storm_prayers_test.go:148`, `:173` |
| M4 | kunskapstestet borttaget (okänd hex godtas) | `storm_prayers_test.go:187` (handlers) |
| M5 | namnkravet borttaget | `storm_prayers_test.go:129` (handlers) |
Alla återställda med `git checkout` efter varje körning.

## Avvikelser mot ordern (se inventory.md §4)
1. `cast_key` (uuid) i stället för `cast_event_id`: effekten körs före att eventet har id.
2. Väck lagras i `sea_storm_wakes`, inte på `sea_storms`.
3. Krympningen skriver om storms senaste spårrad i stället för att skriva en rad för nästa tick.
4. `target` = namn/id resp. `"q,r"`; katalogen `targets.kind` `storm`|`hex`.
5. Bönerna är "everyday" (har gud): svarar alltid; slumpen gäller bara vilken hex som tas.
6. En ändring i ett befintligt test: `kult.test.mjs` blessing-mocken fick returnera null för de två nya väljarna (annars läste `chosenTarget` mockens `{}` som ett val).

## Ytor (ett verb på fyra)
temenos (`rite` + katalog, mig 249) · keryx (`rite --storm/--hex`, `--list`) · webb (Kult → Prayers) · Codex (`still-the-storm`, `wake-the-storm`, `storms`, `rites`, `sea-daimons`, `kharis` genererad tabell).

## Vyer för BILD (efter merge)
Kult → Prayers → Still the Storm (stormlista) och Wake the Storm (hexfält). Inga skärmdumpar tagna.

## Svit
Helkörning `tools/gotest.sh` (GOTEST_PORT=56913, färsk DB på migration 249, commit före detta bevispaket): alla Go-paket `ok`, webb 920 tester, 920 pass, 0 fail, utgångskod 0. Ingen ändring efter helkörningen utom detta dokument.
