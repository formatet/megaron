# Order27 — blodröd fara och varmgrå sekundärtext

## SAMMANFATTNING

Fara använder nu befintlig blood-red; diplomati behåller sin lila accent.
Nedtonad text använder #615D56 enligt Claudes avgränsning19:14.
Produktion **70775387**, från master584a324a. Bara två CSS-token ändras.
667JS gröna, tre fysiska mutationer RED→byte-exaktrestore→GREEN; Firefox
före/efter390+1280 och OSlight/dark. Två rena naturliga världar C/D på
process70775387, migration189. Ingen merge/push/deploy; Claude granskar BILD.

## SLICE-KONTRAKT

Problem: fara och diplomati signalerar lila, sekundärtext orangebrunt.
Spelarsanning: blodröd fara och lugn varmgrå metadata enligt godkänd riktningB.
Invariant: inga ord, speldata, geometrier eller order ändras; diplomati kvarlila.
Scope: två CSS-token, palettprov och Firefoxbevis.
Non-scope: nytt tema, charcoal-kontrast, server/keryx/Codex och spelregler.
Acceptans: danger=blood; dim=#615D56; minst4.5:1 mot papyrus/raised/warm-white;
mutationRED och667JS; riktiga City/Economy/Kult/War iFirefox390/1280.
Stopp: fast färg kontra kontrastkonflikt togs till Claude och avgränsades19:14.
Bevisplan: oförändradJS-bas, kontraktsRED, CSS-inneslutning, mutationer,
selectorbilder och två rena register→CLIlogin/join/found→Firefoxvärldar.
Kedjegrind: **bevisar** läsbar produktion och tydlig farostatus.

## BASLINJE / EXPERIMENT

- Master584a324a, oförändrad fullJS GREEN (`baseline-js.log`). Nya värdeprov
  namedRED mot tidigare danger/dim (`contract-red.log`).
- Ursprungliga beslutsbildens#67635C gav4.3494:1 påbg-raised och föll kontrastprovet.
  Claude valde#615D56 och avgränsade charcoal; WIP99430356 är inte slutkod.
- nativeA visar ursprunglig grå. nativeB är diagnostik: godkändfunktion men
  fysisk CSS-mutation startade innan browserkörningen säkert var avslutad.
  B räknas därför inte som slutbevis. Mutationerna avslutades och återställdes
  före de separata, rena C/D-körningarna. Ingen produktionsfix behövdes.
- Första verify.py anropade re.sub med saknat stringargument; riggfel rättat
  före slutkörningen, ingen produktionsändring.

## GRINDAR

| Grind | Resultat |
|---|---|
| Kod |667/667JS GREEN (`final-js.log`). Ingen Go-/SQL-produktionsändring; inget Go-testsvep krävs. Native bygger riktiga server/CLI-binärer. |
| Mutation | `verify.py`: gammal lilafara, gammal loam-dim och förljusgrå namedRED→byte-exaktrestore→GREEN; mutations.json bär exakt CSS-hash. |
| Inneslutning | Hela CSS byte-identisk med584a324a när enbart de två tokenraderna tas bort; inga mallar eller andra produktionsfiler ändras. |
| Visuell | Firefox DPR1,390/1280,light/dark; statiska bilder identiska vid omtagning; geometrier och diplomati identiska före/efter;0pageerror/overflow (`browser.json`). |
| Användare | NativeC/D: två färska PG189/Redis/servervärldar, register→CLIlogin/join/found/city/status och faktiska City/Economy/Kult/War före/efter390/1280. Healthz70775387/189;0pageerror/overflow, ingaSQL-skrivningar eller API-replay. |
| Semantik | Samma DOM/data i båda färgarmarna. Före återställer endast tidigare danger/dim med CSS. Sällsynt kollaps/faroknapp demonstreras i tydligt statiskt selectorprov; ingen kollaps påstås ha spelats naturligt. |
| BILD/drift | Efterbilder inspekterade1:1; Claude bedömer om resultatet matchar godkänd riktning enligt ordern. Ingen merge/push/deploy. |

Kontrast använder sRGB relativ luminans, kalibrerat i palette.test.mjs med
svart/vitt21 och lika färger1. Måttet beskriver färgkontrast, inte textbegriplighet.
Faktiska kvoter i `contrast.json`: papyrus5.4498, raised4.7664,
warm-white#FDFEFE6.4781; charcoal2.6037 (utanför scope).

## KÄNDA AVGRÄNSNINGAR — mörka ytor

Ingen separat mörk temapalett finns; båda OS-preferenserna använder samma token
(`native-c/color-schemes.json`, ävenD). Ingen kontexttoken införs enligt Claude.
Följande mörka dimtextytor lämnas som särskilt dokumenterad kontrastskuld:

- `.sr-category` i sökfönstrets kategori-, laddnings- och felrader har explicit
  charcoalbakgrund och dimtext (`megaron.css:1711`, `ui/search.js`).
- Sidfoten i base.html är dimtext direkt på bodycharcoal utanför kartläget.
- CSS för `.no-kingdom p` och `.dispossessed-page p` har dimtext utan egen ljus
  yta och skulle ärva mörk body; dessa äldre regler är inte bevisat aktiva routes.
  Generiska `.loading`/`.empty-state` behöver också en ljus förälder; de riktiga
  panelerna i denna slice har det. Ingen allmän granskning av alla overlaylägen görs.

Nativebilderna visar metadata, inte nya verkliga kollaps-/krigshändelser.
Övrig färganvändning av dim/danger (kanter/fyllning) följer respektive token
som beslutat; CSS-geometrin ändras inte.

## REPRODUCERA

`node --experimental-default-type=module --test web/static/js/megaron/palette.test.mjs`

`python3 docs/reviews/palett-beslut/verify.py` (avsluta mutationer före native)

`python3 docs/reviews/palett-beslut/native.py docs/reviews/palett-beslut/native-c`

Bilder: [galleri](index.html), `native-c/`, `native-d/`, `selector-*`.
