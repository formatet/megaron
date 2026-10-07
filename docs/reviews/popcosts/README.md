# Riv oanvänd populationskostnadskatalog

## Kontrakt före kod
Problem: economy håller en oanvänd kopia av rekryteringskatalogens trösklar.
Spelarsanning: befintliga kostnader och rekryteringsgrindar bevaras.
Invariant: province.UnitSpecs och faktiska API-/klientkostnader ändras inte.
Scope: economy/recompute.go katalog + dess enda test i labor_test.go, rapport.
Non-scope: andra döda symboler, rekryteringsbalans, ny katalog eller spelarflöde.
Acceptans: inga kodreferenser till borttagen symbol; full fresh Go/vet gröna.
Stopvillkor: produktionskonsument hittas; rapportera till Claude före ändring.
Bevis: symbol/test/kostnadstextkorsning, deadcode -test före/efter och fresh DB.
Kedjegrind: bevisar ägarskapet för rekryteringskostnader, ingen ny yta.

## Premiss och baslinje
Bas master d9d6d58c. rg symbol över server+web hittar endast definition,
kommentar och TestPopCosts_MirrorTrainingGo:s läsning/feltext. Inga andra tester.
Baslinje spegel och kvarvarande LaborRates-tests på NY PG16/mig160 gröna:
/tmp/megaron-popcosts-baseline.log.
Deadcode -test med Go1.27.1/x-tools0.49.0 körd: samma sju kända funktioner.
Verktyget rapporterar FUNKTIONER, inte den exporterade map-variabeln; grep
och testkorsning är därför nödvändiga. Log /tmp/megaron-popcosts-deadcode-before.log.

## Bevarade symboler/ytor
province.UnitSpecs.PopCost bevaras: faktisk afford-tröskel och JSON pop_cost
från province.go (laborPool >= spec.PopCost, samt UnitCatalogue).
Keryx cmd_recruit.go läser pop_cost ur API; webb war.js visar needs N+ pop.
Codex units.md beskriver faktisk cohort/crew-draft; ingen katalogreferens.
Population draft är annan regel än afford-tröskeln. Dessa ägare berörs inte.
LaborRates och alla dess testfunktioner bevaras: oberoende funktion/skydd.

Samtliga övriga deadcode-fynd lämnas med skäl: loadLaborCapacities (labor),
ProvinceHandler.Marches (annan handler), Client.patch och die (CLI),
rankedFoodSlotsAt (grundande), Worker.RegisterWithTimeout (worker-API),
templeTierMultiplier (kult). Orelaterade, egna premisskontroller krävs.

## Resultat
PENDING. Ingen BILD/TEXT/ny spelarresa för oanvänd katalog; riktiga
kostnadsytors befintliga tester körs i fullsviten. Ingen merge/push/deploy.

## Resume checkpoint
codex/riv-popcosts, /tmp/megaron-codex-popcosts-20261007 från d9d6d58c.
Recept: tools/gotest.sh; från server/: env -i HOME="$HOME" PATH="$PATH" go vet ./...;
/tmp/megaron-riv-marchrecall-tools/deadcode -test ./...; rg symbol över server web.
Claude: core-review, egen mutationskontroll/byte-identitetskontroll och integration.
