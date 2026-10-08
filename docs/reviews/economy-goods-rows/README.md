# N — Economy varurader (BILD)

Kontrakt före kod: bas master ba9e6faf. En smal klient-slice; Goods/Transfer/Automation/Wants och Crewed-by behålls. Transfer är redan vara-select + antal och dess enkla good_key/quantity-POST behålls. Automations två CSV-fält ersätts av tilläggbara/borttagbara vara-select + antal-rader. Utgående threshold är önskat lager hos målet; retur floor är det lager som lämnas kvar hos målet. Noll, decimaler, flera varor och tom retur bevaras. Request-fieldnamn, ordning och crewing-id ska vara exakt samma som dagens serializer för motsvarande indata. Ingen ny transportsyntax eller server-/API-/CLI-regel.

Varukatalog: /api/v1/goods är trade-offertens katalog och utesluter silver/parked varor; den får INTE återanvändas för intern automation. Egna provinsers befintliga /goods innehåller hela inventory-schemat inklusive nollrader, silver och parked varor; använd detta utan stock-filter (cult är ej fraktbar). Ingen ny serverkod.

Bevisplan: actual createStandingOrder-handler som fångar HTTP-body, baslinje före produktionskod, fysisk mutation röd→restored grön; riktig fresh PG16/Redis-rigg med register/join/founding/kolonisering genom vanliga API, POST-body old/new för samma två egna städer, Transfer fortsatt korrekt. Desktop/mobil 1:1 BILD, inga SQL-skrivningar eller State-knowledge-fixturer. Full fresh Go/vet och JS-svit. Codex ska beskriva de nya kontrollerna; CLI och server oförändrade eftersom endast inmatningsytan byts.

Resultat tillkommer efter körning. Ingen merge/push/deploy; hash till Claude och stopp.
