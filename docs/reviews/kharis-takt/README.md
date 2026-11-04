# Kharis-takt (order 2026-11-02, 30 dygn)

Status: WIP (paus 2026-11-04). Hela sviten är INTE körd.

## Beslut: inga kharis-avdrag skalas (Timothy 2026-11-04)
Fula ord, hiketeia och tempelrivning förblir som idag. Inget avdrag ändrat.

## Inventering av absoluta kharis-avdrag (belopp, belopp x 5/18; endast information)
| Plats | Belopp | x 5/18 |
|---|---|---|
| server/internal/religion/profanity.go:12 ProfanityKharisLoss (per ord) | 5 | 1,39 |
| server/internal/religion/hiketeia.go:14 HiketeiaKharisLoss | 70 | 19,44 |
| server/internal/kharis/demolition.go:12 TempleDemolitionPenaltyPerLevel (per nivå; api/handlers/building_demolish.go:162) | 10 | 2,78 |
| server/api/handlers/province.go:2358/2441 spec.CostKharis x män | 0 överallt | 0 |

Böner spenderar aldrig kharis. combat/cancel_training.go:109 återbetalar kharis (inget avdrag).

## Trajektorier (analytisk modell, 1 stad, fullbemannade L1-tempel, tak 40/50, start 40)
Dag 10/20/30/40/60.
- 0 tempel: före 28/16/4/1/1 · efter 36.7/33.3/30/26.7/20
- 1 tempel: före 49/50/50/50/50 · efter 42.5/45/47.5/50/50
- 2 tempel efter: 48.3/50/50/50/50 · 3 tempel: 50 hela vägen

## Mutant
KharisPerDemand=1.2 gör pace_test.go:68 (40->30) och pace_test.go:81 (L1 netto 0,25) röda. Återställd.

## Flaggor
- Straff: ~75 dygn under 30 utan tempel (2 %/dygn) ger ~1,5 förväntade straff före 5 (förut ~0,4).
- Kult-draweren läser netto ur API men visar toFixed(1); med 0,58/0,33/0,25 grovt. Ej ändrat (5 JS-tester).
- Start 40 = tak 40 utan tempel: ingen beteendeändring.
- Ändrade tester: kharis-paketets siffror (l1_climb 180->260 ticks, firstPrayer<=17; realm_rules start 10 för golvklämma; övriga följer konstanterna).

## Återstår
Hela sviten (tools/gotest.sh), baslinje-tabell, ev. drawer-precision.
