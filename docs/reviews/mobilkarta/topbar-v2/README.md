# U — nåbar topprad, tillägg på samma gren

Bas `b19a2479`, kontrakt före kod: [topbar-contract.md](../topbar-contract.md).
Vid 390px slutade sökknappen på x432 och hjälpknappen på x468. Browsern
klippte dem trots att dokumentet inte hade horisontell scroll.

Mobil-CSS reserverar 60% av raden för kalender + de tre knapparna. Kalenderns
båge blir 40×25 och månadstexten får krympa med ellips, fortfarande 13px;
full text finns kvar i DOM och exakt datum i Notifications. Namn/nätstatus
kan krympa när de konkurrerar om resten av raden. Desktop-CSS, kalenderns
beräkningar, knapparnas handlers och U:s input-modul ändras inte.

`after/proof.json`: **90 layoutfall**, 270 knapp-rect/hit-kontroller,
Firefox först sedan Chromium/WebKit, desktop1280×900 och touch390×844.
Originalbildens kalender-fixtur, alla 13 riktiga månadsnamn (via misc.js),
långt mänskligt Wanax-namn och synlig nätstatus. Varje knapp ligger helt
inom viewporten, dess mitt träffar knappen och scrollX är 0. Vid 390 ligger
knapparnas högra kanter på **316, 351, 386 px** i alla motorer.

Verkliga native tap på mobil (mouse click desktop) öppnar Notifications,
sök och Codex via original-HTML:s onclick och main.js:s riktiga bryggor.
Riggen importerar main.js med enbart start-IIFE borttagen: dess bootstrap
ersätts av befintlig State/HTTP-fixtur, dess kontroller kopieras aldrig.
GET-svaren är explicita; Notifications markerar fixture-notiser lästa via
sin vanliga POST read-all. Ingen DB, live-server eller analytics berörs.
Codex Welcome laddas, inga page errors eller oväntade API-rutter.
Browserbegränsningarna för kartgester kvarstår enligt [U-rapporten](../README.md).

**Fysisk mutation:** ta bort mobilens gt-celestial-reservation ur verklig CSS.
`mutation/red.log`: namngivet rött **U topbar buttons are within viewport and
reachable at 390**; `mutation/browser/proof.json` visar knappar utanför
viewport. Återställning byte-identisk, SHA256 i `mutation/proof.json`.
Därefter samtliga sex toppradsprov gröna (`after.log`), **531/531 JS** utan
skips (`js.log`), och sex fulla kart-/gester-/Codex-prov gröna
(`gesture-regression.log`). Desktop Firefoxs fem före/efterbilder är
**byte-identiska** (`desktop-pixel-parity.json`).

Nya bilder i denna mapp; b19-bilderna är bevarade. Namn: Agamemnon, Nestor,
Mycenae, Tiryns, Bronze Guard. Fryst browserdatum, en-GB/Europe/Stockholm,
externa fonter blockerade lika före/efter. Granskade Firefox-bilder vid 1:1:

- [Före, 390px](baseline/firefox-mobile-header.png)
- [Efter, tre knappar](after/firefox-mobile-header.png)
- [Långt riktigt månadsnamn](after/firefox-mobile-shadow-days.png)
- [Notiser via ☍](after/firefox-mobile-notifications.png)
- [Sök via ⌕](after/firefox-mobile-search.png)
- [Codex via ?](after/firefox-mobile-help.png)

Reproduktion:

```sh
python3 tools/mobilkarta_topbar_mutation.py
python3 tools/mobilkarta_browser.py --topbar-only --output docs/reviews/mobilkarta/topbar-v2/after
python3 tools/mobilkarta_browser.py --output docs/reviews/mobilkarta/topbar-v2/gesture-regression
```

`source-sha256.json` gäller tilläggets slutkällor. Ingen mall, server, Keryx,
kanonändring eller migration; inget mallorsakat omstartskrav. BILD väntar
Timothy före merge. Day-grenen orörd. Ingen merge/push/deploy; hash och STOPP.
