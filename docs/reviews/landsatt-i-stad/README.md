# Landsättning i egen stad — inventering och kanonstopp

Bas `762c5672`, egen gren `codex/landsatt-i-stad`. Ingen produktionsändring.

Problem: land-mission avvisar egen stad och webbens marchval utesluter skepp till landmål. Önskat: skepp dockar kvar i egen kuststad och samtliga passagerare blir garnison; bar marks befintliga retur bevaras. Detta blockerar logistiken i kedjegrinden.

## Kontrakt

- Samma resa och klockägare som befintlig sjömarsch; ingen ny stridsmekanik.
- Egna stadsankomster landar alla CargoIDs och lämnar skeppet i målhamnen.
- Främmande stad använder befintlig amfibie eller ett handlingsbart avslag.
- Fyra ytor: server, webb War/march, Keryx, sjötransportartikel.
- Rött först; mutation av stadsgrind och kvarliggande skepp; Firefox desktop/mobil.
- Stopp: beslut om försörjningsankare enligt orderns punkt 5, innan produktionskod.

## Inventering

`combat/march_start.go`: land-mission kring rad 355 avvisar alla aktiva settlements. Befintlig amfibie kring rad 327 gäller lastade skepp mot främmande kuststad utan land/passage-intent och märker resan assault. `combat/unit_arrival.go:landArrived` sätter alla CargoIDs på bar mark och skickar skeppet hem. `arriveGarrison` sätter fysisk settlement_id men ändrar inte support_settlement_id. `handlers/unit.go:Unload` gör alla passagerare till stadsgarnison, behåller deras försörjningsankare. Webbens `marchctx.js` behöver erbjuda egen kuststad som skeppsmål.

## Konkret konflikt — skickad till Claude

`combat/upkeep.go` använder support_settlement_id som silverbetalare. `combat/march_start.go` använder samma fält som proviantkälla vid nästa sjöavgång. Reparation läser fysisk settlement_id och dess varv. Att enbart parkera skeppet i målstad lämnar dess gamla stad som betalare/proviantkälla, även på avstånd. Att flytta support till målstad ändrar försörjningsankaret.

A: ändra endast fysisk hamn, behåll support. B: ändra skeppets support till målhamnen, behåll passagerarnas enligt vanlig Unload. Rekommendation B, men ordern kräver STOPP vid upkeep/proviantkanon. Inväntar svar; inga tester eller bilder för en implementation påstås.


## Beslut och genomförande

Claude16:01 förmedlade Timothys beslut **A**: fysisk settlement_id=mål, support_settlement_id oförändrad för både skepp och passagerare, inklusive nästa avgångs proviant från gamla staden. Det är ett uttryckligt försörjningsbeslut; den tidigare frågan är besvarad.

Ny intern sparad mission `dock` bär den valda stadens hex i de befintliga land_target-fälten. Sjövägen och restidspriset till kusthexen behålls; ankomsten dockar i den valda staden. Befintliga land/passager/pickup-missioners persistenta tolkning ändras inte. Både vanligt segelmål egen stad och land-mission egen stad ger denna nya mission. Alla CargoIDs går i garnison, carried_by_ship_id släpps, skeppet stannar. Försörjningsankare skrivs aldrig om. Om staden hunnit byta ägare seglar skeppet hem med passagerarna; ingen automatisk ny strid.

Riktig DB-baslinje `baseline-red.log`: både land och vanlig segling till egen stad nekades. `first-green.log`: båda vägarna fungerar med tre kohorter på Emporos, dessutom tomt skepp; dubbel ankomst är idempotent, ingen oombedd retur, support oförändrad och nästa uttryckligt beställda resa tar proviant ur gamla staden. Separat prov täcker ägarbyte innan ankomst; gamla bar-mark-/koloniseringstester är gröna.

Cities API ger coastal och has_harbour för valytan; byggnadsuppgiften has_harbour exponeras bara för egna städer. Rykten får ingen sådan information. Webb/Keryx/Codex beskriver samma kontrakt.
