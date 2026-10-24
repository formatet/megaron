# BILD — byggplan 2026-10-24

**22 slutbilder i [index.html](index.html)**, native Firefox 1280×900 och390×844, från oförändrad produktionskälla **3c984c18 / migration210 /1800s per tick**. Endast bevisverktyg, rapport och bilder ändrade; ingen produktionskod, merge, push eller deploy. Timothys BILD-dom återstår.

| Bilder | Visar |
|---|---|
| A1/A2 och A4/A5 | Myrines faktiskt levererade brev till Leandros; Reply-knapp → riktigt klick → hennes tråd och svarsruta |
| A3/A6 | Potnias Offer med28av Latos29korn; en måltid återstår inte |
| C1/C2 | Synlig rest host, Dorian; faktisk POST409 → varning + March anyway; ordern bekräftas inte |
| D1/D1m | City → Food: storm över en bemannad fiskehex, ingen fisk där medan den står |
| D2a–d / D2e–h | Faktisk Potnia-bön, RiteAnswered via riktig WebSocket, bilder vid cirka0,3/1/2/4sekunder |
| D2i/j | Hela omenförloppets slut: minnesgrått |
| G1/G2 | Faktiska byggPOST201-kvittenser vid1800s/tick:8game days/about4hours/klart; mobil4game days/about2hours/klart |

**Provenance:** [gallery/proof.json](gallery/proof.json), [fixtur](gallery/fixture.sql), [slutlig källkontroll](final-source-verification.json).910produktionsfiler hashades före/efter varje komplett körning och stämmer fortfarande med slutkällan. Alla22slutbilder har granskats i native upplösning.0pageerrors på båda bredderna.

Fixtur: egen nyskapad PG/Redis/server. Vanlig registrering/join/grundning; läsbara Wanaxnamn Leandros/Myrine/Dorian och städer Lato/Gournia. Egna DB-fixturer flyttar Host/eskort till en sammanhängande kustscen, ger tempel/varor/kharis, en synlig rest host, och en normal sammanhängande trehexstorm med bara en bemannad fiskehex. Brevet skickas via HTTP och levereras av den riktiga workern efter att dess eget timerjobb görs förfallet. Dispatchen öppnas med klientens riktiga renderer och det sparade payloadet; Reply klickas på riktigt. Marschpanelen öppnas genom appens controller; varningen kommer från verklig server409. För D2 finns ett bestämt okäntsilverfynd; cooldown/scouting återställs endast som scenförvillkor mellan bredderna. Inga SQL-ingrepp sker under bön/WS/animation, och ingen HTTP/WS- eller renderer-mock används.

**D2-tider:** räknas från den riktiga `State.omen.start`; före/efter varje native screenshot finns i proof.json. Nuvarande Potnia v2 visar19hex,300ms förskjutna, med1500ms grönt och900ms stegvis blekning. Därför finns ännu sena gröna hex vid4s; full minnesgrå bild tas efter faktisk slutpunkt (minst7,8s). Ingen animation har snabbspolats eller frysts. En transparent `AudioContext.createOscillator`/`frequency.setValueAtTime`-observatör såg146,8/587,3/622,3/587,3Hz i varje D2-förlopp: playOmen-signaturen anropades. Detta är inte en bedömning av hur ljudet låter.

Tre kompletta nyskapade världskörningar finns i `final`, `final-repeat` och `gallery`, med samma910produktionsbytes och0pageerrors. **Endast `gallery` är valt bildpaket:** de tidigare körningarna fick efter manuell bildgranskning ytterligare väntan på panelövergången och precis scrollning för mobilens Food-rad. Tidiga försök är sparade som råunderlag: positionen ligger på units, inte founder_phase; dispatchrenderern måste importeras från modulen; Food måste inväntas efter drawer-fetch. En tidig enhexstorm ersattes med den riktiga trehexmodellen. De tidigare bilderna är inte Timothys granskningspaket.

Reproducera med `python3 tools/byggplan_bild_20261024.py OUTPUT`; bygg granskningssidan med `python3 tools/byggplan_bild_gallery_20261024.py`. Alla egna processer/containrar städas i finally. Den delade megaron-acc-riggen, kharis-konton och produktion är orörda. Nya Go/JS-sviter eller produktionsmutationer är inte relevanta för denna bildleverans; dess skydd är oförändrad produktionskälla och riktiga nativeflöden.

Granskningssidan är också öppnad i Firefox: alla22bildfiler laddar, breddfiltren visar11bilder vardera och sidan ger0scriptfel (`gallery-check.json`). De valda bildfilernas SHA256 står i `SHA256SUMS`. `git diff 3c984c18 -- server web` är tom; en produktionsmutation är därför inte tillämplig på denna dokumentleverans.
