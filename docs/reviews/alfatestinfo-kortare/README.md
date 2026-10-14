# Alfatestinfo kortare

| Fält | Kontrakt |
|---|---|
| Problem | Presentationssidan är för lång och bilderna visar äldre grafik. |
| Spelarsanning | En kort svensk inbjudan visar vad Megaron och alfan är, verkliga aktuella vyer och vägen till konkret feedback. |
| Invariant | Publik utan login; inga uppdiktade funktioner, tidsregler eller bildtexter; andra sidor/visuella familjer orörda. |
| Scope | alfatestinfo.html, fyra befintliga screenshotfiler, en scoped underrubriksregel i megaron.css, relevanta publicitet/bild-/policyöversättningsprov, bevisrapport. |
| Acceptans | Ungefär halverat synligt ordantal; svensk varm ton; 4 riktiga Firefox1440×1000-bilder; sida på1280/390 utan horisontell overflow; färska Go/JS + fysisk bildmutation gröna. |
| Stopvillkor | Kanonfrågor parkeras i BESLUT, oberoende arbete fortsätter. BILD och rolländring via Claude19:39: utföraren lämnar bilder + HASH + STOPP före merge; Claude granskar, gör egen mutation och äger merge/push/deploy. |
| Bevisplan | Oförändrad public-route-baslinje + ordantal; read-only clone av acceptansens riktiga DB, normal migrationsstartup, aktuell server/assets; Firefox1:1; mutation saknad bildreferens rött→exakt återställt grönt; full färsk Go/JS. |

Kedjegrind: **blockerar** begriplig inbjudan före alfatestet. Ingen ny spelmekanik.
Den äldre :8097-processen kör94101f6/mig174 och används endast som READ ONLY-källa till en separat klon.
Dess container/data ändras inte. Tre befintliga världar och Eirenes speldata behålls i kopian.


## Sammanfattning och status

**Färdig branch, HASH + STOPP före merge.** Bas efter rebase `3e49ed1d` (endast CLAUDE.md ändrades sedan8a3a2769),
kärndiff `6ccbf8fd`, kontrastfix `87fbb9db`, policyprecision `f53a8e11` och bemannad produktionsbild `d401372a`. Claude äger granskning, egen mutation, merge/push/deploy enligt19:39.
Ingen produktionsdata eller delad acceptanscontainer ändrad.

Synligt ordantal från riktig Firefox DOM: **1043→525**, **−49,66%**, identiskt på1280 och390.
Sex korta sektioner, svenska, inga ”ticks”. Tid/rörelse/mat/gudar/bud/bronsregler pekas på via öppningsdispatch/Codex;
alfa, vad vi söker feedback på, Report/keryx report och befintlig chattpolicy finns kvar.
Chattreglerna och integritetsuppgifterna är översatta, inte ändrade; JS-vakten behåller samtliga fyra regler och
åtta integritetspåståenden över språkgränsen till den engelska spel-Codexen.

## Baslinje och provenance

`master-baseline.log`: verklig publik route utan auth/DB och registrar-vakt på oförändrad8a3a2769, separat färskPG16/mig179.
`before/verification.json`: processhealth8a3a2769/mig179 och1043 ord i Firefox1280/390.
Den delade :8097-riggen kör94101f6/mig174 och innehåller tre världar. `proof.py` tar **READ ONLY pg_dump** till
privat temporär PG16/Redis och startar aktuell server med normal174→179-migrering; källcontainrarna lämnas orörda.
Den komprimerade DB-dumpen lagras inte i repo/artifakter; endast dess hash/provenance sparas.

`sources/`: verkliga game-vyer från Eirene (WanaxLykoros).
Bland de tre befintliga världarna valdes `Fardvag1588ebaf` med två ägda städer och verklig korrespondens;
nyaste Offer-världen har inga brev. Valstatistik i `sources/game-views.json`.
Inloggningen går via riktig auth-API med orderns credential. Slutriggen gör **ingen** lösenordsreset, inga
speldata-fixturer, inga HTTP/payload-overrides. Bara login, visa drawers och avfärda brief/chips via vanliga UI-knappar.

## Bildtexternas underlag

| Bild | Vad som faktiskt visas | Kod/regel |
|---|---|---|
| karta.png | Karta kring Mycenae/Knossos/Tyre, känd/minnesmark, hav, enheter och färdvägar | main.js/render, verklig FOW-data från vald snapshotvärld. |
| stad.png | Buildings, ”No buildings yet” och konstruktionsmeny | ui/drawers/city.js, dtab byggnader och refreshCityBuildings. Bildtexten lovar inga befintliga byggnader. |
| produktion.png | Naturligt grundad Mycenae:4/10 placerade,600 fria, growing/+2,3 matnetto; först3 auto och+0,6, sedan ett faktiskt+1-UI-klick | ui/drawers/city.js/citygrid.js; verklig register/join/found och placering, inga direkta SQL-skrivningar. |
| handel.png | Diplomacy/Correspondence, ett verkligt arkiverat brev frånKnossos | ui/drawers/diplomacy.js, faktisk inbox/sent-data. Ingen skickad ny handel eller möjlig reply utlovas. |

Alla fyra nya web-assets är **Firefox1440×1000**, tagna i den riktiga spelvyn, ingen bildredigering. Tre är från Fardvag-snapshoten; den slutliga produktionsbilden är från en fräsch naturlig grundning. Sidans bildnot säger därför separata testvärldar.
Gamla PNG-filer var olika beskurna storlekar trots mallens1440×1000-attribut; nya är verkligen den angivna storleken.

## Experiment och grindar

| Grind/prov | Resultat |
|---|---|
| Kod | Full färsk Go/mig179 grön (world150,984s), vet grön, 623JS gröna utan skip. |
| HTTP/publicitet | TestAlfatestInfoPublic + TestMainUsesPublicAlfatestInfoRegistrar gröna utan auth/DB; alla4 referenser och riktigPNG1440×1000 valideras. |
| Mutation | Saknad produktion.png-referens → namedRED TestAlfatestInfoPublic → identiska mallbytes återställda → nytt grönprov på separat färskPG16. |
| Semantik | Samma feedbackväg, etablerad chattpolicy, sanningsenliga bildtexter och hjälppekare; ingen mekanik eller annan sida ändras. |
| Visuell | Firefox1280×900/390×844, 1:1 granskade; ingen horisontell overflow, alla4 bilder laddade/dekodade. Underrubrikens ärvda ljusgula färg upptäcktes och rättades enbart i alpha-info-scope med CSS-variabler. |
| Användare | Publik sida utan cookie; riktig Eirene-login och vanlig växling City→Buildings/Production och Diplomacy. |
| Upprepning | Två rena slutkörningar (`after/`, `after-repeat/`) på separata snapshotkloner, lika ordantal/layoutmått, inga pageerrors. |
| BILD | Paket lämnas till Claude för hans 1:1-granskning och Timothys samlade runda. Ingen merge från utföraren. |
| Drift | Ej deployad. Underlag för deployreproduktion finns; enbart mall/static/test ändrade, ingen migration/base.html. |

## Riggfel som rättades

Första bilddecode kördes innan lazy-bilden laddat: provet väntar nu på verklig load/decode.
Det första auth-anropet använde fel fältnamn (`username` i stället för `username_or_email`);401 var ett riggfel,
inte belägg om stale lösenord. En onödig lösenordsreset hann prövas i en borttagen privat klon; den är borttagen ur
slutverktyget och används inte i sources/after/after-repeat-bevisen. Källcontainern berördes aldrig.
En privat PG-restore träffade initprocessens avstängning efter UNIX-pg_isready; verktyget väntar nu på TCP-ready,
vilket visar den slutliga postmaster-processen. BILD-provet tog först en tom nyaste testvärld; slutprovet väljer
verklig korrespondens bland Eirenes världar och dokumenterar valet.

## Reproduktion och granskning

```sh
ALPHA_PROOF_MODE=before python3 docs/reviews/alfatestinfo-kortare/proof.py
ALPHA_PROOF_MODE=sources python3 docs/reviews/alfatestinfo-kortare/proof.py
ALPHA_PROOF_MODE=after python3 docs/reviews/alfatestinfo-kortare/proof.py
ALPHA_PROOF_MODE=after ALPHA_PROOF_OUT=after-repeat python3 docs/reviews/alfatestinfo-kortare/proof.py
python3 docs/reviews/alfatestinfo-kortare/mutate.py
tools/gotest.sh
node --test $(rg --files web/static -g '*.test.mjs')
```

Granska sidan: `after-repeat/desktop.png`, `after-repeat/mobile.png` (hela sidan), respektive `*-top.png` (viewport1:1).
Gamebildkällor: `sources/karta.png`, `sources/stad.png`, `sources/produktion.png`, `sources/handel.png`.
`comparison.json` innehåller ordantal, processprovenance, mått och hashes för levererade web-assets.
Full Go kördes före dokument-only rebase och den sista copy-preciseringen. En tom
`git diff 9400caf2..f53a8e11 -- server` bevisar identiska Go-källor/tester. Publik-HTTP-proven kördes dessutom
på separat färskPG16 efter sista copy-ändringen (`final-public.log`); alla623JS kördes på sista versionen.
Efter den bemannade bildens korrigering svarar båda sista rena public-page-processerna medd401372a/mig179; docscommiten ändrar ingen produktion.

## Kända avgränsningar

Screenshotkällan är den befintliga acceptansens testdata, inte en naturlig nyspelad bronskedja. Ord-/layoutmått
bevisar korthet och responsivitet, inte en främlings förståelse; TEXT hör hemma i playtest.


## Claudes BILD-korrigering före merge (2026-10-14 20:00)

En obemannad SQL-fixtur är ett dåligt första intryck för den normala nygrundningen. **Endast produktion.png**
togs om; karta/stad/handel är byteidentiska med de godkända filerna (`approved-assets-preserved.json`).
Fardvag-världen är arkiverad och rätt placerings-UI nekas av servern; den aktiva Offer-fixturen på kullar kunde
inte mata1200 även när alla12 gubbar placerats. Ingen arkivvakt/produktion/silver/cap ändrades för bilden.

Slutbilden kommer från en **fräsch normal acceptance-server** på egen tomPG16/Redis, aktuell kod och vanlig
migrationsstartup1→179: Eirene register→join→found Mycenae via riktiga gameplay-API:er. Grundningen auto-placerade3
arbetare och gav+0,6 matnetto/growing. Ett **riktigt citygrid+1-klick** (polygonval och knapp, faktisk POST-respons)
gav4/10 arbetare,600 fria och+2,3 matnetto/growing. `production-ui/game-views.json` innehåller före/efter-DTO och
UI-åtgärden; `production-ui/verification.json` anger natural_fresh_foundation=true. Ingen DB-fixtur eller
lösenordsreset används. Den delade acceptansen är fortfarande orörd.

Repro för denna bild:
```sh
ALPHA_PROOF_MODE=production ALPHA_PROOF_OUT=production-ui ALPHA_PROOF_FRESH=1 python3 docs/reviews/alfatestinfo-kortare/proof.py
```

Efter bildbytet: ny fysisk bildreferensmutation påd401372a RED→exakta bytes→freshGREEN, ny public-HTTP på färskPG16,
623JS gröna, samt två rena public-page-körningar med alla fyra verkliga bilder laddade. Go-källor/tester är fortfarande
byteidentiska med den fulla Go-körningen; ingen ny Go-mekanik. Grundningsfyndet triageras separat i vault/todo.

**FYND-bedömning:** gamla Fardvag-staden skapas med explicit SQL i `tools/fardvag_fixture.py:30`, utan grundning,
placements eller normal goods-init. Normal createMetropolis anropar PlaceStartingWorkforce + RecomputeProduction i
sammaTX. Vid tillgänglig mat-slot och≥100pop garanterar placeStartingGubbar minst en arbetare. Naturligt exempel
bevisar nu auto3 och positiv mat. 0 efter normal grundning kan däremot inträffa om inga mat-slots finns; hjälparen
returnerarsufficient=false och createMetropolis accepterar detta/visar lagerprognosen. Detta är en separat edge att
verifiera, **inte** bevis om en nyspelarregression och ingen ny balans-/grundningsregel i bild-slicen.
