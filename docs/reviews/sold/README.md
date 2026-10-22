# ORDER 16 — kodcheckpoint, INTE slutlig handover

Status: kod e9b01eca och Keryx-precision04c8fcf2 på codex/sold, bas ea97b332.
Arbetet stannar efter ORDER14 enligt Claudes reviewinstruktion19:35.
Ingen merge/push/deploy eller liveskrivning. Full färsk Go-svit är INTE körd för
ORDER16; detta är uttryckligen ingen fullständigt verifierad slutleverans.

## Genomfört

- Economy UnitUpkeep har grundlön1silver/100personer/tick × typfaktor, size för
  land och faktisk Crew för skepp. Homehalva, spearman0, elite1.5/chariot2,
  båtfaktor1 och laid_up0. Mat och lön delar operational statusägare;
  ORDER12s mat och VoyageRation är numeriskt oförändrade.
- UpkeepSoldShare/SoldCirculation/env och handlerknopp borttagna. Debiteringen
  tar hela nya lönen utan kredit. Auditackumulatorn för cirkulation är borta,
  historiska payloadfält finns kvar med0/{}; gross=destroyed, unpaid separat.
- Payer och replay/desertionregler är oförändrade. Marschprognosen SELECT/Scan:ar
  faktisk Crew och räknar nuvarande respektive fieldstatus separat, inklusive
  cargo exakt en gång. Storage/recruit/province/settlement konsumerar ägaren.
- Keryx återflödestext borttagen; 0.05 och0.005 bevaras i kostnad/bristvarningar.
  Codex upkeep/units/sea samstämmiga. Ingen produktionswebb ändrad.
  Klientbevis: ../sold-arbetarlon/client-surfaces.md.

## Sparade bevis och begränsningar

baseline-red.log: TestSoldierWagesUseActualMenAndHomeHalf rött mot gammal
produktion. pure-final.log: nya siffror och ORDER12-ransoner gröna.

targeted-payroll-final.log: färsk198 combat/API payrollurval GREEN, inklusive
syntetiska riktiga DB-fall elite/chariot/war_galley, damaged crews, home i annan
stad än payer, field, otillräcklig kassa, återspelningsskydd och legacy-audit0/{}.
Retired env satt till1 i DB-testet kan inte återinföra kredit. Denna delkörning
ligger före den slutliga delade statushelperns refaktor och CLI-precisionen;
slutlig bred verifiering återstår därför även om dessa lägen är oförändrade.

Fysisk mutant i separat e9b01eca-checkout: homefaktor.5→.7 ger både namngivna
prisprovet och verkliga debiteringsproven RED. Byteexakt SHA-restore och ny
färsk198 GREEN; mutation-*.log/txt. Tempworktreen är borttagen.

715JS/vet var GREEN före sista Keryx-precisionen. Agentens riktade Keryxprov
inklusive små belopp är gröna efter precisionen. Kör slutligt vet/fullsuite efter
återupptagning; inga färska helsvitpåståenden görs här.

Tidiga röda utvecklingskörningar finns kvar i targeted-packages.log och
targeted-payroll.log: gamla lönetal/roster/audit/fixturer behövde uppdateras,
en testhjälpare seedGoods krävde int och ändrades till separat SQL för0.74kassa.
Desertion-cascade-fixturen hade crew0 men krävde obetald lön; den har nu1roddare,
positiv lön och oförändrat cascade-/tröskeltest. Produktionsdesertering72/10orörd.

## Fortsättning

1. Läs aktuell Claude-order och rebasera när13/14/17 mergats (Claude säger till).
2. Kontrollera hela economy/combat/API och Keryx på frusen slutkod; full färsk
   tools/gotest.sh EN gång på57900 efter eventuella relevanta rättningar, JS/vet.
3. Nytt proof package och hash+STOPP till Claude. Ingen BILD behövs för nuvarande
   diff (webbmarkup/JS/CSS orörda); eventuell ny webbändring måste bedömas separat.

Inventering/liveföre-efter: inventory-before-code.md och comparison.md/json.
READ ONLY17:17UTC/mig198: Pylos galley.45→.1, merchant.3→.05; total.75→.15silver/tick.
Detta är behov ombetalt, inte genomförd debit/likviditetsgaranti. Snapshot är historisk.
