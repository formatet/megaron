# ORDER18 — matbalans efter fiske och nya lönen

## Sammanfattning

**Ingen av de 12 städerna hade otäckt civil mat vid senaste utfodringen. Damon riskerar däremot svält inom 30 ticks om nuvarande produktion och bemanning fortsätter: modellens första otäckta måltid kommer efter 29–30 ticks. Phaistos har redan missat skeppslön.** Övriga 11 städer får inget civilt underskott i 30 ticks-scenariot. Detta är en fryst prognos, inte ett löfte om vad spelarnas framtida handlingar gör.

Rå DB-snapshot **2026-10-22T18:21:09.742197+00:00** (20:21:09 svensk tid), värld 2178e795, tick 2972. Healthz från körande process: **commit d248b09, migration 200, status ok**; DB migration200/dirty=false. Samma SELECT/MVCC-snapshot för alla DB-tabeller; `transaction_read_only=on`, åtkomst via `tools/alfa_db.py` och `BEGIN READ ONLY`. Ingen Keryx-utskrift används. Läsningarna har inte ändrat spelet.

Kodbas för analysen: master d248b09e, egen docs-gren `codex/matbalans-efter-fiske`. ORDER13 är ännu inte integrerad: de fria gubbarna nedan är fysiska `floor(population/100)−count(placements)` enligt nuvarande API, inte ett antagande om framtida tempelbemanning. Gammal procentdevotion finns kvar i snapshoten och dras inte från denna fysiska pool.

## Matproduktion, behov och fria gubbar

Alla produktions-/behovstal i tabellerna är per tick; spelaren läser ett tick som ett dygn. G/F/O/V=grain/fish/oil/wine. Raterna är rå produktion; de innehåller inte matmåltider eller sold. Folk äter0,005 per person. Landarmé och hamnliggande skepp räknas efter faktisk size/Crew och status. Två skepp till sjöss äter ur egna förråd och ingår inte i stadsbehovet; de redovisas separat.

| Stad | G | F | O | V | Folk | Armé ur stad | Skepp ur stad | Netto mat | Fria gubbar |
|---|---|---|---|---|---|---|---|---|---|
| Amnissos | 52.933 | 0 | 26 | 9 | 56.075 | 0.21 | 0 | 31.648 | 0 |
| Damon | 9 | 23.12 | 3.667 | 9 | 96.06 | 1.8 | 0 | -53.073 | 137 |
| Gournia | 24.358 | 0 | 4.333 | 4.333 | 33.085 | 1.4 | 0 | -1.46 | 0 |
| Kydonia | 37.591 | 0 | 7 | 8.5 | 37.115 | 1.14 | 0 | 14.836 | 1 |
| Lato | 28.754 | 17 | 8.667 | 2 | 81.31 | 0.18 | 0 | -25.069 | 103 |
| Petras | 0.5 | 15.35 | 13.333 | 0 | 72.675 | 0.5 | 0 | -43.992 | 98 |
| Phaistos | 12.847 | 5 | 1.333 | 1 | 13.065 | 0 | 0 | 7.115 | 8 |
| Phalasarna | 0.5 | 6 | 4 | 0 | 10 | 0 | 0 | 0.5 | 2 |
| Plateia | 0.5 | 1 | 0 | 0 | 1.5 | 0 | 0 | 0 | 2 |
| Pylos | 11.833 | 4 | 0 | 1 | 10 | 0 | 0.15 | 6.683 | 3 |
| Vasiliki | 1.5 | 7.2 | 1 | 0 | 11.55 | 0 | 0 | -1.85 | 9 |
| Zakros | 15.524 | 9 | 8 | 11.333 | 63.01 | 0.045 | 0 | -19.198 | 35 |

Total produktion **407.01**, behov från städer **490.87**, netto **-83.86 mat/tick**. Ett världssummerat underskott innebär inte att alla städer svälter: lagren är lokala och behöver fysisk transport.

## Placeringar × rate, automatisk mat och råa DB-rater

`placements.csv` visar varje arbetad mat-/silverhex: antal placerade, antal som ryms, DB-grundtakt, relevant byggnadsmultiplikator och resultat. `all-goods.csv` visar **samtliga lagrade varor per stad**, antal placerade på varan, rå DB-rate och lazy/cappat lager. Olja/vin använder även separat förädlingsarbetare och min(boostpotential, förädlingskapacitet); deras arbetare får inte räknas två gånger som två fria flöden.

För de 60 raterna grain/fish/oil/wine/silver återskapar `analyze.py` produktionsreglernas join, byggnad på rätt hex, kapacitetsklamp, vik/deep-regeln, förädling och automatisk mat. Alla stämmer mot DB inom 1e−8. Detta är en oberoende analytisk spegling, **ingen alternativ regelägare i spelet**. Ingen aktiv skördevälsignelse finns. Alla städers population är vid/över murtaket, eller saknar positiv civil matnetto; därför ingen tillväxt i den frysta modellen.

| Stad | Placerad säd | Automatisk säd | Placerad fisk | Automatisk fisk |
|---|---|---|---|---|
| Amnissos | 51.833 | 1.1 | 0 | 0 |
| Damon | 8.5 | 0.5 | 23 | 0.12 |
| Gournia | 23.45 | 0.908 | 0 | 0 |
| Kydonia | 36.7 | 0.891 | 0 | 0 |
| Lato | 27.2 | 1.554 | 17 | 0 |
| Petras | 0 | 0.5 | 15 | 0.35 |
| Phaistos | 12 | 0.847 | 5 | 0 |
| Phalasarna | 0 | 0.5 | 6 | 0 |
| Plateia | 0 | 0.5 | 1 | 0 |
| Pylos | 11.333 | 0.5 | 4 | 0 |
| Vasiliki | 1 | 0.5 | 7 | 0.2 |
| Zakros | 15 | 0.524 | 9 | 0 |

Automatiskt: 0,5 säd från stadshexen plus befolkningens rest under100 som en bråkdel av bästa tillåtna matarbetaren. Olja/vin saknar sådant gratisflöde. Livestock har rate 0 i samtliga städer: dess tillskott sker som diskreta födslar, inte placeringsproduktion.

## Lager och räckvidd per matvara

**Netto/räckvidd per enskild vara är ett diagnosmått:** användningen fördelas proportionellt över de fyra aktuella matlagren för ett vanligt dygns folk+stadsarmé+hamnskepp, utan nödslakt. `netto=produktion−användning`. `räckvidd=lager/(användning−produktion)` vid negativt netto; ∞ betyder ingen minskning under just detta frysta blandningsantagande. Tom/saknad vara markeras—. Den verkliga blandningen ändras varje tick, och hjord/offer påverkar den. **Använd 30 ticks-simuleringen nedan för svältfrågan**, inte en isolerad varas räckvidd.

### Amnissos

| Vara | Lager | Produktion/tick | Vanlig användning/tick | Netto/tick | Räckvidd dygn |
|---|---|---|---|---|---|
| grain | 3319.587 | 52.933 | 20.057 | 32.876 | ∞ |
| fish | 468.841 | 0 | 2.833 | -2.833 | 165.506 |
| oil | 4000 | 26 | 24.168 | 1.832 | ∞ |
| wine | 1527.074 | 9 | 9.227 | -0.227 | 6735.993 |

Hjord: 50 djur = 150 mat vid nödslakt. Senaste verkliga civila måltid: oil 24.069, fish 2.838, wine 9.19, grain 19.977; unmet=0 (tick2971).

### Damon

| Vara | Lager | Produktion/tick | Vanlig användning/tick | Netto/tick | Räckvidd dygn |
|---|---|---|---|---|---|
| grain | 135.38 | 9 | 9.561 | -0.561 | 241.379 |
| fish | 976.37 | 23.12 | 68.954 | -45.834 | 21.302 |
| oil | 79.295 | 3.667 | 5.6 | -1.933 | 41.015 |
| wine | 194.633 | 9 | 13.745 | -4.745 | 41.015 |

Hjord: 50 djur = 150 mat vid nödslakt. Senaste verkliga civila måltid: oil 5.412, fish 68.212, wine 13.283, grain 9.153; unmet=0 (tick2971).

### Gournia

| Vara | Lager | Produktion/tick | Vanlig användning/tick | Netto/tick | Räckvidd dygn |
|---|---|---|---|---|---|
| grain | 918.861 | 24.358 | 10.812 | 13.546 | ∞ |
| fish | 1519.531 | 0 | 17.88 | -17.88 | 84.983 |
| oil | 246.122 | 4.333 | 2.896 | 1.437 | ∞ |
| wine | 246.122 | 4.333 | 2.896 | 1.437 | ∞ |

Hjord: 0 djur = 0 mat vid nödslakt. Senaste verkliga civila måltid: oil 2.752, fish 17.295, wine 2.752, grain 10.287; unmet=0 (tick2971).

### Kydonia

| Vara | Lager | Produktion/tick | Vanlig användning/tick | Netto/tick | Räckvidd dygn |
|---|---|---|---|---|---|
| grain | 2000 | 37.591 | 12.398 | 25.193 | ∞ |
| fish | 599.863 | 0 | 3.719 | -3.719 | 161.314 |
| oil | 1977.633 | 7 | 12.259 | -5.259 | 376.013 |
| wine | 1593.59 | 8.5 | 9.879 | -1.379 | 1155.799 |

Hjord: 150 djur = 450 mat vid nödslakt. Senaste verkliga civila måltid: oil 11.906, fish 3.624, wine 9.577, grain 12.008; unmet=0 (tick2971).

### Lato

| Vara | Lager | Produktion/tick | Vanlig användning/tick | Netto/tick | Räckvidd dygn |
|---|---|---|---|---|---|
| grain | 754.204 | 28.754 | 23.992 | 4.762 | ∞ |
| fish | 1391.973 | 17 | 44.281 | -27.281 | 51.024 |
| oil | 337.564 | 8.667 | 10.738 | -2.072 | 162.933 |
| wine | 77.9 | 2 | 2.478 | -0.478 | 162.928 |

Hjord: 50 djur = 150 mat vid nödslakt. Senaste verkliga civila måltid: oil 10.643, fish 44.492, wine 2.456, grain 23.72; unmet=0 (tick2971).

### Petras

| Vara | Lager | Produktion/tick | Vanlig användning/tick | Netto/tick | Räckvidd dygn |
|---|---|---|---|---|---|
| grain | 0.891 | 0.5 | 0.017 | 0.483 | ∞ |
| fish | 3105.663 | 15.35 | 57.517 | -42.167 | 73.652 |
| oil | 844.59 | 13.333 | 15.642 | -2.308 | 365.872 |
| wine | 0 | 0 | 0 | 0 | — |

Hjord: 50 djur = 150 mat vid nödslakt. Senaste verkliga civila måltid: oil 15.402, fish 57.261, grain 0.012; unmet=0 (tick2971).

### Phaistos

| Vara | Lager | Produktion/tick | Vanlig användning/tick | Netto/tick | Räckvidd dygn |
|---|---|---|---|---|---|
| grain | 928.488 | 12.847 | 3.321 | 9.525 | ∞ |
| fish | 1982.67 | 5 | 7.092 | -2.092 | 947.564 |
| oil | 450.209 | 1.333 | 1.61 | -0.277 | 1624.432 |
| wine | 290.943 | 1 | 1.041 | -0.041 | 7138.529 |

Hjord: 50 djur = 150 mat vid nödslakt. Senaste verkliga civila måltid: oil 1.611, fish 7.096, wine 1.04, grain 3.318; unmet=0 (tick2971).

### Phalasarna

| Vara | Lager | Produktion/tick | Vanlig användning/tick | Netto/tick | Räckvidd dygn |
|---|---|---|---|---|---|
| grain | 1.749 | 0.5 | 0.005 | 0.495 | ∞ |
| fish | 1944.908 | 6 | 5.189 | 0.811 | ∞ |
| oil | 1801.299 | 4 | 4.806 | -0.806 | 2234.631 |
| wine | 0 | 0 | 0 | 0 | — |

Hjord: 50 djur = 150 mat vid nödslakt. Senaste verkliga civila måltid: oil 4.809, fish 5.188, grain 0.003; unmet=0 (tick2971).

### Plateia

| Vara | Lager | Produktion/tick | Vanlig användning/tick | Netto/tick | Räckvidd dygn |
|---|---|---|---|---|---|
| grain | 1.098 | 0.5 | 0.002 | 0.498 | ∞ |
| fish | 595.064 | 1 | 0.906 | 0.094 | ∞ |
| oil | 389.153 | 0 | 0.592 | -0.592 | 656.877 |
| wine | 0 | 0 | 0 | 0 | — |

Hjord: 50 djur = 150 mat vid nödslakt. Senaste verkliga civila måltid: oil 0.593, fish 0.906, grain 0.001; unmet=0 (tick2971).

### Pylos

| Vara | Lager | Produktion/tick | Vanlig användning/tick | Netto/tick | Räckvidd dygn |
|---|---|---|---|---|---|
| grain | 728.944 | 11.833 | 2.619 | 9.215 | ∞ |
| fish | 1761.429 | 4 | 6.328 | -2.328 | 756.571 |
| oil | 0 | 0 | 0 | 0 | — |
| wine | 334.85 | 1 | 1.203 | -0.203 | 1649.56 |

Hjord: 150 djur = 450 mat vid nödslakt. Senaste verkliga civila måltid: fish 6.242, wine 1.186, grain 2.573; unmet=0 (tick2971).

### Vasiliki

| Vara | Lager | Produktion/tick | Vanlig användning/tick | Netto/tick | Räckvidd dygn |
|---|---|---|---|---|---|
| grain | 17.914 | 1.5 | 0.091 | 1.409 | ∞ |
| fish | 1979.341 | 7.2 | 10.109 | -2.909 | 680.473 |
| oil | 264.285 | 1 | 1.35 | -0.35 | 755.665 |
| wine | 0 | 0 | 0 | 0 | — |

Hjord: 50 djur = 150 mat vid nödslakt. Senaste verkliga civila måltid: oil 1.35, fish 10.114, grain 0.086; unmet=0 (tick2971).

### Zakros

| Vara | Lager | Produktion/tick | Vanlig användning/tick | Netto/tick | Räckvidd dygn |
|---|---|---|---|---|---|
| grain | 939.865 | 15.524 | 5.241 | 10.283 | ∞ |
| fish | 6816.544 | 9 | 38.012 | -29.012 | 234.957 |
| oil | 1469.808 | 8 | 8.196 | -0.196 | 7489.472 |
| wine | 2081.236 | 11.333 | 11.606 | -0.272 | 7637.758 |

Hjord: 50 djur = 150 mat vid nödslakt. Senaste verkliga civila måltid: oil 8.171, fish 38.05, wine 11.569, grain 5.22; unmet=0 (tick2971).

## Inom30ticks: fryst simulering och fasgräns

Ordning: produktion till cappade lager → stadsarmé/hamnskepp (full ranson eller ingen debit) → hjordfödslar/grainkostnad → civil måltid → lokalt tempeloffer. Folkets måltid äter djur över hjordtaket först, därefter de fyra varorna proportionellt, sist nödslakt under taket.1djur=3mat. Födslar varje5ticks: floor(hjord/10), högst vad säden kan föda; varje20ticks även obetalda kalvar.1säd per betald kalv. Offerplanen använder aktuell fryst gudsvärdering och ordningen grain/fish/timber/silver,0,25säd i referensvärde; saknad full gåva debiteras inte. Varje steg kontrolleras med varornas massbalans inklusive cappspill och förlorad hjord.

Snapshoten ligger i tick2972 medan senaste FoodTick/UpkeepSettled är2971. Lazy lagret innehåller redan tick2972:s produktion. Därför körs **två uttryckliga armar**: A ett helt nytt dygn med produktion före första måltiden; B det redan påbörjade dygnet med matpass kvar, utan ännu ett produktionspåslag. B är den mer försiktiga30måltidsprognosen från observationen. Skillnaden gör Damon29–30, inte ett falskt exakt datum. Första armen finns ianalysis.json, andra ianalysis-pending.json. Inte en överföring av ticks till realtimmar.

| Stad | 4 matlager | Hjord | Netto/tick | Första civil unmet A/B | Inom30? |
|---|---|---|---|---|---|
| Amnissos | 9315.503 | 50 | 31.648 | >30 / >30 | nej i scenariot |
| Damon | 1385.678 | 50 | -53.073 | 30/29 | JA |
| Gournia | 2930.637 | 0 | -1.46 | >30 / >30 | nej i scenariot |
| Kydonia | 6171.086 | 150 | 14.836 | >30 / >30 | nej i scenariot |
| Lato | 2561.641 | 50 | -25.069 | >30 / >30 | nej i scenariot |
| Petras | 3951.144 | 50 | -43.992 | >30 / >30 | nej i scenariot |
| Phaistos | 3652.309 | 50 | 7.115 | >30 / >30 | nej i scenariot |
| Phalasarna | 3747.956 | 50 | 0.5 | >30 / >30 | nej i scenariot |
| Plateia | 985.315 | 50 | 0 | >30 / >30 | nej i scenariot |
| Pylos | 2825.223 | 150 | 6.683 | >30 / >30 | nej i scenariot |
| Vasiliki | 2261.54 | 50 | -1.85 | >30 / >30 | nej i scenariot |
| Zakros | 11307.453 | 50 | -19.198 | >30 / >30 | nej i scenariot |

Damons vanliga matlager tar slut omkring27–28måltider; hjorden köper ett par måltider till. A-armens första unmet är11,073mat efter30måltider. Det finns137fria gubbar, men simuleringen tilldelar inga nya jobb. Lato och Petras har också tydliga underskott; större lager håller dem utanför30ticks-fönstret. Gournia/Vasiliki har små underskott och mycket större buffert relativt dem. Inga rekommenderade spelåtgärder har utförts.

**Modellgränser:** oförändrad bemanning/terräng/byggnader/population och nuvarande enhetsranson/lön; ingen ny handel, inga byggen, nya rekryteringar, krigsskador eller aktiva spelbeslut. Ingen framtida kultur-/tempeländring från13/15. Förändrad roster och ankomster kan ändra senare skeppsbehov. Modellen låser solden och simulerar inte desertering. Ingen prognos bortom30ticks görs.

### Skepp till sjöss och fysisk betalare

| Skepp | Lönebetalare | Crew | Mat från proviant/tick | Proviant nu | Statisk provianträckvidd dygn | Lön/tick |
|---|---|---|---|---|---|---|
| merchantman | Phaistos | 10 | 0.1 | 1 | 10 | 0.1 |
| galley | Zakros | 6 | 0.06 | 0.18 | 3 | 0.06 |

Dessa proviantbehov ska inte teleporteras till stadsförbrukningen. Statisk räckvidd antar att skeppet förblir ute; det är inte en prognos att dess planerade hemkomst misslyckas. Fulla enhets-ID:n och källor finns ianalysis.json. Tre founder-enheter på land har ingen aktiv upkeep enligt founder_phase; inget fiktivt huvudstadsansvar tilldelas dem. En garnison på42man står i Phaistos men mat/lön belastar Amnissos enligt support_settlement_id: den fysiska placeringen ensam avgör inte betalaren.

## Jämförelse med före-snapshoten

Före: 2026-10-22T17:23:51.022831+00:00, migration198, tick2914. Efter: tick2972,58ticks senare. Befolkning/placeringar kan ha ändrats mellan observationerna; detta är **två observerade tillstånd, inte en isolerad A/B-effekt av regeländringen**. Tabellen jämför samma råmatproduktion och civilbehov i båda, eftersom den gamla fisk-snapshoten saknar en full samtidig armé/proviantbild.

| Stad | Fisk före→efter | Mat före→efter | Civil netto före→efter | Placeringar före→efter |
|---|---|---|---|---|
| Amnissos | 0→0 | 87.933→87.933 | 31.858→31.858 | 112→112 |
| Damon | 82.971→23.12 | 104.637→44.787 | 8.577→-51.273 | 87→55 |
| Gournia | 0→0 | 33.025→33.025 | -0.06→-0.06 | 66→66 |
| Kydonia | 0→0 | 57.091→53.091 | 19.976→15.976 | 74→73 |
| Lato | 63.087→17 | 101.454→56.421 | 20.144→-24.889 | 83→59 |
| Petras | 62.295→15.35 | 76.128→29.183 | 3.453→-43.492 | 71→47 |
| Phaistos | 22.1→5 | 37.547→20.18 | 24.432→7.115 | 26→18 |
| Phalasarna | 10.2→6 | 14.7→10.5 | 4.7→0.5 | 20→18 |
| Plateia | 1→1 | 1.5→1.5 | 0→0 | 1→1 |
| Pylos | 15.4→4 | 28.233→16.833 | 18.233→6.833 | 20→17 |
| Vasiliki | 8.67→7.2 | 11.17→9.7 | -0.38→-1.85 | 14→14 |
| Zakros | 41.334→9 | 76.167→43.857 | 13.157→-19.153 | 109→91 |

Total rå fiskproduktion före 307.057 → efter 87.67. Detta inkluderar automatisk restmat och kan därför skilja sig från ORDER14:s rena placeringsmanifest305→89. Summan av de ursprungliga fiskjobben och dagens placeringar är inte samma population. Löneföre-snapshoten i `../sold/snapshot.json` ochcomparison.json visar Pylos två dåvarande skepp0,75→0,15netto/tick under samma dåvarande roster. Det var ett beräknat behov om betalt, inte en historisk debit. I dagens snapshot är Pylos faktiska debitering0,15; Phaistos tillkomna operativa merchantman och Zakros faktiska6roddare måste räknas från dagens roster.

## Ger säden fortfarande stort överskott på slätt med gård?

**Ja per arbetare och fullt fält, men inte automatiskt för hela staden.** DB-grundtakten på plains är1säd/arbetare/tick. GårdL1 ger1,7 och8platser: full8×1,7=13,6säd mot de800arbetarnas egen konsumtion4, alltså9,6utöver deras mat. L2 har samma8platser men2,4/arbetare:19,2mot4, överskott15,2. Detta är en försörjningskvot, inte ytterligare konsumtion att dra ovanpå tabellens folk.

Observerat: Kydonias två L1-slättgårdar har8arbetare vardera och producerar13,6+13,6; stadens totala matnetto+14,836. Gournia har en L2-slättgård med8arbetare och19,2produktion, men staden totalt−1,46 när resterande folk/armé räknas. Amnissos får+31,648totalt, men dess stora gård står på river_valley, inte plains. Att alla städer med en gård alltid får stort överskott stöds alltså inte av DB.

## Går silvret ihop efter nya lönen?

| Stad | Lager silver | Gruvproduktion/tick | Lönebehov/tick | Netto före övriga köp/offer | Räckvidd lön dygn |
|---|---|---|---|---|---|
| Amnissos | 114.4 | 0 | 0 | 0 | — ingen lön |
| Damon | 0.9 | 0 | 0 | 0 | — ingen lön |
| Gournia | 360 | 0 | 0 | 0 | — ingen lön |
| Kydonia | 484 | 5 | 0 | 5 | — ingen lön |
| Lato | 1.35 | 0 | 0 | 0 | — ingen lön |
| Petras | 0 | 0 | 0 | 0 | — ingen lön |
| Phaistos | 0.05 | 0 | 0.1 | -0.1 | 0.5 |
| Phalasarna | 760.25 | 0 | 0 | 0 | — ingen lön |
| Plateia | 0.2 | 0 | 0 | 0 | — ingen lön |
| Pylos | 852.3 | 0 | 0.15 | -0.15 | 5682 |
| Vasiliki | 0 | 0 | 0 | 0 | — ingen lön |
| Zakros | 891.33 | 0 | 0.06 | -0.06 | 14855.5 |

**Globalt ja, lokalt inte överallt.** Kydonia producerar5silver/tick. Full nuvarande lönesumma är0,31/tick: Pylos0,15, Phaistos0,10, Zakros0,06. Förväntat netto om allt betalas är+4,69. Den senast faktiskt genomförda revisionen tick2971 visar mined_since_last5, net_delta+4,79: bara0,21betalades, eftersom Phaistos inte kunde betala0,10. Ingen återcirkulation har lagts till.

Phaistos har0,05silver, ingen gruva och behöver0,10för merchantman med10Crew ute. **UpkeepSettled id2310660, tick2971**: units_unpaid1, silver_unpaid0,1, silver_gross0, silver_destroyed0; enheten har unpaid_periods1. Detta är redan inträffad lokal insolvens, inte bara en prognos. Den får inte döljas av rikets/världens andra kassor eller avrundas till att0,05räcker en dag. Pylos kassa räcker statiskt5682dygn för nuvarande0,15lön, Zakros14855,5 för0,06. Framtida byggen, gåvor, handel och ändrad roster kan förkorta dessa; ingen kassa poolas.

## Reproduktion och verifiering

Från repo-roten: `python3 docs/reviews/matbalans-efter-fiske/capture.py` tar **ny** read-only-snapshot och skriver omhealthz/live-snapshot. För att reproducera denna rapport använd de redan sparade filerna och kör `analyze.py`, `PENDING_MEAL=1 python3 .../analyze.py`, `verify.py`, `render.py`. Capture kräver SSH tillroot@10.0.1.92 och använder samma fasta DB-åtkomst som befintliga alfa-verktygen. Inga driftvariabler eller .env läses in.

`verify.py`: provenance/read-only/migration,60oberoende produktionsrater,12gubbepooler, verklig senaste upkeep mot rosterbehov (gross+unpaid), nollcirkulation, massbalans i båda simuleringsarmarna, endastDamon≤30 och faktisk obetald lön iPhaistos. Alla gröna. Ingen full Go/JS-svit körd: ingen spelkod ändrad. Ingen mutation av produktionskod behövs för denna läsrapport; prognosens massbalans och två fasarmar prövar analysen.

Ägare/regelreferenser påd248b09e: economy/hex_rules.go,placement_yield.go,recompute.go,automatic_food.go,food_basket.go,food_tick.go,herd_births.go,ship_ration.go; combat/upkeep.go; kharis/temple_offering.go; api/handlers/settlement_placement.go. Råa DB-rader, modell och tabeller är bevisen, inte framtida UX-löften. **Rapport + STOPP; ingen balansändring, merge, push eller deploy.**
