# Umami för varje webbverb

Problem: de flesta muterande webbverb saknar produktmätning; några befintliga event räknar klickflöden i stället för varje godkänd order.
Spelarsanning: aggregaten mäter vilka godkända verb och avslag människor möter. **Bevisar** kedjegrinden (analysberedskap), ingen synlig ändring.
Invariant: ett lyckat API-anrop ger ett event, 4xx bara verb_refused; inga id:n, namn, lösenord, fritext eller positioner i props. Ingen spårning på klick, GET eller nätverksfel.
Scope: web API/telemetri, två kringgående fetch-vägar, befintliga track-anrop, paritetsprov, proof och vault as-built-tabell.
Non-scope: Temenos, Keryx, spelregler, mig163-avslagsloggen, UI/CSS, deploy.
Acceptans: varje aktuellt webbverb kartlagt; success/refusal skiljs; prov mot verblistan; borttaget track ger namngivet rött; varje nytt event landar med test:1 i Umami via vanlig Firefox-UA.
Stopvillkor: behov av kanon/integritetsändring, DB-direktskrivning eller deploy.
Bevisplan: 458 gröna baseline-JS, instrumenterade API-svar (inkl. bulk/204/202/4xx/5xx/secrets), verklig produktionsmutation, full JS och live curl + read-only Umami-DB. Ingen BILD.


## Resultat och inventering

50 metod/rutt/variantposter täcker verblistans 36 muterande rader (en explicit gatad kingdom-rad undantagen), med 47 success-eventnamn och ett gemensamt refusal-event. De åtta befintliga namnen behålls exakt. `drawer_open` och fetch/WS-event stannar där de redan hör hemma. Katalogen omfattar också DELETE-notiser-rutten, utan att lägga till en spelaryta.

| Verb / variant | Event efter OK | Props (endast vitlistade kategorier) | Baslinje 1a12d088 |
|---|---|---|---|
| build | `build_started` | building | befintligt namn; flyttat till API-svar |
| cancel-build | `build_cancelled` | — | saknades |
| recruit | `recruit_started` | unit | befintligt namn; flyttat till API-svar |
| place | `worker_placed` | good | saknades |
| staff | `workplace_staffed` | good | saknades |
| unplace | `placement_removed` | — | saknades |
| labor | `cult_allocated` | — | saknades |
| slaughter | `livestock_slaughtered` | — | befintligt namn; flyttat till API-svar |
| disband | `army_disbanded` | — | saknades |
| transfer | `transfer_sent` | good | saknades |
| gift/tribute | `gift_sent` | good | saknades |
| abandon | `settlement_abandoned` | — | saknades |
| rite | `rite_performed` | rite | befintligt namn; flyttat till API-svar |
| gift | `loyalty_gift_sent` | — | saknades |
| occupation-order | `occupation_order_sent` | — | saknades |
| march | `march_sent` | intent | befintligt namn; flyttat till API-svar |
| recall | `recall_sent` | — | saknades |
| redirect | `redirect_sent` | — | saknades |
| stance | `stance_sent` | stance | saknades |
| retreat-order | `retreat_order_sent` | — | saknades |
| retreat-default | `retreat_default_changed` | — | saknades |
| reinforce | `reinforce_sent` | — | saknades |
| repair | `repair_started` | — | saknades |
| load | `troops_loaded` | — | saknades |
| unload | `troops_unloaded` | — | saknades |
| join | `world_joined` | — | saknades |
| founding settle | `settle` | — | befintligt namn; flyttat till API-svar |
| message | `messenger_sent` | — | befintligt namn; flyttat till API-svar |
| trade-offer | `trade_offer` | kind | befintligt namn; flyttat till API-svar |
| reply | `reply_sent` | — | saknades |
| trade-accept | `trade_accepted` | — | saknades |
| trade-decline | `trade_declined` | — | saknades |
| trade-cancel | `trade_cancelled` | — | saknades |
| arrange passage | `passage_arranged` | — | saknades |
| fetch by ship | `pickup_sent` | — | saknades |
| call back | `runner_called_back` | — | saknades |
| standing order | `standing_order_created` | — | saknades |
| standing order pause | `standing_order_paused` | — | saknades |
| standing order resume | `standing_order_resumed` | — | saknades |
| standing order delete | `standing_order_deleted` | — | saknades |
| report | `report_sent` | — | saknades |
| mark notifications read (all) | `notifications_read` | — | saknades |
| mark notification read (one) | `notification_read` | — | saknades |
| delete notifications | `notifications_deleted` | — | saknades |
| password | `password_changed` | — | saknades |
| agora password | `agora_password_changed` | — | saknades |
| notification-preferences | `notification_preferences_changed` | action (mute/unmute) | saknades |
| place → ta hex | `worker_placed` | good | samma POST-gren; ingen separat rutt |
| march → explore / colonize | `march_sent` | intent | befintligt namn, per godkänd order |
| return-army / kingdom-* | — | — | gatad POST-MVP-yta; uttryckligen utesluten i verblistan |
| avslag (4xx på katalogrutt) | `verb_refused` | verb; reason_code endast insufficient_goods | saknades; kompletterar mig163:s per-spelare-DB |

**Mätsemantik:** framgång betyder godkänt API-svar (även 202/204), inte att ett fysiskt bud redan nått fram eller ett bygge är klart. Bulk räknas per faktiskt godkänd request: +fill och Recall all har ingen extra bulk-händelse. Kartans redirect får redirect_sent, inte en felmärkt march_sent. Det äldre march_sent räknade ett blandat batch-flöde en gång; namnens historik behålls men denna felaktiga räkningsenhet rättas. Livestock-eventets äldre gubbar_placed-antal ersätts inte av något nytt antal: denna slice skickar enbart enumkategorier.

**Integritet:** explicit metod + ankare på rutten, aldrig URL/ID som props. Request/response-objekt går inte till Umami; endast kända byggnad/enhet/rit/intent/vara/kind-värden väljs ut. Unknown kategori utelämnas. `reason_code` tillåter bara det maskintoken servern faktiskt producerar, `insufficient_goods` (nu i `error`, framtida explicit `error_code` läses med samma vitlista). Andra avslag bär bara verb. Ingen fritextorsak gissas. API-svaret läses från klon och caller behåller hela sitt svar. Trackerfel, även Promise-rejections, påverkar inte spelkoden.

## Grindar och bevis

- Baslinje: **458 JS**, exit0, utan skips (`baseline-js.log`).
- Kod/semantik: **519 JS**, varav **61** nya telemetriprover, exit0, utan skips (`full-js.log`, `telemetry-js.log`). Alla 50 katalogvarianter kör det riktiga fetchAuth mot scriptade HTTP-svar; provar OK/202/204, väntande request, 4xx/5xx/network, bulk-delsvar, läsbart caller-svar, sekretess, vitlistor och blockerad/trasig tracker. Live vault läses när den finns; committad snapshot används annars för fristående CI. Nya okända vault-rader och saknade katalogverb ger rött.
- Mutation: borttagen **riktig katalograd**, borttaget **riktigt success-track**, borttagen **riktig API-koppling** ger NAMNGIVET rött, aldrig syntaxfel/timeout; byte-identisk återställning ger 61 gröna (`mutation-*.log`, `restored-js.log`). `tools/umami_verb_mutations.py` reproducerar.
- Användarscenarier: Firefox kör **6 riktiga controllerflöden** med scriptade HTTP-svar (`browser.json`, `browser.log`): join med/utan token + avslag, inspect-bud från host/stad + avslag. Headers/payload, blankning respektive bevarat utkast, ursprunglig feltext och join-redirect bevarade; exakt ett korrekt event, inga pageerrors. Första riggen saknade map-root/canvas som map-modulen behöver vid import; rött sparat i `browser-incomplete-harness.log`, korrigerat i fixturen, ingen produktion ändrad för riggen. Detta är ett kontrollerprov, inte ett nytt live backend-spelscenario.
- Live Umami: **40 nya eventnamn** (39 success + verb_refused) skickade med vanlig Firefox-UA till https://umami.formatet.se/api/send. Varje POST gav HTTP200, och read-only SELECT i CT105 visar precis en rad per namn från denna körning med **test:1** (`live-umami.json`, `live-umami.log`). Inget raderat, ingen direkt DB-skrivning. `tools/umami_verb_proof.py --send --output …` reproducerar när nya testevent uttryckligen är tillåtna. Detta bevisar mottagningen i Umami; browser/JS-proven bevisar vad produktionen avser skicka.
- Visuellt: ingen synlig ändring, ingen BILD-gate. Temenos/Keryx/DB-spelregler ej ändrade; ingen ny full Go-svit påstås.

## Deploy och överlämning

**Mall ändrad: `web/templates/join.html`.** Den använder samma fetchAuth som övriga verb, med samma headers/felhantering/redirect och window.joinWorld för befintlig onclick. Kartans inspect-bud (från stad respektive host) använder också fetchAuth; behåller sin uttryckliga Authorization-header. **Deploy kräver `systemctl restart poleia` eftersom air endast bevakar .go.** Claude ansvarar för merge/push/deploy och ska först vänta ut eventuella migrationskörningar. Ingen deploy från Codex.

Beviset är låst till master **1a12d088** före slicen, mig163 förblir oförändrad. Vault `megaron_plan_umami.md` får samma as-built-tabell, tydligt **byggt för granskning, EJ LIVE**. Nästa steg efter granskning: Claude integrerar denna gren; day/U väntar på egen START.

Repro: `node --test $(rg --files web/static/js -g '*.test.mjs')`; `python3 tools/umami_verb_mutations.py`; `python3 tools/umami_verb_browser.py`. Live-testevent är kvar och skall filtreras med test:1 i produktanalysen.
