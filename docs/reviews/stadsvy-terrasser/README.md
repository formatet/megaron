# Order 25 — den levande terrassvyn

## SAMMANFATTNING

Stadspanelen visar nu 04-gatan, 21c:s terrasser och en ljus varm bergshorisont,
i 160×96 logiska pixlar ×2. Befolkning ger 1/3/5 bostadsplan; verkliga byggnader,
mur och samtidiga statuslager ger innehållet. [BILD-galleri](index.html).

Produktion: `f889014a` + `afb53f17` + `3ee21b95` på `codex/stadsvy-terrasser`.
**HANDOVER / STOPP. Claude granskar, Timothy avgör BILD före merge.** Ingen push/deploy.
[Firefox mobil före](native-a/city-mobile-before.png) ·
[efter](native-a/city-mobile-after.png) · [tre dev-städer](dev-contact.png).
Bas `10e64109`; master har under arbetet gått till `445ffd34` med enbart Go-ändringar,
utan överlapp med denna grens renderer/CSS. Claude äger integrationen.
Dev-snapshotarnas `/healthz`: `commit=10e6410 migration=189 status=ok`.
Båda isolerade slutprocessernas `/healthz`:
`commit=3ee21b95 migration=189 status=ok` (full hash i respektive `proof.json`).

## SLICE-KONTRAKT

- Problem: den levande stadsvyn motsvarade inte den valda gatu-/terrassbilden.
- Spelarsanning: staden växer i sammanhängande terrassplan med fri bergshorisont.
- Invariant: byggnader, mur och status hämtas från ägarens riktiga provinsdata.
- Scope: webbens `render/city.js`, dess tester och `.city-scene` i CSS.
- Non-scope: karta, server, keryx, Codex, mekanik och `regler.md`-förslagen.
- Acceptans: determinism, växande bostadsprojektion, mur/status, Firefox 390/desktop,
  grön JS-svit. Kedjegrinden: **väntar till efter**.
- Stopp: kanonändring eller ny mekanik kräver separat beslut; ingen sådan behövdes.

## BASLINJE

`10e64109`: **651/651 JS** i `baseline-js.log`. Det nya kontraktets första körning
saknade `buildCityScene`-exporten och föll vid import (`contract-red.log`).
Fysiska mutationer nedan prövar sedan de verkliga beteendena.
I det naturliga spelprovets förearm hämtar Firefox baslinjens renderer/CSS vid deras
riktiga asset-URL:er. Bilden sträcktes till 418×198,55 på desktop och 390×185,25 på mobil.
Efterarmen laddar grenens byte-verifierade filer: alltid **320×192**, DPR1.

## EXPERIMENT

| Hypotes | Ändring | Resultat | Beslut |
|---|---|---|---|
| Storlek ska bäras av geometri | 21c:s befolkningsband → 1/3/5 terrassplan | Bostadsprojektion 684 / 2674 / 4012 logiska pixlar vid samma funktionsinventarium/mur | Behåll; mänsklig läsning avgörs i BILD |
| 21b:s varma fond var för mörk | 03:s sky/far-värden, ljusa varma bergsband; ursprungliga ljusa takmaterial | Färg- och gråskaleark vid spelskala | Till Timothy |
| Status måste motsvara servern | `food.growth_state=shrinking`, `besieged`, faktisk `build_queue` | Statuslager kan samexistera utan ändrad grundstad | Behåll |
| Port måste finnas före stängning | Stängd belägringsport bara vid `walls>0` | Separat prov för omurad belägrad stad | Behåll |
| Naturligt nygrundad stad kan genast bygga tempel | Första riggutkastet försökte vanlig CLI-order | Korrekt 422: saknade sten/timmer (`native-material-gate.log`) | Ingen SQL/materialinjektion; slutprovet gäller grundning/panel, positiva byggen i snapshot/fixturer |

## GRINDAR

| Grind | Bevis |
|---|---|
| Kod | **660/660 JS**, `final-js.log`; nio nya renderertester. Ingen Go/SQL ändrad. |
| Mutation | Sex fysiska mutationer: fast terrassantal, saknad mur/svält/belägring/byggställning, mörk himmel. Namngiven RED → byte-exakt återställning → färsk GREEN. `mutations.json`, `*-red.log`; källhash verifierad mot slutkoden. |
| Visuell | `verification.json`: 18 scener, Firefox DPR1 på 1440/390, exakt 320×192, ingen sidspill/pageerror; 2×2-cellernas pixlar identiska. Logisk 1:1, ×4, struktur, bostadsmask, status/liv och gråskala finns. Byggaren har granskat spelskala och gråskala; Timothy återstår. |
| Semantisk | MalliaWest 650, Mallia 6204, Phaistos 11344 ur oförändrade ägar-API-svar. `dev-snapshots.json` + `capture.py`. Phaistos har två verkliga byggen i kön. Tempel/hamn/gruva finns bara när API:t anger dem. Negativt matnetto eller noll spannmål blir inte automatiskt svält. Saknat provinsdata ger ingen påhittad stad. |
| Användare | `native-a/proof.json`, `native-b/proof.json`: två separata färska PG/Redis-världar; riktig registrering → CLI login/join/found → Firefox City 1280/390. Rätt asset-hashar, inga pageerrors/panel- eller sidspill. Ingen SQL-skrivning eller API-replay. |
| Drift | Ej deployad. Dev lästes enbart. BILD och integration ägs av Timothy/Claude. |

## METODISK LÄRDOM

Bostadsmasken räknar synliga bostadspixlar efter skymning, utan terräng, funktionshus,
mur, figurer, rök eller status. Måttet visar växande geometri och kan fånga ett fast
befolkningsband; det bevisar inte att en människa känner igen storlekarna.
`*-structure.png` och `fixture-contact.png` är bildmotparterna till måttet.

Riggens första PostgreSQL-start gav `create migrator: EOF` i en körning
(`native-startup-eof.log`, `native-startup-server.log`). Readiness kräver nu
TCP på `127.0.0.1`, så den tillfälliga Unix-socketservern under init inte räcker.
Slutkörningarna `native-a` och `native-b` är båda gröna, på separata färska databaser.

## KÄNDA AVGRÄNSNINGAR

Dev-galleriet återger frysta riktiga API-snapshotar med produktionsrenderern. Det är
inte ett påstående att grenen kör på dev. De naturliga slutproven visar nystartade städer;
positiv belägring/svält verifieras med tydligt märkta fixturer, inte framtvingade livehändelser.

Bebyggelse och status visar typer på stadsnivå, inte en fysisk rekonstruktion av varje
catchmenthex. Kända belägringsflaggor ger port/spjut, utan påhittade fiendepositioner.
Härdrök motiveras av bebodd bostadsmassa. `temple_offers.fed` är nästa gåvas prognos,
inte bevis för ett genomfört offer; den görs därför inte till offerrök. Kartan är orörd.

## REPRODUCERA

Från arbetsträdet (Node, Python/Pillow/Playwright Firefox; native kräver Docker och Go):

```sh
node --experimental-default-type=module --test $(rg --files web/static/js | rg '\.test\.mjs$')
python3 docs/reviews/stadsvy-terrasser/mutation_check.py
python3 docs/reviews/stadsvy-terrasser/verify.py
python3 docs/reviews/stadsvy-terrasser/native.py docs/reviews/stadsvy-terrasser/native-a
python3 docs/reviews/stadsvy-terrasser/native.py docs/reviews/stadsvy-terrasser/native-b
python3 -m http.server 8099
```

Öppna `/docs/reviews/stadsvy-terrasser/`. Kör mutationerna färdigt före browser/native:
mutationstestet skriver till källfilen tillfälligt. `capture.py` uppdaterar snapshotarna
med läsning från de uttryckligen angivna spelarkontona; behövs inte för reproduktion.
