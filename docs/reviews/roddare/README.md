# ORDER 12 — roddare äter som soldater

Faktisk kvarvarande besättning äter nu spearman-ranson per person på alla tre
båttyper. Hemma = garrison/repairing/fitting_out; ute = marching/positioned/
freighting, även mellan fraktben. Laid up är fortsatt utan mat/sold. Silver,
landranson, varulager, resvägar och befintliga event ändras inte.

Inventering före kod: `inventory.md`, commit b61378c0; Claude godkände samtliga
punkter 2026-10-22 17:51. Implementation: **a6917953**, bas5262aa6e.
Ingen merge/push/deploy. FUNKTION; ingen ny UI/BILD. TEXT i playtest.

## En källa genom hela resan

`economy.UnitUpkeep` härleder mat ur `UpkeepSpecs["spearman"].Grain / 100`
× faktisk Crew, och FieldGrainFactor ute. Inget kopierat matransonstal.
`UnitUpkeep` och `VoyageRation` kräver Crew som nytt obligatoriskt argument;
naval size=1 förväxlas inte med besättningen. Befintliga rader skickar Crew,
nyrekryteringsprognoser använder unit.CrewFor. Inget economy→unit-beroende.

| Full besättning | Hemma, mat/tick | Ute, mat/tick | Silver/tick |
|---|---:|---:|---:|
| Galley, 20 | 0,10 | 0,20 | 1,5 |
| War Galley, 50 | 0,25 | 0,50 | 2,5 |
| Emporos, 10 | 0,05 | 0,10 | 1 |

Två kvarvarande roddare äter 0,01 hemma eller 0,02 ute, oavsett båttyp.
Befintlig landlast tillkommer med sin egen fältranson en gång.

Samma Crew går genom marschpreview/avfärd, fraktens prepareJourney (Dispatch
+ CreateShadow), stående rutters reserv, tickdragning, stadens aggregat/
lagringsprognoser, rekryteringskapacitet samt unit-API:ts mat och proviantdagar.
Döda standingOrderNavalRation och dess gamla tabellberoende är borta; ruttens
befintliga integrationstest använder VoyageRation. Silverprognosens anrop
använder endast Silver, som fortfarande är oberoende av Crew.

Keryx och webb fortsätter läsa serverns tal; båda har verifierade uppdaterade
visningsfixturer. Spelar-Codex upkeep/sea/units beskriver samma regel.
Vault temenos_enheter uppdaterad, den konkurrerande ”ej beslutad”-paragrafen
borttagen. Freighting-proviantbadge är avsiktligt utanför denna slice enligt
Claudes beslut; ingen produktions-JS eller layout ändrad.

## Bevis

- `baseline-red.log`: namngivet TestRowersEatSoldierRations **RED på gammal
  produktion**: 20 rowers eat0.04, want soldier ration0.10.
- Nya ägarprov: alla båttyper, full/två/noll roddare, hemma/ute/laid_up, identisk
  silver; ändrad spearman-rad slår igenom härlett; last och VoyageRation stämmer.
- Nytt riktigt tickprov, alla båttyper × sex aktiva statusar: två roddare drar
  exakt0,01/0,02 ur fisk ombord; provianttotal=varulager; samma tick omkörd
  debiterar inte igen. Freighting i hamn behåller fältranson.
- Nytt ListUnits-API-prov: alla tre båttyper med Crew2 och0,05 fisk visar
  mat0,02/tick och två hela proviantdagar. Aggregat-/kapacitetsproven läser
  besättning från DB; inga fullcrew-fallbacks för befintliga rader.
- Fraktens fyra utgående typer lastar nu exakt0,20 för fixturens faktiska
  tio roddare, med faktisk hamndebitering, en ombordmåltid och retur av samma
  fisk exakt en gång. Befintliga brist-/retur-/stående-ruttprov kvar.
- `mutation.log`: fysisk mutant ersätter faktisk Crew med fasta20, ger
  namngivna **RED**; byte-exakt återställning ger **GREEN**. Kördes i egen
  tillfällig checkout av a6917953, isolerad från fullsvitens worktree.
- `targeted-final.log`: economy/combat/transport/handlers GREEN på ny tom
  PostgreSQL/migration198, port57900. Första riktade körningens röda var gamla
  fixtures som saknade crew eller antog skrovranson (`targeted.log`); rättade
  till uttrycklig faktisk crew och nya förväntningar före slutkörningen.
- `full-go.log`: full fresh Go **GREEN, en gång** (world152,997s), `tools/gotest.sh`, port57900,
  ny tom DB/migration198, ren miljö, `go test -count=1 -p 1 ./...`.
- `js.log`: **713/713 GREEN**. `vet.log`: `go vet ./...` GREEN.

Ingen naturlig live-resa beställd: drift lästes bara. De funktionella
reselivscykelproven kördes mot isolerad test-DB. Fullsvitens loggrad anger
exakt testad implementation; befintlig liveprocess är inte denna branch.

## Före deployment — Claude kör en ny read-only kontroll

**Ingen backfill, gratismat, lager-multiplikation eller automatisk recall.**
Gamla redan lastade långa resor kan annars få mat för kortare tid när ransonen
höjs. Den godkända övergången är att vänta till fysisk hamn, eller fatta ett
uttryckligt separat övergångsbeslut om kontrollen visar gamla positiva laddningar.
Ingen sparad reseversion eller schemaändring införs här.

Kör den bifogade SQL-filen **direkt före deploy**, inte en gammal sparad lista:

```sh
ssh root@10.0.1.92 'runuser -u postgres -- psql -X -v ON_ERROR_STOP=1 -d poleia' < docs/reviews/roddare/predeploy-readonly.sql
```

Filen använder `BEGIN READ ONLY`. Den väljer alla världars marching/positioned/
freighting med proviant >1e-9 och visar crew, varulager, mål/tider/uppdrag och
aktiva fraktben. Små flyttalsrester behandlas inte som finansierade måltider.
Freighting i hamn finns med konservativt: granska ben/hamnmöjlighet innan beslut.

Min läsning **2026-10-22 15:59:57 UTC gav noll rader**
(`predeploy-observation.json`). Den är ett tidsstämplat fynd, inte ett löfte om
läget när Claude deployar. Inventeringens tidigare passage_wait-Galley var
redan praktiskt tom även med gamla ransonen; denna slice skapar inte mat åt den.

Reproduktion: fullsvit `GOTEST_PORT=57900 tools/gotest.sh`, JS med
`node --test $(rg --files web/static/js -g '*.test.mjs')`, vet från server/ i ren
miljö. Kontrollsummor för slutligt bevispaket: `SHA256SUMS`.
