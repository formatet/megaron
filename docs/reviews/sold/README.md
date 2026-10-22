# ORDER 16 — sold som arbetarlön, halv hemma

## Sammanfattning

UnitUpkeep äger lönen för faktisk styrka/besättning. Hemmalönen är halva
fältlönen; inget av lönen krediteras tillbaka. Kod e9b01eca, precisionsfix04c8fcf2,
teststädning ffec0f0b och sista äldre prisassert fcc21e5a. Gren codex/sold, bas ea97b332, inga nya migrationer.
Slutlig full fresh198Go GREEN (world147.339s) efter rättning av en kvarvarande gammal testförväntning;715JS och slutligt vet är gröna.
Ingen merge/push/deploy eller liveskrivning av Codex.

## Kontrakt och beteende

FieldPayPerHundred=1silver per100personer och tick; faktor spearman0,
elite_infantry1.5,war_chariot2,galley/war_galley/merchantman1. Land skalar med
size/100, skepp med faktisk Crew/100; inga gamla silvervärden finns i ransonstabellen.
fieldUpkeep är gemensam statusägare för mat och lön: hemma garrison/repairing/
fitting_out, ute marching/positioned/freighting för skepp samt embarked för land.
Laid_up har ingen besättning i tjänst och kostar0. Inaktiva rosterposter visar
hemkostnad när de blir aktiva; billing-setet inkluderar inte forming/laid_up.

Mat/VoyageRation och full nyrekryterings CrewFor är numeriskt oförändrade från
ORDER12. Ingen ändrad startkassa eller ersättning till lager. Payer, replay,
72obetalda ticks och desertion10man lämnas orörda.

## Delad läsning och audit

- UpkeepSoldShare/SoldCirculation/env och handlerfältet soldShare är borttagna.
  Tick tar hela NYA lönen i sin befintliga atomiska silverUPDATE.
- Auditackumulatorn för krediten är borttagen. Historiska payloadfält behålls:
  silver_circulated0,circulated_to{},silver_gross=silver_destroyed. Unpaid är
  separat och debiteras aldrig; gamla event tolkas inte om. SilverAuditV2s
  stock/mining/replayinstrument är oförändrade.
- Province/settlement/recruit/storage använder samma lön. Kompatibla API-nettofält
  är lika med gross; legacy army_upkeep_circulated_silver är0. upkeepNetPerDay
  subtraherar hela nya lönen och tar ingen cirkulationsparameter.
- unit_march_silver.go SELECT/Scan:ar faktisk Crew (tidigare0), räknar nuvarande
  status och fieldstatus separat och tar med embarkad cargo exakt en gång.
  Kassar hålls separata; runway är den kortaste av relevanta betalare.
- Keryx återflödestext borttagen,0.05/0.005 bevaras i kostnad och bristvarning.
  Webb Army/marsch läser servervärden; Codex upkeep/units/sea säger samma regel.
  Klientbevis: ../sold-arbetarlon/client-surfaces.md. Ingen produktionswebb ändrad.

## Bevis

| Grind | Resultat |
|---|---|
| Namngivet gammalkod-RED | TestSoldierWagesUseActualMenAndHomeHalf i baseline-red.log; alla gamla silvernivåer/statusavvikelser syns |
| Pure slutkontrakt | Samtliga typer, skadad size/Crew, nollcrew, hemma/ute/laid_up, food och Keryx-småbelopp i pure-final.log/pre-full-pure.log |
| Riktig DB-debitering | TestWagesChargeActualPeopleWithoutCirculation:elite/chariot/war_galley, skadade crews, home i annan stad än payer, field/freight/repair/fitting_out,0.74kassa mot0.75lön, legacy-audit0/{},gross=destroyed,unpaid separat,replay. Retired env satt till1 har ingen effekt |
| Marschprognos mot tick | TestMarchSilverForecastMatchesActualTick:aktuell/fältkostnad,faktiskcrew,cargo en gång,flera separata betalare och verklig silverdebit/runway |
| Övriga riktade integrationer | Fresh198 combat1.983s/API0.337s i targeted-payroll-final.log; auditV2 conservation, storageforecast, recruitment, roster, unpaid/desertion/cargo och idempotens |
| Fysisk egen mutation | Separat e9b01eca-checkout:homefaktor.5→.7 gav prisprov OCH verkliga debiteringsprov RED; byteexakt SHA-restore och ny fresh198 GREEN,mutation-*.log/txt. Produktionsägaren är identisk på ffec0f0b |
| JS/vet | Slutliga715JS GREEN i js-final.log; vet-final.log tom/grön |
| Full fresh Go | GREEN på frusen fcc21e5a efter äldre prisassert;57900,world147.339s,full-fresh.log |
| BILD/TEXT | Ingen produktionswebb-JS/CSS/markup ändrad. Ingen webBILD; CLI/Codex-texter TEXT i playtest |

Tidiga röda utvecklingskörningar är sparade i targeted-packages.log och
targeted-payroll.log:gamla lönetal/roster/audit och testhjälparens intstock behövde
uppdateras. Den gamla desertion-cascade-fixturen hade crew0 men förväntade
obetald lön; den har nu1roddare och fortfarande oförändrade cascade-/tröskelassertions.
Ransonstesterna verifierar mat separat från den nya lönens kontrakt, utan en
kvarlämnad oanvänd gammal skrovlönetabell. Inga produktionsdesertionsregler ändrade.

## Liveunderlag och integration

Inventeringen/comparison.md/json bygger på READ ONLY17:17UTC/mig198, healthz
ea97b33. Pylos galley0.45→0.1,merchantman0.3→0.05;total0.75→0.15silver/tick.
12spearman0,nomadic_host0,forminggalley ingen aktuell debitering. Detta är
behov ombetalt, inte historisk betalning eller likviditetsgaranti. Snapshot är
historisk; inga nya världssaldon påstås. capture.py/compare.py reproducerar från
sparad data/runtimeinställning; runtime-sold-share var default0.7 i gamla processen.

Claude mergade14 som6a467b3a medan denna branch testades. Denna gren är avsiktligt
fortfarande frånea97b332 enligt order. Rebase när Claude säger till;13/17delar
province.go och klientytor. ORDER13s199 ska bli201 eftersom live passerat200;
det är noterat men inga ORDER13-filer ändrades i denna slice. Ingen rebase,
merge eller deployment görs av Codex utan aktuell order.

Första hela färska körningen hittade endast TestSpearmanFieldRationAndElitePayRemain:
10silver efter ny fältlön1.5 lämnar8.5 men det äldre testet krävde8 (10−2).
fcc21e5a ändrar bara den testförväntningen; matassert97.8 och all produktionskod
är oförändrade. full-first-legacy-assert-red.log bevarar hela första röda körningen
(alla andra paket gröna,world154.031s). Den nya hela körningen är slutbeviset.
