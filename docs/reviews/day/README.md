# Day — slice contract

Base c093fc11; branch codex/day. Timothy's line C, 2026-10-08: day is the world's unit on web, Keryx, Codex and player-facing server messages. Absolute world dates read `day N`; durations use a shared singular/plural formatter. Rates read `/day`. Wall time uses clock/date. Engine/DB/JSON identifiers and CLI option names retain tick. Turning home remains an action.

This proves legibility of the geography → scarcity → bronze → elite chain; it changes no simulation or timing rule. Calendar month ordinal remains at its existing default until Timothy selects the with/without-parentheses BILD alternative.

Baseline: full JS 519/519; fresh Go tick/Keryx PASS. Final proofs, mutations, browser images and investigation row outcomes follow below.

## Implementation and limits

Web `fmtDays`/`fmtDay` and Keryx `formatDays`/`formatDay` own generated duration/date units, including fractional food coverage and singular at the displayed value. The server's presentation helper is renamed `tick.FormatDays`; `tick.FormatDay` handles absolute days. `GameDaysLeft`, scheduling, DB columns, JSON tags and command flags are retained. Rates and static labels use day; prose can still say “turn home”. Calendar day/month/year calculation is unchanged.

Clock/date formatting is shared separately: dependency-free `ui/fmt_clock.js` avoids the existing `format.js` ↔ `time.js` cycle. Web ETA, notification estimates and ages use it; Keryx founding stores, fallback ETAs, letters, notifications and rumours share `clockTime`/`wallClock`. Approximate derived times carry ≈. Engine cadence, rounding up waits, authoritative arrival_tick and ISO fallback remain as before. No new clock exception.

Guards: `tools/day_guard.py` reads production web JS/static HTML/templates, all Keryx Go, Codex Markdown/index, and all server api/internal Go. Four named suite tests invoke it. The exact exception manifest records each literal, its maximum count, its file and its infrastructure reason; adding a duplicate still fails. JS scanning follows nested template expressions while excluding their identifiers; comments are excluded. SQL/import paths/JSON tags are structural exceptions; `--ticks` and `ticklog` remain command identifiers. New diagnostic literals need explicit review, just as new prose does.

No migration or HTML template changed. `web/static/map.html` changed visible labels. Go changes rebuild normally; there is no additional template-only restart requirement introduced by this slice. No deployment was performed.

BILD blocks integration until reviewed. Calendar alternatives are `browser/*-calendar-with.png` and `browser/*-calendar-without.png`; existing default retains the ordinal. TEXT remains a playtest gate; no choice or player comprehension approval is claimed here.

## Investigation, every table row

Original file/line labels below refer to the investigation's daf740dd snapshot. Current line numbers moved; use the named functions in this branch. Source document SHA-256: `eea714538d626e63df3d0d5412fb2e71f2b6ff74579f2c9946a5e9ca5f402174`. Each original table row appears exactly once, in original order.

### Tabell 1 — Webb (`web/static/js/megaron/`, `web/static/map.html`)

| Original source row | Outcome |
|---|---|
| ui/fmt_num.js:11–13 | Fixed: common duration formatter emits 1 day/N days; displayed-number singular and absolute-day helper tested. |
| map.html:289 | Fixed: static expedition unit days and automatic report attachment current day; input IDs and JSON unchanged. |
| ui/expedition.js:13 | Fixed: generated durations/absolute days use fmtDays/fmtDay; daily rates/labels use day. Existing formatter consumers inherit the new unit. |
| ui/expedition.js:18 | Fixed: generated durations/absolute days use fmtDays/fmtDay; daily rates/labels use day. Existing formatter consumers inherit the new unit. |
| ui/expedition.js:23–24 | Fixed: generated durations/absolute days use fmtDays/fmtDay; daily rates/labels use day. Existing formatter consumers inherit the new unit. |
| ui/citygrid.js:79,85 | Fixed: generated durations/absolute days use fmtDays/fmtDay; daily rates/labels use day. Existing formatter consumers inherit the new unit. |
| ui/marchctx.js:169–170 | Fixed: generated durations/absolute days use fmtDays/fmtDay; daily rates/labels use day. Existing formatter consumers inherit the new unit. |
| ui/marchctx.js:176,194 | Fixed: generated durations/absolute days use fmtDays/fmtDay; daily rates/labels use day. Existing formatter consumers inherit the new unit. |
| ui/marchctx.js:57,172 | Fixed: generated durations/absolute days use fmtDays/fmtDay; daily rates/labels use day. Existing formatter consumers inherit the new unit. |
| ui/march_preview.js:22 | Fixed: generated durations/absolute days use fmtDays/fmtDay; daily rates/labels use day. Existing formatter consumers inherit the new unit. |
| ui/drawers/city_production.js:14 | Fixed: generated durations/absolute days use fmtDays/fmtDay; daily rates/labels use day. Existing formatter consumers inherit the new unit. |
| ui/drawers/city_production.js:34 | Fixed: generated durations/absolute days use fmtDays/fmtDay; daily rates/labels use day. Existing formatter consumers inherit the new unit. |
| ui/drawers/city_production.js:39,43,45 | Fixed: generated durations/absolute days use fmtDays/fmtDay; daily rates/labels use day. Existing formatter consumers inherit the new unit. |
| ui/drawers/city.js:285,672 | Fixed: generated durations/absolute days use fmtDays/fmtDay; daily rates/labels use day. Existing formatter consumers inherit the new unit. |
| ui/drawers/economy.js:26,31,136,246,631,632 | Fixed: generated durations/absolute days use fmtDays/fmtDay; daily rates/labels use day. Existing formatter consumers inherit the new unit. |
| ui/drawers/economy.js:137,423 | Fixed: generated durations/absolute days use fmtDays/fmtDay; daily rates/labels use day. Existing formatter consumers inherit the new unit. |
| ui/drawers/war.js:462 | Fixed: generated durations/absolute days use fmtDays/fmtDay; daily rates/labels use day. Existing formatter consumers inherit the new unit. |
| ui/drawers/war.js:991 | Fixed: generated durations/absolute days use fmtDays/fmtDay; daily rates/labels use day. Existing formatter consumers inherit the new unit. |
| ui/drawers/war.js:541 | Fixed: generated durations/absolute days use fmtDays/fmtDay; daily rates/labels use day. Existing formatter consumers inherit the new unit. |
| render/map.js:4349 | Fixed: generated durations/absolute days use fmtDays/fmtDay; daily rates/labels use day. Existing formatter consumers inherit the new unit. |
| ui/format.js:413 | Fixed: generated durations/absolute days use fmtDays/fmtDay; daily rates/labels use day. Existing formatter consumers inherit the new unit. |
| ui/format.js:430 | Fixed: generated durations/absolute days use fmtDays/fmtDay; daily rates/labels use day. Existing formatter consumers inherit the new unit. |
| ui/format.js:445,448,850,852 | Fixed: generated durations/absolute days use fmtDays/fmtDay; daily rates/labels use day. Existing formatter consumers inherit the new unit. |
| ui/format.js:447,849 | Fixed: generated durations/absolute days use fmtDays/fmtDay; daily rates/labels use day. Existing formatter consumers inherit the new unit. |
| ui/format.js:553–563 | Fixed: generated durations/absolute days use fmtDays/fmtDay; daily rates/labels use day. Existing formatter consumers inherit the new unit. |
| ui/format.js:678 | Fixed: generated durations/absolute days use fmtDays/fmtDay; daily rates/labels use day. Existing formatter consumers inherit the new unit. |
| ui/format.js:246 | Fixed: generated durations/absolute days use fmtDays/fmtDay; daily rates/labels use day. Existing formatter consumers inherit the new unit. |
| ui/format.js:742 | Fixed: generated durations/absolute days use fmtDays/fmtDay; daily rates/labels use day. Existing formatter consumers inherit the new unit. |
| ui/format.js:780 | Retained: today/daily/recent days refer to world-time history, already chosen vocabulary. |
| ui/drawers/economy.js:304 | Fixed: generated durations/absolute days use fmtDays/fmtDay; daily rates/labels use day. Existing formatter consumers inherit the new unit. |
| ui/drawers/diplomacy.js:548–549 | Fixed: generated durations/absolute days use fmtDays/fmtDay; daily rates/labels use day. Existing formatter consumers inherit the new unit. |
| ui/drawers/diplomacy.js:634–635 | Fixed: generated durations/absolute days use fmtDays/fmtDay; daily rates/labels use day. Existing formatter consumers inherit the new unit. |
| ui/drawers/kult.js:59 · kult_kharis.js:29 | Fixed: generated durations/absolute days use fmtDays/fmtDay; daily rates/labels use day. Existing formatter consumers inherit the new unit. |
| ui/drawers/kult.js:84 | Fixed: generated durations/absolute days use fmtDays/fmtDay; daily rates/labels use day. Existing formatter consumers inherit the new unit. |
| ui/drawers/city.js:677 | Day column identifies absolute world days; row number retained, no second chronology introduced. |
| ui/drawers/city.js:755 | Fixed: generated durations/absolute days use fmtDays/fmtDay; daily rates/labels use day. Existing formatter consumers inherit the new unit. |
| ui/drawers/city_production.js:17 | Retained: today/daily/recent days refer to world-time history, already chosen vocabulary. |
| ui/drawers/city.js:670 | Retained: today/daily/recent days refer to world-time history, already chosen vocabulary. |
| map.html:199 | Fixed: static expedition unit days and automatic report attachment current day; input IDs and JSON unchanged. |
| ui/drawers/notif.js:32 | Two BILD alternatives supplied; same date, ordinal with/without; existing default retained pending Timothy. |
| ui/misc.js:430 | Retained: Shadow Days is the intercalary month’s lore name, not wall time. |
| ui/format.js:115–119 | Fixed: all wall estimates use shared clock/date; no real-day/d/h countdown. Past ages also use clock/date. |
| ui/time.js:41,55–60 | Fixed: all wall estimates use shared clock/date; no real-day/d/h countdown. Past ages also use clock/date. |
### Tabell 2 — Keryx (`server/cmd/keryx/`)

| Original source row | Outcome |
|---|---|
| output.go:112,114 | Fixed: player unit day; generated counts use common surface formatter, rates /day. Stable internal identifiers retained. |
| cmd_rite.go:436,438 | Fixed: player unit day; generated counts use common surface formatter, rates /day. Stable internal identifiers retained. |
| cmd_expedition_mission.go:16,23 | Fixed: player unit day; generated counts use common surface formatter, rates /day. Stable internal identifiers retained. |
| cmd_march_preview.go:33 | Fixed: player unit day; generated counts use common surface formatter, rates /day. Stable internal identifiers retained. |
| cmd_unit.go:629,663,829 | Fixed: player unit day; generated counts use common surface formatter, rates /day. Stable internal identifiers retained. |
| (via server) | Fixed: presentation helper renamed FormatDays, cooldown retains existing upward rounding and cadence. |
| output.go:153,155,169 | Fixed: player unit day; generated counts use common surface formatter, rates /day. Stable internal identifiers retained. |
| cmd_messenger.go:503,508 | Fixed: player unit day; generated counts use common surface formatter, rates /day. Stable internal identifiers retained. |
| cmd_goods.go:293 | Fixed: player unit day; generated counts use common surface formatter, rates /day. Stable internal identifiers retained. |
| cmd_expedition.go:64,99 | Fixed: player unit day; generated counts use common surface formatter, rates /day. Stable internal identifiers retained. |
| cmd_notifications.go:367,413 | Fixed: player unit day; generated counts use common surface formatter, rates /day. Stable internal identifiers retained. |
| cmd_notifications.go:443–450 | Fixed: player unit day; generated counts use common surface formatter, rates /day. Stable internal identifiers retained. |
| cmd_notifications.go:793,795,798,1069 | Fixed: player unit day; generated counts use common surface formatter, rates /day. Stable internal identifiers retained. |
| cmd_status.go:309,403,636,676,686,704,729,761,898,900,943,991,997 | Fixed: player unit day; generated counts use common surface formatter, rates /day. Stable internal identifiers retained. |
| cmd_status.go:1082–1084 | Fixed: player unit day; generated counts use common surface formatter, rates /day. Stable internal identifiers retained. |
| cmd_ticklog.go:22,69,73,113,136,139 | Fixed: player unit day; generated counts use common surface formatter, rates /day. Stable internal identifiers retained. |
| cmd_reports.go:152,223 | Fixed: player unit day; generated counts use common surface formatter, rates /day. Stable internal identifiers retained. |
| cmd_recruit.go:281,382–393 | Days column retained; generated coverage uses common duration helper, rates /day. |
| cmd_build.go:99 | Days column retained; generated coverage uses common duration helper, rates /day. |
| cmd_unit.go:374,376 | Fixed: player unit day; generated counts use common surface formatter, rates /day. Stable internal identifiers retained. |
| cmd_unit.go:483,949,965,1012,1227,1331,1654–1661 | Fixed: player unit day; generated counts use common surface formatter, rates /day. Stable internal identifiers retained. |
| cmd_wants.go:122 | Fixed: player unit day; generated counts use common surface formatter, rates /day. Stable internal identifiers retained. |
| cmd_allocate.go:184 | Fixed: player unit day; generated counts use common surface formatter, rates /day. Stable internal identifiers retained. |
| cmd_founding.go:70–72 | Fixed: common world duration plus ≈ Mon Jan 2 HH:MM derived from existing cadence; no real-day or real-hour duration. |
### Tabell 3 — Codex (`web/static/codex/`)

| Original source row | Outcome |
|---|---|
| welcome.md:7 | Fixed: day is the world unit, no player-facing tick definition. Day glossary and Days and the clock heading. |
| time.md:3 | Fixed: day is the world unit, no player-facing tick definition. Day glossary and Days and the clock heading. |
| marching.md:41 | Fixed: world prose now days, whole-day leg rounding and arrival day; no timing change. |
| trade.md:10 | Fixed: world prose now days, whole-day leg rounding and arrival day; no timing change. |
| messengers.md:11 | Fixed: world prose now days, whole-day leg rounding and arrival day; no timing change. |
| keryx.md:32 | Fixed: world prose now days, whole-day leg rounding and arrival day; no timing change. |
| city.md:11 | Fixed: world prose now days, whole-day leg rounding and arrival day; no timing change. |
| city.md:13 | Fixed: world prose now days, whole-day leg rounding and arrival day; no timing change. |
| horde.md:21 | Fixed: world prose now days, whole-day leg rounding and arrival day; no timing change. |
| sea.md:15 | Fixed: world prose now days, whole-day leg rounding and arrival day; no timing change. |
| time.md:1,3 | Fixed: day is the world unit, no player-facing tick definition. Day glossary and Days and the clock heading. |
| index.json:6 | Retained: Days and the clock already names the chosen unit; article body rewritten. |
| glossary.md:27 | Fixed: day is the world unit, no player-facing tick definition. Day glossary and Days and the clock heading. |
| trade.md:40,46 · routes.md:28,34 · transfers.md:37,43 | Fixed: world prose now days, whole-day leg rounding and arrival day; no timing change. |
| marching.md:45 | Retained: --ticks is stable CLI syntax, explicitly excluded as an identifier; its help/prose now says days. |
| sea.md:85 | Fixed: world prose now days, whole-day leg rounding and arrival day; no timing change. |
| sight.md:26,29,33 | Retained: days/daily/weeks are existing world-time prose. No wall-clock duration or engine unit; no mechanic/threshold changed. |
| upkeep.md:1,5,11 · silver.md:3 · units.md:22 | Retained: days/daily/weeks are existing world-time prose. No wall-clock duration or engine unit; no mechanic/threshold changed. |
| food.md:1,5,7,10 | Retained: days/daily/weeks are existing world-time prose. No wall-clock duration or engine unit; no mechanic/threshold changed. |
| growth.md:1,3,4 | Retained: days/daily/weeks are existing world-time prose. No wall-clock duration or engine unit; no mechanic/threshold changed. |
| kharis.md:12,16,20,24 · temples.md:11 · goods.md:10 | Retained: days/daily/weeks are existing world-time prose. No wall-clock duration or engine unit; no mechanic/threshold changed. |
| battle.md:5,9 | Retained: days/daily/weeks are existing world-time prose. No wall-clock duration or engine unit; no mechanic/threshold changed. |
| founding.md:11,15 | Retained: days/daily/weeks are existing world-time prose. No wall-clock duration or engine unit; no mechanic/threshold changed. |
| report.md:9 | Retained: days/daily/weeks are existing world-time prose. No wall-clock duration or engine unit; no mechanic/threshold changed. |
| getting-in.md:10, time.md (sista stycket) | Retained: days/daily/weeks are existing world-time prose. No wall-clock duration or engine unit; no mechanic/threshold changed. |
| coming-back.md:1 | Fixed: arbitrary nine-hours opener replaced with While you were away, the world kept moving. |
| time.md:5 | Retained world-cadence explanation; added explicit clock/date support and day as the world unit. |
### Tabell 4 — Servertext som når spelaren (fel, notiser, API-fält)

| Original source row | Outcome |
|---|---|
| api/handlers/settlement.go:1088–1089 | Fixed: presentation helper renamed FormatDays, cooldown retains existing upward rounding and cadence. |
| internal/combat/expedition.go:50 | Fixed: player unit day; generated counts use common surface formatter, rates /day. Stable internal identifiers retained. |
| internal/combat/march_start.go:749 | Fixed: player unit day; generated counts use common surface formatter, rates /day. Stable internal identifiers retained. |
| internal/combat/march_start.go:763 | Fixed: player unit day; generated counts use common surface formatter, rates /day. Stable internal identifiers retained. |
| internal/combat/march_start.go:939 | Fixed: player unit day; generated counts use common surface formatter, rates /day. Stable internal identifiers retained. |
| internal/combat/unit_arrival.go:897 | Fixed: player unit day; generated counts use common surface formatter, rates /day. Stable internal identifiers retained. |
| api/handlers/settlement.go:1033 · province.go:2214 | Fixed: player unit day; generated counts use common surface formatter, rates /day. Stable internal identifiers retained. |
| api/handlers/province.go:2639–2640 | Fixed: player unit day; generated counts use common surface formatter, rates /day. Stable internal identifiers retained. |
| api/handlers/unit_march_preview.go:64 | Fixed: invalid expedition duration; query key ticks remains unchanged. |
| API-fält (konsumeras av klienterna) | Retained: stable duration_game_days/cooldown_remaining_game_days and tick-related JSON identifiers; never printed as player units. |
| internal/capabilities/kingdom_verbs.go:126–127 | Retained: names a wall-calendar weekday/timezone, not a duration in days; kingdoms remain disabled. |

105 investigation table rows accounted for. Additional findings corrected: server generic current-tick read errors, Keryx occupied-city description, historical display ages across letters/gossip, and shared singular/plural coverage.

## Proof package

- Baseline: c093fc11, all 519 JS tests and fresh DB tick/Keryx packages passed before edits.
- Final JS: **525/525**, no skips, including all existing Codex/misc/notification tests. `js.log`.
- Full Go: `tools/gotest.sh`, newly created PG16, migration 163, `-count=1 -p 1`, clean environment; `go.log`. No reused acceptance DB.
- `go vet ./...` and builds of Temenos/Keryx passed; `vet.log` is empty.
- **10 physical mutations:** game-day and wall-day literals on each of four surfaces, nested JS-template prose, extra occurrence of an exempt CLI identifier. Each fails a named guard assertion, byte-identical source restoration then passes. Logs in `mutations/`; reproduce with `python3 tools/day_mutations.py`.
- **36 pictures**: Firefox first, Chromium then WebKit, 1280×900 and 390×844; Economy, War, notifications, Host and both calendar alternatives. Real map/CSS/controllers with explicitly scripted HTTP, no live-game claim or live writes. Named fixtures: Wanax Agamemnon, Wanax Nestor, Mycenae, Tiryns, Bronze Guard, Sacred Dolphin. `browser/proof.json` contains request inventory, text, bounds and zero page errors/unexpected routes. Proof checks no UUIDs, forbidden units, NaN or undefined in rendered text. All primary day text stays readable; notification clock labels wrap separately at 390. Host retains scrolling Details and its fixed founding action.
- `identifier-proof.json`: JSON tags and SQL byte-identical in all 29 changed production Go files. `source-sha256.json` fingerprints production sources used by the proof.

### Review pictures

[Firefox Economy](browser/firefox-desktop-economy.png) · [Firefox War 390](browser/firefox-390-war.png) · [Firefox notifications 390](browser/firefox-390-notifications.png) · [Firefox Host 390](browser/firefox-390-host.png).

Calendar choice: [with ordinal](browser/firefox-desktop-calendar-with.png) · [without ordinal](browser/firefox-desktop-calendar-without.png). Also both alternatives at 390 and in Chromium/WebKit. Default remains with ordinal pending Timothy's choice.
