# Lagertak — en källa

## SAMMANFATTNING
Premisskoll från master f1217c9d. Föreslagen ägare province.DefaultGoodStorageCap
väntar på Claude; ingen produktionskod ändrad. Ingen merge/push/deploy/BILD.
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
   stort för dagens platta konstant. Claude tillfrågad före kod.
4. Vault megaron_plan_lagerpooler beskriver framtida pooler, men dessa byggs INTE här.
   Befintliga SQL cap/spill-on-conflict och senare silverseed lämnas semantiskt intakta.

## Resume checkpoint
Kontrakt/premisskoll sparad före kod. Väntar på Claudes ägarbeslut; oförändrad
färsk-DB-baslinje kan köras oberoende. Worktree /tmp/megaron-codex-lagertak-20261007,
gren codex/lagertak från f1217c9d. Recall- och karavanarbetsytorna orörda.
