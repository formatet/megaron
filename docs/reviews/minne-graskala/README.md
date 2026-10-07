# Minnesrutor i gråskala — BILD

Minnesrutor hade samma färg som aktuell syn. Nu avfärgas hela remembered-hexen
med bevarad ljushet och terrängdetalj; live och fog behåller sin presentation.
Gren `codex/minne-graskala`, bas `222f4e3f`, renderkod `4545fc81`.
Överlämnas för Claudes granskning/BILD; ingen merge, push eller deploy från Codex.

## Kontrakt före kod
Problem: minnesrutor har samma färg som aktuell syn. Spelarsanning: svart = aldrig sett, gråskala = minne, färg = aktuell syn. Bevisar geografidelen av kedjegrinden.
Invariant: endast serverns remembered avfärgas; ljushet och detaljer bevaras, inga server-/tier-/FOV-ändringar.
Scope: kartpass, canvas-hjälpare, JS-prov, webbrigg och BILD. Non-scope: synregler, fog-palett och nya förmågor.
Acceptans: remembered grå och live/fog oförändrade; hård pixelkant; terräng/byggnader grå före UI/enheter; inspect fungerar; riktig scouting/återresa med alla tre tier i desktop/390-bild.
Stopvillkor: informationsmodellkonflikt till Claude, ingen serverutvidgning.
Bevisplan: oförändrad master222f4e3f, tier-test/fysisk mutation, hela JS/färsk Go/vet, två isolerade scoutresor utan SQL-ingrepp och pixelmätning. Claude bedömer BILD.

## Baslinje
400 JS gröna på oförändrad master222f4e3f (logs/baseline-js.log). Ingen separat minimap: hex-canvas är kartan, city-scene är stadsbyggnadsvy.

## Bevis och grindar

| Grind | Resultat |
|---|---|
| Kod | **404 JS**, hela `tools/gotest.sh` på färsk PG16/mig160 (world 89.988s) och ren `go vet ./...` gröna. Server/keryx-diff tom. |
| Mutation | Fysisk tier-guard, saturation och borttagen faktisk kartpass-anrop: namngivet rött → återställt grönt, `mutations/`. |
| Användare | Registrera/join/grunda → Explore fjorton speldygn → enheten hemma. Oförändrad baseline och två färska efter-armar. Inga SQL-fixturer/ingrepp, inga browserfel. |
| Semantik | Serverns tier är enda urval. Främmande enheters befintliga live-gate orörd. Hover och vänsterklick-inspect på faktiskt minne fungerade i andra efter-armen. Ingen separat minimap; stadspanelens city-scene orörd. Kodex sight förklarar färg/gråskala/svart. |
| Visuell/BILD | Desktop/390px visar alla tre tier samtidigt, även minzoom 0.3. Identiska frysta frames deterministiska. Claude bedömer bilderna enligt Timothys delegation. |
| Drift | Ej deployad av Codex. Health ur de faktiska isolerade processerna nedan. |

Provenance ur `/healthz`: `live-baseline` **222f4e3f/mig160**,
`live-after` och `live-repeat` **4545fc81/mig160**, alla status ok.
API-kartor, orderkvitto, slutenhet och mätningar ligger i respektive `proof.json`.

- Börja med `live-baseline/mobile.png`, `live-after/mobile.png` och `live-repeat/mobile.png`;
  dessa är olika färska världar. `live-repeat/mobile-inspect.png` visar faktiskt minnes-inspect.
- **Exakt samma verkliga kartpayload** i gammal/ny renderer: `replay/before-*` ↔ `replay/after-*`.
  Desktop zoom1, mobil .75 och minzoom .3: **0 ändrade pixlar utanför minnesmasken** och
  **0 färgpixlar i masken**, 133–147 grånivåer, ljushetsfel högst .5/255 på opaka pixlar.
  Gråpasset kostade median .1–.6 ms i denna lilla verkliga vy; ingen storvärldsprestanda hävdas.
- Verkliga minnescentra i andra resan: **69/109/162**, aldrig sedda centra **28**.
  Detta mäter synlig terrängton på hexcentra, inte mörka enskilda objektpixlar eller mänsklig igenkänning.
  Det mörkaste centrat skiljer 41/255 från fog; bilderna är BILD-beviset för läsbarheten.
- `collisions/` är en **syntetisk renderer-matris**, ingen spel-/serverfixtur:
  hav, land, båda skogar, berg, flodfamilj, städer och egen enhet. Samma containment och full
  avmättnad; 172–194 grånivåer vid desktop/mobil. Namn är tomma i mätmatrisen eftersom UI
  avsiktligt ritas ovanpå gråpasset. Verkliga UI-bilder ovan har vanliga namn.

## Experiment och metod

Avfärgningspasset använder saturation med noll färgmättnad efter terräng och remembered-byggnader,
före markeringar, namn och aktörer. Live-städernas gamla lagerordning är kvar.
Device-pixel-spann ger en hård kant också vid fraktionell zoom/pan.

Tidiga riggmätningar förkastades: Explore-forecast är avsiktligt unavailable; riggen måste prova
riktiga Explore-order. Första baslinjeservern läste löpande ändrade webbfiler under scoutresan;
slutbaslinjen kördes om på egen färsk DB med arkiverade, oförändrade 222f4e3f-assets.
Resize måste hinna klarna och följas av en faktisk omritning före bilden.

Pixelsmasken använder canvasens faktiska `getTransform()`: Chromium avrundar .6-skalan,
så en separat double-beräkning felklassade två gränspixlar. Opaka pixlar är ljushetsmåttet;
redan delvis transparenta polygonkanter blir opaka av saturation-fyllningen (max RGB-ljushetsavvikelse
1.1/255 i verklig replay, 3.84/255 i syntetisk minzoom). Ingen färg finns kvar där.

## Avgränsningar och reproduktion

Ingen faktisk främmande stad/enhet behövdes eller skapades i scoutingvärldarna;
remembered-staden provas i renderer-matrisen. Överhängande berg/träd behåller sin befintliga form
utanför hexen; avfärgningen gäller själva remembered-polygonen. UI-texter/markeringar behåller färg.
Recept: `tools/README.md` → Minnesgråskala. Primär acceptans är riktig webb, replay är pixelgrind.
