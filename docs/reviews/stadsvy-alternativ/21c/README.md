# 21c — stadens storlek från gatan

## SLICE-KONTRAKT
Problem: 21b:s 1000 och10000 såg för lika ut från gatan; några fler hus/figurer räckte inte.
Spelarsanning: en liten stad har öppna luckor och odling, en stor stad sammanhängande massor i flera djupplan; båda upplevs som en stadsmiljö med horisont.
Invariant: samma referensbyggnader, mur1, oförändrad kamera inom respektive grepp; pixelraster och synlig berg-/himmelsfond.
Scope: tre procedurkompositioner×population1000/4500/10000, PNG/logisk/×2/×4, jämförelsegalleri och bevis i denna katalog.
Non-scope: live-renderer, nya byggnader/mekanik, karta, balans eller bindande grafikregler.
Acceptans:9bilder, tre faktiskt olika storleksbärare; större urbana massor bortom/kring templet; gles småstad med odlingsluckor; skarp320×192passform; Firefox1:1/gråskala.
Stopvillkor: hash+STOPP, Timothy avgör om storleken läses bättre.
Bevisplan: samma parameterdata och kamera, struktur utan figurer/rök, determinism/exaktzoom, faktisk390px/desktopFirefox, kalibrerat geometriprov och mutationer. Pixeldiff bevisar beroende, inte mänsklig urskiljning.
Kedjegrinden: väntar till efter. Bas c9947e5b; Claude08:42 beställer21c efter22, som nu är överlämnad c99794d1.

## SAMMANFATTNING

[Nio nya bilder](index.html): tre storleksgrepp ×1000/4500/10000 invånare, mur1.160×96logiska pixlar, exakt×2spelskala320×192 och×4detaljbild. De tidigare gallerierna och alla tillstånd bevaras. 21c är fortfarande ett bildförslag, inte en implementation i den levande stadsvyn.

- **Glesa gårdar → stadsklyfta:** odlingsluckor och fristående bostäder blir sammanhängande, högre gatufasader; bakom dessa byggs flera takplan.
- **Staden klättrar i terrasser:** gatan går upp mot templet och bebyggelsen breddas från ett till fem terrassplan.
- **Staden fortsätter bortom templet:** förgrunden har samma mänskliga skala, medan kvarteren fortsätter på sluttningen bakom helgedomen i flera djupplan.

Den varma bandade bergsfonden följer Timothys03-val. Ljus och kamera är fasta inom respektive grepp. Skillnaden finns också i strukturarket utan människor och härdrök; samma figurantal kan därför inte ensam förklara bilden.

## EXPERIMENT OCH MÅTT

| Grepp |1000: bostadspixlar|4500|10000|Vad ändras?|
|---|---:|---:|---:|---|
| Gatuklyfta |1170|4722|7579|Luckor → sammanhängande fasader; två/fyra bakre takplan.|
| Terrasser |832|2867|4232|Ett/tre/fem plan och fler kopplade kvarter.|
| Bortom templet |1150|2828|4966|Liten fri sluttning → en vidare stadsdel i två/fyra plan.|

**Vad måttet mäter:** en separat binär rastermask följer det faktiska målandet av bostäder; senare målade icke-bostadsobjekt tar bort skymda pixlar. Figuren/röken ingår inte. Masken visar den strukturella bostadsmassans synliga projektion, inte verklig golvyta eller antal invånare. Nonzero-pixlar räknas uttryckligen, eftersom Pillowläge1 kan lämna1 i minnet och255 efter PNG-inläsning. Återinläsning kontrollerar samma antal; ingen obekräftad division med255 används.

**Vad måttet inte mäter:** att en människa identifierar stadsstorleken snabbt. Den enda numeriska grinden är att större indata faktiskt ger större bostadsmassa och tre olika strukturraster. Ingen godtycklig procentgräns utges för perceptuell läsbarhet. [Kontaktarket](kontaktark.png), [gråskalan](graskala.png) och [strukturarket utan liv](struktur-kontaktark.png) låter Timothy bedöma det som siffran inte kan avgöra.

Byggarens bildgranskning upptäckte fel målningsordning i två bakre stadsdelar: avlägsna tak målades efter de närmare och gav ett svävande intryck. Planen ritas nu bakifrån och fram. Gatuklyftan framhäver rummet mest, terrasserna gör storleksbytet mest uttryckligt, medan den bakre stadsdelen behåller öppnare förgrund.

## GRINDAR

| Grind | Bevis |
|---|---|
| Artifakter |57genereradePNG byte-identiska mellan körningar;9scener,18exakta skalpar,9exakta struktur+liv-kompositer. |
| Databeroende |Varje grepp har växande bostadsmassa och tre skilda strukturbilder utan figurer/rök. Mur1och samma funktionella referensdata i alla bilder. |
| Horisont |Byggargranskad synlig berg-/himmelsfond. En fast bergspixel(80,11) förblir samma genom populationerna, en kalibrerad sanitycheck som **inte** bevisar hela horisontens läsbarhet. |
| Visuell/användare |Firefox146.0.1,DPR1,1440och390px:9spelbilder exakt320×192CSS, gråskala/logisk1:1/×4 och matris-scroll fungerar, ingen pageerror eller horisontell sidöverströmning. Bildgranskning i spelstorlek/logisk1:1/gråskala. |
| Mutationer |Tre fysiska: lås populationen, byt NEARESTmotBILINEAR, måla över bergsöppningen i största gatuklyftan. Alla namngivet RÖDA→byte-exakt restore→nygenererat GRÖNT. |
| Semantik |Befolkningsband bestämmer massor och rum, inte ett hus per invånare. Fält i det lilla utsnittet är miljö/negativt rum, inte nya mekaniska produktionshexar eller rivna jordbruksplaceringar när staden växer. |
| Drift |Ingen merge/push/deploy, APIändring, servermätning eller ändring i levande stadsvy. BILD till Timothy genom Claude. |

[Verification](verification.json), [mutationer](mutation-results.json), [data/maskantal](manifest.json). Inga hälsosvar eller migrationsnummer påstås för offlinebildstudien.

```sh
python3 docs/reviews/stadsvy-alternativ/21c/generate.py
python3 docs/reviews/stadsvy-alternativ/21c/mutation_check.py
python3 docs/reviews/stadsvy-alternativ/21c/verify.py
```

Kräver samma Pillow/DejaVuSans/PlaywrightFirefox som21b. Den fristående generatorn importerar tidigare rasterverktyg men ändrar inga äldre bilder. Huvudgalleriets generator uppdateras endast med en länk hit; dess gamla artifakt-/browser-/mutationsprov körs igen.

## METODISK LÄRDOM

När stadens population ska synas från gatan behöver den ändra mängden byggd massa och rummets öppna ytor. Fler figurer eller några fönster kan ge liv men ersätter inte ett strukturellt storleksbyte.

## KÄNDA AVGRÄNSNINGAR

Bilderna är tre parameterstyrda prototyper för en specifik referensstad, inte en allmän generator för varje terräng/kultur/byggnad. Endast normalbild och mur1prövas här; status och andra murar finns i21b och är inte påstått integrerade i21c. Gård2/tempel1/hamn/varv/gruva finns i referensdata men kan skymmas av gatans förgrund och växande bostadsmassa; ingen full byggnadsavläsning lovas. Odlingsluckornas synlighet är komposition, inte förändrad produktion.

Masken kan avgöra om ritad bostadsmassa växer, inte om mängden motsvarar exakt1000eller10000invånare. Ingen mänsklig snabbidentifieringsstudie finns. Upplevd skala, horisont och platskänsla avgörs i BILD; ordet"grön" gäller tekniska kontroller och byggargranskning, inte Timothys godkännande.
