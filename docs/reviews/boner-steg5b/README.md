# Proof package — böner steg 5b: tjurens horn och tråden (Asterion)

Order: `.agents/order-boner-steg5b-horn-trad-20261107.md` · gren `boner-steg5b` från master `6d8bd0bf` · **ingen migration** (251 oförändrad) · ej pushad, ej mergad.
Inventering (steg 0): `inventory.md` i denna mapp. Inget STOPP: första rundan avgörs deterministiskt ur lagrade `battle_rounds`.

## Slice-kontrakt (kort)
- **Problem:** Asterion har ingen offensiv eller defensiv bön; karavaner fångas på två hex, trupper slår likadant i hela striden.
- **Spelarsanning:** spelaren väljer enhet/stad/rike (ladan ×1/×2/×4), ser tjurens horn på sina trupper och tråden på sina karavaner, och verkan kommer när den behövs (stridens första runda, nästa vaktskanning).
- **Invarianter:** bara egna enheter/karavaner (ägare vid cast OCH läsning); skepp, budbärare och sjöfrakt omfattas aldrig; tråden läses vid varje scanning, inte bakad vid avfärd; events lagrar utfall; ingen `time.Now()`; G1 intakt.
- **Non-scope:** permanent steg, nya notiskinds, den utgångna `minoan_asterion_thread` och dess `j.Thread`-flagga (orörda).

## Framgångskriterier mot test
| Kriterium | Test |
|---|---|
| Horn: ×1,5 i första rundan, ×1 i andra och i nästa tick; försvar som anfall | `combat: TestHorns_FirstRoundOnly` |
| Horn med Velchanos: 1,25 x 1,5 i runda 1, bara 1,25 efter | `TestHorns_StackWithVelchanosStrength` |
| Skepp får inget | `TestHorns_NotForShips` |
| Enhet som kommer in efter första rundan får inget (första tick läses ur lagrade rader, omspelning är fortfarande första) | `TestHorns_FirstBattleTickIsReadFromTheStoredRounds` |
| Utgången / ännu ej bedd / främmande / annan stad / enhet / stad / rike | `TestHorns_ScopesOwnerAndExpiry` (7 delfall) |
| Utfallet i rundans resultat (`blessed_horns`) | `TestHorns_TheRoundResultSaysSo` |
| Tråd: vakt på 2 hex tar en vanlig karavan men inte en välsignad (enhet/stad/rike); vakt på 1 hex tar båda | `transport: TestThread_SentryTwoHexesOffTakesAnOrdinaryCaravan` · `...CannotTakeABlessedCaravan` (3) · `TestThread_SentryOneHexOffTakesTheBlessedToo` |
| Karavan på väg skyddas från nästa scanning; efter `until_tick` radie 2 igen; ej påbörjad bön skyddar inte | `TestThread_ReadAtEveryScanNotBakedAtDeparture` · `TestThread_NotYetBegunProtectsNothing` |
| Annan ägare / annan karavan / annan stad täcks ej | `TestThread_OnlyTheOwnersCaravans` · `...ANamedCaravanCoversOnlyThatCaravan` · `...ACityThreadCoversOnlyCaravansFromThatCity` |
| Sjöfrakt omfattas aldrig | `TestThread_SeaFreightIsNeverCovered` |
| Cast: pris ×1/×2/×4, rad (mover_id/mover_kind), RiteCast-payload; 409 samma mål; 4xx fel mål; inget permanent steg | `handlers: TestHornsCast_*` (3) · `TestThreadCast_*` (3) |
| Budbärare, sjöfrakt, främmande/obefintlig karavan -> 404; saknat mål 400 | `TestThreadCast_OnlyLandCaravansOfTheCasterAreNamable` |
| Katalog: mål (karavaner resp. trupper), antal, steg utan permanent | `TestAsterionCatalogue_ListsCaravansAndTroopsAndNoFourthStep` |
| Ägarens vy: enhetens `blessings[]` ("the bull's horns, N game days"), `/trades` `blessings[]` ("the Thread, N game days", bara egna) | `TestHornsView_TheOwnersTroopsShowTheHornsWhileTheyLast` · `TestMapTrades_TheOwnerSeesTheThreadOnTheirCaravanOnly` |
| Keryx: `--list` (karavaner, "land caravan", aldrig Runner), cast med namn, cargo-raden | `cmd/keryx: rite_asterion_test.go` (3) |
| Webb: målväljare, Thread-ordval, ingen fjärde radio, cargo-raden | `kult_offer.test.mjs` (2 nya) · `economy_rows.test.mjs` (1) |
| Codex: två artiklar + rites/asterion/transfers, facts + reference | `codex.test.mjs` · `tools/codex_facts.py --check` · `TestCodexReferenceMatchesAuthoritativeCatalogues` |
| Katalogräkning 12 -> 14 | `rite_relation_test.go`, `religion/alpha_test.go` |

## Mutantlogg (committat före varje mutation; "dödad" = rött på en `_test.go:NN`-rad; alla återställda med `git checkout`)
| # | Mutant | Dödad av |
|---|---|---|
| M1 | hornen i alla rundor (`hornsNow = blessedHorns`) | `horns_test.go:86`, `:149`, `:206` (första försöket gav `[build failed]` på oanvänd variabel och räknades inte) |
| M1c | hornen i runda 1 av VARJE tick | `horns_test.go:86` (nästa tick) |
| M2 | `HornsDiceFactor` 1,5 -> 1,0 (första sed-försöket matchade inte, upptäckt via tom diff; körd om) | `horns_test.go:79`, `:82`, `:130`, `:146` |
| M3 | trådens radie 1 -> 2 | `thread_test.go:101` (3), `:122` |
| M4a | tråden läses inte vid scanning (som bakad vid avfärd: bara gamla `j.Thread`) | `thread_test.go:101` (3), `:122` |
| M4b | `ThreadCovered` ignorerar from/until | `thread_test.go:128`, `:136` |
| M5a | owner-filtret borta (trådläsning) | `thread_test.go:144` |
| M5b | owner-filtret borta (`blessingCovers`, hornens läsning) | `horns_test.go:82`, `:130`, `:206` |
| M5c | owner-filtret borta (trådcast) | `rite_asterion_test.go:204`, `:209` |
| M6a | skepp får hornen | `horns_test.go:196` |
| M6b | sjöfrakt kan väljas för tråden (cast) | `rite_asterion_test.go:204`, `:209` |
| M6c | `ThreadCovered` täcker även sjöfrakt | överlevde först (inget test); `TestThread_SeaFreightIsNeverCovered` tillagt, nu `thread_test.go:176` |

## Ytor (ett verb på fyra)
- **temenos:** `POST .../rite` tar `target_scope` unit|city|realm + `target` (enhets-id resp. karavan-id); `available_prayers[].targets` (trupper resp. egna landkaravaner) + `scope_cost_factors` utan permanent; `GET /units` `blessings[]`; `GET /trades` `blessings[]` på egna karavaner; battle-round-payloaden `blessed_horns` (additivt, omitempty).
- **keryx:** `rite --scope … --target …`, `--list` (trådens stad/rike-rad säger "land caravan"), `army/unit` visar hornen, `cargo` visar tråden.
- **webb:** Kult -> Prayers (befintlig målväljare), kraftkortet visar hornen, Economy -> Transfer "Your cargo in transit" visar tråden.
- **Codex:** `the-bulls-horns`, `the-thread`, `rites`, `asterion`, `transfers` (spärrradien), `kharis`-tabellen och `reference.json` regenererade.

## Vyer för BILD (Timothy dömer efter merge)
1. Kult -> Prayers -> *The Thread* -> Offer: målväljaren med karavanlistan ("Caravan to X (arrives in N game days)") och raden "every land caravan of <stad>".
2. Kult -> Prayers -> *The Bull's Horns* -> Offer: målväljaren med truppslistan.
3. Kraftkortet för en trupp under hornen ("the bull's horns, 12 game days") och cargo-listan med "the Thread, 12 game days".
(Inga skärmdumpar tagna.)

## Avvikelser och flaggor
- Ordern sa att 2a gav styrkan ett fält i battle-round-payloaden. Det gjorde den inte; `blessed_horns` är nytt (additivt, `omitempty`, ingen ändrad tolkning av gamla rundor).
- Hornens "första runda" = runda 1 av stridens första battle-tick (rundor 1–3 körs per tick); ordern skrev `tick_index 0, round_index 0` men rundorna är 1-indexerade och `tick_index` är absolut tick.
- `rollSide` fick inget nytt argument: `hornsNow` sätts på deltagaren per runda i `resolveTick`.
- Mellanavrundning: Velchanos ×1,25 avrundas först, sedan hornen ×1,5 (testet följer samma ordning).
- Stad/rike-trådens `Always`: ingen karavan behövs för att be (som Swift Feet).
- Felmeddelanden fick omskrivas för `tick/day_guard` ("first tick", "world tick" i spelartext).
- Pre-existerande `gofmt -l`-träffar (`found_metropolis.go`, `unit_march_silver.go`, `world.go`, `cmd_codex.go`, `cmd_ticklog.go`) orörda; mina rader i `world.go` följer filens stil.

## Svit
`tools/gotest.sh` (hela, färsk DB på migration 251, `-p 1`): 967 Go-paket `ok`, inga FAIL i Go-delen; webbtester 938/938 efter rättning. Första helkörningen föll på `tick/day_guard` (orden "tick" i två felmeddelanden) plus två kataloglängder (12 -> 14: `rite_relation_test.go`, `religion/alpha_test.go`); rättade, helsviten kördes om. Den andra helkörningen hade ett enda rött: webbens `day_guard` på frasen "first battle day" i `the-bulls-horns.md`; texten omskriven, `codex_facts.py`, `codex.test.mjs`, `day_guard_test.py`, `test_verb_parity.py` och hela webbsviten (938/938) kördes om gröna. Go-koden är oförändrad mellan den körningen och sluthuvudet.
