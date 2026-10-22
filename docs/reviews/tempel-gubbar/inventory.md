# ORDER 13 — inventering före kod

Bas `ea97b332` efter ORDER12 LIVE. Gren `codex/tempel-gubbar`, egen worktree
`/home/tk/wt/codex-tempel-gubbar`. **Ingen produktionskod eller migration skriven.
Ingen livedata ändrad. Väntar på Claude ok.**

Beslutat: gamla procentregeln bort; två faktiska tempelgubbar per nivå ur samma
arbetsstyrka som övriga placeringar. Devotion är antal placerade tempelgubbar,
inte en andel eller en extra arbetsstyrka. Fyra ytor och BILD före merge.

## Två uttryckliga godkännanden behövs

1. **Vad händer vid fullt placerad arbetsstyrka i övergången?** Den föreslagna
   avrundningen ger två gubbar i alla åtta befintliga tempelstäder. Tre städer
   saknar fria gubbar; sex nuvarande jobb måste flyttas om alla åtta ska behålla
   bemanning. Alternativ och uppmätta data nedan. Rekommendation: lediga först,
   därefter uttryckligt godkänd deterministisk omplacering från icke-matjobb.
2. **Bekräfta kharis-talets enhetsbyte.** Idag är gain `devotion_andel × 14`.
   Att bokstavligt ersätta andelen med gubbeantal ger L1 med två gubbar
   **28** före offer/scarcity/börda, mot dagens **2,1**. En gubbe ger14. Detta
   följer orderns ”formeln ska läsa antalet” om koefficienten behålls, men är
   en faktor13⅓ vid full L1. Jag föreslår att du uttryckligen bekräftar denna
   följd med Timothy innan kod. Att i stället bevara gamla fulla L1-gain skulle
   behöva ett separat beslutat tal1,05/gubbe. Inget nytt balanstal väljs här.

## Hela nuvarande kedjan

| Ägare / konsument | Faktiskt nuläge och ändring som behövs |
|---|---|
| settlement_labor | Enda levande vikt är cult. PUT LaborAlloc skriver andel, 15%-golv, tak0,15×nivå. KH1 bevarar andelen när klienten utelämnar den. Ny modell behöver inga vikter eller procentbudgetar. |
| kharis/tick.go | templeDevotionCapacity / TempleDevotionPerLevel=0,15; tickens SQL summerar min(vikt,0,15×nivå); processMaintenance multiplicerar med kharisPerTempleDay=14. Ska läsa placerat antal från gemensam ägare. |
| kharis/project.go | Egen nästan identisk SQL för devotionSum och capacity; samma gain och offerprojektion. Båda SQL-kopiorna måste ersättas, inte bara debit-sidan. |
| province.go GET | devotion/devotion_capacity läses separat ur labor/buildings och skickas som floatandelar. Ska bli uttryckliga heltalsfält, t.ex. temple_staffed/temple_capacity. Inga procenttolkningar får leva kvar i klienter. |
| province.go Goods / db.go | Gamla laborWeights, cult/LaborCapacity-gren och procent-/employedberäkningar. Radera cult-viktläsningen när tabellen försvinner; övrig varuproduktion är redan placements. |
| settlement.go PlacementRoster | Ska visa tempelplaceringar som andra byggnadsarbetare. Dagens kommentar säger uttryckligen att cult inte finns där. Roster/City/API måste räkna dem i placerade/fria. |
| LoadBuildingProductionOptions | Inner-joinar production_rules; temple saknas eftersom cult inte är producerad vara. Behöver verklig staffing-only tempeloption, cap2×nivå, utan att återinföra cult som lagrad/producerad vara. |
| PlaceGubbe / PlacementOptions | Validerar idag RatePerGood>0 och PlaceCapPerGood (som för industri ofta är L1-cap, INTE2×nivå). Tempel måste få verklig levelberoende platstak och semantisk arbetsroll som inte kräver positiv varuproduktion. Befintliga ägar-/populationslås, ordinals och hela arbetskraftspoolen återanvänds. |
| RecomputeProduction | Ska räkna tempelgubbar som upptagna i poolen men aldrig tillverka cult-stock, varurate eller silver. Behåll goods.cult, weight0: nya settlement_placement.good_key har också FK dit. |
| Grundning | **Metropolisfilen skriver faktiskt ingen cult0,15 idag och grundar inget gratis tempel**; kontrollen hittar bara kolonivägen unit_arrival.go:1994 som INSERT cult0,15. Ta bort den gamla skrivvägen. Nygrundad/bebyggd stad får inte gratis fiktiv tempelbemanning; inget automatiskt tempel/gudval genom ORDER13. |
| Livscykel | Demolish lyfter redan byggnadstypens placeringar. PrunePlacementsToPopulation prioriterar redan mat före cult före andra jobb; cult finns där som hittills oanvänd prioritet. Kontrollera befolkningsfall/rekrytering, tempelrivning och eventuella nivåsänkningar så inget jobb står över cap eller på ett borttaget tempel. Recompute/backfill/reconcile får inte återställa procentvikter eller autoplacera tempel igen. |
| Offer | loadTempleOffers väljer tempelstäder och lokal gåva, inte devotion. Varje tempels stående offer krävs idag även om bemanningen är noll; föreslår bevara detta, inte införa en ny offergrind genom ORDER13. |
| Riter / capabilities | Faktiska Rite/canRite-grindar använder tempel, kharis, gåva/cooldown; ingen direkt devotion-andelsgrind hittad. Föreslår lämna deras spelregel orörd. Noll bemanning ger inget löpande gain, inte ett nytt förbud mot bön. |

**Föreslagen EN ägare:** economy äger tempelns arbetsplatser (`2 × level`) och
läsning av verkliga giltiga cult-placeringar. Samma staffing-snapshot per stad/
rike används av tick, ProjectDailyNet och API. Inget economy→kharis-beroende.
Kharis äger fortsatt hur antal omvandlas till gain, offer och decay. Sätt inte
capformeln separat i SQL hos tre konsumenter. Inför inte en falsk production_rule
eller dummy-cult-rate för att passera RatePerGood>0.

## Live, bara läst — vad en migration faktiskt skulle möta

`tools/alfa_db.py`, varje fråga i `BEGIN READ ONLY`; data i `live-readonly.json`,
beräknad omräkning i `transition-analysis.json`, SELECT i `queries.sql`.
Värld2178e795… tick2838, **2026-10-22 16:07 UTC** (exakt DB-tid i JSON).
Tolv icke-terminala städer, åtta med tempel, alla dessa L1. Alla tolv har
cult_weight0,15. Totalt13 legacy-cult-rader inklusive en terminal/ej inkluderad
stad; inga legacyrader för andra varor. Inga cult-production_rules finns.

Omräkningsförslag att godkänna:
`target = min(2×nivå, floor(population/100), max(1, round(vikt×population/100)))`
för vikt>0 och verkligt tempel; annars0. Hela gubbar, inga nya personer.
Gamla obesatta överandelar ska inte ge större tak än2×nivå. Migrationens
ordning/policy måste vara deterministisk och omkörning ofarlig.

| Stad | Befolkning | Tempel | Andel | Placerade / gubbar | Lediga | Måltal | Saknas |
|---|---:|---:|---:|---|---:|---:|---:|
| Amnissos | 11215 | 1 | 0,15 | 112/112 | 0 | 2 | 2 |
| Damon | 19212 | 1 | 0,15 | 87/192 | 105 | 2 | 0 |
| Gournia | 6617 | 1 | 0,15 | 66/66 | 0 | 2 | 2 |
| Kydonia | 7423 | 1 | 0,15 | 74/74 | 0 | 2 | 2 |
| Lato | 16262 | 1 | 0,15 | 83/162 | 79 | 2 | 0 |
| Petras | 14535 | 1 | 0,15 | 71/145 | 74 | 2 | 0 |
| Vasiliki | 2310 | 1 | 0,15 | 14/23 | 9 | 2 | 0 |
| Zakros | 12622 | 1 | 0,15 | 109/126 | 17 | 2 | 0 |
| Phaistos | 2623 | 0 | 0,15 | 26/26 | 0 | 0 | 0 |
| Phalasarna | 2000 | 0 | 0,15 | 20/20 | 0 | 0 | 0 |
| Plateia | 300 | 0 | 0,15 | 1/3 | 2 | 0 | 0 |
| Pylos | 2000 | 0 | 0,15 | 20/20 | 0 | 0 | 0 |

- **A: bara lediga.** Fem tempelstäder kan fyllas (10 gubbar); tre blir utan
  bemanning. Inga jobb flyttas, men deras gamla positiva devotion går förlorad.
- **B: lediga först, flytta sedan sex icke-matjobb. Rekommenderas efter ok.**
  Två vardera i Amnissos/Gournia/Kydonia; de har gott om icke-matplaceringar.
  Följ befintlig pruningprioritet och högsta ordinal deterministiskt; håll
  matjobben kvar. Ger16tempelgubbar. Produktionsbortfall är verkligt och ska
  redovisas i torrkörningsmanifestet; inget additivt dubbelarbete eller överfull
  ordinalbudget. Om en annan stad vid faktisk migration bara har matjobb får
  migrationen inte gissa ny policy; avbryt/rapportera eller använd separat
  uttryckligt godkänt fallback.
- **C: lämna vikterna som kompatibilitet.** Avråds: behåller två sanningar och
  kringgår kravet att tempel konkurrerar om samma hela arbetare.

**Ingen av dessa migrationer är körd.** Data kan ändras innan merge/deploy;
bygg och visa ett nytt read-only förhandsmanifest före integrationen, inklusive
exakt vilka jobb som i alternativB skulle flyttas. Ingen tempelbemanning för
städer utan tempel bara för att gammalt grundningsgolv0,15 finns kvar.

## Gammal operation och schema — rekommendation

Ta bort route `PUT .../labor` och LaborAlloc, inte lämna en levande no-op.
Ta bort Keryx `allocate --cult` och dess read-only procentvy; `staff temple cult`
(eller motsvarande tydlig arbetsroll) och `place`/`unplace` tar över. Kontroll
med sökning: allocate-hjälparna ligger i cmd_allocate.go och saknar andra
produktionsanropare, utom registreringen i main.go. Använd CLI-hjälp som pekar
på staff/city i ändrad dokumentation; ingen procentskrivande alias.

Efter migrerade placeringar: radera settlement_labor och dess nu döda
kodläsare. **Behåll goods-raden cult med weight0**, nu FK-ankare för riktiga
placeringar; den får inte bli handels-/lagervara. Historiska LaborAllocated
behåller gamla weightpayloads och tolkas inte om till antal; inga nya
antalpayloads under samma eventtyp. Placements befintliga mutationer återanvänds.
CLAUDE.md:s ”måste behållas, permanent hem” är gammal regel som denna uttryckliga
order ersätter; projektinstruktionerna och kult-vaultens äldre KH1-status behöver
uppdateras i samma slice så nästa agent inte bygger tillbaka den gamla vägen.

## Fyra ytor, Umami och bildplan

- **Webb:** viktig inventeringsrättning: procentinput ligger i
  `ui/drawers/city.js` (city-devotion-sec, saveLaborAlloc), **inte kult.js**.
  Ta bort blocket, exporten/windowbridgen i main.js och eventuell död CSS/DOM.
  citygrid får faktiska2×nivå-tempelplatser med +1/−1 och heltalsbemanning.
  Visa kult som tjänst/devotion, inte ”producerar0cult/dag”. Kult-drawerns net,
  idle och offer läser samma bemanning/prognos; en tydlig väg till stadens
  tempelbemanning behövs. Ingen separat ny procentkontroll i Kult.
- **Keryx:** cmd_staff.go har explicit specialavslag ”cult isn't a placed
  citizen — allocate instead”; det ska bort. city/roster/placements och staff
  visar rätt kapacitet och lediga ordinals. Status/net/idle visar antal, inte
  procent; avveckla allocate-texter och de gamla vikthjälparna.
- **Temenos:** befintlig POST/DELETE placements med lås/folkbudget; ny
  staffing-only option och samma härledda räkning i tick/project/GET/roster.
- **Codex:** temples.md (hela procentavsnittet), kharis.md, citizens.md,
  keryx.md och eventuella hänvisningar till allocate/15% uppdateras gemensamt.
  Verblistan: labor-raden tas bort, staff/place/unplace anger temple-cult.
- **Umami:** verb_telemetry.js tar bort labor/cult_allocated-routen. Riktiga
  tempelbyggnadsplaceringar ger workplace_staffed, borttagning placement_removed.
  Den aktuella good-whitelisten saknar cult: lägg till som tillåten arbetsroll
  för place/staff, inte ett nytt löfte om skeppbar cult för transfer.
  Verifiera mot verbregistret och dess tester. Inga gamla lyckade labor-events
  ska förfalskas till staffing i mätningen.
- **BILD före merge:** Firefox390(has_touch,tap) och1000, samma faktiska
  map.html/CSS/Kult/City/citygrid. Visa L1tom/1/2, fullt tempel som avvisar tredje,
  L2 med4platser, otillräcklig fri gubbe, −1→+1→Kultnet, långa namn/scroll/header.
  Två rena körningar, färdig CSS-transition och verkligt inläst innehåll,
  determinism och SHA. Mobiltryck på plats och återgång till Kult ska fungera;
  tidigare ORDER11 mobilpanelägare måste respekteras. Baslinje före kod.

## Funktionella bevis efter ok

Namngivet RED på gamla cap-/andelsmodellen → GREEN; fysisk mutation som läser
legacyvikt eller lämnar bemanning utanför populationbudget ska bli RED.
Testa 2×nivå-tak, 0/1/2/4gubbar, ingen cultproduktion/handel, faktisk
arbetskraftskonkurrens, unplace/rivning/populationsfall och borttagen laborväg.
Tick och ProjectDailyNet måste matcha för samma staffing/offers, inklusive
0bemanning och delvis matade tempel. Migration testas mot tom DB och representativ
gammal modell med både lediga och fullplacerade städer, deterministiskt och utan
dubbla ordinals. Ny grundning får inga fantomvikter. Fyra ytor och Umami.
Full fresh Go en gång på57900, JS och vet efter färdig implementation.

**Inväntar godkännande av omplaceringspolicy och kharis-koefficient/enhet.
Inga kodändringar, DDL eller andra liveskrivningar gjorda.**
