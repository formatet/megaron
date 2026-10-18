# Slice11 — serialiserad placering

**Klart för Claudegranskning:** samtidiga placeringsanrop som gav500 serialiseras nu per stad.
Kärna9579ccb6, inget deployat av Codex. Färsk fullGo183(world150.089s), vet,654JS/0skip,
10 fysiska mutationer och naturlig parallell API/CLI/Firefox **GREEN**. Nativehealth9579ccb6/183.
Ingen migration eller liveomräkning behövs; Claude integrerar/deployar Go-koden. Restfolk väntar separat.

Bas e38acc13, gren codex/placering-las. Restfolkets separata WIP är parkerad.

| Kontrakt | Innehåll |
|---|---|
| Problem | Samtidiga POST väljer samma ordinal/läser samma lediga pool och ger500 eller överplacering. |
| Spelarsanning | Parallella placeringar lyckas i turordning eller ger legitim409; inget arbete skapas utöver befolkning/kapacitet. |
| Invariant | Stadens rad låses i samma TX före populations-/placeringsläsning och före andra placeringsskrivningar. |
| Scope | POST/DELETE, economy-funktioner som ändrar placeringar, låsordning vid take, tester/bevis. |
| Non-scope | Restfolk, andra nya mekaniker, livefix av agentkontonas placeringar. |
| Acceptans | N riktiga parallella HTTP-anrop, färsk population under lås, parallell tillväxt/DELETE, alla ekonomins skrivvägar och take utan stadslås-cykel. |
| Stopp | Befintlig skrivväg kan inte serialiseras utan ny spelarregel. |
| Bevis | PG-låsbarriär fångar gammalt fel, fix+fysisk mutation, fullfreshGo/vet/JS, naturligt parallellt API/CLI/Firefox-prov. |

Blockerar arbetskraft→produktion i kedjegrinden. Inga nya spelarord/verb; befintlig409 används.

## Baslinje och resultat

`baseline.log`: åtta verkliga parallella autentiserade POST mot en stad med population100.
En extern PG-transaktion håller målhexen tills samtliga anrop väntar inne i PG. På gamla koden
gör alla sitt pool-/ordinalval före väntan: **201×1,500×7**. På fixen väntar de på staden före
läsningen: **201×1,409×7**. Ingen mockad fördröjning eller respons används.

POST och DELETE låser stadsraden före underordnade placeringsrader/hexar. Ägande och population
läses om efter väntan, så ett populationsfall eller ägarbyte mellan förkontroll och TX inte
låter ett gammalt svar auktorisera ny placering. Ordinal och totalantal väljs under samma lås.

`placementWriteTx` ger economy-skrivare en skyddad TX även när anroparen är en pool; en
fristående FOR UPDATE på poolen skulle släppa låset före nästa query. En befintlig pgx.Tx
återanvänds och commit ägs fortsatt av dess anropare. Growth/PlaceNext avvisar också en gammal
nyföddordinal utanför den nu låsta populationen och en redan full pool med kvarvarande höga
ordinaler efter populationsfall. Starting/prune/reconcile använder samma hjälpare.

### Låsordning och inventering

- POST/DELETE: stad → eventuell hex → placeringar/varor. Båda ägar-/populationskontrollerna är i TX.
- Tillväxtens bulk-UPDATE har avslutat sin statement/rows.Close före PlaceNext/prune-looparna;
  poolanrop får varsin kort stadstransaktion. Reconcile och grundning återanvänder sin redan öppna TX.
- Take ändrar två städer. Förlorarens stad låses med **NOWAIT** innan dess placeringar/byggen/varor
  ändras; upptaget ger atomiskt409 med retry-text. Det undviker cykeln eget stadslås→hex→vänta på
  annan stad mot ett motriktat take eller tickens bulk-UPDATE. Inga delvisa överföringar syns.
- Befintlig ConcurrentTake-regression kräver fortfarande att B till slut ensam äger hexen och
  A har0/B1placering; den gör nu ett enda legitimt retry när konkurrerande A-anrop avslutats.
- Full sökning efter INSERT/DELETE/UPDATE settlement_placement ger endast de två HTTP-vägarna,
  take, starting/next/prune, reconcile och SyncHexBlockade. Blockade ändrar endast en befintlig
  rads metadata, inte ordinal, antal, mål eller pool; dess korta statement behöver ingen ny
  kapacitetskontroll. Cascade vid stadsradering/erase äger redan den borttagna föräldraraden.
- Ingen ny mall/CLI/webb/Codex-formel behövs: alla spelarvägar använder dessa befintliga handlers.
  Befintligt409/besked visas av båda klienterna. Inga API-nycklar, notiskind eller migrationsfält ändras.

### Prov som pinnar invarianten

`TestPlacementConcurrentPOSTsSerialize` ger exakt1framgång/7legitima409.
`TestPlacementUsesPopulationAfterSettlementLock` håller en populationsminskning öppen medan
POST startar och kräver sedan409. `TestPlacementDELETEWaitsBeforeLockingPlacement` bevisar att
en tredje TX kan låsa placeringsraden NOWAIT medan DELETE väntar på staden — omvänd ordning
skulle ta barnlåset för tidigt. `TestPlacementTakeBusyHolderReturnsConflict` kräver snabbt409,
ingen delvis take och en lyckad normal retry när den andra stadsraden släppts.

`TestPlacementGrowthConcurrentRespectsCapacity`: åtta verkliga poolanrop, med en uttrycklig
fixture-only PG-trigger som håller INSERT efter dess kapacitetsläsning. Gamla läsningar kan då
packa åtta på en fyrplatshex; fixen placerar inom verkliga tak. Triggern är endast adversariell
DB-testapparat, aldrig produktion eller det naturliga spelprovet. Särskilt prov pinnar gammal
framtida ordinal och redan full pool. Den äldre tiogubbstestfixturen hade bara500invånare;
den har nu1000 så kapacitetsprovet får sina tio riktiga gubbar, utan att bevara överplacering.

`mutations.py`: sex fysiska kompilerande ändringar, namngivna assertionRED → exakta bytes →
GREEN på varsin ny tom PG16/migration183. SHA256 i mutations.log. Mellanliggande `targeted*.log`
visar först den gamla testfixturens brist och det gamla take-testets krav på omedelbar201;
`targeted-green.log` är korrigerad slutlig kontroll, inga permanenta skips/röda normaliseras.

## Användarprov och begränsningar

`native.py` använder en egen PG16/Redis/server med nya normala konton: registrering/join/grundning,
parallella riktiga POST och DELETE, CLI-placering och Firefox1280/390. Inga SQL-skrivningar,
barriärtrigger, svarsoverrides eller livekonton i det provet. Privata configar/token/binärer
kopieras inte till bevispaketet. Restfolkets formel och GPT-livekonton är orörda i denna gren.

### Anslutningspoolen är också en del av låsordningen

Vid slutgranskningen upptäcktes att POSTs terrain/eyes/remembered-läsningar använde poolen
medan TX redan höll en anslutning. Många anrop kan fylla hela poolen med väntande stadslås;
då kan den främsta transaktionen inte låna sin extra läsanslutning och kön fastnar.
De tre läsningarna använder nu **samma TX**. Visibility-wrapperna accepterar den redan befintliga
province.Queryer-typen; FOW-predikat eller urval ändras inte, ingen ny G1-kant.
`TestPlacementSingleConnectionCannotStarve` kör åtta riktiga POST med **MaxConns1** och kräver
201×4/409×4 inom timeout. Tre ytterligare fysiska mutationer byter tillbaka varsin läsning till
pool och ger namedRED→exakta bytes→GREEN. Totalt **nio** mutationsprov.
Samtidighetsprovens egna pooler är uttryckligt dimensionerade för åtta blockerade transaktioner,
så barriären inte beror på testmaskinens CPU-antal/defaultpool. Produktionspoolen ändras inte.

Kärnaea5ecb6f kompletterad av0eb9201e för pool-/FOW-läsningen. `full-go.log` är mellanprovet
före kompletteringen; **full-go-final.log** prövar kompletteringen före slaktvägens låsordningsfynd. Ingen slutlig grön kontroll
påstås utifrån mellanprovet. `vet-final.log` gäller den mellanversionen. Slutlig helkontroll är full-go-complete.log/vet-complete.log på9579ccb6.


### Sista skrivvägen: slaktens låsordning

SlaughterLivestock kan anropa PlaceNext när populationen korsar hundratal. Inventeringen visade
att dess TX först låste livestock-varan och därefter staden — motsatt både placering och
FoodTick/Sitos. Den läser nu population/ägare under stadslås **före** varudeduktionen, samma
ordning som den redan skyddade Recruit-vägen. Själva slaktregeln, kostnad och befolkningsvinst
är oförändrade. Kharis bulk-decay/growth är separata poolstatement som redan avslutats före
placeringslooparna; förstärkningens goods/pop-uppdateringar är också separata statement och
håller därför inte ett varulås över väntan på en stadsrad. Disband låser enheter före population;
placeringsrecompute läser enheter utan enhetsradlås och bildar ingen sådan låscykel.

`TestPlacementSlaughterLocksSettlementBeforeGoods` håller stadsraden och bevisar med en tredje
TX:s NOWAIT att varuraden fortfarande är olåst medan slakten väntar. En tionde fysisk mutation
tar bort just det tidiga stadslåset: namedRED→exakta bytes→GREEN. Produktionsslutkod9579ccb6;
alla tio mutanters restaurationshashar finns i mutations.log. Hela sviten och naturliga provet
körs därefter om; mellanproven ersätter inte slutbeviset.


## Slutlig överlämning

`full-go-complete.log`: hela färska PG16/migration183, -count=1/-p1, värld150.089s, alla paket gröna.
`vet-complete.log`: tom, exit0. `js.log`:654pass/0fail/0skip; inga JS-källor ändrades under slicen.
`native-final/evidence.json`: health9579ccb6/183,0pageerrors. Inland och kust har vardera
POST201×1/409×7 samt DELETE200×1/404×7; riktiga CLI-borttagningar/återplaceringar också gröna.
Tio kompilerande mutationer var namngivet röda och byte-exakt återställda till grönt.
Go-kod och tester i denna leverans är exakt9579ccb6; senare commit innehåller bara bevis.

Allt lämnas på codex/placering-las. Ingen push, merge, deploy eller liveplaceringsreparation.
**Hash + STOPP enligt Claudes slice11-order.** Separat codex/restfolk har ofärdig egen WIP,
kontrakt och före60tick; den återupptas efter denna integration. GPT-runda1: e6d19f24 på codex/gpt-spelare.
