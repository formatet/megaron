# Proof package — böner steg 2b: fart, även mitt i en färd

Order: `.agents/order-boner-steg2b-fart-20261106.md` · gren `boner-steg2b` från master `7ebb20fe` · migration **247** · ej pushad, ej mergad.
Inventering (steg 0): `inventory.md` i denna mapp (ingen tabell saknade säker omköning; två ordrepremisser rättade).

## Slice-kontrakt (kort)
- **Problem:** fart var en stadsflagga som bara gällde nästa avgång; ingen färd kunde snabbas upp under väg.
- **Spelarsanning:** spelaren väljer vem (en Runner/karavan/ett skepp, stadens, rikets), ser priset ×1/×2/×4, och kvarvarande färd tar ¾ av tiden direkt; nya avgångar under välsignelsen likaså. Ny ankomst visas på alla ytor.
- **Invarianter:** bara egna färder; position nu identisk före/efter; en färd snabbas högst en gång; events lagrar utfall (`RiteCast.effect.sped_up[]`); ingen `time.Now()`.
- **Non-scope:** soldaters landmarsch, stormbönerna (3), permanent (4), "svårare att fånga".

## Framgångskriterier mot test
| Kriterium | Test |
|---|---|
| Ny ankomst `nu+ceil(0,75·T)`, gångna gränser orörda, position identisk | `movement: TestSpeedUpRemaining_ArrivalPositionAndPastBoundaries` |
| Transport (land/sjö, samma journey-väg): ankomst, position, ETT jobb flyttat på plats, andra körning no-op | `transport: TestSpeedUpTransport_RemainingTimeIdempotentAndInPlace` |
| Runner: ankomst, position, jobb på plats, no-op igen | `messenger: TestSpeedUpMessenger_RemainingTimeIdempotentAndInPlace` |
| Skepp: ankomst, position, nytt UnitArrivalV2, **gamla jobbet gör ingenting**, no-op igen | `combat: TestSpeedUpMarchingUnit_RemainingTimeNewArrivalAndIdempotent` |
| Märkt färd lämnas orörd | `movement …MarkedOrLegacy…` · `combat …MarkedVoyageIsLeftAlone` · `transport …DepartureSpedIsLeftAlone` |
| Budbärare ombord snabbas inte separat | `messenger: TestSpeedUpMessenger_AboardACarrierIsLeftAlone` |
| Scope unit/city/realm väljer rätt; främmande rörs aldrig; ingen stapling mellan böner | `handlers: TestSpeedCast_LandScopesOwnerAndNoStacking` · `TestSpeedCast_SeaShipsOfTheOwnerOnly` |
| `sped_up` i RiteCast | `TestSpeedCast_LandScopesOwnerAndNoStacking` (läser payload) |
| Avgång under välsignelsen ×0,75, andra stad orörd, efter `until_tick` normal, ej dubbelt | `transport: TestBlessDeparture_SpeedPrayerHoldsOnlyWhileItLasts` · `messenger: TestSaveDeparture_SpeedPrayerOverTheCity` |
| Keryx: mål/pris, "arrives N game days sooner" | `cmd/keryx: TestRiteList_SpeedPrayerHoldsForDeparturesToo` · `TestRiteCast_SaysWhichJourneysArriveSooner` |
| Webb: vägens mål, city/realm alltid öppna | `kult_offer.test.mjs` (2 nya) |
| Codex: Swift Feet / Following Seas, Thread/Passage borta | `codex.test.mjs` · `TestCodexReferenceMatchesAuthoritativeCatalogues` · `tools/codex_facts.py --check` |

## Mutantlogg (committat före varje mutation; "dödad" = rött på `_test.go:NN`)
| # | Mutant | Dödad av |
|---|---|---|
| M1a | märket ignoreras (movement) | `speedup_test.go:53` |
| M1b | märket ignoreras (transport) | `transport/speedup_test.go:91` (första körningen överlevde: testet körde andra passet för sent; tidpunkten ändrad) |
| M2 | faktorn på hela färden, inte resten | `movement/speedup_test.go:23`; `rite_speed_test.go:120,175` |
| M3 | gamla UnitArrivalV2 ignoreras ej (RouteKey) | `combat/speedup_test.go:83` (överlevde först: staleness fångades av ArriveTick; gamla jobbet bär nu nya ticken) |
| M4a/b/c | owner-filtret borta (Runner / karavan / skepp) | `rite_speed_test.go:131`, `:160` (främmande karavan tillagd efter överlevnad), `:175` |
| M5 | blessing ignorerar `until_tick` | `transport/speedup_test.go:166` |
| M6 | avgång dubbelsnabbas (märke) | `transport/speedup_test.go:147,180` |
Alla återställda (`git checkout` efter varje körning).

## Avvikelser mot ordern (se inventory.md §3)
1. Likformig skalning ⇒ ny `movement.SpeedUpRemaining` (Costs omskrivs som milli) + `Validate`-undantag för märkta transporter, i stället för "skala kvarvarande StepCosts".
2. Namngivna Runner/karavan lagras som `mover_id` (mig 247), inte `unit_id`.
3. Transport/Runner: ankomstjobbet flyttas på plats; det finns aldrig ett gammalt jobb. Skepp: som recall (nytt jobb, gammalt ignoreras).
4. Ingen kolumn på `messengers`: märket sitter i `land_route`; Runnern snabbas vid avgång efter `SaveDepartureTx`.
5. Namngivet skepp täcker inte sjöfrakt vid avgång (bara stad/rike); namngiven Runner/karavan inte framtida avgångar.
6. Ej snabbade (medvetet): Runner ombord/väntande, skepp på expedition, rutter utan precis klocka.
7. Död kod kvar (nämns, ej borttagen): `departurePending` i `cult_prayers.go` och `EffectNext*Departure`-grenarna (väntande rader hedras ännu).
8. Retirerade tester ändrade: `religion_departure_test.go` lägger `departure_prayers` direkt (`f.pend`) eftersom Thread/Passage ej kan casts; Thread/Passage struken ur recipe-/relations-/wait-reason-tester.

## Ytor (ett verb på fyra)
temenos (`rite` + `sped_up`, katalogens `targets.always`) · keryx (`rite --list`, sooner-rader) · webb (Kult → Prayers) · Codex (`swift-feet`, `following-seas`, rites/storms/temples/kharis).

## Vyer för BILD (efter merge)
Kult → Prayers → Swift Feet / Following Seas → Offer: målväljaren (vägens lista, city/realm öppna). Inga skärmdumpar tagna.

## Svit
Se slutrapporten (hela `tools/gotest.sh`, färsk DB på migration 247, + webbtester).
