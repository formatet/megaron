# Död startbefolkning bort

Kontrakt: ta bort den oanvända joinStartingPopulation=5000 och dess inaktuella kommentar. Den enda kodträffen var definitionen; faktisk Nomadic Host/grundning och befolkning lämnas orörda. Scope join.go, inget annat. Spelarsanning: ingen ändring. Acceptans: inga kodreferenser kvar, befintliga Join/Rise/Host-prov gröna. Ingen ny testkod för en död konstant. Bevisplan: riktad Go mot färsk DB, vet och källsökning. Huvudgrind: väntar till efter; tar bort missvisande teknisk skuld från todo.

Resultat: färsk migration205, befintliga Join/Rise/SeedNomadicHost-prov gröna (handlers2.559s); vet grönt. Källsökning i server/web: inga referenser kvar. Ingen beteendeändring eller ny mutationstest är meningsfull för en redan oanvänd konstant. Ingen ändring av någon spelartext, startbefolkning eller grundningsväg.
