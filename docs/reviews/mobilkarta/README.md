# U — mobilkartan

Kontrakt före kod, bas `c093fc11`, gren `codex/mobilkarta`.
Blockerar huvudgrinden: geografin kan inte undersökas med fingret. Provar
samma karta och samma orderväg för mus, finger och penna. BILD väntar Timothy.

En Pointer Events-väg: drag panorerar, kort tryck väljer hex, två fingrar
zoomar runt mittpunkten med befintliga kameragränser, stilla långtryck (500 ms)
öppnar den befintliga högerklicksvägen. Avbrutna gester gör inga val. Hjul,
högerklick, mus-tooltip och tangentbord behålls. Bara canvas får touch-action:none;
lådor behåller vanlig rullning. Kontroller dokumenteras i Codex. Inget server-/Keryxarbete.

390-menyn får två rader med tre knappar: alla sex namn och den befintliga
13px-textstorleken behålls. Kartans höjd och lådornas botten följer menyn.
Desktop behåller en rad. Baslinje och bilder nedan använder mänskliga namn,
produktions-CSS och riktiga kart-/marschkontroller med explicit, skrivskyddad HTTP-fixtur.

Testmetod: Firefox först, därefter Chromium/WebKit, desktop 1280×900 och touch
390×844. Playwright stöder has_touch i alla tre; is_mobile saknas i Firefox.
Riktiga touchscreen.tap och musprov körs i alla. Programmerade Pointer Events
provar samma drag/nyp/långtryck i alla. Chromium CDP ger därutöver verkliga
flerfingerhändelser och provar webbläsarens scroll/cancel-beteende vid CSS-mutationen.
Det är browseremulering, inte prov på en fysisk telefon eller ett live-spel.

## Resultat

Baslinje: 519/519 JS gröna. `baseline/proof.json` visar kameran oförändrad
efter touchdrag i alla sex kombinationer. Chromium-proben rullar sidan
100/125px i stället. Vid 390 sitter DIPLOMACY och ECONOMY utan vänster
textmarginal; provet kräver minst 4px på båda sidor. Alla sex namn klarar det
efter ändringen. Basens bilder finns i `baseline/*-baseline.png`.

Efter: 531/531 JS, inga skips. De befintliga Host-/Inspect-/inlinegrundningsproven
använder nu Pointer Events och behåller sina innehålls-, FOW-, sena-svar- och
POST-assertioner. Tolv nya prov täcker mus/finger/penna, gestens ursprung,
långtryck, rörelse tillbaka till start, två fingrar, kamera-gränser, avbrott,
förlorad capture, blur/hidden-cleanup, tooltip, högerklick och hjul.
Drag slutar nu utan det gamla oavsiktliga hexvalet: tidigare jämförde mouseup
med sista mousemove, inte med gestens ursprung. Capture gör att släpp utanför
canvas avslutar gesten; focus/visibility och pointercancel rensar också.

`browser/proof.json`: sex kombinationer gröna, inga page errors eller oväntade
API-rutter. Alla HTTP-anrop är GET med samma authväg; ingen spelorder skickas.
Drag: kamera (150,250) → (190,310). Nyp: zoom 1 → 1.6 och samma världsposition
under mittpunkten. Tryck/klick öppnar Tiryns och visar Wanax Nestor; långtryck/
högerklick öppnar samma Tiryns-meny med Bronze Guard från Mycenae och färdig
serverfixtur för ankomstprognosen. Chromium native: drag −140px, scrollY 0,
nyp 1 → 1.8, långtryck öppnar menyn; alla inspelade touch-pointerhändelser är
trusted. Codex-panelens native touchscroll flyttar innehållet 125px.

Browsergränser redovisas uttryckligt: Firefox has_touch gör att Playwrights
native mushändelser inte genererar mouse Pointer Events (minimal repro
bekräftad); native mus provas därför i separat desktop-context utan touch.
Mobilens Firefox-musprov använder programmerade Pointer Events. WebKit saknar
Playwright mouse.wheel i is_mobile-context; där provas en WheelEvent, medan
desktop använder riktigt hjul. Programmerat drag/nyp/långtryck bevisar
handlerkedjan, inte operativsystemets gesthantering. Native Chromium kompletterar
med det senare. Telefonhårdvara och live-spel har inte provats.

## Mutation och återställning

`python3 tools/mobilkarta_mutations.py` redovisar fyra fysiska källmutationer:

| Mutation | Namngivet rött |
|---|---|
| Ta bort canvas touch-action | U native touch drag keeps page still and moves camera |
| Ta bort dragets consumed-spärr | U motion cancels long press even if finger returns to its start |
| Ta bort långtryckets consumed-spärr | U long touch/pen opens orders once, release never selects… |
| Ersätt nypfaktorn med 1 | U pinch uses moving midpoint… |

CSS-mutationen ger scrollY 125 och trusted pointercancel; kameran når bara
−30px innan browsern tar över. Scrollproben gör dokumentet tillfälligt rullbart
med en separat testspacer för att mäta browserns gestägande; spelets normala
body är overflow:hidden. Det är en avsiktlig browserprov-fixtur, inte ändrad
produkt-CSS. Varje källfil återställs byte för byte i finally. Hashar och
namngivna fel: `mutations/proof.json`, full grön svit: `mutations/restored-js.log`.
Slutlig browserkörning använder återställda källor.

## BILD och reproduktion

Firefox först, 1:1, 1280×900 och 390×844. Trettio efterbilder i `browser/`:
`*-overview.png` (jämför bas och meny), `*-map.png` (nyp), `*-tap.png`,
`*-orders.png`, `*-controls.png`. Exempel för Timothys BILD:

- [Firefox mobil, före](baseline/firefox-mobile-baseline.png)
- [Firefox mobil, efter](browser/firefox-mobile-overview.png)
- [Firefox mobil, tryck](browser/firefox-mobile-tap.png)
- [Firefox mobil, långtryck](browser/firefox-mobile-orders.png)
- [Firefox mobil, kontroller](browser/firefox-mobile-controls.png)
- [Firefox desktop](browser/firefox-desktop-overview.png)

Mänskliga namn: Agamemnon, Nestor, Mycenae, Tiryns, Bronze Guard. Datum är
fryst till 2026-10-09 07:00 UTC med fortsatt riktiga timers; en-GB och
Europe/Stockholm. Externa fonter/analytics blockeras i riggen, samma fallback
på bas/efter. Riggen väntar på stylesheets och öppningsanimation före BILD.
Källornas SHA256 finns i `source-sha256.json`.

```sh
python3 tools/mobilkarta_browser.py --baseline --source-root /PATH/TO/c093fc11 --output docs/reviews/mobilkarta/baseline
python3 tools/mobilkarta_mutations.py
python3 tools/mobilkarta_browser.py
```

BILD (meny, paneler) väntar Timothy före merge; TEXT (kontrollraden) hör till
playtest. Inga HTML-mallar, server-/Keryx-filer eller migrationer ändrade;
inget mallorsakat systemctl-restartkrav. Ingen merge, push eller deploy utförd.
Day-grenen @72e56c2d är orörd och väntar sina redan bokförda val.

## Tillägg efter granskning: topprad vid 390px

[Topbar v2](topbar-v2/README.md) gör ☍, ⌕ och ? nåbara med native tap i alla
motorer. Ny fysisk CSS-mutation, 90 layoutfall, 531 JS och full gestregression
gröna; nya Firefox-bilder i topbar-v2. Ovanstående bilder/källhashar är den
bevarade b19-basleveransen; tilläggets slutkällor finns i topbar-v2/source-sha256.json.
