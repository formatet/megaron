# ORDER 5 — tidsangivelser

Klart för Claudes granskning på `codex/tidsangivelser`, bas `2aed60f9`. Händelsetid visas med klockslag/datum; längder behåller dagar. Full fresh Go på mig197 (world153.367s), 702 JS, vet och två day-N-mutationer röda→gröna. Firefox-midnatt grön. Ingen integration/deploy; TEXT prövas i Haiku-spelandet.

## Slice-kontrakt
Problem: absoluta speldagsnummer blandas med klockslag, särskilt i expeditionskvittensen.
Spelarsanning: när något händer visas med lokal klocka/datum; längder i speldagar.
Invariant: ticks, ankare, eventpayloads, rörelse och världens takt ändras inte.
Scope: webbens och Keryx tidsformatterare och deras konsumenter; Codex och relevanta prov.
Utanför: migration 198/matlager, ny kalender, gameplay och deploy.
Acceptans: inga absoluta day-N-nummer i spelarprosa; samma ankare används för tider; midnatt, tick 0 och saknat ankare täcks.
Grind: TEXT i playtest; kod verifieras med fresh Go (57900), JS och named mutation.
Kedjegrind: bevisar begriplig information om order/ankomst i geografi→brist→brons→elit.

## Inventering före ändringar (bas 2aed60f9)

| Yta | Konsumenter av absoluta ticknummer |
|---|---|
| Webb gemensam | ui/fmt_num.js fmtDay: format.js räddning, skickat bud, upptäckt armé/ETA; expedition.js vändning/hemkomst; march_preview.js ankomst/hemkomst; drawers/war.js väntar till; drawers/economy.js transport; drawers/diplomacy.js gods/silver/handelsschema |
| Webb direkta strängar | ui/notification_state.js Day # (Dispatches, chips, fönster); ui/hover.js och stormname.js sist sedd; ui/account_profile.js grundad; drawers/city_production.js senaste måltid; march_preview.js world day |
| Keryx gemensam | output.go formatDay: transport/gift, handel, notification_text.go + cmd_notifications.go armé/ETA, cmd_notifications_carrier.go skickad/hemma; cmd_expedition_mission.go; cmd_march_preview.go; cmd_messenger.go scheman; cmd_goods.go karavaner; cmd_unit.go väntar/upphämtning; cmd_ticklog.go; cmd_reports.go; cmd_status.go senaste produktion/statusrubrik |
| Keryx direkta strängar | notification_state.go Day # (notifications/watch); cmd_storms.go sist sedd; cmd_me.go grundad; cmd_status.go måltid + världens dag; cmd_march_preview.go world day |
| Codex | time.md föreskriver day 12; runner-rescued-at-sea.md två Home on day 509-exempel; coming-back.md lovar Day #/Day unknown (kompletterande träff) |

Behåll: formatDays/fmtDays och durationer (mat, sold, expedition, väntetid, tillväxt, cooldown, upptagning) samt /day-produktion. JSON/API-fältnamn och interna ticknummer behålls. Världskalendern (Day 6 of the Olive, Year 1) i misc.js/drawers/notif.js är en separat kalender, ingen löpande day-N-visning; föreslås lämnas enligt orderns non-scope. Keryx statusrubrikens löpande världsdagsnummer ändras.

Befintliga ägare: webb world_time.js ankare → fmt_clock.js lokal kalender; time.js framtida ankomstformatterare klampar redan passerad tid till arrived. Historiska datum behöver en formatterare utan den klampningen. Keryx Client.WorldTickETA använder redan world.current_tick/tick_anchor_at/tick_seconds; output.go clockTime visar idag alltid veckodag+datum. Notisernas created_at kan vara leveranstid, inte händelsetid: prioritet för registrerat world_tick bibehålls.


## Resultat och avgränsningar
Webbens `fmt_clock.js:fmtTickTime` och Keryx `output.go:formatWorldTime` konverterar registrerade ticks med befintligt världsankare. Webbens gamla `fmtDay`-export är ett alias till samma formatterare, inte en andra ägare. Keryx skickar kommandots Client till tidigare rena presentationsfunktioner; ankaret hämtas högst en gång per Client. Absoluta ISO-fallbacks i Keryx visas också som klocka.

Notisernas `created_at` används inte som händelsetid: det kan vara leveranstid. Saknat world_tick eller ankare ger `time unavailable`. Tick noll är giltigt. Historiska ticks visas som klockslag/datum, aldrig `arrived`. Världskalendern är uttryckligen orörd efter Claudes granskning 23:06. Inga server/API-, event-, ticklängds- eller migrationsändringar; matlager/mig198 ingår inte.

Historiska ticks projiceras med dagens ankare och takt enligt ordern. En äldre taktförändring eller serverpaus kan inte rekonstrueras ur enbart det ankaret; ingen ny historisk tidsdatamodell införs.

## Bevis
- Namngiven Keryx-regression: `TestAbsoluteWorldClockUsesAnchorNotTicksOrDelivery`, med tick0, förflutet tick, falsk created_at, ändrad takt, saknat ankare och expeditionens 30-dagarslängd.
- `TestWorldClockMidnightHistoryAndDates` täcker idag/imorgon/igår/veckodag/datum/annat år, i Europe/Stockholm.
- Webb `time_presentation.test.mjs` täcker motsvarande kontrakt inklusive skev klientklocka.
- Verkliga kommandon med HTTP-fixtur: befintliga march/preview/transfer/gift/trade/me-regressioner har kompletta världsankare. Deras JSON-lägen behåller råa ticks; förhandsvisning skickar fortfarande inga ordrar.
- Firefox integrerar grenens riktiga ES-moduler via lokal HTTP, vid midnatt i Europe/Stockholm. Expedition: `30 days; turns home by tomorrow 00:10; home by tomorrow 00:25`; historisk tick0: `today 22:10`; räddningsrapport samma tid. Inga page errors. Recept `python3 docs/reviews/tidsangivelser/browser_check.py`; resultat `firefox.json`. Detta är ett funktionellt webbläsarprov, inte en BILD-grind.
- Fysisk mutation på båda ytor: ersätt formatteraren med `day N` → namngiven assertion RED; ursprungliga bytes återställda → GREEN. Loggar `{web,keryx}-mutation-red.log` och `*-restored-green.log`.

TEXT-grinden återstår i Haiku-playtest hos Claude. Ingen merge, push eller deploy av Codex.


## Grindar och reproduktion
| Grind | Resultat |
|---|---|
| Kod | `GOTEST_PORT=57900 tools/gotest.sh`: ny tom PG16, mig197 ren, hela sviten GREEN; `full-fresh-go.log`. `go -C server vet ./...` GREEN. |
| Webb | Alla 702 Node-prov GREEN; `js-summary.log`. Firefox integration GREEN; `firefox.json`. |
| Semantik | Klocka för när, dagar för längd; JSON/event/rörelse/kalender orörda. Två fysiska mutationer RED → återställd GREEN. |
| Användare | TEXT återstår i Haiku-playtest, Claude äger nästa steg. |
| Drift | Ej deployad, ingen levande värld använd som prov. |

Proveniens: egen worktree `/home/tk/wt/codex-tidsangivelser`, uttryckligen beställd bas `2aed60f9`; `tested-files.json` låser samtliga testade ändrade server-/webbfiler med SHA256. Fullsviten avslutades utan senare produktionsändringar. Claude mergade matlager/mig198 separat under provningen; den ingår inte i grenens bas eller bevis. Firefox kör lokala statiska ES-moduler och inga gameplay-anrop.
