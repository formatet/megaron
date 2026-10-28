# Forces och handelns rörelser

Forces ersätter spelarordet Units; War visar Forces, Recruit och Marches. Land Forces och Ships har tillstånd per rad, People visas bara med Nomadic Host. Bronze Guard och Chariot har samma interna typer och regler som tidigare. Automatiserad frakt samlas i Economy → Routes & cargo; route-bound skepp visar sitt riktiga rutt-ID och ruttnamn i Forces. **Bevisar kedjegrinden** genom begriplig rekrytering och läsbar fysisk logistik.

Baslinje `c1c871a8`, produktionskod `d3653d6a`, verifierad LIVE-process `3966ea69` (ren217, bevarade grundantal,50 byte-exakta assets). Order: `.agents/order-forces-taxonomi-20261025.md`. Spelarord ändras på server/Keryx/webb/Codex; API-fält, artikel-ID och DB behålls. Gamla rekryteringsnamn och units-kommandot är alias. Inga migrationer eller ändrade eventtolkningar.

| Grind | Bevis |
|---|---|
| Kod | Hela färska Go-sviten grön, world154.737s;813JS gröna, vet och fyra day_guard-ytor samt Forces-namnvakt gröna. Loggar här. |
| Egen kärngrånskning/mutation |8 server/Keryx +6 webbmutanter: assertion-rött, exakt byteåterställning, färsk grön återkontroll. `mutations-server/proof.json`, `mutations-web/proof.json` innehåller ersättning och provnamn. |
| Semantik | Ägarprivat route-binding oberoende av skeppstillstånd, avsändarprivat standing_order_id, verklig ruttidentitet i cargo-gruppering, inga fraktrörelser i Marches. |
| Användare/BILD | `native/proof.json`: ny egen process, riktig register/join/founding/standing-order/land- och sjötransfer/marsch/Keryx och Firefox. Två fulla rundor i1280/390,20bilder, oförändrade produktionshashar. [Bilder](index.html). |
| Drift | `ground-before.json` före deploy, `deploy-proof.json` efter ny process: hash, ren217, bevarad värld och grundantal samt byte-exakta ändrade statiska filer. |

Kärndiffen granskades mot ordern: gemensamma displayNames äger namn; route-binding läses med world+owner; /trades lämnar rutt-ID enbart till avsändaren; Economy använder sparat standing_order_id och inget gissat namn; intern unit/movement-wire ändras inte. Namnvakten granskar spelarsträngar, synliga artikelrubriker och HTML, med API-/kommandoundantag.

Reproduktion: `python3 tools/forces_bild_20261027.py OUTPUT`; `python3 tools/forces_server_mutations_20261027.py CHECKOUT OUTPUT`; `python3 tools/forces_web_mutations_20261027.py CHECKOUT OUTPUT`. Mutanter körs i separat checkout. `python3 tools/forces_deploy_verify_20261028.py --before` fångar driftbaslinjen; utan flagga verifierar den deploy.

Fixtur och avgränsning: lokalt explicit ölandskap, extra styrkor, städer och lager. Transporterna beställs genom API men kopplas därefter uttryckligen till standing_order_id som läsmodellfixtur; provet påstår inte att stående-rutt-workern dispatchat dem. Native har verkliga land- och sjölaster och ägarens route-bound skepp. Nya integritetsprovet använder landtransport med avsändare/mottagare; ingen ny separat sjö/tredjepartsmatris. World-filter granskat, inget nytt wrong-world-fixturfall. Rekryteringsalias prövas vid normalisering och kommandoregistrering, inte full HTTP-rekrytering för varje stavning. JSON-grenen är oförändrad; ingen ny separat roundtrip. Alla nya grupp-/ruttkortstillstånd täcks av JS/Keryx-prov.

Underkända tidigare riggförsök: CSS-versaler i textjämförelse, upptagen grundningskoordinat, fixtur som återväckte förbrukad Host, ogiltig Playwright-filterparameter och mobil drawer som täckte navigeringen. Riggen rättades; slutliga två rundor kördes rent efter sista produktionsändringen. Nativebilden avslöjade dessutom gammalt agema-ord i krigsrådet; ändrat till Bronze Guard och fångat med egen mutant.

Metodisk lärdom: skilj beständig ruttbindning från aktuellt fraktben; gruppera med identitet, aldrig ortnamn. Låt riktig native navigering även pröva mobilens drawer-byte.
