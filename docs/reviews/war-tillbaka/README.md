# P — återställ War-kort

Kontrakt före kod. Bas f586270d, order `.agents/order-war-tillbaka-och-host.md`.

Problem: War-kontroller flyttades i I till stängd More utan nytta för Timothy. Spelarsanning: hållning/Set, reträtt i strid, Fetch, Load/Unload/Repair och Redirect nås direkt på tillämpligt kort igen. Invariant: samma eligibility, bodies och handlers; senare NaN-vakt, siffror, late-DOM, inline-svar och recall-all bevaras. Scope: kortets HTML, war_cards-test, fyra Codex-artiklar och reproducerbara prov. Host/chips/misc/server/keryx utanför. Acceptans: inga More i kort; giltiga kontroller direkt synliga; ogiltiga fortsatt saknas; befintliga statusord/siffror/ETA-vakt består; real webb stance→march→recall→hemma + BILD stad/marsch 1:1 desktop/390. Stopvillkor: kanon eller backend-behov; inget sådant antas. Bevisar kedjegrindens nåbara orderyta.

Bevisplan: oförändrad JS-baslinje och arkiverade webbassets; färsk PG16/Redis per arm med healthz-hash; slutlig JS-svit, fysisk mutation av kontrollsynlighet, verkliga webbflöden två gånger; server/keryx och handlers jämförs byte för byte. BILD till Timothy via Claude före merge.

Status: KLAR FÖR ÖVERLÄMNING, BILD/integration hos Claude/Timothy. Produktionskod 8650991a; efterföljande commit innehåller endast provverktyg/bevis. Steg4-WIP parkerad på 7f41178e. Ingen merge/push/deploy.

## Resultat

Åtta korttester beskriver direkt åtkomst, status/lifecycle/gates och siffer/ETA-bevarande. Bas 431/431 JS; slut 432/432. Fyra ytförväntningar röda mot förenklad kod före fix. Fysiska mutationer: stängd More återinförd → `garrison actions must not be hidden` röd; NaN-vakt bort → `unknown arrival must be empty` röd; navalrecall öppnad → `ships must never promise recall or redirect` röd. Alla återställda gröna.

Skyddade delar (`logs/protected.json`): orderhandlers och drawer-livscykel/lateDOM byte-identiska; server/keryx, time, recall-all, chips och misc tomdiff. Statusord, siffror, I:s R3/R6 och matgate-buggfix behållna. Server byggd ren env med GOMAXPROCS=2/-p2; ingen ny Go-svit/vet påstås (inga Go-ändringar).

Healthz ur varje verklig process:

| Arm | Commit | Migration/status | Resultat |
|---|---|---|---|
| live-baseline | f586270d40840fc8afb550bca80a2ee73d4e7a97 | 160 / ok | stance sentry/fortify/clear → March202 → Recall202 → garrison |
| live-after1 | 8650991adb9347a4fe27dfe4b7f0d7c01ef4f1f6 | 160 / ok | samma hela resa, utan More |
| live-after2 | 8650991adb9347a4fe27dfe4b7f0d7c01ef4f1f6 | 160 / ok | andra färska hela resan, utan More |
| live-naval | 8650991adb9347a4fe27dfe4b7f0d7c01ef4f1f6 | 160 / ok | Load200 → Unload200, samma landunit tillbaka i garrison |

Varje arm register/join/founding, ny PG16/Redis, ren servermiljö, SQL endast read-only audit. Noll browserfel/horisontell overflow. Stance/Set syns direkt i stad och på marsch; Redirect syns direkt och Q/R-formen öppnas via vanliga klick. Stad/marsch finns som BILD 1:1 desktop1280/390. `live-after2` är den rena repetitionen efter alla upptäckta rigg-/scenariofynd; produktionskod identisk med första grönarm.

Bilder: [före stad](live-baseline/city-desktop.png), [efter stad](live-after2/city-desktop.png), [före marsch](live-baseline/marching-desktop.png), [efter marsch](live-after2/marching-desktop.png), [mobil stad](live-after2/city-mobile.png), [mobil marsch](live-after2/marching-mobile.png). BILD bedöms av Timothy genom Claude före merge.

## Avgränsningar och sidofynd

Battle retreat, Fetch och Repair verifieras i statematris, inte med verklig strid/transport/skada. Deras handlers/gates/val oförändrade. Typed redirect öppnas men dispatchas inte. Keryx/server oförändrade. Ingen kart-renderingsändring; ingen pixeldiff med olika slumpvärldar hävdas.

Den ärvda riggen använde Explore. Ett kort första ben gav korrekt422: ingen Runner hann ikapp. Att vänta på nästa ben visade sedan ett **befintligt backend-fynd**, på både oförändrad bas och slutkod: Recall går tillbaka till det benets starthex, inte staden. `ExecuteRecall` (`server/internal/combat/recall_redirect.go:116`) sätter `newTarget := origin` från aktuell legs q/r. Explorationsarmarna är **röda**, sparade separat i `known-findings` och skickade till Claude för egen triage; de räknas aldrig som passerade bevis. P:s gröna landarmar använder vanlig March till en känd, nåbar hex vald genom läsande march-preview. Ingen serverfix smygs in.

## Reproduktion

`node --test 'web/static/js/megaron/**/*.test.mjs'` och `python3 tools/war_cards_mutations.py OUT`. Bygg `OUT/temenos` från fast commit med receptet i `logs/build.json`. Kör `python3 tools/war_cards_live.py OUT COMMIT before-restoration ARCHIVED_WEB` för basen, `restored` för slutland och `restored-naval` för skepp. Alla OUT/tool/cache under HOME, inga /tmp-worktrees. Verktyget arkiverar kvitton, healthz, audit, slutunit, bilder och serverlogg och stänger endast egna resurser.
