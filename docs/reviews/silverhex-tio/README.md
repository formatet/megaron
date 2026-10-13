# Silverhex tio

## Slice-kontrakt

| Fält | Innehåll |
|---|---|
| Problem | Fyra fyndigheter på64×64 ger för få oberoende silverlägen. Timothy har valt fler. |
| Spelarsanning | Nya64×64-världar siktar på tio fyndigheter; ingen spelare lovas silver nära sin start. |
| Invariant | Fem platser, L1-takt1/2,5, spacing5, kandidatfilter, spawnordning och befintliga världars kartdata bevaras. |
| Scope | Silvermålets ytskalning i world/mapgen.go; regressionsprov och bevis; silvergeografins vaultbeslut. |
| Non-scope | Spjutsold/B, startpengar, terrängkonvertering, täckningsgaranti, backfill/migration och nya spelarytor. |
| Acceptans | Samma20frön ger exakt10hex på64×64; effektivseed/övrig geografi bevaras; validateMap passerar; mutationtill4 blirröd; fullfärskGo+vet grön. |
| Stopvillkor | Ny terräng/spawn/spacing behövs för målet eller ankartal måste ändras. |
| Bevisplan | Baslinje före kod; kodens verkligaGenerateMap jämförs med master i ett reproducerbart verktyg; test+mutation+freshDB-svit. |

Blockerar/geografiunderlag till kedjegrinden, inte bevis för naturlig gruvdrift/handel/brons/elit.

## Godkänd ytskalning

`max(4, round(10 × bredd × höjd / 4096))`.

10 är Timothys ankare på64×64. Claude godkände formeln2026-10-13 06:53 och betonade golvet4.
Direkt ytskalning följer ordern även när playersFor har sin miniminivå10.
Det befintliga silvergolvet4 behålls för mindre kartor. Exempel:32×32→4,56×40→5,60×60→9,64×64→10,
128×128→40,230×230→129. Dessa är mål, inte löften: befintlig girig placering kan mättas under spacing5.
Ingen universell exaktantal-/täckningsvalidator införs; exakt10 verifieras på de beställda20fröna.

Underlaget är `codex/silverutredning` @ea19f188. Implementation `8927a027`, från master `97712ede`;
Claude granskar/integrerar. Ingen merge, push eller deploy gjord här, ingen migration.

## Baslinje och experiment

Före implementation: två befintliga verkliga kartprov gröna på oförändrad master, separat tomPG16,
mig177 (`baseline.log`, world24,613s). Kontrollkartorna i jämförelsen byggs från `git show 97712ede`
i en egen tillfällig Go-modul; kandidatkoden byggs i en annan. Båda får samma20begärda seeds1009×1..20.
GenerateMap kör sin ordinarie validator och eventuell reseeding, utan ändrad kandidatlista/spacing.

| Mätning | Master | Kandidat |
|---|---:|---:|
| Silverhex på varje64×64-karta | 4 | 10 |
| Distinkta effektiva frön | 20 | 20 |
| Samma effektiva frö per begärt frö | — | 20/20 |
| Identiska MapTile-fält utom SilverDeposit | — | 20/20 |
| Medel fullbemannat silver/tick L1, femplatser | 27,875 | 66,125 |
| Medel fullbemannat silver/tick L3, femplatser | 50,831 | 120,581 |

Kontrollen hashar **hela MapTile** efter att enbart SilverDeposit satts false: koordinater, UUID,
terräng, kustegenskap, fertilitet, mineral och koppar/tenn/ceder bevaras. Detta är starkare än enbart
terräng-/koppar-/tenn-jämförelse. Silverkoordinater och counts sparas också.
[comparison.json](comparison.json), [baseline.jsonl](baseline.jsonl), [candidate.jsonl](candidate.jsonl).
Kapaciteterna kräver alla fyndigheters verkliga bemanning, gruvor och mat; de är inte realiserad produktion.

## Grindar

| Grind | Bevis |
|---|---|
| Kod | Namngivna test för20verkliga kartor och ytskalning; validateMap och spacing5 kontrolleras. Fysisk referens10→4 ger `TestGenerateMapTenSilverSourcesOn64x64` rött4≠10; exaktbyteåterställning och färsk grön körning. FullfärskGo/tomPG16/mig177 på8927a027 exit0 (world89,180s), vet exit0. |
| Visuell | Ingen renderer, asset, layout eller ny bildfamilj ändrad. Ingen BILD-grind tillförs. |
| Semantisk | Inget löfte om täckning eller universellt exaktantal. Gammal catchment-potentialtest behålls, dess kommentar rättas. Fem arbetsplatser och takter skyddas även av befintligt `TestSilverCalibrationMineAnchorsAndMaxLevel` i fullsviten. |
| Användare | Spelaren möter samma befintliga silver_deposit i karta/grundningsprognos/CLI; inga verb eller DTO-fält ändras. Ingen naturlig gruv-/handelskedja påstås genomspelad här. |
| Drift | Endast ny generering påverkas. Alla produktionsanrop är nyvärldsskapande: `cmd/server/main.go:838–865` returnerar befintlig värld före GenerateMap; `api/handlers/world.go:222–230` skapar ny värld; `cmd/create-world/main.go:61–73` skapar uttryckligen en ny reseedad värld. Mapgen-debug är ett fristående verktyg. Ingen befintlig värld uppdateras av slicen. |

Fyra ytor: temenos genererar nya kartdata; keryx/webb läser samma befintliga fyndighetsfält; Codex
`web/static/codex/silver.md` nämner inget kartantal eller närhetslöfte och lämnas därför orörd.
Inget verb förändras och ingen ny rad behövs i verbregistret. Vaultens silvergeografi och dess
beslutsregister uppdateras med Timothys reviderade beslut; gamla2026-08-28-rapporten är historik.

## Reproduktion

```sh
python3 docs/reviews/silverhex-tio/compare.py
python3 docs/reviews/silverhex-tio/mutate.py
tools/gotest.sh
# från server/ med ren miljö:
env -i HOME="$HOME" PATH="$PATH" go vet ./...
```

[targeted.log](targeted.log), [mutation.log](mutation.log), [mutation-four-red.log](mutation-four-red.log),
[mutation-restored-green.log](mutation-restored-green.log), [full-go.log](full-go.log), [vet.log](vet.log).
Första manuella testanropet kördes från reporoten där go.mod saknas; det var ett arbetskatalogfel,
korrigerat till server/ innan grönt testresultat. Jämförelseverktyget var redan grönt och koden ändrades inte.

## Metodisk lärdom och avgränsning

Ett måltal är inte en garanti: oförändrad girig placering kan mättas, särskilt på större kartor.
20utvalda kartor med exakt10 är ett konkret regressionsprov, inte ett kontrakt för alla frön.
Ingen närhets-/rättvise-/framkomlighetsvalidator, ny terräng, ny gruvkapacitet, spjutsold eller
startbuffert byggs. Claude gav köbesked07:00: B→marsch-soldrad→komplettSilverAudit, med fasta
oförändrade startbelopp/Rise60/kolonipung i B; B börjar först efter Claudes besked att denna slice
är mergad. Ingen kod för den kön finns här. Analysens audit-/projectionsfynd är utanför scope.

**STOPP. Fullsvit och vet är gröna.**
