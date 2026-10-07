# Lagertak — en källa

## SAMMANFATTNING
Premisskoll från master f1217c9d. Föreslagen ägare province.DefaultGoodStorageCap
godkänd av Claude 2026-10-07 17:04; implementation pågår. Ingen merge/push/deploy/BILD.
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

## Resume checkpoint
Implementation och konsumentprov skrivna; premiss/ägare godkänd av Claude.
Nästa: giltig mutation av ägarkonstanten, återställning, full tools/gotest.sh och vet,
rapport/vault-handover. Worktree /tmp/megaron-codex-lagertak-20261007, gren
codex/lagertak från f1217c9d. Ingen merge/push/deploy. Recall-arbetsytan orörd.
