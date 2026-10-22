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

Produktion är planerad nästa ticks brutto från just de tempelgubbar vars eget offer kan betalas, inte nominell kapacitet. Knapphetsfaktor 1.15973035, härledd från snapshotens gudavärde för korn / goods.labor_value och klämd till1–2. Lokala lager settled() utan påhittad capklämning, tempel i gud-ID-ordning,2platser/nivå. Inga andra framtida förändringar antas. Nettot är före golv/tak. PWR utan städer visas separat från stadsägare; ingen negativ extra-stadterm för0städer. Stadsfria raden visar formel/projektionsnetto, inte faktisk Hostdebitering: KharisTick väljer kapitalägare.

Invånarkälla: settlements.population i samma icke-avslutade städer som stadsräkning och tempelläsare (state NOT IN sunk/collapsed/razed); detta är stadsbefolkningen som mat/tillväxt läser. Inte units.size, gubbar avrundade till hundratal eller lagrad födelseprognos.

Ingen kod eller DBskrivning föregick denna simulering. capture.py/snapshot.sql och simulate.py gör underlaget reproducerbart.

## Kontrakt och implementation

R2: kultförsörjning = .1 + .2 per stad utöver första + .1 per1000 invånare.
En beräkningsägare i kharis; tick, ProjectDailyNet och ProjectDecay läser samma population ur settlements.
Kult/API/Keryx/Codex visar grund/städer/invånare samt produktion och netto. Marginalutbytet +1.05 är
före knapphet och förutsätter en fri gubbe, tempelplats och matad tjänst; det är ingen garanti att
spelaren faktiskt kan bemanna en till. Ingen ändring av bemanning, offer, böner, vrede, tak eller rivning.
Ingen migration. Gate: proves kultförsörjningens begriplighet i huvudkedjan.

Baseline: nytt verkligt DB/HTTP-oberoende tickprov röjer både prognos−.4 i stället för−.85 och
lagrad kharis39.6 i stället för39.15 (3städer/3500inv). Baseline-red.log före kod.
Äldre testfixturer/förväntningar uppdateras till ny belastning; inga produktionsgenvägar för att hålla gamla tal gröna.

Produktionscheckpoint `7a67c549`, bas `3c80cc20`; steg0-simuleringen committad före kod i `600c0ca4`. Inga live-, merge-, push- eller deployoperationer i denna slice.

## Bevis och avgränsningar

- `targeted-final.log` och `owner-projection.log`: formelpunkter, riktig hel-tick mot prognos,
  invånare mellan hundratal, avslutade städer, ägarens API och främmande läsares frånvaro av privat prognos gröna.
- `mutation-red.log`: invånartermen droppas fysiskt i EN läsare (ProjectDailyNet), nytt DB-prov blir namngivet rött
  medan riktig hel-tick fortfarande ger rätt värde. `mutate.py` återställer byteexakt i finally och kör fresh green.
  `mutation.json` sparar källans SHA256.
- `player-scenario.log`: två separata syntetiska stadssetup, riktig auth/HTTP, byggd Keryx status, Firefox Kult.
  API visar3500inv/.35invånarkostnad/.65total/2.1produktion/1.45netto; CLI/webb visar dessa tal med respektive precision.
  DB-kharistupel och lager förblir oförändrade av läsningen.0JSfel. APIpayload, CLIutskrift och native390/1000bilder sparade.
  BILD A1–A2 i index.html, kontrollerad vid1:1. TEXT-begriplighet kräver playtest.
- Tidiga `packages.log`/`packages-final.log` är röda försök med äldre basförfallsassert och handbyggda
  wanaxSnap-fixturer som saknade population. De uppdaterades till sitt faktiska invånarantal; ingen produktionskod
  ändrades för att bevara gamla balansförväntningar. Notisfelprovets befolkning varierar100→101→101,
  därför följer dess lagrade kharis29.89→29.7799→29.6698.
- `player-scenario-rounding-attempt.log`: felaktig testförväntning+1.4 för net1.45; faktisk CLIprecision ger+1.5.
  Endast verifierarens förväntning rättad; exakt APIvärde1.45 verifieras separat.

Ny privat information: invånartermen gör det möjligt att härleda hela rikets befolkning ur kultkostnaden.
Provinssvarets kultbreakdown/produktion/netto går därför genom samma ägarkontroll som temple_offers;
främmande läsare får inte denna prognos. Befintlig annan stadsinformation/FOW ändras inte.
Dagens underhållsevent behåller gamla utfallsfält och får additiva realm_population/decay_rule_version
för den nya beräkningen. Gamla event och deras innebörd skrivs inte om.

Detta är inte en ny join→grundning→tempelbygge-speltestning; klientproven börjar med isolerade syntetiska städer.
Ingen automatiserad rådgivning om exakt nödvändig bemanning byggs. Marginalutbytet anges uttryckligen före knapphet
med kraven på ledig arbetskraft/plats/offer. Övriga religionsbeslut väntar på egna order.

Reproducera klientproven: `python3 docs/reviews/belastning/run_player_scenario.py` (egen fresh DB,
tillfällig Go-harness från player_scenario.go.txt, alltid bortstädad; inga browserkrav i normal Go-svit).
Reproducera mutation: `python3 docs/reviews/belastning/mutate.py`.
Slutligt JS:725/725 gröna (`js-final.log`); vet grönt (`vet.log`). Full färsk Go-svit GRÖN: 26 paket med tester, migration202, world157.803s (`full-fresh.log`). Provenance +ändringar gäller enbart rapportfiler; server/web-koden är exakt checkpoint `7a67c549`.

Samtliga bevisfiler är SHA256-listade i SHA256SUMS; `python3 docs/reviews/belastning/verify.py` kontrollerar snapshot, formeltabell, rött→återställt grönt och slutbevis.
