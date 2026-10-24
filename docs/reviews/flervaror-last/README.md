# Flera varor, en försändelse

A genomför server, Keryx, webb och Codex på bas 38cb103b. Produktionscheckpoint
8b87b297; migration 211. Claude äger granskning/integration. Ingen prod/acc-kontakt,
merge, push eller deploy. Varvet 7ad899ac och truppmodell B är orörda.

## Kontrakt och resultat

Transfer och gåva accepterar `items: [{good_key,quantity}]`, sammanför dubbletter
och drar hela manifestet atomiskt. En försändelse binder ett skrov, levererar alla
varor och skapar en tom hemresa. Gamla `good_key`/`quantity` och deras eventhandlers
fungerar fortsatt. Nya listpayloadar har en egen ShipmentDeliveryV2-timer och
GiftDispatchedV2/GiftDeliveredV2/GiftLostV2/TransferDeliveredV2/TransferLostV2.

Kapacitet är summan av antal: Galley 20, Emporos 120. Samma enhet gäller avtal
(båda benen) och stående rutters ut-/returmanifest. Karavaner saknar lasttak.
Ockupationsplundringens viktregel ändras inte. Skepp med trupper väljs fortsatt
inte för godsfrakt. Ett nytt nullable transport_id på trade_routes länkar dess
fullständiga manifest, medan äldre scalarfält är bakåtkompatibla.

Webb och Keryx visar ledigt skrov och Cargo före sändning, uppdaterad lastmätare
på webben, handlingsbart överlastfel och alla varor under färd. Preview reserverar
inte skeppet; POST prövar allt igen i sin transaktion. Gåvohistorik, korrespondens,
notiser och kartans partsvisning följer listan. Utomstående får ingen lastinformation;
mottagarens personliga notis/arkiv avslöjar inte givarens stad. Fysiska budpassagerares
hamnankomst registreras även vid listleverans och exakt en gång vid retry.

## Bevis

- Ren bas 38cb103b: TestTransferManifestAtomicDelivery RÖD (400 på items).
- API-prov: blandad sjölast, en transport/skrov/hemresa, exakt120, totalöverlast
  utan debitering, saknad vara med rollback, dubbletter, gåva och retry,
  gammalt singelformat, komplett förhands-/rutt-/kartdata och sekretess.
- Transportprov: tvingad kapning, skada och förlisning av silver100+grain20;
  hela lasten tas, varje vara halveras eller alla går förlorade. Retry duplicerar inte.
- Fyra fysiska huvudmutanter RÖDA: per-varukapacitet, gamla vikter, bara första
  varan levererad, borttaget avslag. Två stående-ruttmutanter RÖDA. Exakt restore.
- [Firefoxgalleri](index.html): tom, delvis fylld och överlastad lastmätare samt
  faktisk tvåvarulast i färd, 1280/390. Även verklig Keryx med upprepade flaggor.
- Körloggar i [tests](tests/); slutlig fullsvit och provenance sammanställs nedan.

## Avgränsning

B (tre kohorter/Emporos) börjar först efter att A mergats. Avtalens varuformat
förblir singulärt; deras kapacitetsenhet är uppdaterad. Inga event har fått ändrad
payloadtolkning. En tidig röd-först-körning mot delad pågående agentredigering gav
kompilationsfel; endast separata rena baselinekörningen ovan är baslinjebevis.

## Slutlig verifiering

- 764 JavaScriptprov gröna, inklusive Codex. `go vet ./...` och alla fyra
  day_guard gröna. Bildgalleriets alla lokala länkar fungerar.
- Full Go-svit körd en gång på färsk migration 211 från 8b87b297. API-paketet
  gav det befintliga TestSilverFlowRecruitHTTP/merchantman-felet: fixturen hade
  16 ceder, medan bascommitten redan kräver 90. Samma fel reproducerat på
  oförändrad 38cb103b och en separat färsk migration 210.
- Endast fixturen rättad 16→90 i separat commit ceead828; därefter hela
  API-paketet grönt på en ny migration 211 (60,909 s). Hela sviten körs inte
  om; dess övriga paketresultat och API-omprovet redovisas separat.
- Fyra huvudmutanter upprepade mot ceead828: alla RÖDA. Exakt återställning
  verifierad med 935 oförändrade produktionshashar och nytt grönt manifestprov.
- Hälsosvaret i slutriggen var 8b87b297/migration211. 8 Firefoxbilder, noll
  sidfel; faktiska webbsändningar och Keryx-sändning. Samtliga 935 källhashar
  matchar även efter testfixturcommitten, som bara ändrar ett test.

Fullsvitens övriga 25 testpaket var gröna, inklusive world 148,867 s.
Det enda röda paketet var API enligt fixturfelet ovan; slutligt separat API-omprov grönt.

## Granskningsrättning efter 6111d5c6

Claude påpekade att listtransfer hade fått ett nytt förlustfall vid ägarbyte.
Det är borttaget. `TestTransferV2MatchesLegacyDestinationChanges` kör gamla och
nya leveransvägen på samma fyra typer av ändrade destinationer: ny ägare,
kollapsad stad, ägarlös stad och raderad stad. Alla fyra var RÖDA före ändringen,
och är nu GRÖNA. Legacy krediterar även en befintlig kollapsad/ägarlös stad;
när destinationen raderats försvinner rutten och leveranshandlingen gör ingenting.
V2 följer detta exakt. Gåvans separata regler ändras inte.

Egen fysisk ägarbytesmutation RÖD; exakt restore och färska manifest-/gåvo-/
förlust-/legacyparitetsprov GRÖNA (1,683 s). Hela sviten körs inte om för denna
avgränsade rättning. Bildriggen byggdes från 8b87b297; efter denna rättning skiljer
sig en av dess 935 produktionshashar (`shipment_v2.go`). Webb/Keryx/Codex är
byte-identiska med bildriggen. Ingen ny slutserverbuild påstås.
