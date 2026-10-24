# Flervaror-last: historisk inventering och STOPP

STOPP hävdes av Claude med START A. Aktuellt resultat finns i [README](README.md);
nedan bevaras inventeringen som låg till grund för uppdelningen A/B.

Bas: 38cb103b. Hela ordern inklusive båda tilläggen läst. Slutliga beslut:
en vara = en lastplats; Galley 20, Emporos 120; Emporos upp till tre landenheter.
Inga produktionsändringar eller tester har gjorts. Varvet 7ad899ac är orörd.
Arbetet stöder kedjegrindens logistik; implementationen väntar på uppdelning.

## Befintlig varuväg

- `server/api/handlers/province.go`, Trade: intern transfer och gåva delar väg,
  med kontaktkontroll för gåvan. En enda good_key/quantity i API och trade_routes.
- `transport.CreateShadow` skapar den fysiska försändelsen; transport_goods har
  redan en rad per vara. ScheduledTradeDelivery respektive ScheduledGiftDelivery
  driver leveransen för dessa shadow-transporter. De gamla payloadarna är singulära;
  listformat behöver nya eventtyper och bibehållna gamla handlers.
- `transport/arrival.go` läser hela transport_goods. `intercept.go` läser hela
  manifestet, krediterar hela lasten vid kapning och bygger halva varje mängd
  vid damaged_return. Skrovet binds till transporten, inte varje varurad.
- Inget karavantak hittat i Trade eller transport.Dispatch/CreateShadow.
- Bilateral handel har egen väg i economy/trade.go och handlers/messenger.go;
  heaviestTradeLeg räknar tyngsta benet. Stående order har redan listmanifest och
  egna ut-/returlastberäkningar i combat/standing_orders.go. List-API för avtal
  är separat arbete; kapacitetsenheten behöver däremot ändras konsekvent där.
- Gods-viktens kapacitetskonsumenter: Trade, heaviestTradeLeg, standing_orders.
  IsShippableGood och capabilities/trade_verbs använder positiv vikt som fraktflagga.
  combat/occupation.go använder vikt för plundringsmängd: lämnas utanför lastrumsändringen.

## Stoppvillkoret i tillägg 2 har inträffat

Tre kohorter är en annan datainvariant än flera varor. `units.cargo_unit_id`
är en enda referens, inte ett manifest. Följande måste övergå till en samling:

- Load/Unload i handlers/unit.go: låser, sätter och tömmer en enda referens.
- combat/unit_arrival.go: pickup, hemkomst, landstigning och amfibieanfall.
- combat/ship_hull.go och sea_storm.go: skada/förlisning läser en enda kohort.
- combat/collapse.go, recall_redirect.go, storage_forecast.go samt marsch- och
  klientreadmodeller konsumerar samma singelreferens.
- Ny relationsmodell kräver migration/backfill, samtidighetskontroll och
  oförändrad kapacitet för galär/krigsgalär. Rekrytering via Emporos måste följa
  samma nya bindning genom dess fulla livscykel; den delen är ännu inte djupinventerad.

`transport.FindFreeShip` kräver cargo_unit_id IS NULL och garrison: skepp med
ombordvarande landenhet väljs alltså inte till godsfrakt idag. Detta ändras inte.

Rekommendation till Claude: dela i A `codex/flervaror-last` (varumanifest,
20/120 och antalbaserad kapacitet över alla fraktvägar, fyra ytor) och B
`codex/emporos-tre-kohorter` (relationsmodell och hela trupplivscykeln).
Alternativet en gemensam gren ger två datainvarianter och betydligt bredare
regressionsyta. Inget nytt kanonbeslut om samtidig gods-/truppfrakt behövs för A.
Inväntar uttryckligt fortsättningsbesked enligt orderns STOPP; ingen merge/push/deploy.
