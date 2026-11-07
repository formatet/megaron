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

PROOF_PLACEHOLDER
