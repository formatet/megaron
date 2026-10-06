# Stadsvy: bebodda gårdar

Problem: den låga scenen visar platta symboler som Timothy inte känner igen som verksamheterna.
Spelarsanning: se stadens byggnader och pågående bygge som begripliga arbetsplatser.
Invariant: bara payloadens verksamheter; nivå, kö och mur bevaras.
Scope: city.js, citybuildings.js, stadsvyns CSS och befintliga bildverktyg.
Non-scope: karta, server, balans, nya verb eller kulturkanon.
Acceptans: volym och arbetsredskap; sammanhängande gårdar; alla typer får plats; mobil fungerar; byggkö syns.
Stopvillkor: ändring i spelmodell eller kulturkanon krävs.
Bevis: samma fem payloadfall före/efter, riktig drawer, JS-svit. BILD-förslag till Timothy.
Kedjegrind: väntar till efter; särskilt beställd grafikrunda.
Förlaga: img/stadsvy.jpeg (terrasser, ljus puts, gårdar, pithoi och trappor), vaultens akhaiska platta tak.

## Resultat

Bildförslag på `codex/stadsvy-mysig`, implementation `9741ce05`, inte publicerat.
Tidigare iteration utgick främst från befintliga stämplar och vaultregler; stadsvy.jpeg kom in sent.
Nu: större scen, tredimensionella hus, verksamheternas redskap, bostadsgårdar, trappor och grönska.

- [Liten stad](after/small.png), [utbyggd stad](after/equipped.png), [före](before/equipped.png), [riktig drawer med fixtur](drawer/01a-city-drawer.png).
- 360 JS-tester gröna; fem frusna stadsfall pixelidentiska mellan två körningar.
- Samtliga 39 typ/nivå-kombinationers pixlar ryms i rapporterad byggnadsbredd.
- Riktig lokal PG16/Redis/server: registrering → anslutning → grundning → City-drawer, desktop och mobil. Inga browserfel, mobilcanvas inom 390 px.
- Runtime `/healthz`: `9741ce0`, migration 158, status ok. Första kontrollen avbröts före registrering eftersom förväntat commit-id angavs med åtta tecken medan servern rapporterar sju; korrigerad till faktisk proveniens.
- Kartans renderer och serverregler oförändrade. Mobilbilderna visar grundad stad; de utbyggda bildfallen använder uttryckliga fixturer, inte uppspelad ekonomi.
- BILD: Timothy bedömer mysighet och igenkänning. Förslaget ligger kvar på grenen för jämförelse; ännu ingen merge/deploy av denna iteration.

Avgränsning: procedurritad pixelgrafik i faktisk UI-skala, inte förlagans illustrationsdetalj. Aktivitetsfigurer är atmosfär, inte en avbildning av bemanningsantal.
