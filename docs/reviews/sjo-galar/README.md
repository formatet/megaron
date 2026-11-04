# Stängd sjö ger ingen galärgåva (Sonnet 2026-11-04)

Order `.agents/order-sjo-flod-alfabet-20261101.md` punkt 1. Beslut Timothy 2026-11-01.

## Ändring
- `province.HasThalassaWaterway` (`server/internal/province/waterway.go`): den rekursiva vattenvägs-CTE:n som varvet redan använde, flyttad hit — EN definition.
- Varvsgrinden (`handlers/province.go`) anropar den; ingen egen SQL kvar där.
- Grundandet (`found_metropolis.go` steg 8): galär bara om `coastal` OCH vattenväg till Thalassa. Hamnen/`coastal`-flaggan orörd.
- Prognosen (`world.go` `foundingGifts`, ny param `galleyGift`) lovar bara galären där settle ger den.
- Codex `lake.md`, `founding.md`; keryx `founding settle`-hjälptext.

## Inventering
- Enda stället som delar ut galär vid grundande är `found_metropolis.go`. `create_metropolis.go` ger bara farmen (`!Coastal`); kolonier (`unit_arrival.go foundColony`) delar ut ingen galär (grep: ingen `TypeGalley` där).
- `kharis/tick.go:1495` är bara en kommentar om gamla blessing-regeln.

## Bevis
Tester `found_galley_waterway_test.go` (stängd sjö: 0 galär; sjö med flod→ford→hav: 1; havskust: 1) och `founding_gifts_test.go` (rad "closed lake").
Mutanter: (a) predikatet ersatt av `true` (= gamla `coastal`) dödas av `TestFoundingClosedLakeGivesNoGalley`; (b) negerat predikat dödas av alla tre; (c) prognosen ignorerar `galleyGift` dödas av `TestFoundingGifts_DemeterOnlyWhenAFarmHexExists`.

## Öppen fråga till Timothy
Stad på stängd sjö får varken galär eller Demeters farm (farmen styrs av `coastal`, "ENTEN farm ELLER galär"). Sjöfisket matar den, som kusten gör, men en sjöstad utan farm och utan galär kan svälta på karg mark. Ordern rör inte farmen — oförändrat, men värt ett beslut: ska en stängd-sjö-stad få farmen i stället?
