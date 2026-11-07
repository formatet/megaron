# Proof package — böner steg 5a: fruktsamhet, byggets välsignelse, ersättningsriten

Order: `.agents/order-boner-steg5a-ekonomi-20261107.md` · gren `boner-steg5a` från master `e8f4c367` · migration **251** · ej pushad, ej mergad.
Inventering (steg 0): `inventory.md` i denna mapp. Inget STOPP: bygge kan köas om säkert (revisionen i mig 245), tillväxten har EN SQL-väg.
Grundläggningsriten ingår inte (struken av Timothy 11-07).

## Slice-kontrakt (kort)
- **Problem:** inga böner rör befolkningstillväxt, bygge eller straff.
- **Spelarsanning:** spelaren väljer stad eller rike (ingen enhet), ser priset ×2/×4, och ser verkan: tillväxt/kalvar ×1,5, kvarvarande byggtid ×0,75 (ny färdigtid i byggkön), eller att nästa straff bärs av en bock.
- **Invarianter:** bara egna städer/riket (owner kontrolleras vid läsning); bygge skärs högst en gång; events lagrar utfall; ingen `time.Now()`; G1 intakt (economy läser tabellen själv).
- **Non-scope:** grundläggningsriten, reparation av skepp, riskens storlek för straff.

## Framgångskriterier mot test
| Kriterium | Test |
|---|---|
| Matad stad växer ×1,5 mot kontroll; efter `until_tick` normal | `kharis: TestFertility_FedCityGrowsHalfAgainFasterThenNormalAfterwards` |
| Svältande stad oförändrad (krymper som förut) | `TestFertility_AStarvingCityStillShrinks` |
| Rike omfattar egen stad, aldrig främmande/annan stad | `kharis: TestFertility_RealmCoversOwnCityNotAnotherWanaxsNorAnotherCity` · `economy: ...OutsideTheWindowOrForAnotherCity...` |
| Statusvyn (samma SQL) visar den välsignade tillväxten | `economy: TestFertility_StatusProjectionShowsTheBlessedGrowth` |
| Hjordfödsel inom fönstret ×1,5 (nedåt, aldrig färre), foder oförändrat | `economy: TestFertileBirths_RoundsDownAndNeverFewer` · `TestFertility_HerdCalvesHalfAgainMoreWhileItLasts` · `...RealmCoversEveryCityOfTheOwner` |
| Pågående bygge T kvar -> nu+ceil(0,75·T), `sped_up` i RiteCast, gamla händelsen gör inget | `handlers: TestBuilderBlessing_CutsWhatIsLeftToThreeQuarters` |
| Andra bönen på samma bygge ger ingen ny uppsnabbning | `TestBuilderBlessing_OnlyOncePerBuild` |
| Bygge som startar under välsignelsen skärs från start; efter den normalt; väntande rörs ej förrän start | `TestBuilderBlessing_ABuildBegunDuringItIsCutFromItsStartAndNotAfter` |
| Scope/ägare: stad rör bara staden (även vid nästa sync), rike alla egna, främmande aldrig | `TestBuilderBlessing_ScopesOwnerAndNeverBelowOneDay` |
| SQL-skärningen följer `religion.BuildTimeFactor`, aldrig < 1 dag, aldrig mer arbete | `TestBuilderBlessing_SQLCutMatchesTheFactor` |
| Varvsskepp (forming) skärs en gång | `TestBuilderBlessing_ShipsUnderConstructionAreCutToo` |
| Pris ×2/×4, rad, RiteCast-payload, 4xx för unit/permanent/ingen, 409 samma stad | `TestFertilityCast_*` · `TestBuilderBlessing_CostAndTargets` |
| Bocken: straff med riten aktiv -> ingen förlust, skydd satt, bock förbrukad, event + notis; nästa straff efter skyddet slår normalt | `kharis: TestScapegoat_TheGoatBearsThePunishmentAndTheMercyHolds` |
| Utgången / framtida / främmande bock bär inget | `TestScapegoat_AnExpiredOrForeignOrFutureGoatBearsNothing` |
| Stad-scope -> 400, 409 "a scapegoat already stands for you", ny efter 30 ticks, pris 12 livestock | `handlers: TestScapegoatCast_RealmOnlyFixedPriceOneAtATime` |
| Katalog (targets.kind=world, steg, längder), `city_blessings`, byggkön `blessed` + ny `complete_tick`, wrath-status | `TestWorldBlessingCatalogue_StepsTargetsAndWhatStandsOverTheCity` · `TestBuilderBlessing_TheQueueShowsTheNewFinishAndWhoWasCut` · `kharis: TestScapegoat_WrathStatusNamesTheStandingGoat` |
| economy-strängen = religion-konstanten | `TestFertility_EconomyNamesTheEffectTheCastStores` |
| Keryx | `cmd/keryx: rite_world_test.go` (4) · `TestEveryNotificationKindHasReadableDetail` (DivinePunishmentAverted) |
| Webb | `kult_offer.test.mjs` (2) · `city_production.test.mjs` · `kult_kharis.test.mjs` · `format.test.mjs` |
| Codex | `codex.test.mjs` (notiskind -> `the-scapegoat`, länkar) · `tools/codex_facts.py --check` · `TestCodexReferenceMatchesAuthoritativeCatalogues` |

## Mutantlogg (committat före varje mutation; "dödad" = rött på en `_test.go:NN`-rad; alla återställda med `git checkout`)
| # | Mutant | Dödad av |
|---|---|---|
| M1 | `FertilityGrowthFactor` 1,5 -> 1 | `economy/fertility_test.go:19,50,94,122`; `kharis/fertility_growth_test.go:78,107` |
| M2 | bygget snabbas två gånger (markeringen ignoreras, mig 251) | `rite_building_test.go:128` |
| M3 | hela byggtiden i stället för resten | `rite_building_test.go:93,128`; `rite_world_surface_test.go:118` |
| M4 | bocken förbrukas inte | `scapegoat_test.go:97` |
| M5 | bocken tar bort skyddet | `scapegoat_test.go:91` |
| M6a | owner-filtret borta (bock) | `scapegoat_test.go:142` |
| M6b/d | owner-filtret borta (fruktsamhet) | `economy/fertility_test.go:83,114`; `kharis/fertility_growth_test.go:103` |
| M6c | owner-filtret borta (byggets välsignelse, SQL) | `rite_building_test.go:158,274`; `rite_world_surface_test.go:118` |
| M7 | city-scope läses som alla städer | `rite_building_test.go:207` (överlevde först: casten synkade bara den städer; testet synkar nu även grannstaden) |

## Ytor (ett verb på fyra)
- **temenos:** tre böner via `POST .../rite` (`target_scope` city|realm, bocken realm); `available_prayers[].targets.kind="world"`; `city_blessings[]`; `build_queue[].blessed`; `kharis_wrath.scapegoat_*`; event `DivinePunishmentAverted` + notis; mig 251.
- **keryx:** `rite --scope city|realm` (steg som saknas nekas före offret), `--list`, "ready N game days sooner"-rader, `status` visar blessings och bocken, notisen `DivinePunishmentAverted`.
- **webb:** Kult -> Prayers (bara de steg bönen har, "Knossos only"/"all N of your cities"), stadens produktionsruta, bygg-kön "Builder's Blessing", Kult -> Kharis "Scapegoat", notisen.
- **Codex:** `fruitful-season`, `builders-blessing`, `the-scapegoat` (notisartikel), `rites`, `kharis`, reference.

## Vyer för BILD (Timothy dömer efter merge)
1. Kult -> Prayers -> *The Scapegoat* -> Offer: bara realm-valet med priset ×4.
2. Stadens produktionsruta med en rad om fruktsamhet/Builder's Blessing, och byggkön med "Builder's Blessing" och ny färdigtid.
3. Kult -> Kharis med raden "Scapegoat".
(Inga skärmdumpar tagna.)

## Avvikelser och flaggor
- `sped_up[].kind` är `"build"` för bygg-kön och `"ship"` för varvsjobb (ordern nämnde bara "build"); skepp utan arbetstakt än (inga arbetare) skärs men rapporteras inte (ingen färdigtid att jämföra).
- Skeppsreparation (`repairing`) skärs INTE (ingen konstruktion); ordern sa "varvets skepp".
- Hjordens prognosfunktioner (`NextHerdBirth`, `HerdSurplusRate`) skalas inte med fruktsamhet.
- Bocken lagras som en `prayer_blessings`-rad (scope realm, effect `scapegoat`), förbrukas genom `until_tick = nu`; `prayer_blessing_wards_used` kräver en enhet och används inte.
- `DivinePunishmentAverted` har notisnivå 3 (info); ingen "divine gossip".
- Pre-existerande `gofmt -l`-träffar (`found_metropolis.go`, `unit_march_silver.go`, `world.go`, `cmd_codex.go`, `cmd_ticklog.go`) orörda.

## Svit
`tools/gotest.sh` (hela, färsk DB på migration 251, `-p 1`): alla Go-paket `ok`, inga FAIL; webbtester 935/935 (inkl. `codex.test.mjs`); `codex_facts.py --check`, `day_guard_test.py`, `test_verb_parity.py` gröna. Efter sviten ändrades bara ett test (M7-skärpningen) och README; berörda paket kördes om gröna.
