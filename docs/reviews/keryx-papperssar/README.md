# Keryx-papperssår + tre bildfixar (order-keryx-papperssar-20261104)

Gren `sonnet/keryx-papperssar` (från master b8a5c8b5). En commit per punkt.

| # | Commit | Test / not |
|---|---|---|
| 1 | 198b5872 | `transport_choice_test.go` — minuter avrundas, ankomst via `arrivalETA` |
| 2 | 1edaf4c5 | `cmd_unit_repair_test.go` — namn/prefix, kort indata ger fel, ingen panik |
| 3 | 446fecce | `temple_staffing_test.go` `TestStaffGodReadbackMatchesGod`; mutant (ta bort `GodID`-matchningen) dödad. Gudens namn i resultatraden och `goodCell` |
| 4 | 63126a98 | `cmd_recruit_total_test.go` (kvitto, march-/actions-hjälp) |
| 5 | f44f9689 | `cmd_gossip_passage_test.go` — `keryx passage --id … --ship <id>` i svaret |
| 6 | 81c9f1be | `cmd_march_preview_test.go` — catchment-prognos i `--preview` (läsning, inget skickas) |
| 7 | 24cef412 | `cmd_runners_test.go` — tomt skeppsnamn → "a ship"; aboard-raden säger "landfall made … runner ashore" resp. "still at sea" (tolkning av "status": landfall passerad eller ej, ur `lands_at`) |
| 8 | 0027d984 | `null ticks_left` = hastighet >= 0 (servern sätter bara `ticks_left` vid negativ takt) -> "not draining". CLI + `map.js` + node-test |
| 9 | 0027d984+1 (`2d09e4ee`) | `temple_staffing_test.go` — läser `build_queue` i province-svaret |
| 10 | b9e8e889 | `client_401_test.go`. Login sparade INTE refresh-token förut (parsad men kastad); nu sparas den (`Config.RefreshToken`, omitempty). 401 -> en refresh -> en omkörning; POLEIA_TOKEN-env hoppar över refresh |
| 11 | ef95309d | `building_purpose_test.go`; Go-texten + `codex/reference.json` |
| 12 | 8db3347a | `small_texts_test.go` (konstanten); ingen DB-test av själva 409:an |
| 13 | 2f8f5ccd | Preview-felet säger "the shortest expedition is 4 game days" (inte "at least": befintligt läckageprov förbjuder det ordet). Rutt-minimum fortsatt dolt |
| 14 | 8b73ace2 | `transfer_storm_warning_test.go` (DB: förhandsvisning + kvitto, hull 5 vs 2), `cmd_storm_warning_test.go` (keryx). Passage-kvittot: bara enhetstest av `stormShipWarning`, ingen DB-test (fixturen tung) |
| 15 | bea78edf | `place_reach_test.go` `TestPlaceReachHomeCatchmentGetsNoNotice`; mutant (`if false &&`) dödad vid `place_reach_test.go:642`. Tolkning: egen catchment = radie `CatchmentRadius` kring egen stad ELLER kring målet för en marscherande colonize-kolonn (staden finns inte ännu när sista hexen skannas). Naming-rätten behålls, bara notisen utebliver. `--intent land`+colonize från skepp täcks inte |
| 16 | e2e89461 | `codex_facts.py` körd; `olive-press.md`, `trade.md`, `trade-offer --help` |
| 17 | 1f722998 | `ai_reports_20261031_test.go` — flodfyllning över sedd passerbar land från egna städer; rad utan väg: "no known land route to it — may be unreachable by land" |
| 18 | b50b168c | "No info"; förklaring i `foreign-cities.md` |
| 19 | f74ca400 | stycket struket; meningen i `market.md` |
| 20 | 4ddbfb4e | `--accent`; <=480px bryts raden över två rader och döljer `.gt-wanax` medan notisen visas (`:has`). **Ej sedd i Firefox — BILD** |

Hoppat: ingen.
