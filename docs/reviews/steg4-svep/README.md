# Steg 4 — mätt arkitektursvep

Klar för Claudes steg 5. **Källan är `daf740ddd0efa051adc78c945b92d3c0c5d94d48`**, inklusive S och T1; gamla mätningen på `f586270d` är ersatt. Åtta instrumentarmar avslutade med exit 0, sekventiellt utan OOM. 19 kontroller gröna, inklusive två verkliga verktygsmutationer rött→grönt och planterad kopia hittad→borttagen. **Ingen produktionskod, spelregel, DB eller spelaryta ändrad. Ingen merge/push/deploy.**

Läs [åtta kandidater](classified/candidates.md), [klassificerade fynd](classified/findings.md) och [DB-baslinjen](classified/handler-db-baseline.md). Vault-ingång: `megaron_arkitektur_svep_20261008.md`. Claude avgör ägarkarta v1 och prioritering; detta underlag avgör ingen kanon.

## Slice-kontrakt

Problem: ägarslicarna saknar uppmätta kopior, churn och konsumentmängd. Spelarsanning: detta bevisar underlag till kedjegrinden geografi→brist→brons→elit; ingen spelarhandling förändras. Invariant: tokenlikhet, RTA-nåbarhet och numerisk likhet är inte bevis för samma spelregel. Scope: mätverktyg/rådata/klassificering här, egen vault-rapport och kirurgiska länkar. Non-scope: produktionsflyttar, deadcode-radering, nya balansvärden, steg 5. Acceptans: alla instrument lyckas, alla fynd har kartrad eller explicit ingen rad med skäl, 8 kandidater med separata mått, handler-baslinje, kända och planterade kontroller. Stopvillkor: mätfel/lucka får inte redovisas som noll. Bevisplan: låst källcommit, versioner/SHA, kommandon/exits, fil:rad, kontrollprov och verktygsmutationer.

## Källa och baslinje

`raw/provenance.json` låser källa, verktygs-SHA, 677 Go-filer (242 produktion), Go 1.27.1-X:nodwarf5 linux/amd64, inga build tags och mättid. Churn: **2026-08-09 20:47:53 +02:00 → 2026-10-08 20:47:53 +02:00**, exakt 60 dygn före basens committertid. Inga driftvariabler ärvda: collect.py allowlistar HOME/PATH/USER/LANG och sätter isolerade cache-/tmp-värden, GOMAXPROCS=2 och GOMEMLIMIT=1536MiB. Varje instrument körs ensamt. Rådata från den tidigare basen är borttagen, inte blandad med slutresultatet.

Det är en källkodsmätning; ingen server/acceptansvärld har startats. Därför gäller källcommit, inte en påhittad healthz/migrations-provenance. Collect vägrar ändrade eller oregistrerade server/web-filer.

## Resultat och klassificering

| Instrument | Råfynd | Bedömning / hem |
|---|---:|---|
| dupl Go, production t100 / t50 | 8 / 83 unika par | 83 t50-par individuellt granskade: 26 kandidatpar, 37 avsiktlig parallellitet, 20 brus. Överlappande par är inte regelantal. |
| dupl alla Go, t100 | 35 unika par | 27 test/test-tillägg till production t100; inga production/test-par vid denna tröskel. `go-clones.json` innehåller unionen 110 par. |
| jscpd JS, ≥100 tokens/5 lines, weak | 18 par, 102 filer | 15 test-mock bootstrap, 3 avsiktliga canvasmönster för olika terränger. Ingen regelägarbeställning från dessa JS-par. |
| deadcode produktion / med test | 40 / 9 funktioner | 33 produktionsfynd nåbara endast med test; 7 kvar ej nåbara från main-roots; 2 extra test-double-metoder. Lista, ingen raderingsorder. |
| SQL normaliserade statements | 105 grupper | Kommentar-/whitespace-normalisering; 69 överlappande långrad-fragment separat. 11 SQL-literals med axial ABS-formler kopplas till rad17. |
| SQL produktion↔test | 43 grupper | Seed/assertion/kontraktsfixture, inte extra produktionsregelägare. |
| AST oanvända parametrar | 34 produktion + 66 test | arrivedID finns med; HTTP-adapter-/mocksignatur är ofta avsiktlig. |
| Upprepade Go-literaler | 3150 lexikala grupper | Nycklar, tests och vanliga tal skiljs från styrkta regelkopior. CLI:s matkonstanter mot economy är ett konkret rad14-fynd. |
| AST DB-anrop i handlers | 457 / 35 filer | Filvis baslinje, inklusive nollfiler; största province.go 114. Metoder/receivers och 12 uteslutna r.URL.Query finns dokumenterade. |
| Git churn | exakt 60 dygn | Commitmängd och +/− per fil; kandidaterna deduplicerar commitmängden. Filchurn är inte funktionschurn. |

Varje fynd har `owner_rows` (1–19) eller `[]` = **ingen rad**, klass och skäl. Go-klonernas manuella omdömen är låsta till fil/rad-par i `clone-reviews.json`. SQL-routing är mönsterbaserad mot berörd representation/primitiv; den avgör inte transaktionsägarskap. Literalgrupper med samma tal är konservativt brus tills konkret semantik styrkts. Fulla listor i `classified/`, läsbar katalog i `findings.md`.

**Konkreta kontroller:** `capabilities/context.go:191` och `province/contacts.go:9` innehåller samma kontaktregel. dupl hittar den inte vid valda trösklar, men SQL-normalisering hittar ett två-sites-fynd. `economy/trade.go:535` har oanvänd `arrivedID`; deadcode analyserar funktionsnåbarhet, därför kompletteras det med AST-parametrar. Detta är redovisade instrumentgränser, inga nollfynd.

**Literalgräns:** `0.005` betyder både mat, tillväxt, svält och loggtolerans. Endast CLI:s `grainConsumptionPerCitizenPerTick` (`cmd_status.go:94`) motsvarar economy-konstanten (`recompute.go:290`). Livestock-värdet speglas också (`cmd_status.go:101`, `recompute.go:354`). CLI-testets egna konstanter binder inte denna spegel till servern. FoodNet/growth-pin är redan byggt och ska inte beställas igen. Tekniskt lagertak är också redan ägt/vaktat; de privatliteraler som rivits föreslås inte på nytt.

## Experiment och falsifiering

| Prövning | Resultat |
|---|---|
| Kör varje instrument ensamt med storleksgräns/ignores | Alla 8 exit 0; ingen OOM. jscpd max-size 250kb; node_modules/vendor/generated/min.js uteslutna. Största fil map.js 243813 bytes ingår. |
| Kopiera carrier.go till isolerad tempfil, dupl t100 | Kopian ger träff; fil borttagen, exakt samma före-/efter-utdata och ingen tempträff. Tempfiler utanför produktionssökområdet. |
| Ta bort SQL-kommentarnormalisering i faktisk negativ subprocess | visibleOrigins-kontrollen exit 1; återställd regel hittar båda sites. |
| Stäng av oanvänd-parameterregeln i tempkopia av Go-instrumentet | arrivedID-kontrollen exit 1; omuterad AST grön. |
| AST använd/unused/shadow/closure-fixture | Oanvänd och shadow identifieras; använd och closure capture utesluts. |
| Full klassnings- och DB-receivercoverage | 19 checks pass; inga okända receivers bland metoderna. |

[verification.json](classified/verification.json) bär kontroller, mutationernas exit och stderr; [verification.log](verification.log) är slutkörningen. Muterade verktyg lever enbart i TemporaryDirectory och rensas. `server/` och `web/` har noll diff mot basen.

## Reproduktion

Kör från denna worktree. Versioner: dupl `github.com/mibk/dupl@v1.1.0`, deadcode `golang.org/x/tools/cmd/deadcode@v0.51.0`, jscpd `5.4.0`. Binärer/SHA i provenance; cache under `$HOME/.cache/megaron-steg4`. Vid ny miljö: installera dessa versioner till dess `bin/` respektive `tools/` (Go-install med task-specifik GOBIN, npm med `--prefix`). Ingen installation i produktionsrepot.

```sh
python3 docs/reviews/steg4-svep/collect.py
python3 docs/reviews/steg4-svep/verify.py > docs/reviews/steg4-svep/verification.log
python3 docs/reviews/steg4-svep/candidates.py > docs/reviews/steg4-svep/candidates.log
```

collect sparar exakt argv/cwd/exit/tid i varje `.run.json`. verify kör analysen och kontrollproven; candidates beräknar churn/konsumenter och rapport. Full AST och literalgrupper är deterministiskt gzip-komprimerade: `gzip -dc raw/ast.json.gz` respektive `classified/repeated-literals.json.gz` (från denna katalog). SHA/manifester och klassificering är granskningsunderlag, inte nya produktionsgrindar.

## Grindar och kända avgränsningar

Kod/mätning: grön, 8 instrument + 19 kontroller; inga nya produktionsgrindar. Visuell/användare/drift: ej tillämpligt på denna analysslice. Semantisk: kopie-/churn-/konsumentmått separerade, varje klass kan spåras till sites, åtta kandidater utan slutrankning. **Full Go/vet/JS-spelsvit har inte körts eller påståtts grön**; Go-instrumentet kompileras/körs och deadcode laddar programmen som mätning.

Trösklar missar korta och semantiskt olika skrivna kopior. SQL är Go-strängliterals, inte komplett dynamisk SQL-expansion eller migrationsinventering. Upprepade literaler gäller Go-AST; JS mäts av jscpd. Syntaktiska call-sites är inte runtimefrekvens eller en typupplöst callgraph. RTA utgår från samtliga Go main-rötter i ./..., inte bara HTTP-route-registret. `jscpd` rapporterar canvaslikhet som inte motiverar förändring i reglerna. Kronologisk rådata och versioner finns kvar; inga lagertak-, FoodNet- eller ruttarbete som redan landat räknas som nya genomföranden.

## Metodisk lärdom

1. Kända kontroller måste pröva instrumentens egna luckor, inte bara att en process exitade grönt.
2. Skilda SQL-kommentarer gömmer samma regel, samma tal kan stå för skilda regler.
3. Filchurn och syntax-call-sites ger prioriteringsunderlag; felrisk behöver konkret bugghistoria eller uttalad osäkerhet.

## Överlämning

Gren `codex/steg4-svep`, worktree `/home/tk/wt/codex-steg4-svep`. Claude granskar rapporten, gör steg 5 och integrerar. Ingen T2/Umami/U har startats. Slut-hash lämnas i chat.log och board; därefter STOPP.
