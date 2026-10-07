# Statisk verbparitet

Input SHA256: `f02422e7ca3e02799dc8df7d2e840cfe07bda79d4a96a83e7271c1109fe9a83d`

✓ = källanrop; Codex ✓ = uttryckligt rutt-/aliasomnämnande, inte fullständigt beteendebevis.
— = ingen statisk evidens hittad; kontrollera olösta anrop före en verklig lucka hävdas.

| Serverrutt | Keryx | Webb | Codex |
|---|---|---|---|
| GET /api/v1/auth/me | ✓ server/cmd/keryx/cmd_status.go:1123 | ✓ web/static/js/megaron/main.js:283 | ✓ web/static/codex/getting-in.md:3 |
| POST /api/v1/auth/password | ✓ server/cmd/keryx/cmd_password.go:37 | ✓ web/static/js/megaron/ui/account_window.js:102 | ✓ web/static/codex/community-chat.md:7 |
| GET /api/v1/agora | ✓ server/cmd/keryx/cmd_agora.go:22 | ✓ web/static/js/megaron/api.js:74 | ✓ web/static/codex/community-chat.md:1 |
| POST /api/v1/agora/password | ✓ server/cmd/keryx/cmd_agora.go:51 | ✓ web/static/js/megaron/api.js:75 | ✓ web/static/codex/community-chat.md:7 |
| GET /api/v1/buildings | ✓ server/cmd/keryx/cmd_build.go:70 | ✓ web/static/js/megaron/ui/drawers/city.js:610 | ✓ web/static/codex/buildings.md:1 |
| GET /api/v1/units | ✓ server/cmd/keryx/cmd_recruit.go:189 | ✓ web/static/js/megaron/ui/drawers/war.js:38 | ✓ web/static/codex/units.md:16 |
| GET /api/v1/recipes | ✓ server/cmd/keryx/cmd_status.go:273 | ✓ web/static/js/megaron/ui/drawers/city.js:586 | ✓ web/static/codex/bronze.md:3 |
| GET /api/v1/goods | — | ✓ web/static/js/megaron/ui/drawers/diplomacy.js:29 | ✓ web/static/codex/goods.md:1 |
| GET /api/v1/notification-preferences | ✓ server/cmd/keryx/cmd_dispatches.go:58 | ✓ web/static/js/megaron/ui/dispatch_window.js:319 | ✓ web/static/codex/coming-back.md:12 |
| PUT /api/v1/notification-preferences/{id} | ✓ server/cmd/keryx/cmd_dispatches.go:46 | ✓ web/static/js/megaron/ui/dispatch_window.js:325 | ✓ web/static/codex/coming-back.md:12 |
| DELETE /api/v1/notification-preferences/{id} | ✓ server/cmd/keryx/cmd_dispatches.go:52 | ✓ web/static/js/megaron/ui/dispatch_window.js:325 | ✓ web/static/codex/coming-back.md:12 |
| GET /api/v1/worlds | ✓ server/cmd/keryx/cmd_login.go:126 | — | ✓ web/static/codex/getting-in.md:6 |
| GET /api/v1/worlds/{id} | ✓ server/cmd/keryx/client.go:148 | ✓ web/static/js/megaron/main.js:298 | ✓ web/static/codex/getting-in.md:6 |
| GET /api/v1/worlds/{id}/map | ✓ server/cmd/keryx/cmd_map.go:101 | ✓ web/static/js/megaron/render/map.js:3908 | ✓ web/static/codex/screen.md:7 |
| GET /api/v1/worlds/{id}/colonize-preview | ✓ server/cmd/keryx/cmd_unit.go:904 | ✓ web/static/js/megaron/render/map.js:4262 | ✓ web/static/codex/colonies.md:5 |
| GET /api/v1/worlds/{id}/provinces | ✓ server/cmd/keryx/cmd_brief.go:23 | ✓ web/static/js/megaron/main.js:299 | ✓ web/static/codex/city.md:1 |
| GET /api/v1/worlds/{id}/marches | — | ✓ web/static/js/megaron/render/map.js:3910 | ✓ web/static/codex/marching.md:1 |
| GET /api/v1/worlds/{id}/messengers | — | ✓ web/static/js/megaron/render/map.js:3911 | ✓ web/static/codex/messengers.md:1 |
| GET /api/v1/worlds/{id}/trades | ✓ server/cmd/keryx/cmd_goods.go:259 | ✓ web/static/js/megaron/render/map.js:3912 | ✓ web/static/codex/trade.md:20 |
| GET /api/v1/worlds/{id}/foreign-units | ✓ server/cmd/keryx/cmd_sightings.go:108 | ✓ web/static/js/megaron/render/map.js:3915 | ✓ web/static/codex/sight.md:1 |
| GET /api/v1/worlds/{id}/rural-projections | — | ✓ web/static/js/megaron/render/map.js:3914 | ✓ web/static/codex/catchment.md:3 |
| GET /api/v1/worlds/{id}/wanaxes | ✓ server/cmd/keryx/cmd_gossip.go:102 | ✓ web/static/js/megaron/ui/drawers/gossip.js:86 | ✓ web/static/codex/messengers.md:1 |
| GET /api/v1/worlds/{id}/cities | ✓ server/cmd/keryx/cmd_cities.go:26 | ✓ web/static/js/megaron/ui/drawers/diplomacy.js:99 | ✓ web/static/codex/city.md:1 |
| GET /api/v1/worlds/{id}/diplomacy | ✓ server/cmd/keryx/cmd_diplomacy.go:17 | ✓ web/static/js/megaron/ui/drawers/diplomacy.js:122 | ✓ web/static/codex/messengers.md:1 |
| GET /api/v1/worlds/{id}/provinces/{id} | ✓ server/cmd/keryx/cmd_allocate.go:207 | ✓ web/static/js/megaron/ui/drawers/city.js:139 | ✓ web/static/codex/city.md:1 |
| GET /api/v1/worlds/{id}/provinces/{id}/actions | ✓ server/cmd/keryx/cmd_actions.go:76 | ✓ web/static/js/megaron/ui/misc.js:559 | ✓ web/static/codex/buildings.md:1 |
| GET /api/v1/worlds/{id}/provinces/{id}/army | — | ✓ web/static/js/megaron/render/map.js:4183 | ✓ web/static/codex/units.md:1 |
| GET /api/v1/worlds/{id}/provinces/{id}/buildings | ✓ server/cmd/keryx/cmd_staff.go:197 | — | ✓ web/static/codex/buildings.md:1 |
| GET /api/v1/worlds/{id}/provinces/{id}/goods | ✓ server/cmd/keryx/cmd_allocate.go:91 | ✓ web/static/js/megaron/ui/drawers/city.js:234 | ✓ web/static/codex/goods.md:1 |
| GET /api/v1/worlds/{id}/provinces/{id}/ticklog | ✓ server/cmd/keryx/cmd_ticklog.go:44 | ✓ web/static/js/megaron/ui/drawers/city.js:720 | ✓ web/static/codex/time.md:1 |
| POST /api/v1/worlds/{id}/provinces/{id}/build | ✓ server/cmd/keryx/cmd_build.go:189 | ✓ web/static/js/megaron/ui/drawers/city.js:530 | ✓ web/static/codex/buildings.md:1 |
| DELETE /api/v1/worlds/{id}/provinces/{id}/build-queue/{id} | ✓ server/cmd/keryx/cmd_build.go:277 | ✓ web/static/js/megaron/ui/drawers/city.js:740 | ✓ web/static/codex/buildings.md:1 |
| POST /api/v1/worlds/{id}/provinces/{id}/recruit | ✓ server/cmd/keryx/cmd_recruit.go:103 | ✓ web/static/js/megaron/ui/drawers/war.js:353 | ✓ web/static/codex/units.md:1 |
| GET /api/v1/worlds/{id}/provinces/{id}/trade | — | — | ✓ web/static/codex/transfers.md:1 |
| POST /api/v1/worlds/{id}/provinces/{id}/trade | ✓ server/cmd/keryx/cmd_goods.go:200 | ✓ web/static/js/megaron/ui/drawers/economy.js:382 | ✓ web/static/codex/transfers.md:1 |
| POST /api/v1/worlds/{id}/provinces/{id}/disband | ✓ server/cmd/keryx/cmd_disband.go:82 | ✓ web/static/js/megaron/ui/drawers/war.js:401 | ✓ web/static/codex/units.md:24 |
| PUT /api/v1/worlds/{id}/provinces/{id}/labor | ✓ server/cmd/keryx/cmd_allocate.go:59 | ✓ web/static/js/megaron/ui/drawers/city.js:125 | ✓ web/static/codex/temples.md:3 |
| GET /api/v1/worlds/{id}/provinces/{id}/placement-options | ✓ server/cmd/keryx/cmd_gubbe.go:109 | ✓ web/static/js/megaron/ui/citygrid.js:225 | ✓ web/static/codex/catchment.md:3 |
| GET /api/v1/worlds/{id}/provinces/{id}/placements | ✓ server/cmd/keryx/cmd_status.go:1236 | ✓ web/static/js/megaron/ui/citygrid.js:151 | ✓ web/static/codex/catchment.md:3 |
| POST /api/v1/worlds/{id}/provinces/{id}/placements | ✓ server/cmd/keryx/cmd_place.go:86 | ✓ web/static/js/megaron/ui/citygrid.js:137 | ✓ web/static/codex/catchment.md:3 |
| DELETE /api/v1/worlds/{id}/provinces/{id}/placements/{id} | ✓ server/cmd/keryx/cmd_place.go:192 | ✓ web/static/js/megaron/ui/citygrid.js:163 | ✓ web/static/codex/catchment.md:3 |
| POST /api/v1/worlds/{id}/provinces/{id}/slaughter-livestock | ✓ server/cmd/keryx/cmd_slaughter.go:39 | ✓ web/static/js/megaron/ui/drawers/city.js:489 | ✓ web/static/codex/city.md:9 |
| POST /api/v1/worlds/{id}/standing-orders | ✓ server/cmd/keryx/cmd_route.go:120 | ✓ web/static/js/megaron/ui/drawers/economy.js:516 | ✓ web/static/codex/battle.md:10 |
| GET /api/v1/worlds/{id}/standing-orders | ✓ server/cmd/keryx/cmd_route.go:161 | ✓ web/static/js/megaron/ui/drawers/economy.js:493 | ✓ web/static/codex/battle.md:10 |
| POST /api/v1/worlds/{id}/standing-orders/{id}/pause | ✓ server/cmd/keryx/cmd_route.go:207 | ✓ web/static/js/megaron/ui/drawers/economy.js:532 | ✓ web/static/codex/routes.md:8 |
| POST /api/v1/worlds/{id}/standing-orders/{id}/resume | ✓ server/cmd/keryx/cmd_route.go:207 | ✓ web/static/js/megaron/ui/drawers/economy.js:537 | ✓ web/static/codex/routes.md:8 |
| DELETE /api/v1/worlds/{id}/standing-orders/{id} | ✓ server/cmd/keryx/cmd_route.go:242 | ✓ web/static/js/megaron/ui/drawers/economy.js:542 | ✓ web/static/codex/battle.md:10 |
| GET /api/v1/worlds/{id}/market/wants | ✓ server/cmd/keryx/cmd_wants.go:28 | ✓ web/static/js/megaron/ui/drawers/economy.js:550 | ✓ web/static/codex/trade.md:7 |
| POST /api/v1/worlds/{id}/founding/settle | ✓ server/cmd/keryx/cmd_founding.go:240 | ✓ web/static/js/megaron/render/map.js:4398 | ✓ web/static/codex/founding.md:1 |
| POST /api/v1/worlds/{id}/founding/messengers | ✓ server/cmd/keryx/cmd_messenger_split.go:52 | ✓ web/static/js/megaron/render/map.js:4488 | ✓ web/static/codex/messengers.md:1 |
| GET /api/v1/worlds/{id}/founding/messengers | ✓ server/cmd/keryx/cmd_messenger.go:280 | — | ✓ web/static/codex/messengers.md:1 |
| GET /api/v1/worlds/{id}/founding/status | ✓ server/cmd/keryx/cmd_founding.go:48 | ✓ web/static/js/megaron/main.js:329 | ✓ web/static/codex/founding.md:1 |
| GET /api/v1/worlds/{id}/units | ✓ server/cmd/keryx/cmd_actions.go:205 | ✓ web/static/js/megaron/render/map.js:3913 | ✓ web/static/codex/units.md:16 |
| POST /api/v1/worlds/{id}/units/{id}/march | ✓ server/cmd/keryx/cmd_unit.go:80 | ✓ web/static/js/megaron/ui/drawers/war.js:900 | ✓ web/static/codex/marching.md:1 |
| GET /api/v1/worlds/{id}/units/{id}/march-preview | ✓ server/cmd/keryx/cmd_march_preview.go:15 | ✓ web/static/js/megaron/ui/march_preview.js:12 | ✓ web/static/codex/marching.md:11 |
| POST /api/v1/worlds/{id}/units/{id}/recall | ✓ server/cmd/keryx/cmd_unit.go:1099 | ✓ web/static/js/megaron/ui/drawers/war.js:723 | ✓ web/static/codex/marching.md:21 |
| POST /api/v1/worlds/{id}/units/{id}/stance | ✓ server/cmd/keryx/cmd_unit.go:1362 | ✓ web/static/js/megaron/ui/drawers/war.js:929 | ✓ web/static/codex/stances.md:1 |
| POST /api/v1/worlds/{id}/units/{id}/standing-orders | ✓ server/cmd/keryx/cmd_unit.go:1473 | ✓ web/static/js/megaron/ui/drawers/war.js:965 | ✓ web/static/codex/battle.md:10 |
| GET /api/v1/worlds/{id}/retreat-default | ✓ server/cmd/keryx/cmd_retreat_default.go:51 | ✓ web/static/js/megaron/ui/drawers/war.js:988 | ✓ web/static/codex/battle.md:10 |
| PUT /api/v1/worlds/{id}/retreat-default | ✓ server/cmd/keryx/cmd_retreat_default.go:62 | ✓ web/static/js/megaron/ui/drawers/war.js:1007 | ✓ web/static/codex/battle.md:10 |
| POST /api/v1/worlds/{id}/units/{id}/load | ✓ server/cmd/keryx/cmd_unit.go:1533 | ✓ web/static/js/megaron/ui/drawers/war.js:1072 | ✓ web/static/codex/sea.md:15 |
| POST /api/v1/worlds/{id}/units/{id}/unload | ✓ server/cmd/keryx/cmd_unit.go:1571 | ✓ web/static/js/megaron/ui/drawers/war.js:1087 | ✓ web/static/codex/sea.md:15 |
| POST /api/v1/worlds/{id}/units/{id}/pickup | ✓ server/cmd/keryx/cmd_unit.go:1626 | ✓ web/static/js/megaron/ui/drawers/war.js:1119 | ✓ web/static/codex/sea.md:63 |
| POST /api/v1/worlds/{id}/units/{id}/reinforce | ✓ server/cmd/keryx/cmd_unit.go:1311 | ✓ web/static/js/megaron/ui/drawers/war.js:1029 | ✓ web/static/codex/units.md:28 |
| POST /api/v1/worlds/{id}/units/{id}/repair | ✓ server/cmd/keryx/cmd_unit.go:1202 | ✓ web/static/js/megaron/ui/drawers/war.js:1144 | ✓ web/static/codex/sea.md:22 |
| GET /api/v1/worlds/{id}/settlements | — | — | ✓ web/static/codex/city.md:1 |
| GET /api/v1/worlds/{id}/settlements/overview | — | ✓ web/static/js/megaron/ui/drawers/economy.js:214 | ✓ web/static/codex/city.md:1 |
| GET /api/v1/worlds/{id}/settlements/placement-roster | ✓ server/cmd/keryx/cmd_roster.go:53 | ✓ web/static/js/megaron/render/map.js:3916 | ✓ web/static/codex/catchment.md:3 |
| GET /api/v1/worlds/{id}/settlements/{id} | — | ✓ web/static/js/megaron/ui/drawers/kult.js:27 | ✓ web/static/codex/city.md:1 |
| POST /api/v1/worlds/{id}/settlements/{id}/occupation-order | ✓ server/cmd/keryx/cmd_occupation.go:58 | ✓ web/static/js/megaron/ui/dispatch_window.js:131 | ✓ web/static/codex/occupation.md:5 |
| POST /api/v1/worlds/{id}/settlements/{id}/gift | ✓ server/cmd/keryx/cmd_goods.go:339 | ✓ web/static/js/megaron/ui/drawers/city.js:81 | ✓ web/static/codex/loyalty.md:9 |
| GET /api/v1/worlds/{id}/settlements/{id}/loyalty-log | ✓ server/cmd/keryx/cmd_status.go:1170 | ✓ web/static/js/megaron/ui/drawers/city.js:99 | ✓ web/static/codex/loyalty.md:1 |
| POST /api/v1/worlds/{id}/settlements/{id}/rite | ✓ server/cmd/keryx/cmd_rite.go:226 | ✓ web/static/js/megaron/ui/drawers/kult.js:182 | ✓ web/static/codex/rites.md:1 |
| POST /api/v1/worlds/{id}/settlements/{id}/abandon | ✓ server/cmd/keryx/cmd_abandon.go:29 | ✓ web/static/js/megaron/ui/drawers/war.js:696 | — |
| GET /api/v1/worlds/{id}/gossip | ✓ server/cmd/keryx/cmd_gossip.go:21 | ✓ web/static/js/megaron/ui/drawers/gossip.js:85 | ✓ web/static/codex/gossip.md:1 |
| POST /api/v1/worlds/{id}/settlements/{id}/messengers | ✓ server/cmd/keryx/cmd_gossip.go:124 | ✓ web/static/js/megaron/render/map.js:4488 | ✓ web/static/codex/messengers.md:1 |
| GET /api/v1/worlds/{id}/settlements/{id}/messengers | ✓ server/cmd/keryx/cmd_messenger.go:291 | ✓ web/static/js/megaron/ui/drawers/diplomacy.js:148 | ✓ web/static/codex/messengers.md:1 |
| GET /api/v1/worlds/{id}/messengers/inbox | ✓ server/cmd/keryx/cmd_marches.go:19 | ✓ web/static/js/megaron/main.js:337 | ✓ web/static/codex/messengers.md:1 |
| POST /api/v1/worlds/{id}/messengers/{id}/reply | ✓ server/cmd/keryx/cmd_messenger.go:26 | ✓ web/static/js/megaron/ui/drawers/diplomacy.js:483 | ✓ web/static/codex/messengers.md:1 |
| POST /api/v1/worlds/{id}/messengers/{id}/trade-accept | ✓ server/cmd/keryx/cmd_messenger.go:61 | ✓ web/static/js/megaron/ui/drawers/diplomacy.js:443 | ✓ web/static/codex/trade.md:14 |
| POST /api/v1/worlds/{id}/messengers/{id}/trade-decline | ✓ server/cmd/keryx/cmd_messenger.go:105 | ✓ web/static/js/megaron/ui/drawers/diplomacy.js:471 | ✓ web/static/codex/trade.md:14 |
| POST /api/v1/worlds/{id}/messengers/{id}/trade-cancel | ✓ server/cmd/keryx/cmd_messenger.go:134 | ✓ web/static/js/megaron/ui/drawers/diplomacy.js:429 | ✓ web/static/codex/trade.md:14 |
| POST /api/v1/worlds/{id}/messengers/{id}/passage | ✓ server/cmd/keryx/cmd_messenger.go:181 | ✓ web/static/js/megaron/ui/dispatch_window.js:186 | ✓ web/static/codex/sea.md:5 |
| POST /api/v1/worlds/{id}/messengers/{id}/call-back | ✓ server/cmd/keryx/cmd_messenger.go:225 | ✓ web/static/js/megaron/ui/dispatch_window.js:202 | — |
| GET /api/v1/worlds/{id}/notifications | ✓ server/cmd/keryx/cmd_notifications.go:811 | ✓ web/static/js/megaron/ui/chips.js:42 | ✓ web/static/codex/coming-back.md:7 |
| POST /api/v1/worlds/{id}/notifications/read-all | ✓ server/cmd/keryx/cmd_notifications.go:780 | ✓ web/static/js/megaron/ui/drawers/notif.js:76 | ✓ web/static/codex/coming-back.md:11 |
| POST /api/v1/worlds/{id}/notifications/{id}/read | — | ✓ web/static/js/megaron/ui/chips.js:102 | ✓ web/static/codex/coming-back.md:11 |
| POST /api/v1/worlds/{id}/reports | ✓ server/cmd/keryx/cmd_reports.go:140 | ✓ web/static/js/megaron/ui/drawers/report.js:73 | ✓ web/static/codex/report.md:3 |

## Saknar statisk evidens: keryx

- GET /api/v1/goods
- GET /api/v1/worlds/{id}/marches
- GET /api/v1/worlds/{id}/messengers
- GET /api/v1/worlds/{id}/rural-projections
- GET /api/v1/worlds/{id}/provinces/{id}/army
- GET /api/v1/worlds/{id}/provinces/{id}/trade
- GET /api/v1/worlds/{id}/settlements
- GET /api/v1/worlds/{id}/settlements/overview
- GET /api/v1/worlds/{id}/settlements/{id}
- POST /api/v1/worlds/{id}/notifications/{id}/read

## Saknar statisk evidens: webb

- GET /api/v1/worlds
- GET /api/v1/worlds/{id}/provinces/{id}/buildings
- GET /api/v1/worlds/{id}/provinces/{id}/trade
- GET /api/v1/worlds/{id}/founding/messengers
- GET /api/v1/worlds/{id}/settlements

## Saknar statisk evidens: codex

- POST /api/v1/worlds/{id}/settlements/{id}/abandon
- POST /api/v1/worlds/{id}/messengers/{id}/call-back

## Dokumenterade undantag

- POST /api/v1/auth/register: Web via templates/index.html:186, outside requested megaron JS module scan; CLI has no account registration
- POST /api/v1/auth/login: Web via templates/index.html:172, outside module scan; CLI login present
- POST /api/v1/auth/refresh: Auth transport token refresh, internal not a player verb
- GET /api/v1/admin/worlds/{id}/god-view: Admin, intentional outside player parity
- GET /api/v1/admin/worlds/{id}/reports: Admin, intentional outside player parity
- POST /api/v1/admin/worlds/{id}/backfill-placements: Admin, intentional outside player parity
- POST /api/v1/worlds: WorldHandler.Create now requires X-Admin-Key fail-closed (bc15f01d); administration, not a player verb
- POST /api/v1/worlds/{id}/join: Web via templates/join.html:31, outside module scan; CLI join present
- GET /api/v1/worlds/{id}/kingdoms: KINGDOMS_ENABLED gated, post-MVP
- POST /api/v1/worlds/{id}/kingdoms: KINGDOMS_ENABLED gated, post-MVP
- GET /api/v1/worlds/{id}/kingdoms/invitations: KINGDOMS_ENABLED gated, post-MVP
- POST /api/v1/worlds/{id}/kingdoms/{id}/invite: KINGDOMS_ENABLED gated, post-MVP
- POST /api/v1/worlds/{id}/kingdoms/{id}/join: KINGDOMS_ENABLED gated, post-MVP
- DELETE /api/v1/worlds/{id}/kingdoms/{id}/leave: KINGDOMS_ENABLED gated, post-MVP
- GET /api/v1/worlds/{id}/kingdoms/{id}/council: KINGDOMS_ENABLED gated, post-MVP
- PATCH /api/v1/worlds/{id}/kingdoms/{id}/council/{id}: KINGDOMS_ENABLED gated, post-MVP
- POST /api/v1/worlds/{id}/kingdoms/{id}/borrow-army: KINGDOMS_ENABLED gated, post-MVP
- POST /api/v1/worlds/{id}/kingdoms/{id}/election: KINGDOMS_ENABLED gated, post-MVP
- POST /api/v1/worlds/{id}/kingdoms/{id}/vote: KINGDOMS_ENABLED gated, post-MVP
- GET /api/v1/worlds/{id}/kingdoms/{id}/election: KINGDOMS_ENABLED gated, post-MVP
- GET /api/v1/worlds/{id}/kingdoms/{id}/borrowed-armies: KINGDOMS_ENABLED gated, post-MVP
- POST /api/v1/worlds/{id}/kingdoms/{id}/treasury/deposit: KINGDOMS_ENABLED gated, post-MVP
- DELETE /api/v1/worlds/{id}/notifications: Archive deletion intentionally removed from web; notif.js documents server legacy endpoint
- POST /api/v1/worlds/{id}/settlements/{id}/return-army: requireKingdomsEnabled, post-MVP

## I verblistan, inte registrerad

- POST /api/v1/worlds/{id}/units/recall

## Registrerad, inget uttryckligt ruttmönster i verblistan

- GET /api/v1/agora
- GET /api/v1/auth/me
- GET /api/v1/buildings
- GET /api/v1/goods
- GET /api/v1/recipes
- GET /api/v1/units
- GET /api/v1/worlds
- GET /api/v1/worlds/{id}
- GET /api/v1/worlds/{id}/cities
- GET /api/v1/worlds/{id}/colonize-preview
- GET /api/v1/worlds/{id}/diplomacy
- GET /api/v1/worlds/{id}/foreign-units
- GET /api/v1/worlds/{id}/founding/messengers
- GET /api/v1/worlds/{id}/founding/status
- GET /api/v1/worlds/{id}/gossip
- GET /api/v1/worlds/{id}/map
- GET /api/v1/worlds/{id}/marches
- GET /api/v1/worlds/{id}/market/wants
- GET /api/v1/worlds/{id}/messengers
- GET /api/v1/worlds/{id}/messengers/inbox
- GET /api/v1/worlds/{id}/notifications
- GET /api/v1/worlds/{id}/provinces
- GET /api/v1/worlds/{id}/provinces/{id}
- GET /api/v1/worlds/{id}/provinces/{id}/actions
- GET /api/v1/worlds/{id}/provinces/{id}/army
- GET /api/v1/worlds/{id}/provinces/{id}/buildings
- GET /api/v1/worlds/{id}/provinces/{id}/goods
- GET /api/v1/worlds/{id}/provinces/{id}/placement-options
- GET /api/v1/worlds/{id}/provinces/{id}/placements
- GET /api/v1/worlds/{id}/provinces/{id}/ticklog
- GET /api/v1/worlds/{id}/provinces/{id}/trade
- GET /api/v1/worlds/{id}/rural-projections
- GET /api/v1/worlds/{id}/settlements
- GET /api/v1/worlds/{id}/settlements/overview
- GET /api/v1/worlds/{id}/settlements/placement-roster
- GET /api/v1/worlds/{id}/settlements/{id}
- GET /api/v1/worlds/{id}/settlements/{id}/loyalty-log
- GET /api/v1/worlds/{id}/settlements/{id}/messengers
- GET /api/v1/worlds/{id}/standing-orders
- GET /api/v1/worlds/{id}/trades
- GET /api/v1/worlds/{id}/units
- GET /api/v1/worlds/{id}/units/{id}/march-preview
- GET /api/v1/worlds/{id}/wanaxes
- POST /api/v1/agora/password
- POST /api/v1/worlds/{id}/founding/messengers
- POST /api/v1/worlds/{id}/notifications/read-all
- POST /api/v1/worlds/{id}/notifications/{id}/read

## Klientanrop utan registrerad rutt (även dynamiska kandidater)

- webb: GET /api/v1/worlds/{id}/{id}

## Olösta anrop (parserbegränsning, inte ytlucka)


## Generisk transport/statiska assets (dokumenterade undantag)

- {"file": "server/cmd/keryx/client.go", "line": 163, "call": "do", "paths": ["<id>"], "methods": ["GET"], "reasons": ["Generic HTTP transport; actual get/post/put/delete callers inventoried"]}
- {"file": "server/cmd/keryx/client.go", "line": 174, "call": "do", "paths": ["<id>"], "methods": ["POST"], "reasons": ["Generic HTTP transport; actual get/post/put/delete callers inventoried"]}
- {"file": "server/cmd/keryx/client.go", "line": 185, "call": "do", "paths": ["<id>"], "methods": ["PUT"], "reasons": ["Generic HTTP transport; actual get/post/put/delete callers inventoried"]}
- {"file": "server/cmd/keryx/client.go", "line": 196, "call": "do", "paths": ["<id>"], "methods": ["PATCH"], "reasons": ["Generic HTTP transport; actual get/post/put/delete callers inventoried"]}
- {"file": "server/cmd/keryx/client.go", "line": 207, "call": "do", "paths": ["<id>"], "methods": ["DELETE"], "reasons": ["Generic HTTP transport; actual get/post/put/delete callers inventoried"]}
- {"file": "web/static/js/megaron/api.js", "line": 38, "call": "fetch", "paths": ["<id><id>"], "methods": ["GET"], "reasons": ["fetchAuth transport; concrete callers inventoried"]}
- {"file": "web/static/js/megaron/main.js", "line": 272, "call": "fetch", "paths": ["<id><id>"], "methods": ["GET"], "reasons": ["bootstrap get transport; concrete get callers inventoried"]}
- {"file": "web/static/js/megaron/ui/codex.js", "line": 175, "call": "fetch", "paths": ["/static/codex/index.json"], "methods": ["GET"], "reasons": ["Static article/index asset loading, not player API"]}
- {"file": "web/static/js/megaron/ui/codex.js", "line": 188, "call": "fetch", "paths": ["/static/codex/index.json"], "methods": ["GET"], "reasons": ["Static article/index asset loading, not player API"]}
- {"file": "web/static/js/megaron/ui/codex.js", "line": 193, "call": "fetch", "paths": ["/static/codex/<id>.md"], "methods": ["GET"], "reasons": ["Static article/index asset loading, not player API"]}
