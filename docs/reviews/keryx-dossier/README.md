# keryx dossier — process report (Sonnet, branch `sonnet/keryx-dossier`)

Order: `.agents/order-keryx-dossier-20261104.md`. Client code only: no endpoint, no migration, no server change.

## Inventory (written before the code)

Every section is built from the player's own GET endpoints, so the fog of war applies by itself. No admin endpoint is used.

| Section | GET calls | Reused from keryx |
|---|---|---|
| header | `/me`, `/worlds/{id}` (name, `tick_seconds`) | `keryxTZ` |
| Failed orders | `/notifications?unread=true&exclude=<noisy kinds>` | `printNotificationDetail` (output captured, `grabPrinted`), `notificationDay` |
| Urgent | same reply | same |
| Cities | `/provinces` (own markers), `/provinces/{id}`, `/provinces/{id}/placement-options` | `fetchPlacementOptions`, `buildingLabel`, `buildQueueState`, `formatDays`, `formatNetPerTick`, `resource` |
| Forces and orders on the way | `/units`, `/standing-orders`, `/runners`, `/provinces` (homes) | `unitRow`, `locationStr`, `formatSize`, `ordersEnRouteLine`, `renderRunners`, `fetchOwnSettlementPositions` |
| Known places | `/cities`, `/places`, `/map` | `placeKindLabel`, `hexDist` |
| Contacts | `/diplomacy`, `/cities` | none |
| Storms at sea | `/storms` (+ `/units`, `/cities` for the ship/coast test) | `formatStorms` |
| Since you last read | notifications reply | `printNotificationDetail` |
| What you can do now | `/provinces/{capital}/actions` | `actionVerb`, `categoryOrder` |

Gaps and choices (nothing was added to the server):

- **Urgent kinds** (a fixed list, `dossierUrgentKinds`): `FoodRunningLow`, `SubsistenceWarning`, `GarrisonStarvationStarted`, `CivilianStarvationStarted` (the two starvation starts are the same hunger as the food warnings), `ForeignMarchSighted`, `ForeignMarchSightedV2` (foreign marches seen), `ShipStormDamaged`, `ShipFoundered` (storm on own hull), `MessengerLostAtSea` (lost messengers). `OrderFailed` has its own section and is never repeated. Everything else unread goes to "Since you last read".
- **"Storm warning on your own hull"** is not a notification: the warning is a field in march and transfer previews (`storm_warning`), computed per order. So Urgent carries the storm *events* (`ShipStormDamaged`, `ShipFoundered`), and the section "Storms at sea" carries the map of known storms. No preview call was added (that is a per-order GET with a unit and a target).
- **Standing orders** in the order are the standing *trade routes* (`/standing-orders`); they are listed with their pause reason. Retreat orders and stance are shown per unit (`stance`).
- **Known places**: capped (15 cities, 15 named places, 10 deposits, 15 unnamed places, each with "…and N more — `keryx …`"). Deposits come from visible `/map` tiles; "terrain/deposit" for cities comes from `/cities`. Distance is hexes from the nearest own city. Rumour-only cities are left out (they have no coordinates).
- **Storms at sea** reads `/storms` only when the player has a ship or an own coastal city; otherwise it says "None. You have no ship and no coastal city." The `/storms` reply was checked against master `e8710588` (the storm life cycle: `tier` is `live` or `remembered`, a dead storm is never `live`): `formatStorms` already reads exactly that, so the section reuses it as is.
- The notification detail printers write to stdout; the dossier captures them (`grabPrinted`) instead of keeping a second copy of every sentence.

## What is built

- `server/cmd/keryx/cmd_dossier.go` and `cmd_dossier_test.go`; registered in `main.go`.
- `keryx dossier [--out FILE]`; `--json` is refused with "dossier is markdown; use the individual commands with --json". Only GET.
- Codex: a paragraph in `web/static/codex/keryx.md` (and the line in the command list), `articles.json` regenerated with `tools/codex_facts.py`.
- Vault: `megaron_verblista.md` (read view, keryx + Codex), `keryx_botmanual.md` (tier 0 in the orientation loop, the minimal turn, the command list).
- Web: nothing (Timothy 2026-11-05: no download button). temenos: nothing.

## Proof

Provenance of the example: `rigg: commit e871058 · migration 249/249 · värld 1312f0dd · tick 5419 · TICK_SECONDS=60 · 2026-11-07 10:20`
(the rig was in a crash loop on an old image when I started; `tools/acceptance.sh up --build` from this worktree rebuilt it at master `e8710588`).
The rig world was a founder phase with no cities, so I founded Aigina, marched one cohort out as a forward post and colonised Thera with the other.

- **Size:** `docs/reviews/keryx-dossier/example.md`, produced by `keryx dossier --out world.md` on the rig, is **65 lines** (1962 bytes) for a realm with two cities, three forces and nothing in flight. Target was about 150. Growth is bounded: Known places is capped (15 + 15 + 10 + 15 lines), Since you last read is capped at 30, and each city is 5 lines; units and runners are one line each and are the part that can grow.
- **Not in the example:** a failed order, an urgent notification, a foreign Wanax, a storm and a runner in flight do not exist on the rig (one player, no sight of anything else). They are covered by the `httptest` fixtures below.
- **Tests** (`server/cmd/keryx/cmd_dossier_test.go`, fake server answers only GET and returns 405 and records anything else):
  `TestDossierSectionsInFixedOrder` (exact order) · `TestDossierHeader` · `TestDossierEmptySectionsSayNone` (all nine empty sections start with "None.") · `TestDossierOrderFailedOnlyInFailedOrders` · `TestDossierUrgentKindsSeparated` · `TestDossierOnlyReadsAndNeverMarksRead` (no non-GET, no `read`/`read-all`, no `/admin/`) · `TestDossierSinceCapsAtThirty` (30 rows plus "…and 5 more — `keryx notifications --unread`") · `TestDossierCityBlockAndContent` · `TestDossierStormsNeedShipOrCoast` · `TestDossierOutWritesFileAndPrintsPathAndSize` (file overwritten; stdout is only `path (N bytes)`) · `TestDossierRejectsJSON` · `TestDossierIsRegisteredAndRootHasNoOtherFlags`.
- **Mutants** (committed first, restored after; killed = a red test on a `cmd_dossier_test.go:NN` line, not `[build failed]`):

  | Mutant | Result |
  |---|---|
  | swap "Contacts" and "Storms at sea" in `dossierSections` | killed, `cmd_dossier_test.go:124` |
  | `orNone` returns nil instead of "None." | killed, `cmd_dossier_test.go:155` (five sections) |
  | `buildDossier` POSTs `/notifications/read-all` | killed, `cmd_dossier_test.go:197` and `:201` |

- **Gates:** `python3 tools/day_guard_test.py` OK, `python3 tools/day_guard.py keryx` and `codex` PASS, `python3 tools/test_verb_parity.py` OK, `tools/test-web.sh` 920/920, `python3 tools/codex_facts.py --check` current.
- **Whole suite once, last:** `tools/gotest.sh` on a fresh DB (migration 249, commit `196fe28a`, `GOTEST_PORT=57911`): no failure, web 920/920.

## Observations (not fixed, not in scope)

- On the rig, wall-clock ETAs (`≈ 26 Oct 22:37`) are twelve days off real time: the world's `tick_anchor_at` sits on 2026-10-26 while ticks advance one per minute in real time; the server log said it had "absorbed" an 11.5-day downtime into the game clock. Every keryx ETA on that rig inherits it. Worth a look in `megaron_drift.md` terms; it does not touch the dossier logic.
- `locationStr` (`cmd_unit.go`) prints a settlement as `settlement 35f553f9…` (id, not name) for a garrisoned unit, and panics on an id shorter than 8 characters. The dossier replaces the id with the city name; `unit list` itself still prints the id.
- `unit list` prints `—` for an unset stance; the dossier leaves the stance out instead.

## Open questions

- Should "Urgent" also carry `BattleBegun`/`SiegeBegun`? The order named four sorts; I stayed with those plus the matching starvation and ship-foundering kinds.
- The dossier fetches `/units` and `/cities` twice (forces, known places, storms use them independently). Cheap, but one shared read would be tidier if the number of requests ever matters.
