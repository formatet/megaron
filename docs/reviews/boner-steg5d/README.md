# Proof package - fågeloraklet (böner steg 5d, Asterion)

Order: `.agents/order-boner-steg5d-fageloraklet-20261107.md` · gren `boner-steg5d` från master `4872d82a` · migration **253** · ej pushad, ej mergad.
Inventering (steg 0, inget STOPP): `inventory.md`.

## Slice-kontrakt (kort)
- **Problem:** det fanns inget sätt att se långt bort utan en egen människa där.
- **Spelarsanning:** med Asterions tempel ser en Wanax en namngiven plats på sin karta (stad hen känner, eller namngiven plats vars ankare hen känner) live, radie 2, en speldag; efteråt är det grått minne; den sedda får ingenting.
- **Invarianter:** ögat är bara bedjarens; fast radie 2 (ingen berg-/havshorisont); exakt en speldag; minnet skrivs även om bedjaren aldrig läser kartan; ingen notis/event hos den sedda; mål utan namn/okänt = 4xx och gåvan orörd.
- **Non-scope:** ingen ny notifikationskind, ingen kartmarkering utöver att området ritas som levande (det följer av levande öga).

## Val
- **Tid:** ögat lagras i tusendels tick (`from_milli`..`until_milli` = cast-milli + 1000), aktivt när `from <= at < until`; `LoadLiveEyes(.., at)` har redan `at`. Province får inget tick-beroende. En cast mitt i en tick ger alltså en hel speldag.
- **Minne:** vid cast (`SweepLiveRadius` i cast-TX:n) och via schemalagt `BirdOracleClose` vid tick+1 (efter ordern: "vid cast och vid tick+1"). `/map` under dagen skriver som vanligt.
- **Styrka:** `/foreign-units` visar redan typ/storlek/hållning i full upplösning för allt levande syn släpper igenom -> ingen FLAGGA, ingen ny kod.
- **Intercept/sighting:** orakelögat når march-sighting (bedjaren får "sighted"-notis, korrekt) och intercept-skanningarna (kräver ändå egen vaktpost inom räckvidd).
- **Alfatal:** silver 10 + wine 8, vila 24, MinKharis 30.

## Kriterium -> test
| Kriterium | Test |
|---|---|
| Under cast-tick syns armé på 2 hex, inte på 3; vid tick+1 syns den inte | `TestBirdOracle_CastPaysAndTheEyeLastsExactlyOneGameDay`, `TestBirdOracle_ShowsAForeignArmyWithItsStrengthLikeAnyLiveEye` |
| Offer betalas (silver 10, wine 8), event bär `place_kind/place_name/q/r/radius/until_tick` | samma |
| Minne: hexar grå, stadsminne uppdaterat (mur 1 -> 3) | `TestBirdOracle_TheCastWritesGreyMemoryAndRefreshesTheCityMemory` |
| Minne även utan inloggning: dagen efter, idempotent | `TestBirdOracle_TheNextGameDayWritesMemoryAgainEvenIfTheCasterNeverLogsIn` |
| Mål: okänd stad / namnlös hex / okänd plats / tomt / okänt -> 4xx, gåvan orörd; känd plats OK | `TestBirdOracle_RefusesWhatIsNotANamedPlaceOfYourMapWithoutTakingTheGift` |
| Egen stad är en känd plats | `TestBirdOracle_YourOwnCityIsAKnownPlaceToo` |
| Den sedda får inget (notiser och events räknade) | `TestBirdOracle_TheWatchedHearNothing` |
| Ögat är bedjarens | `TestBirdOracle_TheEyeIsTheCastersAlone` |
| Katalog: städer + namngivna platser, okända dolda | `TestBirdOracle_CatalogueListsKnownCitiesAndNamedPlaces` |
| Keryx `--list`, `--target` (stad-id, platsnamn, delträff) | `cmd/keryx: TestRiteList_ShowsTheCitiesAndNamedPlacesTheBirdsCanWatch`, `TestRiteCast_BirdOracleByCityIdOrPlaceName` |
| Webb: väljare, post med `target`, inget skickas utan val | `kult_offer.test.mjs` (2 nya), `kult.test.mjs` (1 ny) |
| Codex + katalogtal | `codex.test.mjs`, `TestCodexEntityCoverageFromRuntimeCatalogues`, `TestCodexReferenceMatchesAuthoritativeCatalogues`, `tools/codex_facts.py --check`, `alpha_test.go` (16), `rite_relation_test.go` (16) |

## Mutantlogg (committat före varje mutation; dödad = rött på `_test.go:NN`)
| # | Mutant | Dödad av |
|---|---|---|
| M1 | ögat varar 2 ticks (`BirdOracleTicks = 2`) | `bird_oracle_prayer_test.go:98`, `:214` |
| M2 | radie 3 | `:103`, `:193` |
| M3 | okänd plats godtas (kunskapstestet borttaget; första försöket `[build failed]` räknades inte) | `:253` |
| M4 | minnet skrivs inte vid cast | `:189` |
| M5 | owner-filtret borta (alla ser ögat) | `:172` |
Alla återställda med `git checkout` efter varje körning.

## Avvikelser mot ordern
1. Inget `cast_event_id` i `oracle_sights` (ingen läsare behöver det; castens event bär utfallet).
2. Ögats tid i milli, inte tick: se Val.

## Ytor (ett verb på fyra)
temenos (`rite` + katalog `targets.kind="place"`, mig 253, `BirdOracleClose` registrerad i `cmd/server/main.go`) · keryx (`rite --target`, `--list`; `map`/`hex` visar det sedda via vanliga live-ögon) · webb (Kult -> Prayers platsväljare) · Codex (`the-bird-oracle`, `rites`, `asterion`, `sight`, `kharis` genererad tabell).

## Vyer för BILD (efter merge)
Kult -> Prayers -> The Bird Oracle (väljare med Cities/Named places) och kartan under dagen efter en cast: området runt platsen ritas i färg (levande) med främmande enheter blinkande, sedan grått. Inga skärmdumpar tagna. Ingen särskild orakelmarkering ritad (området syns som levande syn).

## Hela sviten
`GOTEST_PORT=55501 tools/gotest.sh` (färsk DB, mig 253): alla Go-paket `ok`, inga `FAIL`/`_test.go:`-rader; webbtesterna gröna efter att en dagvaktsträff i `the-bird-oracle.md` ("that day") skrevs om och Codex-testerna kördes om. `codex_facts.py --check`, `day_guard_test.py`, `test_verb_parity.py` gröna.
