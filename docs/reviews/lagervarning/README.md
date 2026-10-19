# Order29 — lagertak, förvarning och redovisat överflöd

## SLICE-KONTRAKT

Problem: kolonins silvertak klipper en verklig leverans utan varning eller förlustbesked.
Spelarsanning: kolonier har samma silvertak som huvudstäder; nära fullt lager
visar en prognos och en dispatch, verklig förlust visas med mängd/orsak.
Invariant: inga saldon höjs i migrationen; bara tak höjs. Krediter, audit och
notis sker i samma TX, retry skapar inte en andra förlust/notis. Gammal
leveransaudit tolkas oförändrat; nytt utfall får egen eventtyp.
Scope: lagerkredit/latproduktion/prognos, order26:s övergångsminne, migration,
API+keryx+web+Codex. Inga nya lagerpooler eller warehouse-byggnader.
Acceptans:150silver till960 utan förlust;150 tilltomt60 ger90 förlust och
varning före ankomst;10ticks samma period→1varning; retry/omstart säkra;
mutation utan förlustnotis eller inkommandeprognos blir RED.
Stopp: konflikt med frusen auditsemantik, FOW eller lagerpoolsbeslut tas tillClaude.
Bevisplan: färsk baslinje, faktiska leverans-/produktionsvägar, pipeline/retry/
ägare/övergångsprov, fysiska mutationer, full freshGo/vet/JS och naturligt prov.
Kedjegrind: **bevisar** att hemförd resurs och bytesbetalning kan förstås och
planeras utan tyst förlust på vägen brist→brons.

## BASLINJE

Faktisk grenbas4506c09e (28a och27 integrerade); inga ändringar i huvudträdet.

## LIVSCYKEL OCH ÄGARE

`credit_settlement_good` är SQL-ägaren för varukredit under samma radlås/TX.
`storage_capped` skriver nytt StorageOverflow-utfall, inbox och befintligt
scheduled_events-jobb atomiskt när en verklig kredit eller realiserad produktion
klipps. Transport behöver ingen förbjuden import till economy. Befintliga
leverans-/gåvoutfalls betydelser lämnas kvar; det nya utfallet är additivt.
Livejobbet återanvänder den redan sparade notisidentiteten och återskapar aldrig
arkivrader som spelaren tagit bort. Produktionsankaret flyttas med realisering,
så samma lagerintervall inte kan förlora samma mängd vid retry.

Prognosen komponeras i combat (Upkeep/PlanFood), injiceras via konsumentgräns
inkharis och läses från samma funktion i API. Order26:s recordTransition och
kharis_notice_state återanvänds, med fem observerade fria ticks innan en ny
varningsperiod kan börja. Återgången ger ingen extra inboxrad.

Claude19:30 bekräftade FOW: egna transporter och accepterad handel får räknas;
oombedda främmande gåvor räknas först vid fysisk ankomst. Nytt API-fält visas
bara för stadens autentiserade ägare. Ingen dold transportinformation lämnas ut.
