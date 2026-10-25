# Hex för hex: frakt och budets landben

Gren `codex/hex-for-hex-frakt-bud`, från `8fa5d1c4`. **Ingen merge, push eller deploy.** Artikelutbyggnaden är fortfarande köad; Varvet är orörd.

Frakt och bud står nu kvar på starthexen tills nästa hex verkligen nås. Deras befintliga avrundade restid behålls. Varje terrängkostnads prefix fördelar hexankomsterna över den totalen, från faktisk avgång. Server, webbkarta och Keryx läser den sparade rörelsen; sikt får aldrig börja från en framtida hex.

**Kod och slutresultat:** `f657a960`, migration 214; återstartens hela färska Go-svit grön (26 paket med tester, world 150.165s), vet grön. **BILD:** [native Firefox, desktop och mobil](index.html).

## Inventering och ägare

Inventeringen skickades i `chat.log` 2026-10-25 10:23, före implementation. Två verkliga baseline-fel finns i [baseline-red.log](baseline-red.log): karavanen stod i hex 1 direkt vid avgång, och det hemvändande budet såg från hex 2 innan det hade lämnat hex 3. Samma prov blev [gröna](first-green.log).

| Tidigare skrivare/läsare | Slutligt ansvar |
| --- | --- |
| `province.TradeJourney` | Behåller sin enda frysta geometri/kostnadslista; sparar `movement.RouteClock` som metadata. Ingen andra vägkopia. |
| `transport.prepareJourney`, `Dispatch`, `CreateShadow` | Fryser klockan efter avgångsvälsignelsen. Tickfält och tidsstämplar projiceras från samma klocka. |
| Shadowdispatch i `handlers/province.go`, `handlers/messenger.go` | Transfer, gåva och avtal använder samma faktiska avgång för kvitto, fysisk rad och deadline. |
| `economy/trade.go` | Varje faktiskt avsänt returben stämplas på nytt, även kedjad vederlagsfrakt och tom hemsegling. Ekonomi importerar inte transport eller movement: G1 behålls. |
| `ship_hospitality_transport.go` | Inventeringen visade endast duration-/proviantläsning, ingen fysisk dispatchskrivare. Ingen onödig ändring. |
| `ResolveDeparture`/`ResolveReturnDeparture` och deras anropare | Behåller vägval och avrundad duration. Efter välsignelse fryser `SaveDepartureTx`/`SaveLandRouteTx` det faktiskt avsända landbenet; `EnqueueLandTx` läser dess deadline och passagegeneration. |
| `StartReturnLeg`, `CallBack`/supersede, bordning och landningswitness | Sparar respektive aktuellt landben. Återkallelse vänder från nådd hex. Budet måste ha nått hamnen före bärarens avgång. Efter landstigning används det egna benets avgång, inte brevets ursprungliga `sent_at`. |
| `SavedPosition`, `journeyRoute`, `ReadMarch`, live eyes, markörer | Konsumerar sparad movement-rutt. Ny position räknas aldrig om från ändrad terräng. |

`movement.NewRouteClock` är den gemensamma konstruktorn. `tick.Anchor` omvandlar avgången till tickkoordinaten; klockan sparar offset, faktisk avgång och kadens. Tidsstämplar fryses i PostgreSQL:s mikrosekundprecision, så en återläst ETA inte hamnar en bråkdel av en mikrosekund före sin egen slutgräns. Prefix behåller movement-kärnans millitickkvantisering.

## Händelser och gamla rader

Ny deadline-tolkning får nya händelsetyper. Scheduler testar både tickbehörighet och den sparade `process_after`, och behåller ankomstprioriteten.

| Befintligt schemaevent | Ny frakt |
| --- | --- |
| `TransportArrival` | `TransportArrivalV2` |
| `TradeDelivery` | `TradeDeliveryV2` |
| `GiftDelivery` | `GiftDeliveryV2` |
| `ShipmentDeliveryV2` | `ShipmentDeliveryV3` |
| `TradeReturn` | `TradeReturnV2` |

Bud använder del 1:s precisa `OrderDeliveryV2`/`MessengerArrivalV2`/`MessengerReturnV2`. Nytt eller ersatt landben får ny passagegeneration; gamla timers kan inte verkställa det. Befintliga V1-event behåller sina hanterare och payloadtolkningar.

Migration **214** lägger till nullable `messengers.land_route`. Gamla bud med NULL-rutt och frakt utan `journey.clock` behåller sin uttryckliga legacytolkning tills respektive ben tar slut. Ingen retroaktiv rutt söks fram eller skrivs om vid deploy. Nästa faktiskt avsända ben får den nya klockan. [Schema 213→214→213→214](schema.log) verifierades på egen färsk rigg.

## Sikt, storm och interception

Frakten projicerar sin sparade klocka till del 1:s `StoredRoute`; därmed använder stormar, platsinträden och ETA samma gränser. Befintlig tidsordnad batch kör även fraktens sentry-kontroll vid nådda prefix. `InTransitAt` avgränsar den till avgång–slutgräns: en redan lagrad rad får inte fångas i en tidigare återspelad tidpunkt, eller efter fysisk ankomst. Slutgränsen ingår för ankomstens riskordning.

Budens egna landben ger öga och prefixminne först när hexen nåtts. Ombord, före ett framtida landben, förseglat eller med obehandlat bärarutfall ges inget separat landöga. Räddningssekretessen kvarstår. Bud får varken interception eller geografisk namngivningsrätt.

**Inga nya per-hex-jobb behövs:** del 1:s återspelning går igenom sparade prefix och vädergränser före precisa ankomster. Befintliga tickbatchar sparar budens nådda prefix även utan kartpollning; terminaljobbet sveper innan budet lämnar resestatus. Idempotenta minnes-/utfalls-/progressionsskrivningar klarar upprepning. Vanlig workerpoll-latens finns kvar; bilden visar fysisk rörelse och därefter verkligt bokförd leverans.

## Fyra ytor

- **Temenos:** frysta klockor, precisa jobb, faktiska returben, sikt och risk per nådd hex.
- **Megaron:** viktad frakt-/budposition och återstående väg från samma metadata; bud använder det aktuella landbenets tidsfönster. Nya frakt-ETA:er visas från den sparade tidsstämpeln.
- **Keryx:** `cargo` visar aktuell hex och sparad ETA; dispatch-/avtalskvitton använder den precisa tiden. Verklig Keryx kördes i alla tre bildfaser.
- **Codex:** befintliga `trade.md` och `runners.md` beskriver förloppet. Den separata ordern om artikelutbyggnad har inte startats.

Gate: bevisar att fysisk logistik och bud för geografi→brist→brons kan läsas på befintliga ytor. Inget separat mänskligt textspeltest eller godkännande påstås.

## Verifiering

- [Färsk fullGo](full-go.log), [hela JS-sviten](full-js.log), [vet](vet.log), fyra `day-*.log`.
- [Fem fysiska mutanter](mutations.json): tidig frakthex, tidigt budöga, tidig bordning, frakt utanför resefönstret och fraktjobb före deadline. Var och en gav en riktig testassertion röd; originalbytes återställdes exakt. Körbar reproduktion: `tools/freight_hex_mutants_20261025.py`.
- [Portgräns röd](port-precision-red.log) → [grön](port-precision-green.log): ingen bordning 30ms före nådd hamn. [Fraktfönster rött](freight-window-red.log) → [gröna gränsprov](final-boundaries-green.log): varken framtida eller avslutad resa fångas; en pågående resa fångas som positiv kontroll.
- [Bildproveniens](bild-handover/proof.json), [fixtur](bild-handover/fixture.sql), riktiga API-/Keryxsvar och sex nativebilder. Reproduktion: `python tools/freight_hex_bild_20261025.py OUTPUT`. Egen färsk PostgreSQL/Redis/server, inga prod-/acc-ingrepp, ingen DOM- eller svars-mockning.

Äldre fixtures läser nu faktisk V1/V2/V3-jobbtyp och korrekt generation; historiska V1-envelopeprov behåller generation 0. ETA-provet jämför med samma mikrosekundprecision som lagringen. En callback-från-hamn-fixtur låter först budet nå hamnen; separat prov täcker återkallelse ute på vägen.

## Återstart: kvarvarande klockprov och korsningsbild

Produktionsrättning `f657a960`: ombordbudets karta hade korrekt fysisk position men ett felaktigt webbfönster från `boarded_at`. V2-fönstret kommer nu från bärarens sparade avgång och slutgräns, både för enhet och frakt. Det tidigare ofärdiga testet är verkligt [rött på checkpointen](resume-carrier-test.log), [grönt med rättningen](resume-carrier-green.log), och [rött när rättningen fysiskt tas bort](resume-carrier-mutant.log). Originalbytes återställdes exakt.

[Utökad Firefoxrigg](bild-crossing/proof.json): åtta nativebilder i 1280/390, tre samtidiga korsande rutter vid 0.5/15.4/23.4 sekunder, inklusive ett hemvändande bud; riktig karavanleverans vid 34.4 sekunder. Karavanen går genom verklig transfer-API och Keryx. De två korsande aktörerna är öppet loggade DB-scenfixturer på samma sparade klocka, inte ett påstående att dessa skickats genom order-API. Samtliga källhashar oförändrade och inga sidfel. Linjernas befintliga pixelmönster är oförändrade; ingen före/efter-stiländring.

En första återstartskörning av bildriggen underkändes korrekt av källhashvakten när mutationstestet kördes för tidigt parallellt. Den körningen är inte bevis; efter exakt restore kördes riggen rent igen. Fullsviten startades om från färsk DB efter restore; se `resume-full-go.log`.

Slutkontroll: hela `resume-full-go.log` grön efter exakt restore; `resume-vet.log` grön. Alla 949 produktionshashar i bildprovet matchar slutkoden. Tidigare 770 JS, schema- och fem mutationsbevis gäller oförändrade delar; återstarten tillför ett sjätte mutationsprov för bärarens webbfönster.
