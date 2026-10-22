# ORDER24 — kultförsörjning efter städer och invånare

## Steg 0: skrivskyddad simulering före kod

Snapshot 2026-10-22T22:27:06.021497+00:00, tick3218, BEGIN READ ONLY=on, mig202. Körande healthz: {"commit":"3c80cc2","migration":202,"status":"ok"}.

| Wanax | Städer | Invånare | Matade/placerade tempelgubbar | Produktion | Belastning före | Efter | Netto före | Efter |
|---|---|---|---|---|---|---|---|---|
| Hephaistos | 2 | 9423 | 2/2 | 2.4354 | 0.2000 | 1.2423 | 2.2354 | 1.1931 |
| Pheidon | 2 | 13798 | 2/2 | 2.4354 | 0.2000 | 1.6798 | 2.2354 | 0.7556 |
| Talthybios | 3 | 8775 | 2/2 | 2.4354 | 0.4000 | 1.3775 | 2.0354 | 1.0579 |
| formatet | 2 | 13587 | 4/4 | 4.8709 | 0.2000 | 1.6587 | 4.6709 | 3.2122 |
| gpt-damon | 1 | 12547 | 2/2 | 2.4354 | 0.1000 | 1.3547 | 2.3354 | 1.0807 |
| gpt-kleio | 2 | 10175 | 2/2 | 2.4354 | 0.2000 | 1.3175 | 2.2354 | 1.1179 |
| gpt-lykon | 1 | 12562 | 2/2 | 2.4354 | 0.1000 | 1.3562 | 2.3354 | 1.0792 |
| zilip | 0 | 0 | 0/0 | 0.0000 | 0.1000 | 0.1000 | -0.1000 | -0.1000 |

Negativt efter: 1/8 samtliga PWR (zilip); bland stadsägare 0/7 (). Nytt negativt: ingen.

Produktion är planerad nästa ticks brutto från just de tempelgubbar vars eget offer kan betalas, inte nominell kapacitet. Knapphetsfaktor 1.15973035, härledd från snapshotens gudavärde för korn / goods.labor_value och klämd till1–2. Lokala lager settled() utan påhittad capklämning, tempel i gud-ID-ordning,2platser/nivå. Inga andra framtida förändringar antas. Nettot är före golv/tak. PWR utan städer visas separat från stadsägare; ingen negativ extra-stadterm för0städer.

Invånarkälla: settlements.population i samma icke-avslutade städer som stadsräkning och tempelläsare (state NOT IN sunk/collapsed/razed); detta är stadsbefolkningen som mat/tillväxt läser. Inte units.size, gubbar avrundade till hundratal eller lagrad födelseprognos.

Ingen kod eller DBskrivning föregick denna simulering. capture.py/snapshot.sql och simulate.py gör underlaget reproducerbart.
