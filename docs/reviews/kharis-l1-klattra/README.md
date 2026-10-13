# Skött nivå1-tempel klättrar

Timothy2026-10-13: ja till svagt positivt netto enligt rekommendationen.
Bas7053ac2d, migration179. Produktion ännu oförändrad vid kontraktet.

| Fält | Kontrakt |
|---|---|
| Problem | Matat fullt L1 ger0,15×8−1,8=−0,6/tick och kan inte ta relationen från golvet till tidiga böner. |
| Spelarsanning | Ett skött första tempel låter relationen långsamt växa utan nivå2/ceder. |
| Invariant | Endast kultutbytet ändras. Grundförfall, bemanningskapacitet, matgate, scarcity, golv, tak, bönekostnader och väntetider består. |
| Scope | kharis utbyteskonstant, målkurvprov/anpassade äldre balansprov och Codexordalydelse. Ingen migration eller ny visuell familj. |
| Non-scope | Ceder, metaller, AI, andra religionsgrindar eller befintliga eventpayloads. |
| Acceptans | L1netto0<net≤0,5 utan scarcity; golv→tidig bönegräns inom15ticks; L2/L3 snabbare men respektive tak består; omatat/tempellöst faller; prognos=faktiskt tick. |
| Stopvillkor | Annan regel behöver ändras eller målkurvan inte kan uppnås via utbytet. |
| Bevisplan | Färsk baslinje, faktisk DB-underhållskurva förL1/L2/L3, fysisk återgång8→namedRED→byteåterställning→freshGREEN, fullfreshGo/vet/JS, två naturliga byggen i isoleradserver+Keryx/Firefox. |

Kedjegrind: blockerar tidiga böner efter första tempelbygget.
Målval: kharisPerTempleDay14 ger L1+0,3,L2+2,4,L3+4,5 vid fullbemanning
utan scarcity. Golv1→5 vidtick14, golv1→50 vidtick164; L2golv1→75 vidtick31,
L3golv1→100 i storleksordningen22–23ticks; faktisk DB når taktick23. Ingen böneanvändning eller expansion i dessa referenskurvor.
Gamla8 ger L1−0,6,L2+0,6,L3+1,8; detta dokumenterar högre tempels följd.


## Bevis

Produktion4083c54e. Enda mekaniska ändringen är utbyteskonstant8→14.
Äldre tester som låste negativtL1 uppdateras efter Timothys nya beslut; nya
faktiska180-tickskurvor perL1/L2/L3 bevisar netton+0,3/+2,4/+4,5, första
bönegräns vid14/2/1 och tak50/75/100 vid164/31/23. Prognos=varje faktiskt
underhållsutfall, omatat har fortfarande−1,8, L1tak under blessings.
Testerna ger namedRED med gamla8, fysisk mutation14→8 ger samma RED, exakt
byteåterställning följd av färskGREEN. Första restore-riggen fick en portkrock;
logg bevarad, omkört båda armar med ledig fast GOTEST_PORT57411. Ingen
produktionsändring för riggfelet eller dold skip.

Full färskGo179/world94,278s, vet och620JS gröna. Codex12/12 hos klientagenten.
Naturlig registrering/join/grundning, gubbarproducerat byggmaterial, byggAPI
och riktig worker: fullt L1 visar cirka+0,3, relationen passerar5 och riktig
Vision-bön tas emot (200, fyra kharis debiteras; slumpat utfall kan misslyckas).
Keryx/nativeFirefox1280/390, inga SQL-fixturer eller svarsoverrides. Bilder
granskade1:1: nettoraden ochgåvor läsbara. Andra renakörning i native-repeat/.
Den första omkörningen fick legitimt409 vid grundning på en annan startplats;
avslaget gällde ett redan använt stadsnamn. Riggen rättad till unika namn
för kandidater och fortsätter genom vanlig registrering/join, inte bypass
eller SQL. Misslyckade riggförsök sparade i separata kataloger.

```sh
GOTEST_PORT=57411 python3 docs/reviews/kharis-l1-klattra/mutate.py
python3 docs/reviews/kharis-l1-klattra/live.py
L1_NATIVE_OUT=native-repeat python3 docs/reviews/kharis-l1-klattra/live.py
tools/gotest.sh
```

Avgränsning: högre tempel får snabbare klättring samtidigt, enligt beslutets
val att justera kultutbytet; kurvorna redovisas uttryckligt. Inga gamla
eventpayloads/kolonibelastning/tak/templekostnader ändras. Bildfrågorna
godkända av Timothy13/10; ingen ny visuell familj. Driftkvitto efterdeploy
sparas i agentloggen och .agents/kharis-l1-live-final.json.
