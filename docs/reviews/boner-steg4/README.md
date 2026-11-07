# Proof package — böner steg 4: permanent välsignelse på en namngiven enhet

Order: `.agents/order-boner-steg4-permanent-20261107.md` · gren `boner-steg4` från master `9b203f77` · migration **250** · ej pushad, ej mergad.
Inventering (steg 0): `inventory.md` i denna mapp. Inget STOPP: varje läsare av skrovtaket i en het slinga (strid, storm) har redan skeppets rad i handen.

## Slice-kontrakt (kort)
- **Problem:** en bön varar högst några speldagar; en stor gåva kan inte köpa något som består.
- **Spelarsanning:** vid ×20 av gåvan blir Velchanos arm, Following Seas eller Stout Hulls kvar på en NAMNGIVEN enhet för alltid. Ägaren ser det i sin enhetslista; andra ser bara vad strid redan avslöjar.
- **Invarianter:** bara en egen enhet i `units`, aldrig stad/rike/Runner/karavan; ingen stapling; skeppets tak är skeppets eget; events lagrar utfall; ingen `time.Now()`.
- **Non-scope:** världshandlingar (5), återbetalning vid nedlagd enhet, Swift Feet permanent.

## Framgångskriterier mot test
| Kriterium | Test |
|---|---|
| Pris ×20 dras; vila = som ×1; raden saknar slut; RiteCast bär `target_scope: permanent` | `handlers: TestPermanentCast_CostsTwentyFoldRestsLikeOneUnitAndNeverEnds` |
| Stad/rike kan inte bli permanent; Runner/karavan inte; fel slag, främmande, nedlagd -> 4xx, inget dras | `TestPermanentCast_RefusesWhatIsNotANamedUnitOfTheRightKind` (7 delfall) |
| Styrka: permanent enhet slår ×1,25 vid tick 1010 då en vanlig bön från tick 0 för länge sedan löpt ut; annan sida inget | `combat: TestPermanentStrength_StillBoostedAThousandDaysLater` · `religion: TestPermanentBlessing_HoldsLongAfterAnOrdinaryOneEndedAndCoversOnlyTheNamedUnit` (tick 2 000 000, annan enhet, främmande) |
| Fart: två på varandra följande färder med samma skepp får båda ×0,75 (3 mot 4 för ett vanligt skepp, inte 2); annat skepp inget | `combat: TestPermanentSpeed_EveryVoyageOfTheNamedShipSailsAtThreeQuarters` · `religion: TestPermanentBlessing_SeaSpeedReachesEveryVoyageOfTheNamedShipOnly` |
| Fart: färd under väg snabbas vid cast (106 mot 108) | `handlers: TestPermanentCast_SeaSpeedSpeedsTheVoyageUnderWay` |
| Skrov: efter cast 6/6 (en skadad 3 -> 4), annat skepp orört, ingen ward-rad, payload `hull_bonus/hull_before/hull_after` | `TestPermanentCast_HullGivesAPlankToThisShipOnly` |
| Skrov: storm tar 2 -> 4/6, notisen säger 6 -> 4 av 6; tre stormar sänker som förut | `combat: TestPermanentHull_AStormTakesTwoOfSix` · `TestPermanentHull_StillSinksAtZero` |
| Skrov: reparation återställer till 6 (inte 5), bedömd och prissatt mot 6, båda slutföranden; 6/6 är fullt, 5/6 är en poäng kort | `handlers: TestPermanentHull_RepairRestoresToTheShipsOwnCeiling` (legacy + varvsjobb) · `TestPermanentHull_FullMeansTheShipsOwnCeiling` |
| Varvets färdigbygge påverkar inte andra skepp (hull = CASE bara för `repairing`, bonus 0 -> 5) | `TestShipyardPauseResumePreservesPaidProgressAndRejectsStaleTimer` (oförändrad, grön) |
| Stridstärningar: fullt 6/6 = samma antal som fullt 5/5; 3/6 färre än 3/5; sjunker vid 0 som förut | `combat: TestPermanentHull_BattleDiceScaleWithTheShipsOwnCeiling` · `TestPermanentHull_CargoLossIsTheShareOfTheShipsOwnHull` |
| Stapling: samma permanenta två gånger -> 409 (alla tre); vanlig bön av samma slag på permanent enhet -> 409; stad-bön bredvid går | `TestPermanentCast_NoStackingTwiceOrWithAnOrdinaryPrayer` |
| Katalogen: fjärde steget ×20 bara för strength/hull/sea speed (inte Swift Feet); enhet som redan har det märkt `forever` | `TestBlessingCatalogue_ListsTargetsAndStepCosts` (utökad) · `religion: TestScopeCostFactorsFor_PermanentOnlyWhereItExists` |
| Ägarens vy: `hull_max` 6, `blessings[]`; främmande vy bär inget | `TestPermanentBlessings_TheOwnersUnitViewShowsThem` · `TestPermanentBlessings_ForeignViewCarriesNone` |
| Stormvarning "Hull 2 of 6" | `TestPermanentHull_StormWarningNamesTheShipsCeiling` |
| Keryx: `--scope permanent --target`, `--list` visar steget och "already blessed for ever", enhetsrad "hull 6/6" + välsignelser | `cmd/keryx: TestRiteList_ShowsThePermanentStepAndWhoAlreadyHasIt` · `TestRiteCast_PermanentNamesOneUnit` · `TestFormatSize_ShowsTheHullCeilingAndTheBlessingsForEver` |
| Webb: fjärde radioknappen, x20-gåva, grå när alla redan har det; kraftkortets hull x/6, välsignelser, repair mot eget tak; ShipRepaired 6/6 | `kult_offer.test.mjs` (3 nya) · `force_card.test.mjs` (4 nya, ny fil) · `format.test.mjs` (1 ny) |
| Codex | `codex.test.mjs` · `tools/codex_facts.py --check` · `TestCodexReferenceMatchesAuthoritativeCatalogues` |

Notering: styrka och fart behövde ingen ny läsarkod (sentinelen `until_tick = 2147483647`), så de testerna var gröna direkt; skrov, cast, katalog, repair och vy skrevs röda först (kompilerade inte / gav fel tal) och gjordes gröna.

## Mutantlogg (committat före varje mutation, `git checkout` efter; "dödad" = rött på en `_test.go:NN`-rad)
| # | Mutant | Dödad av |
|---|---|---|
| M1a | reparationens mål/poäng mot `HullMax` i stället för skeppets tak (`unit_repair.go`) | `unit_repair_permanent_test.go:50`, `:90` |
| M1b | varvets färdigbygge sätter `hull = HullMax` (`shipyard_work.go`) | `unit_repair_permanent_test.go:71` |
| M1c | `ShipRepairComplete` sätter `hull = HullMax` (`ship_repair.go`) | `unit_repair_permanent_test.go:71` |
| M2 | stridsskalningen mot `hullMax` i stället för skeppets tak (`battle.go`) | `permanent_blessing_test.go:148` |
| M3 | den permanenta raden löper ut (`PermanentUntil` 1000) | `permanent_test.go:19`, `:42`; `permanent_blessing_test.go:43` |
| M4 | ×20 -> ×1 | `rite_permanent_test.go:46`, `:172`; `religion/permanent_test.go:60` |
| M5a | staplingsspärren för skrov borttagen | `rite_permanent_test.go:123` |
| M5b | staplingsspärren för styrka/fart borttagen | `rite_permanent_test.go:123` |
| M6 | stormnotisens `hull_max` = `HullMax` | `permanent_blessing_test.go:123` |
| M7 | casten ger ingen extra plank nu (`hull = hull`) | `rite_permanent_test.go:184` |
Alla restaurerade (`git status` ren efter varje körning).

## Ytor (ett verb på fyra)
- **temenos:** `POST .../rite` med `target_scope:"permanent"`; katalogens `scope_cost_factors.permanent` + `targets.unit[].forever`; `GET /units` `hull_max` + `blessings[]`; mig 250.
- **keryx:** `rite --scope permanent --target <namn|id>`, `--list`; `army`/`unit`/`hex` visar hull x/6 och välsignelserna; ShipRepaired-raden.
- **webb:** Kult -> Prayers -> Offer: fjärde radioknappen "For ever"; kraftkortets hull och välsignelser.
- **Codex:** `rites`, `the-young-gods-arm`, `stout-hulls`, `following-seas`, `storms`.

## Vyer för BILD (efter merge)
1. Kult -> Prayers -> *Stout Hulls* -> Offer: fyra radioknappar, den fjärde med "gift ×20" och skeppslistan ("blessed for ever" på dem som har den).
2. Forces/hex: ett skepp med "hull 6/6" och raden "Stout Hulls for ever".
(Inga skärmdumpar tagna; BILD efter merge enligt arbetssättet.)

## Avvikelser mot ordern (se `inventory.md` §4)
1. **Sentinel i stället för tak:** `until_tick = 2147483647` + `permanent boolean`; läsarna oförändrade. Skrov har ingen rad (kolumn).
2. **`cargoSizeAfterHullLoss` fick skeppets tak** (`cargoSizeAfterHullLossOf`); annars hade ett 6-skepp som förlorat 5 av 6 mist hela lasten. Står inte i ordern.
3. **Mig 250 lättar CHECK-villkoret** `units_hull_check` (0..5) -> `hull <= 5 + hull_bonus`; saknades i ordern men gör ett 6-skepp lagringsbart.
4. **Vanlig Stout Hulls-sköld på ett skepp med den permanenta plankan tillåts** (ordern kräver 409 bara för styrka/fart).
5. **Permanent spärras inte av en pågående vanlig bön** på samma enhet (ordern säger bara tvärtom); stad/rike-bön bredvid påverkar inte.
6. `stormShipWarning(type, hull, bonus)` fick en tredje parameter; `transport.FreeShip` fick `HullBonus`.
7. `cmd_goods.go:434`/`cmd_storms.go:19` säger fortfarande "of 5" i hjälptext om stormskada (text om basen); orörda.
8. Pre-existerande `gofmt -l`-träffar (`unit_march_silver.go`, `world.go`) orörda.

## Öppna frågor
- Ska en sammanslagning/uppdelning av trupper flytta en permanent välsignelse? Idag följer den enhetens id (nedlagd = borta).
- Webbens målväljare delar EN rullista mellan "One named" och "For ever"; ska "For ever" ha egen lista som döljer redan välsignade i stället för att märka dem?

## Svit
Helkörning `tools/gotest.sh` (GOTEST_PORT=56914, färsk DB på migration 250, commit `8c3ea247` före detta dokument; ingen kodändring efter): alla Go-paket `ok`, inga FAIL; webbtester 928 pass, 0 fail (inkl. `codex.test.mjs`, `forces_guard`, `day_guard`); utgångskod 0. Första helkörningen var grön; ingen rättning efter den.
