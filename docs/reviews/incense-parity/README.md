# Rökelsens paritet

Kontrakt: rökelse som redan beslutad fyndighetsvara ska kunna hittas av den nuvarande Potnia-visionen och visas i Keryx kända stadsfynd. Server/API/webb bär redan incense_deposit i städer; Keryx ignorerar fältet. Nuvarande oracle_memory_v2 väljer bara koppar/tenn/silver. Bygglistan i megaron_rokelse_plan_20261019 pekar uttryckligen på oraklets fyndighetsval; todo anger kvarvarande hålet.

Invariant: vision ger bara nytt minne, aldrig liveögon/aktivitet; redan kända fynd väljs inte. CLI läser bara kända fält som API:t gett; rykten får inga exakta fynd eller positioner. Scope: nuvarande Potnia-effekt, dess presentationstext, Keryx cities, Codex och relevanta prov. Äldre orakeleffekter, eventsemantik, offerrecept, smakvikter och kartseeding ändras inte. Stoppvillkor: behov av ny FOW-/offermekanik. Bevisplan: rökelse-enda-okända-fynd ger gammalkodsRED via verklig rite-handler; CLI HTTP-konsumentRED; gröna efter fix, fysisk kandidaturmutation, fresh full en gång och riktad FOW/klientverifiering.

## Riktade resultat och faktisk användning

Gammal kod: TestPotniaVisionFindsUnknownIncenseAsMemory/unknown_incense ger HTTP200 men ingen funnen källa; TestCitiesListsAllKnownDepositsIncludingIncense tappar både cedar och incense (baseline-api.log/baseline-cli.log). Ny kod: båda gröna, inklusive redan känt rökelsefynd som inte får väljas. Befintliga TestAlphaVision-prov gröna på färsk migration205.

Fysiska källmutanter: rökelse bort ur nuvarande oraklets SQL-kandidatur respektive fel fältnamn i Keryx. Namngivna prov röda, byte-exakt återställning i finally; återställd API/FOW-körning grön. Recept, kostnader, källantal, smakvikter och äldre orakeleffekter oförändrade. Legacy-wirefältet revealed_ore.ore behålls och kan nu innehålla incense; ny text säger source of incense, inte ore vein.

Två nya helt isolerade PostgreSQL16/Redis-världar: normal registrering/anslutning/grundning, därefter explicit tempel-/stock-/fyndfixtur före handlingarna. Bara det nya provets DB ändras. Ett känt eget rökelsefynd och ett avlägset okänt rökelsefynd, inga andra malmkandidater, isolerar filtret. Detta är inte naturlig upptäckt eller tempelbyggnad.

Faktisk nybyggd Keryx cities visar incense; rite --list beskriver källvalet. Native Firefox Offer→Bring the offering returnerar rätt avlägsna incense-koordinat. Kart-API före=fog, efter=remembered med incense_deposit=true, aldrig live. Både1280/390 utan sidledsöverfyllnad eller pageerrors. run1/proof.json och run2/proof.json innehåller processhash58cebec+incense/migration205 och käll-diffhash. Egna processer och containrar städade i finally. Run1:s bilder råkade visa listans nederdel; run2:s instrumentering fokuserar den ändrade Vision-texten, inspekterad1:1. Semantik-/handlingsproven var gröna i båda körningarna.

739 JS-prov och go vet ./... gröna. Full Go-svit körd EN gång mot färsk migration205:26 paket gröna, world155.083s. Inga nya notistyper eller UI-handlingar. Webben och rite --list läser den gemensamma serverbeskrivningen; Keryx cities förbrukar redan exponerade stadsfält. Codex rites/incense uppdaterade.

## Levererat

6e690df5 mergad/pushad och verifierad som NY process6e690df via healthz. Migration205 dirty=false, samma aktiva värld2178e795;4 rökelsehexar avlästa som grundtillstånd (inte en ny regel eller seedändring). Codex/assets byteexakta, hash i deploy.json. Ingen ERROR/fatal/panic i den granskade journalperioden. Installerad Keryx anger vcs.revision6e690df50e5b05be16ea2e292c48a8af9111a85c och vcs.modified=false. Ingen migration, reseed eller manuell restart gjord.
