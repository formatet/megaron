# Stadsvyalternativ — slice 21

## SAMMANFATTNING
Tolv olika bildgrepp för samma Phaistos-lika stad, i ett fristående galleri. Två preliminära favoriter får fyra tillstånd vardera. BILD väntar på Timothy; ingen produktionsgrafik eller vaultregel ändras. Bas: c761626b. Ingen spelserver körs och därför finns ingen healthz-/migrationsproveniens; detta är en syntetisk bildstudie, inte spelacceptans.

## SLICE-KONTRAKT
| Fält | Innehåll |
|---|---|
| Problem | Tidigare stadsvyer upplevs platta och ger för få verkligt olika val. |
| Spelarsanning | Storlek, mur, funktioner och mående ska kunna läsas i en liten stadsscen. |
| Invariant | Hela pixlar; samma referensdata; separata statuslager; bronsåldersmaterial. |
| Scope | Endast denna katalog: generator, PNG, galleri och regelprover. |
| Non-scope | Levande renderer, server, API, balans och bindande grafikregler. |
| Acceptans | ≥8 grundgrepp; två favoriter × fyra tillstånd; logisk PNG + exakt ×4; beskrivningar; Firefox vid 1:1. |
| Stopvillkor | Leverera hash + STOPP; Timothy väljer bildriktning före eventuell produktionsslice. |
| Bevisplan | Generera två gånger och jämför bytes; kontrollera ×4, data och lager; Firefox desktop/mobil samt gråskalekontaktark; granska bilder. |

Kedjegrinden: **väntar till efter**. Bildstudien föreslår hur den befintliga stadspanelen kan göra stadens tillstånd begripligt; inga nya spelhandlingar införs.

## BASLINJE
Oförändrad master är c761626b; tidigare renderer har logiskt 160×76 och ×2-visning. Den läses som bakgrund, återanvänds inte som kompositionsmall. Alla nya scener är offline-fixturer med 4 500 invånare, mur 1, tempel 1, gård 2, hamn 1, varv 1 och gruva 1. Normalt finns inget bygge. Generatordata är gemensam för alla; miljö och kamera är bildgrepp, inte nya speldata.

## EXPERIMENT

Tolv kompositioner, varav 03 medvetet återanvänder 01:s geometri som ett kontrollerat palettprov. Övriga skiljer kamera, massfördelning, rum eller bildgrammatik. Beskrivningarna i [galleriet](index.html) anger respektive idé, regelprov och möjlig koppling till speldata. [Regelförslagen](regler.md) är granskningsmaterial, inte nya bindande regler.

01 och 05 är preliminära favoriter och finns normalt, svältande, belägrade och under tempelbygge. Struktur och status sparas också separat. Vid byggarens bildgranskning upptäcktes att ett belägringsläger i dioramat hamnade på vatten; lägren flyttades till de två yttre landkanterna och hela generering/Firefox-kontrollen kördes om.

## GRINDAR

| Grind | Utfall |
|---|---|
| Kod/artifakter | 52 genererade PNG identiska mellan två körningar; 18 scenpar exakt närmaste-granne ×4. Ingen produktionskod ändrad, så Go-/JS-spelsviter eller testdatabas är inte relevanta för denna slice. |
| Visuell | Byggaren granskade kontaktark i logisk 1:1, förstoringar, gråskala och statustillstånd. Firefox 146.0.1 vid DPR 1, 1440 px och 390 px: 20 logiska galleriinstanser i exakt CSS-storlek, fungerande gråskala/×4, ingen sidöverströmning eller pageerror. Timothy bedömer BILD före en eventuell produktionsändring. |
| Semantisk | Gemensam referensdata för alla; inga uppfunna funktionsbyggnader. Åtta statusbilder återskapas exakt som struktur + transparent statuslager. Ingen ny mekanik, ingen FOW eller liveuppgift. |
| Användare | Offline: öppna galleriet → jämför tolv alternativ → öppna alla ×4 → växla gråskala → jämför två favoriter i fyra tillstånd. Detta är ett faktiskt browserflöde men ingen registrering eller spelacceptans. |
| Drift | Ej tillämplig; ingen merge, push eller deploy. |

Fysiska mutationer: byt `NEAREST` till `BILINEAR` → verifieraren blir RÖD; ta bort konstruktionens statuslager → RÖD eftersom byggbilden blir identisk med normal. Byte-exakt återställning och ny generering ger GRÖNT efter båda. Se [mutation-results.json](mutation-results.json) och [verification.json](verification.json).

### Reproduktion

```sh
python3 docs/reviews/stadsvy-alternativ/generate.py
python3 docs/reviews/stadsvy-alternativ/mutation_check.py
python3 docs/reviews/stadsvy-alternativ/verify.py
python3 -m http.server 8765 --directory /tmp/megaron-codex-stadsvy-alternativ-20261018
```

Öppna `http://127.0.0.1:8765/docs/reviews/stadsvy-alternativ/index.html` i Firefox vid 100 %. Generatorn kräver Pillow (använt 12.3.0) och DejaVu Sans (`DejaVuSans.ttf`, för svenska rubrikglyfer; hård rastertext utan kantutjämning); browserkontrollen kräver Python Playwright med Firefox. Efter clone/worktree på annan plats ändras bara serverkommandots katalog. Generator och verifierare hittar roten själva.

## METODISK LÄRDOM

- Kamera och fördelning av stora massor ger mer variation än att lägga ornament på samma layout.
- Skilj kompositionsprov från palettprov: 01→03 visar exakt vad färgen tillför.
- Statustestet behöver ett geografiskt ankare också för saker utanför muren; en generell skärmposition kan sätta lägret i havet.

## KÄNDA AVGRÄNSNINGAR

- Detta är fasta kompositionsprototyper för en enda referensstad, inte en färdig renderer för godtyckliga data. Murnivåer 0/2/3, andra populationer, byggnadsnivåer och kulturer är inte verifierade. Beskrivningarna anger hur kopplingen kan byggas efter bildvalet; de påstår inte att den redan finns.
- Templet har platt minoisk tak-/pelarform. Ingen fristående palatsbyggnad tillförs. Gårdens nivå 2 ger flera odlingsrader; ingen byggnad per invånare ritas.
- Bilderna har 192×112 logiska pixlar, förutom panoramat 240×96 och skymningsprofilen 224×112. De visar samma data men olika utsnitt och luft. En eventuell panelintegration måste välja storlek och heltalszoom: ×2 för 192 kräver 384 px **inre** bredd, mer än en 390 px-panel med normala marginaler. Galleriet visar logisk 1:1 och ×4 enligt briefen; det bevisar inte passform i den levande stadspanelen.
- Husen är fortfarande förenklade och flera bilder använder samma byggnadsfamilj. 03 är ett kontrollerat färgprov, inte ett ytterligare kameragrepp. 12 varierar kvartersriktning mer än de tidigare kuberna.
- Svält läses huvudsakligen genom frånvaro av liv/härdar och stängd matplats; det kan vara för svagt utan jämförelsebild. Stadens exakta storlek och vissa funktioner läses svagare i 04. Mänsklig snabbidentifiering av byggnader eller status är ännu inte testad.
- Baslinjebilden kör den oförändrade renderern med en syntetisk fixtur. Inga API-/DB-bevis eller påståenden om live-Phaistos ingår.

Granskningsrättelse efter första handoff: kontaktarkets standardfont ersatt med explicit DejaVu Sans för å/ä/ö. Motivens PNG är byte-identiska; bara kontaktarken och deras hashvärden ändras.
