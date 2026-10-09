---
title: Megaron — verblistan (temenos ↔ keryx ↔ megaron)
description: "Enkel paritetslista: varje spelarverb och om det finns på servern, i CLI:t och i webben. UPPDATERAS SAMMA SLICE som ett verb skapas eller ändras."
metadata:
  type: reference
---

# Verblistan

> ⛔⛔ **UPPDATERA DEN HÄR FILEN I SAMMA SLICE som du skapar eller ändrar ett verb.**
> Inte efteråt, inte "när vi städar". Timothy 2026-09-05.
>
> **Ett verb finns på FYRA ställen och ändringen berör alla fyra** (Timothy 2026-09-25):
> **temenos** (servern — där funktionen bor) · **keryx** (CLI:t) · **megaron** (webben) ·
> **codex** (`web/static/codex/`, spelarens wiki i spelet — sanningen för spelaren).
> Att servern kan något är inte att spelaren kan det. [[feedback_keryx_surface]]:
> allt i temenos ska vara synligt OCH actionabelt i keryx. [[feedback_feature_order]]:
> ordningen är temenos → keryx → megaron, så webben får släpa *medvetet*, aldrig av förbiseende.
>
> ⚠️ **Föregångaren `keryx_cli.md` (2026-07-08) gick stale på två månader** — den listade 23 av
> dagens 69 kommandon och påstod att `craft` fungerade två månader efter att verbet avskaffats.
> Skälet var inte lättja: **ingen hade sagt att den skulle uppdateras.** Den här rutan är den enda
> skillnaden.

Mätt mot koden 2026-09-05 (`cmd/keryx/main.go` · `cmd/server/main.go`s routetabell ·
`fetchAuth`-anropen i `web/static/js/megaron/`).

> **Grind (2026-09-25, mergad `a3e063b`):** under `worlds.state='forming'` avslås varje skrivande verb
> med 409 *"The world has not begun — N of M Wanaxes have arrived"* (`world_guard.go` `RequireStartedWorld`,
> fail-closed för nya rutter). Undantag: join, reports, notiser read-all/delete, password, notisinställningar.

## Muterande verb — det spelaren gör

| Verb | temenos (rutt) | keryx | megaron | Not |
|---|---|---|---|---|
| build | `POST /provinces/:id/build` | `build` | ✓ | 2026-09-30 (byggnadsregeln): hex-väljare (webb) och `build --hex` (keryx) visar serverns `effect`/`upgrade_effect` från `placement-options`; Construct/Built/`--list`/`city` utan effektrader; Codex `buildings.md` har regeln · 2026-10-01 (omtag 1): effekttexten i ord (`grain production ×1.7 · space for 4 more workers`), **en byggnad per hex** — servern vägrar en annan hexbunden typ (egen, köad eller grannstadens), väljaren utesluter upptagna hexar |
| cancel-build | `DELETE /provinces/:id/build-queue/:id` | `cancel-build` | ✓ | |
| recruit | `POST /provinces/:id/recruit` | `recruit` | ✓ | 2026-10-04 arbetskopia: naval crew i capabilities; betalning/enhet/jobb/event atomiska. CLI/webb behåller befintlig payload, Codex units förtydligad. Testat på färskDB, ej deployat. [[megaron_kodgranskning_20261004]] §12 |
| place / staff | `POST/DELETE /provinces/:id/placements` | `place`, `staff` | ✓ | P5 |
| place → ta hex (gren av `place`) | `POST /provinces/:id/placements` på en hållen hex (tagbar när egen enhet står i fortify/sentry och hållaren saknar) → 201; `placement-options` bär `held_by`/`takeable`/`held_workers`/`held_building`; notis `HexTaken` till förloraren | `place` (serverns 409-text oförändrad), `city` markerar `held by`/`TAKEABLE`, `HexTaken` i notiser | ✓ citygrid *Take: …*-knapp, `format.js` | `catchment.md`, `stances.md`, `sieges.md` · 2026-10-02 (delad catchment, `megaron_plan_delad_catchment.md`) |
| labor (cult) | `PUT /provinces/:id/labor` | `allocate` | ✓ | **endast cult** sedan P4 |
| slaughter | `POST /provinces/:id/slaughter-livestock` | `slaughter` | ✓ | |
| disband | `POST /provinces/:id/disband` | `disband` | ✓ | |
| transfer | `POST /provinces/:id/trade` | `transfer` | ✓ | egna städer eller kontaktad främmande stad som gåva/tribut utan motprestation; Economy Transfer, gåvorad i Diplomacy, Codex gifts. S byggd på codex/gava, väntar granskning/BILD före merge. |
| abandon | `POST /settlements/:id/abandon` | `abandon` | ✓ | |
| rite | `POST /settlements/:id/rite` | `rite` | ✓ | |
| gift | `POST /settlements/:id/gift` | `gift` | ✓ | koloniens City-drawer, bredvid lojalitetsloggen — mergad `a3e063b` |
| password | `POST /auth/password` | `password` | ✓ | klick på Wanax-namnet → Account — mergad `a3e063b` 2026-09-25 |
| agora password | `POST /agora/password` | `agora password` | ✓ Account → Community chat | Byter chattlösenordet och visar det en gång; separat från spelkontots auth/password. |
| occupation-order | `POST /settlements/:id/occupation-order` | `occupation` | ✓ | knappar i dispatch-fönstret (CityOccupied/CityAnnexReady) — mergad `a3e063b` 2026-09-25 |
| return-army | `POST /settlements/:id/return-army` | — | — | **kingdom-verb, gatat** — grinden rättad och produktionsregistreringen testad 2026-10-04 i arbetskopian, ej deployat; legacy-SQL måste byggas om inför aktivering. Räknas inte, se nedan |
| march | `POST /units/:id/march` | `march`, `unit march` | ✓ | explore = områdesexpedition med `ticks`; alla fyra ytor LIVE 2026-10-07 (`6b7f200`, migration159). Kartmeny och War har serverstyrt längdval, keryx `--ticks`; båda visar uppdrag och hemkomstrapport. [[megaron_plan_upptackarexpeditionen]] |
| recall / redirect | `POST /units/:id/recall` | `recall --unit`, `recall --all`, `redirect` | ✓ War→Army Recall / Recall all | Recall all = en vanlig recall per marscherande enhet (klientsidan loopar). Ingen gruppering, filter eller ny serverrutt; kort summa och namngivna avslag. Byggd på codex/recall-all-enkel, BILD före merge återstår. R (`c7e8496c`, granskning återstår): expedition-recall till hemstad/garnison; vanlig recall till avfärdshex, redirect oförändrad. Keryx-hjälp + Codex marching uppdaterade; webb använder samma API. |
| stance | `POST /units/:id/stance` | `unit stance` | ✓ | på en MARSCHERANDE enhet: Runner (`stance_pursuit`) hinner ikapp som redirect, annars till målet; biter där enheten stannar — `4483f13` 2026-09-25 |
| retreat-order | `POST /units/:id/standing-orders` | `unit retreat-order` | ✓ | War → Army, syns bara när enheten är `in_battle`; override för DENNA strid. ⚠️ `retreat_at_loss` = andel KVAR, inte andel förlorad (webbens 25/75 var omkastade — lagat) — mergad `a3e063b` |
| retreat-default | `GET/PUT /worlds/:id/retreat-default` | `retreat-default` | ✓ | War → Army "When to retreat" (per Wanax, mig 145, sås in i varje ny strid); undantagen forming-grinden — mergad `a3e063b` |
| reinforce | `POST /units/:id/reinforce` | `reinforce` | ✓ | |
| repair | `POST /units/:id/repair` | `unit repair` | ✓ | |
| load / unload | `POST /units/:id/load`, `/unload` | `unit load`, `unload` | ✓ | sjötransport |
| join | `POST /worlds/:id/join` | `join` | ✓ | CLI-verbet tillkom 2026-09-05 |
| founding settle | `POST /founding/settle` | `founding settle` | ✓ | |
| message / reply | `POST /settlements/:id/messengers`, `POST /founding/messengers`, `/messengers/:id/reply` | `message` (även från host före grundning), `reply` | ✓ sändning från host eller stad | Host-utkorgens läsyta i webben är separat öppet fynd; Keryx outbox läser founding/messengers. |
| trade offer/accept/decline/cancel | `POST /messengers/:id/trade-*` | `trade-offer` m.fl. | ✓ | |
| arrange passage (skepp för ett väntande bud) | `POST /messengers/:id/passage` | `passage --id --ship` | ✓ (budets rad + dispatchen `PassageStalled`) | 3b-3/3b-4, 2026-09-27 |
| fetch by ship (hämta hem en fältenhet) | `POST /units/:id/pickup` | `unit pickup <enhet> --ship <skepp> [--wait N]` | ✓ ("Fetch by ship" på enhetskortet) | 2b, 2026-09-28 |
| call back (väntande bud i egen hamn) | `POST /messengers/:id/call-back` | `call-back --id` | ✓ (budets rad + dispatchen) | 3b-4, 2026-09-27 |
| **standing order** | `POST/DELETE /standing-orders[/:id]`, `/pause`, `/resume` | `route`, `routes`, `route-pause`, `route-resume`, `route-delete` | ✓ | byggd 2026-09-05 |
| report | `POST /reports` | `report` | ✓ | buggrapport |
| notification-preferences | `GET/PUT/DELETE /notification-preferences[/:kind]` | `dispatches [--mute/--unmute <Kind>]` | ✓ | dispatch-mute, webb 2026-09-05, keryx 2026-09-26 |
| mark notifications read (all) | `POST /notifications/read-all` | `notifications --mark-read` | ✓ arkivöppning utan typfilter | Markerar alla lästa; raderna finns kvar i arkivet. |
| mark notification read (one) | `POST /notifications/:id/read` | — (CLI har read-all) | ✓ dispatch-chip | Markerar en notis läst, inte hela arkivet; inget separat per-item-anrop i Keryx. |

⚠️ **Kingdom-verben** (`kingdom-*`, 10 st i keryx, rutter under `/kingdoms/`) är **POST-MVP och
gatade** bakom `KINGDOMS_ENABLED` — all spelaryta avstängd med flit (Timothy 2026-07-08). De hör
inte i paritetsräkningen förrän gaten öppnas.

## Läsytor — det spelaren ser

Webbens ⌕ söker också i befintlig inbox och ryktesflöde (högst 30 per källa); Enter öppnar
Correspondence/Gossip. Serverns och keryx befintliga läsytor äger urvalet; inget nytt verb.
Bevis: repo `docs/reviews/search-messages/README.md` (`42844152`).

`status` · `map` · `cities` · `city` · `goods` · `settlements` · `units`/`unit list` · `army` ·
`sightings` · `notifications` · `inbox`/`outbox` · `gossip` · `actions` · `ticklog` · `brief` ·
`idle` · `wants` · `worlds` · `reports` (admin) · `watch`.
Webben täcker dessa genom sina drawers; **läsparitet är inte lika kritisk som verbparitet**, men ett
verb utan läsyta är lika obrukbart som en läsyta utan verb.

| Läsyta | temenos (rutt) | keryx | megaron | Not |
|---|---|---|---|---|
| community chat account | `GET /agora` | `agora` | ✓ Account → Community chat | Konto/status gäller utanför vald spelvärld; artikeln community-chat i Codex. |

## Öppna luckor just nu (2026-09-26)

**Inga.** `notification-preferences` stängd 2026-09-26 (`keryx dispatches`).

*(Stängda 2026-09-25, `a3e063b`: gift · occupation-order · retreat-order · retreat-default · password ·
land-explore · redirect via kartan. `return-army` är ett kingdom-verb och räknas inte.)*
