# Alfa-ekonomi — värld test-purse-c39bae8e-062a-4013-8b8c-d4591cab24a0 (a42f78f3), tick 3

## Per stad (människors städer)

| Wanax | stad | befolkning | matnetto/tick | säd | fisk | magasin | magasin Δ | täckning (dagar) | silver | garnison (män) | garnisonens förbrukning/tick |
|---|---|---|---|---|---|---|---|---|---|---|---|
| purse-4e66e4f5-4dd4-43dd-bd06-42c3addf3e32 | Mother City (huvud) | 4000 | -20.00 | 999 |  |  | ej mätbart (<2 event) |  | 720 | 0 | 1.20 säd / 2.00 silver (tick 2) |
| purse-4e66e4f5-4dd4-43dd-bd06-42c3addf3e32 | Newhaven-e5fe44ca | 50 | 0.25 | 8 | 0 |  | ej mätbart (<2 event) |  | 15 | 0 |  |

## Per Wanax

| Wanax | städer | silver i städer | byggnader | köade byggen | enheter | tid till första bygget (tick) |
|---|---|---|---|---|---|---|
| purse-4e66e4f5-4dd4-43dd-bd06-42c3addf3e32 | 2 | 735 | 0 | 0 | 1 / 50 män | inget bygge klart än (grundad 0) |
| 4d9dee87-b48a-4fd1-8250-9eac929e6059 | 0 |  | 0 | 0 | 0 / 0 män | inget bygge klart än (grundad 0) |

## Komplett silverrevision (SilverAuditV2)

| tick | mining_epoch | total_silver | städer | BUY-escrow | SELL-escrow | Host-kassor | enheternas pungar | transportlast | väntande handelsretur | därav ägarlöst | V2-intervall | realiserat utvunnet sedan föregående | utvinning krediterad totalt | audit_prev_total | net_delta |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 3 | f8ba3dc9-bf95-4462-99ce-8795e72068ae | 735.00 | 735.00 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | öppningsrevision (egen baslinje) | 0.00 | 0.00 | 735.00 | 0.00 |
| 3 | afecfb63-89b8-4af6-a533-d04bd06757a7 | 735.00 | 735.00 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | 3→3 | 0.00 | 7.00 | 735.00 | 0.00 |
| 3 | afecfb63-89b8-4af6-a533-d04bd06757a7 | 735.00 | 720.00 | 0.00 | 0.00 | 0.00 | 15.00 | 0.00 | 0.00 | 0.00 | 3→3 | 0.00 | 7.00 | 735.00 | 0.00 |

Utvinning är faktiskt krediterat silver, inte produktionstakt × tick. Ägarlöst silver ingår i stockkomponenterna; det läggs inte till igen.

Övrigt netto = net_delta − mined_since_last: 0.00 silver (tick 3). Detta är kranar minus sänkor utöver gruvutvinningen, inte automatiskt ett bokföringsfel.

### Fördelning per Wanax — revision tick 3

| population | count | total_silver | median_silver | max_silver | zero_count | zero_share |
|---|---|---|---|---|---|---|
| world records and asset owners, including AI and dispossessed Wanax | 2 | 735.00 | 367.50 | 735.00 | 1 | 0.5000 |

zero_share är en andel (0–1). Fördelningens population anges av servern; människors stadsstock ovan är en annan avgränsning.

Ej mätbart i DB: befolkningskurva före de dagliga körningarna. Per-Wanax-silver i städer ovan är nuläge; V2-fördelningen är revisionens snapshot. Matnetto = rå produktion minus invånare × 0.005 (spegel av economy.FoodNet).
