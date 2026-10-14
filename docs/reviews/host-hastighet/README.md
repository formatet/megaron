# Host-hastighet

## SAMMANFATTNING
Hostens gamla halva spjutfart fördröjer utforskningen. Timothy har beslutat 0,9 × spjutmannens hastighet; nya marscher använder exakt tidsfaktor 1/0,9. Bas cff64ade. Claude integrerar; HASH + STOPP.

## SLICE-KONTRAKT
| Fält | Kontrakt |
|---|---|
| Problem | Hosten går för långsamt för alfatestets utforskning. |
| Spelarsanning | Nya hostmarscher går med 90 % av spjutmannens hastighet; förhandsvisning och mottagen ETA överensstämmer. |
| Invariant | Lagrade marschers avgång, ankomst och väg ändras inte. Övriga enheter och avrundningsregeln förblir oförändrade. |
| Scope | unit-faktor, berörda tester/kommentarer, Codex horde och kontroll av CLI/webb som använder serverpreview. |
| Non-scope | Inga nya features, migrationer, andra rörelsetal eller grace/startlagerändringar. |
| Acceptans | Exakt faktor; slätt/skog/kulle/öken med vanlig avrundning; preview=dispatch; gamla marscher bevarade; fysisk 2.0-mutation röd→grön och färsk full Go/vet/JS. |
| Stopvillkor | Kanonfrågor parkeras i BESLUT. Handover innan integration. |
| Bevisplan | Ren baslinje, verklig HTTP/PG timing, naturlig register/join/preview/march i CLI och Firefox, DB-bevarande, mutation. |

Arbetet **bevisar** kedjegrindens geografi-ingång. Spelaren möter ändringen när Hostens marschmeny visar ankomstdagen.

## BASLINJE
`baseline.log`: oförändrad cff64ade, ny tom PG på migration179, unit/combat/handlers/keryx berörda tester gröna. Före: faktor2,0 och Codex "half an army's pace". Efter: exakt `1 / hostSpeedOfSpearman`, konstant0,9 och Codex90%. Kommentarerna använder tickkostnad; helperns gamla namn behålls för en liten slice.

## EXPERIMENT
| Ändring | Resultat | Beslut |
|---|---|---|
| Tidsfaktor 1/0,9 | Literal kontrakt och verkliga HTTP/PG-preview=dispatch=stored gröna | Behåll |
| Faktorn fysisk tillbaka till2,0 | `TestHostSpeed_HTTPPreviewDispatchAndStoredMarch` namngiven röd; exakta originalbytes tillbaka och separat tom PG grön | Mutation bevisad |
| Full Go efter ändringen | Äldre RÖ1-test väntade8 ticks för4slätthex; enda felet | Uppdatera den ersatta hostförväntningen till4; ny full färsk körning |
| Testfixturens skog först som `forest` | Detta är ingen aktuell terrängnyckel och fick defaultpris1 | Använd riktig `forest_cedar` samt `forest_olive_grove`; inga produktionsändringar |

Literal provtabell: distanser1/2/5/9 hex; host slätt1/2/6/10, cederskog och kulle2/4/11/20, olivlund2/3/8/15, halvöken3/7/17/30 ticks. Spjutets motsvarande tabell bevarad. Regeln är `max(1, round(total vägkostnad × faktor))`, inte ceil eller avrundning per hex.

## GRINDAR
| Grind | Bevis |
|---|---|
| Kod | `full-go.log`: färsk tom PG179, alla paket. `vet.log`: ren. `js.log`:623/623, ingen skip. `affected.log`, `ro1-green.log`. |
| Funktion/mutation | `factor-2-red.log` → exakta återställda bytes → `factor-restored-green.log`. HTTP-testets fem terränger jämför också lagrade ankomstticks. |
| Semantik/fyra ytor | Temenos äger faktorn; keryx och webb använder samma GET `/march-preview`. Codex `horde` säger90%. Ingen klientformel införs. |
| Användare | Två separata naturliga register→join→preview(API/CLI/Firefox1280+390)→CLI-march→PG/ListUnits. `native/verification.json` health bfb32fb1/179, `native-repeat/verification.json` health45801cef/179; ren produktionsdiff. Inga SQL-fixturer/spelskrivningar, payload- eller CSS-overrides. |
| Visuell | Båda faktiska Firefoxbilderna granskade vid1:1; ETA inom390/1280 och ingen intern horisontell overflow/pageerror. Ingen ny layout/BILD-ändring. |
| Bevarande | HTTP-test läser gammal lagrad hostväg med sluttick8: API behåller ankomst8 och duration8, hela gamla units-raden byteidentisk efter preview, dispatch och listläsning. Native skriver ny ankomst och bekräftar läsning inte muterar den. |
| Drift | Ej integrerad/deployad här; Claude granskar, muterar, integrerar. Ingen migration. |

Webben visar restiden i dagar och ankomsten som lokal klockuppskattning; CLI visar den auktoritativa ankomstdagen. `native-repeat` kontrollerar dessutom det **verkliga** Firefox-previewsvarets `arrival_tick`/`duration_ticks` mot CLI/API/dispatch/PG (ingen request interception). Härledda UTC-stämplar kan skilja någon sekund mellan verkliga anrop; auktoritativa ticks överensstämmer.

## INVENTERING OCH ORÖRDA TRÄFFAR
Sökningar och radnummer: `factor-consumers.txt`, `broad-inventory.txt`, `host-tests-inventory.txt`, `handler-host-inventory.txt`, `client-inventory.txt`, `vault-inventory.txt`.

- Alla faktor-konsumenter som lämnats: `expedition.go` (nya etapper), `ship_hull.go` (skeppshemresa), `unit.go` (ny march), `recall_redirect.go` (ny kurs) anropar redan samma `TravelFactor`; inget behöver dubbleras. `march_start.go` använder samma kärna för preview och dispatch. Redan pågående marschers läsningar använder `LoadActiveRoute`/sparade depart/arrive, och ingen datafix eller startupskrivning införs.
- CLI `cmd_march_preview.go`, webb `march_preview.js`/`marchctx.js`, befintliga klienttester är read-only GET och visar serverfält; ingen hårdkodad hostfart. De lämnas eftersom naturliga båda klientprov och623JS bevisar deras utfall.
- `crew_speed_factor_test.go` host=1 avser **besättningens faktor**, som inte ska ändras. Övriga host-träffar i unit naming/lookup/nomadic-host-tests avser kategori, namn, embarkerings- och stridsregler. `place_reach_test.go` samt handlerns recall/eye/origin/tick-tester bygger uttryckligen redan **lagrade** rutter/tider; dessa bevaras. `unit_arrival_disbanded_host_test.go` gäller upplöst host och försenat event. Samtliga lämnas oförändrade och ingår i fullsviten.
- Varje "halv"/"halved" och2,0 i den breda återstående listan: `battle.go`, `battle_wall_test.go`, `unit_arrival.go` handlar om stormens murabsorption; `standing_orders_naval_test.go` om kornvikt; `ship_repair_test.go` om cederkostnad; `silver_audit_test.go`, `upkeep_circulation_test.go`, `unit_upkeep_silver_test.go` om tidigare soldändring; `food_ordering_test.go` om matranson; `march_start.go`/`crew_speed_factor_test.go` om tomma skeppsbänkar; `upkeep.go` om fältranson och adoption; `embarked_upkeep_test.go` om mat/soldkontext. Ingen av dem beskriver hostens fart.
- Handlerns fras "half" i `found_metropolis.go` avser irreversibel grundningshalva, `order_origin_host_test.go` halva sparade sträckan, `nomadic_host.go` halvförbrukat förråd och `messenger_origin_eye_test.go` testets hosthälft; webb `storms.js` halvton för stormminne. Inte fart.
- Vault uppdaterad smalt: `temenos_nomadic_host_plan` aktuell fart + huvudnot som ersätter äldre rörelseexempel, `temenos_nomadic_host_bygg`, `temenos_terrain` RÖ1-rad och nytt beslut, arkivmanualens spelarfras, beslutregister och verblista. `megaron_plan_rorelsekarnan` och `megaron_plan_rorelse_sparad_vag` är exekverade rörelse-/lagringsplaner, deras "halv" gäller intervall/avrundning och deras rutter sparade: orörda. `megaron_alfatest` har endast taktbyte och inga hostfartstal. Öppningsdispatch/welcome har inga hostfartstal: orörda.
- Varje äldre fartträff kvar i vault-inventeringen: valtabellarkiv20261004 är dåtida mätning; graveyard-fas4 är ursprungligt genomförandekvitto; skeppebesättningsplanens kodsnutt/Non-scope är dess genomförandekontrakt, inte nutida hostbeslut; todoarkiv20260727 är dåtida fynd; spawn-planens `halfQ` är hemisfärdelning. Nomadic-host-planens äldre2,0-exempel står uttryckligen som ersatta av topprutan. Historiken är orörd och får inte läsas som aktuell fart.

## METODISK LÄRDOM
En hastighetsfaktor måste inverteras till tidskostnad. Bevisa sedan både korta och längre rutter: en kort avrundad sträcka kan dölja ett fel i faktorn. Gamla sparade färder ska inte omtolkas när ett nytt tal beslutas.

## KÄNDA AVGRÄNSNINGAR
Testfixturer används för kontroll över terräng och gamla scheman; användarbevisen använder en ny naturligt genererad värld per process. Inget livevärldstillstånd ändras. Nya återkallnings-/omdirigeringsetapper använder den nya faktorn; redan lagrade etapper har kvar sina tider. Nästa nådefrist/startlagerorder är separata slicar.

Repro: `tools/gotest.sh`, från `server/` ren `go vet ./...`, `node --test $(rg --files web/static -g '*.test.mjs')`, `python3 docs/reviews/host-hastighet/mutate.py`, `HOST_PROOF_OUT=native-repeat python3 docs/reviews/host-hastighet/live.py`.
