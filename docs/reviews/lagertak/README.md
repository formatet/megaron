# Lagertak — en källa

## SAMMANFATTNING
Premisskoll från master f1217c9d. Föreslagen ägare province.DefaultGoodStorageCap
godkänd av Claude 2026-10-07 17:04. Implementation cf57d5bd klar, full Go/vet gröna. Ingen merge/push/deploy/BILD.
Märkning: bevisar kedjegrindens oförändrade lagersemantik; arkitekturprogram steg 7.

## SLICE-KONTRAKT
| Fält | Kontrakt |
|---|---|
| Problem | Sex produktions-SQL-ställen duplicerar economy.goodCaps tekniska lagertak. |
| Spelarsanning | Samma varulager och spill som före refaktorn. |
| Invariant | Endast ny rad får standardtak; befintliga cap/LEAST/grain/silver-regler ändras inte. |
| Scope | Gemensamt nedre ägarpaket, goodCap och de sex utpekade cap-konsumenterna, tester och rapport. |
| Non-scope | Poolmodellen, balansering, migrationer, recall, UI och deploy. |
| Acceptans | Baslinje på fresh DB; varje konsument läser samma tak; dubblerade literaler borta; giltig enställig mutation röd; full fresh Go +vet gröna. |
| Stopvillkor | G1 transport→economy förbjudet: ägarförslag tillbaka till Claude före implementation. Canon/BILD/live/filerkonflikt → Claude. |
| Bevisplan | Färsk PG16, SQL/konsumentprov, en konstantmutation och fullsvit. |

## PREMISSKOLL (före kod)
1. `economy.goodCap(key string) float64` (recompute.go:916) returnerar 1_000_000
   för ALLA nycklar. Det är ett tekniskt tak, inte varuspecifikt eller lager-/populationsskalat.
2. Samtliga sex utpekade literaler är cap-kolumnen vid INSERT i settlement_goods:
   handlers/logistics.go:79, handlers/create_metropolis.go:133,
   combat/unit_arrival.go:1922, transport/arrival.go:147,
   transport/intercept.go:368, economy/trade.go:359. Ingen är ett pris eller separat silvertak.
   goodCap används redan i recompute och trade_return. Andra testliteraler, tidsstämplar
   och migrationers historiska SQL ingår inte.
3. G1 tillåter inte transport→economy; den enkla exporten skulle bryta kontraktet.
   Förslag: province.DefaultGoodStorageCap (redan tillåtet nedre katalogpaket för
   economy/transport/combat); goodCap delegerar dit. Inga nya G1-kanter.
   Alternativ: nytt goodspaket kräver flera G1-beslut; consumer-interface är onödigt
   stort för dagens platta konstant. Claude tillfrågad före kod och godkände province 17:04: endast INSERT-cap, inga nya G1-kanter.
4. Vault megaron_plan_lagerpooler beskriver framtida pooler, men dessa byggs INTE här.
   Befintliga SQL cap/spill-on-conflict och senare silverseed lämnas semantiskt intakta.

## BASLINJE
`tools/gotest.sh` på ny PG16/migration 160, kontraktscommit 94c2fda3:
berörda fem paket, namngivet urval Cap/Storage/Found/Coloni/Delivery/Arrival/Intercept/G1,
gröna (baseline.log). Källvakten `TestDefaultGoodStorageCapConsumers` gav assertions
rött för alla sju kopior före implementation (red.log). Inget runtimefel ändras:
rött→grönt är arkitekturinvarianten, tidigare numeriskt beteende var redan rätt.

## IMPLEMENTATION
Claude godkände province som ägare. `province.DefaultGoodStorageCap` är enda
produktionsliteralen. SQL använder bundna cap-parametrar; economy.goodCap
returnerar samma konstant. Inga nya paketkanter: economy/transport/combat hade
redan province; api/handlers är protokolladapter och använder redan province.
G1-tabellen/CLAUDE.md behöver därför ingen ändring. Inget verb/spelartext ändras:
ingen fyraytors- eller BILD-slice.

## EXPERIMENT
| Hypotes / ändring | Resultat |
|---|---|
| Privat literal i sex SQL-INSERT plus goodCap | Källvakt röd före implementation, grön efter. |
| Grundning av metropolis och koloni | DB cedar-cap är exakt historiskt 1_000_000. |
| Logistics / transport delivery / loot | Ny DB-rad cap=1_000_000, amount=5; vid befintlig cap=7 klipps nästa last till amount=7 och cap förblir 7. |
| Trade delivery och return | Befintliga DB-regressionsprov läser exakt 1_000_000 och goodCap efter första last, 120 efter två laster. |
| goodCap | Grain/fish/livestock/cedar/silver/okänd nyckel returnerar exakt samma historiska tak. |
| Ägarkonstant 1_000_000→500_000 på ETT ställe | Alla åtta namngivna prov gav assertions rött, inga compile-fel; original återställt. |

Reproduktion: `python3 docs/reviews/lagertak/mutation.py`; scriptet återställer
produktionsfilen även vid fel, men avslutar utan slutlig grön svit. Kör därefter
`tools/gotest.sh` mot ny DB. Varje körning skapar egen PG16 och migration 160.
Loggar: [baseline.log](baseline.log), [red.log](red.log), [green.log](green.log),
[mutation.log](mutation.log), [full-suite.log](full-suite.log), [vet.log](vet.log).

## GRINDAR
Kod: fokuserade fresh-DB-prov, full `tools/gotest.sh` (PG16/migration160, inklusive G1), och full `go vet ./...` gröna.
Semantisk: numeriskt tak och befintligt cap/spill oförändrade; samtliga sex
SQL-vägar, goodCap och trade-return har namngivna DB-/funktionsprov.
Visuell/användare: inget nytt gränssnitt eller verb; interna kredit- och
grundningsvägar provas mot riktig PostgreSQL. Inget fullständigt register/join-
browserflöde görs för denna mekaniska refaktor. Drift: ingen deploy.
Provenance är gotest.sh:s faktiska commit+schema-rad i varje logg; ingen server-
healthz finns för denna testslice. Fullsviten kör implementation cf57d5bd.

## METODISK LÄRDOM
En delad domänkonstant måste placeras under samtliga konsumenter i G1, inte
exporteras ur den nuvarande ägaren om den gör beroendet uppåtgående.

## KÄNDA AVGRÄNSNINGAR
Ingen migration/backfill eller ny lagerpoolmodell. Test-/historiska migrations-
literaler rörs inte. Standardtakets befintliga numeriska värde flyttas, inte tunas.
Grenen utgår från f1217c9d; Claudes efterföljande recall-rivning är inte rebased
in här. Claude äger integration mot nyare master.

## Resume checkpoint
Implementation cf57d5bd och kontrakt94c2fda3. Alla fokuserade konsumentprov och
mutation bevisade, original återställt; full Go/vet gröna.
Nästa: Claude granskar/integrerar mot nyare master. Codex tar därefter
köad order .agents/order-aggregat-recall.md från aktuell master, egen gren.
Worktree /tmp/megaron-codex-lagertak-20261007, gren codex/lagertak.
Ingen merge/push/deploy. Recall-arbetsytan orörd.
