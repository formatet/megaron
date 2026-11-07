# Proof package — böner steg 5c: murarna faller (Potnia, offensiv)

Order: `.agents/order-boner-steg5c-murarna-20261107.md` · gren `boner-steg5c` från master `9f0ace59` · **migration 252** (`wall_falls`) · ej pushad, ej mergad.
Inventering (steg 0): `inventory.md`. STOPP-villkoret (en pågående strid läser en cachad murnivå, `battles.wall_level`) lyftes till koordinatorn och **godtogs**: en pågående strid behåller sin ögonblicksbild, bara senare strider ser fallet.

## Slice-kontrakt (kort)
- **Problem:** Potnia har bara defensiva böner; ingen bön når en annan Wanax.
- **Spelarsanning:** en Wanax med Potnias tempel väljer en främmande stad på sin karta vars mur hen minns; tre game days senare förlorar staden en murnivå. Bedjaren får bara veta att Potnia hörde; målägaren får alltid veta att muren föll och, om hen står väl hos gudarna, ett omen i förväg, aldrig av vem.
- **Invarianter:** mur och murbyggnad sänks i samma TX; fallet körs en gång per cast; events lagrar utfall (`RiteCast`, `WallsFell`); ingen `time.Now()`; G1 intakt (handlern i `combat`, notis via `Broadcaster`).
- **Non-scope:** målstege, tvingande belägring, att bedjaren får bekräftelse, att en pågående strid ändras.

## Kriterium -> test
| Kriterium | Test |
|---|---|
| Mur 2 -> 1 efter 3 ticks, inte före (due 103, muren 2 direkt efter casten) | `handlers: TestWallsFallPrayer_AnsweredNowFallsThreeTicksLaterNotBefore` |
| Mur och murbyggnad lika; 1 -> 0 raderar byggnadsraden | `combat: TestWallsFall_OneLevelInBothStores`, `TestWallsFall_ToZeroDeletesTheWallBuildingRow` |
| Mur 0 -> inget (ingen notis) | `TestWallsFall_WallAlreadyZeroDoesNothing` |
| Två böner -> två nivåer | `TestWallsFall_TwoCastsFallOneLevelEach`, `handlers: TestWallsFallPrayer_TwoPrayersFallTwoLevels` |
| Eventet dubbelkört -> en nivå, en notis | `TestWallsFall_TheSameCastRunTwiceIsOneLevel` |
| Mål: egen stad (även i minnet), okänd stad, ihågkommen mur 0, tomt, okänt -> 4xx, offret orört, inget schemalagt | `TestWallsFallPrayer_RefusesOwnUnknownAndUnwalledMemoryWithoutTakingTheGift` |
| Omen: kharis exakt 60 -> en omen-notis till målägaren utan bedjarens id/stad; 59 -> ingen | `TestWallsFallPrayer_OmenOnlyWhenTheOwnerStandsWellAndNeverNamesTheCaster` |
| Fallnotis alltid, utan bedjare (notis- och eventpayload) | `TestWallsFall_OwnerIsToldAlwaysAndNeverByWhom` |
| Bedjarens svar och minne visar inte nya nivån | `TestWallsFallPrayer_TheCasterIsNeverToldTheResultOrTheNewLevel` |
| Strid efter fallet använder nya nivån; pågående strid behåller sin | `TestWallsFall_RunningBattleKeepsItsSnapshotNewBattlesSeeTheFall` |
| Katalog: bara främmande städer med ihågkommen mur >= 1 | `TestWallsFallPrayer_CatalogueOffersOnlyRememberedWalledForeignCities` |
| Keryx: `--list`, `--target`, avvisning, notistexter utan "tick" | `cmd/keryx: TestRiteList_ShowsTheCitiesWhoseWallsPotniaCanBringDown`, `TestRiteCast_WallsFallByCityNameAsGiven`, `TestWallsNotices_AreReadableAndNeverNameTheCaster`, `TestEveryNotificationKindHasReadableDetail` |
| Webb: stadsväljare, post med `target`, inget val skickar inget | `kult_offer.test.mjs` (2 nya), `kult.test.mjs` (1 ny) |
| Codex | `codex.test.mjs`, `TestCodexEntityCoverageFromRuntimeCatalogues`, `TestCodexReferenceMatchesAuthoritativeCatalogues`, `tools/codex_facts.py --check`, `day_guard_test.py`, `test_verb_parity.py` |
| Katalogens bönantal 14 -> 15 | `rite_relation_test.go`, `religion/alpha_test.go` |

## Mutantlogg (committat före varje mutation; dödad = rött på `_test.go:NN`, aldrig `[build failed]`)
| # | Mutant | Dödad av |
|---|---|---|
| M1 | muren sänks direkt vid casten | `walls_fall_prayer_test.go:119`, `:190` |
| M1b | `WallsFallTicks = 0` | `walls_fall_prayer_test.go:123` |
| M2 | tröskeln borta (alltid omen) | `walls_fall_prayer_test.go:239` |
| M3 | bedjarens id i omen-payloaden | `walls_fall_prayer_test.go:251` |
| M4 | murbyggnadens nivå sänks inte | `wall_fall_test.go:61`, `:92`, `:105` |
| M4b | byggnadsraden raderas inte vid 0 | `wall_fall_test.go:70` |
| M5 | idempotensen borta | `wall_fall_test.go:92` |
| M6 | egen stad godtas | `walls_fall_prayer_test.go:213` |
| M7 | ihågkommen mur 0 godtas | `walls_fall_prayer_test.go:213` |
| M8 | minnet uppdateras vid fallet (bedjaren får veta) | `walls_fall_prayer_test.go:164` |
| M9 | pågående strids mur sänks | `wall_fall_test.go:149` |
| M10 | fallnotisen uteblir | `wall_fall_test.go:95`, `:114` |
Två första försök (M2, M5) gav `[build failed]` (oanvänd import/variabel) och räknades inte; mutanterna skrevs om och dödades på testrad. Alla återställda med `git checkout`.

## Ytor (ett verb på fyra)
temenos (`rite` + katalog `targets.kind="city"`, mig 252, `WallsFall`-händelsen registrerad i `cmd/server/main.go`) · keryx (`rite --target <stad>`, `--list`, två notistexter) · webb (Kult -> Prayers stadsväljare, notistexter/kategori) · Codex (`the-walls-fall`, `wrath-over-the-walls`, `rites`, `potnia`, `kharis`-tabellen, genererad referens).

## Avvikelser / flaggor
1. `RiteCast`-eventets id behövs i den schemalagda payloaden, så schemaläggningen sker i cast-TX:n *efter* `AppendTx` (inte i `applyWallsFall`).
2. `wall_falls` (mig 252) är idempotensspärren, bakom `cast_event_id`.
3. Omenet avgörs vid cast och registreras som `omen_sent` i `RiteCast` (utfall lagras).
4. Kant: ett murbygge i kö som slutförs efter ett fall höjer från det nya läget och ger alltså en nivå mindre än vad det kostade. Orört, noterat i vault.
5. Ingen ändring i befintliga tester utom de två katalogtalen (14 -> 15).

## Vyer för BILD (efter merge, Timothy)
- Kult -> Prayers -> The Walls Fall: stadsväljaren med "held by …, wall level N as you last saw it" och texten "you will not be told".
- Dispatch-rutan för `WallsOmen` och `WallsFell` hos målägaren.
Inga skärmdumpar tagna.

## Svit
Helkörning: se slutrapporten (färsk DB, `-p 1`).
