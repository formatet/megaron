# Steg 5d - inventering (inga ändringar)

Ingen STOPP-grund. `province.LoadLiveEyes` är den enda källan; alla anropare går via den (direkt eller via
`handlers.loadLiveEyes`, som bara lägger på tick-ankaret). Ett tidsbegränsat öga kan därför läggas in på ETT ställe.

## 1. Anropare av LoadLiveEyes (direkta: 4 i internal/, ~30 via handlers.loadLiveEyes)
| Anropare | Vad den gör med ögonen |
|---|---|
| `handlers.loadLiveEyes` (world.go:1142) -> `/map`, `/provinces`, `/wanaxes`, `/cities`, `/foreign-units`, `/units`, `/places`, `/storms`, foreign_city, found_metropolis, gift/trade/march-preview, place_story m.fl. | `AnyEyeSees` = vad spelaren ser live; `/map` skriver dessutom minne (world.go:517 `player_scouted_tiles`, :524 och :1108 `RecordSeenSettlements`) vid LÄSNING |
| `combat/march_sighting.go:118` | sveper alla marscher; notis "sighted" TILL den som ser (ej till den sedda). Orakelögat ger alltså bedjaren en sighting-notis om en armé marscherar i området - korrekt, det är live syn |
| `combat/unit_intercept_scan.go:215` | en vaktpost (sentry) hos ägaren måste redan stå inom UnitInterceptRadius; ögat är bara FOW-grind. Orakelögat kan inte skapa en avskärning utan egen vaktpost |
| `transport/intercept.go:251` | samma mönster för karavaner |

## 2. Styrka i live-syn
`/foreign-units` visar type, size, crew, stance i full upplösning för allt `AnyEyeSees` släpper igenom
(foreign_units.go:~80, kanonbeslut 2026-08-03: "Live tier reveals a foreign unit in full"). Orakelögat ger därför
samma styrka som ett eget öga, utan nytt kod. Ingen FLAGGA.

## 3. Minnesskrivare
- Läsning-driven: `/map` (world.go:517/524, 1108). Skriver bara om spelaren läser kartan under fönstret.
- Händelse-driven: `province.SweepLiveRadius(Along)` (eyes.go) tvingar samma skrivning (tiles + `RecordSeenSettlements`) för ett öga som står på en hex.
- Beslut: minnet skrivs (a) vid cast, i cast-TX, med `SweepLiveRadius` för ögonkind `oracle`, och (b) vid cast-tick+1 via ett schemalagt event
  (som WallsFall), så att det syns även om spelaren inte loggar in. `/map` under fönstret skriver som vanligt. Båda vägarna är idempotenta
  (`ON CONFLICT DO NOTHING`, stadsminnet skrivs över med samma sedda tillstånd).

## 4. Kunskapstest
- Stad: `player_seen_settlements` (stadsminnet, namn + q,r; 5c använde det) för främmande städer; egna städer ur `settlements` (owner = du).
- Plats: `place_names` med `name IS NOT NULL`, ankar-hexen (anchor_q, anchor_r) måste vara `knownToPlayer` (live eller minne), som `/places`.
- Hex utan namn: finns inget namn att slå upp -> 404.

## 5. Tidsmodell
`LoadLiveEyes(.., at)` får `at` = `tick.Anchor.MilliAt(now)` (tusendels tick). Öga lagras som `from_milli..until_milli`
(cast-milli .. +1000 = en speldag); aktivt när `from <= at < until`. province har inget tick-beroende; `at` räcker.

## 6. Att passa upp
- `SetSeaHorizons` ger varje öga ett öppet-vatten-horisont (upp till 4 hex). Orakelögat har FAST radie 2: hoppas över där (annars 4 över havet).
- `LiveRadius` ger +2 på berg; orakelögat får ingen terrängmodifierare (gudarna ser).
- Migration: nästa lediga = 253.
