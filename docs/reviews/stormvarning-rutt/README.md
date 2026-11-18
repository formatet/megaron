# Stormvarning på rutten (order stormvarning-rutt-20261118)

Problem: skepp sjönk vid ett hamnutlopp utan att förhandsvisningen nämnde en storm spelaren redan sett.
Lösning: sjöresornas `storm_warning` får en rad om kända stormar (synliga nu eller i minnet) på eller intill rutten. Osedda stormar nämns aldrig.

## §0 Inventering (hur rutthexarna fås, utan andra vägsökning)
| Plats | Rutt finns? | Källa |
|---|---|---|
| march-preview `unit_march_preview.go` | ja | `combat.PreviewMarch` -> nytt fält `MarchStarted.Path` (samma `FindPath`-väg som redan sökts) |
| march-kvitto `unit.go` | ja | `combat.StartMarch` -> `res.Path` (samma fält) |
| passage `messenger_passage.go` | ja | `StartMarch` -> `res.Path` |
| trade-preview `trade_preview.go` | ja | `j.Path` (`PlanTradeJourney` kördes redan) |
| varor-kvitto `province.go` | ja | `journey.Path` (planerad i samma request) |
Ingen plats saknade rutt. Enda ändring i combat: `MarchStarted.Path` (read-only, visas aldrig som rutt). Explore: kvittot skickar `nil`-rutt, preview-handlern returnerar före PreviewMarch-resultatet, så bara skrovraden gäller.

## Regel (en ägare)
- `knownStorms` (storms.go): den gemensamma, FOW-filtrerade läsningen, utbruten ur `GET /storms` (som nu anropar den).
- `knownStormsOnRoute` / `stormRouteLine` / `stormForecast` (storm_warning.go). Konstant `stormRouteReach = 1` (rutthex eller granne). Bara rutthexar med `IsWaterBarrier`-terräng räknas. Närmast längs färden nämns, sedan "(+N more)". Skrovraden först, sedan ruttraden, i samma fält.

## Ytor
temenos (alla fem), keryx (skriver redan `⚠ <fält>`, test tillagt), web (march_preview.js fanns; NYTT: send_goods förhandsvisning + kvitto, passage-kvitto i diplomacy.js), Codex (storms.md, transfers.md).

## Tester (alla röda före: symbolerna saknades på master, kompileringsfel; gröna efter)
`api/handlers/storm_route_warning_test.go`: LiveStormNextToRouteIsNamed, RememberedStormOnRouteSaysGameDaysAgo, UnseenStormOnRouteIsNeverNamed (kanongrinden), LandRouteGetsNoLine, SeveralStormsNameFirstAndCountRest, MarchPreviewAndReceipt, ExploreGetsNoRouteLine, PassageReceipt, TradePreviewAndTransferReceipt. keryx: `TestMarchPreviewAndReceiptPrintRouteStormWarning` (+ befintliga transfer/passage). web: send_goods.test.mjs (storm_warning).

## Mutanter (committat före, återställt efter)
- `sees` i knownStorms -> `true ||`: DÖDAD av UnseenStormOnRouteIsNeverNamed + RememberedStormOnRoute.
- vattenfilter -> `true`: dödad av LandRouteGetsNoLine.
- `stormRouteReach` 0: dödad av fem tester (live, several, march, passage, trade).

## Texter som skrivs ut (keryx = `⚠ ` + fältet, verbatim)
- live: `Storm on this route at (2, 1), drifting NE. A storm takes 2 hull.`
- minne: `Storm seen on this route at (12, 0), 3 game days ago. A storm takes 2 hull.`
- flera: `Storm on this route at (2, 1), drifting NE (+1 more). A storm takes 2 hull.`
- med skrovrad: `Hull 2 of 5: any storm ... sink it (...). Unseen storms may lie on the way; repair at a shipyard first. Storm on this route at ...`
Helsvit: Go-paketen alla ok (färsk DB, -p 1); webb 1045/1047 först, de två röda var Codex-buntens artiklar (stale `articles.json`) – åtgärdat med `python3 tools/codex_facts.py`, codex.test.mjs 37/37 grön.
TEXT-dom: i playtest.

## Avgränsningar
- Skrovradens egen mening "Unseen storms may lie on the way" står kvar oförändrad.
- Kurirordrar till skepp i öppet hav (`dispatchMarchCourier`) får fortfarande bara skrovvarning.
- Varning vid dag-0-minne ("0 game days ago") är ej särskilt formulerad.
