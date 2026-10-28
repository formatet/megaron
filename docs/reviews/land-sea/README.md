# Välj land eller hav

Spelaren kunde inte välja färdsätt när båda vägarna fanns. Nu jämför Transfer, stående rutter och handelsbrev land/sjö med verklig restid, risk, krav och hinder; Keryx har `--by land|sea`. Valet sparas och omkontrolleras vid avfärd/accept/retur. **Bevisar kedjegrinden:** varor kan skickas på den väg spelaren avser utan att försvinna bakom automatiken.

Bas `0f26d0af`, slutlig produktionskod `673addd6`, verifierad LIVE-process `08d8ffcc` (ren218, bevarade grundantal och10 byte-exakta assets). Order `.agents/order-land-eller-hav-20261025.md`, kanon [[temenos_ekonomi]] §Transportval och [[megaron_transportrisk]]. `by` är valfri metadata; tomt bevarar äldre automatik. Migration218 lägger `route_mode` på standing_orders/transports/trade_routes; handelsbrev bär `trade_offer.by`. Befintliga eventtyper och sparade resor behåller sin tolkning. Inga nya risker, kostnader eller hamnkrav.

| Grind | Bevis |
|---|---|
| Baslinje | `server-baseline-red.log`: uttryckligt land blev sjö med ledigt skepp. CLI-vakten saknade flaggan; komponentens kontrakt gav assertion-rött före implementation. |
| Kod | Hel färsk Go grön (world151.582s),820JS/CLI gröna, `full-go.log`, `vet.log`, fyra tidsgrindar och Forces-namnvakt. Slutlig återställd JS/CLI i `mutations-client-final/`. |
| Mutation | `mutations-client-final/proof.json` och `mutations-server/proof.json`: 15 server- och11klientmutationer i separata arbetskopior, namngivna assertion-röda prov, exakt återställning och färsk grön återkontroll. |
| Native/BILD | `native/proof.json`:24bilder från två rundor i1280/390, egen ny PG/Redis/server, processhash/migration och oförändrade produktionshashar.14 verkliga transporter,4stående rutter och1handelsbrev; sparat val verifierat i `native/persisted.json`. [Bilder](index.html). |
| Migration | `native/migration-roundtrip.json`:218→217→218 på samma stoppade isolerade värld; hash för alla tidigare fält i9tabeller bevarad. Nedgradering tar avsiktligt bort den nya valmetadatan. |
| Drift | `ground-before.json` och `deploy-proof.json`: ny process först, ren218, bevarad värld och grundantal, byte-exakta ändrade statiska filer, anonym ny endpoint401. |

Kärngranskning: province äger färdsättets validering och ruttval; transport äger kategorikontroll och skrivning. Explicit val får inte falla tillbaka. Accept använder avtalets sparade val och initiativtagarens befintliga fraktägare/skepp, även vid köp. Standing-sweep läser samma val ut och hem. Returmetadata kopieras från verkligt transportben; gamla event får ingen ny tolkning. Preview använder samma planering, välsignelse och Clock i en återställd transaktion: inget skepp binds, ingen bön förbrukas. Endast egna skrov läses; destinationens befintliga FOW-gate kvarstår.

Reproduktion: `python3 tools/land_sea_bild_20261028.py OUTPUT`; mutationsverktygen `tools/land_sea_client_mutations_20261028.py CHECKOUT OUTPUT` och `tools/land_sea_server_mutations_20261028.py CHECKOUT OUTPUT` kräver separat checkout; `tools/gotest.sh` skapar tom DB. `python3 tools/land_sea_deploy_verify_20261028.py --before` fångar driftbaslinjen, utan flagga verifierar den den nyss deployade committen.

Täckning: faktisk one-off land/sjö och strikt fel utan byte, bevarat fritt skepp vid landval, både köp/sälj och båda färdsätt genom accept och DeliveryHandler-retur, upptaget skepp vid accept lämnar avtalet pending utan transport, one-off tom sjöretur bevarar valet, standing CRUD och båda sweepbenen, tomt legacyval med/utan skepp, preview-tid/risk/krav och oförbrukad bön. Keryx visar alternativ och skickar by på transfer/route/trade-offer/äldre messenger-bilaga. Webbprov följer by till verkliga POST-anrop, spärrar saknat val och hindrar gamla svar/val från att skriva över nytt destinationsval. Det äldre cedar-provet för Chariot cancel land/spill återkört grönt när province.go berördes.

Avgränsningar: havsalternativet verifierar aktuell fri båt och lastplats; proviantkravet står uttryckligen som kontrollerat vid dispatch, ingen reservation utlovas. Standing native skapar rutten; båda arbetsbenen verifieras i riktiga DB-domänprov. Native skickar brevet men accepterar inte i Firefox; accept och kedjad leverans prövas för köp/sälj i DB-proven. Ingen ny separat hostile-FOW-matris för preview; samma tidigare handlergate granskas och äldre prov ingår i fullsviten. Migrationroundtrip är lokal, ingen nedgradering i drift.

Underkända riggförsök: saknad varuväljning i ruttformuläret, saknad vald CLI-stad och assertion mot föregående preview innan det nya svaret kom. Den sista observationen gav också en riktig rättning: gamla alternativ töms medan ny preview laddas. Bildgranskningen fångade ett alltför smalt ruttnät i handelsbrevet; valytan spänner nu hela formuläret och Firefox kontrollerar bredden. Slutpaketets alla bilder togs om rent efter sista produktionsändringen.

Metodisk lärdom: spara avsikten skilt från enskilda frysta resor, och följ den hela vägen hem. En readonly preview som använder effektägaren måste rullas tillbaka. Kontrollera verklig kolumnbredd, inte bara att en ruta saknar horisontell överströmning.
