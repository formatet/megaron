# S — gåva och tribut — WIP, PAUS

**Inte färdig och inte för merge.** Paus på Timothys/Claudes uttryckliga order
2026-10-08 (sessionsrensning). Gren `codex/gava`, worktree
`/home/tk/wt/codex-gava`, bas `eb523aa2`. Nästa session läser huvudträdets
`.agents/restart-codex.md`, aktuell `CLAUDE.md`, chatlogg och detta checkpoint.
Claude integrerar; Codex gör ingen merge/push/deploy.

## Kontrakt och beslut

Transfer kan skicka silver/varor till en annan Wanax utan motprestation eller
samtycke. Samma brev-FOW-grind: `VisibleOrigins` + `VisibleFrom(...,6)`.
Fysisk rånbar karavan eller riktigt bundet skepp. Intern transfer behåller sina
befintliga regler. Fyra ytor: server, Keryx, webb, Codex. Gåvan är en rad bland
breven; ingen relationsliggare/summering. Nya GiftDelivered/GiftLost bär
sändarens och avsedda mottagarens Wanax-id och stad-id samt faktisk mottagare.

Timothy valde **B, PLAYTESTFRÅGA**: staden får lasten även efter ägarbyte; faktisk
ny ägare får gåvan, sändare/avsedd mottagare får också besked. Fullt lager får det
som ryms, resten förloras med exakta mängder. Fallen/ägarlös/raderad stad ger
GiftLost. Ingen godsretur i dessa leveransfall. Vanligt skepp går hem tomt för
att frigöras genom befintlig ankomsthandler.

Riskorder 19:26 ersätter tidigare provkrav: ingen egen gåvorisk eller nya
förlusttärningsprov. Befintlig leveransrisk har flyttats till `deliveryLoss`,
anropad av både gamla leveranser och gåvor. Den gamla platta tärningen finns
kvar tills Claude beställer T; T får ett gemensamt ställe att ändra.

**Sjörån, Claude bekräftat 19:43:** bevara befintliga captured/limped/sunk.
Limped förstör halva godset och skickar resten i damaged_return. Nästa steg är
att GiftLost redovisar `returned_quantity` separat från `lost_quantity` och
mottagarens `credited_quantity`. Nuvarande WIP redovisar felaktigt hela lasten
som lost i detta fall. Ändra inte själva interceptionsutfallen.

## Klart i WIP

- GET `/provinces/{provinceID}/trade/destinations`: egna och kontaktade aktiva
  städer, ägarmärkning, inga dolda namn/id:n. Kontaktkällans SQL flyttad till
  province som nedåtriktad gemensam källa för API och capabilities.
- Riktig Transfer-handler godtar främmande kontaktad stad, ägarlåser båda
  ändpunkter, atomisk debit/mover/audit/timer. Ny GiftDelivery med fryst separat
  payload; egna gamla TradeDelivery-payloads behålls.
- GiftDelivery krediterar faktiskt lager under lås med cap/lazy-stock, exakt
  kredit/förlust, faktisk ägare och bestående identitet. TX-audit/notiser och
  idempotens både för samma och ny timer-id. Hårdraderad destination hanteras.
- GET `/gifts`: endast egna avsändningar och parternas utfall; mottagaren kan
  inte läsa avsändningen före ankomst. En senaste rad per transport.
- Webb: Economy Transfer med ägarmärkta främmande destinationer även vid en
  egen stad; Diplomacy visar gåvor bland brev utan Accept/Reply för gåvoraden.
  Notiser visar exakta mängder och ägarbyte. Sena destinationssvar får inte
  fylla ett ersatt formulär.
- Keryx: destinationens namn löses ur kontaktlistan (UUID direkt till server),
  främmande utskrift säger Gift; exakt leveransnotis. Ny Codex-artikel och
  indexkoppling GiftDelivered/GiftLost.
- Sjöleverans följer riktigt bundet skepp genom tom hemresa till garrison,
  inklusive raderad mottagarstad. Hjälparen tål NULL endpoint-id efter delete.

## Verifiering — faktiska utfall

`baseline.log`: färsk PG16/migration160, befintlig intern transfer/karavan grön.
`baseline-js.log`: **437** tester gröna (tidigare README sade felaktigt 433).
`red.log`: riktig handler/DB silver till kontaktad främmande stad gav **403**
own-only (ordertexten angav 400), acceptansprovet rött före kod.

`destinations.log`, `destinations-restored.log`: kontaktad/dold lista gröna.
`mutation-destination-list-fow.log`: borttagen FOW gör hidden-listprovet rött.
`dispatch-green.log`: främmande dispatch201 + befintlig intern fysisk transport,
exakt engångskredit och utebliven kredit efter raid gröna.
`lifecycle.log`: färsk Gift-handler-svit **grön**, inklusive silver/timber,
lagertak, ägarbyte (3 notiser), kollaps/hard delete, markerad raid, två replay-ID,
naval tom hemresa/garrison och privat historik. Denna fokuskörning gjordes före
senare tillagda capability-provet nedan. Markeringen av raid är en fixture;
**en riktig fysisk raid i spelarrigg är ännu inte bevisad.**
`surfaces.log`: föregående fokuskörning grön, före sista nya CLI/capability-prov.
`vet.log`: `env -i ... go vet -p 1 ./...` **exit0**.
`js.log`: **440/440 pass**, actual drawer/notis/destination-loader-test inkluderat.

`full-go.log`: `tools/gotest.sh` färsk PG16/migration160 **exit1/RÖD**.
Enda rapporterade testfelet: `TestGiftCapabilityWithOneOwnCity/contacted`.
Destinationkravet är true men varukravet false trots att fixture har silver i
lager; canTransfer använder befintlig varusökning som exkluderar silver.
Alla andra paket gröna, även nya CLI-prov. **Tidigt besked om full Go grön var
fel: det läste paketrader innan processen avslutats. Rättelsen skickad till
Claude. Den fulla sviten är inte godkänd.**

## Resume — exakt nästa steg

1. Rätta transfer-capability så dess lagerkrav matchar verklig Transfer, även
   silver och andra shippable goods. Behåll handelsförslagets varufilter.
   Kör kontaktad/dold capability-provet på färsk DB. Fixa whitespace i
   economy.js:271 (`git diff --check` rapporterade trailing whitespace).
2. Implementera separat returned_quantity vid limped sjörån, under samma
   idempotenta utfallslås, och visa mängden i webb/Keryx/Codex. Följ verklig
   damaged_return genom returnerade varor, skeppsrelease och nya gåvonotisen.
   Bevara befintliga naval-interception-event och respektive fysisk väg.
3. Slutmutationer på färska DB: dispatch-FOW bort, destinationskredit bort;
   täck cap/ägare/replay/ship-release där det behövs. Inga nya tärningsprov i S.
   Hittills bara destinationslistans FOW-mutation verifierad.
4. `tools/gift_live.py` är **ny, py_compile godkänd men ALDRIG körd**. Ingen
   proof.json/bild eller byggd livebinär finns. Granska riggen innan start:
   den väljer nära partner via vanliga joins, kontakt vid behov via expedition,
   skickar silver i webb och annan vara via CLI, läser mottagarlager/notiser,
   marscherar mottagarsentry för en fysisk raid. Antaganden om spawn/landväg,
   notifications-response och fångstgeometri måste verifieras; riskutfall
   arkiveras, inte döljas. Den skapar bara egna PG/Redis och SQL är read-only.
5. **BILD Firefox först, Chromium och WebKit också**, desktop1280×900 och
   mobil390×844 touch. Riggen står fortfarande på Chromium enbart och måste
   utvidgas före körning. Installera saknade motorer under HOME. Kör samma
   Transfer-lista/tråd-rad i alla tre; notera faktiska skillnader och overflow.
   Timothy dömer Firefox. Inga ögonbevis är klara.
6. Kör slutlig full fresh Go, vet, JS och diff-check efter alla ändringar.
   Lägg verkliga leveranspayloads i bevispaketet (fält ska läsas från riktiga
   payloads, Q:s timber/lumber lärdom). Uppdatera README/vault/verblista vid
   behov, slutcommit med branch/hash/resultat/mutationer/BILD och STOPP.

P/Q/R och huvudträdets övriga arbete lämnas orörda. Inga nya orders/slices
startas under pausen. Ingen live-server/container startades för S-riggen.
