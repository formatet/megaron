# Inlandsstadens mat — slice 6

Status: klar för Claudegranskning på `codex/inland-mat`, kärna `15641f70`, bas `8f3d778e`. Ingen integration/deploy av Codex.

**Verifiering:** full FÄRSK Go på migration182 exit0 (world150,304s), `go vet ./...` ren,649JS/0skip, två fysiska namngivna mutations-RED→exakta bytes→fresh GREEN, katalogupgrade/rollback och naturlig CLI/Firefox-rigg gröna. Healthz i riggen: `15641f70a157dec3ef0da5e3a38b65f8a5d50c10`/182.

## Kontrakt (före kod)

| Fält | Innehåll |
|---|---|
| Problem | Tylissos har sin gård men kullmarkens korn föder inte 1000 människor trots nio matgubbar. Blockerar kedjegrinden genom att låsa arbetskraften före timmer/sten. |
| Spelarsanning | Ny inlandsstad på vanlig kornmark föder tio gubbar med högst sju på mat; minst tre kan arbeta med byggmaterial. |
| Invariant | Samma terräng-/byggnadsregel, FoodTick och matprognos för alla städer; fisk är mat. Förändrat r0 gäller även befintliga städers härledda rater efter omräkning. |
| Scope | En matbalansspak, eventuell katalogmigration, DB-regression, prognos/klientparitet och Codex, 60-tickexperiment med riktiga ekonomihandlers. |
| Non-scope | Nya mekaniker, silverprissättning, konsumtion, tillväxtformel, fisktakt, byggnadsregel eller särskilda Tylissos-undantag. |
| Acceptans | ≤7/10 på mat och FoodNet≥0 på Tylissoslik kullmark samt slätt; prognos=verklig grundning; 60 tick inland/kust före/efter och Amnissos; full färsk Go/vet/JS; fysisk mutation ger namngivet RED och exakt återställning ger fresh GREEN. |
| Stopp | Om båda tillåtna spakarna kräver ny mekanik eller inte klarar målet: rapportera konkret konflikt till Claude/Timothy. Balansändringen är uttryckligen beställd. |
| Bevis | Läs live enbart read-only; isolerade SQL-fixturer mäter motor och får inte beskrivas som naturligt spelarflöde. Separat naturlig register/join/grundning via CLI och webb på egen server, healthz-proveniens. |

## Baslinje

Read-only live: Tylissos (58,53), population1000, farmL1 på hills(58,54). Ringen har nio hills, resten skog/kalkberg. Katalogens grain r0: hills≈0.3333333333, plains≈1, river_valley≈1.6666666667, river_delta≈2.6666666667; fish coastal_sea=1, river=0.5. FarmL1 multiplicerar med1.7 och ger hills fem platser. Silver/grain/fish labor_value=1; inga NPC-priser.

## Resultat och experiment

Claude godkände 2026-10-17 17:10 kullmarkens **grain r0 ⅓ → 0,5**. Produktionsändringen är en enda katalograd i migration182. Kärna `15641f70a157dec3ef0da5e3a38b65f8a5d50c10`.

| Hypotes | Utfall | Beslut |
|---|---|---|
| Gåvan saknas | Live har farmL1 på hills(58,54). | Ingen gåvolagning behövs. |
| GåvofarmL2 | Beräknat med nuvarande regel: fem farmgubbar + två andra kullar + närjord =5,167 grain. Endast nya städer förbättras. Alternativet är inte 60-ticktestat. | Avstå: befintliga Tylissos behöver också hjälpen. |
| Höj bara hills grain-r0 till0,5 | Fem farmgubbar ger4,25; en annan kulle0,5; närjord0,5. **Sex matgubbar ger5,25 mot behov5: netto+0,25.** | Vald/godkänd spak. |

Ordinarie L1-gåva, samma per-hexgränser och ×1,7/×2,4/×3,1-byggnadsregel gäller. Ingen konsumtions-/tillväxtändring eller särregel för en namngiven stad. Målet gäller faktisk kornmark med tillräckliga platser, inte ett sterilt omland eller en ensam isolerad grainkulle.

### 60 tick i verkliga ekonomihandlers

Var sin **ny PG16/migration181 respektive182**, ren miljö via `tools/gotest.sh`. `baseline-60.log`/`after-60.log`, fulla tioticksserier i `sixty-tick.json`. Alla börjar med1000 invånare/1200korn, L1-farm endast inland, ingen farm för kust/Amnissos. Startplacering är motorns egen greedy; därefter upp till två lediga gubbar på timmer och en på sten, om platser finns. Nya hundratal auto-placeras av den riktiga tillväxtmotorn. Kör Sitos→FoodTick→kharis tillväxt/omräkning i normal ordning, inte en separat kalkylmodell. Förrådsförfall och Sitos syns i slutsaldon.

| Omland | Matgubbar tick0 före→efter | Population tick60 före→efter | FoodNet tick60 före→efter | Korn+Sitos tick60 före→efter | Timmer/sten tick60 före→efter |
|---|---:|---:|---:|---:|---:|
| Tylissos kullmarksring | 10→6 | 1000→1331 | ≈0→+1,5985 | 790,707→914,712 | 0/0→89,663/60 |
| Slättkontroll | 3→3 | 1331→1331 | +7,672→+7,672 | 1139,062→1139,062 | 89,663/30→89,663/30 |
| Kustkontroll | 5→5 | 1371→1371 | +4,485→+4,485 | 677,921→677,921 | 89,663/30→89,663/30 |
| Amnissos geometri | 3→3 | 1331→1331 | +8,2883→+8,495 | 1186,864→1200,577 | 0/30→0/30 |

Ingen otillfredsställd måltid under något förlopp. Före ändringen producerar nio Tylissos-matgubbar4,667, i linje med spelrapporten; greedy fyller även den tionde och når5 minus flyttalsdamm, med noll byggare och ingen tillväxt. Efter ändringen växer inlandet33,1%, som slätten; kustens37,1% är samma före/efter (befintlig matvariationstillväxt). Tillväxten förbrukar ingen extra mat enligt befintligt beslut. Amnissos blir inte långsammare; dess kullars remainder-bidrag ökar lite. Timmer saknas alltjämt i Amnissos geometri, därför0timmer.

Detta är **kontrollerade DB-fixturer**, inte60 tick av Timothys livevärld. Tylissos/Amnissos har samma terrängring som read-only live, men alla kontroller börjar vid1000 invånare, inte Amnissos aktuella1391. Ingen armé/upkeep, byggkö, handel, kult eller spelpolicy simuleras; måttet isolerar den beställda mat-/arbetskraftsbalansen. Separat naturlig rigg nedan provar spelarens ingång och vanliga världstickar.

### Befintliga städer och silver

`TestInlandFoodBalance_ExistingCityRecompute` håller placeringarna fasta i fyra miljöer: gamla raten tjänas in över10tick, katalogen ändras, sedan RecomputeProduction. Upplupet förråd bevaras exakt inom1e−8; grain sjunker inte och fisk/timmer/sten påverkas inte. Testet visar också varför en katalogmigration ensam inte uppdaterar en tyst stads lagrade rate.

`live-delta.sql` är **enbart SELECT**, körd read-only live. Fyra ägda städer med grainkullar, fyra positiva marginaler, **noll negativa**, sammanlagt+2,716667grain/tick vid samma population/placeringar inklusive remainder-termen. Övriga städer får noll delta. Den gemensamma formeln är monoton i r0 för varje byggnadsnivå och arbetsstyrka; inga andra rater eller konsumtionsled ändras. Detta är balanseffekt vid fast tillstånd, inte ett påstående om framtida saldo efter spelorder/tillväxt eller en redan inaktuell projektion.

`catalogue.py` provar181→182→181 på ny PG16: **exakt en ändrad raterad**, alla övriga regler och hela goods-tabellen inklusive labor_value är identiska; rollback återställer raten (float-tolerans1e−14). Korn/fisk/silver har fortfarande labor_value1. Det finns inget automatiskt silverpris eller NPC-köp. Mot oförändrad L1-silvergruva1silver/gubbe/tick minskar kullkornets rena arbetstillfälle från3 till2silver per korn utan farm, och från≈1,765 till≈1,176 med L1-farm (−⅓); det är en jämförelse av produktion, **inte ett marknadspris**. Wanaxer väljer fortfarande priser och gudarnas standardvärdering ändras inte. Se `megaron_silverkalibrering_20261012`.

## Grindar och reproduktion

| Grind | Bevis |
|---|---|
| Kod | `TestInlandFoodBalance_SevenOfTen` + faktisk `TestCreateMetropolis_InlandFoodSevenOfTen`; forecast=lagrad grain/fish. `mutation.py`: fysiskt byt migrationen till⅓, båda namngivna assertioner RED; återställ exakt SHA2561591d22538595a49cdfac0729d3500577b767d9426f5cafdd31ddfaadfa67f91→fresh GREEN. Full-Go/vet/JS loggar i denna katalog. |
| Semantik/fyra ytor | Temenos r0 läses av ordinarie prognos/grundning/placements/recompute; webb och keryx visar serverns nya tal utan egna balanstanter. Keryx `city` visar hills `next +0.5 per day`; `founding.md` beskriver den nya kullekonomin. Inget API-kontrakt eller verb ändrat. |
| Användare | `native.py`, egen PG16+Redis+server, normal register→join→CLI preview→grundning→city/goods/status/notifications, Firefox1280+390 och Sitos via vanliga tickar. Ingen SQL-skrivning/response-override. `native/evidence.json`: **healthz commit15641f70…/migration182**, page_errors[]. Inland/grain+fiskprognos och kust matchar spelarutskrifterna. Riggvalet föll på ett inland med bättre spannmålsjord; den rena kullmarkens budget bevisas separat av DB-regressionerna. |
| Visuell | Befintliga UI-ytor inspekterade1:1 i Firefox på1280 och390; ingen ny grafik/BILD-förändring. TEXT får fortsatt möta speltest. |
| Drift | Överlämnas till Claude; ingen merge/push/deploy av Codex. Se nedan. |

Kör från worktree-roten:

```sh
python3 docs/reviews/inland-mat/mutation.py
python3 docs/reviews/inland-mat/catalogue.py
python3 docs/reviews/inland-mat/native.py /tmp/megaron-inland-native-new
# GOTEST_PORT ska vara en fri port. Harness kör med ren miljö.
tools/gotest.sh
```

Nativebilderna/API-/CLI-texterna är kopierade selektivt; privata configar/token/binaries ligger inte i repot. Apparaten hade först fel DueTick-typ (kompilationsfel) respektive pg_isready mot temporär unix-startserver (anslutningsfel); båda rättades innan giltiga bevis. Inget sådant räknas som mutations-RED.

### Deploy för integratören

Migration182 ändrar endast SQL i produktion: dra den integrerade hashen och **restarta tjänsten** enligt CLAUDE.md när ingen migration är i gång. Verifiera rätt process/hash/healthz och182/dirty=false. Kör sedan befintliga **`go run ./cmd/recompute-all`** med korrekt drift-DSN så ägda städers gamla intjänade förråd först avräknas och raten skrivs om; kontrollera att verktyget rapporterar0failed. Läs Tylissos verkliga grainrate före/efter och andra städers rater. På rollback: down181 och samma omräkning.

Omräkningen flyttar inte giltiga befintliga placeringar: Tylissos redan bundna matgubbar frigörs genom vanliga webb/keryx-place. Efter omplacering till fem på gården+en annan kulle räcker sex; fyra kan arbeta med annat. Nya städer får sex automatiskt via befintlig greedy. Ingen retroaktiv gåva eller automatisk policy läggs till.

## Kända avgränsningar och lärdom

Målet är nått på ordinär kornmark med minst en annan grainplats utöver gårdens fem. Barren terräng och omfördelning av redan placerade arbetare är fortfarande spelarbeslut. Silvertalen ovan är alternativkostnad; riktiga handelspriser och långsiktig makroekonomi mäts i fortsatt playtest. Mät det faktiska terräng-r0:t: slätten var redan stark nog och skulle ha dolt Tylissos rot.
