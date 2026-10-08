# Q — enkel Host-panel

Kontrakt före kod; bas35232e3d, order `.agents/order-war-tillbaka-och-host.md` §Q.
Problem: platsvalet drunknar i prognos/förrådsrader. Spelarsanning: terräng, Feeds itself yes/no, Timber yes/no, Stone yes/no och Found är synliga; all detaljerad befintlig information går att öppna via EN stängd Details. Invariant: samma colonize-preview/status/request och FOW; ingen ny ekonomi/prognosformel; inline grundning/cancel kvar. Scope: Host-HTML och forecast-sammanfattning i map.js, produces-radens ID i map.html, founding Codex och prov. Övriga paneler/renderer/server/keryx utanför. Acceptans: sammanfattning från grain.est_net_per_tick≥0 och positiva goods.timber/goods.stone; tomt eller felaktigt svar visar unknown, ingen falsk yes; alla gamla detaljer/kontroller bevaras; följande panel återställer produces; grundning kvar nåbar desktop/390; sena forecast-svar får inte ändra ny panel. Stopvillkor: nytt server-/kanonbehov. Bevisar geografi-grindens platsval.
Bevisplan: oförändrad JS och actualhealth-bas, riktade actual-map-click tester rött→grönt och fysisk summary/Details-mutation; färsk riktig webb register/join→Host→Details→inline avbryt→confirm→city två gånger; BILD 1:1 före/efter; skyddade handlers/server diff.
Status: KLAR FÖR ÖVERLÄMNING. Klientkod `09711458`; BILD/integration hos Claude/Timothy. Ingen merge/push/deploy.

## Bevis

| Grind | Utfall |
|---|---|
| Kod | 433/433 JS bas; 437/437 slut. Fyra actual-map-click regressioner röda på oförändrad bas. Fem fysiska net/Details/stone/timber/stale mutationer named röd→återställd grön. |
| Semantik | Samma `grain.est_net_per_tick` och `goods.timber/stone` före avrundning; ingen ny matregel. Full forecast-renderare och dess payload/URL orörda. Unknown vid saknat underlag. Skyddade renderer-prefixen, inlinegrundningshandlern och server/keryx identiska (`logs/protected.json`). |
| Användare | Actualhealth bas **35232e3d543da9f09b415b1143098ce378e4e5d0 / 160 / ok** samt TVÅ rena efterarmar **097114588bed9aef7f71ffe4fd55686642fa6eb6 / 160 / ok**. Färsk DB/Redis per arm, normal register/join och kartklick. Summary exakt mot riktig forecast, positiv `goods.timber` krävs i båda armarna och **Timber: yes** verifieras; Details stängd/öppen; alla förrådsrader/fullprognos finns; avbryt ger ingen POST; confirm ger EN POST201 body `{}`, grundad stad och Host borta. Noll browserfel/dialoger/overflow/SQL-ingrepp. |
| BILD | Desktop1280/390, stängd/öppen Details samt inlineconfirmation före/efter. Found-knappen inom viewport även med full forecast öppen; detaljer scrollar i samma gamla body. Bedöms av Timothy genom Claude före merge. |
| Bygge/drift | Ren env serverbuild GOMAXPROCS2/-p2, binär-SHA i `logs/build.json`. Ingen Go-svit/vet omkörd eller påstådd för JS-only. Ingen deploy. |

Bilder: [före desktop](live-baseline/host-closed-desktop.png), [efter desktop](live-after2/host-closed-desktop.png), [efter mobil](live-after2/host-closed-mobile.png), [Details mobil](live-after2/host-details-mobile.png), [confirm mobil](live-after2/host-confirm-mobile.png). Rå prognos/found-status/kvitton i varje `proof.json`.

## Avgränsningar och förkastat

Samma befintliga prognos avgör självbärigheten; ingen ny garanti om verklig produktion, okända hexar eller fisk har införts. Ingen terränghärledning av timber/stone. De är potentialen i samma `goods`-fält; carried stocks används inte. Saknat/ogiltigt underlag och negativa/små positiva/zero värden provas i actual-handler-test med stubbat HTTP; livevärldarnas naturliga prognoser arkiveras.

Ingen renderer-/kartutseendeändring och ingen pixeldiff med olika slumpvärldar hävdas. Övriga inspect-paneler återfår Produces i routing-reset; sena Host-prognoser får inte skriva i nästa panel.

Första efterarmen avvisad: span-text saknade blanksteg mellan label/värde; verklig visning hade layoutavstånd men assertion/accessibel råtext upptäckte bristen. Fixad i538ce8cb. Interimgrön after2 avvisas som slutbevis eftersom separat före-kod-regression temporärt återställde källan under samma arm. Claudes granskning avvisade även de tidigare after3/after4-armarna: nyckeln var felaktigt `lumber`, medan riktig forecast har `timber` (8 i arkivet). Stubbar och första liveassert speglade samma fel och kunde därför passera. Alla gamla efterbevis flyttade till `rejected/wrong-commodity-key`. Den arkiverade verkliga forecasten i `fixtures/real-colonize-preview.json` provas nu med actual Host-handler: röd före rättningen (`logs/timber-before-red.log`), grön efter. En fysisk timber→lumber-mutation gör exakt denna riktiga-payload-assertion röd. De två aktuella slutarmarna (`timber-after1/2` i HOME-cache, `live-after1/2` här) kräver båda positiva timber-värden från verklig API, visar Timber yes och kör hela grundningen igen; fast kod/samma binär, inga källmutationer under armarna.

## Reproduktion

`node --test 'web/static/js/megaron/**/*.test.mjs'`; `python3 tools/host_simple_mutations.py OUT`. Byggserver enligt `logs/build.json`, sedan `python3 tools/host_simple_live.py OUT FULL_HASH after`. Baslinje: build35232e3d och `baseline ARCHIVED_WEB` med oförändrade assets. En OUT/ny DB per arm. Verktyget stänger enbart egna processer/containers; allt arbete under HOME.
