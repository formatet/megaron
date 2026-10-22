# ORDER 14 — SLUTLIG fiskeregel, Timothy2026-10-22 19:15

## Sammanfattning

Kust och vik håller två fiskare; viken ger dubbelt per fiskare. Hamnens enda
fiskeeffekt är djuphavsplatser efter nivå. Gemensam economy-ägare, migration200
och atomisk engångsövergång före workers/HTTP. Kod `060c8354`, testfixturrättning `190f5b97`, aktiveringsfix `af222df8`, bas `ea97b332`.
Ingen merge/push/deploy eller liveskrivning. Full fresh200Go inklusive aktiveringsfix af222df8 GREEN (world154.066s),714JS/vet GREEN.

## Slice-kontrakt

| Vatten | Platser | Fisk per arbetare och tick | Hamneffekt |
|---|---|---|---|
| Kust coastal_sea | 2 | 1 | Ingen |
| Vik coastal_sea med minst4landsidor | 2 | 2 | Ingen |
| Djuphav deep_sea | 0/1/2/4 vid hamnL0/1/2/3 | 1 | Endast platser |
| river/river_ford | 1 | 0.5, oförändrat | Ingen |

Land utesluter bara sea/coastal_sea/deep_sea. River/ford räknas; saknad
kartgranne gör det inte. Faktisk lokal kapacitet visas på synlig produktiv hex,
men ingen grannterräng eller nytt grannfält skickas. Hamnens handel-/skeppsgrind,
catchmentradie och landbyggnadsregeln för övriga varor är orörda.

## Ägare och övergång

`loadHexRuleTiles` i economy/hex_rules.go laddar kartkontext i en batch;
hexgrid.Neighbors äger geometri. Samma regel används av LoadHexProductionOptions,
LoadHexCapacity, HexBuildEffects och RuleBuildingGoods. Deepplatstabellen bor
här, inte i konsumenterna. Bays höjer intrinsic rate, inte huvudantalet.

Migration200 sätter production_rules deep fish från1/6 till1 och skapar en
engångsmarkör. CompleteFishingRuleTransition kör före workers/HTTP: en TX
låser markören, besöker ägda icke-terminala städer i aktiva världar i id-ordning,
låser staden, frigör överflödiga fiskare med högsta ordinaler och recompute.
Markören sätts klar först efter alla städer. Demolition och övergång använder
SAMMA utdragna liftHexPlacementsToCapacity; ingen ny automatisk matplacering.
Recompute settle:ar gamla lager med sparad gammal rate innan ny rate skrivs.
Efter Claudes review kör RecomputeProduction alltid samma fisktrim, under en
befintlig eller egen placementWriteTx. En tidigare pausad/forming värld som
aktiveras efter startupmarkören trimmar därför vid nästa recompute; markören
kan inte lämna kvar överfulla fiskjobb där. current_world_tick() läser aktiva
världens tick, så startup projicerar inte om andra världar med fel världsklocka.
Replay av startup gör ingenting. Fel i någon recompute rullar tillbaka beskärning och markör.
Arkiverade världar återaktiveras inte eller projiceras om av denna övergång.

HTTP buildGoods behåller explicit cap0; null skulle kunna tolkas som obegränsat.
Befintlig PlaceGubbes cap<=0-grind stoppar otillgängligt djuphav. Byggnadseffektens
levelplatser/rate följer samma ägare; deepuppgradering visar platser1→2→4.

## Bevis och grindar

| Grind | Bevis |
|---|---|
| Named RED mot föregående produktion | TestFinalFishingNumbers på c13443d2: gamla kust-/vikplatser och hamnmultiplikatorer fel; final-rule-baseline-red.log |
| Domän och livscykel | Final-rule-tabell, hamnL0–3, option/capacity/effects/goods, verkligt placerad deep-rate1, founding/restfolk cap0 och hamnrivning i fishing_rules_test.go |
| Övergång | TestFishingTransitionPrunesHighestAndRecomputesOnce:6→2 på kusten, ordinal1/2 kvar, gamla fish10+2settlas12, ny rate2, populationoförändrad, replayno-op. TestFishingTransitionRollsBackPruningOnRecomputeFailure:provocerad stockskrivning misslyckas, alla3jobb kvar och markerfalse |
| HTTP | TestDeepFishingHTTPZeroCapAndHarbour cap0/409→cap1/201/rate1; TestBayFishingHTTPUsesLocalCapacity cap2/rate2 på alla hamnnivåer, inga grannfält |
| Reviewfixens breddning | Fresh200 hela economy/API/kharis/recompute-all GREEN, review-resume-fresh.log; ny vet GREEN, review-resume-vet.log |
| Aktivering efter global markör | TestFishingResumeRecomputesAfterGlobalTransition:paused värld med6kustjobb, annan aktiv värld sätter markören true, aktivera paused och recompute ger2jobb/ordinal1–2/rate2 |
| Hela berörda paket | Färsk200 economy5.488s och API43.477s GREEN, final-economy-api.log |
| Egen fysisk mutant | Separat checkout060c8354:>=4→>=5. OwnerDB ochHTTP RED på utebliven vikrate; byteexakt SHA-restore och ny fresh200 GREEN. final-mutation-*.log/txt |
| JS / vet | 714JS GREEN; final-js.log. go vet ./... GREEN; final-vet.log |
| Full fresh Go | GREEN inklusive aktiveringsfix af222df8, world154.066s,57900; review-full-fresh.log. Föregående gröna final-full-fresh.log avser före aktiveringsfix |
| Fyra ytor | Befintlig Keryx/webb läser servercap/rate; nya exakta fishing/harbourartiklar och rättade Buildings. client-surfaces.md |
| BILD / TEXT | Ingen produktions-JS/CSS/markup eller ny hexpaneltext ändrad. Befintlig kapacitetsrendering fixturetestad för0/1/2/4ochrate1/2. Ingen BILD-grind utlöst; artiklar TEXT i playtest |

Detta är riktig PostgreSQL, HTTP och befintlig renderer med explicit fixtures.
Ingen ny manuell spelarrunda eller Firefox-serie påstås. Äldre0/4- och0/2-order
är ersatta. earlier-rule-report.md och loggar utan final-prefix är historik,
INTE slutregelns bevis. Slutligt verifieringsscript kontrollerar final-loggarna.

Första fulla körningen för slutregeln misslyckades endast i äldre
TestRestfolk_Soak60/scheria1042. Den hade tre kustfiskare och antog konstant
produktionsbrist; nya vikraten gav korrekt tillväxt till1200 före underskott.
190f5b97 ändrar ENDAST fixturen till en fiskare så testets befintliga assertion
om1042 och produktionsunderskott fortfarande prövar lager kontra tillväxt.
Ingen produktionsfix eller försvagad assertion. Hela färska kharispaketet är
sedan grönt (12.300s); den misslyckade fullrundan ligger öppet i
final-full-first-fixture-failure.log. Ny helrunda är GREEN på oförändrad produktion.

## Live före/efter, endast läst

`final-live-capture.py` använder tools/alfa_db.Db:s READ ONLY-transaktion;
raw snapshot och deterministisk ordinalmanifest finns i final-live-snapshot.json,
final-live-impact.json/md. Det räknar placerad fisks potential före/efter,
inte hunger/lager/armé/blockad eller restfolkets mat. 18deepjobb ligger på varsin
hex och hamnL1; inget av dem är över nytt tak1. Deep3→18fisk/tick.
Samtliga placerade fiskjobb i snapshoten305→89fisk/tick;109gubbar blir lediga.
Det är främst kustens gamla hamnbonus som försvinner. Exakta orter/ordinaler i manifestet.
Detta är en projektion, inte utförd liveövergång. Ingen skyddad övergång eller
kompensation: Timothy godkände beskärning enligt slutordern.

Upprepa direkt före deploy från grenroten:

```sh
python3 docs/reviews/fiske-hamn-vik/final-live-capture.py
```

## Integrationsordning och begränsningar

Migration199 tillhör ORDER13 och är INTE kopierad in här. Slutlig fresh-rigg
migrerar denna grens198→200; Claude integrerar13före14 och löser main.go om
båda sätter startupövergångar. ORDER17 ligger separat. Ingen osäker kontroll av
live-schema har gjorts före ny process. Efter deploy verifierar Claude faktisk
processcommit/migration/dirty, completed-markör, rates och nya tak i DB/API.

Landregeln och FOW följer godkänt19:15beslut. Rå snapshot tar ingen reservation;
spelvärlden kan ändras innan deploy. Ingen historisk fiskmängd räknas om.

## Aktiveringsfixens egen fysiska mutation

Separat af222df8-checkout tar bort liftHexPlacementsToCapacity från
RecomputeProduction. TestFishingResumeRecomputesAfterGlobalTransition och
TestFishingTransitionPrunesHighestAndRecomputesOnce ska då bli RED på kvarvarande
överfulla jobb. Originalet återställs byteexakt (review-mutation-sha.txt) och
färska identiska tester körs gröna. review-mutation-*.log är detta bevis, skilt
från den tidigare vikmutanten. Ingen produktionskod ändras efter af222df8. CLAUDE.md:s gamla hamnpåstående rättas till en pekare på fiskägaren/temenos_arbete; inga nya tal dupliceras där.
