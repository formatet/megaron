# Platser vid grundandet — PlacesInNewCity (2026-11-04)

Problem: places in a new city's own catchment either notified one by one hours later (PlaceFirstReached) or, after bea78edf, not at all.
Lösning: `combat.ClaimFoundingPlaces` (server/internal/combat/place_founding.go) claims every unclaimed place whose target hexes
(`placeReachTargets`) lie within `hexgrid.CatchmentRadius` of the new city, in the founding transaction, and writes ONE `PlacesInNewCity`
notification + event (`{settlement_id, settlement_name, places:[{kind, place_id, q, r}]}`). Empty list = no notice.
Anrop: `foundMetropolisFromNomadicHost` (api/handlers/found_metropolis.go, levereras efter commit i Settle) och `foundColony`
(combat/unit_arrival.go, via expeditionOutcomes-bufferten; båda anropen :530 och :804 går genom foundColony).
Senare steg: `recordPlaceEntriesTx` hittar `claimed_by` satt -> ErrNoRows -> ingen PlaceFirstReached (test: TestFoundingPlacesLaterMarchGivesNoFirstReached).
Ytor: keryx (cmd_notifications.go, listar `keryx places name <kind> <id>` per plats), webb (format.js text/ikon/kategori, ws.js refreshTiles), Codex (naming-the-land, founding).

## Tester
- combat/place_founding_test.go: en notis + båda claimade · utanför catchment orört · redan claimad utelämnad, tom lista tyst · senare march ger ingen PlaceFirstReached · foundColony köar en notis.
- api/handlers/found_metropolis_places_test.go: metropol-grundning.
- cmd/keryx/cmd_places_test.go, web/static/js/megaron/ui/placename.test.mjs.

## Mutanter (alla dödade)
- radie +1 -> TestFoundingPlacesOutsideCatchmentUntouched (place_founding_test.go:71)
- släpp `claimed_by IS NULL` -> TestFoundingPlacesAlreadyClaimedOmittedAndEmptyIsSilent (:92)
- en notis per plats -> TestFoundingPlacesOneNoticeBothClaimed (:57), ...OmittedAndEmpty (:99), TestFoundColonyAnnouncesCatchmentPlacesOnce (:151)
- foundColony köar inte outcome -> TestFoundColonyAnnouncesCatchmentPlacesOnce (:154)
- metropolens returvärde tappas -> TestFoundMetropolisAnnouncesCatchmentPlacesOnce (:38)

## Kända avgränsningar
- Vid metropolgrundning kan catchment-hexar vara okända (FOW). Platsens vittneshex (`target.hex`) läggs i `player_scouted_tiles`, som vid ett steg; fler hexar än så läggs inte till. Platsens art visas därmed även om bara en del av den var känd.
- Webben centrerar på staden (settlement_id), inte per plats; texten listar arter, aldrig koordinater.
